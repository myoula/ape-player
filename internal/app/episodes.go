package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func detailSeries(item emby.Item) emby.Item {
	if item.Type == "Series" {
		return item
	}
	if item.Type == "Episode" {
		return emby.Item{ID: item.SeriesID, Name: item.SeriesName, Type: "Series"}
	}
	return emby.Item{}
}

func fetchDetailEpisodes(ctx context.Context, client *emby.Client, detail emby.Item) (emby.Items, emby.Items, string, error) {
	series := detailSeries(detail)
	if series.ID == "" {
		return emby.Items{}, emby.Items{}, "", nil
	}
	seasons, err := client.List(ctx, "Shows/"+series.ID+"/Seasons", map[string][]string{"UserId": {client.UserID}})
	if err != nil {
		return seasons, emby.Items{}, "", err
	}
	seasonID := ""
	if detail.Type == "Episode" {
		seasonID = detail.SeasonID
		if seasonID == "" {
			for _, season := range seasons.Items {
				if season.IndexNumber == detail.ParentIndexNumber {
					seasonID = season.ID
					break
				}
			}
		}
	} else if len(seasons.Items) > 0 {
		seasonID = seasons.Items[0].ID
	}
	episodes, err := fetchEpisodes(ctx, client, series, seasonID)
	return seasons, episodes, seasonID, err
}

func (a *App) selectedEpisodeID() string {
	if a.detail.Type == "Episode" {
		return a.detail.ID
	}
	return a.playback.Item.ID
}

func fetchEpisodes(ctx context.Context, client *emby.Client, series emby.Item, season string) (emby.Items, error) {
	query := map[string][]string{
		"UserId": {client.UserID}, "Fields": {"MediaSources,Overview"},
		"EnableUserData": {"true"}, "SortBy": {"ParentIndexNumber,IndexNumber"}, "SortOrder": {"Ascending"},
	}
	if season != "" {
		query["SeasonId"] = []string{season}
	}
	result, err := client.List(ctx, "Shows/"+series.ID+"/Episodes", query)
	for i := range result.Items {
		ep := &result.Items[i]
		if ep.SeriesID == "" {
			ep.SeriesID = series.ID
		}
		if ep.SeriesName == "" {
			ep.SeriesName = series.Name
		}
		if ep.Type == "" {
			ep.Type = "Episode"
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.ParentIndexNumber != b.ParentIndexNumber {
			return a.ParentIndexNumber < b.ParentIndexNumber
		}
		return a.IndexNumber < b.IndexNumber
	})
	return result, err
}

func (a *App) episodeSeasonSelect(c *ui.Context, label string) {
	if len(a.seasons) == 0 {
		ui.Text(c, "剧集").FontSize(18).Bold()
		return
	}
	var options []option
	for _, season := range a.seasons {
		options = append(options, option{season.ID, season.Name})
	}
	ui.Column(c).Width(240).Children(func() {
		selection(c, &a.season, options, func() { a.loadSeason(a.season) }).Label(label)
	})
}

func (a *App) playEpisode(ep emby.Item) {
	a.episodePickerOpen = false
	a.play(ep)
}

func (a *App) episodeShelf(c *ui.Context) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		a.episodeSeasonSelect(c, "剧集季度")
		ui.Spacer(c)
		actionButton(c, "list-video", "全部剧集").Disabled(a.busy || len(a.episodes) == 0).OnClick(func() {
			a.episodeGrid = ui.GridState{}
			a.episodePickerOpen = true
		})
	})
	if a.busy {
		ui.Text(c, "正在加载剧集…").TextColor(c.Theme().TextMuted)
		return
	}
	if len(a.episodes) == 0 {
		ui.Text(c, "暂无可播放剧集").TextColor(c.Theme().TextMuted)
		return
	}
	// Reserve the whole row, but build only the visible cards and two nearby
	// cards on each side. Offscreen episodes do not start poster requests.
	const cardWidth, stride = float32(208), float32(224)
	width, _ := c.Size() // Window width safely covers the shelf during resizing.
	total := float32(len(a.episodes))*stride - (stride - cardWidth)
	left := min(max(0, a.episodeScroll.X), total)
	first := max(0, int(left/stride)-2)
	last := min(len(a.episodes), int(math.Ceil(float64((left+width)/stride)))+2)
	ui.ScrollHorizontal(c.Key("episodes:" + a.season)).DragScroll().HideScrollbars().Height(240).AlignItems(ui.Start).TrackScroll(&a.episodeScroll).Label("剧集横排").Children(func() {
		ui.Box(c).Width(total).Height(212).Shrink(0).Children(func() {
			for i := first; i < last; i++ {
				ep := a.episodes[i]
				ui.Box(c.Key(ep.ID)).Absolute().Left(float32(i) * stride).Top(0).Width(cardWidth).Height(212).Children(func() {
					a.episodeCard(c, ep)
				})
			}
		})
	})
}

