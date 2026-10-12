package app

import "github.com/egoist/mygo/ui"

// Apply display geometry to the existing video texture, keeping the decoder,
// selected tracks and playback position intact. The player surface fills the
// window; controls are overlays and don't change its available dimensions.
func (a *App) videoImage(c *ui.Context) {
	fit := ui.Contain
	ratio := float32(0)
	switch a.aspectRatio {
	case "16:9":
		ratio = 16.0 / 9
	case "4:3":
		ratio = 4.0 / 3
	case "拉伸铺满":
		fit = ui.FillBox
	case "裁剪填充":
		fit = ui.Cover
	}
	if ratio == 0 {
		ui.Image(c, a.bitmap).Fill().Fit(fit)
		return
	}
	w, h := c.Size()
	width, height := w, w/ratio
	if height > h {
		width, height = h*ratio, h
	}
	// Force only the chosen display rectangle; don't resize mpv's render target
	// or add letterboxing inside it, which would reintroduce the source ratio.
	ui.Image(c, a.bitmap).Absolute().Left((w-width)/2).Top((h-height)/2).Size(width, height).Fit(ui.FillBox)
}
