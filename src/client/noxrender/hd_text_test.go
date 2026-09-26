package noxrender

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noxfont"
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

// hdGlyphLegacy hides the RGBA64 methods, so DrawMask takes the generic
// At/Set path that hdGlyph used before.
type hdGlyphLegacy struct{ p *hdGlyphPhase }

func (l hdGlyphLegacy) ColorModel() color.Model     { return l.p.ColorModel() }
func (l hdGlyphLegacy) Bounds() image.Rectangle     { return l.p.Bounds() }
func (l hdGlyphLegacy) At(x, y int) color.Color     { return l.p.At(x, y) }
func (l hdGlyphLegacy) Set(x, y int, c color.Color) { l.p.Set(x, y, c) }

// Catches any output change from the allocation-free path: every 16-bit
// destination value under 1-bit and 8-bit masks, opaque and translucent text.
func TestHDGlyphRGBA64MatchesGenericDrawMask(t *testing.T) {
	const n, scale = 256, 3
	phase := image.Pt(2, 1)
	bm := &noxfont.Bitmap{Pix: make([]byte, n*n/8), Stride: n / 8, Rect: image.Rect(3, 5, 3+n, 5+n)}
	for i := range bm.Pix {
		bm.Pix[i] = byte(i*37 + i>>5)
	}
	alpha := image.NewAlpha(image.Rect(0, 0, n, n))
	for i := range alpha.Pix {
		alpha.Pix[i] = byte(i + i>>8)
	}
	newDst := func() *noximage.Image16 {
		pix := noximage.NewImage16(image.Rect(0, 0, n*scale, n*scale))
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				pix.Pix[pix.PixOffset(x*scale+phase.X, y*scale+phase.Y)] = uint16(y*n + x)
			}
		}
		return pix
	}
	srcs := []color.Color{color.White, color.NRGBA{R: 200, G: 100, B: 50, A: 255}, color.NRGBA{R: 10, G: 220, B: 130, A: 90}}
	for si, src := range srcs {
		for _, m := range []struct {
			name     string
			old, cur image.Image
			maskp    image.Point
		}{
			{"bitmap", bm, &hdGlyphBitmap{Bitmap: bm}, bm.Rect.Min},
			{"alpha", alpha, alpha, alpha.Rect.Min},
		} {
			want, got := newDst(), newDst()
			dr := image.Rect(0, 0, n, n)
			oldView := &hdGlyphPhase{pix: want, clip: dr, scale: scale, phase: phase}
			newView := &hdGlyphPhase{pix: got, clip: dr, scale: scale, phase: phase}
			draw.DrawMask(hdGlyphLegacy{oldView}, dr, image.NewUniform(src), image.Point{}, m.old, m.maskp, draw.Over)
			draw.DrawMask(newView, dr, image.NewUniform(src), image.Point{}, m.cur, m.maskp, draw.Over)
			if !reflect.DeepEqual(want.Pix, got.Pix) {
				t.Fatalf("src %d mask %s: output differs from generic DrawMask", si, m.name)
			}
		}
	}
}

// Catches a regression to per-pixel color boxing in HD bitmap-font text.
func TestHDGlyphBitmapAllocations(t *testing.T) {
	r := hdTestRender(16, 16)
	r.p.SetTextColor(color.White)
	r.text.Src = image.NewUniform(color.White)
	if !r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 48, 48))) {
		t.Fatal("start failed")
	}
	bm := &noxfont.Bitmap{Pix: make([]byte, 2*16), Stride: 2, Rect: image.Rect(0, 0, 16, 16)}
	for i := range bm.Pix {
		bm.Pix[i] = 0xff
	}
	allocs := testing.AllocsPerRun(20, func() {
		r.hdGlyph(bm.Rect, bm, image.Point{})
	})
	if allocs > 2 {
		t.Fatalf("hdGlyph allocs=%v for 16x16 glyph at 3x", allocs)
	}
}

// 4.4-004 B1: world-frame glyphs use Scale2x sub-samples; with the option off
// or in a menu frame every logical bit is replicated as before, and the
// logical output is identical in all cases.
func TestHDGlyphScale2x(t *testing.T) {
	const w, h = 6, 6
	grid := [h]string{"#.....", ".#....", "..##..", "..##..", "....#.", "#....#"}
	bm := &noxfont.Bitmap{Pix: make([]byte, h), Stride: 1, Rect: image.Rect(0, 0, w, h)}
	on := func(x, y int) bool { return x >= 0 && y >= 0 && x < w && y < h && grid[y][x] == '#' }
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if on(x, y) {
				bm.Pix[y] |= 0x80 >> x
			}
		}
	}
	epx := func(x, y int) [4]bool {
		p, a, b, c, d := on(x, y), on(x, y-1), on(x+1, y), on(x-1, y), on(x, y+1)
		e := [4]bool{p, p, p, p}
		if c == a && c != d && a != b {
			e[0] = a
		}
		if a == b && a != c && b != d {
			e[1] = b
		}
		if d == c && d != b && c != a {
			e[2] = c
		}
		if b == d && b != a && d != c {
			e[3] = d
		}
		return e
	}
	draw1 := func(smooth, world bool) *noximage.Image16 {
		r := hdTestRender(w, h)
		r.p.SetTextColor(color.White)
		if !r.BeginWorldHDFrame() {
			t.Fatal("world HD frame not opened")
		}
		r.hd.world = world
		r.SetHDGlyphSmoothing(smooth)
		r.text.Src = image.NewUniform(color.White)
		r.hdGlyph(bm.Rect, bm, bm.Rect.Min)
		return r.hd.pix
	}
	white := func(pix *noximage.Image16, x, y int) bool { return pix.Pix[pix.PixOffset(x, y)] != 0 }
	changed := 0
	for _, c := range []struct{ smooth, world bool }{{true, true}, {false, true}, {true, false}} {
		pix := draw1(c.smooth, c.world)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				want := [4]bool{on(x, y), on(x, y), on(x, y), on(x, y)}
				if c.smooth && c.world {
					want = epx(x, y)
				}
				for k, v := range want {
					if got := white(pix, 2*x+k%2, 2*y+k/2); got != v {
						t.Fatalf("smooth=%t world=%t: sample %d,%d = %t, want %t", c.smooth, c.world, 2*x+k%2, 2*y+k/2, got, v)
					}
					if c.smooth && c.world && v != on(x, y) {
						changed++
					}
				}
			}
		}
	}
	if changed == 0 {
		t.Fatal("test glyph has no diagonal to smooth")
	}
	// At/RGBA64At agree so generic and fast DrawMask paths match.
	m := &hdGlyphBitmap{Bitmap: bm, epx: true}
	for _, ph := range []image.Point{{}, {X: 1}, {Y: 1}, {X: 1, Y: 1}} {
		m.phase = ph
		for y := -1; y <= h; y++ {
			for x := -1; x <= w; x++ {
				_, _, _, a := m.At(x, y).RGBA()
				if (a != 0) != (m.RGBA64At(x, y).A != 0) {
					t.Fatalf("At and RGBA64At differ at %d,%d phase %v", x, y, ph)
				}
			}
		}
	}
}
