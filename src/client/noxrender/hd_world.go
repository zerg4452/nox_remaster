package noxrender

import (
	"encoding/binary"
	"image"
	"unsafe"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

// BeginWorldHDFrame is a separate density-2 contract; menu 1/3x eligibility is
// unchanged. Logical pixels are only read and all unhandled writes still veto HD.
func (r *NoxRender) BeginWorldHDFrame() bool {
	censusFrame()
	r.InvalidateHDFrame()
	r.hd.world = true
	r.hd.detail = 0
	r.hd.reason = ""
	r.hd.wall, r.hd.wallSpan = nil, false
	if r.pix == nil || r.pix.Rect.Min != (image.Point{}) || r.pix.Rect.Empty() {
		return false
	}
	rect := image.Rectangle{Max: r.pix.Rect.Max.Mul(2)}
	r.hd.worldPix = resizeHDTarget(r.hd.worldPix, rect)
	r.hd.pix = r.hd.worldPix
	r.hd.scale = 2
	for y := 0; y < r.pix.Rect.Dy(); y++ {
		row := r.hd.pix.Row(y * 2)[:rect.Dx()]
		for x, c := range r.pix.Row(y)[:r.pix.Rect.Dx()] {
			row[x*2] = c
			row[x*2+1] = c
		}
		copy(r.hd.pix.Row(y*2+1), row)
	}
	r.hd.active = true
	return true
}

func (r *NoxRender) WorldHDStatus() (bool, int, string) {
	return r.hd.world && r.hd.active, r.hd.detail, r.hd.reason
}
func (r *NoxRender) RejectWorldHD(reason string) {
	if r.hd.world {
		if r.hd.reason == "" {
			r.hd.reason = reason
		}
		r.InvalidateHDFrame()
	}
}

// WorldFloorBuffer is restricted to the audited floor writer, which mirrors
// every written span. General callers must continue using PixBuffer's veto.
func (r *NoxRender) WorldFloorBuffer() *noximage.Image16 { return r.pix }

// Audited world primitive operations mirror every write below. Preserve the
// menu's stricter unsupported-primitive behavior.
func (r *NoxRender) worldPrimitiveBuffer() *noximage.Image16 {
	if r.hd.world {
		return r.hdPixBuffer()
	}
	return r.PixBuffer()
}
func (r *NoxRender) worldPrimitivePixel(x, y int, op func(uint16) uint16) {
	if r.hd.world {
		r.hdRect(image.Rect(x, y, x+1, y+1), op)
	}
}

func (r *NoxRender) worldSpanPosition(dst []uint16) (image.Point, bool) {
	if !r.hd.world || !r.hd.active || len(dst) == 0 {
		return image.Point{}, false
	}
	base := uintptr(unsafe.Pointer(&r.pix.Pix[0]))
	ptr := uintptr(unsafe.Pointer(&dst[0]))
	if ptr < base || (ptr-base)%2 != 0 || (ptr-base)/2 >= uintptr(len(r.pix.Pix)) {
		r.RejectWorldHD("destination outside logical buffer")
		return image.Point{}, false
	}
	i := int((ptr - base) / 2)
	p := image.Pt(i%r.pix.Stride, i/r.pix.Stride)
	if !p.In(r.pix.Rect) || len(dst) > r.pix.Rect.Max.X-p.X {
		r.RejectWorldHD("destination crosses logical row")
		return image.Point{}, false
	}
	return p, true
}

// WorldFloorSpan validates one already-clipped logical row span and then writes
// its density-2 samples without rediscovering the destination for every pixel.
type WorldFloorSpan struct {
	r      *NoxRender
	top    []uint16
	bottom []uint16
}

func (r *NoxRender) BeginWorldFloorSpan(dst []uint16) WorldFloorSpan {
	p, ok := r.worldSpanPosition(dst)
	if !ok {
		return WorldFloorSpan{}
	}
	i := r.hd.pix.PixOffset(p.X*2, p.Y*2)
	n := len(dst) * 2
	return WorldFloorSpan{
		r: r, top: r.hd.pix.Pix[i : i+n],
		bottom: r.hd.pix.Pix[i+r.hd.pix.Stride : i+r.hd.pix.Stride+n],
	}
}

func (s WorldFloorSpan) Set(x int, samples [4]uint16, detail bool) {
	if s.r == nil || !s.r.hd.active {
		return
	}
	if x < 0 || x >= len(s.top)/2 {
		s.r.RejectWorldHD("floor span index outside logical buffer")
		return
	}
	i := x * 2
	s.top[i], s.top[i+1] = samples[0], samples[1]
	s.bottom[i], s.bottom[i+1] = samples[2], samples[3]
	if detail {
		s.r.hd.detail++
	}
}

// Rows lets the audited floor writer fill a validated span without per-pixel
// checks. Index x maps to top/bottom[2x:2x+2], as in Set. ok is false for a
// rejected span or inactive HD frame; report detail samples with AddDetail.
func (s WorldFloorSpan) Rows() (top, bottom []uint16, ok bool) {
	if s.r == nil || !s.r.hd.active {
		return nil, nil, false
	}
	return s.top, s.bottom, true
}

func (s WorldFloorSpan) AddDetail(n int) {
	if s.r != nil && s.r.hd.active {
		s.r.hd.detail += n
	}
}

func (r *NoxRender) WorldFloorPixel(dst []uint16, samples [4]uint16, detail bool) {
	p, ok := r.worldSpanPosition(dst)
	if !ok {
		return
	}
	i := r.hd.pix.PixOffset(p.X*2, p.Y*2)
	row := r.hd.pix.Pix[i : i+2]
	row[0], row[1] = samples[0], samples[1]
	i += r.hd.pix.Stride
	row = r.hd.pix.Pix[i : i+2]
	row[0], row[1] = samples[2], samples[3]
	if detail {
		r.hd.detail++
	}
}

// BeginWorldWall selects the HD asset of the lit opaque wall image h that
// nox_xxx_edgeDraw_480EF0 is about to draw (4.3-001c). Like sprites, the asset
// only applies when the bag record itself (no override data) is drawn.
func (r *NoxRender) BeginWorldWall(h ImageHandle) {
	r.hd.wall, r.hd.wallSpan = nil, false
	if !r.hd.active || !r.hd.world {
		return
	}
	im := r.Bag.AsImage(h)
	if im == nil || im.override != nil {
		return
	}
	if pix := r.Bag.WorldHDAsset(im); pix != nil {
		if data := im.Pixdata(); len(data) >= 4 {
			r.hd.wall, r.hd.wallStride = pix, 2*int(binary.LittleEndian.Uint32(data))
		}
	}
}

// WorldWallSpan announces the image-local position of the next lit wall span.
func (r *NoxRender) WorldWallSpan(local image.Point) {
	r.hd.wallLocal, r.hd.wallSpan = local, r.hd.wall != nil
}

// worldWallRows consumes the announced span and returns its two asset rows and
// two HD destination rows (2*len(dst) samples each); ok=false keeps the 2x2 mirror.
func (r *NoxRender) worldWallRows(dst []uint16) (src, out [2][]uint16, ok bool) {
	if !r.hd.wallSpan {
		return src, out, false
	}
	r.hd.wallSpan = false
	s, stride, l, n := r.hd.wall, r.hd.wallStride, r.hd.wallLocal, len(dst)
	if l.X < 0 || l.Y < 0 || n == 0 || 2*(l.X+n) > stride || (2*l.Y+2)*stride > len(s) {
		return src, out, false
	}
	p, pok := r.worldSpanPosition(dst)
	if !pok {
		return src, out, false
	}
	for y := 0; y < 2; y++ {
		i := (2*l.Y+y)*stride + 2*l.X
		src[y] = s[i : i+2*n]
		out[y] = r.hd.pix.Row(2*p.Y + y)[2*p.X : 2*p.X+2*n]
	}
	return src, out, true
}

// LitWallSpan is the lit opaque wall span of nox_xxx_edgeDraw_480EF0
// (sub_480860): each pixel is scaled by a light that advances by step per
// pixel. With an announced wall asset both density-2 rows are lit with the
// same per-pixel light (4.3-001 D3); otherwise the span is mirrored 2x2.
// r may be nil (no client).
func (r *NoxRender) LitWallSpan(dst, src []uint16, light, step []uint32) {
	var hs, hd [2][]uint16
	asset := false
	if r != nil {
		hs, hd, asset = r.worldWallRows(dst)
	}
	for i := range dst {
		dst[i] = litWallPixel(src[i], light)
		if asset {
			for y := range hd {
				hd[y][2*i] = litWallPixel(hs[y][2*i], light)
				hd[y][2*i+1] = litWallPixel(hs[y][2*i+1], light)
			}
		}
		light[0] += step[0]
		light[1] += step[1]
		light[2] += step[2]
	}
	if r != nil && !asset {
		r.MirrorWorldOpaque(dst)
	}
}

func litWallPixel(v uint16, light []uint32) uint16 {
	c := SplitColor16(v)
	c.R = uint16((light[0] * uint32(c.R)) >> 16)
	c.G = uint16((light[1] * uint32(c.G)) >> 16)
	c.B = uint16((light[2] * uint32(c.B)) >> 16)
	return c.Make16()
}

// MirrorWorldOpaque is called only after an audited opaque wall span writes.
// Alpha operations must use the decoder's pure operation on each HD sample.
func (r *NoxRender) MirrorWorldOpaque(dst []uint16) {
	p, ok := r.worldSpanPosition(dst)
	if !ok {
		return
	}
	for x, c := range dst {
		for dy := 0; dy < 2; dy++ {
			i := r.hd.pix.PixOffset((p.X+x)*2, p.Y*2+dy)
			r.hd.pix.Pix[i] = c
			r.hd.pix.Pix[i+1] = c
		}
	}
}

// hdSpanIndexed is hdSpan for an indexed run (4.2-M6b): with an installed
// asset both HD rows take their shades from the asset's low bytes and keep the
// run's colour slot; otherwise, or if the run does not fit, it is the 2x path.
func (r *NoxRender) hdSpanIndexed(pos, local image.Point, src []byte, n int, op byte, fn drawOp8Func) {
	s, stride := r.hd.sprite, r.hd.spriteStride
	if s == nil || !r.hd.world || local.X < 0 || local.Y < 0 || n <= 0 || 2*(local.X+n) > stride || (2*local.Y+2)*stride > len(s) {
		r.hdImageSpanIndexed(pos, src, n, op, fn)
		return
	}
	if !r.hd.active {
		return
	}
	if fn == nil || !pos.In(r.pix.Rect) || n > r.pix.Rect.Max.X-pos.X {
		r.RejectWorldHD("invalid indexed span")
		return
	}
	if cap(r.hd.worldSpanSource) < 2*n {
		r.hd.worldSpanSource = make([]byte, 2*n)
	}
	shades := r.hd.worldSpanSource[:2*n]
	for y := 0; y < 2; y++ {
		i := (2*local.Y+y)*stride + 2*local.X
		for x, v := range s[i : i+2*n] {
			shades[x] = byte(v)
		}
		_, _ = fn(r.hd.pix.Row(pos.Y*2 + y)[pos.X*2:], shades, op, 2*n)
	}
}

func (r *NoxRender) hdImageSpanIndexed(pos image.Point, src []byte, n int, op byte, fn drawOp8Func) {
	if !r.hd.active || !r.hd.world || n <= 0 {
		return
	}
	if fn == nil || !pos.In(r.pix.Rect) || n > r.pix.Rect.Max.X-pos.X {
		r.RejectWorldHD("invalid indexed span")
		return
	}
	for y := 0; y < 2; y++ {
		row := r.hd.pix.Row(pos.Y*2 + y)
		for x := 0; x < n; x++ {
			for dx := 0; dx < 2; dx++ {
				_, _ = fn(row[(pos.X+x)*2+dx:], src[x:], op, 1)
			}
		}
	}
}
