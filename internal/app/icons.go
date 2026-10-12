package app

import (
	"ape-player/internal/icons"
	"github.com/egoist/mygo/ui"
)

func glyph(c *ui.Context, name string) ui.Element {
	return ui.Icon(c, icons.Get(name)).Size(18, 18).Shrink(0)
}

func actionButton(c *ui.Context, name, label string) ui.Element {
	return buttonWithIcon(c, name, label, false)
}
func primaryAction(c *ui.Context, name, label string) ui.Element {
	return buttonWithIcon(c, name, label, true)
}
func buttonWithIcon(c *ui.Context, name, label string, primary bool) ui.Element {
	var button ui.Element
	if primary {
		button = ui.PrimaryButton(c, "")
	} else {
		button = ui.Button(c, "")
	}
	return button.Label(label).Height(36).Padding(8, 12).Gap(8).Radius(8).Children(func() {
		glyph(c, name)
		ui.Text(c, label).FontSize(13).SingleLine()
	})
}

func iconButton(c *ui.Context, name, label string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Size(36, 36).Padding(0).Radius(8).Center().Children(func() { glyph(c, name) })
}
func playButton(c *ui.Context, paused bool) ui.Element {
	name, label := "pause", "暂停"
	if paused {
		name, label = "play", "播放"
	}
	return ui.PrimaryButton(c, "").Label(label).Tooltip(label+" · 空格").Size(44, 44).Padding(0).Radius(22).Center().Children(func() { glyph(c, name).Size(22, 22) })
}
func navButton(c *ui.Context, name, label string, selected bool) ui.Element {
	button := actionButton(c, name, label).FillWidth().Height(44).Justify(ui.Start)
	if selected {
		button.Background(ui.Hex("#293524")).Border(1, ui.Hex("#45533b")).TextColor(c.Theme().Accent)
	}
	return button
}

// playerSelect keeps the toolbar compact while retaining full option names in
// the popup and the current selection in the tooltip.
func playerSelect(c *ui.Context, name, label string, value *string, options []option, changed, extra func()) ui.Element {
	tip := label
	for _, opt := range options {
		if opt.id == *value {
			tip += " · " + opt.label
			break
		}
	}
	sel := ui.SelectBase(c, value)
	button := sel.Trigger.Label(label).Tooltip(tip).Size(36, 36).Padding(0).Radius(8).Center().Background(c.Theme().Surface)
	if button.Hovered() || sel.Open() {
		button.Background(ui.Hex("#293524")).TextColor(c.Theme().Accent)
	}
	button.Children(func() { glyph(c, name) })
	if changed != nil {
		button.OnChange(changed)
	}
	sel.Popup(func(panel ui.Element) {
		panel.MinWidth(180).MaxWidth(440).MaxHeight(360).Padding(6).Gap(2).Radius(10).Background(c.Theme().Surface).Border(1, c.Theme().Border)
		panel.Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, .25))
		if extra != nil {
			panel.MinWidth(320)
		}
		for _, opt := range options {
			item := sel.Item(opt.id).Label(opt.label).Padding(8, 10).Gap(10).Radius(6).AlignItems(ui.Center)
			if item.Highlighted() {
				item.Background(ui.Hex("#293524"))
			}
			item.Children(func() {
				check := glyph(c, "check").Size(15, 15).TextColor(c.Theme().Accent)
				if opt.id != *value {
					check.Opacity(0)
				}
				ui.Text(c, opt.label).FontSize(13)
			})
		}
		if extra != nil {
			extra()
		}
	})
	return button
}
func queueButton(c *ui.Context, label string, selected bool) ui.Element {
	name := "play"
	if selected {
		name = "audio-lines"
	}
	button := actionButton(c, name, label).FillWidth().Justify(ui.Start)
	if selected {
		button.TextColor(c.Theme().Accent).Background(ui.Hex("#304127")).Border(1, c.Theme().Accent).Label("正在播放 · " + label).Tooltip("正在播放 · " + label)
	}
	return button
}

func favoriteColor(c *ui.Context, favorite bool) ui.Color {
	if favorite {
		return c.Theme().Accent
	}
	return c.Theme().Text
}
func muteIcon(muted bool) string {
	if muted {
		return "volume-x"
	}
	return "volume-2"
}
func muteLabel(muted bool) string {
	if muted {
		return "取消静音"
	}
	return "静音"
}
func fullscreenIcon(full bool) string {
	if full {
		return "minimize"
	}
	return "maximize"
}
