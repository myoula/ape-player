package app

import (
	"testing"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestDetailTrackSelectorsLayoutAndVersionChange(t *testing.T) {
	a := testApp(t)
	a.page = "detail"
	a.detail = emby.Item{ID: "movie", Type: "Movie", MediaSources: []emby.MediaSource{
		{ID: "a", Name: "1080p", MediaStreams: []emby.Stream{{Index: 1, Type: "Audio", DisplayTitle: "国语"}, {Index: 3, Type: "Subtitle", DisplayTitle: "简体"}}},
		{ID: "b", Name: "4K", MediaStreams: []emby.Stream{{Index: 7, Type: "Audio", DisplayTitle: "英语"}}},
	}}
	a.version, a.detailAudio, a.detailSub = "a", "1", "3"
	tt := ui.NewTester(a.view, 940, 640)
	v, vok := tt.Find("详情视频")
	au, aok := tt.Find("详情音轨")
	s, sok := tt.Find("详情字幕")
	if !vok || !aok || !sok || v.Y != au.Y || au.Y != s.Y || v.X+v.W > au.X || au.X+au.W > s.X || s.X+s.W > 940 {
		t.Fatalf("selectors must fit side by side: %+v %+v %+v", v, au, s)
	}
	tt.SetSize(1920, 1080)
	tt.Frame()
	v, _ = tt.Find("详情视频")
	au, _ = tt.Find("详情音轨")
	s, _ = tt.Find("详情字幕")
	if s.X+s.W-v.X > 640.5 || v.W <= au.W || v.H != 32 || au.H != 32 || s.H != 32 {
		t.Fatalf("selectors should stay compact on wide windows: %+v %+v %+v", v, au, s)
	}
	tt.SetSize(940, 640)
	tt.Frame()
	if err := tt.Click("详情视频"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("2 · 4K"); err != nil {
		t.Fatal(err)
	}
	if a.version != "b" || a.detailAudio != "auto" || a.detailSub != "auto" {
		t.Fatal("version change retained old track indices")
	}
	if err := tt.Click("详情音轨"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("国语") || !tt.HasText("7 · 英语") {
		t.Fatal("track menu does not match selected version")
	}
	if err := tt.Click("7 · 英语"); err != nil {
		t.Fatal(err)
	}
	audio, sub := a.detailTrackIndices(a.detail)
	if audio == nil || *audio != 7 || sub != nil {
		t.Fatal("selected version indices missing")
	}
	a.navigate("queue")
	a.goBack()
	if a.version != "b" || a.detailAudio != "7" {
		t.Fatal("back lost detail selections")
	}
	if audio, sub := a.detailTrackIndices(emby.Item{ID: "other"}); audio != nil || sub != nil {
		t.Fatal("track indices leaked into another item")
	}
	a.resetSource()
	if a.detailAudio != "auto" || a.detailSub != "auto" {
		t.Fatal("source switch retained track choices")
	}
}
