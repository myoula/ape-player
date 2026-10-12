package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"ape-player/internal/diagnostics"
	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func (a *App) poster(c *ui.Context, item emby.Item, w, h float32) {
	key := emby.ImageKey(item)
	a.imageLastUsed[key] = a.imageFrame
	if b := a.images[key]; b != nil {
		ui.Image(c, b).Size(w, h).Fit(ui.Cover).Radius(8)
		return
	}
	ui.Column(c).Size(w, h).Radius(8).Background(ui.Hex("#252a35")).Center().Padding(14).Children(func() {
		ui.Text(c, "APE").FontSize(22).Bold().TextColor(ui.Hex("#576373"))
		ui.Text(c, item.Name).FontSize(12).TextColor(c.Theme().TextMuted)
	})
	if a.client == nil || a.imagePending[key] {
		return
	}
	a.imagePending[key] = true
	capi, ctx := a.client, a.ctx
	gen := a.generation
	cache, _ := os.UserCacheDir()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(a.cfg.Active+":"+capi.UserID+":"+key)))
	file := filepath.Join(cache, "ape-player", "posters", hash)
	go func() {
		bitmap, e := loadPoster(ctx, capi, item, file, a.imageSlots)
		if e != nil && ctx.Err() == nil {
			diagnostics.Log("poster.load_failed", "item_id", item.ID, "error", e)
		}
		a.update(func() {
			if e == nil {
				a.cachePoster(gen, key, bitmap)
			}
		})
	}()
}

// Episode shelves show scene thumbnails; portrait cards use series/season art.
func (a *App) thumbnail(c *ui.Context, item emby.Item, w, h float32) {
	item.Type, item.SeriesID, item.SeasonID = "", "", ""
	item.ParentPrimaryImageItemID, item.ParentPrimaryImageTag = "", ""
	a.poster(c, item, w, h)
}

// Limit simultaneous image work and retry transient failures without triggering
// requests on every frame. Failed or corrupt responses never enter disk cache.
func loadPoster(ctx context.Context, client *emby.Client, item emby.Item, file string, slots chan struct{}) (*ui.Bitmap, error) {
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if data, err := os.ReadFile(file); err == nil {
		if bitmap, err := ui.DecodeBitmap(data); err == nil {
			return bitmap, nil
		}
		_ = os.Remove(file)
	}
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * time.Second)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
		}
		var data []byte
		data, err = client.Image(ctx, item)
		if err == nil {
			var bitmap *ui.Bitmap
			bitmap, err = ui.DecodeBitmap(data)
			if err == nil {
				_ = os.MkdirAll(filepath.Dir(file), 0700)
				_ = os.WriteFile(file, data, 0600)
				return bitmap, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, emby.ErrImageNotFound) || errors.Is(err, emby.ErrUnauthorized) {
			break
		}
	}
	return nil, err
}

const posterCacheLimit = 160

func (a *App) cachePoster(generation uint64, key string, bitmap *ui.Bitmap) {
	if generation == a.generation {
		a.images[key] = bitmap
	}
}

// Keep every poster used by this view, even if a large home page exceeds the
// cache target. Evicting those posters would cause a reload on every frame.
// Trim only after the whole view is built, so later shelves are protected too.
func (a *App) trimPosterCache() {
	if len(a.images) <= posterCacheLimit {
		return
	}
	var unused []string
	for key := range a.images {
		if a.imageLastUsed[key] != a.imageFrame {
			unused = append(unused, key)
		}
	}
	sort.Slice(unused, func(i, j int) bool {
		return a.imageLastUsed[unused[i]] < a.imageLastUsed[unused[j]]
	})
	for _, key := range unused {
		if len(a.images) <= posterCacheLimit {
			break
		}
		delete(a.images, key)
		delete(a.imageLastUsed, key)
		delete(a.imagePending, key)
	}
}

// Bound the persistent poster cache. Filenames are opaque hashes; no credentials
// or server URLs are persisted alongside the images.
func prunePosters() {
	root, e := os.UserCacheDir()
	if e != nil {
		return
	}
	dir := filepath.Join(root, "ape-player", "posters")
	entries, e := os.ReadDir(dir)
	if e != nil {
		return
	}
	type entry struct {
		name     string
		size     int64
		modified int64
	}
	var files []entry
	var total int64
	for _, v := range entries {
		info, e := v.Info()
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, entry{v.Name(), info.Size(), info.ModTime().Unix()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified < files[j].modified })
	for i, f := range files {
		if total <= 256<<20 && len(files)-i <= 2048 {
			break
		}
		if os.Remove(filepath.Join(dir, f.name)) == nil {
			total -= f.size
		}
	}
}
