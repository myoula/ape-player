package emby

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecentlyPlayedIncludesUnfinishedAndCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emby/Users/alice/Items" || r.Header.Get("X-Emby-Token") != "token" {
			t.Error("history must use the current source and user")
		}
		q := r.URL.Query()
		if q.Get("SortBy") != "DatePlayed" || q.Get("SortOrder") != "Descending" || q.Get("EnableUserData") != "true" || q.Get("Limit") != "18" {
			t.Errorf("history query: %v", q)
		}
		if !strings.Contains(q.Get("Fields"), "UserDataLastPlayedDate") {
			t.Error("Emby omits LastPlayedDate unless explicitly requested")
		}
		if q.Has("IsPlayed") || q.Has("Filters") {
			t.Error("history must not omit unfinished items")
		}
		_, _ = w.Write([]byte(`{"Items":[
   {"Id":"finished","Name":"Finished","UserData":{"Played":true,"LastPlayedDate":"2026-10-11T10:00:00Z"}},
   {"Id":"unseen","UserData":{"LastPlayedDate":null}},
   {"Id":"unfinished","Name":"Continue","UserData":{"Played":false,"PlaybackPositionTicks":500000000,"LastPlayedDate":"2026-10-12T10:00:00Z"}}
  ]}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL+"/emby", "token", "alice", "device", 5)
	items, e := c.RecentlyPlayed(context.Background(), 18)
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 2 || items[0].ID != "unfinished" || items[1].ID != "finished" {
		t.Fatalf("wrong history/order: %+v", items)
	}
	if items[0].UserData.PlaybackPositionTicks != 500000000 {
		t.Fatal("resume position lost")
	}
}
