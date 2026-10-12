package ui

import "testing"

func TestDragScrollClicksAndBounds(t *testing.T) {
	var offset ScrollState
	clicks := 0
	enabled := true
	tt := coreNewTester(func(c *context) {
		row := coreScrollHorizontal(c).Fill().TrackScroll(&offset)
		if enabled {
			row.DragScroll()
		}
		row.Children(func() {
			coreRow(c).Width(800).Height(100).Shrink(0).Children(func() {
				if coreButton(c, "Card").Width(200).Height(70).ContextMenu(func(m *Menu) { m.Item("Open") }).Clicked() {
					clicks++
				}
			})
		})
	}, 300, 100)
	tt.Press(150, 30)
	tt.Release(147, 31)
	if clicks != 1 || offset.X != 0 {
		t.Fatalf("small movement must click: clicks=%d offset=%v", clicks, offset.X)
	}
	tt.Press(150, 30)
	tt.Move(70, 30)
	tt.Release(70, 30)
	if clicks != 1 || offset.X != 80 {
		t.Fatalf("drag must pan without clicking: clicks=%d offset=%v", clicks, offset.X)
	}
	if err := tt.RightClick("Card"); err != nil || len(tt.Menu()) == 0 {
		t.Fatalf("context menu lost: %v", err)
	}
	tt.CloseMenu()
	// Empty space also pans; both ends clamp, and returning from an edge is immediate.
	tt.Press(250, 85)
	tt.Move(-1000, 85)
	if offset.X != offset.MaxX {
		t.Fatalf("end not clamped: %+v", offset)
	}
	tt.Move(-990, 85)
	if offset.X != offset.MaxX-10 {
		t.Fatalf("drag stuck at end: %+v", offset)
	}
	tt.Release(1000, 85)
	if offset.X != 0 || clicks != 1 {
		t.Fatalf("start not clamped: %+v clicks=%d", offset, clicks)
	}
	if err := tt.Click("Card"); err != nil || clicks != 2 {
		t.Fatalf("click after dragging failed: %v clicks=%d", err, clicks)
	}
	enabled = false
	tt.Frame()
	tt.Press(150, 30)
	tt.Release(70, 30)
	if offset.X != 0 || clicks != 3 {
		t.Fatalf("default behavior changed: %+v clicks=%d", offset, clicks)
	}
}

func TestDragScrollCancellation(t *testing.T) {
	for _, reason := range []string{"blur", "remove", "disable"} {
		t.Run(reason, func(t *testing.T) {
			var offset ScrollState
			visible, disabled := true, false
			tt := coreNewTester(func(c *context) {
				if visible {
					coreScrollHorizontal(c).Fill().DragScroll().Disabled(disabled).TrackScroll(&offset).Children(func() {
						coreBox(c).Width(800).Height(100).Shrink(0)
					})
				}
			}, 300, 100)
			tt.Press(250, 50)
			tt.Move(200, 50)
			switch reason {
			case "blur":
				tt.SetFocused(false)
			case "remove":
				visible = false
				tt.Frame()
			case "disable":
				disabled = true
				tt.Frame()
			}
			tt.Move(100, 50)
			tt.Release(100, 50)
			if offset.X != 50 || tt.rt.contentDrag.st != nil || tt.rt.pressed != nil {
				t.Fatalf("gesture survived %s: %+v", reason, offset)
			}
		})
	}
}

func TestDragScrollSurvivesVirtualChildRemoval(t *testing.T) {
	var offset ScrollState
	clicks := 0
	tt := coreNewTester(func(c *context) {
		coreScrollHorizontal(c).Fill().DragScroll().TrackScroll(&offset).Children(func() {
			coreBox(c).Width(800).Height(100).Shrink(0).Children(func() {
				if offset.X < 50 {
					if coreButton(c, "Virtual card").Width(200).Clicked() {
						clicks++
					}
				}
			})
		})
	}, 300, 100)
	tt.Press(150, 15)
	tt.Move(80, 15)
	tt.Move(20, 15)
	tt.Release(20, 15)
	if offset.X != 130 || clicks != 0 {
		t.Fatalf("virtual removal interrupted drag: %+v clicks=%d", offset, clicks)
	}
}

func TestDragScrollPreservesSliderAndWheel(t *testing.T) {
	var offset ScrollState
	value := 0.0
	tt := coreNewTester(func(c *context) {
		coreScrollHorizontal(c).Fill().DragScroll().TrackScroll(&offset).Children(func() {
			coreRow(c).Width(800).Height(100).Shrink(0).Children(func() {
				coreSlider(c, &value, 0, 1).Width(200).Label("Volume")
			})
		})
	}, 300, 100)
	r, ok := tt.Find("Volume")
	if !ok {
		t.Fatal("slider missing")
	}
	tt.Press(r.X+10, r.Y+r.H/2)
	tt.Move(r.X+150, r.Y+r.H/2)
	tt.Release(r.X+150, r.Y+r.H/2)
	if offset.X != 0 || value < .6 {
		t.Fatalf("slider drag was hijacked: value=%v offset=%v", value, offset.X)
	}
	tt.Scroll(250, 50, 80, 0)
	if offset.X != 80 {
		t.Fatalf("wheel scrolling broken: %+v", offset)
	}
}
