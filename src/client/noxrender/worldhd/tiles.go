package worldhd

import "encoding/binary"

// Tiles parallels the legacy linear ring buffer: each logical pixel owns four
// density-2 samples. It never aliases or writes the original tile buffer.
type Tiles struct {
	Width, Height int
	pix           [][4]uint16
	detail        []bool
	valid         []bool
	ok            bool
}

func NewTiles(w, h int) *Tiles {
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil
	}
	return &Tiles{Width: w, Height: h, pix: make([][4]uint16, w*h), detail: make([]bool, w*h), valid: make([]bool, w*h), ok: true}
}
func (t *Tiles) Invalidate() {
	if t != nil {
		t.ok = false
	}
}
func (t *Tiles) Ready() bool { return t != nil && t.ok }

// Opaque mirrors the original ground-decoration writer after its ring clipping.
func (t *Tiles) Opaque(index int, src []uint16) {
	if !t.Ready() {
		return
	}
	for j, p := range src {
		i := t.index(index + j)
		t.pix[i] = [4]uint16{p, p, p, p}
		t.detail[i] = false
		t.valid[i] = true
	}
}
func (t *Tiles) index(i int) int {
	// The draw loop usually supplies an index already inside the ring.
	// Keep modulo normalization for wrapped and negative writer coordinates.
	if uint(i) < uint(len(t.pix)) {
		return i
	}
	i %= len(t.pix)
	if i < 0 {
		i += len(t.pix)
	}
	return i
}

func tileRow(y int) (start, n, offset int) {
	for j := 0; j < y; j++ {
		s := 23 - j
		if j >= 23 {
			s = j - 22
		}
		offset += 47 - 2*s
	}
	start = 23 - y
	if y >= 23 {
		start = y - 22
	}
	n = 47 - 2*start
	return
}

func tileSamples(raw []byte, offset, x, y int, hd *Floor) (v [4]uint16, detail bool) {
	if hd != nil {
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				c := hd.NRGBAAt(x*2+dx, y*2+dy)
				v[dy*2+dx] = uint16(c.R&248)<<7 | uint16(c.G&248)<<2 | uint16(c.B)>>3
			}
		}
		return v, true
	}
	p := binary.LittleEndian.Uint16(raw[2*offset:])
	return [4]uint16{p, p, p, p}, false
}

// Base mirrors one complete packed type-0 diamond at its linear-ring anchor.
func (t *Tiles) Base(anchor int, raw []byte, hd *Floor) bool {
	if !t.Ready() {
		return false
	}
	if len(raw) != 2116 {
		t.Invalidate()
		return false
	}
	for y := 0; y < 46; y++ {
		start, n, off := tileRow(y)
		for x := start; x < start+n; x++ {
			i := t.index(anchor + y*t.Width + x)
			t.pix[i], t.detail[i] = tileSamples(raw, off+x-start, x, y, hd)
			t.valid[i] = true
		}
	}
	return true
}

// Edge mirrors packed edge ops in original order: 1 keep destination,
// 2 edge-owned opaque pixels, 3 source floor (possibly HD). Validate first so
// a malformed stream cannot leave a partially accepted ring update.
func (t *Tiles) Edge(anchor int, raw, edge []byte, hd *Floor) bool {
	if !t.Ready() {
		return false
	}
	if len(raw) != 2116 || len(edge) < 2 || edge[0] > edge[1] || edge[1] >= 46 {
		t.Invalidate()
		return false
	}
	for pass := 0; pass < 2; pass++ {
		pos := 2
		for y := int(edge[0]); y <= int(edge[1]); y++ {
			start, n, off := tileRow(y)
			for x := 0; x < n; {
				if pos+2 > len(edge) {
					t.Invalidate()
					return false
				}
				op, count := edge[pos], int(edge[pos+1])
				pos += 2
				if count == 0 || count > n-x || op < 1 || op > 3 || (op == 2 && pos+2*count > len(edge)) {
					t.Invalidate()
					return false
				}
				if pass == 1 && op != 1 {
					for j := 0; j < count; j++ {
						i := t.index(anchor + y*t.Width + start + x + j)
						if op == 3 {
							t.pix[i], t.detail[i] = tileSamples(raw, off+x+j, start+x+j, y, hd)
						} else {
							p := binary.LittleEndian.Uint16(edge[pos+2*j:])
							t.pix[i] = [4]uint16{p, p, p, p}
							t.detail[i] = false
						}
						t.valid[i] = true
					}
				}
				if op == 2 {
					pos += 2 * count
				}
				x += count
			}
		}
		if pos != len(edge) {
			t.Invalidate()
			return false
		}
	}
	return true
}

// Pixel returns original for a ring cell not yet initialized, never old HD.
func (t *Tiles) Pixel(index int, original uint16) (v [4]uint16, detail bool) {
	if !t.Ready() {
		return [4]uint16{original, original, original, original}, false
	}
	i := t.index(index)
	if !t.valid[i] {
		return [4]uint16{original, original, original, original}, false
	}
	return t.pix[i], t.detail[i]
}

// Light uses the legacy RGB555 fixed-point multipliers at the logical pixel.
// It does not step animation or resample the logical light grid at HD density.
func Light(c uint16, r, g, b int) uint16 {
	// Select product bits 16..20 directly into their RGB555 positions.
	// This also preserves the legacy uint32 overflow and truncation behavior.
	rr := (uint32(r) * uint32((c>>10)&31) >> 6) & 0x7c00
	gg := (uint32(g) * uint32((c>>5)&31) >> 11) & 0x03e0
	bb := (uint32(b) * uint32(c&31) >> 16) & 0x001f
	return uint16(rr | gg | bb)
}