func (a *App) episodeCard(c *ui.Context, ep emby.Item) {
	ui.Column(c.Key(ep.ID)).Width(208).Gap(6).Children(func() {
		button := ui.ButtonBase(c).FillWidth().Padding(0).Justify(ui.Start).Radius(8).
			Label("播放剧集：" + title(ep)).Tooltip(title(ep)).OnClick(func() { a.playEpisode(ep) })
		if ep.ID == a.selectedEpisodeID() && ep.ID != "" {
			button.Border(2, c.Theme().Accent)
		}
		button.Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				a.thumbnail(c, ep, 208, 117)
				ui.Text(c, fmt.Sprintf("%d. %s", ep.IndexNumber, ep.Name)).SingleLine().Bold().FontSize(13)
			})
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			var meta []string
			if !ep.PremiereDate.IsZero() {
				meta = append(meta, ep.PremiereDate.Format("2006-01-02"))
			}
			if ep.RunTimeTicks > 0 {
				meta = append(meta, duration(float64(ep.RunTimeTicks)/emby.TicksPerSecond))
			}
			ui.Text(c, strings.Join(meta, " · ")).Grow(1).SingleLine().FontSize(11).TextColor(c.Theme().TextMuted)
			if ep.UserData.Played {
				glyph(c, "check").Size(13, 13).Label("已观看").TextColor(c.Theme().Accent)
			}
			iconButton(c, "info", "剧集详情："+title(ep)).Size(22, 22).OnClick(func() { a.openDetail(ep) })
		})
		ui.Text(c, ep.Overview).FontSize(11).MaxLines(2).TextColor(c.Theme().TextMuted)
	})
}

func (a *App) episodePicker(c *ui.Context) {
	width, height := c.Size()
	ui.Modal(c, &a.episodePickerOpen, func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, detailSeries(a.detail).Name).FontSize(20).Bold().Grow(1).MinWidth(0).SingleLine()
			iconButton(c, "x", "关闭选集弹窗").OnClick(func() { a.episodePickerOpen = false })
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			a.episodeSeasonSelect(c, "选集季度")
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("共 %d 集", len(a.episodes))).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		if a.busy {
			ui.Text(c, "正在加载剧集…").Margin(20)
		} else if len(a.episodes) == 0 {
			if a.message != "" {
				ui.Text(c, a.message).TextColor(ui.Hex("#f3bd80"))
				actionButton(c, "refresh-cw", "重新加载剧集").OnClick(func() { a.loadSeason(a.season) })
			} else {
				ui.Text(c, "暂无可播放剧集").Margin(20)
			}
		} else {
			ui.GridView(c.Key(a.season), &a.episodeGrid, len(a.episodes), 82, 42, func(i int) {
				ep := a.episodes[i]
				name := fmt.Sprintf("第%d集", ep.IndexNumber)
				if a.season == "" {
					name = fmt.Sprintf("S%02dE%02d", ep.ParentIndexNumber, ep.IndexNumber)
				}
				button := ui.Button(c, name).Fill().Padding(4).Radius(10).
					Label("选集：" + title(ep)).Tooltip(title(ep)).OnClick(func() { a.playEpisode(ep) })
				if ep.ID == a.selectedEpisodeID() && ep.ID != "" {
					button.Border(1, c.Theme().Accent).Background(ui.Hex("#293524")).TextColor(c.Theme().Accent)
				} else if ep.UserData.Played {
					button.TextColor(c.Theme().TextMuted)
				}
			}).Height(min(float32(400), height-260)).Gap(10)
		}
	}).Width(min(float32(780), width-64)).Label("全部剧集弹窗")
}
