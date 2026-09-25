package render

import (
	"image"
	"testing"
)

// Catches a present buffer reallocated on every switch between the HD world
// size and the logical loading size, and wrong geometry after reuse.
func TestResizeImage16ReusesBacking(t *testing.T) {
	hd, logical := image.Rect(0, 0, 2560, 1440), image.Rect(0, 0, 1280, 720)
	buf := resizeImage16(nil, hd)
	base := &buf.Pix[:1][0]
	for i, rect := range []image.Rectangle{logical, hd, logical, hd} {
		buf = resizeImage16(buf, rect)
		if &buf.Pix[:1][0] != base {
			t.Fatalf("step %d %v: backing array reallocated", i, rect)
		}
		if buf.Rect != rect || buf.Stride != rect.Dx() || len(buf.Pix) != rect.Dx()*rect.Dy() {
			t.Fatalf("step %d: geometry rect=%v stride=%d len=%d", i, buf.Rect, buf.Stride, len(buf.Pix))
		}
	}
	if big := resizeImage16(buf, image.Rect(0, 0, 3840, 2160)); len(big.Pix) != 3840*2160 {
		t.Fatalf("grown len=%d", len(big.Pix))
	}
}
