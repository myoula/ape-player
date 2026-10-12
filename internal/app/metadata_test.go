package app

import (
	"testing"
	"time"

	"ape-player/internal/emby"
	"github.com/egoist/mygo/ui"
)

func TestMediaCardYears(t *testing.T) {
	date := func(year int) time.Time { return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		name string
		item emby.Item
		want string
	}{
		{"movie", emby.Item{Type: "Movie", ProductionYear: 2026}, "2026"},
		{"continuing", emby.Item{Type: "Series", ProductionYear: 2025, Status: "Continuing"}, "2025–现在"},
		{"ended", emby.Item{Type: "Series", ProductionYear: 2020, Status: "Ended", EndDate: date(2026)}, "2020–2026"},
		{"ended same year", emby.Item{Type: "Series", ProductionYear: 2026, Status: "Ended", EndDate: date(2026)}, "2026"},
		{"ended without end date", emby.Item{Type: "Series", ProductionYear: 2026, Status: "Ended"}, "2026"},
		{"unknown status", emby.Item{Type: "Series", ProductionYear: 2026}, "2026"},
		{"premiere fallback", emby.Item{Type: "Series", PremiereDate: date(2025), Status: "Continuing"}, "2025–现在"},
		{"production year preferred", emby.Item{Type: "Movie", ProductionYear: 2024, PremiereDate: date(2025)}, "2024"},
		{"missing year", emby.Item{Type: "Series", Status: "Continuing"}, ""},
		{"episode subtitle", emby.Item{Type: "Episode", SeriesName: "电视剧", Name: "分集标题", ProductionYear: 2026, ParentIndexNumber: 1, IndexNumber: 2}, "S01E02 · 分集标题"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			tc.item.ID = "item"
			tt := ui.NewTester(func(c *ui.Context) { a.card(c, tc.item) }, 200, 320)
			if tc.want != "" && !tt.HasText(tc.want) {
				t.Fatalf("missing metadata %q", tc.want)
			}
			if _, zero := tt.Find("0"); zero || (tc.want == "" && yearLabel(tc.item) != "") {
				t.Fatal("missing year rendered as a value")
			}
			if tc.item.Status != "Continuing" && tt.HasText("2026–现在") {
				t.Fatal("unknown or ended series marked as continuing")
			}
		})
	}
}
