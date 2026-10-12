package player

import (
	"reflect"
	"testing"
)

func TestCacheRanges(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []CacheRange
	}{
		{"separate ranges", `{"seekable-ranges":[{"start":0,"end":30},{"start":80,"end":100}],"reader-pts":10,"cache-end":110}`, []CacheRange{{0, 30}, {80, 100}}},
		{"read ahead", `{"seekable-ranges":[],"reader-pts":12,"cache-end":40}`, []CacheRange{{12, 40}}},
		{"unavailable", `{}`, nil},
		{"invalid json", `unavailable`, nil},
		{"incomplete ranges", `{"seekable-ranges":[{"start":null,"end":30},{"start":20},{"start":50,"end":10}]}`, nil},
		{"missing reader", `{"cache-end":100}`, nil},
		{"reset", `{"seekable-ranges":[],"reader-pts":null,"cache-end":null}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCacheRanges(tc.raw); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
