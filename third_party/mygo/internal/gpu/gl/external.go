//go:build (linux && (amd64 || arm64)) || js

package gl

import (
	"math"

	"github.com/egoist/mygo/internal/scene"
)

type externalImage struct {
	w, h         int
	texture, fbo uint32
	source       *scene.Image
}

// Geometry is already in device pixels. Reconstruct the full image footprint
// from its visible UV fraction (Cover may crop most of the original image).
// A uniform scale keeps mpv from inserting letterboxing into a distorted target.
func externalTargetSize(op scene.Op) (int, int) {
	img := op.Image
	w, h := img.GL.Width, img.GL.Height
	if !img.GL.FitToDisplay || w <= 0 || h <= 0 || op.Rect.Empty() || op.Src.Empty() || img.W <= 0 || img.H <= 0 || op.Transform.Perspective() {
		return w, h
	}
	xx, xy := op.Transform.VectorAt(0, 0, op.Rect.W, 0)
	yx, yy := op.Transform.VectorAt(0, 0, 0, op.Rect.H)
	pixelW := math.Hypot(float64(xx), float64(xy)) * float64(img.W) / float64(op.Src.W)
	pixelH := math.Hypot(float64(yx), float64(yy)) * float64(img.H) / float64(op.Src.H)
	scale := min(1, max(pixelW/float64(w), pixelH/float64(h)))
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return w, h
	}
	// UV division can put an integer footprint a few ULPs above the integer.
	return max(1, int(math.Ceil(float64(w)*scale-1e-6))), max(1, int(math.Ceil(float64(h)*scale-1e-6)))
}

// Render external producers before atlas uploads and batching, then rebind all
// state the renderer uses. Producers never receive the window's framebuffer.
func (r *Renderer) prepareExternal(s *scene.Scene) {
	if r.external == nil {
		r.external = make(map[uint64]*externalImage)
	}
	type target struct {
		image *scene.Image
		w, h  int
	}
	// A bitmap can appear more than once. Use the largest required target so
	// draw order cannot make a larger copy blurry or resize the FBO twice.
	targets := map[uint64]target{}
	for _, op := range s.Ops {
		img := op.Image
		if img == nil || img.GL == nil {
			continue
		}
		w, h := externalTargetSize(op)
		if w <= 0 || h <= 0 {
			continue
		}
		old := targets[img.ID()]
		targets[img.ID()] = target{img, max(old.w, w), max(old.h, h)}
	}
	var previous int32
	glGetIntegerv(glFramebufferBinding, &previous)
	for _, target := range targets {
		img, w, h := target.image, target.w, target.h
		e := r.external[img.ID()]
		if e != nil && (e.w != w || e.h != h) {
			glDeleteFramebuffers(1, &e.fbo)
			glDeleteTextures(1, &e.texture)
			e = nil
		}
		if e == nil {
			e = &externalImage{source: img, w: w, h: h}
			glActiveTexture(glTexture0)
			e.texture = newTexture(glRGBA8, glRGBA, w, h, nil, w*4)
			glGenFramebuffers(1, &e.fbo)
			glBindFramebuffer(glFramebuffer, e.fbo)
			glFramebufferTexture2D(glFramebuffer, glColorAttachment0, glTexture2D, e.texture, 0)
			r.external[img.ID()] = e
		}
		img.GL.Render(r.context, e.fbo, w, h)
	}
	for id, e := range r.external {
		if _, seen := targets[id]; !seen {
			e.release()
			delete(r.external, id)
		}
	}
	glBindFramebuffer(glFramebuffer, uint32(previous))
	glActiveTexture(glTexture0)
	glBindBuffer(0x88EC, 0) // GL_PIXEL_UNPACK_BUFFER
	glPixelStorei(glUnpackAlignment, 4)
	glPixelStorei(glUnpackRowLength, 0)
	glDisable(0x0B71) // depth
	glDisable(0x0B44) // cull
	glDisable(0x0B90) // stencil
}
func (r *Renderer) externalTexture(img *scene.Image) uintptr {
	if e := r.external[img.ID()]; e != nil {
		return uintptr(e.texture)
	}
	return 0
}
func (e *externalImage) release() {
	if e.source.GL.Release != nil {
		e.source.GL.Release()
	}
	glDeleteFramebuffers(1, &e.fbo)
	glDeleteTextures(1, &e.texture)
}
func (r *Renderer) releaseExternal() {
	for _, e := range r.external {
		e.release()
	}
	clear(r.external)
}
