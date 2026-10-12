package app

import (
	"ape-player/internal/emby"
	"fmt"
	"strings"
	"time"
)

// Smoke runs the same native application and playback backend with a bounded
// interaction sequence. The caller supplies a local multitrack test video.
func (a *App) Smoke(file string) error {
	var failure string
	var frames uint64
	var pos float64
	var tracks int
	var cacheRanges int
	a.onReady = func() {
		if strings.HasPrefix(file, "emby:") {
			a.activate(a.cfg.Active)
		}
		go func() {
			if strings.HasPrefix(file, "emby:") {
				time.Sleep(300 * time.Millisecond)
				a.update(func() {
					if a.client == nil {
						failure = "no configured Emby source"
						return
					}
					a.prepare(emby.Item{ID: strings.TrimPrefix(file, "emby:"), Name: "Emby integration fixture", Type: "Movie", RunTimeTicks: 14 * emby.TicksPerSecond}, false, 0, nil, nil)
				})
			}
			time.Sleep(3 * time.Second)
			a.update(func() {
				if a.player == nil || !a.playerReady {
					failure = "GPU did not initialize: " + a.message
					return
				}
				a.player.Command("set", "mute", "yes")
				frames = a.player.FrameCount()
				pos = a.state.Position
				tracks = len(a.state.Tracks)
				cacheRanges = len(a.state.CacheRanges)
				a.player.Command("set", "pause", "yes")
				a.win.SetFullScreen(true)
			})
			time.Sleep(time.Second)
			a.update(func() {
				if a.player == nil {
					return
				}
				if !a.state.Paused {
					failure = "pause failed"
				}
				a.seek(7)
				for _, track := range a.state.Tracks {
					if (track.Type == "audio" && track.ID == 2) || (track.Type == "sub" && track.ID == 1) {
						a.chooseTrack(track.Type, a.trackKey(track))
					}
				}
				a.player.Command("set", "pause", "no")
				a.win.SetFullScreen(false)
			})
			time.Sleep(2 * time.Second)
			a.update(func() {
				if a.player != nil {
					frames = a.player.FrameCount()
					if a.state.Position < 7 {
						failure = "seek failed"
					}
					if a.message != "" {
						failure = a.message
					}
					if a.mediaLoading {
						failure = "media loading indicator did not clear after playback started"
					}
					audio, sub := false, false
					for _, track := range a.state.Tracks {
						if track.Type == "audio" && track.ID == 2 && track.Selected {
							audio = true
						}
						if track.Type == "sub" && track.ID == 1 && track.Selected {
							sub = true
						}
					}
					if !audio || !sub {
						failure = "track switch failed"
					}

					cacheRanges = max(cacheRanges, len(a.state.CacheRanges))
					if strings.HasPrefix(file, "emby:") && cacheRanges == 0 {
						failure = "HTTP playback did not expose buffered ranges"
					}
					fmt.Printf("gpu frames=%d position=%.2f tracks=%d hwdec=%s size=%dx%d dropped=%d cache_ranges=%d\n", frames, a.state.Position, len(a.state.Tracks), a.state.HWDec, a.state.Width, a.state.Height, a.state.DroppedFrames, cacheRanges)
				}
				a.win.Close()
			})
		}()
	}
	local := file
	if strings.HasPrefix(file, "emby:") {
		local = ""
	}
	if e := a.Run(local); e != nil {
		return e
	}
	if failure != "" {
		return fmt.Errorf("smoke: %s", failure)
	}
	if frames < 20 || pos <= 0 || tracks < 3 {
		return fmt.Errorf("smoke: incomplete playback frames=%d position=%.2f tracks=%d", frames, pos, tracks)
	}
	return nil
}
