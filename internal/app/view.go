package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func (a *App) view(c *ui.Context) {
	c.SetTheme(a.theme)
	if a.page != "queue" && (a.page != "player" || !a.queueOpen) {
		a.queueRevealID = ""
	}
	ui.Row(c).Fill().Background(c.Theme().Background).Children(func() {
		if a.page != "player" {
			a.sidebar(c)
		}
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			if a.page != "player" {
				a.header(c)
			}
			if a.message != "" && a.page != "player" {
				ui.Row(c).Padding(10, 24).Background(ui.Hex("#382d23")).Gap(10).Children(func() {
					ui.Text(c, a.message).Grow(1)
					actionButton(c, "x", "关闭提示").OnClick(func() { a.message = "" })
				})
			}
			switch a.page {
			case "sources":
				a.sourcesView(c)
			case "login":
				a.loginView(c)
			case "home":
				a.homeView(c)
			case "library", "favorites":
				a.libraryView(c)
			case "detail":
				a.detailView(c)
			case "queue":
				a.queueView(c)
			case "settings":
				a.settingsView(c)
			case "player":
				a.playerView(c)
			case "closing":
				ui.Text(c, "正在保存播放进度…").Margin(40)
			}
		})
	})
}
func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Width(208).FillHeight().Padding(20, 16).Gap(8).Background(ui.Hex("#161920")).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Margin(12, 8, 26).Children(func() {
			ui.Box(c).Size(32, 32).Radius(9).Background(c.Theme().Accent).Center().Children(func() { glyph(c, "play").TextColor(c.Theme().AccentText) })
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "APE PLAYER").FontSize(17).Bold()
				ui.Text(c, "你的私人影院").FontSize(11).TextColor(c.Theme().TextMuted)
			})
		})
		if a.client != nil {
			for _, n := range []struct{ id, label, icon string }{{"home", "首页", "house"}, {"library", "媒体库", "library-big"}, {"favorites", "我的收藏", "heart"}, {"queue", "播放列表", "list-video"}} {
				navButton(c, n.icon, n.label, a.page == n.id).Disabled(!a.canBrowse()).OnClick(func() { a.library = ""; a.navigate(n.id) })
			}
			if len(a.libraries) > 0 {
				ui.Text(c, "媒体库").FontSize(11).TextColor(c.Theme().TextMuted).Margin(24, 10, 8)
				for _, l := range a.libraries {
					navButton(c, "folder-open", l.Name, a.page == "library" && a.library == l.ID).OnClick(func() { a.openCategory(l.ID, libraryKind(l)) })
				}
			}
		}
		ui.Spacer(c)
		navButton(c, "server", "来源管理", a.page == "sources").OnClick(func() { a.navigate("sources") })
		navButton(c, "settings-2", "偏好设置", a.page == "settings").OnClick(func() { a.navigate("settings") })
		ui.Divider(c).Margin(12, 0)
		if s := a.active(); s != nil && a.client != nil {
			ui.Text(c, s.Name).Bold().Margin(0, 8)
			ui.Text(c, s.Username).FontSize(12).TextColor(c.Theme().TextMuted).Margin(0, 8)
			if a.canBrowse() {
				actionButton(c, "log-out", "退出登录").OnClick(a.logout)
			}
		} else {
			ui.Text(c, "尚未连接服务器").FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
}
func (a *App) header(c *ui.Context) {
	names := map[string]string{"sources": "来源管理", "login": "连接你的媒体世界", "home": "首页", "library": "媒体库", "favorites": "我的收藏", "detail": "影片详情", "settings": "偏好设置", "queue": "播放列表"}
	ui.Row(c).Padding(22, 28).Gap(16).Children(func() {
		if a.client != nil && a.page != "sources" {
			iconButton(c, "arrow-left", "返回来源管理").OnClick(func() { a.navigate("sources") })
		}
		name := names[a.page]
		if a.page == "home" {
			if source := a.active(); source != nil {
				name = source.Name
			}
		}
		ui.Text(c, name).FontSize(25).Bold()
		ui.Spacer(c)
		if a.canBrowse() {
			ui.TextInput(c, &a.query).Placeholder("搜索电影、剧集…").Width(260).OnSubmit(func() { a.library = ""; a.navigate("library") })
			actionButton(c, "search", "搜索").OnClick(func() { a.library = ""; a.navigate("library") })
		}
		if a.busy {
			ui.Text(c, "加载中…").TextColor(c.Theme().Accent)
		}
	})
}
func label(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).TextColor(c.Theme().TextMuted).Margin(8, 0, 0)
}
func bitrateLabel(n int) string {
	if n <= 0 {
		return "原画 / 自动"
	}
	return fmt.Sprintf("%d Mbps", n/1000000)
}
func parseBitrate(s string) int { n, _ := strconv.Atoi(strings.Fields(s)[0]); return n * 1000000 }
func (a *App) loginView(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Padding(32).Children(func() {
		ui.Column(c).Width(400).Padding(30).Radius(16).Background(c.Theme().Surface).Gap(14).Children(func() {
			ui.Text(c, "欢迎回来").FontSize(28).Bold()
			if s := a.active(); s != nil {
				ui.Text(c, s.Name).TextColor(c.Theme().Accent)
			}
			label(c, "Emby 账号（用户名）")
			ui.TextInput(c, &a.username).Label("登录用户名")
			label(c, "密码")
			ui.TextInput(c, &a.password).Password().Label("登录密码").OnSubmit(func() {
				if !a.busy {
					a.login()
				}
			})
			ui.Checkbox(c, &a.remember, "记住登录")
			primaryAction(c, "log-in", "登录 Emby").Disabled(a.busy || strings.TrimSpace(a.username) == "").FillWidth().OnClick(a.login)
		})
	})
}
func (a *App) homeView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(0, 28, 28).Gap(22).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Text(c, "好故事，继续看。").FontSize(28).Bold()
				ui.Text(c, "按分类探索收藏，继续未看完的故事。").TextColor(c.Theme().TextMuted)
			})
			ui.Button(c, "刷新首页").Disabled(a.busy).OnClick(a.loadHome)
		})
		ui.Column(c).Padding(20).Radius(14).Background(ui.Hex("#24312a")).Gap(14).Children(func() {
			ui.Text(c, "分类").FontSize(19).Bold()
			ui.Row(c).Gap(10).Wrap().Children(func() {
				for _, category := range []struct{ label, kind string }{{"全部媒体", "全部类型"}, {"电影", "电影"}, {"剧集", "剧集"}} {
					ui.Button(c, category.label).Label("分类：" + category.label).Height(42).MinWidth(108).OnClick(func() { a.openCategory("", category.kind) })
				}
			})
			if len(a.libraries) > 0 {
				ui.Text(c, "我的媒体库").FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Row(c).Gap(10).Wrap().Children(func() {
					for _, library := range a.libraries {
						ui.Button(c.Key(library.ID), library.Name).Label("媒体库分类：" + library.Name).Height(42).MinWidth(130).OnClick(func() { a.openCategory(library.ID, libraryKind(library)) })
					}
				})
			}
		})
		if a.busy && len(a.sections) == 0 {
			ui.Text(c, "正在加载播放记录…").TextColor(c.Theme().TextMuted)
		}
		for _, s := range a.sections {
			key := "section:" + s.Title
			resume := s.LibraryID == "" && s.Title == "继续观看"
			if s.LibraryID != "" {
				key = "library:" + s.LibraryID
			}
			ui.Column(c.Key(key)).Gap(12).Children(func() {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, s.Title).FontSize(20).Bold().Grow(1).MinWidth(0).SingleLine().Tooltip(s.Title)
					if s.LibraryID != "" {
						actionButton(c, "chevron-right", "查看所有").Label("查看所有：" + s.Title).OnClick(func() { a.openCategory(s.LibraryID, s.Kind) })
					}
				})
				if s.Error != "" {
					ui.Text(c, "加载失败："+s.Error).TextColor(c.Theme().TextMuted)
				} else if len(s.Items) == 0 {
					text := "暂无内容"
					if resume {
						text = "暂无可继续观看的内容，播放电影或剧集后会显示在这里。"
					}
					ui.Text(c, text).TextColor(c.Theme().TextMuted)
				} else {
					ui.ScrollHorizontal(c).Height(292).Children(func() {
						ui.Row(c).Gap(16).Children(func() {
							for _, item := range s.Items {
								if resume {
									a.mediaCard(c, item, true)
								} else {
									a.card(c, item)
								}
							}
						})
					})
				}
			})
		}
	})
}
func (a *App) card(c *ui.Context, item emby.Item) {
	a.mediaCard(c, item, false)
}

