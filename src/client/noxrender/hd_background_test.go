package noxrender

import (
	"encoding/binary"
	"image"
	"testing"
)

func TestHDBackgroundDrawEvidence(t *testing.T) {
	for _, kind := range []string{"full", "clipped", "offset", "trimmed", "alpha", "interlace"} {
		r := hdTestRender(3, 1)
		img := hdTestImage(3, 3, 2, 3, 0, 0, 0, 0, 0, 0)
		pos := image.Point{}
		switch kind {
		case "clipped":
			r.p.SetClip(true)
			r.p.SetClipRect(image.Rect(0, 0, 2, 1))
		case "offset":
			binary.LittleEndian.PutUint32(img.Pixdata()[8:], 1)
			r.p.SetClip(true)
		case "trimmed":
			r.Set_dword_5d4594_3799484(1)
		case "alpha":
			r.p.SetAlphaEnabled(true)
		case "interlace":
			r.SetInterlacing(true, 0)
		}
		r.DrawImage16(img, pos)
		// Wrappers restore this state before the menu callback executes.
		r.p.SetClip(false)
		r.Set_dword_5d4594_3799484(0)
		r.p.SetAlphaEnabled(false)
		r.SetInterlacing(false, 0)
		if got := r.ConsumeFullFrameImage(); got != (kind == "full") {
			t.Fatalf("%s eligible=%v", kind, got)
		}
		if r.ConsumeFullFrameImage() {
			t.Fatal("evidence consumed twice")
		}
	}
}
