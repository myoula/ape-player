package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestHomeShelfDragKeepsCardClickAndMenu(t *testing.T) {
	a := testApp(t)
	a.page = "home"
	items := make([]emby.Item, 18)
	for i := range items {
		items[i] = emby.Item{ID: fmt.Sprint(i), Name: fmt.Sprintf("影片%d", i), Type: "Movie"}
		a.imagePending[emby.ImageKey(items[i])] = true
	}
	a.sections = []section{{Title: "继续观看", Items: items}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	defer a.cancel()
	a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
	tt := ui.NewTester(a.view, 1280, 900)
	label := "查看详情：" + title(items[1])
	before, ok := tt.Find(label)
	if !ok {
		t.Fatal("poster missing")
	}
	x, y := before.X+before.W/2, before.Y+60
	tt.Press(x, y)
	tt.Move(x-90, y)
	tt.Release(x-90, y)
	after, _ := tt.Find(label)
	if after.X != before.X-90 || a.page != "home" || a.busy {
		t.Fatalf("shelf did not pan or opened detail: before=%+v after=%+v page=%s", before, after, a.page)
	}
	if err := tt.RightClick(label); err != nil || len(tt.Menu()) == 0 {
		t.Fatalf("drag broke context menu: %v", err)
	}
	tt.CloseMenu()
	if err := tt.Click(label); err != nil || a.page != "detail" || a.detail.ID != items[1].ID {
		t.Fatalf("click after dragging did not open detail: %v", err)
	}
}

func TestHomeCategoryNavigationResetsFilters(t *testing.T) {
	for _, tc := range []struct{ label, library, kind string }{{"查看所有：动画", "anime", "剧集"}} {
		t.Run(tc.label, func(t *testing.T) {
			a := testApp(t)
			a.page = "home"
			a.libraries = []emby.Item{{ID: "anime", Name: "动画", CollectionType: "tvshows"}}
			a.sections = []section{{Title: "动画", LibraryID: "anime", Kind: "剧集", Items: []emby.Item{{ID: "series", Name: "动画剧集", Type: "Series"}}}}
			a.query = "old search"
			a.watched = "已观看"
			a.sort = "年份"
			a.offset = 60
			tt := ui.NewTester(a.view, 1280, 900)
			if e := tt.Click(tc.label); e != nil {
				t.Fatal(e)
			}
			if a.page != "library" || a.library != tc.library || a.kind != tc.kind {
				t.Fatalf("wrong category: %s %s %s", a.page, a.library, a.kind)
			}
			if a.query != "" || a.watched != "全部状态" || a.sort != "最近添加" || a.offset != 0 {
				t.Fatal("previous filters leaked into category")
			}
		})
	}
}
func TestHomeContinueWatchingView(t *testing.T) {
	a := testApp(t)
	a.page = "home"
	a.libraries = []emby.Item{{ID: "movies", Name: "电影收藏", CollectionType: "movies"}, {ID: "anime", Name: "动画", CollectionType: "tvshows"}}
	a.sections = []section{{Title: "继续观看", Items: []emby.Item{
		{ID: "recent", Name: "未看完的电影", ProductionYear: 2026, RunTimeTicks: 1000 * emby.TicksPerSecond, UserData: emby.UserData{PlaybackPositionTicks: 400 * emby.TicksPerSecond, LastPlayedDate: time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)}},
	}}}
	tt := ui.NewTester(a.view, 1280, 900)
	for _, s := range []string{"继续观看", "未看完的电影", "观看进度"} {
		if !tt.HasText(s) {
			t.Errorf("missing %s", s)
		}
	}
	if tt.HasText("最近播放") {
		t.Fatal("duplicate history shelf remains visible")
	}
	for _, label := range []string{"分类", "我的媒体库", "分类：全部媒体", "媒体库分类：动画"} {
		if _, found := tt.Find(label); found {
			t.Fatalf("removed home category panel still shows %s", label)
		}
	}
	if dir := os.Getenv("APE_TEST_ARTIFACTS"); dir != "" {
		if e := os.MkdirAll(dir, 0755); e != nil {
			t.Fatal(e)
		}
		f, e := os.Create(filepath.Join(dir, "home.png"))
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		if e = png.Encode(f, tt.Image()); e != nil {
			t.Fatal(e)
		}
	}
	a.sections = []section{{Title: "继续观看"}}
	tt.Frame()
	if !tt.HasText("暂无可继续观看的内容，播放电影或剧集后会显示在这里。") {
		t.Fatal("history empty state missing")
	}
}
func TestHomePartialFailurePreservesCategoriesAndHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Users/u/Views":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"movies","Name":"电影","CollectionType":"movies"}]}`))
		case r.URL.Query().Get("SortBy") == "DatePlayed":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"recent","UserData":{"LastPlayedDate":"2026-10-12T08:00:00Z"}}]}`))
		case r.URL.Query().Get("ParentId") == "movies":
			w.WriteHeader(500)
		default:
			_, _ = w.Write([]byte(`{"Items":[]}`))
		}
	}))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "token", "u", "d", 5)
	data, e := fetchHome(context.Background(), c, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.libraries) != 1 || len(data.sections) != 2 || len(data.sections[0].Items) != 1 {
		t.Fatalf("lost successful sections: %+v", data)
	}
	if data.sections[1].Error == "" {
		t.Fatal("failed section not marked")
	}
}
func TestHomeWaitsForReportAndCanCancel(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); _, _ = w.Write([]byte(`{"Items":[]}`)) }))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "token", "u", "d", 5)
	after := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, e := fetchHome(ctx, c, after); result <- e }()
	select {
	case e := <-result:
		t.Fatalf("history loaded before report: %v", e)
	case <-time.After(30 * time.Millisecond):
	}
	if count.Load() != 0 {
		t.Fatal("history requested before reporting finished")
	}
	close(after)
	select {
	case e := <-result:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not refresh after report")
	}
	if count.Load() != 3 {
		t.Fatalf("expected all sections, got %d requests", count.Load())
	}
	blocked := make(chan struct{})
	cancel()
	if _, e := fetchHome(ctx, c, blocked); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation: %v", e)
	}
}

func TestHomeLibraryShelves(t *testing.T) {
	libraries := []emby.Item{
		{ID: "cn", Name: "国产剧", CollectionType: "tvshows"},
		{ID: "jp", Name: "日韩剧", CollectionType: "tvshows"},
		{ID: "movies", Name: "电影", CollectionType: "movies"},
		{ID: "mixed", Name: "最近播放"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Users/u/Views" {
			json.NewEncoder(w).Encode(emby.Items{Items: libraries})
			return
		}
		parent := r.URL.Query().Get("ParentId")
		if parent == "" {
			if r.URL.Query().Get("SortBy") == "DateCreated" {
				t.Error("removed recently added shelf still requested")
			}
			json.NewEncoder(w).Encode(emby.Items{})
			return
		}
		q := r.URL.Query()
		for _, field := range []string{"ProductionYear", "PremiereDate", "EndDate", "Status"} {
			if !slices.Contains(strings.Split(q.Get("Fields"), ","), field) {
				t.Errorf("category request missing %s", field)
			}
		}
		want := "Series"
		if parent == "movies" {
			want = "Movie"
		}
		if parent == "mixed" {
			want = "Movie,Series"
		}
		if r.URL.Path != "/Users/u/Items" || q.Get("IncludeItemTypes") != want || q.Get("Recursive") != "true" || q.Get("Limit") != "18" || q.Get("SortBy") != "DateCreated" || q.Get("SortOrder") != "Descending" || q.Get("EnableUserData") != "true" {
			t.Errorf("incorrect category query: %s", r.URL.String())
		}
		if parent == "jp" {
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(emby.Items{Items: []emby.Item{{ID: parent + "-item", Name: parent + " 的影片", Type: "Series", ProductionYear: 2026, Status: "Continuing"}}})
	}))
	defer srv.Close()
	client, _ := emby.New(srv.URL, "token", "u", "d", 5)
	data, err := fetchHome(context.Background(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.sections) != 5 {
		t.Fatalf("missing shelves: %d", len(data.sections))
	}
	for i, library := range libraries {
		shelf := data.sections[i+1]
		if shelf.LibraryID != library.ID || shelf.Title != library.Name || shelf.Kind != libraryKind(library) {
			t.Fatalf("wrong shelf order or navigation: %+v", shelf)
		}
		if library.ID == "jp" {
			if shelf.Error == "" {
				t.Fatal("failed category should have its own error")
			}
		} else if shelf.Error != "" || len(shelf.Items) != 1 || shelf.Items[0].ID != library.ID+"-item" {
			t.Fatalf("category items mixed or lost: %+v", shelf)
		}
	}
	a := testApp(t)
	a.page = "home"
	a.libraries = data.libraries
	a.sections = data.sections[1:]
	tt := ui.NewTester(a.view, 1280, 900)
	for _, library := range libraries {
		if !tt.HasText("查看所有：" + library.Name) {
			t.Fatalf("missing category navigation: %s", library.Name)
		}
	}
	if tt.HasText("观看进度") {
		t.Fatal("a library named 最近播放 should not become a history shelf")
	}
	if !tt.HasText("cn 的影片") || !tt.HasText("mixed 的影片") || !tt.HasText("加载失败") {
		t.Fatal("category content or partial error missing")
	}
	if _, zero := tt.Find("0"); !tt.HasText("2026–现在") || zero {
		t.Fatal("category year or continuing status missing")
	}
}

func TestHomeCategoryUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Users/u/Views" {
			w.Write([]byte(`{"Items":[{"Id":"tv","Name":"剧集","CollectionType":"tvshows"}]}`))
			return
		}
		if r.URL.Query().Get("ParentId") == "tv" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"Items":[]}`))
	}))
	defer srv.Close()
	client, _ := emby.New(srv.URL, "token", "u", "d", 5)
	if _, err := fetchHome(context.Background(), client, nil); !errors.Is(err, emby.ErrUnauthorized) {
		t.Fatalf("authorization failure lost: %v", err)
	}
}