func watchProgress(item emby.Item) float64 {
	if item.UserData.Played {
		return 1
	}
	progress := item.UserData.PlayedPercentage / 100
	if item.RunTimeTicks > 0 && item.UserData.PlaybackPositionTicks > 0 {
		progress = float64(item.UserData.PlaybackPositionTicks) / float64(item.RunTimeTicks)
	}
	return min(1, max(0, progress))
}

func (a *App) mediaCard(c *ui.Context, item emby.Item, showProgress bool) {
	ui.Column(c.Key(item.ID)).Width(156).Gap(7).Children(func() {
		poster := ui.Box(c).Radius(8).Clip().Label("查看详情：" + title(item)).OnClick(func() { a.openDetail(item) })
		if showProgress {
			poster.Tooltip(fmt.Sprintf("已观看 %.0f%%", watchProgress(item)*100))
		}
		poster.Children(func() {
			a.poster(c, item, 156, 222)
			if showProgress {
				ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(28).Padding(0, 10).Center().Background(ui.RGBA(0, 0, 0, .55)).PassThrough().Children(func() {
					ui.Progress(c, watchProgress(item)).FillWidth().Height(4).Background(ui.RGBA(255, 255, 255, .3)).Label("观看进度").PassThrough()
				})
			}
		})
		name := item.Name
		meta := fmt.Sprint(item.ProductionYear)
		if item.Type == "Episode" {
			if strings.TrimSpace(item.SeriesName) != "" {
				name = item.SeriesName
			}
			meta = fmt.Sprintf("S%02dE%02d · %s", item.ParentIndexNumber, item.IndexNumber, item.Name)
		}
		ui.Text(c, name).SingleLine().Bold().Tooltip(name)
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Text(c, meta).Grow(1).MinWidth(0).SingleLine().Tooltip(meta).FontSize(11).TextColor(c.Theme().TextMuted)
			if item.UserData.IsFavorite {
				glyph(c, "heart").Size(13, 13).TextColor(c.Theme().Accent).Label("已收藏")
			}
			if item.UserData.Played {
				glyph(c, "check").Size(13, 13).TextColor(c.Theme().TextMuted).Label("已观看")
			}
		})
	})
}
func (a *App) libraryView(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(0, 28, 20).Gap(14).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Select(c, &a.kind, []string{"全部类型", "电影", "剧集"}).Width(150).OnChange(func() { a.offset = 0; a.loadItems() })
			ui.Select(c, &a.sort, []string{"最近添加", "名称", "年份"}).Width(150).OnChange(func() { a.offset = 0; a.loadItems() })
			ui.Select(c, &a.watched, []string{"全部状态", "未观看", "已观看"}).Width(150).OnChange(func() { a.offset = 0; a.loadItems() })
			ui.Spacer(c)
			actionButton(c, "refresh-cw", "刷新").OnClick(a.loadItems)
		})
		if !a.busy && len(a.items) == 0 {
			ui.Text(c, "没有找到内容，试试更换筛选条件。").Margin(30).TextColor(c.Theme().TextMuted)
		}
		ui.GridView(c, &a.grid, len(a.items), 174, 292, func(i int) { a.card(c, a.items[i]) }).Grow(1).Gap(14)
		ui.Row(c).Gap(12).Children(func() {
			ui.Text(c, fmt.Sprintf("共 %d 项 · 第 %d 页", a.total, a.offset/60+1)).TextColor(c.Theme().TextMuted)
			ui.Spacer(c)
			actionButton(c, "chevron-left", "上一页").Disabled(a.offset == 0 || a.busy).OnClick(func() { a.offset = max(0, a.offset-60); a.loadItems() })
			actionButton(c, "chevron-right", "下一页").Disabled(a.offset+60 >= a.total || a.busy).OnClick(func() { a.offset += 60; a.loadItems() })
		})
	})
}
func (a *App) detailView(c *ui.Context) {
	d := a.detail
	ui.Scroll(c).Grow(1).Padding(0, 28, 28).Gap(20).Children(func() {
		actionButton(c, "arrow-left", "返回媒体库").OnClick(func() { a.navigate("library") })
		ui.Row(c).AlignItems(ui.Start).Gap(30).Children(func() {
			a.poster(c, d, 220, 330)
			ui.Column(c).Grow(1).Gap(14).Children(func() {
				ui.Text(c, title(d)).FontSize(30).Bold()
				ui.Row(c).Gap(10).AlignItems(ui.Center).TextColor(c.Theme().TextMuted).Children(func() {
					ui.Text(c, fmt.Sprint(d.ProductionYear))
					glyph(c, "star").Size(15, 15)
					ui.Text(c, fmt.Sprintf("%.1f", d.CommunityRating))
					ui.Text(c, duration(float64(d.RunTimeTicks)/emby.TicksPerSecond))
				})
				if d.Overview != "" {
					ui.Text(c, d.Overview).FontSize(14)
				}
				ui.Row(c).Gap(10).Wrap().Children(func() {
					text := "播放"
					if d.UserData.PlaybackPositionTicks > 0 {
						text = "继续观看"
					}
					primaryAction(c, "play", text).Disabled(a.busy).OnClick(func() { a.play(d) })
					fav := "收藏"
					if d.UserData.IsFavorite {
						fav = "已收藏"
					}
					actionButton(c, "heart", fav).TextColor(favoriteColor(c, d.UserData.IsFavorite)).OnClick(func() { a.toggleFavorite(d) })
					watched := "标记已观看"
					if d.UserData.Played {
						watched = "标记未观看"
					}
					actionButton(c, "check", watched).OnClick(func() { a.togglePlayed(d) })
					if d.Type != "Series" {
						actionButton(c, "list-plus", "加入播放列表").OnClick(func() { a.queue = append(a.queue, d); a.message = "已加入播放列表" })
					}
				})
				if len(d.MediaSources) > 1 {
					label(c, "媒体版本")
					selection(c, &a.version, mediaOptions(d.MediaSources), nil)
				}
				for _, s := range d.MediaSources {
					if s.ID != a.version && len(d.MediaSources) > 1 {
						continue
					}
					ui.Text(c, s.Name).FontSize(12).TextColor(c.Theme().Accent)
					for _, t := range s.MediaStreams {
						ui.Text(c, t.Type+" · "+t.DisplayTitle).FontSize(12).TextColor(c.Theme().TextMuted)
					}
				}
			})
		})
		if len(a.seasons) > 0 {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				for _, s := range a.seasons {
					ui.Button(c, s.Name).Disabled(s.ID == a.season).OnClick(func() { a.loadSeason(s.ID) })
				}
			})
		}
		for _, ep := range a.episodes {
			ui.Row(c.Key(ep.ID)).Padding(14).Radius(9).Background(c.Theme().Surface).Gap(12).Children(func() {
				ui.Text(c, fmt.Sprintf("%02d", ep.IndexNumber)).TextColor(c.Theme().Accent)
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					ui.Text(c, ep.Name).Bold()
					ui.Text(c, duration(float64(ep.RunTimeTicks)/emby.TicksPerSecond)).FontSize(12).TextColor(c.Theme().TextMuted)
				})
				if ep.UserData.Played {
					ui.Text(c, "已观看")
				}
				actionButton(c, "info", "详情").OnClick(func() { a.openDetail(ep) })
				primaryAction(c, "play", "播放").OnClick(func() { a.play(ep) })
			})
		}
	})
}

