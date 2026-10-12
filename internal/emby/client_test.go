package emby

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPrefixAuthenticationAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxy/emby/Users/AuthenticateByName" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("X-Emby-Authorization") == "" {
			t.Error("missing client identity")
		}
		var b map[string]string
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["Pw"] != "password" {
			t.Error("password body missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"AccessToken":"secret","User":{"Id":"u1"}}`))
	}))
	defer srv.Close()
	c, e := New(srv.URL+"/proxy/emby/", "", "", "device", 5)
	if e != nil {
		t.Fatal(e)
	}
	a, e := c.Login(context.Background(), "user", "password")
	if e != nil || a.User.ID != "u1" || a.AccessToken != "secret" {
		t.Fatalf("%+v %v", a, e)
	}
	for _, bad := range []string{"ftp://host", "http://", "https://user:pw@host", "https://host/?token=secret"} {
		if _, e := New(bad, "", "", "", 1); e == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
func TestPlaybackNegotiation(t *testing.T) {
	for _, tc := range []struct {
		name, source, method string
		force                bool
	}{{"direct", `{"Id":"m1","SupportsDirectPlay":true}`, "DirectPlay", false}, {"stream", `{"Id":"m1","SupportsDirectStream":true,"DirectStreamUrl":"Videos/v/stream?x=1"}`, "DirectStream", false}, {"transcode", `{"Id":"m1","TranscodingUrl":"/proxy/emby/Videos/v/master.m3u8?StartTimeTicks=400000000"}`, "Transcode", true}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/proxy/emby/Items/v/PlaybackInfo" {
					t.Error(r.URL.Path)
				}
				if r.Header.Get("X-Emby-Token") != "token" {
					t.Error("token missing")
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["EnableTranscoding"] != tc.force {
					t.Error("unexpected transcoding request")
				}
				profile := body["DeviceProfile"].(map[string]any)
				if !tc.force && profile["TranscodingProfiles"] != nil {
					t.Error("advertised transcoding for original playback")
				}
				if body["EnableDirectPlay"] == tc.force {
					t.Error("wrong direct play flag")
				}
				if body["StartTimeTicks"] != float64(400000000) {
					t.Error("resume position missing")
				}
				_, _ = w.Write([]byte(`{"PlaySessionId":"session","MediaSources":[` + tc.source + `]}`))
			}))
			defer srv.Close()
			c, _ := New(srv.URL+"/proxy/emby", "token", "user", "device", 5)
			p, e := c.Prepare(context.Background(), Item{ID: "v"}, "m1", 4000000, 400000000, nil, nil, tc.force)
			if e != nil {
				t.Fatal(e)
			}
			if p.Method != tc.method || p.SessionID != "session" {
				t.Fatalf("%+v", p)
			}
			u, _ := url.Parse(p.URL)
			if u.Host == "" {
				t.Error("relative stream URL")
			}
			if tc.method == "DirectStream" && u.Path != "/proxy/emby/Videos/v/stream" {
				t.Error(u.Path)
			}
		})
	}
}
func TestProgressAndFavorite(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/Sessions/Playing/Progress" {
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["PositionTicks"] != float64(125000000) || b["PlaySessionId"] != "session" {
				t.Errorf("bad progress %#v", b)
			}
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "t", "u", "d", 5)
	ctx := context.Background()
	p := Playback{Item: Item{ID: "i"}, Source: MediaSource{ID: "m"}, SessionID: "session"}
	if e := c.Report(ctx, "/Progress", p, 12.5, true); e != nil {
		t.Fatal(e)
	}
	_ = c.Favorite(ctx, "i", true)
	_ = c.Favorite(ctx, "i", false)
	if paths[1] != "POST /Users/u/FavoriteItems/i" || paths[2] != "DELETE /Users/u/FavoriteItems/i" {
		t.Fatal(paths)
	}
}
func TestUnauthorizedAndCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	c, _ := New(srv.URL, "expired", "u", "d", 5)
	if e := c.Validate(context.Background()); e != ErrUnauthorized {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := c.Validate(ctx); e != context.Canceled {
		t.Fatal(e)
	}
}

func TestPasswordLoginFailureAndRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.RawQuery != "" {
			t.Error("credentials must be sent in a POST body")
		}
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["Username"] != "alice" {
			t.Errorf("unexpected username %q", body["Username"])
		}
		if body["Pw"] != " correct password " {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"AccessToken":"session-token","User":{"Id":"alice-id"}}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "", "", "device", 5)
	if _, e := c.Login(context.Background(), "alice", "wrong"); e != ErrInvalidCredentials {
		t.Fatalf("wrong password should have a login-specific error: %v", e)
	}
	auth, e := c.Login(context.Background(), "alice", " correct password ")
	if e != nil || auth.AccessToken != "session-token" || auth.User.ID != "alice-id" {
		t.Fatalf("retry failed: %+v %v", auth, e)
	}
	if calls != 2 {
		t.Fatalf("requests=%d", calls)
	}
}

func TestOriginalStreamSelection(t *testing.T) {
	for _, tc := range []struct {
		name                string
		source              MediaSource
		want                string
		wantAuth, wantError bool
	}{
		{"custom direct URL", MediaSource{SupportsDirectPlay: true, DirectStreamUrl: "/custom/play?signature=opaque%2Fvalue", RequiredHttpHeaders: map[string]string{"Referer": "https://media.example/", "User-Agent": "required-client"}}, "/custom/play?signature=opaque%2Fvalue", true, false},
		{"stream fallback", MediaSource{SupportsDirectStream: true}, "/prefix/Videos/v/stream", true, false},
		{"remote signed URL", MediaSource{SupportsDirectPlay: true, DirectStreamUrl: "https://cdn.example/movie.mp4?signature=opaque%2Fvalue"}, "https://cdn.example/movie.mp4?signature=opaque%2Fvalue", false, false},
		{"no implicit transcode", MediaSource{TranscodingUrl: "/transcode.m3u8"}, "", false, true},
		{"invalid scheme", MediaSource{SupportsDirectPlay: true, DirectStreamUrl: "file:///etc/passwd"}, "", false, true},
		{"invalid headers", MediaSource{SupportsDirectPlay: true, RequiredHttpHeaders: map[string]string{"Referer": "value\r\ninjected: yes"}}, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(PlaybackInfo{MediaSources: []MediaSource{tc.source}})
			}))
			defer srv.Close()
			c, _ := New(srv.URL+"/prefix", "secret-token", "user", "device", 5)
			p, err := c.Prepare(context.Background(), Item{ID: "v"}, "", 0, 0, nil, nil, false)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(p.URL)
			if tc.name == "stream fallback" {
				if u.Path != tc.want || u.Query().Get("Static") != "true" {
					t.Fatal("bad fallback", p.URL)
				}
			} else {
				want := tc.want
				if want[0] == '/' {
					want = srv.URL + want
				}
				if p.URL != want {
					t.Fatalf("URL changed: %s", p.URL)
				}
			}
			if (p.Headers["X-Emby-Token"] != "") != tc.wantAuth {
				t.Fatal("wrong auth scope")
			}
			if tc.name == "custom direct URL" && (p.Headers["Referer"] != "https://media.example/" || p.Headers["User-Agent"] != "required-client") {
				t.Fatal("required headers lost")
			}
		})
	}
}
