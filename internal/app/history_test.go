package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"ape-player/internal/config"
	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestContinueWatchingContextMenu(t *testing.T) {
	for _, tc := range []struct{ action, method, path string }{
		{"未观看", "DELETE", "/Users/u/PlayedItems/item"},
		{"已观看", "POST", "/Users/u/PlayedItems/item"},
		{"移除历史", "POST", "/Users/u/Items/item/HideFromResume?Hide=true"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			a := testApp(t)
			a.page = "home"
			item := emby.Item{ID: "item", Name: "测试影片"}
			a.sections = []section{{Title: "继续观看", Items: []emby.Item{item}}}
			a.imagePending[emby.ImageKey(item)] = true
			requests := make(chan string, 1)
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Emby-Token") != "token" {
					t.Error("missing authentication")
				}
				requests <- r.Method + " " + r.URL.RequestURI()
				<-release
				w.WriteHeader(500)
			}))
			defer srv.Close()
			defer close(release)
			defer a.cancel()
			a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
			tt := ui.NewTester(a.view, 1280, 820)
			if err := tt.RightClick("查看详情：测试影片"); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(tt.Menu(), []string{"未观看", "已观看", "-", "移除历史"}) || a.page != "home" {
				t.Fatalf("unexpected menu/navigation: %v %s", tt.Menu(), a.page)
			}
			if err := tt.ChooseMenuItem(tc.action); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-requests:
				if got != tc.method+" "+tc.path {
					t.Fatalf("wrong endpoint: %s", got)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("menu did not call server")
			}
			if len(a.sections[0].Items) != 1 || a.sections[0].Items[0].UserData.Played {
				t.Fatal("changed history before server success")
			}
		})
	}
}

func TestHiddenResumePersistsAndAllowsLaterPlayback(t *testing.T) {
	a := testApp(t)
	a.cfg.Active = "server"
	a.client, _ = emby.New("http://example.test", "token", "u", "d", 5)
	item := emby.Item{ID: "item", UserData: emby.UserData{PlaybackPositionTicks: 10 * emby.TicksPerSecond, LastPlayedDate: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}}
	a.sections = []section{{Title: "继续观看", Items: []emby.Item{item}}, {Title: "电视剧", LibraryID: "tv", Items: []emby.Item{item}}}
	a.finishHideResume(a.generation, a.resumeScope(), item, errors.New("offline"))
	if len(a.sections[0].Items) != 1 || len(a.cfg.HiddenResume) != 0 {
		t.Fatal("failed request removed history")
	}
	a.finishHideResume(a.generation, a.resumeScope(), item, nil)
	if len(a.sections[0].Items) != 0 || len(a.sections[1].Items) != 1 || a.sections[1].Items[0].UserData.PlaybackPositionTicks != 10*emby.TicksPerSecond {
		t.Fatal("removal deleted media or progress")
	}
	stored, err := config.Load(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg = stored
	a.sections[0].Items = continueWatching([]emby.Item{item}, []emby.Item{item}, 12)
	a.filterResumeSections()
	if len(a.sections[0].Items) != 0 {
		t.Fatal("recent history restored hidden item after reload")
	}
	item.UserData.LastPlayedDate = item.UserData.LastPlayedDate.Add(time.Hour)
	a.sections[0].Items = []emby.Item{item}
	a.filterResumeSections()
	if len(a.sections[0].Items) != 1 {
		t.Fatal("new play should reappear")
	}
	a.cfg.Active = "other"
	item.UserData.LastPlayedDate = item.UserData.LastPlayedDate.Add(-time.Hour)
	a.sections[0].Items = []emby.Item{item}
	a.filterResumeSections()
	if len(a.sections[0].Items) != 1 {
		t.Fatal("hidden items leaked across sources")
	}
	a.cfg.Active = "server"
	a.client.UserID = "other-user"
	a.filterResumeSections()
	if len(a.sections[0].Items) != 1 {
		t.Fatal("hidden items leaked across users")
	}
	a.finishHideResume(a.generation+1, a.resumeScope(), item, nil)
	if len(a.sections[0].Items) != 1 {
		t.Fatal("stale result changed current source")
	}
}
