package noxrender

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Only glyph data is artificial; layout and compositing use the real Drawer.
type hdTextFace struct {
	calls []rune
}

func TestHDTextReplacementGlyph(t *testing.T) {
	r := hdTestRender(16, 16)
	r.p.SetTextColor(color.White)
	r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 16, 16)))
	r.drawString(basicfont.Face7x13, "한", image.Point{})
	out := r.EndHDFrame()
	if out == nil || !reflect.DeepEqual(out.Pix, r.pix.Pix) {
		t.Fatal("replacement glyph missing from HD")
	}
}

func (*hdTextFace) Close() error                 { return nil }
func (*hdTextFace) Metrics() font.Metrics        { return font.Metrics{CapHeight: fixed.I(2)} }
func (*hdTextFace) Kern(a, b rune) fixed.Int26_6 { return -fixed.I(1) }
func (*hdTextFace) GlyphAdvance(c rune) (fixed.Int26_6, bool) {
	return fixed.I(4), c == 'A' || c == '한'
}
func (f *hdTextFace) GlyphBounds(c rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	a, ok := f.GlyphAdvance(c)
	return fixed.R(0, -2, 3, 0), a, ok
}
func (f *hdTextFace) Glyph(dot fixed.Point26_6, c rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	f.calls = append(f.calls, c)
	a, ok := f.GlyphAdvance(c)
	if !ok {
		return image.Rectangle{}, nil, image.Point{}, a, false
	}
	// Nonzero mask origin catches lost mask offsets after destination clipping.
	m := image.NewAlpha(image.Rect(5, 7, 8, 9))
	copy(m.Pix, []byte{0, 128, 255, 255, 64, 0})
	if c == '한' {
		copy(m.Pix, []byte{255, 128, 0, 0, 64, 255})
	}
	p := image.Pt(dot.X.Floor(), dot.Y.Floor()-2)
	return image.Rect(p.X, p.Y, p.X+3, p.Y+2), m, m.Rect.Min, a, true
}

// Catches missing mirroring, clip/mask drift, replicated logical colors and
// repeated layout/Glyph calls, including a Korean rune and nonzero kerning.
func TestHDTextMaskClipAdvanceSingleGlyph(t *testing.T) {
	for _, scale := range []int{1, 3} {
		for _, clip := range []image.Rectangle{
			image.Rect(0, 0, 8, 4), image.Rect(2, 1, 5, 2),
			image.Rect(-4, -4, 12, 9), image.Rect(9, 9, 10, 10),
		} {
			t.Run(fmt.Sprintf("%dx/%v", scale, clip), func(t *testing.T) {
				r := hdTestRender(8, 4)
				r.p.SetClipRect(clip)
				r.p.SetTextColor(color.White)
				r.text.advance = 1
				bg := noximage.NewImage16(image.Rect(0, 0, 8*scale, 4*scale))
				for y := 0; y < 4*scale; y++ {
					for x := 0; x < 8*scale; x++ {
						bg.Set(x, y, color.NRGBA{R: byte(x * 13), G: byte(y * 19), B: byte((x + y) * 7), A: 255})
					}
				}
				for y := 0; y < 4; y++ {
					for x := 0; x < 8; x++ {
						r.pix.Set(x, y, bg.At(x*scale, y*scale))
					}
				}
				want := noximage.NewImage16(bg.Rect)
				copy(want.Pix, bg.Pix)
				// Independent full-resolution masks: no production phase view or
				// second Face call is used to construct the expected pixels.
				for i, samples := range [][]byte{{0, 128, 255, 255, 64, 0}, {255, 128, 0, 0, 64, 255}} {
					m := image.NewAlpha(bg.Rect)
					for y := 0; y < 2*scale; y++ {
						for x := 0; x < 3*scale; x++ {
							m.SetAlpha((1+4*i)*scale+x, scale+y, color.Alpha{A: samples[(y/scale)*3+x/scale]})
						}
					}
					c := clip.Intersect(r.pix.Rect)
					c = image.Rectangle{Min: c.Min.Mul(scale), Max: c.Max.Mul(scale)}
					draw.DrawMask(want, c, image.White, image.Point{}, m, c.Min, draw.Over)
				}
				if !r.BeginHDFrame(bg) {
					t.Fatal("start failed")
				}
				face := new(hdTextFace)
				if got := r.drawString(face, "A한", image.Pt(1, 1)); got != 10 {
					t.Fatalf("advance=%d want=10", got)
				}
				if !reflect.DeepEqual(face.calls, []rune{'A', '한'}) {
					t.Fatalf("Glyph calls=%q", face.calls)
				}
				out := r.EndHDFrame()
				if out == nil {
					t.Fatal("text invalidated frame")
				}
				if !reflect.DeepEqual(out.Pix, want.Pix) {
					t.Fatal("HD mask/clip pixels differ from independent full-resolution DrawMask")
				}
				if scale == 1 && !reflect.DeepEqual(out.Pix, r.pix.Pix) {
					t.Fatal("1x differs from logical Drawer")
				}
			})
		}
	}
}

// Independent literal: half-white over opaque black quantizes to RGB555 16/16/16.
func TestHDTextHalfMaskLiteral(t *testing.T) {
	for _, scale := range []int{1, 3} {
		r := hdTestRender(3, 2)
		r.p.SetTextColor(color.White)
		if !r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 3*scale, 2*scale))) {
			t.Fatal("start failed")
		}
		r.drawString(new(hdTextFace), "A", image.Point{})
		out := r.EndHDFrame()
		if out == nil {
			t.Fatal("text invalidated frame")
		}
		for y := 0; y < scale; y++ {
			for x := scale; x < 2*scale; x++ {
				if got := out.Pix[out.PixOffset(x, y)]; got != 0x4210 {
					t.Fatalf("half mask=%#x want=0x4210", got)
				}
			}
		}
	}
}

// Catches measurement writes, a leaked drawing flag and default-off writes.
func TestHDTextMeasurementAndDisabled(t *testing.T) {
	r := hdTestRender(8, 4)
	r.p.SetTextColor(color.White)
	bg := noximage.NewImage16(image.Rect(0, 0, 24, 12))
	if !r.BeginHDFrame(bg) {
		t.Fatal("start failed")
	}
	face := new(hdTextFace)
	r.drawString(face, "A", image.Point{})
	before := append([]uint16(nil), r.hd.pix.Pix...)
	r.text.face.Glyph(fixed.P(4, 2), '한')
	r.GetStringSizeWrapped(face, "A한", 0)
	if !reflect.DeepEqual(before, r.hd.pix.Pix) {
		t.Fatal("measurement wrote HD pixels")
	}
	r.InvalidateHDFrame()
	r.drawString(face, "한", image.Pt(4, 0))
	if !reflect.DeepEqual(before, r.hd.pix.Pix) {
		t.Fatal("disabled drawing wrote HD pixels")
	}
	plain := hdTestRender(8, 4)
	plain.p.SetTextColor(color.White)
	if got := plain.drawString(new(hdTextFace), "A", image.Point{}); got != 4 {
		t.Fatalf("default-off advance=%d", got)
	}
	if plain.pix.Pix[2] != 0x7fff || plain.hd.pix != nil {
		t.Fatal("default-off logical text or HD allocation changed")
	}
}
