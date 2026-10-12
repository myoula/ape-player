package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func episodeDetailFixture(t *testing.T, count int) *App {
	a := testApp(t)
	a.page = "detail"
	a.detail = emby.Item{ID: "series", Type: "Series", Name: "航海王", Overview: "一段关于伙伴与冒险的故事。"}
	a.seasons = []emby.Item{{ID: "s1", Name: "第1季 东海篇"}, {ID: "s2", Name: "第2季"}}
	a.season = "s1"
	for i := 1; i <= count; i++ {
		a.episodes = append(a.episodes, emby.Item{
			ID: fmt.Sprint(i), Type: "Episode", SeriesID: "series", SeriesName: a.detail.Name,
			Name: fmt.Sprintf("冒险的开始 · 第%d集", i), IndexNumber: i, ParentIndexNumber: 1,
			Overview:     "路飞与伙伴们一起踏上旅途，新的冒险即将展开。",
			PremiereDate: time.Date(1999, 10, 19, 0, 0, 0, 0, time.UTC), RunTimeTicks: 1500 * emby.TicksPerSecond,
		})
	}
	a.playback.Item = a.episodes[0]
	return a
}

func TestEpisodeCardsAndPicker(t *testing.T) {
	for _, size := range [][2]int{{1280, 900}, {940, 640}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a := episodeDetailFixture(t, 1000)
			tt := ui.NewTester(a.view, size[0], size[1])
			if !tt.HasText("1999-10-19 · 25:00") || !tt.HasText(a.episodes[0].Overview) {
				t.Fatal("episode card metadata missing")
			}
			if tt.HasText("播放剧集：" + title(a.episodes[12])) {
				t.Fatal("preview must bound poster requests")
			}
			if size[0] == 1280 {
				saveEpisodePreview(t, tt, "episode-detail.png")
			}
			if err := tt.Click("全部剧集"); err != nil {
				t.Fatal(err)
			}
			if !a.episodePickerOpen || !tt.HasText("共 1000 集") {
				t.Fatal("episode picker did not open")
			}
			for _, label := range []string{"关闭选集弹窗", "选集：" + title(a.episodes[0])} {
				r, found := tt.Find(label)
				if !found || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1]) {
					t.Fatalf("picker control outside viewport: %s %+v", label, r)
				}
			}
			saveEpisodePreview(t, tt, fmt.Sprintf("episode-picker-%d.png", size[0]))
			a.episodeGrid.ScrollTo(999, ui.End)
			tt.Frame()
			if !tt.HasText("选集：" + title(a.episodes[999])) {
				t.Fatal("last episode is inaccessible in a large season")
			}
			tt.Key(0, ui.KeyEscape)
			if a.episodePickerOpen || tt.HasText("全部剧集弹窗") {
				t.Fatal("Escape did not close episode picker")
			}
			if err := tt.Click("全部剧集"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("关闭选集弹窗"); err != nil {
				t.Fatal(err)
			}
			if a.episodePickerOpen {
				t.Fatal("close button did not dismiss picker")
			}
			if err := tt.Click("全部剧集"); err != nil {
				t.Fatal(err)
			}
			tt.ClickAt(10, 400)
			if a.episodePickerOpen {
				t.Fatal("backdrop did not dismiss picker")
			}
		})
	}
}

