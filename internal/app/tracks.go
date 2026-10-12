package app

import (
	"fmt"
	"strconv"
	"strings"

	"ape-player/internal/emby"
	"ape-player/internal/player"
)

func streamType(kind string) string {
	if kind == "sub" {
		return "Subtitle"
	}
	return "Audio"
}

func streamLabel(s emby.Stream) string {
	var parts []string
	for _, text := range []string{s.Title, s.DisplayTitle} {
		text = strings.TrimSpace(text)
		if text != "" && (len(parts) == 0 || !strings.EqualFold(parts[0], text)) {
			parts = append(parts, text)
		}
	}
	if s.Language != "" && !strings.Contains(strings.ToLower(strings.Join(parts, " ")), strings.ToLower(s.Language)) {
		parts = append(parts, s.Language)
	}
	if s.Title == "" && s.DisplayTitle == "" {
		if s.Codec != "" {
			parts = append(parts, strings.ToUpper(s.Codec))
		}
		if s.Channels > 0 {
			parts = append(parts, fmt.Sprintf("%d 声道", s.Channels))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "未命名轨道")
	}
	if s.IsDefault && !strings.Contains(strings.Join(parts, " "), "默认") {
		parts = append(parts, "默认")
	}
	if s.IsForced {
		parts = append(parts, "强制")
	}
	if s.IsExternal {
		parts = append(parts, "外挂")
	}
	return fmt.Sprintf("%d · %s", s.Index, strings.Join(parts, " · "))
}

// Match server indices to mpv's container stream indices, not its separate
// per-type track IDs. External subtitles are matched by their delivery URL.
func (a *App) trackKey(t player.Track) string {
	for _, s := range a.playback.Source.MediaStreams {
		if s.Type != streamType(t.Type) {
			continue
		}
		if !s.IsExternal && !t.External && s.Index == t.FFIndex {
			return "emby:" + strconv.Itoa(s.Index)
		}
		if s.IsExternal && t.External && t.ExternalFilename != "" && a.client != nil && t.ExternalFilename == a.client.SubtitleURL(a.playback, s) {
			return "emby:" + strconv.Itoa(s.Index)
		}
	}
	return "mpv:" + strconv.Itoa(t.ID)
}

func (a *App) trackOptions(kind string) []option {
	var opts []option
	for _, s := range a.playback.Source.MediaStreams {
		if s.Type == streamType(kind) {
			opts = append(opts, option{"emby:" + strconv.Itoa(s.Index), streamLabel(s)})
		}
	}
	// Also expose local tracks and tracks absent from sparse server metadata.
	for _, t := range a.state.Tracks {
		if t.Type != kind {
			continue
		}
		key := a.trackKey(t)
		if !strings.HasPrefix(key, "mpv:") {
			continue
		}
		name := strings.TrimSpace(strings.Join([]string{t.Lang, t.Title, strings.ToUpper(t.Codec)}, " "))
		if name == "" {
			name = "未命名轨道"
		}
		opts = append(opts, option{key, fmt.Sprintf("%d · %s（本地）", t.ID, name)})
	}
	if len(opts) == 0 {
		return nil
	}
	prefix := []option{{"auto", "自动"}}
	if kind == "sub" {
		prefix = append(prefix, option{"off", "关闭字幕"})
	}
	return append(prefix, opts...)
}

func (a *App) chooseTrack(kind, key string) {
	if a.player == nil {
		return
	}
	property := "aid"
	if kind == "sub" {
		property = "sid"
	}
	switch {
	case key == "auto":
		if kind == "sub" {
			a.playback.SubtitleIndex = nil
		} else {
			a.playback.AudioIndex = nil
		}
		a.player.Command("set", property, "auto")
		a.report("/Progress")
	case key == "off":
		a.selectStream(kind, -1)
	case strings.HasPrefix(key, "emby:"):
		if index, err := strconv.Atoi(strings.TrimPrefix(key, "emby:")); err == nil {
			a.selectStream(kind, index)
		}
	case strings.HasPrefix(key, "mpv:"):
		if id, err := strconv.Atoi(strings.TrimPrefix(key, "mpv:")); err == nil {
			a.player.Command("set", property, strconv.Itoa(id))
		}
	}
}

func (a *App) syncTrackChoices() {
	if len(a.state.Tracks) == 0 || a.playback.Method == "Transcode" {
		return
	}
	audio, sub := "", "off"
	var audioIndex, subIndex *int
	disabled := -1
	subIndex = &disabled
	for _, t := range a.state.Tracks {
		if !t.Selected || (t.Type != "audio" && t.Type != "sub") {
			continue
		}
		key := a.trackKey(t)
		var index *int
		if strings.HasPrefix(key, "emby:") {
			n, _ := strconv.Atoi(strings.TrimPrefix(key, "emby:"))
			index = &n
		} else if !t.External {
			n := t.FFIndex
			index = &n
		}
		if t.Type == "audio" {
			audio, audioIndex = key, index
		} else {
			sub, subIndex = key, index
		}
	}
	if audio != "" {
		a.audioChoice = audio
		a.playback.AudioIndex = audioIndex
	}
	a.subChoice = sub
	a.playback.SubtitleIndex = subIndex
}
