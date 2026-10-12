package app

import (
	"math"

	"ape-player/internal/emby"
	"ape-player/internal/player"
	"github.com/egoist/mygo/ui"
)

func (a *App) playbackProgress(c *ui.Context) {
	pos := a.state.Position
	offset := 0.0
	if a.playback.Method == "Transcode" {
		offset = float64(a.playback.StartTicks) / emby.TicksPerSecond
	}
	bufferedSlider(c, &pos, a.state.Duration, a.state.CacheRanges, offset).
		Label("播放进度").Disabled(a.player == nil || a.state.Duration <= 0).
		OnChange(func() { a.seek(pos) })
}

func bufferedSlider(c *ui.Context, position *float64, duration float64, ranges []player.CacheRange, offset float64) ui.Element {
	t := c.Theme()
	kw, kh := t.Space(5), t.Space(4)
	slider := ui.SliderBase(c, position, 0, max(1, duration)).FillWidth().
		Height(t.Space(5)).PaddingX(kw / 2)
	return slider.Draw(func(p *ui.Painter, r ui.Rect) {
		h, cy := t.Space(1.5), r.Y+r.H/2
		track := ui.Rect{X: r.X, Y: cy - h/2, W: r.W, H: h}
		p.Fill(track, t.Border, h/2)
		if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
			return
		}
		x := func(seconds float64) float32 {
			return r.X + kw/2 + max(0, r.W-kw)*float32(min(1, max(0, seconds/duration)))
		}
		for _, cached := range ranges {
			start, end := cached.Start+offset, cached.End+offset
			if math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || end <= start {
				continue
			}
			left, right := x(start), x(end)
			if right > left {
				p.Fill(ui.Rect{X: left, Y: track.Y, W: right - left, H: h}, ui.Hex("#858994"), h/2)
			}
		}
		current := x(*position)
		p.Fill(ui.Rect{X: track.X, Y: track.Y, W: current - track.X, H: h}, t.Accent, h/2)
		p.Fill(ui.Rect{X: current - kw/2, Y: cy - kh/2, W: kw, H: kh}, t.Text, kh/2)
	})
}
