package app

import (
	"testing"

	"ape-player/internal/player"
	"github.com/egoist/mygo/ui"
)

func TestBufferedProgressRenderingAndSeek(t *testing.T) {
	a := testApp(t)
	pos, total := 20.0, 100.0
	offset := 0.0
	ranges := []player.CacheRange{{Start: 0, End: 40}, {Start: 60, End: 80}}
	seeks := 0
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.theme)
		bufferedSlider(c, &pos, total, ranges, offset).Label("progress").OnChange(func() { seeks++ })
	}, 400, 40)
	r, _ := tt.Find("progress")
	x := func(f float32) float32 { return r.X + 10 + (r.W-20)*f }
	y := r.Y + r.H/2
	pixels := tt.Image()
	played := pixels.RGBAAt(int(x(.1)), int(y))
	buffered := pixels.RGBAAt(int(x(.3)), int(y))
	gap := pixels.RGBAAt(int(x(.5)), int(y))
	later := pixels.RGBAAt(int(x(.7)), int(y))
	if played.G <= played.R || buffered.R <= gap.R || buffered != later {
		t.Fatalf("cache segments not distinguishable: %v %v %v %v", played, buffered, gap, later)
	}
	tt.Press(x(.5), y)
	tt.Move(x(.9), y)
	tt.Release(x(.9), y)
	if pos < 89 || pos > 91 || seeks == 0 {
		t.Fatalf("cache slider broke dragging: %v seeks=%d", pos, seeks)
	}
	pos = 20
	offset = 30
	ranges = []player.CacheRange{{Start: 0, End: 10}, {Start: 60, End: 200}}
	tt.Frame()
	if tt.Image().RGBAAt(int(x(.35)), int(y)) != buffered || tt.Image().RGBAAt(int(x(.5)), int(y)) != gap || tt.Image().RGBAAt(int(x(.95)), int(y)) != buffered {
		t.Fatal("buffered ranges did not honor timeline offset and duration bounds")
	}
	ranges = nil
	tt.Frame()
	if got := tt.Image().RGBAAt(int(x(.3)), int(y)); got != gap {
		t.Fatal("old cache remained after state reset")
	}
	total = 0
	tt.Frame()
	if got := tt.Image().RGBAAt(int(x(.3)), int(y)); got != gap {
		t.Fatal("unknown duration should not show cache")
	}
}
