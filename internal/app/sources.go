package app

import (
	"strings"

	"ape-player/internal/config"
	"github.com/egoist/mygo/ui"
)

func (a *App) openSourceEditor(source config.Source) {
	a.draft = source
	a.editing = source.ID
	a.sourcePassword = ""
	a.sourceError = ""
	a.sourceEditorOpen = true
}

func (a *App) closeSourceEditor() {
	a.sourceEditorOpen = false
	a.draft = config.Source{}
	a.editing = ""
	a.sourcePassword = ""
	a.sourceError = ""
}

func (a *App) sourcesView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(0, 28, 28).Gap(18).Children(func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "选择一个服务，开始观看。").Grow(1).TextColor(c.Theme().TextMuted)
			primaryAction(c, "server", "添加 Emby 来源").OnClick(func() { a.openSourceEditor(config.Source{}) })
		})
		if len(a.cfg.Sources) == 0 {
			ui.Column(c).Padding(36).Gap(12).Center().Background(c.Theme().Surface).Radius(14).Children(func() {
				glyph(c, "server").Size(36, 36).TextColor(c.Theme().Accent)
				ui.Text(c, "还没有添加服务").FontSize(20).Bold()
				ui.Text(c, "添加 Emby 地址和账号后，服务会显示在这里。").TextColor(c.Theme().TextMuted)
			})
		}
		for _, source := range a.cfg.Sources {
			ui.Column(c.Key(source.ID)).Padding(20).Gap(14).Radius(14).Background(c.Theme().Surface).Children(func() {
				ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
					ui.ButtonBase(c).Grow(1).MinWidth(0).Justify(ui.Start).Label("服务：" + source.Name).OnClick(func() { a.activate(source.ID) }).Children(func() {
						ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
							ui.Box(c).Size(48, 48).Radius(12).Background(ui.Hex("#293524")).Center().Children(func() { glyph(c, "server").Size(24, 24).TextColor(c.Theme().Accent) })
							ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
								ui.Text(c, source.Name).FontSize(20).Bold().SingleLine()
								ui.Text(c, source.URL).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine()
								status := "点击进入后登录"
								if source.Token != "" {
									status = "已保存登录"
								}
								if source.Username != "" {
									status = source.Username + " · " + status
								}
								ui.Text(c, status).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
							})
						})
					})
					primaryAction(c, "chevron-right", "进入服务").Label("进入服务：" + source.Name).OnClick(func() { a.activate(source.ID) })
					iconButton(c, "pencil", "编辑来源").Label("编辑来源：" + source.Name).OnClick(func() { a.openSourceEditor(source) })
					iconButton(c, "trash", "删除来源").Label("删除来源：" + source.Name).OnClick(func() { a.deleting = source.ID })
				})
				if a.deleting == source.ID {
					ui.Row(c).Gap(12).Children(func() {
						ui.Text(c, "删除此来源和保存的登录信息？").Grow(1)
						ui.Button(c, "取消").OnClick(func() { a.deleting = "" })
						ui.Button(c, "确认删除").OnClick(func() { a.deleteSource(source.ID) })
					})
				}
			})
		}
	})
	a.sourceEditor(c)
}

func (a *App) sourceEditor(c *ui.Context) {
	heading := "添加 Emby 来源"
	if a.editing != "" {
		heading = "编辑来源"
	}
	_, height := c.Size()
	modal := ui.Modal(c, &a.sourceEditorOpen, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, heading).Grow(1).FontSize(22).Bold()
			iconButton(c, "x", "关闭来源弹窗").OnClick(a.closeSourceEditor)
		})
		ui.Scroll(c).Height(min(float32(480), height-220)).Gap(8).Children(func() {
			if a.sourceError != "" {
				ui.Text(c, a.sourceError).FontSize(13).TextColor(ui.Hex("#f3bd80"))
			}
			label(c, "名称")
			ui.TextInput(c, &a.draft.Name).Label("来源名称").Placeholder("我的 Emby")
			label(c, "服务器地址")
			ui.TextInput(c, &a.draft.URL).Label("来源地址").Placeholder("https://emby.example.com 或 http://192.168.1.2:8096")
			label(c, "Emby 账号（用户名）")
			ui.TextInput(c, &a.draft.Username).Label("来源账号").Placeholder("输入此服务器的用户名")
			label(c, "密码")
			ui.TextInput(c, &a.sourcePassword).Password().Label("来源密码").Placeholder("输入账号密码")
			if a.editing != "" && a.draft.Token != "" {
				ui.Text(c, "保留当前地址与账号且不填写密码，可继续使用已保存的登录。").FontSize(12).TextColor(c.Theme().TextMuted)
			}
			ui.Row(c).Gap(24).Children(func() {
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					label(c, "连接超时（秒）")
					timeout := float64(a.draft.Timeout)
					if timeout == 0 {
						timeout = 20
					}
					ui.NumberInput(c, &timeout, 5, 120, 1).Label("连接超时").OnChange(func() { a.draft.Timeout = int(timeout) })
				})
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					label(c, "最高播放码率")
					choice := bitrateLabel(a.draft.Bitrate)
					ui.Select(c, &choice, []string{"原画 / 自动", "20 Mbps", "10 Mbps", "4 Mbps"}).Label("最高播放码率").OnChange(func() { a.draft.Bitrate = parseBitrate(choice) })
				})
			})
		})
		ui.Row(c).Gap(10).Children(func() {
			ui.Spacer(c)
			ui.Button(c, "取消").OnClick(a.closeSourceEditor)
			text := "保存并连接"
			if strings.TrimSpace(a.draft.Username) != "" {
				text = "保存并登录"
			}
			primaryAction(c, "save", text).OnClick(a.saveSource)
		})
	}).Width(600).Label(heading)
	if modal.Dismissed() {
		a.closeSourceEditor()
	}
}
