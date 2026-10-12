package app

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVideoAspectRendering(t *testing.T) {
	a := testApp(t)
	source := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			pixel := color.RGBA{R: 255, A: 255}
			if x < 40 {
				pixel = color.RGBA{G: 255, A: 255}
			} else if x >= 120 {
				pixel = color.RGBA{B: 255, A: 255}
			}
			source.SetRGBA(x, y, pixel)
		}
	}
	a.bitmap = ui.NewBitmap(source)
	for _, tc := range []struct {
		mode        string
		top, bottom int
		cropped     bool
	}{
		{"原始比例", 88, 312, false},
		{"16:9", 88, 312, false},
		{"4:3", 50, 350, false},
		{"拉伸铺满", 0, 400, false},
		{"裁剪填充", 0, 400, true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			a.aspectRatio = tc.mode
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Background(ui.Hex("#000000")).Children(func() { a.videoImage(c) })
			}, 400, 400)
			img := tt.Image()
			for _, y := range []int{20, 65, 100, 200, 300, 335, 380} {
				r, g, b, _ := img.At(200, y).RGBA()
				inside := y >= tc.top && y < tc.bottom
				if inside && (r < 60000 || g > 1000 || b > 1000) {
					t.Fatalf("video missing at y=%d RGB=(%d,%d,%d)", y, r, g, b)
				}
				if !inside && (r > 1000 || g > 1000 || b > 1000) {
					t.Fatalf("expected letterbox at y=%d", y)
				}
			}
			r, g, _, _ := img.At(40, 200).RGBA()
			if tc.cropped && (r < 60000 || g > 1000) {
				t.Fatal("cover mode did not crop side content")
			}
			if !tc.cropped && (g < 60000 || r > 1000) {
				t.Fatal("side content was lost without cropping")
			}
		})
	}
}

func TestAspectMenuSelection(t *testing.T) {
	a := testApp(t)
	a.page = "player"
	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("画面比例选择"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("4:3"); err != nil {
		t.Fatal(err)
	}
	if a.aspectRatio != "4:3" || a.controlsHidden {
		t.Fatal("aspect choice did not apply or toggled controls")
	}
	if err := tt.Click("画面比例选择"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("原始比例"); err != nil {
		t.Fatal(err)
	}
	if a.aspectRatio != "原始比例" {
		t.Fatal("could not restore original ratio")
	}
}
