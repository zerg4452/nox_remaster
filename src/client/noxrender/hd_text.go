package noxrender

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

// hdGlyph mirrors the mask returned to the existing font.Drawer. Each phase
// blends against its own destination through the same standard DrawMask path.
func (r *NoxRender) hdGlyph(dr image.Rectangle, mask image.Image, maskp image.Point) {
	clip := r.p.ClipRect().Intersect(r.pix.Bounds())
	if dr.Intersect(clip).Empty() {
		return
	}
	view := hdGlyphPhase{pix: r.hd.pix, clip: clip, scale: r.hd.scale}
	for y := 0; y < view.scale; y++ {
		for x := 0; x < view.scale; x++ {
			view.phase = image.Pt(x, y)
			// Keep dr and maskp paired; DrawMask adjusts both for clipping.
			draw.DrawMask(&view, dr, r.text.Src, image.Point{}, mask, maskp, draw.Over)
		}
	}
}

type hdGlyphPhase struct {
	pix   *noximage.Image16
	clip  image.Rectangle
	scale int
	phase image.Point
}

func (p *hdGlyphPhase) ColorModel() color.Model { return p.pix.ColorModel() }
func (p *hdGlyphPhase) Bounds() image.Rectangle { return p.clip }
func (p *hdGlyphPhase) At(x, y int) color.Color {
	return p.pix.At(x*p.scale+p.phase.X, y*p.scale+p.phase.Y)
}
func (p *hdGlyphPhase) Set(x, y int, c color.Color) {
	p.pix.Set(x*p.scale+p.phase.X, y*p.scale+p.phase.Y, c)
}
