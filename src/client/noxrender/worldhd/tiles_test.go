package worldhd

import (
	"encoding/binary"
	"image/color"
	"testing"
)

func TestTilesRingAndEdges(t *testing.T) {
	s, src, files := fixture(t)
	set, e := LoadFloors(files, []FloorSpec{s}, func(int) (FloorSource, error) { return src, nil })
	if e != nil {
		t.Fatal(e)
	}
	f := set.Lookup(s.ID, 0, s.SourceSHA256)
	f.pixels.SetNRGBA(41, 41, color.NRGBA{R: 200, G: 128, B: 64, A: 255})
	raw := make([]byte, 2116)
	for i := 0; i < len(raw); i += 2 {
		binary.LittleEndian.PutUint16(raw[i:], 0x1234)
	}
	ring := NewTiles(64, 64)
	anchor := 4090
	if !ring.Base(anchor, raw, f) {
		t.Fatal("base rejected")
	}
	index := anchor + 20*64 + 20
	v, detail := ring.Pixel(index, 0)
	if !detail || v[0] == v[3] {
		t.Fatal("HD detail lost at wrapped anchor")
	}
	if !ring.Edge(anchor, raw, []byte{0, 0, 1, 1}, nil) {
		t.Fatal("keep rejected")
	}
	_, detail = ring.Pixel(anchor+23, 0)
	if !detail {
		t.Fatal("keep erased base detail")
	}
	if !ring.Edge(anchor, raw, []byte{0, 0, 2, 1, 0x56, 0x34}, nil) {
		t.Fatal("opaque edge rejected")
	}
	v, detail = ring.Pixel(anchor+23, 0)
	if detail || v != [4]uint16{0x3456, 0x3456, 0x3456, 0x3456} {
		t.Fatal("edge did not overwrite HD")
	}
	if !ring.Edge(anchor, raw, []byte{0, 0, 3, 1}, f) {
		t.Fatal("underlay rejected")
	}
	_, detail = ring.Pixel(anchor+23, 0)
	if !detail {
		t.Fatal("underlay lost source HD")
	}
	ring.Opaque(anchor+23, []uint16{0x4567})
	v, detail = ring.Pixel(anchor+23, 0)
	if detail || v != [4]uint16{0x4567, 0x4567, 0x4567, 0x4567} {
		t.Fatal("ground decoration retained HD floor")
	}
	if !ring.Base(anchor, raw, nil) {
		t.Fatal("original redraw rejected")
	}
	v, detail = ring.Pixel(index, 0)
	if detail || v[0] != 0x1234 {
		t.Fatal("original redraw retained stale HD")
	}
	if ring.Edge(anchor, raw, []byte{0, 0, 2, 1, 0x56}, nil) || ring.Ready() {
		t.Fatal("truncated stream accepted")
	}
	v, detail = ring.Pixel(index, 0x2222)
	if detail || v != [4]uint16{0x2222, 0x2222, 0x2222, 0x2222} {
		t.Fatal("invalid buffer exposed stale data")
	}
}

func TestTilesDiamondAndLight(t *testing.T) {
	count := 0
	for y := 0; y < 46; y++ {
		s, n, off := tileRow(y)
		if off != count || s < 1 || s+n > 46 {
			t.Fatal(y, s, n, off)
		}
		count += n
	}
	if count != 1058 {
		t.Fatal(count)
	}
	for _, tc := range []struct {
		c       uint16
		r, g, b int
		want    uint16
	}{{0x7fff, 65536, 65536, 65536, 0x7fff}, {0x7fff, 0, 0, 0, 0}, {0x7fff, 65536, 0, 0, 0x7c00}, {0x7fff, 0, 65536, 0, 0x3e0}, {0x7fff, 0, 0, 65536, 31}} {
		if got := Light(tc.c, tc.r, tc.g, tc.b); got != tc.want {
			t.Fatalf("light got %x want %x", got, tc.want)
		}
	}
}
