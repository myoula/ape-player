package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ape-player/internal/config"
	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestDetailBackRestoresEntryPage(t *testing.T) {
	for _, page := range []string{"home", "library", "favorites", "detail"} {
		t.Run(page, func(t *testing.T) {
			a := testApp(t)
			requests := make(chan struct{}, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- struct{}{}
				<-r.Context().Done()
			}))
			defer srv.Close()
			defer a.cancel()
			a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
			a.page, a.query, a.library, a.offset = page, "航海王", "anime", 120
			a.kind, a.sort, a.watched = "剧集", "名称", "未观看"
			a.detail = emby.Item{ID: "series", Type: "Series", Name: "航海王"}
			a.season, a.version = "season2", "version2"
			a.episodeScroll.X = 448
			a.items = []emby.Item{{ID: "cached", Name: "列表内容"}}
			a.episodes = []emby.Item{{ID: "episode", Type: "Episode", Name: "当前集"}}
			a.imagePending["episode:"], a.imagePending["series:"], a.imagePending["cached:"] = true, true, true
			originalClient, originalContext := a.client, a.ctx
			a.openDetail(a.episodes[0])
			select {
			case <-requests:
			case <-time.After(2 * time.Second):
				t.Fatal("detail request missing")
			}
			request := a.request
			tt := ui.NewTester(a.view, 1280, 900)
			if tt.HasText("返回媒体库") {
				t.Fatal("duplicate detail back bar remains")
			}
			if err := tt.Click("返回上一页"); err != nil {
				t.Fatal(err)
			}
			if a.page != page || a.library != "anime" || a.query != "航海王" || a.offset != 120 || a.sort != "名称" || a.watched != "未观看" {
				t.Fatalf("back lost browsing context: %s %s %s %d", a.page, a.library, a.query, a.offset)
			}
			if a.detail.ID != "series" || a.season != "season2" || a.version != "version2" {
				t.Fatal("nested detail context lost")
			}
			if a.client != originalClient || originalContext.Err() != nil || a.request <= request || a.busy || len(a.history) != 0 {
				t.Fatal("back left the service, kept a pending request active, or added history")
			}
		})
	}
}

func TestSidebarReturnPreservesLoginAndClearsHistory(t *testing.T) {
	a := testApp(t)
	a.page = "home"
	a.cfg.Active = "server"
	a.cfg.Sources = []config.Source{{ID: "server", Name: "家庭影院", Token: "saved-token", UserID: "u", Remember: true}}
	requests := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- r.URL.Path; w.WriteHeader(204) }))
	defer srv.Close()
	a.client, _ = emby.New(srv.URL, "saved-token", "u", "d", 5)
	a.save()
	a.navigate("queue")
	oldctx := a.ctx
	tt := ui.NewTester(a.view, 1280, 900)
	if tt.HasText("退出登录") {
		t.Fatal("footer still logs out")
	}
	for _, label := range []string{"来源管理", "偏好设置"} {
		if _, found := tt.Find(label); found {
			t.Fatalf("service sidebar still shows %s", label)
		}
	}
	if err := tt.Click("返回来源管理"); err != nil {
		t.Fatal(err)
	}
	if a.page != "sources" || a.client != nil || len(a.history) != 0 || oldctx.Err() != context.Canceled {
		t.Fatal("return did not leave service and clear navigation history")
	}
	if err := tt.Click("偏好设置"); err != nil {
		t.Fatal("global settings should remain accessible after leaving the service:", err)
	}
	stored, err := config.Load(a.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Sources[0].Token != "saved-token" || a.cfg.Sources[0].UserID != "u" {
		t.Fatal("return cleared saved login")
	}
	select {
	case path := <-requests:
		t.Fatalf("return unexpectedly called server: %s", path)
	default:
	}
}

func TestBackThroughSettingsPreservesLibraryFilters(t *testing.T) {
	a := testApp(t)
	a.page, a.library, a.query, a.offset = "library", "anime", "海贼", 60
	a.navigateTo("queue", func() { a.library = "" })
	a.navigate("settings")
	a.goBack()
	if a.page != "queue" {
		t.Fatal("settings back should return to queue")
	}
	a.goBack()
	if a.page != "library" || a.library != "anime" || a.query != "海贼" || a.offset != 60 || len(a.history) != 0 {
		t.Fatal("back did not restore the original library state")
	}
}

func TestPlayerBackRestoresDetailWithoutAddingHistory(t *testing.T) {
	a := testApp(t)
	a.page = "detail"
	a.detail = emby.Item{ID: "movie", Type: "Movie", Name: "电影"}
	a.version = "version2"
	requests := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		<-release
	}))
	defer srv.Close()
	defer close(release)
	defer a.cancel()
	a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
	a.prepare(a.detail, false, 0, nil, nil)
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("playback request missing")
	}
	// Returning to detail waits for the final progress report before refreshing.
	a.pendingReportDone = make(chan struct{})
	serial := a.playbackSerial
	tt := ui.NewTester(a.view, 1280, 900)
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	if a.page != "detail" || a.detail.ID != "movie" || a.version != "version2" || len(a.history) != 0 || a.playbackSerial <= serial {
		t.Fatal("player back did not restore detail and invalidate pending playback")
	}
}
