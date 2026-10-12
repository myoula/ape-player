package app

import (
	"context"
	"errors"
	"sort"
	"sync"

	"ape-player/internal/emby"
)

type homeData struct {
	libraries []emby.Item
	sections  []section
	warning   string
}

// Wait for the last stop report before reading playback history. Cancellation
// keeps switching servers and signing out responsive during a slow report.
func fetchHome(ctx context.Context, c *emby.Client, after <-chan struct{}) (homeData, error) {
	var data homeData
	if after != nil {
		select {
		case <-after:
		case <-ctx.Done():
			return data, ctx.Err()
		}
	}
	libraries, e := c.List(ctx, "Users/"+c.UserID+"/Views", nil)
	if errors.Is(e, emby.ErrUnauthorized) {
		return data, e
	}
	if ctx.Err() != nil {
		return data, ctx.Err()
	}
	if e != nil {
		data.warning = "分类加载失败：" + e.Error()
	} else {
		data.libraries = libraries.Items
	}
	type homeLoader struct {
		section section
		load    func() ([]emby.Item, error)
	}
	loaders := []homeLoader{
		{section{Title: "最近播放"}, func() ([]emby.Item, error) { return c.RecentlyPlayed(ctx, 18) }},
		{section{Title: "继续观看"}, func() ([]emby.Item, error) {
			r, e := c.List(ctx, "Users/"+c.UserID+"/Items/Resume", map[string][]string{
				"Limit": {"12"}, "MediaTypes": {"Video"}, "EnableUserData": {"true"},
				"Fields": {"UserDataLastPlayedDate," + emby.YearFields}, "SortBy": {"DatePlayed"}, "SortOrder": {"Descending"},
			})
			return r.Items, e
		}},
		{section{Title: "最近添加"}, func() ([]emby.Item, error) {
			r, e := c.List(ctx, "Users/"+c.UserID+"/Items", map[string][]string{"Recursive": {"true"}, "IncludeItemTypes": {"Movie,Episode"}, "SortBy": {"DateCreated"}, "SortOrder": {"Descending"}, "Limit": {"18"}, "Fields": {"Overview," + emby.YearFields}})
			return r.Items, e
		}},
	}
	for _, library := range data.libraries {
		if library.ID == "" {
			continue
		}
		loaders = append(loaders, homeLoader{
			section: section{Title: library.Name, LibraryID: library.ID, Kind: libraryKind(library)},
			load: func() ([]emby.Item, error) {
				types := "Movie,Series"
				switch library.CollectionType {
				case "movies":
					types = "Movie"
				case "tvshows":
					types = "Series"
				}
				r, err := c.List(ctx, "Users/"+c.UserID+"/Items", map[string][]string{
					"ParentId": {library.ID}, "Recursive": {"true"}, "IncludeItemTypes": {types},
					"SortBy": {"DateCreated"}, "SortOrder": {"Descending"}, "Limit": {"18"}, "EnableUserData": {"true"},
					"Fields": {emby.YearFields},
				})
				return r.Items, err
			},
		})
	}
	// Bound requests while preserving the server's category order. One failed
	// shelf must not discard the successfully loaded history or other shelves.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]section, len(loaders))
	errs := make([]error, len(loaders))
	jobs := make(chan int, len(loaders))
	for i := range loaders {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(4, len(loaders)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				loader := loaders[i]
				s := loader.section
				s.Items, errs[i] = loader.load()
				if errs[i] != nil {
					s.Error = errs[i].Error()
					if errors.Is(errs[i], emby.ErrUnauthorized) {
						cancel()
					}
				}
				results[i] = s
			}
		})
	}
	workers.Wait()
	for _, err := range errs {
		if errors.Is(err, emby.ErrUnauthorized) {
			return data, err
		}
	}
	if ctx.Err() == nil && results[1].Error == "" {
		results[1].Items = continueWatching(results[0].Items, results[1].Items, 12)
	}
	// Recent plays supplement Continue Watching; they are not a separate shelf.
	data.sections = results[1:]
	if results[0].Error != "" {
		if data.warning != "" {
			data.warning += "\n"
		}
		data.warning += "部分观看记录加载失败：" + results[0].Error
	}
	return data, ctx.Err()
}

// Emby can record a play without retaining a resume position (for example a
// short viewing session). Keep those unfinished recent items visible as well.
func continueWatching(recent, resume []emby.Item, limit int) []emby.Item {
	seen := make(map[string]bool)
	items := make([]emby.Item, 0, len(recent)+len(resume))
	for _, group := range [][]emby.Item{recent, resume} {
		for _, item := range group {
			if item.ID == "" || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			if !item.UserData.Played {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UserData.LastPlayedDate.After(items[j].UserData.LastPlayedDate)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
func (a *App) loadHome() {
	if a.client == nil {
		return
	}
	a.page = "home"
	a.busy = true
	a.request++
	a.message = ""
	req, gen := a.request, a.generation
	c, ctx := a.client, a.ctx
	after := a.pendingReportDone
	go func() {
		data, e := fetchHome(ctx, c, after)
		a.update(func() {
			if gen != a.generation || req != a.request {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			a.libraries = data.libraries
			a.sections = data.sections
			a.message = data.warning
		})
	}()
}
func (a *App) openCategory(library, kind string) {
	a.navigateTo("library", func() {
		a.library = library
		a.kind = kind
		a.query = ""
		a.watched = "全部状态"
		a.sort = "最近添加"
		a.offset = 0
	})
}
func libraryKind(item emby.Item) string {
	switch item.CollectionType {
	case "movies":
		return "电影"
	case "tvshows":
		return "剧集"
	default:
		return "全部类型"
	}
}
