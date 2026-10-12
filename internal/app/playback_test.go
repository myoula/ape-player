package app

import (
	"ape-player/internal/emby"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestReporterOrdersStartStopAndCleansTranscode(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c, _ := emby.New(srv.URL, "token", "u", "d", 5)
	r := newReporter(c, nil)
	p := emby.Playback{SessionID: "s", Method: "Transcode"}
	r.ch <- report{event: "", playback: p}
	r.stop <- report{event: "/Stopped", playback: p}
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("reporter did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 || calls[0] != "POST /Sessions/Playing" || calls[1] != "POST /Sessions/Playing/Stopped" || calls[2] != "DELETE /Videos/ActiveEncodings" {
		t.Fatal(calls)
	}
}
func TestSourceGenerationRejectsStaleResult(t *testing.T) {
	a := testApp(t)
	old := a.generation
	ctx := a.ctx
	a.activate("")
	if old == a.generation || ctx.Err() != context.Canceled {
		t.Fatal("source generation not invalidated")
	}
}
