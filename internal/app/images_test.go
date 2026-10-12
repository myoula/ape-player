package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestPosterRecoversFromCorruptCacheAndTemporaryFailure(t *testing.T) {
	var calls atomic.Int32
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.Write(data.Bytes())
	}))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "", "u", "d", 5)
	file := filepath.Join(t.TempDir(), "poster")
	if err := os.WriteFile(file, []byte("corrupt image"), 0600); err != nil {
		t.Fatal(err)
	}
	slots := make(chan struct{}, 1)
	item := emby.Item{ID: "movie"}
	bitmap, err := loadPoster(context.Background(), c, item, file, slots)
	if err != nil || bitmap == nil || calls.Load() != 2 {
		t.Fatalf("recovery failed: bitmap=%v err=%v calls=%d", bitmap, err, calls.Load())
	}
	if _, err = loadPoster(context.Background(), c, item, file, slots); err != nil || calls.Load() != 2 {
		t.Fatal("valid cache should avoid network requests")
	}
}

func TestEpisodeThumbnailsUseEpisodeImages(t *testing.T) {
	a := testApp(t)
	ep := emby.Item{ID: "episode", Type: "Episode", SeriesID: "series", SeasonID: "season", ImageTags: map[string]string{"Primary": "episode-tag"}, SeriesPrimaryImageTag: "series-tag"}
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 3)))
	a.images[emby.ImageKey(ep)] = bitmap
	a.images["episode:episode-tag"] = bitmap
	tt := ui.NewTester(func(c *ui.Context) { a.thumbnail(c, ep, 208, 117) }, 300, 200)
	if _, placeholder := tt.Find("APE"); placeholder {
		t.Fatal("episode thumbnail used series poster key")
	}
	if _, used := a.imageLastUsed["episode:episode-tag"]; !used {
		t.Fatal("thumbnail must use episode image")
	}
	tt = ui.NewTester(func(c *ui.Context) { a.poster(c, ep, 156, 222) }, 300, 300)
	if _, placeholder := tt.Find("APE"); placeholder {
		t.Fatal("portrait card used thumbnail key")
	}
	if _, used := a.imageLastUsed[emby.ImageKey(ep)]; !used {
		t.Fatal("portrait card must use inherited poster")
	}
}

func TestLargeHomePostersStayLoaded(t *testing.T) {
	a := testApp(t)
	a.page = "home"
	const count = 12 * 18 // A home page with twelve library shelves.
	for shelf := range 12 {
		s := section{Title: fmt.Sprintf("分类 %d", shelf), LibraryID: fmt.Sprint(shelf)}
		for i := range 18 {
			id := fmt.Sprint(shelf*18 + i)
			s.Items = append(s.Items, emby.Item{ID: id, Name: "影片 " + id})
			a.imagePending[id+":"] = true
		}
		a.sections = append(a.sections, s)
	}
	a.imagePending["still-loading:"] = true
	a.imagePending["no-cover:"] = true
	tt := ui.NewTester(a.view, 1280, 820)
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 3)))
	// Deliver image completions in batches, with real view rebuilds between them.
	for i := range count {
		a.cachePoster(a.generation, fmt.Sprint(i)+":", bitmap)
		if i%18 == 17 {
			tt.Frame()
		}
		for loaded := 0; loaded <= i; loaded++ {
			key := fmt.Sprint(loaded) + ":"
			if a.images[key] != bitmap || !a.imagePending[key] {
				t.Fatalf("loading poster %d discarded an already displayed poster %s", i, key)
			}
		}
	}
	for range 3 {
		tt.Frame()
		if len(a.images) != count {
			t.Fatal("redraw evicted posters used by the current home page")
		}
		if _, placeholder := tt.Find("APE"); placeholder {
			t.Fatal("loaded cover reverted to placeholder")
		}
	}
	if !a.imagePending["still-loading:"] || !a.imagePending["no-cover:"] {
		t.Fatal("cache pressure cleared in-flight or failed request records")
	}
	a.page = "sources"
	tt.Frame()
	if len(a.images) != posterCacheLimit {
		t.Fatalf("leaving large home did not trim cache: %d", len(a.images))
	}
	if !a.imagePending["still-loading:"] || !a.imagePending["no-cover:"] {
		t.Fatal("eviction changed unrelated request records")
	}
}

func TestPosterCacheEvictsOldestUnusedAndIgnoresOldSource(t *testing.T) {
	a := testApp(t)
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 3)))
	for i := range posterCacheLimit + 2 {
		key := fmt.Sprint(i)
		a.cachePoster(a.generation, key, bitmap)
		a.imageLastUsed[key] = uint64(i + 1)
		a.imagePending[key] = true
	}
	a.imageFrame = 1000
	a.imageLastUsed["0"] = a.imageFrame // The oldest poster is on this page.
	a.trimPosterCache()
	if a.images["0"] != bitmap || a.images["3"] != bitmap || len(a.images) != posterCacheLimit {
		t.Fatal("eviction lost active/recent posters or failed to bound unused cache")
	}
	for _, key := range []string{"1", "2"} {
		if a.images[key] != nil || a.imagePending[key] {
			t.Fatal("oldest unused image must be evicted and allowed to reload")
		}
	}
	oldGeneration := a.generation
	a.resetSource()
	a.cachePoster(oldGeneration, "late", bitmap)
	if len(a.images) != 0 || len(a.imageLastUsed) != 0 || len(a.imagePending) != 0 {
		t.Fatal("old source completion leaked into the new cache")
	}
}
