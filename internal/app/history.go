package app

import (
	"time"

	"ape-player/internal/emby"
)

func (a *App) resumeScope() string {
	if a.client == nil {
		return ""
	}
	return a.cfg.Active + ":" + a.client.UserID
}

func (a *App) hideResume(item emby.Item) {
	if a.client == nil {
		return
	}
	c, ctx, gen := a.client, a.ctx, a.generation
	scope := a.resumeScope()
	go func() {
		err := c.HideFromResume(ctx, item.ID, true)
		a.update(func() {
			a.finishHideResume(gen, scope, item, err)
		})
	}()
}

func (a *App) finishHideResume(gen uint64, scope string, item emby.Item, err error) {
	if gen != a.generation {
		return
	}
	if err != nil {
		a.failure(err)
		return
	}
	if a.cfg.HiddenResume == nil {
		a.cfg.HiddenResume = map[string]map[string]time.Time{}
	}
	if a.cfg.HiddenResume[scope] == nil {
		a.cfg.HiddenResume[scope] = map[string]time.Time{}
	}
	lastPlayed := item.UserData.LastPlayedDate
	if lastPlayed.IsZero() {
		lastPlayed = time.Now().UTC()
	}
	a.cfg.HiddenResume[scope][item.ID] = lastPlayed
	a.save()
	a.filterResumeSections()
}

// The server hides Resume entries, but RecentlyPlayed may still return them.
// Remember the dismissed play per service/user; a later play is visible again.
func (a *App) filterResumeSections() {
	hidden := a.cfg.HiddenResume[a.resumeScope()]
	for s := range a.sections {
		section := &a.sections[s]
		if section.LibraryID != "" || section.Title != "继续观看" {
			continue
		}
		items := make([]emby.Item, 0, len(section.Items))
		for _, item := range section.Items {
			if item.UserData.Played {
				continue
			}
			if date, ok := hidden[item.ID]; ok && !item.UserData.LastPlayedDate.After(date) {
				continue
			}
			items = append(items, item)
		}
		section.Items = items
	}
}
