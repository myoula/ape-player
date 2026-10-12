package app

import (
	"ape-player/internal/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGPUPlayback(t *testing.T) {
	file := os.Getenv("APE_GPU_TEST_FILE")
	if file == "" {
		t.Skip("set APE_GPU_TEST_FILE to run native GPU integration")
	}
	bin := filepath.Join(t.TempDir(), "ape-player")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/ape-player")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build: %s %v", b, e)
	}
	cmd = exec.Command(bin, "--smoke", file)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "XDG_STATE_HOME="+t.TempDir())
	b, e := cmd.CombinedOutput()
	t.Log(string(b))
	if e != nil {
		t.Fatal(e)
	}
}

// Exercise HTTP playback through the real app and mpv, using a local Emby
// fixture. No user's server or saved credentials are touched.
func TestGPUEmbyPlayback(t *testing.T) {
	file := os.Getenv("APE_GPU_TEST_FILE")
	if file == "" {
		t.Skip("set APE_GPU_TEST_FILE")
	}
	var mu sync.Mutex
	var reports []string
	streamRequests := 0
	subtitleRequests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "fixture-token" {
			t.Errorf("missing media/API auth on %s", r.URL.Path)
			w.WriteHeader(401)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/PlaybackInfo"):
			_ = json.NewEncoder(w).Encode(map[string]any{"PlaySessionId": "fixture-session", "MediaSources": []any{map[string]any{"Id": "fixture-media", "SupportsDirectPlay": true, "DefaultSubtitleStreamIndex": 4, "MediaStreams": []any{map[string]any{"Index": 0, "Type": "Video"}, map[string]any{"Index": 1, "Type": "Audio", "DisplayTitle": "English"}, map[string]any{"Index": 2, "Type": "Audio", "DisplayTitle": "Japanese"}, map[string]any{"Index": 3, "Type": "Subtitle", "DisplayTitle": "Chinese"}, map[string]any{"Index": 4, "Type": "Subtitle", "DisplayTitle": "Chinese external", "IsExternal": true, "Codec": "srt", "DeliveryUrl": "/proxy/subtitles/chinese.srt"}}}}})
		case r.URL.Path == "/proxy/subtitles/chinese.srt":
			mu.Lock()
			subtitleRequests++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/x-subrip")
			_, _ = w.Write([]byte("1\n00:00:00,000 --> 00:00:12,000\nServer default external subtitle\n"))
		case strings.HasPrefix(r.URL.Path, "/proxy/Videos/"):
			mu.Lock()
			streamRequests++
			mu.Unlock()
			http.ServeFile(w, r, file)
		case strings.HasPrefix(r.URL.Path, "/proxy/Sessions/Playing"):
			mu.Lock()
			reports = append(reports, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(204)
		case r.URL.Path == "/proxy/Users/u":
			_, _ = w.Write([]byte(`{"Id":"u"}`))
		default:
			_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	c := config.Default()
	c.Active = "fixture"
	c.Sources = []config.Source{{ID: "fixture", Name: "Fixture", URL: srv.URL + "/proxy", UserID: "u", Token: "fixture-token", Remember: true, Timeout: 5}}
	if e := config.Save(filepath.Join(root, "ape-player", "config.json"), c); e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(t.TempDir(), "ape-player")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/ape-player")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%s %v", b, e)
	}
	cmd = exec.Command(bin, "--smoke", "emby:movie")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+root, "XDG_STATE_HOME="+t.TempDir())
	b, e := cmd.CombinedOutput()
	t.Log(string(b))
	if e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	defer mu.Unlock()
	if subtitleRequests == 0 {
		t.Fatal("server default external subtitle was not loaded")
	}
	if streamRequests == 0 {
		t.Fatal("mpv never fetched the authenticated stream")
	}
	if len(reports) < 2 || reports[0] != "/proxy/Sessions/Playing" || reports[len(reports)-1] != "/proxy/Sessions/Playing/Stopped" {
		t.Fatalf("bad playback lifecycle %v", reports)
	}
}
