package noxrender

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/noxworld-dev/opennox-lib/noxfont"
	"github.com/noxworld-dev/opennox-lib/noximage"
)

// hdGlyph mirrors the mask returned to the existing font.Drawer. Each phase
// blends against its own destination through the same standard DrawMask path.
// The phase view and bitmap mask implement the RGBA64 interfaces, so DrawMask
// applies the same Over formula without boxing a color per pixel.
func (r *NoxRender) hdGlyph(dr image.Rectangle, mask image.Image, maskp image.Point) {
	clip := r.p.ClipRect().Intersect(r.pix.Bounds())
	if dr.Intersect(clip).Empty() {
		return
	}
	view := hdGlyphPhase{pix: r.hd.pix, clip: clip, scale: r.hd.scale}
	if bm, ok := mask.(*noxfont.Bitmap); ok {
		view.bitmap.Bitmap = bm
		view.bitmap.epx = r.hd.glyphSmooth && r.hd.world && view.scale == 2
		mask = &view.bitmap
	}
	for y := 0; y < view.scale; y++ {
		for x := 0; x < view.scale; x++ {
			view.phase = image.Pt(x, y)
			view.bitmap.phase = view.phase
			// Keep dr and maskp paired; DrawMask adjusts both for clipping.
			draw.DrawMask(&view, dr, r.text.Src, image.Point{}, mask, maskp, draw.Over)
		}
	}
}

type hdGlyphPhase struct {
	pix    *noximage.Image16
	clip   image.Rectangle
	scale  int
	phase  image.Point
	bitmap hdGlyphBitmap
	out    color.RGBA64
}

func (p *hdGlyphPhase) ColorModel() color.Model { return p.pix.ColorModel() }
func (p *hdGlyphPhase) Bounds() image.Rectangle { return p.clip }
func (p *hdGlyphPhase) At(x, y int) color.Color {
	return p.pix.At(x*p.scale+p.phase.X, y*p.scale+p.phase.Y)
}
func (p *hdGlyphPhase) Set(x, y int, c color.Color) {
	p.pix.Set(x*p.scale+p.phase.X, y*p.scale+p.phase.Y, c)
}

// RGBA64At returns the same values as At(x, y).RGBA().
func (p *hdGlyphPhase) RGBA64At(x, y int) color.RGBA64 {
	r, g, b, a := p.pix.NRGBAAt(x*p.scale+p.phase.X, y*p.scale+p.phase.Y).RGBA()
	return color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(a)}
}

// SetRGBA64 converts through Image16.Set exactly like Set(x, y, &c); the
// pointer targets the view itself, which already lives on the heap.
func (p *hdGlyphPhase) SetRGBA64(x, y int, c color.RGBA64) {
	p.out = c
	p.pix.Set(x*p.scale+p.phase.X, y*p.scale+p.phase.Y, &p.out)
}

// hdGlyphBitmap adds RGBA64At to a 1-bit font glyph; it reads the same bit as
// Bitmap.At (which boxes color.Opaque per call) and returns its RGBA values.
// With epx set it returns the Scale2x (EPX) sub-sample of the drawing phase,
// which only smooths diagonal steps and never moves the glyph outline by more
// than half a logical pixel.
type hdGlyphBitmap struct {
	*noxfont.Bitmap
	epx   bool
	phase image.Point
}

func (m *hdGlyphBitmap) bit(x, y int) bool {
	if !(image.Point{X: x, Y: y}.In(m.Rect)) {
		return false
	}
	i, j := m.BitOffsets(x, y)
	return (m.Pix[i]>>j)&1 != 0
}

// At matches RGBA64At, so generic DrawMask callers see the same samples.
func (m *hdGlyphBitmap) At(x, y int) color.Color {
	if m.RGBA64At(x, y).A != 0 {
		return color.Opaque
	}
	return color.Transparent
}

func (m *hdGlyphBitmap) RGBA64At(x, y int) color.RGBA64 {
	on := m.bit(x, y)
	if m.epx {
		a, b, c, d := m.bit(x, y-1), m.bit(x+1, y), m.bit(x-1, y), m.bit(x, y+1)
		switch m.phase {
		case image.Point{}:
			if c == a && c != d && a != b {
				on = a
			}
		case image.Point{X: 1}:
			if a == b && a != c && b != d {
				on = b
			}
		case image.Point{Y: 1}:
			if d == c && d != b && c != a {
				on = c
			}
		default:
			if b == d && b != a && d != c {
				on = d
			}
		}
	}
	if !on {
		return color.RGBA64{}
	}
	return color.RGBA64{R: 0xffff, G: 0xffff, B: 0xffff, A: 0xffff}
}
