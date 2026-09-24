package input

import (
	"image"
	"testing"
)

func TestQueuedMouseClickPreservesTransitions(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  noxMouseEventType
		btn  MouseButton
	}{
		{"left", noxMouseEventLeft, NOX_MOUSE_LEFT},
		{"right", noxMouseEventRight, NOX_MOUSE_RIGHT},
		{"middle", noxMouseEventMiddle, NOX_MOUSE_MIDDLE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMouseHandler(&Handler{inputSeq: 1}, false)
			h.SetDrawWinSize(image.Pt(640, 480))
			pos := image.Pt(200, 150)
			h.pushEvent(noxMouseEvent{Type: noxMouseEventMotion, Pos: pos})
			// Two complete clicks arrive between consecutive game ticks.
			for i := 0; i < 2; i++ {
				h.pushEvent(noxMouseEvent{Type: tc.typ, Pos: pos, Pressed: true})
				h.pushEvent(noxMouseEvent{Type: tc.typ, Pos: pos})
			}
			for i, want := range []MouseState{NOX_MOUSE_DOWN, NOX_MOUSE_UP, NOX_MOUSE_DOWN, NOX_MOUSE_UP} {
				h.nox_client_readMouseBuffer_4306A0(uint(i+1), true)
				if got := h.cur.btn[tc.btn].state; got != ToMouseState(tc.btn, want) {
					t.Fatalf("tick %d: got %d, want %d", i+1, got, ToMouseState(tc.btn, want))
				}
				if h.GetMousePos() != pos {
					t.Fatalf("position: got %v, want %v", h.GetMousePos(), pos)
				}
			}
			if h.nox_client_readMouseBuffer_4306A0(5, true) || h.cur.btn[tc.btn].pressed {
				t.Fatal("click queue should be empty and button released")
			}
		})
	}
}

func TestQueuedMouseDragPreservesMotionAndWheel(t *testing.T) {
	h := newMouseHandler(&Handler{inputSeq: 1}, false)
	h.SetDrawWinSize(image.Pt(640, 480))
	start, end := image.Pt(200, 150), image.Pt(250, 170)
	h.pushEvent(noxMouseEvent{Type: noxMouseEventLeft, Pos: start, Pressed: true})
	h.nox_client_readMouseBuffer_4306A0(1, true)
	if h.cur.btn[NOX_MOUSE_LEFT].state != NOX_MOUSE_LEFT_DOWN {
		t.Fatal("drag must begin with down")
	}
	h.pushEvent(noxMouseEvent{Type: noxMouseEventMotion, Pos: end})
	h.nox_client_readMouseBuffer_4306A0(2, true)
	if h.cur.btn[NOX_MOUSE_LEFT].state != NOX_MOUSE_LEFT_PRESSED || h.GetMousePos() != end {
		t.Fatal("held button must preserve motion")
	}
	h.pushEvent(noxMouseEvent{Type: noxMouseEventWheel, Pos: end, Wheel: 1})
	h.pushEvent(noxMouseEvent{Type: noxMouseEventLeft, Pos: end})
	h.nox_client_readMouseBuffer_4306A0(3, true)
	if h.cur.btn[NOX_MOUSE_LEFT].state != NOX_MOUSE_LEFT_DRAG_END || h.GetMouseWheel() != 19 {
		t.Fatal("release must preserve drag end and wheel")
	}
	h.nox_client_readMouseBuffer_4306A0(4, true)
	if h.cur.btn[NOX_MOUSE_LEFT].pressed || h.GetMouseWheel() != 0 {
		t.Fatal("released button and wheel must reset")
	}
}