func saveEpisodePreview(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("APE_TEST_ARTIFACTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestEpisodePickerSeasonChangeClearsOldEpisodes(t *testing.T) {
	a := episodeDetailFixture(t, 8)
	a.detail = a.episodes[0]
	a.episodeScroll.X = 400
	a.episodePickerOpen = true
	tt := ui.NewTester(a.view, 1280, 900)
	requests := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Shows/series/Episodes" {
			t.Errorf("season change used the episode ID: %s", r.URL.Path)
		}
		requests <- r.URL.Query().Get("SeasonId")
		<-r.Context().Done()
	}))
	defer srv.Close()
	defer a.cancel()
	a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
	a.imagePending["series:"] = true
	for _, ep := range a.episodes {
		a.imagePending[ep.ID+":"] = true
	}
	if err := tt.Click("选集季度"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("第2季"); err != nil {
		t.Fatal(err)
	}
	if !a.episodePickerOpen || !a.busy || len(a.episodes) != 0 || a.episodeScroll.X != 0 || !tt.HasText("正在加载剧集…") {
		t.Fatal("season change must keep picker open and remove stale choices")
	}
	select {
	case season := <-requests:
		if season != "s2" {
			t.Fatalf("wrong requested season: %s", season)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("season change did not request episodes")
	}
}

func TestEpisodeShelfLazilyLoadsPosters(t *testing.T) {
	a := episodeDetailFixture(t, 1000)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	requests := make(chan string, 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		w.WriteHeader(http.StatusNotFound) // Use placeholders; no GUI state updates from background downloads.
	}))
	defer srv.Close()
	defer a.cancel()
	a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
	tt := ui.NewTester(a.view, 1280, 900)
	initial := len(a.imagePending)
	if initial < 2 || initial > 11 || a.imagePending["13:"] || a.imagePending["1000:"] {
		t.Fatalf("initial render fetched offscreen posters: %d", initial)
	}
	row, ok := tt.Find("剧集横排")
	if !ok || a.episodeScroll.MaxX < 200000 {
		t.Fatal("shelf must expose the entire season's scroll range")
	}
	maxX := a.episodeScroll.MaxX
	tt.Scroll(row.X+row.W/2, row.Y+40, 224*14, 0)
	tt.Frame()
	if !tt.HasText("播放剧集："+title(a.episodes[14])) || !a.imagePending["15:"] || tt.HasText("播放剧集："+title(a.episodes[0])) {
		t.Fatal("scrolling past the first 12 did not load and render the next episodes")
	}
	if len(a.imagePending) > initial+12 || a.episodeScroll.MaxX != maxX {
		t.Fatal("scrolling fetched the whole season or changed its scroll extent")
	}
	a.episodeScroll.X = math.MaxFloat32
	tt.Frame()
	if !tt.HasText("播放剧集："+title(a.episodes[999])) || !a.imagePending["1000:"] {
		t.Fatal("last episode is missing after scrolling to the end")
	}
	saveEpisodePreview(t, tt, "episode-shelf-end.png")
	loaded := len(a.imagePending)
	if loaded > 35 {
		t.Fatalf("too many posters loaded: %d", loaded)
	}
	a.episodeScroll.X = 0
	tt.Frame()
	if !tt.HasText("播放剧集："+title(a.episodes[0])) || len(a.imagePending) != loaded {
		t.Fatal("scrolling back must restore cards without duplicate image requests")
	}
	for range loaded {
		select {
		case path := <-requests:
			if path == "/Items/500/Images/Primary" {
				t.Fatal("unvisited episode image was requested")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("visible poster request was not sent")
		}
	}
}

func TestEpisodeScrollbarVisibleAndDraggable(t *testing.T) {
	a := episodeDetailFixture(t, 1000)
	tt := ui.NewTester(a.view, 1280, 900)
	row, ok := tt.Find("剧集横排")
	if !ok {
		t.Fatal("episode shelf missing")
	}
	x, y := row.X+12, row.Y+row.H-6
	tt.Move(20, 20)
	if tt.Image().RGBAAt(int(x), int(y)).R < 50 {
		t.Fatal("scrollbar is invisible before hovering the shelf")
	}
	saveEpisodePreview(t, tt, "episode-scrollbar.png")
	tt.Press(x, y)
	tt.Move(row.X+row.W-3, y)
	tt.Release(row.X+row.W-3, y)
	if a.episodeScroll.X != a.episodeScroll.MaxX || !tt.HasText("播放剧集："+title(a.episodes[999])) {
		t.Fatal("dragging the scrollbar did not reveal the last episode")
	}
	tt.Move(20, 20)
	if tt.Image().RGBAAt(int(row.X+row.W-12), int(y)).R < 50 {
		t.Fatal("scrollbar disappeared after dragging")
	}
	if a.page != "detail" || a.busy {
		t.Fatal("dragging scrollbar triggered playback")
	}
}

func TestEpisodeSelectionStartsChosenEpisode(t *testing.T) {
	for _, modal := range []bool{false, true} {
		t.Run(fmt.Sprint(modal), func(t *testing.T) {
			a := episodeDetailFixture(t, 20)
			a.episodePickerOpen = modal
			tt := ui.NewTester(a.view, 1280, 900)
			chosen := 1
			if !modal {
				chosen = 14
				a.episodeScroll.X = 224 * float32(chosen)
				tt.Frame()
			}
			requests := make(chan string, 1)
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/Shows/series/Episodes" {
					json.NewEncoder(w).Encode(emby.Items{Items: a.episodes})
					return
				}
				requests <- r.URL.Path
				<-release
			}))
			defer srv.Close()
			defer close(release)
			defer a.cancel()
			a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
			// Fixtures use placeholders without starting asynchronous image loads.
			a.imagePending["series:"] = true
			for _, ep := range a.episodes {
				a.imagePending[ep.ID+":"] = true
			}
			prefix := "播放剧集："
			if modal {
				prefix = "选集："
			}
			if err := tt.Click(prefix + title(a.episodes[chosen])); err != nil {
				t.Fatal(err)
			}
			select {
			case path := <-requests:
				if path != fmt.Sprintf("/Items/%d/PlaybackInfo", chosen+1) || a.episodePickerOpen || a.queueIndex != chosen {
					t.Fatalf("incorrect playback selection: %s, index=%d", path, a.queueIndex)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("selected episode did not reach playback")
			}
		})
	}
}

