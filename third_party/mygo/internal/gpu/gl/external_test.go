//go:build linux && (amd64 || arm64)

package gl

import (
	"github.com/egoist/mygo/internal/scene"
	"testing"
)

func TestExternalImageCompositesAndReleases(t *testing.T) {
	read := offscreen(t, false, 64, 64)
	r, e := New()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Release()
	rendered, released := 0, 0
	img := scene.NewImageRGBA(1, 1, []byte{0, 0, 0, 255})
	img.GL = &scene.GLImage{Width: 32, Height: 32, Render: func(_ uintptr, fbo uint32, w, h int) {
		rendered++
		if w != img.GL.Width || h != img.GL.Height {
			t.Fatalf("render target %dx%d, want %dx%d", w, h, img.GL.Width, img.GL.Height)
		}
		glBindFramebuffer(glFramebuffer, fbo)
		glViewport(0, 0, int32(w), int32(h))
		glDisable(glScissorTest)
		glClearColor(1, 0, 0, 1)
		glClear(glColorBufferBit)
	}, Release: func() { released++ }}
	s := &scene.Scene{Width: 64, Height: 64, Ops: []scene.Op{{Kind: scene.OpImage, Image: img, Rect: scene.Rect{W: 64, H: 64}, Src: scene.Rect{W: 1, H: 1}, Opacity: 1}, {Kind: scene.OpFill, Rect: scene.Rect{W: 16, H: 16}, Color: scene.Color{G: 255, A: 255}, Opacity: 1}}}
	// Empty atlases are sufficient for images and fills.
	s.MaskAtlas = scene.NewAtlas(1, 64, 64)
	s.ColorAtlas = scene.NewAtlas(4, 64, 64)
	if e := r.Render(s); e != nil {
		t.Fatal(e)
	}
	pix := read()
	if pix[(32*64+32)*4+2] < 240 {
		t.Fatalf("external video texture missing: center=%v corner=%v rendered=%d external=%v batches=%v", pix[(32*64+32)*4:(32*64+32)*4+4], pix[(4*64+4)*4:(4*64+4)*4+4], rendered, r.external, r.b.Batches)
	}
	if pix[(4*64+4)*4+1] < 240 {
		t.Fatal("native overlay not above external texture")
	}
	img.GL.Width, img.GL.Height = 48, 48
	img.Changed()
	if e := r.Render(s); e != nil {
		t.Fatal(e)
	}
	if released != 0 {
		t.Fatal("resize released producer")
	}
	s.Ops = nil
	if e := r.Render(s); e != nil {
		t.Fatal(e)
	}
	if rendered != 2 || released != 1 {
		t.Fatalf("render=%d release=%d", rendered, released)
	}
}
