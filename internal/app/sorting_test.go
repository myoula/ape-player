package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestLibrarySortSelectionRequestsServerOrder(t *testing.T) {
	for _, tc := range []struct{ label, field, order string }{
		{"最后一集添加时间", "DateLastContentAdded", "Descending"},
		{"最近添加", "DateCreated", "Descending"},
		{"名称", "SortName", "Ascending"},
		{"年份", "ProductionYear", "Descending"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			a := testApp(t)
			requests := make(chan url.Values, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Query()
				<-r.Context().Done()
			}))
			defer srv.Close()
			defer a.cancel()
			a.client, _ = emby.New(srv.URL, "token", "u", "d", 5)
			a.page, a.library, a.kind, a.watched = "library", "tv", "剧集", "未观看"
			a.offset, a.total = 120, 240
			a.sort = "名称"
			if tc.label == a.sort {
				a.sort = "最近添加"
			}
			tt := ui.NewTester(a.view, 940, 640)
			if err := tt.Click("排序方式"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(tc.label); err != nil {
				t.Fatal(err)
			}
			if a.offset != 0 || a.sort != tc.label {
				t.Fatal("sort selection did not reset pagination")
			}
			check := func(offset string) {
				t.Helper()
				select {
				case q := <-requests:
					if q.Get("SortBy") != tc.field || q.Get("SortOrder") != tc.order || q.Get("StartIndex") != offset || q.Get("Limit") != "60" || q.Get("ParentId") != "tv" || q.Get("IncludeItemTypes") != "Series" || q.Get("IsPlayed") != "false" {
						t.Fatalf("wrong sorting, pagination or filters: %v", q)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("sort request missing")
				}
			}
			check("0")
			a.offset = 60
			a.loadItems()
			check("60")
		})
	}
}
