package app

import (
	"strconv"
	"strings"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func (a *App) resetDetailTracks() {
	a.detailAudio, a.detailSub = "auto", "auto"
}

func (a *App) detailStreams() []emby.Stream {
	for i, source := range a.detail.MediaSources {
		if source.ID == a.version || (a.version == "" && i == 0) {
			if len(source.MediaStreams) > 0 {
				return source.MediaStreams
			}
			if len(a.detail.MediaSources) > 1 {
				return nil
			}
			break
		}
	}
	return a.detail.MediaStreams
}

func (a *App) detailTrackOptions(kind string) []option {
	opts := []option{{"auto", "自动"}}
	if kind == "sub" {
		opts = append(opts, option{"off", "关闭字幕"})
	}
	for _, stream := range a.detailStreams() {
		if stream.Type == streamType(kind) {
			opts = append(opts, option{strconv.Itoa(stream.Index), streamLabel(stream)})
		}
	}
	return opts
}

func (a *App) detailTrackIndices(item emby.Item) (audio, sub *int) {
	if a.detail.ID != item.ID {
		return nil, nil
	}
	index := func(kind, choice string) *int {
		if kind == "sub" && choice == "off" {
			n := -1
			return &n
		}
		for _, opt := range a.detailTrackOptions(kind) {
			if opt.id == choice {
				if n, err := strconv.Atoi(choice); err == nil {
					return &n
				}
			}
		}
		return nil
	}
	return index("audio", a.detailAudio), index("sub", a.detailSub)
}

func (a *App) detailTrackSelectors(c *ui.Context) {
	video := mediaOptions(a.detail.MediaSources)
	for i, source := range a.detail.MediaSources {
		for _, stream := range source.MediaStreams {
			if stream.Type == "Video" {
				text := strings.TrimSpace(stream.DisplayTitle)
				if text == "" {
					text = strings.ToUpper(stream.Codec)
				}
				if text != "" {
					video[i].label += " · " + text
				}
				break
			}
		}
	}
	if len(video) == 0 {
		video = []option{{"", "自动"}}
	}
	ui.Row(c).FillWidth().MaxWidth(640).Gap(10).AlignItems(ui.Start).Children(func() {
		for _, field := range []struct {
			name    string
			value   *string
			options []option
			changed func()
			grow    float32
		}{
			{"视频", &a.version, video, a.resetDetailTracks, 1.6},
			{"音轨", &a.detailAudio, a.detailTrackOptions("audio"), nil, 1},
			{"字幕", &a.detailSub, a.detailTrackOptions("sub"), nil, 1},
		} {
			ui.Column(c).Width(0).Grow(field.grow).MinWidth(0).Gap(4).Children(func() {
				label(c, field.name)
				selection(c, field.value, field.options, field.changed).FillWidth().MinWidth(0).Height(32).Label("详情" + field.name).Disabled(a.busy)
			})
		}
	})
}
