package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"ape-player/internal/emby"
	"ape-player/internal/player"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type report struct {
	event    string
	position float64
	paused   bool
	playback emby.Playback
}
type reporter struct {
	ch   chan report
	stop chan report
	done chan struct{}
}

func newReporter(c *emby.Client, notify func(error)) *reporter {
	r := &reporter{make(chan report, 1), make(chan report, 1), make(chan struct{})}
	go func() {
		defer close(r.done)
		send := func(s report) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			e := c.Report(ctx, s.event, s.playback, s.position, s.paused)
			cancel()
			if e != nil && notify != nil {
				notify(e)
			}
		}
		// Always send Started before progress or Stopped, even for very short plays.
		first := <-r.ch
		send(first)
		for {
			var s report
			select {
			case s = <-r.stop:
			default:
				select {
				case s = <-r.stop:
				case s = <-r.ch:
				}
			}
			send(s)
			if s.event == "/Stopped" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = c.StopTranscode(ctx, s.playback)
				cancel()
				return
			}
		}
	}()
	return r
}
func (a *App) report(event string) {
	if a.reporting == nil {
		return
	}
	r := report{event, a.state.Position, a.state.Paused, a.playback}
	if event == "/Stopped" {
		a.pendingReportDone = a.reporting.done
		select {
		case a.reporting.stop <- r:
		default:
		}
		return
	}
	select {
	case a.reporting.ch <- r:
	default:
	}
}
func (a *App) stopPlayback() {
	a.playbackSerial++
	a.mediaLoading = false
	if a.player != nil {
		a.player.Command("stop")
		a.player.CloseUnused()
		if a.reporting == nil && a.client != nil && a.playback.SessionID != "" {
			c, p := a.client, a.playback
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = c.StopTranscode(ctx, p)
			}()
		}
		a.report("/Stopped")
		a.reporting = nil
		a.player = nil
		a.bitmap = nil
	}
	if a.releaseAwake != nil {
		a.releaseAwake()
		a.releaseAwake = nil
	}
	a.playerReady = false
	if a.win != nil && a.win.IsFullScreen() {
		a.win.SetFullScreen(false)
	}
}
func (a *App) play(item emby.Item) {
	if item.Type == "Series" {
		if len(a.episodes) > 0 {
			for _, ep := range a.episodes {
				if !ep.UserData.Played && ep.UserData.PlaybackPositionTicks > 0 {
					a.play(ep)
					return
				}
			}
			for _, ep := range a.episodes {
				if !ep.UserData.Played {
					a.play(ep)
					return
				}
			}
			a.play(a.episodes[0])
		}
		return
	}
	if item.Type != "Episode" {
		a.queueIndex = -1
		for i, v := range a.queue {
			if v.ID == item.ID {
				a.queueIndex = i
				break
			}
		}
		if a.queueIndex < 0 {
			a.queue = append(a.queue, item)
			a.queueIndex = len(a.queue) - 1
		}
		a.prepare(item, false, -1, nil, nil)
		return
	}
	// Fetch the complete series, rather than limiting next/previous to the visible season.
	a.busy = true
	c, ctx := a.client, a.ctx
	gen := a.generation
	a.playbackSerial++
	serial := a.playbackSerial
	go func() {
		r, e := c.List(ctx, "Shows/"+item.SeriesID+"/Episodes", map[string][]string{"UserId": {c.UserID}, "Fields": {"MediaSources"}})
		a.update(func() {
			if gen != a.generation || serial != a.playbackSerial {
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			sort.SliceStable(r.Items, func(i, j int) bool {
				x, y := r.Items[i], r.Items[j]
				if x.ParentIndexNumber != y.ParentIndexNumber {
					return x.ParentIndexNumber < y.ParentIndexNumber
				}
				return x.IndexNumber < y.IndexNumber
			})
			a.queue = r.Items
			a.queueIndex = 0
			for i, v := range a.queue {
				if v.ID == item.ID {
					a.queueIndex = i
				}
			}
			if len(a.queue) == 0 {
				a.queue = []emby.Item{item}
			}
			a.prepare(item, false, -1, nil, nil)
		})
	}()
}
func (a *App) prepare(item emby.Item, transcode bool, start float64, audio, sub *int) {
	a.stopPlayback()
	a.page = "player"
	a.busy = true
	a.message = ""
	a.playback = emby.Playback{Item: item}
	a.state = player.State{}
	a.controlsHidden = false
	a.speed = "1.0"
	a.audioChoice = "auto"
	a.subChoice = "auto"
	a.subDelay = 0
	c, ctx := a.client, a.ctx
	gen, serial := a.generation, a.playbackSerial
	mediaID := ""
	if a.detail.ID == item.ID {
		mediaID = a.version
	}
	bitrate := 0
	if s := a.active(); s != nil {
		bitrate = s.Bitrate
	}
	if start < 0 {
		start = float64(item.UserData.PlaybackPositionTicks) / emby.TicksPerSecond
	}
	go func() {
		p, e := c.Prepare(ctx, item, mediaID, bitrate, int64(start*emby.TicksPerSecond), audio, sub, transcode)
		a.update(func() {
			if gen != a.generation || serial != a.playbackSerial {
				if e == nil {
					go func() {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						_ = c.StopTranscode(ctx, p)
					}()
				}
				return
			}
			a.busy = false
			if e != nil {
				a.failure(e)
				return
			}
			a.playback = p
			a.openPlayer()
		})
	}()
}
func (a *App) startLocal(file string) {
	a.stopPlayback()
	a.page = "player"
	a.playback = emby.Playback{Item: emby.Item{Name: filepath.Base(file)}, URL: file, Method: "Local"}
	a.state = player.State{}
	a.audioChoice, a.subChoice = "auto", "auto"
	a.controlsHidden = false
	a.openPlayer()
}
func (a *App) openPlayer() {
	a.mediaLoading = true
	p, e := player.New(func() { a.win.Invalidate() })
	if e != nil {
		a.mediaLoading = false
		a.failure(e)
		return
	}
	a.player = p
	serial := a.playbackSerial
	a.playerReady = false
	// 1920×1080 is replaced with the source's native size when metadata is known.
	width, height := 1920, 1080

	a.bitmap = ui.NewGLBitmap(width, height, p.Render, func() {
		p.ReleaseGraphics()
		if a.player != p {
			p.Close()
		}
	})
	go func() {
		for e := range p.Events {
			a.update(func() {
				if a.player != p || serial != a.playbackSerial {
					return
				}
				a.playerEvent(e)
			})
		}
	}()
	go func() {
		time.Sleep(8 * time.Second)
		a.update(func() {
			if a.player == p && !a.playerReady {
				a.mediaLoading = false
				a.controlsHidden = false
				a.message = "GPU 未初始化，请检查 OpenGL 驱动；本播放器需要 GPU 渲染"
			}
		})
	}()
}
func (a *App) playerEvent(e player.Event) {
	switch e.Kind {
	case "ready":
		a.playerReady = true
		start := float64(a.playback.StartTicks) / emby.TicksPerSecond
		if a.playback.Method == "Transcode" {
			start = 0
		}
		a.player.Load(a.playback.URL, a.playback.Headers, start, a.cfg.Settings.Volume, a.cfg.Settings.AudioLanguage, a.cfg.Settings.SubtitleLanguage)
	case "loaded":
		a.mediaLoading = false
		a.state = e.State
		if a.playback.Method == "Transcode" {
			a.state.Position += float64(a.playback.StartTicks) / emby.TicksPerSecond
			a.state.Duration = float64(a.playback.Item.RunTimeTicks) / emby.TicksPerSecond
		}
		if a.bitmap != nil && e.State.Width > 0 && e.State.Height > 0 {
			a.bitmap.ResizeGL(e.State.Width, e.State.Height)
		}
		if a.playback.Method != "Local" {
			p := a.player
			a.reporting = newReporter(a.client, func(e error) {
				a.update(func() {
					if a.player == p {
						a.message = "播放进度同步失败：" + e.Error()
					}
				})
			})
			a.report("")
			r := a.reporting
			go func() {
				tick := time.NewTicker(10 * time.Second)
				defer tick.Stop()
				for {
					select {
					case <-r.done:
						return
					case <-tick.C:
						a.update(func() {
							if a.player == p && a.reporting == r {
								a.report("/Progress")
							}
						})
					}
				}
			}()
		}

		if a.releaseAwake == nil {
			a.releaseAwake = mygo.Power.KeepAwake("Ape Player 正在播放", true)
		}
		if a.playback.AudioIndex != nil && a.playback.Method != "Transcode" {
			a.selectStream("audio", *a.playback.AudioIndex)
		}
		if a.playback.SubtitleIndex != nil && a.playback.Method != "Transcode" {
			a.selectStream("sub", *a.playback.SubtitleIndex)
		}
	case "state":
		old := a.state.Paused
		a.state = e.State
		a.syncTrackChoices()
		if a.bitmap != nil && e.State.Width > 0 && e.State.Height > 0 {
			a.bitmap.ResizeGL(e.State.Width, e.State.Height)
		}
		if a.playback.Method == "Transcode" {
			a.state.Position += float64(a.playback.StartTicks) / emby.TicksPerSecond
			a.state.Duration = float64(a.playback.Item.RunTimeTicks) / emby.TicksPerSecond
		}
		if a.playerReady && old != a.state.Paused {
			a.report("/Progress")
			if a.state.Paused && a.releaseAwake != nil {
				a.releaseAwake()
				a.releaseAwake = nil
			} else if !a.state.Paused && a.releaseAwake == nil {
				a.releaseAwake = mygo.Power.KeepAwake("Ape Player 正在播放", true)
			}
		}
	case "ended":
		if a.state.Duration > 0 {
			a.state.Position = a.state.Duration
		}
		a.report("/Stopped")
		a.reporting = nil
		if a.cfg.Settings.AutoNext && a.queueIndex+1 < len(a.queue) {
			a.next(1)
		} else {
			a.stopPlayback()
			a.message = "播放完毕"
		}
	case "error":
		a.mediaLoading = false
		a.state.Buffering = false
		a.failure(e.Err)
	case "playback-error":
		a.stopPlayback()
		a.failure(e.Err)
	}
}
func (a *App) seek(seconds float64) {
	if a.player == nil {
		return
	}
	seconds = max(0, seconds)
	if a.state.Duration > 0 {
		seconds = min(seconds, a.state.Duration)
	}
	if a.playback.Method == "Transcode" {
		a.prepare(a.playback.Item, true, seconds, a.playback.AudioIndex, a.playback.SubtitleIndex)
		return
	}
	a.player.Command("seek", fmt.Sprintf("%.3f", seconds), "absolute+exact")
	a.state.Position = seconds
	a.report("/Progress")
}
func (a *App) next(delta int) {
	i := a.queueIndex + delta
	if i < 0 || i >= len(a.queue) {
		return
	}
	a.queueIndex = i
	a.prepare(a.queue[i], false, -1, nil, nil)
}
func (a *App) selectStream(kind string, index int) {
	if a.player == nil {
		return
	}
	property := "aid"
	if kind == "sub" {
		property = "sid"
		a.playback.SubtitleIndex = &index
	} else {
		a.playback.AudioIndex = &index
	}
	if a.playback.Method == "Transcode" {
		a.prepare(a.playback.Item, true, a.state.Position, a.playback.AudioIndex, a.playback.SubtitleIndex)
		return
	}
	if index < 0 {
		a.player.Command("set", property, "no")
		a.report("/Progress")
		return
	}
	for _, s := range a.playback.Source.MediaStreams {
		if s.Index != index || s.Type != streamType(kind) {
			continue
		}
		if s.IsExternal && kind == "sub" {
			subtitleURL := a.client.SubtitleURL(a.playback, s)
			for _, track := range a.state.Tracks {
				if track.Type == "sub" && track.External && track.ExternalFilename == subtitleURL {
					a.player.Command("set", "sid", fmt.Sprint(track.ID))
					a.report("/Progress")
					return
				}
			}
			a.player.Command("sub-add", subtitleURL, "select", streamLabel(s), s.Language)
			a.report("/Progress")
			return
		}
	}
	if id, ok := player.TrackID(a.state.Tracks, kind, index); ok {
		a.player.Command("set", property, fmt.Sprint(id))
		a.report("/Progress")
	} else {
		a.message = "当前媒体未提供该轨道，请选择其他音轨或字幕"
	}
}
func (a *App) addLocalSubtitle() {
	p := a.player
	go func() {
		files, e := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择字幕", Filters: []mygo.FileFilter{{Name: "字幕", Extensions: []string{"srt", "ass", "ssa", "vtt", "sub"}}}})
		a.update(func() {
			if a.player != p {
				return
			}
			if e != nil {
				a.failure(e)
				return
			}
			if len(files) > 0 {
				p.Command("sub-add", files[0], "select")
			}
		})
	}()
}

func (a *App) playQueueItem(index int) {
	if index < 0 || index >= len(a.queue) {
		return
	}
	a.queueIndex = index
	a.queueOpen = false
	a.prepare(a.queue[index], false, -1, nil, nil)
}
