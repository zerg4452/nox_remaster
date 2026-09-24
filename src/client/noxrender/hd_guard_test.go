package noxrender

import (
	"image"
	"image/color"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

// A writer that cannot mirror must never expose a stale HD frame.
func TestHDMutableBufferInvalidates(t *testing.T) {
	r := hdTestRender(3, 3)
	r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 9, 9)))
	r.PixBuffer().Set(1, 1, color.White)
	if r.EndHDFrame() != nil {
		t.Fatal("mutable buffer exposed stale HD frame")
	}
}

func TestHDUnhandledLineInvalidates(t *testing.T) {
	r := hdTestRender(3, 3)
	r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 9, 9)))
	r.DrawLine(image.Pt(0, 0), image.Pt(2, 2), color.White)
	if r.EndHDFrame() != nil {
		t.Fatal("unhandled diagonal exposed stale HD frame")
	}
}
