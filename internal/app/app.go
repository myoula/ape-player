package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ape-player/internal/config"
	"ape-player/internal/emby"
	"ape-player/internal/player"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type section struct {
	Error     string
	Title     string
	Items     []emby.Item
	LibraryID string
	Kind      string
}
type App struct {
	onReady                                              func()
	cfg                                                  config.Config
	configPath                                           string
	win                                                  *mygo.Window
	theme                                                *ui.Theme
	client                                               *emby.Client
	ctx                                                  context.Context
	cancel                                               context.CancelFunc
	generation, request                                  uint64
	page, title                                          string
	history                                              []navigationState
	busy                                                 bool
	message                                              string
	sourceEditorOpen                                     bool
	sourceError                                          string
	draft                                                config.Source
	editing                                              string
	deleting                                             string
	username, password                                   string
	sourcePassword                                       string
	remember                                             bool
	sections                                             []section
	items, libraries, seasons, episodes                  []emby.Item
	detail                                               emby.Item
	query, sort, kind, watched, library, season, version string
	detailAudio, detailSub                               string
	total, offset                                        int
	grid                                                 ui.GridState
	episodeGrid                                          ui.GridState
	episodeScroll                                        ui.ScrollState
	episodePickerOpen                                    bool
	images                                               map[string]*ui.Bitmap
	imagePending                                         map[string]bool
	imageLastUsed                                        map[string]uint64
	imageFrame                                           uint64
	imageSlots                                           chan struct{}
	player                                               *player.Player
	bitmap                                               *ui.Bitmap
	playback                                             emby.Playback
	state                                                player.State
	queue                                                []emby.Item
	queueIndex                                           int
	queueOpen, infoOpen                                  bool
	queueRevealID                                        string
	speed, audioChoice, subChoice                        string
	aspectRatio                                          string
	subDelay                                             float64
	controlsHidden                                       bool
	reporting                                            *reporter
	reportMessage                                        string
	pendingReportDone                                    <-chan struct{}
	releaseAwake                                         func()
	playbackSerial                                       uint64
	localFile                                            string
	playerReady                                          bool
	mediaLoading                                         bool
	closing                                              bool
}

