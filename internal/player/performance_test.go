package player

import (
	"testing"
	"unsafe"
)

// Stable native property buffers let tests count reads without a decoder or GPU.
func fakeProperties(t testing.TB) (map[string][]byte, map[string]int) {
	t.Helper()
	previous := lib
	t.Cleanup(func() { lib = previous })
	values := map[string][]byte{}
	for key, value := range map[string]string{
		"time-pos": "12.5", "duration": "120", "volume": "70",
		"dwidth": "1920", "dheight": "1080", "frame-drop-count": "0",
		"pause": "no", "mute": "no", "paused-for-cache": "no",
		"hwdec-current": "vaapi", "video-format": "h264", "audio-codec-name": "aac",
		"track-list":          `[{"id":1,"type":"video","codec":"h264"},{"id":1,"type":"audio","lang":"eng","selected":true},{"id":2,"type":"audio","lang":"jpn"},{"id":1,"type":"sub","lang":"zho","selected":true}]`,
		"demuxer-cache-state": `{"seekable-ranges":[{"start":0,"end":40}],"reader-pts":12.5,"cache-end":40}`,
	} {
		values[key] = append([]byte(value), 0)
	}
	reads := map[string]int{}
	lib.property = func(_ uintptr, name string) uintptr {
		reads[name]++
		if value := values[name]; len(value) > 0 {
			return uintptr(unsafe.Pointer(&value[0]))
		}
		return 0
	}
	lib.free = func(uintptr) {}
	return values, reads
}

func BenchmarkPlaybackState(b *testing.B) {
	fakeProperties(b)
	p := &Player{}
	p.state()
	b.ReportAllocs()
	for b.Loop() {
		p.state()
	}
}

func TestStateRefreshesChangedMetadata(t *testing.T) {
	values, reads := fakeProperties(t)
	p := &Player{}
	first := p.state()
	values["time-pos"] = append([]byte("14"), 0)
	values["demuxer-cache-state"] = append([]byte(`{"seekable-ranges":[{"start":2,"end":50}]}`), 0)
	next := p.state()
	if next.Position != 14 || len(next.CacheRanges) != 1 || next.CacheRanges[0].End != 50 {
		t.Fatalf("progress/cache stopped updating: %+v", next)
	}
	if reads["track-list"] != 1 || reads["dwidth"] != 1 || reads["time-pos"] != 2 {
		t.Fatalf("unchanged metadata reread: %v", reads)
	}
	values["pause"] = append([]byte("yes"), 0)
	values["dwidth"] = append([]byte("3840"), 0)
	p.invalidateState(mpvEvent{ID: 22, Userdata: metadataObservation})
	next = p.state()
	if !next.Paused || next.Width != 3840 || reads["track-list"] != 1 {
		t.Fatalf("metadata invalidation failed: %+v reads=%v", next, reads)
	}
	// A track switch or external subtitle arrival must not mutate a snapshot
	// still being read by the UI.
	values["track-list"] = append([]byte(`[{"id":2,"type":"audio","selected":true},{"id":2,"type":"sub","external":true}]`), 0)
	p.invalidateState(mpvEvent{ID: 22, Userdata: tracksObservation})
	next = p.state()
	if len(next.Tracks) != 2 || next.Tracks[0].ID != 2 || len(first.Tracks) != 4 || first.Tracks[1].ID != 1 {
		t.Fatalf("track refresh mutated snapshot: first=%v next=%v", first.Tracks, next.Tracks)
	}
	// Missing metadata in the next file clears stale dimensions and tracks.
	delete(values, "track-list")
	delete(values, "dwidth")
	p.invalidateState(mpvEvent{ID: 6})
	next = p.state()
	if len(next.Tracks) != 0 || next.Width != 0 {
		t.Fatalf("metadata leaked into next file: %+v", next)
	}
	values["dwidth"] = append([]byte("1280"), 0)
	p.invalidateState(mpvEvent{ID: 24})
	if got := p.state().Width; got != 1280 {
		t.Fatalf("queue overflow did not refresh metadata: %d", got)
	}
}

func TestRenderReusesFrameUntilUpdatedOrTargetChanged(t *testing.T) {
	previous := lib
	t.Cleanup(func() { lib = previous })
	var updates uint64
	updateCalls, renders := 0, 0
	lib.renderUpdate = func(uintptr) uint64 {
		updateCalls++
		return updates
	}
	lib.render = func(uintptr, *param) int32 { renders++; return 0 }
	lib.renderCallback = func(uintptr, uintptr, uintptr) {}
	lib.renderFree = func(uintptr) {}
	p := &Player{renderContext: 1, glContext: 2}
	p.Render(2, 3, 1920, 1080) // First draw, even without an update flag.
	for range 10 {
		p.Render(2, 3, 1920, 1080) // UI/controls repaint the same video texture.
	}
	if renders != 1 || updateCalls != 11 {
		t.Fatalf("idle repaint: renders=%d updates=%d", renders, updateCalls)
	}
	updates = 1 // New frame, seek, or subtitle/OSD redraw from mpv.
	p.Render(2, 3, 1920, 1080)
	updates = 0
	p.Render(2, 3, 3840, 2160) // Resized target must be filled, even paused.
	p.Render(2, 4, 3840, 2160) // New FBO must also be filled.
	if renders != 4 || p.FrameCount() != 4 {
		t.Fatalf("required draws missing: renders=%d frames=%d", renders, p.FrameCount())
	}
	p.ReleaseGraphics()
	p.renderContext = 1 // Simulate a recreated context with the same target IDs.
	p.Render(2, 4, 3840, 2160)
	if renders != 5 {
		t.Fatal("recreated graphics reused stale frame")
	}
}
