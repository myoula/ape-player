package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func (a *App) poster(c *ui.Context, item emby.Item, w, h float32) {
	key := item.ID + ":" + item.ImageTags["Primary"]
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
		b, e := os.ReadFile(file)
		if e != nil {
			b, e = capi.Image(ctx, item)
			if e == nil {
				_ = os.MkdirAll(filepath.Dir(file), 0700)
				_ = os.WriteFile(file, b, 0600)
			}
		}
		var bitmap *ui.Bitmap
		if e == nil {
			bitmap, e = ui.DecodeBitmap(b)
		}
		a.update(func() {
			if gen != a.generation {
				return
			}
			if e == nil {
				if len(a.images) > 160 {
					a.images = map[string]*ui.Bitmap{}
					a.imagePending = map[string]bool{}
				}
				a.images[key] = bitmap
			}
		})
	}()
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