func TestFetchEpisodesMetadataAndSeason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/Shows/series/Episodes" || q.Get("Fields") != "MediaSources,Overview" || q.Get("EnableUserData") != "true" {
			t.Errorf("incorrect episode request: %s", r.URL)
		}
		if q.Get("SeasonId") != "s2" && q.Has("SeasonId") {
			t.Error("incorrect season")
		}
		w.Write([]byte(`{"Items":[{"Id":"b","IndexNumber":2,"PremiereDate":"1999-10-19T00:00:00Z","Overview":"简介"},{"Id":"a","IndexNumber":1}]}`))
	}))
	defer srv.Close()
	client, _ := emby.New(srv.URL, "token", "u", "d", 5)
	for _, season := range []string{"s2", ""} {
		items, err := fetchEpisodes(context.Background(), client, emby.Item{ID: "series", Name: "剧名"}, season)
		if err != nil {
			t.Fatal(err)
		}
		if len(items.Items) != 2 || items.Items[0].ID != "a" || items.Items[0].SeriesID != "series" || items.Items[0].Type != "Episode" || items.Items[1].PremiereDate.IsZero() || items.Items[1].Overview != "简介" {
			t.Fatalf("episode metadata or order lost: %+v", items)
		}
	}
}

func TestEpisodeDetailLoadsItsSeriesAndSeason(t *testing.T) {
	for _, tc := range []struct {
		name, seasonID, wantSeason string
		seasonNumber               int
	}{
		{"season ID", "s2", "s2", 2},
		{"season number fallback", "", "s2", 2},
		{"specials", "", "specials", 0},
		{"unknown season shows all", "", "", 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/Shows/series/Seasons":
					w.Write([]byte(`{"Items":[{"Id":"s1","Name":"第1季","IndexNumber":1},{"Id":"s2","Name":"第2季","IndexNumber":2},{"Id":"specials","Name":"特别篇","IndexNumber":0}]}`))
				case "/Shows/series/Episodes":
					if r.URL.Query().Get("SeasonId") != tc.wantSeason {
						t.Errorf("wrong season: %s", r.URL)
					}
					w.Write([]byte(`{"Items":[{"Id":"episode","Name":"当前集","IndexNumber":1,"ParentIndexNumber":2}]}`))
				default:
					t.Errorf("used episode ID instead of series ID: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			client, _ := emby.New(srv.URL, "token", "u", "d", 5)
			detail := emby.Item{ID: "episode", Type: "Episode", Name: "当前集", SeriesID: "series", SeriesName: "东北插班生", SeasonID: tc.seasonID, ParentIndexNumber: tc.seasonNumber}
			seasons, episodes, selected, err := fetchDetailEpisodes(context.Background(), client, detail)
			if err != nil {
				t.Fatal(err)
			}
			if selected != tc.wantSeason || len(seasons.Items) != 3 || len(episodes.Items) != 1 || episodes.Items[0].SeriesID != "series" {
				t.Fatalf("episode detail lost its series context: %s %+v", selected, episodes)
			}
			a := testApp(t)
			a.page, a.detail, a.season = "detail", detail, selected
			a.seasons, a.episodes = seasons.Items, episodes.Items
			tt := ui.NewTester(a.view, 1280, 900)
			if !tt.HasText("播放剧集：" + title(episodes.Items[0])) {
				t.Fatal("episode detail is missing episode cards")
			}
			if err := tt.Click("全部剧集"); err != nil {
				t.Fatal(err)
			}
			if !a.episodePickerOpen || !tt.HasText("东北插班生") || !tt.HasText("选集："+title(episodes.Items[0])) {
				t.Fatal("episode detail is missing its series picker")
			}
			if a.selectedEpisodeID() != detail.ID {
				t.Fatal("current detail episode should be highlighted")
			}
		})
	}
}
