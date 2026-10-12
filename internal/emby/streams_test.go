package emby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPlaybackPreservesServerTrackMetadataAndDefaults(t *testing.T) {
	audio, sub := 4, 7
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PlaybackInfo{MediaSources: []MediaSource{{ID: "edition-b", SupportsDirectPlay: true, DefaultAudioStreamIndex: &audio, DefaultSubtitleStreamIndex: &sub, MediaStreams: []Stream{{Index: 4, Type: "Audio", Codec: "aac"}}}}})
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token", "user", "device", 5)
	item := Item{ID: "movie", MediaSources: []MediaSource{
		{ID: "edition-a", MediaStreams: []Stream{{Index: 4, Type: "Audio", Title: "Wrong edition"}}},
		{ID: "edition-b", MediaStreams: []Stream{{Index: 4, Type: "Audio", Title: "日本語", Language: "jpn"}, {Index: 7, Type: "Subtitle", Title: "简体中文", Codec: "ass", IsExternal: true, DeliveryUrl: "/subtitle.ass"}}},
	}}
	p, err := c.Prepare(context.Background(), item, "edition-b", 0, 0, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Source.MediaStreams) != 2 || p.Source.MediaStreams[0].Title != "日本語" || p.Source.MediaStreams[0].Codec != "aac" || !p.Source.MediaStreams[1].IsExternal {
		t.Fatalf("metadata lost/mixed: %+v", p.Source.MediaStreams)
	}
	if p.AudioIndex == nil || *p.AudioIndex != 4 || p.SubtitleIndex == nil || *p.SubtitleIndex != 7 {
		t.Fatal("server default tracks lost")
	}
	off := -1
	p, err = c.Prepare(context.Background(), item, "edition-b", 0, 0, &audio, &off, false)
	if err != nil || p.SubtitleIndex == nil || *p.SubtitleIndex != -1 {
		t.Fatal("explicit subtitle disable overridden")
	}
}

func TestSubtitleAuthenticationScope(t *testing.T) {
	c, _ := New("https://emby.example/proxy", "private-token", "user", "device", 5)
	p := Playback{Item: Item{ID: "movie"}, Source: MediaSource{ID: "media"}}
	for _, tc := range []struct {
		stream Stream
		path   string
		auth   bool
	}{
		{Stream{Index: 3, Codec: "subrip"}, "/proxy/Videos/movie/media/Subtitles/3/Stream.srt", true},
		{Stream{DeliveryUrl: "/subs/chinese.ass?version=1"}, "/subs/chinese.ass", true},
		{Stream{DeliveryUrl: "https://cdn.example/sub.srt?signature=original"}, "/sub.srt", false},
	} {
		raw := c.SubtitleURL(p, tc.stream)
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if u.Path != tc.path || (u.Query().Get("api_key") == "private-token") != tc.auth {
			t.Fatal("subtitle authentication/path mismatch")
		}
		if !tc.auth && raw != tc.stream.DeliveryUrl {
			t.Fatal("signed external subtitle URL changed")
		}
	}
}