func New() (*App, error) {
	c, e := config.Load(config.Path())
	if e != nil {
		return nil, e
	}
	a := &App{cfg: c, configPath: config.Path(), page: "sources", sort: "最近添加", kind: "全部类型", watched: "全部状态", speed: "1.0", aspectRatio: "原始比例", images: map[string]*ui.Bitmap{}, imagePending: map[string]bool{}, remember: true}
	a.theme = ui.DarkTheme()
	a.imageLastUsed = map[string]uint64{}
	a.imageSlots = make(chan struct{}, 6)
	a.theme.Background = ui.Hex("#101217")
	a.theme.Surface = ui.Hex("#1b1e27")
	a.theme.Accent = ui.Hex("#c8ef80")
	a.theme.AccentText = ui.Hex("#132009")
	a.ctx, a.cancel = context.WithCancel(context.Background())
	go prunePosters()
	return a, nil
}
func (a *App) Run(local string) error {
	mygo.Theme.SetSource(mygo.ThemeDark)
	mygo.App.WhenReady(func() {
		// GTK has now applied the system locale; reset only numeric parsing for mpv.
		if err := player.ConfigureNumericLocale(); err != nil {
			a.failure(err)
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "Ape Player", Width: 1280, Height: 820, MinWidth: 940, MinHeight: 640, Content: ui.View(a.view)})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.closing {
				return
			}
			e.PreventDefault()
			a.closing = true
			oldPlayer, oldReporter := a.player, a.reporting
			a.stopPlayback()
			a.cancel()
			a.save()
			a.page = "closing"
			a.win.Invalidate()
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				if oldPlayer != nil {
					select {
					case <-oldPlayer.Done():
					case <-ctx.Done():
					}
				}
				if oldReporter != nil {
					select {
					case <-oldReporter.done:
					case <-ctx.Done():
					}
				}
				a.win.Update(func() { a.win.Close() })
			}()
		})
		if local != "" {
			a.localFile = local
			a.startLocal(local)
		}
		if a.onReady != nil {
			a.onReady()
		}
	})
	return mygo.App.Run()
}
func (a *App) save() {
	if e := config.Save(a.configPath, a.cfg); e != nil {
		a.message = "保存配置失败：" + e.Error()
	}
}
func (a *App) active() *config.Source {
	for i := range a.cfg.Sources {
		if a.cfg.Sources[i].ID == a.cfg.Active {
			return &a.cfg.Sources[i]
		}
	}
	return nil
}
func (a *App) update(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
	} else {
		fn()
	}
}
func (a *App) failure(e error) {
	if e == nil {
		return
	}
	a.message = e.Error()
	a.controlsHidden = false
	if e == emby.ErrUnauthorized {
		if s := a.active(); s != nil {
			s.Token = ""
			s.UserID = ""
		}
		a.save()
		a.activate(a.cfg.Active)
		a.message = e.Error()
	}

}
func (a *App) resetSource() {
	a.history = nil
	a.episodePickerOpen = false
	a.episodeScroll = ui.ScrollState{}
	a.stopPlayback()
	a.cancel()
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.generation++
	a.request++
	a.client = nil
	a.items = nil
	a.queue = nil
	a.queueIndex = 0
	a.sections = nil
	a.pendingReportDone = nil
	a.libraries = nil
	a.seasons = nil
	a.episodes = nil
	a.images = map[string]*ui.Bitmap{}
	a.imagePending = map[string]bool{}
	a.imageLastUsed = map[string]uint64{}
	a.imageFrame = 0
	a.message = ""
	a.busy = false
	a.library = ""
	a.query = ""
	a.offset = 0
	a.kind, a.watched, a.sort = "全部类型", "全部状态", "最近添加"
	a.version = ""
	a.resetDetailTracks()
	a.password = ""
	a.detail = emby.Item{}
	a.playback = emby.Playback{}
}
func (a *App) activate(id string) {
	a.resetSource()
	a.closeSourceEditor()
	a.cfg.Active = id
	s := a.active()
	if s == nil {
		a.page = "sources"
		return
	}
	a.username = s.Username
	a.password = ""
	a.remember = s.Remember
	c, e := emby.New(s.URL, s.Token, s.UserID, a.cfg.DeviceID, s.Timeout)
	if e != nil {
		a.failure(e)
		a.page = "sources"
		return
	}
	a.client = c
	a.save()
	if s.Token == "" {
		a.page = "login"
		return
	}
	a.page = "home"
	a.busy = true
	gen, req := a.generation, a.request
	ctx := a.ctx
	go func() {
		e := c.Validate(ctx)
		a.update(func() {
			if gen != a.generation || req != a.request {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
			} else {
				a.loadHome()
			}
		})
	}()
}
func (a *App) login() {
	if a.client == nil || a.busy {
		return
	}
	a.username = strings.TrimSpace(a.username)
	if a.username == "" {
		a.message = "请输入 Emby 账号（用户名）"
		return
	}
	a.busy = true
	a.message = ""
	c := a.client
	name, pw := a.username, a.password
	remember := a.remember
	gen := a.generation
	ctx := a.ctx
	go func() {
		auth, e := c.Login(ctx, name, pw)
		a.update(func() {
			if gen != a.generation {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			s := a.active()
			s.Token = auth.AccessToken
			s.UserID = auth.User.ID
			s.Username = name
			s.Remember = remember
			c.Token = s.Token
			c.UserID = s.UserID
			a.password = ""
			a.save()
			a.loadHome()
		})
	}()
}
func (a *App) logout() {
	c := a.client
	a.stopPlayback()
	if c != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = c.Logout(ctx)
		}()
	}
	if s := a.active(); s != nil {
		s.Token = ""
		s.UserID = ""
	}
	a.save()
	a.activate(a.cfg.Active)
}
func (a *App) navigate(page string) {
	a.navigateTo(page, nil)
}

