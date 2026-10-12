package app

import (
	"ape-player/internal/emby"
	"ape-player/internal/player"
	"context"
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
