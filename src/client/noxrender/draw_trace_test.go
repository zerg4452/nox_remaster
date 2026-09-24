package noxrender

import (
	"encoding/binary"
	"image"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"golang.org/x/image/font/basicfont"
)

func TestDrawTracePreservesPixels(t *testing.T) {
	draw := func(trace bool) ([]uint16, map[string]uint64) {
		r := NewRender(nil)
		r.SetData(newRenderData(32, 32))
		r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, 32, 32)))
		if trace {
			r.BeginDrawTrace()
		}
		r.DrawLineHorizontal(0, 0, 10, color.White)
		data := make([]byte, 21)
		binary.LittleEndian.PutUint32(data, 1)
		binary.LittleEndian.PutUint32(data[4:], 1)
		copy(data[17:], []byte{2, 1, 0, 0x7c})
		r.DrawImage16(NewRawImage16(3, data), image.Pt(2, 2))
		r.drawString(basicfont.Face7x13, "PRIVATE_TEXT_SENTINEL", image.Pt(2, 4))
		r.drawFadeScreen(image.Rect(0, 0, 32, 32), 1)
		summary := r.EndDrawTrace()
		return r.PixBuffer().Pix, summary
	}
	baseline, disabled := draw(false)
	actual, counts := draw(true)
	if !reflect.DeepEqual(actual, baseline) {
		t.Fatal("tracing changed output pixels")
	}
	if disabled != nil {
		t.Fatal("disabled tracing produced counters")
	}
	for _, key := range []string{"buffer:DrawLineHorizontal", "buffer:drawFadeScreen", "text:drawString", "image:type3", "image:op2"} {
		if counts[key] != 1 {
			t.Errorf("%s=%d, want 1", key, counts[key])
		}
	}
}

func TestDrawTraceUnknownImageWithoutRenderData(t *testing.T) {
	r := NewRender(nil)
	r.BeginDrawTrace()
	r.DrawImage16(NewRawImage16(63, nil), image.Point{})
	if counts := r.EndDrawTrace(); counts["image:type63"] != 1 {
		t.Fatal("missing unknown type count")
	}
}

func TestDrawTraceNoTextContent(t *testing.T) {
	r := NewRender(nil)
	r.SetData(newRenderData(32, 32))
	r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, 32, 32)))
	r.BeginDrawTrace()
	r.drawString(basicfont.Face7x13, "PRIVATE_TEXT_SENTINEL", image.Point{})
	counts := r.EndDrawTrace()
	if counts["text:drawString"] != 1 {
		t.Fatal("missing text operation")
	}
	for key := range counts {
		if strings.Contains(key, "PRIVATE_TEXT_SENTINEL") {
			t.Fatal("trace contains rendered text")
		}
	}
}

func TestDrawTraceEndsAndResets(t *testing.T) {
	r := NewRender(nil)
	r.SetData(newRenderData(4, 4))
	r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, 4, 4)))
	r.BeginDrawTrace()
	r.DrawPixel(image.Pt(0, 0), color.White)
	first := r.EndDrawTrace()
	r.DrawPixel(image.Pt(1, 0), color.White)
	if first["buffer:DrawPixel"] != 1 {
		t.Fatal("ended summary missing or changed")
	}
	if r.EndDrawTrace() != nil {
		t.Fatal("trace remained active")
	}
	r.BeginDrawTrace()
	if counts := r.EndDrawTrace(); len(counts) != 0 {
		t.Fatal("new frame retained old counters")
	}
}

// Characterize the actual type-8 operation observed during menu clicks. It is
// additive/saturating, not a source replacement or ordinary alpha-over.
func TestMenuParticleAdditiveTrace(t *testing.T) {
	for _, tc := range []struct{ background, want uint16 }{
		{0x0000, 0x7c00}, {0x001f, 0x7c1f}, {0x7c00, 0x7c00},
	} {
		r := NewRender(nil)
		r.SetData(newRenderData(2, 2))
		pix := noximage.NewImage16(image.Rect(0, 0, 2, 2))
		pix.Pix[0] = tc.background
		r.SetPixBuffer(pix)
		data := make([]byte, 21)
		binary.LittleEndian.PutUint32(data, 1)
		binary.LittleEndian.PutUint32(data[4:], 1)
		copy(data[17:], []byte{7, 1, 0, 0xf8})
		r.BeginDrawTrace()
		r.DrawImage16(NewRawImage16(8, data), image.Point{})
		counts := r.EndDrawTrace()
		if pix.Pix[0] != tc.want {
			t.Fatalf("background %#x -> %#x, want %#x", tc.background, pix.Pix[0], tc.want)
		}
		if counts["image:type8"] != 1 || counts["image:op7"] != 1 {
			t.Fatalf("missing particle trace: %v", counts)
		}
	}
}

// A short fade can fall entirely between 60-frame menu samples. Exercise every
// actual renderer fade frame here, without pretending this is a GUI transition.
func TestMenuFadeFramesAndTrace(t *testing.T) {
	for _, fadeIn := range []bool{true, false} {
		r := NewRender(nil)
		r.SetData(newRenderData(2, 2))
		pix := noximage.NewImage16(image.Rect(0, 0, 2, 2))
		r.SetPixBuffer(pix)
		done := 0
		callback := func() { done++ }
		want := []uint16{0x7fff, 0x4210, 0}
		if fadeIn {
			r.FadeInScreen(2, true, callback)
		} else {
			r.FadeOutScreen(2, true, callback)
			want = []uint16{0, 0x4210, 0x7fff}
		}
		pix.Pix[0] = 0x7fff
		r.BeginDrawTrace()
		r.DrawFade(false)
		if counts := r.EndDrawTrace(); len(counts) != 0 || pix.Pix[0] != 0x7fff || done != 0 {
			t.Fatal("menu fade advanced on world draw")
		}
		for i, expected := range want {
			// The game draws a fresh scene before applying its fade each frame.
			for j := range pix.Pix {
				pix.Pix[j] = 0x7fff
			}
			r.BeginDrawTrace()
			r.DrawFade(true)
			counts := r.EndDrawTrace()
			if pix.Pix[0] != expected {
				t.Fatalf("fadeIn=%v frame=%d got=%#x want=%#x", fadeIn, i, pix.Pix[0], expected)
			}
			if counts["buffer:drawFadeScreen"] != 1 {
				t.Fatalf("missing fade trace: %v", counts)
			}
			if i < 2 && done != 0 {
				t.Fatal("completion callback ran early")
			}
		}
		r.BeginDrawTrace()
		r.DrawFade(true)
		if counts := r.EndDrawTrace(); len(counts) != 0 || done != 1 {
			t.Fatal("completed fade repeated drawing/callback")
		}
	}
}