func (a *App) navigateTo(page string, configure func()) {
	a.episodePickerOpen = false
	if page == "sources" {
		a.resetSource()
		a.closeSourceEditor()
		a.page = "sources"
		return
	}
	if a.page != page || configure != nil {
		a.rememberPage()
	}
	if configure != nil {
		configure()
	}
	a.stopPlayback()
	a.message = ""
	a.page = page
	a.request++
	a.busy = false
	switch page {
	case "home":
		a.loadHome()
	case "library", "favorites":
		a.offset = 0
		a.loadItems()
	}
}
func (a *App) loadItems() {
	if a.client == nil {
		return
	}
	a.busy = true
	a.request++
	req, gen := a.request, a.generation
	c, ctx := a.client, a.ctx
	q := map[string][]string{"Recursive": {"true"}, "IncludeItemTypes": {"Movie,Series"}, "StartIndex": {strconv.Itoa(a.offset)}, "Limit": {"60"}, "Fields": {"Overview,MediaSources," + emby.YearFields}, "SortOrder": {"Descending"}, "SortBy": {"DateCreated"}}
	if a.page == "favorites" {
		q["Filters"] = []string{"IsFavorite"}
		q["IncludeItemTypes"] = []string{"Movie,Series,Episode"}
	}
	if a.query != "" {
		q["SearchTerm"] = []string{a.query}
		q["IncludeItemTypes"] = []string{"Movie,Series,Episode"}
	}
	if a.library != "" {
		q["ParentId"] = []string{a.library}
	}
	if a.kind == "电影" {
		q["IncludeItemTypes"] = []string{"Movie"}
	} else if a.kind == "剧集" {
		q["IncludeItemTypes"] = []string{"Series"}
	}
	if a.watched != "全部状态" {
		q["IsPlayed"] = []string{strconv.FormatBool(a.watched == "已观看")}
	}
	if a.sort == "名称" {
		q["SortBy"] = []string{"SortName"}
		q["SortOrder"] = []string{"Ascending"}
	} else if a.sort == "年份" {
		q["SortBy"] = []string{"ProductionYear"}
	} else if a.sort == "最后一集添加时间" {
		q["SortBy"] = []string{"DateLastContentAdded"}
	}
	go func() {
		r, e := c.List(ctx, "Users/"+c.UserID+"/Items", q)
		a.update(func() {
			if gen != a.generation || req != a.request {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			a.items = r.Items
			a.total = r.TotalRecordCount
			a.grid = ui.GridState{}
		})
	}()
}
func (a *App) openDetail(item emby.Item) {
	if a.client == nil {
		return
	}
	if a.page != "detail" || a.detail.ID != item.ID {
		a.rememberPage()
	}
	a.stopPlayback()
	a.page = "detail"
	a.detail = item
	a.seasons = nil
	a.episodes = nil
	a.version = ""
	a.resetDetailTracks()
	a.season = ""
	a.episodePickerOpen = false
	a.episodeGrid = ui.GridState{}
	a.episodeScroll = ui.ScrollState{}
	a.busy = true
	a.message = ""
	a.request++
	req, gen := a.request, a.generation
	c, ctx := a.client, a.ctx
	go func() {
		d, e := c.Item(ctx, item.ID)
		var seasons, episodes emby.Items
		seasonID := ""
		if e == nil {
			seasons, episodes, seasonID, e = fetchDetailEpisodes(ctx, c, d)
		}
		a.update(func() {
			if gen != a.generation || req != a.request {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			a.detail = d
			a.seasons = seasons.Items
			a.episodes = episodes.Items
			a.season = seasonID
			if len(d.MediaSources) > 0 {
				a.version = d.MediaSources[0].ID
			}
		})
	}()
}
func (a *App) loadSeason(id string) {
	if a.client == nil {
		return
	}
	a.season = id
	a.episodes = nil
	a.episodeGrid = ui.GridState{}
	a.episodeScroll = ui.ScrollState{}
	a.message = ""
	a.busy = true
	a.request++
	req, gen := a.request, a.generation
	c, ctx := a.client, a.ctx
	series := detailSeries(a.detail)
	go func() {
		r, e := fetchEpisodes(ctx, c, series, id)
		a.update(func() {
			if gen != a.generation || req != a.request {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			a.episodes = r.Items
		})
	}()
}
func (a *App) toggleFavorite(item emby.Item) {
	c, ctx := a.client, a.ctx
	gen := a.generation
	on := !item.UserData.IsFavorite
	go func() {
		e := c.Favorite(ctx, item.ID, on)
		a.update(func() {
			if gen != a.generation {
				return
			}
			if e != nil {
				a.failure(e)
				return
			}
			a.mutateItem(item.ID, func(i *emby.Item) { i.UserData.IsFavorite = on })
		})
	}()
}
func (a *App) togglePlayed(item emby.Item) {
	a.setPlayed(item, !item.UserData.Played)
}
func (a *App) setPlayed(item emby.Item, on bool) {
	if a.client == nil {
		return
	}
	c, ctx := a.client, a.ctx
	gen := a.generation
	go func() {
		e := c.Played(ctx, item.ID, on)
		a.update(func() {
			if gen != a.generation {
				return
			}
			if e != nil {
				a.failure(e)
				return
			}
			a.mutateItem(item.ID, func(i *emby.Item) {
				i.UserData.Played = on
				i.UserData.PlaybackPositionTicks = 0
				i.UserData.PlayedPercentage = 0
				if on {
					i.UserData.PlayedPercentage = 100
				}
			})
			a.filterResumeSections()
		})
	}()
}
func (a *App) mutateItem(id string, f func(*emby.Item)) {
	if a.detail.ID == id {
		f(&a.detail)
	}
	for _, list := range [][]emby.Item{a.items, a.episodes, a.queue} {
		for i := range list {
			if list[i].ID == id {
				f(&list[i])
			}
		}
	}
	for s := range a.sections {
		for i := range a.sections[s].Items {
			if a.sections[s].Items[i].ID == id {
				f(&a.sections[s].Items[i])
			}
		}
	}
}
func (a *App) saveSource() {
	a.sourceError = ""
	c, e := emby.New(a.draft.URL, "", "", a.cfg.DeviceID, a.draft.Timeout)
	if e != nil {
		a.sourceError = e.Error()
		return
	}
	if strings.TrimSpace(a.draft.Name) == "" {
		a.draft.Name = c.Base.Host
	}
	a.draft.URL = c.Base.String()
	a.draft.Username = strings.TrimSpace(a.draft.Username)
	if a.draft.Timeout <= 0 {
		a.draft.Timeout = 20
	}
	if a.editing != "" {
		for i := range a.cfg.Sources {
			if a.cfg.Sources[i].ID == a.editing {
				if a.cfg.Sources[i].URL != a.draft.URL || a.cfg.Sources[i].Username != a.draft.Username || a.sourcePassword != "" {
					a.draft.Token = ""
					a.draft.UserID = ""
				}
				a.cfg.Sources[i] = a.draft
			}
		}
	} else {
		a.draft.ID = config.ID()
		a.draft.Remember = true
		a.cfg.Sources = append(a.cfg.Sources, a.draft)
	}
	id := a.draft.ID
	username, password := a.draft.Username, a.sourcePassword
	a.sourcePassword = ""
	a.draft = config.Source{}
	a.editing = ""
	a.save()
	a.activate(id)
	if a.page == "login" && username != "" {
		a.password = password
		a.login()
	}
}
func (a *App) deleteSource(id string) {
	for i, s := range a.cfg.Sources {
		if s.ID == id {
			a.cfg.Sources = append(a.cfg.Sources[:i], a.cfg.Sources[i+1:]...)
			break
		}
	}
	a.deleting = ""
	if a.cfg.Active == id {
		a.activate("")
	}
	a.save()
}
func duration(sec float64) string {
	n := max(0, int(sec))
	if n >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", n/3600, n/60%60, n%60)
	}
	return fmt.Sprintf("%02d:%02d", n/60, n%60)
}
func title(i emby.Item) string {
	if i.Type == "Episode" {
		return fmt.Sprintf("%s · S%02dE%02d %s", i.SeriesName, i.ParentIndexNumber, i.IndexNumber, i.Name)
	}
	return i.Name
}
func (a *App) canBrowse() bool { return a.client != nil && a.client.Token != "" }
