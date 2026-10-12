package ui

// contentDrag starts as a possible click and captures the scroller only once
// movement is horizontal. Capturing the container survives virtualized children
// leaving the view while it scrolls.
type contentDrag struct {
	st     *state
	x, y   float32
	active bool
}

func (rt *engine) beginContentDrag(chain []uint64, x, y float32) {
	for _, id := range chain {
		s := rt.states[id]
		if s == nil {
			continue
		}
		if s.flags&(flagDisabled|flagEditable|flagSelectable|flagDraggable|flagTrackPointer|flagDragWindow|flagMenuButton) != 0 || s.input != nil {
			return
		}
		if s.dragScroll && s.flags&flagScrollX != 0 && s.contentW > float64(s.w) {
			rt.contentDrag = contentDrag{st: s, x: x, y: y}
			return
		}
	}
}

func (rt *engine) cancelContentDrag() {
	if d := &rt.contentDrag; d.active && rt.pressed == d.st {
		rt.pressed.pressed = false
		rt.pressed = nil
	}
	rt.contentDrag = contentDrag{}
}

func (rt *engine) moveContentDrag(x, y float32) bool {
	d := &rt.contentDrag
	s := d.st
	if s == nil {
		return false
	}
	if !s.dragScroll || s.flags&flagDisabled != 0 {
		rt.cancelContentDrag()
		return false
	}
	dx, dy := s.vector(d.x, d.y, x-d.x, y-d.y)
	if !d.active {
		if abs32(dy) > 6 && abs32(dy) >= abs32(dx) {
			rt.cancelContentDrag()
			return false
		}
		if abs32(dx) <= 6 || abs32(dx) <= abs32(dy) {
			return false
		}
		if p := rt.pressed; p != nil {
			p.pressed, p.pressPending = false, false
		}
		rt.pressed, rt.pressButton = s, 0
		s.pressed = true
		rt.lastPress.id = 0
		d.active = true
	}
	scrollBy(s, -dx, 0)
	d.x, d.y = x, y
	rt.pointerX, rt.pointerY, rt.pointerIn = x, y, true
	rt.requestFrame()
	return true
}
