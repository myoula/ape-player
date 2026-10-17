package ui

import (
	"image/color"
	"testing"
)

var benchmarkGLBitmap *Bitmap

func BenchmarkGLBitmap4K(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		benchmarkGLBitmap = NewGLBitmap(3840, 2160, nil, nil)
	}
}

func TestGLBitmapUsesSmallFallbackAndKeepsGeometry(t *testing.T) {
	bm := NewGLBitmap(3840, 2160, nil, nil)
	id := bm.img.ID()
	for _, size := range [][2]int{{3840, 2160}, {1920, 1080}, {7680, 4320}} {
		bm.ResizeGL(size[0], size[1])
		if w, h := bm.Size(); w != size[0] || h != size[1] {
			t.Fatalf("bitmap geometry %dx%d, want %v", w, h, size)
		}
		if bm.img.GL.Width != size[0] || bm.img.GL.Height != size[1] || bm.img.ID() != id {
			t.Fatal("resize lost GL target or image identity")
		}
		if len(bm.img.Pix) != 4 || cap(bm.img.Pix) != 4 || bm.img.W != 1 || bm.img.H != 1 {
			t.Fatal("GL bitmap allocated a full-sized CPU fallback")
		}
	}
	// Headless/CPU rendering must still show opaque black with the correct
	// aspect ratio, including after a resize; no mipmap may read full-size pixels.
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Size(100, 100).Background(Hex("#ffffff")).Children(func() {
			coreImage(c, bm).Size(100, 100).Fit(Contain)
		})
	}, 100, 100)
	img := tt.Image()
	if got := img.RGBAAt(50, 50); got != (color.RGBA{A: 255}) {
		t.Fatalf("fallback center = %v, want opaque black", got)
	}
	if got := img.RGBAAt(50, 5); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("aspect ratio lost: top pixel = %v", got)
	}
}
