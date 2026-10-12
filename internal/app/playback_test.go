package app

import (
	"ape-player/internal/emby"
	"ape-player/internal/player"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestReporterOrdersStartStopAndCleansTranscode(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "token", "u", "d", 5)
	r := newReporter(c, nil)
	p := emby.Playback{SessionID: "s", Method: "Transcode"}
	r.ch <- report{event: "", playback: p}
	r.stop <- report{event: "/Stopped", playback: p}
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("reporter did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 || calls[0] != "POST /Sessions/Playing" || calls[1] != "POST /Sessions/Playing/Stopped" || calls[2] != "DELETE /Videos/ActiveEncodings" {
		t.Fatal(calls)
	}
}
func TestSourceGenerationRejectsStaleResult(t *testing.T) {
	a := testApp(t)
	old := a.generation
	ctx := a.ctx
	a.activate("")
	if old == a.generation || ctx.Err() != context.Canceled {
		t.Fatal("source generation not invalidated")
	}
}

type reportTransport func(*http.Request) (*http.Response, error)

func (f reportTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReporterTimeoutRecoveryAndStop(t *testing.T) {
	client, _ := emby.New("http://example.test", "token", "u", "d", 30)
	progress := 0
	blocked := make(chan struct{})
	client.HTTP.Transport = reportTransport(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || (req.URL.Path != "/Sessions/Playing/Stopped" && remaining < 25*time.Second) {
			t.Errorf("configured report timeout lost: %v", remaining)
		}
		if req.URL.Path == "/Sessions/Playing/Progress" {
			progress++
			if progress == 1 {
				return nil, context.DeadlineExceeded
			}
			if progress == 3 {
				close(blocked)
				<-req.Context().Done()
				return nil, req.Context().Err()
			}
		}
		if req.URL.Path == "/Sessions/Playing/Stopped" && (req.Context().Err() != nil || remaining > 5*time.Second) {
			t.Error("stop must use an independent bounded context")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	results := make(chan error, 8)
	r := newReporter(client, func(err error) { results <- err })
	a := testApp(t)
	a.reporting = r
	defer func() {
		a.report("/Stopped")
		select {
		case <-r.done:
		case <-time.After(2 * time.Second):
			t.Error("stopping did not cancel the pending request")
		}
	}()
	check := func(wantError bool) {
		t.Helper()
		select {
		case err := <-results:
			if (err != nil) != wantError {
				t.Fatalf("report result=%v, want error=%v", err, wantError)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("missing report result")
		}
	}
	a.report("")
	check(false)
	a.report("/Progress")
	check(true)
	a.report("/Progress")
	check(false) // Recovery must notify the UI too.
	a.report("/Progress")
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("progress request did not start")
	}
}

func TestReportWarningRecoversWithoutClearingPlaybackError(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	p := &player.Player{}
	a.player = p
	a.reportResult(p, a.playbackSerial, context.DeadlineExceeded)
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("播放进度暂未同步，将自动重试") || tt.HasText("原画重试") {
		t.Fatal("sync failure should have its own warning without playback retry")
	}
	a.message = "video decode failed"
	a.reportResult(p, a.playbackSerial, nil)
	tt.Frame()
	if a.reportMessage != "" || !tt.HasText("video decode failed") {
		t.Fatal("successful report must clear only the sync warning")
	}
	a.reportResult(p, a.playbackSerial, emby.ErrUnauthorized)
	if !strings.Contains(a.reportMessage, "重新登录") {
		t.Fatal("expired login needs actionable feedback")
	}
	a.reportMessage = ""
	a.playbackSerial++
	a.reportResult(p, a.playbackSerial-1, errors.New("late failure"))
	if a.reportMessage != "" {
		t.Fatal("old playback result leaked into a new session")
	}
}

func TestDetailRestartAndResume(t *testing.T) {
	for _, kind := range []string{"Movie", "Episode"} {
		for _, action := range []string{"播放", "从头播放", "继续观看"} {
			t.Run(kind+action, func(t *testing.T) {
				a := testApp(t)
				a.page = "detail"
				a.detail = emby.Item{ID: "item", Name: "测试影片", Type: kind, SeriesID: "series", IndexNumber: 2,
					UserData: emby.UserData{PlaybackPositionTicks: 120 * emby.TicksPerSecond}}
				a.version = "selected-version"
				if action == "播放" {
					a.detail.UserData.PlaybackPositionTicks = 0
				}
				savedPosition := a.detail.UserData.PlaybackPositionTicks
				a.detail.MediaSources = []emby.MediaSource{{ID: a.version, Name: "1080p", MediaStreams: []emby.Stream{
					{Index: 2, Type: "Audio", DisplayTitle: "国语"},
					{Index: 4, Type: "Subtitle", DisplayTitle: "简体中文", IsExternal: true},
				}}}
				requests := make(chan struct {
					StartTimeTicks                        int64
					MediaSourceID                         string
					AudioStreamIndex, SubtitleStreamIndex *int
				}, 1)
				release := make(chan struct{})
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/Shows/series/Episodes" {
						json.NewEncoder(w).Encode(emby.Items{Items: []emby.Item{{ID: "first", IndexNumber: 1}, a.detail, {ID: "third", IndexNumber: 3}}})
						return
					}
					if r.URL.Path != "/Items/item/PlaybackInfo" {
						t.Errorf("unexpected request: %s", r.URL.Path)
					}
					var body struct {
						StartTimeTicks                        int64
						MediaSourceID                         string
						AudioStreamIndex, SubtitleStreamIndex *int
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					requests <- body
					<-release
				}))
				defer srv.Close()
				defer close(release)
				defer a.cancel()
				a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
				a.imagePending[emby.ImageKey(a.detail)] = true
				tt := ui.NewTester(a.view, 1280, 900)
				choose := func(control, value string) {
					t.Helper()
					if err := tt.Click(control); err != nil {
						t.Fatal(err)
					}
					if err := tt.Click(value); err != nil {
						t.Fatal(err)
					}
				}
				choose("详情音轨", "2 · 国语")
				wantSub := 4
				if action == "从头播放" {
					choose("详情字幕", "关闭字幕")
					wantSub = -1
				} else {
					choose("详情字幕", "4 · 简体中文 · 外挂")
				}
				restart, restartOK := tt.Find("从头播放")
				resume, resumeOK := tt.Find("继续观看")
				if action != "播放" && (!restartOK || !resumeOK || restart.X >= resume.X) {
					t.Fatal("restart should precede resume")
				}
				if err := tt.Click(action); err != nil {
					t.Fatal(err)
				}
				select {
				case body := <-requests:
					want := int64(0)
					if action == "继续观看" {
						want = 120 * emby.TicksPerSecond
					}
					if body.StartTimeTicks != want || body.MediaSourceID != "selected-version" {
						t.Fatalf("wrong start or version: %+v", body)
					}
					if body.AudioStreamIndex == nil || *body.AudioStreamIndex != 2 || body.SubtitleStreamIndex == nil || *body.SubtitleStreamIndex != wantSub {
						t.Fatalf("detail track selections were not used: %+v", body)
					}
					if kind == "Episode" && (len(a.queue) != 3 || a.queueIndex != 1) {
						t.Fatal("restart lost episode navigation")
					}
					if a.detail.UserData.PlaybackPositionTicks != savedPosition {
						t.Fatal("button modified saved progress before playback")
					}
				case <-time.After(2 * time.Second):
					t.Fatal("playback request missing")
				}
			})
		}
	}
}
