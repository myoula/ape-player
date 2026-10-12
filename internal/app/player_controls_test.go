package app

import (
	"errors"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ape-player/internal/emby"
	"ape-player/internal/player"
	"github.com/egoist/mygo/ui"
)

func TestVideoClickTogglesControls(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	a.state.Paused = true
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("倍速") || !tt.HasText("返回") || !tt.HasText("详情") || !tt.HasText("播放列表") {
		t.Fatal("controls should start visible")
	}
	tt.ClickAt(640, 350)
	if !a.controlsHidden || tt.HasText("倍速") || tt.HasText("返回") || tt.HasText("详情") || tt.HasText("播放列表") {
		t.Fatal("video click did not hide controls while paused")
	}
	tt.Move(650, 355)
	tt.Frame()
	if !a.controlsHidden {
		t.Fatal("pointer movement undid explicit hide")
	}
	if tt.HasText("返回") {
		t.Fatal("top navigation should remain hidden")
	}
	tt.ClickAt(640, 350)
	if a.controlsHidden || !tt.HasText("倍速") || !tt.HasText("返回") || !tt.HasText("详情") || !tt.HasText("播放列表") {
		t.Fatal("second click did not show controls")
	}
	if err := tt.Click("音量"); err != nil {
		t.Fatal(err)
	}
	if a.controlsHidden {
		t.Fatal("control click toggled video surface")
	}
	if err := tt.Click("详情"); err != nil {
		t.Fatal(err)
	}
	if a.controlsHidden || !a.infoOpen {
		t.Fatal("top control click reached video surface")
	}
}

func TestQueueClickPlaysWithoutEditing(t *testing.T) {
	for _, page := range []string{"queue", "player"} {
		t.Run(page, func(t *testing.T) {
			a := testApp(t)
			requests := make(chan string, 1)
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- r.URL.Path; <-release }))
			defer srv.Close()
			defer close(release)
			defer a.cancel()
			a.client, _ = emby.New(srv.URL, "token", "user", "device", 5)
			a.page = page
			a.queueOpen = true
			a.queue = []emby.Item{{ID: "a", Name: "第一集", Type: "Episode"}, {ID: "b", Name: "第二集", Type: "Episode"}}
			tt := ui.NewTester(a.view, 1280, 820)
			for _, removed := range []string{"↑", "↓", "移除"} {
				if tt.HasText(removed) {
					t.Fatalf("queue still offers %s", removed)
				}
			}
			if err := tt.Click(title(a.queue[1])); err != nil {
				t.Fatal(err)
			}
			if a.queueIndex != 1 || a.queueOpen || a.page != "player" {
				t.Fatal("queue item did not switch playback and close panel")
			}
			select {
			case path := <-requests:
				if path != "/Items/b/PlaybackInfo" {
					t.Fatal(path)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("click did not request playback")
			}
		})
	}
}

func TestPlayerIconMenus(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	a.playback.Source.MediaStreams = []emby.Stream{
		{Index: 1, Type: "Audio", DisplayTitle: "English AC3 5.1"},
		{Index: 2, Type: "Audio", DisplayTitle: "Japanese AAC"},
		{Index: 3, Type: "Subtitle", DisplayTitle: "简体中文 ASS"},
	}
	tt := ui.NewTester(a.view, 940, 640)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("倍速")
	click("1.5×")
	if a.speed != "1.5" {
		t.Fatal("speed menu did not update playback speed")
	}
	click("音轨")
	click(streamLabel(a.playback.Source.MediaStreams[1]))
	if a.audioChoice != "emby:2" {
		t.Fatal("audio menu lost server track ID")
	}
	click("字幕")
	click(streamLabel(a.playback.Source.MediaStreams[2]))
	if a.subChoice != "emby:3" {
		t.Fatal("subtitle menu lost server track ID")
	}
	click("字幕")
	click("关闭字幕")
	if a.subChoice != "off" {
		t.Fatal("could not turn subtitles off")
	}
	click("字幕")
	if !tt.HasText("加载字幕") {
		t.Fatal("subtitle file action missing")
	}
	click("Increase")
	if a.subDelay != .1 || a.controlsHidden {
		t.Fatal("subtitle delay control failed inside popup")
	}
	tt.ClickAt(450, 300)
	if tt.HasText("字幕延迟") || a.controlsHidden {
		t.Fatal("outside click should dismiss menu without hiding controls")
	}
}

func TestQueueRevealsCurrentEpisode(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	for i := range 80 {
		a.queue = append(a.queue, emby.Item{ID: fmt.Sprint(i), Name: fmt.Sprintf("第 %d 集", i+1), Type: "Episode"})
	}
	// Match the playing item even if the cached queue position is stale.
	a.queueIndex = 0
	a.playback.Item = a.queue[60]
	tt := ui.NewTester(a.view, 1280, 820)
	open := func() {
		t.Helper()
		if err := tt.Click("播放列表"); err != nil {
			t.Fatal(err)
		}
	}
	visible := func() ui.Rect {
		t.Helper()
		r, ok := tt.Find("正在播放 · " + title(a.playback.Item))
		if !ok || r.Y < 72 || r.Y+r.H > 640 {
			t.Fatalf("current episode not revealed: %+v, found=%v", r, ok)
		}
		return r
	}
	open()
	r := visible()
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, -240)
	tt.Frame()
	moved, _ := tt.Find("正在播放 · " + title(a.playback.Item))
	if moved.Y <= r.Y {
		t.Fatal("automatic reveal prevented manual scrolling")
	}
	open() // close
	open() // reopen and reveal again
	visible()
	a.playback.Item = a.queue[70]
	tt.Frame()
	visible()
}

func TestPlaybackLoadingIndicator(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	a.busy = true
	tt := ui.NewTester(a.view, 1280, 820)
	check := func(want bool) {
		t.Helper()
		tt.Frame()
		r, found := tt.Find("正在加载媒体")
		if found != want {
			t.Fatalf("loading indicator visible=%v, want %v", found, want)
		}
		if found && (r.X+r.W/2 != 640 || r.Y+r.H/2 != 410) {
			t.Fatalf("loading indicator not centered: %+v", r)
		}
	}
	check(true) // PlaybackInfo request.
	a.busy = false
	a.mediaLoading = true
	a.bitmap = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 160, 90)))
	check(true) // GPU surface exists, but media is still opening.
	tt.ClickAt(640, 410)
	if !a.controlsHidden {
		t.Fatal("loading overlay blocked video click")
	}
	check(true)
	a.playback.Method = "Local"
	a.releaseAwake = func() {} // Avoid native power APIs in this headless test.
	a.playerEvent(player.Event{Kind: "loaded", State: player.State{Duration: 60}})
	check(false)
	a.player = &player.Player{}
	a.playerEvent(player.Event{Kind: "state", State: player.State{Duration: 60, Paused: true}})
	check(false) // Pausing alone is not loading.
	a.playerEvent(player.Event{Kind: "state", State: player.State{Duration: 60, Buffering: true}})
	check(true)
	a.playerEvent(player.Event{Kind: "state", State: player.State{Duration: 60}})
	check(false)
	a.mediaLoading = true
	a.playerEvent(player.Event{Kind: "error", Err: errors.New("test media load failure")})
	check(false)
	if a.controlsHidden || !tt.HasText("test media load failure") {
		t.Fatal("load error should replace the spinner with a visible error")
	}
	a.player = nil
	a.mediaLoading = true
	a.stopPlayback()
	check(false)
}
