//go:build (linux && (amd64 || arm64)) || js

package gl

import "github.com/egoist/mygo/internal/scene"

type externalImage struct {
	w, h         int
	texture, fbo uint32
	source       *scene.Image
}

// Render external producers before atlas uploads and batching, then rebind all
// state the renderer uses. Producers never receive the window's framebuffer.
func (r *Renderer) prepareExternal(s *scene.Scene) {
	if r.external == nil {
		r.external = make(map[uint64]*externalImage)
	}
	seen := map[uint64]bool{}
	var previous int32
	glGetIntegerv(glFramebufferBinding, &previous)
	for _, op := range s.Ops {
		img := op.Image
		if img == nil || img.GL == nil || seen[img.ID()] {
			continue
		}
		seen[img.ID()] = true
		e := r.external[img.ID()]
		if e != nil && (e.w != img.W || e.h != img.H) {
			glDeleteFramebuffers(1, &e.fbo)
			glDeleteTextures(1, &e.texture)
			e = nil
		}
		if e == nil {
			e = &externalImage{source: img, w: img.W, h: img.H}
			glActiveTexture(glTexture0)
			e.texture = newTexture(glRGBA8, glRGBA, img.W, img.H, nil, img.W*4)
			glGenFramebuffers(1, &e.fbo)
			glBindFramebuffer(glFramebuffer, e.fbo)
			glFramebufferTexture2D(glFramebuffer, glColorAttachment0, glTexture2D, e.texture, 0)
			r.external[img.ID()] = e
		}
		img.GL.Render(r.context, e.fbo, img.W, img.H)
	}
	for id, e := range r.external {
		if !seen[id] {
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
