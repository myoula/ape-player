package player

import "encoding/json"

// CacheRange is a buffered interval on mpv's media timeline, in seconds.
type CacheRange struct {
	Start, End float64
}

func parseCacheRanges(raw string) []CacheRange {
	var state struct {
		Ranges []struct{ Start, End *float64 } `json:"seekable-ranges"`
		Reader *float64                        `json:"reader-pts"`
		End    *float64                        `json:"cache-end"`
	}
	if json.Unmarshal([]byte(raw), &state) != nil {
		return nil
	}
	var ranges []CacheRange
	for _, r := range state.Ranges {
		if r.Start != nil && r.End != nil && *r.End > *r.Start {
			ranges = append(ranges, CacheRange{*r.Start, *r.End})
		}
	}
	// Some demuxers cannot expose seekable ranges but do report read-ahead.
	if len(ranges) == 0 && state.Reader != nil && state.End != nil && *state.End > *state.Reader {
		ranges = append(ranges, CacheRange{*state.Reader, *state.End})
	}
	return ranges
}
