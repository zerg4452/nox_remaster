package noxrender

import (
	"image"
	"unsafe"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

type hdTarget struct {
	world             bool
	detail            int
	reason            string
	pix               *noximage.Image16
	scale             int
	active            bool
	menuSprites       map[string]*noximage.Image16
	suppressImageSpan bool
	worldSpanSource   []byte
	// sprite is the installed density-2 asset of the image being drawn in a
	// world frame (4.2-M6), spriteStride its row length in samples.
	sprite       []uint16
	spriteStride int
	// menuPix and worldPix keep one target per mode so switching between the
	// menu and the world (every map load) reuses them instead of reallocating.
	menuPix, worldPix *noximage.Image16
}

// BeginHDFrame starts an optional operation target without replacing the logical
// buffer. Scale 1 is for equivalence checks; scale 3 is for menu diagnostics.
// The caller must invalidate frames containing unhandled writes, including C.
// The menu presentation path enables this only for a verified background draw.
func (r *NoxRender) BeginHDFrame(background *noximage.Image16) bool {
	r.InvalidateHDFrame()
	r.hd.world = false
	if r.pix == nil || background == nil || r.pix.Rect.Min != (image.Point{}) || background.Rect.Min != (image.Point{}) {
		return false
	}
	sz := r.pix.Size()
	if sz.X <= 0 || sz.Y <= 0 {
		return false
	}
	scale := 1
	if background.Size() != sz {
		scale = 3
		if background.Size() != sz.Mul(scale) {
			return false
		}
	}
	if r.hd.menuPix == nil || r.hd.menuPix.Rect != background.Rect {
		r.hd.menuPix = noximage.NewImage16(background.Rect)
	}
	r.hd.pix = r.hd.menuPix
	for y := 0; y < background.Rect.Dy(); y++ {
		copy(r.hd.pix.Row(y), background.Row(y))
	}
	r.hd.scale = scale
	r.hd.active = true
	return true
}

// EndHDFrame returns a borrowed frame, valid until the next BeginHDFrame call.
// An invalid or already ended frame has no HD output.
func (r *NoxRender) EndHDFrame() *noximage.Image16 {
	if !r.hd.active {
		return nil
	}
	r.hd.active = false
	return r.hd.pix
}

// ConsumeFullFrameImage consumes evidence from the immediately preceding draw.
func (r *NoxRender) ConsumeFullFrameImage() bool {
	out := r.lastFullImage
	r.lastFullImage = false
	return out
}

func (r *NoxRender) InvalidateHDFrame() {
	if r.hd.world && r.hd.active && r.hd.reason == "" {
		r.hd.reason = "unmirrored operation"
	}
	r.hd.active = false
}

// hdRect applies a pixel operation to the high-density area corresponding to
// an already clipped logical rectangle. It never runs a stateful draw callback.
func (r *NoxRender) hdRect(rc image.Rectangle, op func(uint16) uint16) {
	if !r.hd.active {
		return
	}
	rc = rc.Intersect(r.pix.Rect)
	s := r.hd.scale
	for y := rc.Min.Y * s; y < rc.Max.Y*s; y++ {
		row := r.hd.pix.Row(y)
		for x := rc.Min.X * s; x < rc.Max.X*s; x++ {
			row[x] = op(row[x])
		}
	}
}

// hdFillWorldRect mirrors an opaque constant without a callback per subpixel.
// Only the clipped HD destination is written; alpha operations use hdRect.
func (r *NoxRender) hdFillWorldRect(rc image.Rectangle, value uint16) {
	if !r.hd.active || !r.hd.world {
		return
	}
	rc = rc.Intersect(r.pix.Rect)
	if rc.Empty() {
		return
	}
	s := r.hd.scale
	x1, x2 := rc.Min.X*s, rc.Max.X*s
	y1, y2 := rc.Min.Y*s, rc.Max.Y*s
	first := r.hd.pix.Row(y1)[x1:x2]
	for i := range first {
		first[i] = value
	}
	for y := y1 + 1; y < y2; y++ {
		copy(r.hd.pix.Row(y)[x1:x2], first)
	}
}

// hdSpan draws one clipped 16-bit run whose first pixel is at image-local
// position local. With an installed world asset both density-2 rows come from
// it (same pure operation, asset samples instead of repeated originals);
// otherwise, or if the run does not fit the asset, it is the 2x path.
func (r *NoxRender) hdSpan(pos, local image.Point, src []byte, n int, fn drawOp16Func) {
	s, stride := r.hd.sprite, r.hd.spriteStride
	if s == nil || !r.hd.world || local.X < 0 || local.Y < 0 || n <= 0 || 2*(local.X+n) > stride || (2*local.Y+2)*stride > len(s) {
		r.hdImageSpan(pos, src, n, fn)
		return
	}
	if !r.hd.active {
		return
	}
	if fn == nil || !pos.In(r.pix.Rect) || n > r.pix.Rect.Max.X-pos.X {
		r.InvalidateHDFrame()
		return
	}
	if r.hd.suppressImageSpan {
		return
	}
	for y := 0; y < 2; y++ {
		i := (2*local.Y+y)*stride + 2*local.X
		row := s[i : i+2*n]
		// Samples are little-endian uint16, the byte layout the ops decode.
		b := unsafe.Slice((*byte)(unsafe.Pointer(&row[0])), 4*n)
		_, _ = fn(r.hd.pix.Row(pos.Y*2 + y)[pos.X*2:], b, 2*n)
	}
}

// hdImageSpan receives an already clipped decoder span. Reusing the selected
// pure pixel operation preserves legacy rounding without replaying rendering
// callbacks, decoder state, or the logical destination.
func (r *NoxRender) hdImageSpan(pos image.Point, src []byte, n int, fn drawOp16Func) {
	if !r.hd.active || n <= 0 {
		return
	}
	if fn == nil || !pos.In(r.pix.Rect) || n > r.pix.Rect.Max.X-pos.X {
		r.InvalidateHDFrame()
		return
	}
	if r.hd.suppressImageSpan {
		return
	}
	scale := r.hd.scale
	if r.hd.world {
		// Expand the read-only source once, then run the same pure operation on
		// each complete HD row. Every subpixel still blends with its own old
		// value; only repeated material/alpha setup and single-pixel calls go away.
		count := n * scale
		size := count * 2
		if cap(r.hd.worldSpanSource) < size {
			r.hd.worldSpanSource = make([]byte, size)
		}
		expanded := r.hd.worldSpanSource[:size]
		for x := 0; x < n; x++ {
			lo, hi := src[x*2], src[x*2+1]
			for dx := 0; dx < scale; dx++ {
				i := (x*scale + dx) * 2
				expanded[i], expanded[i+1] = lo, hi
			}
		}
		for y := 0; y < scale; y++ {
			row := r.hd.pix.Row(pos.Y*scale + y)
			_, _ = fn(row[pos.X*scale:], expanded, count)
		}
		return
	}
	for y := 0; y < scale; y++ {
		row := r.hd.pix.Row(pos.Y*scale + y)
		for x := 0; x < n; x++ {
			for dx := 0; dx < scale; dx++ {
				_, _ = fn(row[(pos.X+x)*scale+dx:], src[2*x:], 1)
			}
		}
	}
}
