//go:build linux && (amd64 || arm64)

package gl

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

func TestExternalTargetSize(t *testing.T) {
	img := scene.NewImageRGBA(1, 1, []byte{0, 0, 0, 255})
	img.GL = &scene.GLImage{Width: 3840, Height: 2160, FitToDisplay: true}
	for _, tc := range []struct {
		name      string
		rect, src scene.Rect
		transform scene.Affine
		w, h      int
	}{
		{"small window", scene.Rect{W: 960, H: 540}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 960, 540},
		{"HiDPI physical pixels", scene.Rect{W: 1920, H: 1080}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 1920, 1080},
		{"fullscreen", scene.Rect{W: 3840, H: 2160}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 3840, 2160},
		{"no upscale allocation", scene.Rect{W: 7680, H: 4320}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 3840, 2160},
		{"forced 4:3 preserves source", scene.Rect{W: 960, H: 720}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 1280, 720},
		{"cover uses full image footprint", scene.Rect{W: 960, H: 960}, scene.Rect{X: .21875, W: .5625, H: 1}, scene.Affine{}, 1707, 960},
		{"rotated", scene.Rect{W: 960, H: 540}, scene.Rect{W: 1, H: 1}, scene.Rotation(90), 960, 540},
		{"scaled and reflected", scene.Rect{W: 960, H: 540}, scene.Rect{W: 1, H: 1}, scene.Scaling(-2, 2), 1920, 1080},
		{"subpixel minimum", scene.Rect{W: .01, H: .01}, scene.Rect{W: 1, H: 1}, scene.Affine{}, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, h := externalTargetSize(scene.Op{Image: img, Rect: tc.rect, Src: tc.src, Transform: tc.transform})
			if w != tc.w || h != tc.h {
				t.Fatalf("target %dx%d, want %dx%d", w, h, tc.w, tc.h)
			}
		})
	}
	img.GL.FitToDisplay = false
	if w, h := externalTargetSize(scene.Op{Image: img, Rect: scene.Rect{W: 960, H: 540}, Src: scene.Rect{W: 1, H: 1}}); w != 3840 || h != 2160 {
		t.Fatalf("fixed target changed: %dx%d", w, h)
	}
}

func TestExternalDisplayResizeKeepsProducerAndPixels(t *testing.T) {
	read := offscreen(t, false, 64, 64)
	r, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	renders, releases := 0, 0
	var sizes [][2]int
	img := scene.NewImageRGBA(1, 1, []byte{0, 0, 0, 255})
	img.GL = &scene.GLImage{Width: 3840, Height: 2160, FitToDisplay: true, Render: func(_ uintptr, fbo uint32, w, h int) {
		renders++
		sizes = append(sizes, [2]int{w, h})
		glBindFramebuffer(glFramebuffer, fbo)
		glViewport(0, 0, int32(w), int32(h))
		glDisable(glScissorTest)
		glClearColor(1, 0, 0, 1)
		glClear(glColorBufferBit)
		glEnable(glScissorTest)
		glScissor(int32(w/2), 0, int32(w-w/2), int32(h))
		glClearColor(0, 0, 1, 1)
		glClear(glColorBufferBit)
	}, Release: func() { releases++ }}
	op := scene.Op{Kind: scene.OpImage, Image: img, Rect: scene.Rect{W: 64, H: 36}, Src: scene.Rect{W: 1, H: 1}, Opacity: 1}
	s := &scene.Scene{Width: 64, Height: 64, MaskAtlas: scene.NewAtlas(1, 64, 64), ColorAtlas: scene.NewAtlas(4, 64, 64)}
	for _, width := range []float32{64, 32, 64} {
		op.Rect.W, op.Rect.H = width, width*9/16
		s.Ops = []scene.Op{op}
		if err := r.Render(s); err != nil {
			t.Fatal(err)
		}
		want := [2]int{int(width), int(width * 9 / 16)}
		if sizes[len(sizes)-1] != want || releases != 0 {
			t.Fatalf("resize: sizes=%v releases=%d", sizes, releases)
		}
		pixels := read()
		left, right := (4*64+int(width/4))*4, (4*64+int(width*3/4))*4
		if pixels[left+2] < 240 || pixels[right] < 240 {
			t.Fatal("resized texture lost left red/right blue sampling")
		}
	}
	// The same image shown at two sizes renders once, using the larger target.
	small := op
	small.Rect.W, small.Rect.H = 16, 9
	s.Ops = []scene.Op{small, op}
	before := renders
	if err := r.Render(s); err != nil {
		t.Fatal(err)
	}
	if renders != before+1 || sizes[len(sizes)-1] != [2]int{64, 36} {
		t.Fatalf("duplicate image: renders=%d sizes=%v", renders, sizes)
	}
	s.Ops = nil
	if err := r.Render(s); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("releases=%d", releases)
	}
}