type option struct{ id, label string }

func selection(c *ui.Context, value *string, options []option, changed func()) {
	labels := make([]string, len(options))
	current := "自动"
	for i, o := range options {
		labels[i] = o.label
		if o.id == *value {
			current = o.label
		}
	}
	ui.Select(c, &current, labels).OnChange(func() {
		for _, o := range options {
			if o.label == current {
				*value = o.id
				if changed != nil {
					changed()
				}
				return
			}
		}
	})
}
func mediaOptions(s []emby.MediaSource) []option {
	var o []option
	for i, v := range s {
		o = append(o, option{v.ID, fmt.Sprintf("%d · %s", i+1, v.Name)})
	}
	return o
}
func (a *App) settingsView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(0, 28, 28).Gap(18).Children(func() {
		ui.Column(c).MaxWidth(650).FillWidth().Padding(24).Background(c.Theme().Surface).Radius(14).Gap(12).Children(func() {
			ui.Text(c, "播放偏好").FontSize(20).Bold()
			ui.Checkbox(c, &a.cfg.Settings.AutoNext, "播放结束后自动播放下一项").OnChange(a.save)
			label(c, "首选音轨语言（ISO 639，例如 jpn,eng）")
			ui.TextInput(c, &a.cfg.Settings.AudioLanguage).OnSubmit(a.save)
			label(c, "首选字幕语言（例如 zho,chi,eng）")
			ui.TextInput(c, &a.cfg.Settings.SubtitleLanguage).OnSubmit(a.save)
			label(c, "默认音量")
			ui.Slider(c, &a.cfg.Settings.Volume, 0, 100).OnChange(a.save)
			primaryAction(c, "save", "保存偏好").OnClick(func() { a.save(); a.message = "偏好已保存" })
		})
		ui.Text(c, "Ape Player 0.1.0\nMyGO Native UI · libmpv\n空格播放/暂停 · ←/→ 跳转 10 秒 · F 全屏 · Esc 退出全屏").FontSize(13).TextColor(c.Theme().TextMuted)
	})
}
func (a *App) playerView(c *ui.Context) {
	loading := a.busy || a.mediaLoading || (a.player != nil && a.state.Buffering)
	root := ui.Box(c).Fill().Background(ui.Hex("#000000"))
	root.Children(func() {
		ui.Box(c.Key("video-surface")).Fill().Label("视频画面").OnClick(func() { a.controlsHidden = !a.controlsHidden }).Children(func() {
			if a.bitmap != nil {
				a.videoImage(c)
			} else if !loading {
				ui.Column(c).Fill().Center().Gap(16).Children(func() {
					ui.Text(c, title(a.playback.Item)).FontSize(24)
				})
			}
		})
		if loading {
			// Rebuild while media opens: the spinner's paint-only animation can
			// otherwise keep the pre-video scene alive while the GPU attaches.
			c.After(75 * time.Millisecond)
			ui.Box(c.Key("playback-loading")).Absolute().Top(0).Bottom(0).Left(0).Right(0).Center().PassThrough().Children(func() {
				ui.Box(c).Size(76, 76).Radius(38).Background(ui.RGBA(15, 18, 23, .72)).Center().PassThrough().Children(func() {
					ui.Spinner(c).Size(40, 40).TextColor(c.Theme().Accent).Label("正在加载媒体").PassThrough()
				})
			})
		}
		if !a.controlsHidden {
			ui.Row(c).Absolute().Top(0).Left(0).Right(0).Padding(18, 22).Gap(14).Background(ui.RGBA(15, 18, 23, .92)).Children(func() {
				iconButton(c, "arrow-left", "返回").OnClick(func() {
					if a.canBrowse() {
						a.navigate("home")
					} else {
						a.stopPlayback()
						a.page = "sources"
					}
				})
				ui.Text(c, title(a.playback.Item)).Grow(1).Bold().SingleLine()
				iconButton(c, "info", "详情").OnClick(func() { a.infoOpen = !a.infoOpen })
				iconButton(c, "list-video", "播放列表").OnClick(func() { a.queueOpen = !a.queueOpen })
			})
			ui.Column(c).Absolute().Bottom(0).Left(0).Right(0).Padding(16, 22).Gap(10).Background(ui.RGBA(15, 18, 23, .96)).Children(func() {
				if a.message != "" {
					ui.Row(c).Gap(10).Children(func() {
						ui.Text(c, a.message).Grow(1).TextColor(ui.Hex("#f3bd80"))
						if a.playback.Method != "Local" {
							actionButton(c, "refresh-cw", "原画重试").OnClick(func() { a.prepare(a.playback.Item, false, a.state.Position, nil, nil) })
						}
						iconButton(c, "x", "关闭提示").OnClick(func() { a.message = "" })
					})
				}
				pos := a.state.Position
				ui.Slider(c, &pos, 0, max(1, a.state.Duration)).Label("播放进度").Disabled(a.player == nil).OnChange(func() { a.seek(pos) })
				ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
					iconButton(c, "skip-back", "上一集").Disabled(a.queueIndex <= 0).OnClick(func() { a.next(-1) })
					iconButton(c, "rotate-ccw", "后退 10 秒").Disabled(a.player == nil).OnClick(func() { a.seek(a.state.Position - 10) })
					playButton(c, a.state.Paused).Disabled(a.player == nil).OnClick(func() { a.player.Command("cycle", "pause") })
					iconButton(c, "rotate-cw", "前进 10 秒").Disabled(a.player == nil).OnClick(func() { a.seek(a.state.Position + 10) })
					iconButton(c, "skip-forward", "下一集").Disabled(a.queueIndex+1 >= len(a.queue)).OnClick(func() { a.next(1) })
					ui.Text(c, duration(a.state.Position)+" / "+duration(a.state.Duration)).FontSize(12)
					ui.Spacer(c)
					iconButton(c, muteIcon(a.state.Muted), muteLabel(a.state.Muted)).Disabled(a.player == nil).OnClick(func() { a.player.Command("cycle", "mute") })
					volume := a.cfg.Settings.Volume
					ui.Slider(c, &volume, 0, 100).Width(90).Label("音量").OnChange(func() {
						a.cfg.Settings.Volume = volume
						if a.player != nil {
							a.player.Command("set", "volume", fmt.Sprint(volume))
						}
					})
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						var speeds []option
						for _, speed := range []string{"0.5", "0.75", "1.0", "1.25", "1.5", "2.0"} {
							speeds = append(speeds, option{speed, speed + "×"})
						}
						playerSelect(c, "gauge", "倍速", &a.speed, speeds, func() {
							if a.player != nil {
								a.player.Command("set", "speed", a.speed)
							}
						}, nil)
						var ratios []option
						for _, ratio := range []string{"原始比例", "16:9", "4:3", "拉伸铺满", "裁剪填充"} {
							ratios = append(ratios, option{ratio, ratio})
						}
						playerSelect(c, "ratio", "画面比例选择", &a.aspectRatio, ratios, nil, nil)
						a.trackMenu(c, "audio")
						a.trackMenu(c, "sub")
						iconButton(c, fullscreenIcon(a.win != nil && a.win.IsFullScreen()), "全屏").OnClick(func() {
							if a.win != nil {
								a.win.ToggleFullScreen()
							}
						})
					})
				})
			})
		}
		if a.queueOpen {
			ui.Scroll(c).Absolute().Top(72).Bottom(180).Right(16).Width(350).Padding(14).Gap(8).Background(c.Theme().Surface).Radius(10).Children(func() {
				ui.Text(c, "播放列表").FontSize(18).Bold()
				if len(a.queue) == 0 {
					ui.Text(c, "播放列表为空")
				}
				a.queueItems(c, false)
			})
		}
		if a.infoOpen {
			ui.Scroll(c).Absolute().Top(72).Left(20).Width(430).MaxHeight(330).Padding(20).Gap(10).Radius(10).Background(c.Theme().Surface).Children(func() {
				ui.Text(c, title(a.playback.Item)).FontSize(20).Bold()
				ui.Text(c, a.playback.Item.Overview)
				ui.Text(c, a.playback.Source.Name).FontSize(12)
				ui.Text(c, a.playback.Method+" · "+a.state.VideoCodec+" / "+a.state.AudioCodec+" · "+a.state.HWDec).FontSize(11).TextColor(c.Theme().TextMuted)
				for _, s := range a.playback.Source.MediaStreams {
					ui.Text(c, s.Type+" · "+s.DisplayTitle).FontSize(12)
				}
			})
		}
	})
	if c.Shortcut(0, ui.KeySpace) && a.player != nil {
		a.player.Command("cycle", "pause")
		a.controlsHidden = false
	}
	if c.Shortcut(0, ui.KeyLeft) {
		a.seek(a.state.Position - 10)
	}
	if c.Shortcut(0, ui.KeyRight) {
		a.seek(a.state.Position + 10)
	}
	if c.Shortcut(0, ui.KeyF) {
		a.win.ToggleFullScreen()
	}
	if c.Shortcut(0, ui.KeyEscape) {
		a.win.SetFullScreen(false)
		a.queueOpen = false
		a.infoOpen = false
		a.controlsHidden = false
	}
}
func (a *App) trackMenu(c *ui.Context, kind string) {
	selected, label := &a.audioChoice, "音轨"
	empty := "无可选音轨"
	if kind == "sub" {
		selected, label, empty = &a.subChoice, "字幕", "无独立字幕"
	}
	icon := "audio-lines"
	if kind == "sub" {
		icon = "captions"
	}
	opts := a.trackOptions(kind)
	if len(opts) == 0 {
		if a.player != nil && len(a.state.Tracks) == 0 {
			empty = "读取中…"
		}
		label = empty
	}
	var extra func()
	if kind == "sub" {
		extra = func() {
			ui.Column(c).Padding(10, 6, 4).Gap(10).Children(func() {
				if len(opts) == 0 {
					ui.Text(c, empty).FontSize(12).TextColor(c.Theme().TextMuted)
				}
				actionButton(c, "folder-open", "加载字幕").Disabled(a.player == nil).OnClick(a.addLocalSubtitle)
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "字幕延迟（秒）").FontSize(12)
					ui.NumberInput(c, &a.subDelay, -60, 60, .1).Width(150).Label("字幕延迟").Tooltip("字幕延迟（秒）").OnChange(func() {
						if a.player != nil {
							a.player.Command("set", "sub-delay", fmt.Sprint(a.subDelay))
						}
					})
				})
			})
		}
	}
	playerSelect(c, icon, label, selected, opts, func() { a.chooseTrack(kind, *selected) }, extra).Disabled(len(opts) == 0 && kind != "sub")
}

func (a *App) queueView(c *ui.Context) {
	ui.Scroll(c).Grow(1).Padding(0, 28, 28).Gap(12).Children(func() {
		if len(a.queue) == 0 {
			ui.Text(c, "从影片详情页加入播放列表，或播放一集以建立剧集队列。").TextColor(c.Theme().TextMuted)
		}
		a.queueItems(c, true)
	})
}

func (a *App) queueItems(c *ui.Context, spacious bool) {
	current := a.playback.Item.ID
	revealID := a.page + ":" + current
	for i, v := range a.queue {
		selected := current != "" && current == v.ID
		button := queueButton(c.Key(fmt.Sprint(i)), title(v), selected).OnClick(func() { a.playQueueItem(i) })
		if spacious {
			button.Padding(14)
		}
		if selected && a.queueRevealID != revealID {
			// Reveal on opening or changing episodes, without fighting manual scrolling.
			button.ScrollIntoView()
			a.queueRevealID = revealID
		}
	}
}
