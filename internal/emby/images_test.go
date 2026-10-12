package emby

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEpisodePosterUsesSeriesThenSeason(t *testing.T) {
	for _, tc := range []struct {
		name         string
		seriesStatus int
		want         []string
		wantError    error
	}{
		{"series", 200, []string{"/proxy/Items/series/Images/Primary"}, nil},
		{"season fallback", 404, []string{"/proxy/Items/series/Images/Primary", "/proxy/Items/season/Images/Primary"}, nil},
		{"expired login", 401, []string{"/proxy/Items/series/Images/Primary"}, ErrUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Header.Get("X-Emby-Token") != "test-token" {
					t.Error("missing image authentication")
				}
				if r.URL.Path == "/proxy/Items/series/Images/Primary" {
					if r.URL.Query().Get("tag") != "series-tag" {
						t.Error("wrong series image tag")
					}
					w.WriteHeader(tc.seriesStatus)
				} else if r.URL.Query().Get("tag") != "season-tag" {
					t.Error("wrong inherited image tag")
				}
				w.Write([]byte("image"))
			}))
			defer srv.Close()
			c, _ := New(srv.URL+"/proxy", "test-token", "u", "d", 5)
			_, err := c.Image(context.Background(), Item{ID: "episode", Type: "Episode", SeriesID: "series", SeasonID: "season", ImageTags: map[string]string{"Primary": "episode-tag"}, SeriesPrimaryImageTag: "series-tag", ParentPrimaryImageItemID: "season", ParentPrimaryImageTag: "season-tag"})
			if !errors.Is(err, tc.wantError) || !reflect.DeepEqual(paths, tc.want) {
				t.Fatalf("paths=%v err=%v", paths, err)
			}
		})
	}
}

func TestMoviePrimaryImageTagAndMissingPoster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Items/movie/Images/Primary" || r.URL.Query().Get("tag") != "primary-tag" {
			t.Errorf("wrong image request %s", r.URL.String())
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "", "u", "d", 5)
	if _, err := c.Image(context.Background(), Item{ID: "movie", Type: "Movie", PrimaryImageTag: "primary-tag"}); !errors.Is(err, ErrImageNotFound) {
		t.Fatal(err)
	}
}
