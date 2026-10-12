package app

import (
	"strings"
	"testing"

	"ape-player/internal/emby"
	"ape-player/internal/player"
	"github.com/egoist/mygo/ui"
)

func TestServerTrackOptionsAndSelection(t *testing.T) {
	a := testApp(t)
	a.client, _ = emby.New("https://emby.example/proxy", "token", "user", "device", 5)
	a.playback.Source = emby.MediaSource{ID: "source", MediaStreams: []emby.Stream{
		{Index: 1, Type: "Audio", Title: "英语 English", DisplayTitle: "AC3 5.1 (默认)", IsDefault: true},
		{Index: 4, Type: "Audio", Language: "jpn", Codec: "aac", Channels: 2},
		{Index: 7, Type: "Subtitle", Language: "zho", Codec: "ass", IsExternal: true, DeliveryUrl: "/subs/chinese.ass"},
		{Index: 8, Type: "Subtitle", DisplayTitle: "English SDH", Codec: "subrip"},
	}}
	opts := a.trackOptions("audio")
	if len(opts) != 3 || opts[1].id != "emby:1" || !strings.Contains(opts[1].label, "英语 English") || !strings.Contains(opts[1].label, "AC3 5.1") || !strings.Contains(opts[2].label, "jpn") {
		t.Fatalf("server labels missing: %+v", opts)
	}
	external := a.client.SubtitleURL(a.playback, a.playback.Source.MediaStreams[2])
	a.state.Tracks = []player.Track{
		{ID: 2, Type: "audio", FFIndex: 4, Selected: true},
		{ID: 1, Type: "sub", FFIndex: 8},
		{ID: 2, Type: "sub", External: true, ExternalFilename: external, Selected: true},
		{ID: 3, Type: "sub", External: true, ExternalFilename: "/tmp/local.srt", Title: "Local subtitles"},
	}
	opts = a.trackOptions("sub")
	if len(opts) != 5 || opts[2].id != "emby:7" || opts[3].id != "emby:8" || opts[4].id != "mpv:3" {
		t.Fatalf("subtitle tracks missing or duplicated: %+v", opts)
	}
	a.syncTrackChoices()
	if a.audioChoice != "emby:4" || a.subChoice != "emby:7" || *a.playback.SubtitleIndex != 7 {
		t.Fatalf("wrong selection: audio=%s sub=%s", a.audioChoice, a.subChoice)
	}
	a.state.Tracks[2].Selected = false
	a.syncTrackChoices()
	if a.subChoice != "off" || *a.playback.SubtitleIndex != -1 {
		t.Fatal("subtitle disable not reflected")
	}
	a.state.Tracks[3].Selected = true
	a.syncTrackChoices()
	if a.subChoice != "mpv:3" || a.playback.SubtitleIndex != nil {
		t.Fatal("local subtitle incorrectly reported as a server stream")
	}
}

func TestLocalSubtitlesAndMissingServerSubtitles(t *testing.T) {
	a := testApp(t)
	a.playback.Method = "Local"
	a.state.Tracks = []player.Track{{ID: 1, Type: "audio", FFIndex: 0}, {ID: 1, Type: "sub", FFIndex: 1, Title: "Chinese", Selected: true}}
	opts := a.trackOptions("sub")
	if len(opts) != 3 || opts[2].id != "mpv:1" {
		t.Fatalf("local subtitles missing: %+v", opts)
	}
	a.syncTrackChoices()
	if a.subChoice != "mpv:1" {
		t.Fatal("local track not selected")
	}
	a.page = "player"
	a.playback.Method = "DirectPlay"
	a.playback.Source.MediaStreams = []emby.Stream{{Index: 1, Type: "Audio", DisplayTitle: "AC3 5.1"}}
	a.state.Tracks = []player.Track{{ID: 1, Type: "audio", FFIndex: 1, Selected: true}}
	tt := ui.NewTester(a.view, 1280, 820)
	if !tt.HasText("无独立字幕") {
		t.Fatal("missing subtitle state should be explicit")
	}
}
