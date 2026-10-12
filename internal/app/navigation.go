package app

import (
	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

// Keep browsing state with each history entry so Back restores the previous
// category, search, page and detail rather than opening a default media library.
type navigationState struct {
	busy                                                       bool
	page, query, sort, kind, watched, library, season, version string
	detailAudio, detailSub                                     string
	offset, total                                              int
	detail                                                     emby.Item
	items, seasons, episodes                                   []emby.Item
	sections                                                   []section
	episodeX                                                   float32
}

func (a *App) rememberPage() {
	switch a.page {
	case "home", "library", "favorites", "detail", "queue", "settings", "sources", "login":
	default:
		return // Retrying or changing episodes must not create player history.
	}
	entry := navigationState{
		busy: a.busy,
		page: a.page, query: a.query, sort: a.sort, kind: a.kind, watched: a.watched,
		library: a.library, season: a.season, version: a.version, offset: a.offset, total: a.total,
		detailAudio: a.detailAudio, detailSub: a.detailSub,
		detail: a.detail, items: a.items, seasons: a.seasons, episodes: a.episodes,
		sections: a.sections, episodeX: a.episodeScroll.X,
	}
	a.history = append(a.history, entry)
	if len(a.history) > 32 {
		a.history = a.history[len(a.history)-32:]
	}
}

func (a *App) goBack() {
	if len(a.history) == 0 {
		if a.canBrowse() && a.page != "home" {
			a.navigate("home")
			a.history = nil
		} else {
			a.navigate("sources")
		}
		return
	}
	entry := a.history[len(a.history)-1]
	a.history = a.history[:len(a.history)-1]
	if entry.page == "sources" {
		a.navigate("sources")
		return
	}
	wasPlayer := a.page == "player"
	a.stopPlayback()
	a.request++ // Ignore requests belonging to the page we just left.
	a.busy, a.episodePickerOpen = false, false
	a.message = ""
	a.page, a.query, a.sort, a.kind, a.watched = entry.page, entry.query, entry.sort, entry.kind, entry.watched
	a.library, a.season, a.version = entry.library, entry.season, entry.version
	a.detailAudio, a.detailSub = entry.detailAudio, entry.detailSub
	a.offset, a.total, a.detail = entry.offset, entry.total, entry.detail
	a.items, a.seasons, a.episodes, a.sections = entry.items, entry.seasons, entry.episodes, entry.sections
	a.grid, a.episodeGrid = ui.GridState{}, ui.GridState{}
	a.episodeScroll = ui.ScrollState{X: entry.episodeX}
	if entry.busy {
		switch a.page {
		case "home":
			a.loadHome()
		case "library", "favorites":
			a.loadItems()
		case "detail":
			a.openDetail(a.detail)
		}
		return
	}
	if wasPlayer && a.page == "home" {
		a.loadHome() // Refresh Continue Watching after the stop report finishes.
	} else if wasPlayer && a.page == "detail" && a.client != nil {
		a.refreshDetailAfterPlayback()
	}
}

func (a *App) refreshDetailAfterPlayback() {
	c, ctx, done := a.client, a.ctx, a.pendingReportDone
	id, req, gen := a.detail.ID, a.request, a.generation
	go func() {
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return
			}
		}
		item, err := c.Item(ctx, id)
		a.update(func() {
			if req != a.request || gen != a.generation {
				return
			}
			if err != nil {
				a.failure(err)
				return
			}
			a.mutateItem(id, func(saved *emby.Item) { saved.UserData = item.UserData })
		})
	}()
}
