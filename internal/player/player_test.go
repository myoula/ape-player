package player

import "testing"

func TestTrackMapping(t *testing.T) {
	tracks := []Track{{ID: 1, Type: "audio", FFIndex: 2}, {ID: 2, Type: "audio", FFIndex: 4}, {ID: 1, Type: "sub", FFIndex: 3}, {ID: 2, Type: "sub", FFIndex: 0, External: true}}
	for _, tc := range []struct {
		kind      string
		index, id int
		ok        bool
	}{{"audio", 4, 2, true}, {"sub", 3, 1, true}, {"sub", 0, 0, false}, {"audio", 1, 0, false}} {
		id, ok := TrackID(tracks, tc.kind, tc.index)
		if id != tc.id || ok != tc.ok {
			t.Fatalf("%+v => %d %v", tc, id, ok)
		}
	}
}

func TestHTTPHeaders(t *testing.T) {
	got := headerFields(map[string]string{"User-Agent": "client", "Referer": "https://example.com/", "X-Test": `a,b\c`, "Bad": "value\r\ninjected: yes"})
	want := `Referer: https://example.com/,X-Test: a\,b\\c`
	if got != want {
		t.Fatalf("headers %q, want %q", got, want)
	}
}
