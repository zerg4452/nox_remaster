package noxrender

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

// Catches missing mirrored writes, incorrect clipping, and opaque black loss.
func TestHDPrimitiveWrites(t *testing.T) {
	for _, name := range []string{"point", "rect", "clear", "fullrect"} {
		t.Run(name, func(t *testing.T) {
			for _, scale := range []int{1, 3} {
				r := hdTestRender(2, 2)
				bg := noximage.NewImage16(image.Rect(0, 0, 2*scale, 2*scale))
				for i := range bg.Pix {
					bg.Pix[i] = 0x7fff
				}
				for i := range r.pix.Pix {
					r.pix.Pix[i] = 0x7fff
				}
				if !r.BeginHDFrame(bg) {
					t.Fatal("start failed")
				}
				want := []uint16{0x7fff, 0, 0x7fff, 0x7fff}
				switch name {
				case "point":
					r.Data().SetClip(true)
					r.Data().SetClipRect2(image.Rect(1, 0, 2, 1))
					r.DrawPixel(image.Pt(0, 0), color.Black)
					r.DrawPixel(image.Pt(1, 0), color.Black)
				case "rect":
					r.Data().SetClip(true)
					r.Data().SetClipRect(image.Rect(1, 0, 2, 1))
					r.DrawRectFilledOpaque(-1, -1, 4, 4, color.Black)
				case "clear":
					r.ClearScreen(color.Black)
					want = []uint16{0, 0, 0, 0}
				case "fullrect":
					r.DrawRectFilledOpaque(0, 0, 2, 2, color.Black)
					want = []uint16{0, 0, 0, 0}
				}
				out := r.EndHDFrame()
				if out == nil {
					t.Fatal("supported primitive rejected")
				}
				if !reflect.DeepEqual(r.pix.Pix, want) {
					t.Fatalf("logical=%v want=%v", r.pix.Pix, want)
				}
				for y := 0; y < 2*scale; y++ {
					for x := 0; x < 2*scale; x++ {
						if got := out.Pix[out.PixOffset(x, y)]; got != want[(y/scale)*2+x/scale] {
							t.Fatalf("%s scale=%d (%d,%d)=%#x", name, scale, x, y, got)
						}
					}
				}
			}
		})
	}
}

func TestHDAlphaRectPreservesSubpixels(t *testing.T) {
	for _, rc := range []image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(0, 0, 2, 2), image.Rect(-1, -1, 3, 3)} {
		r := hdTestRender(2, 2)
		bg := noximage.NewImage16(image.Rect(0, 0, 6, 6))
		for y := 0; y < 6; y++ {
			copy(bg.Row(y), []uint16{0, 0x001f, 0x7c00, 0, 0x001f, 0x7c00})
		}
		if !r.BeginHDFrame(bg) {
			t.Fatal("start failed")
		}
		r.Data().SetAlphaEnabled(true)
		r.Data().SetClip(true)
		r.DrawRectFilledOpaque(rc.Min.X, rc.Min.Y, rc.Dx(), rc.Dy(), color.White)
		out := r.EndHDFrame()
		if out == nil {
			t.Fatal("supported alpha rectangle rejected")
		}
		for y := 0; y < 6; y++ {
			for x := 0; x < 6; x++ {
				want := bg.Pix[bg.PixOffset(x, y)]
				if rc.Intersect(r.pix.Rect) == r.pix.Rect {
					want = 0x7fff // Original full-screen fast path ignores alpha.
				} else if x < 3 && y < 3 {
					want = []uint16{0x3def, 0x3dff, 0x7def}[x]
				}
				if got := out.Pix[out.PixOffset(x, y)]; got != want {
					t.Fatalf("rect=%v (%d,%d)=%#x want=%#x", rc, x, y, got, want)
				}
			}
		}
	}
}

func TestHDAxisLines(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		r := hdTestRender(3, 3)
		r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 9, 9)))
		if vertical {
			r.drawLineVertical(1, 2, 0, color.White)
		} else {
			r.DrawLineHorizontal(2, 1, 0, color.White)
		}
		out := r.EndHDFrame()
		if out == nil {
			t.Fatal("axis line rejected")
		}
		for y := 0; y < 9; y++ {
			for x := 0; x < 9; x++ {
				want := uint16(0)
				if (vertical && x/3 == 1) || (!vertical && y/3 == 1) {
					want = 0x7fff
				}
				if out.Pix[out.PixOffset(x, y)] != want {
					t.Fatalf("axis pixel (%d,%d)", x, y)
				}
			}
		}
	}
}

// Catches applying the logical fade result over the detailed background and
// advancing the fade twice when rendering an additional target.
func TestHDFadePreservesSubpixelsAndCallback(t *testing.T) {
	r := hdTestRender(1, 1)
	bg := noximage.NewImage16(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		copy(bg.Row(y), []uint16{0x7fff, 0x001f, 0x7c00})
	}
	done := 0
	r.FadeInScreen(2, true, func() { done++ })
	for i, want := range [][]uint16{{0x7fff, 0x001f, 0x7c00}, {0x4210, 0x0010, 0x4000}, {0, 0, 0}} {
		r.pix.Pix[0] = 0x7fff
		if !r.BeginHDFrame(bg) {
			t.Fatal("start failed")
		}
		r.DrawFade(false)
		r.DrawFade(true)
		out := r.EndHDFrame()
		if out == nil {
			t.Fatal("fade invalidated frame")
		}
		for y := 0; y < 3; y++ {
			if !reflect.DeepEqual(out.Row(y), want) {
				t.Fatalf("frame=%d got=%v want=%v", i, out.Row(y), want)
			}
		}
		if r.pix.Pix[0] != want[0] {
			t.Fatal("logical fade changed")
		}
		if i < 2 && done != 0 {
			t.Fatal("callback ran early")
		}
	}
	r.DrawFade(true)
	if done != 1 {
		t.Fatalf("callback count=%d", done)
	}
}