func TestHomeIncludesRecentPlayMissingFromResume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Users/u/Items/Resume":
			q := r.URL.Query()
			if !slices.Contains(strings.Split(q.Get("Fields"), ","), "UserDataLastPlayedDate") || q.Get("EnableUserData") != "true" || q.Get("SortBy") != "DatePlayed" {
				t.Errorf("resume metadata not requested: %v", q)
			}
			w.Write([]byte(`{"Items":[
    {"Id":"existing","Name":"已在续播中","UserData":{"PlaybackPositionTicks":200000000,"LastPlayedDate":"2026-10-11T10:00:00Z"}},
    {"Id":"older","Name":"更早的续播","UserData":{"PlaybackPositionTicks":500000000,"LastPlayedDate":"2026-10-10T10:00:00Z"}}
   ]}`))
		case r.URL.Query().Get("SortBy") == "DatePlayed":
			w.Write([]byte(`{"Items":[
    {"Id":"new","Type":"Episode","Name":"东北小伙转学被堵厕所","SeriesName":"东北插班生","UserData":{"PlaybackPositionTicks":0,"Played":false,"LastPlayedDate":"2026-10-12T10:00:00Z"}},
    {"Id":"done","UserData":{"Played":true,"LastPlayedDate":"2026-10-12T09:00:00Z"}},
    {"Id":"existing","UserData":{"PlaybackPositionTicks":200000000,"LastPlayedDate":"2026-10-11T10:00:00Z"}}
   ]}`))
		default:
			w.Write([]byte(`{"Items":[]}`))
		}
	}))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "token", "u", "d", 5)
	data, err := fetchHome(context.Background(), c, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, shelf := range data.sections {
		if shelf.LibraryID == "" && shelf.Title == "最近播放" {
			t.Fatal("recent history should only supplement continue watching")
		}
	}
	items := data.sections[0].Items
	if len(items) != 3 || items[0].ID != "new" || items[1].ID != "existing" || items[2].ID != "older" {
		t.Fatalf("missing, duplicated, completed or unordered resume entries: %+v", items)
	}
	if items[0].UserData.PlaybackPositionTicks != 0 || items[1].UserData.PlaybackPositionTicks != 200000000 {
		t.Fatal("resume positions must retain server values")
	}
	if got := continueWatching(items, nil, 1); len(got) != 1 || got[0].ID != "new" {
		t.Fatal("limit discarded the newest episode")
	}
	a := testApp(t)
	a.page = "home"
	a.sections = data.sections
	tt := ui.NewTester(a.view, 1280, 900)
	if !tt.HasText("东北插班生") || !tt.HasText("观看进度") {
		t.Fatal("new episode or progress missing from continue watching")
	}
}

func TestWatchProgress(t *testing.T) {
	for _, tc := range []struct {
		name string
		item emby.Item
		want float64
	}{
		{"ticks", emby.Item{RunTimeTicks: 1000, UserData: emby.UserData{PlaybackPositionTicks: 400, PlayedPercentage: 5}}, .4},
		{"percentage only", emby.Item{UserData: emby.UserData{PlayedPercentage: 55}}, .55},
		{"zero position with percentage", emby.Item{RunTimeTicks: 1000, UserData: emby.UserData{PlayedPercentage: 25}}, .25},
		{"completed with reset position", emby.Item{RunTimeTicks: 1000, UserData: emby.UserData{Played: true}}, 1},
		{"overflow", emby.Item{RunTimeTicks: 1000, UserData: emby.UserData{PlaybackPositionTicks: 2000}}, 1},
		{"negative", emby.Item{UserData: emby.UserData{PlayedPercentage: -5}}, 0},
		{"unknown", emby.Item{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := watchProgress(tc.item); got != tc.want {
				t.Fatalf("progress=%v want=%v", got, tc.want)
			}
		})
	}
}
