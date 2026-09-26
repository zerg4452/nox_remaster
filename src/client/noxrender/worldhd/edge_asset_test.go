package worldhd

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"

	noxcolor "github.com/noxworld-dev/opennox-lib/color"
)

// edgeReplica returns a 2x replica PNG image of a packed edge stream: keep
// alpha 0, underlying floor alpha 128, edge-owned colours opaque.
func edgeReplica(t *testing.T, raw []byte) *image.NRGBA {
	t.Helper()
	ops, err := edgeOps(raw)
	if err != nil {
		t.Fatal(err)
	}
	colors := edgeOwnColors(raw)
	im := image.NewNRGBA(image.Rect(0, 0, 92, 92))
	for i, op := range ops {
		var c color.NRGBA
		switch op {
		case EdgeFloor:
			c = color.NRGBA{R: 9, G: 9, B: 9, A: 128}
		case EdgeOwn:
			c = noxcolor.RGBA5551(colors[i] & 0x7fff).ColorNRGBA()
			c.A = 255
		}
		x, y := i%46, i/46
		for d := 0; d < 4; d++ {
			im.SetNRGBA(2*x+d%2, 2*y+d/2, c)
		}
	}
	return im
}

// edgeOwnColors maps each op-2 pixel of the stream to its raw colour.
func edgeOwnColors(raw []byte) map[int]uint16 {
	out := map[int]uint16{}
	pos := 2
	for y := int(raw[0]); y <= int(raw[1]); y++ {
		start, n, _ := tileRow(y)
		for x := 0; x < n; {
			op, count := raw[pos], int(raw[pos+1])
			pos += 2
			if op == EdgeOwn {
				for j := 0; j < count; j++ {
					out[y*46+start+x+j] = binary.LittleEndian.Uint16(raw[pos+2*j:])
				}
				pos += 2 * count
			}
			x += count
		}
	}
	return out
}

func convertEdgePNG(raw []byte, im image.Image) ([]uint16, error) {
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		return nil, err
	}
	s := FloorSpec{ID: 1, Type: 1, SourceSHA256: sha256.Sum256(raw), PNGSHA256: sha256.Sum256(b.Bytes()), Path: "e.png", LogicalSize: image.Pt(46, 46), Density: 2}
	dst := make([]uint16, AssetPixels(s))
	return dst, ConvertAsset(s, b.Bytes(), FloorSource{Type: 1, Raw: raw}, dst)
}

// testEdge: row 10 (13..33) keep 5, floor 6, own 4, keep 6; row 11 (12..34)
// keep 4, floor 15, keep 4.
func testEdge() []byte {
	raw := []byte{10, 11, 1, 5, 3, 6, 2, 4}
	for i := 0; i < 4; i++ {
		raw = binary.LittleEndian.AppendUint16(raw, uint16(0x1000+i*0x421))
	}
	return append(raw, 1, 6, 1, 4, 3, 15, 1, 4)
}

func TestWorldHDEdgeAsset(t *testing.T) {
	raw := testEdge()
	if n := AssetPixels(FloorSpec{Type: 1, LogicalSize: image.Pt(46, 46), Density: 2}); n != 2*EdgeSamples {
		t.Fatalf("edge samples %d", n)
	}
	got, err := convertEdgePNG(raw, edgeReplica(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	colors, ops, ok := EdgeAsset(got)
	if !ok {
		t.Fatal("edge planes")
	}
	own := edgeOwnColors(raw)
	at := func(x, y int) int { return y*92 + x }
	for y := 0; y < 92; y++ {
		for x := 0; x < 92; x++ {
			l := (y/2)*46 + x/2
			want := uint16(EdgeKeep)
			switch {
			case y/2 == 11 && x/2 >= 16 && x/2 < 31, y/2 == 10 && x/2 >= 18 && x/2 < 24:
				want = EdgeFloor
			case y/2 == 10 && x/2 >= 24 && x/2 < 28:
				want = EdgeOwn
				if colors[at(x, y)] != own[l] {
					t.Fatalf("colour at %d,%d: %#x != %#x", x, y, colors[at(x, y)], own[l])
				}
			}
			if ops[at(x, y)] != want {
				t.Fatalf("op at %d,%d: %d != %d", x, y, ops[at(x, y)], want)
			}
		}
	}

	// Half-pixel refinement: a keep pixel next to the floor run may take floor,
	// and an own pixel next to floor may take floor.
	im := edgeReplica(t, raw)
	im.SetNRGBA(2*17+1, 2*10, color.NRGBA{A: 128})
	im.SetNRGBA(2*24, 2*10+1, color.NRGBA{A: 128})
	if got, err = convertEdgePNG(raw, im); err != nil {
		t.Fatalf("neighbour refinement rejected: %v", err)
	}
	if _, ops, _ = EdgeAsset(got); ops[at(35, 20)] != EdgeFloor || ops[at(48, 21)] != EdgeFloor {
		t.Fatal("refined samples not converted")
	}

	for name, edit := range map[string]func(*image.NRGBA){
		"two pixels from any floor":   func(m *image.NRGBA) { m.SetNRGBA(2*14, 2*10, color.NRGBA{A: 128}) },
		"own colour beside keep only": func(m *image.NRGBA) { m.SetNRGBA(2*30, 2*10, color.NRGBA{R: 255, A: 255}) },
		"outside the edge rows":       func(m *image.NRGBA) { m.SetNRGBA(2*20, 2*12, color.NRGBA{A: 128}) },
		"outside the diamond":         func(m *image.NRGBA) { m.SetNRGBA(2*12, 2*10, color.NRGBA{A: 128}) },
		"partial alpha":               func(m *image.NRGBA) { m.SetNRGBA(2*20, 2*10, color.NRGBA{A: 64}) },
	} {
		m := edgeReplica(t, raw)
		edit(m)
		if _, err := convertEdgePNG(raw, m); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	for name, bad := range map[string][]byte{
		"rows reversed": {11, 10, 3, 23},
		"short row":     {11, 11, 3, 22},
		"trailing":      append(testEdge(), 1),
		"bad op":        {11, 11, 4, 23},
	} {
		if _, err := edgeOps(bad); err == nil {
			t.Fatalf("%s edge stream accepted", name)
		}
	}
}

// edgeRings draws base then edge (without and with asset) on two rings at a
// wrapping anchor and returns them.
func edgeRings(t *testing.T, raw []byte, floor []uint16, asset []uint16, base bool) (legacy, hd *Tiles) {
	t.Helper()
	under := make([]byte, 2116)
	for i := 0; i < len(under); i += 2 {
		binary.LittleEndian.PutUint16(under[i:], uint16(0x2000+i))
	}
	top := make([]byte, 2116)
	for i := 0; i < len(top); i += 2 {
		binary.LittleEndian.PutUint16(top[i:], 0x1111)
	}
	const anchor = 4090
	legacy, hd = NewTiles(64, 64), NewTiles(64, 64)
	for _, r := range []*Tiles{legacy, hd} {
		if base && !r.Base(anchor, top, nil) {
			t.Fatal("base rejected")
		}
	}
	if !legacy.Edge(anchor, under, raw, floor, nil) || !hd.Edge(anchor, under, raw, floor, asset) {
		t.Fatal("edge rejected")
	}
	return legacy, hd
}

func TestTilesEdgeAsset(t *testing.T) {
	raw := testEdge()
	floor := fixtureSamples(t)
	replica, err := convertEdgePNG(raw, edgeReplica(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	own := edgeOwnColors(raw)
	for _, f := range [][]uint16{nil, floor} {
		for _, base := range []bool{true, false} {
			a, b := edgeRings(t, raw, f, replica, base)
			for i := range a.pix {
				if a.pix[i] != b.pix[i] || a.valid[i] != b.valid[i] {
					t.Fatalf("replica differs at cell %d (HD floor %t, base %t)", i, f != nil, base)
				}
			}
		}
	}
	// Own colours count as HD detail; everything else keeps the legacy flag.
	a, b := edgeRings(t, raw, floor, replica, true)
	for y := 0; y < 46; y++ {
		for x := 0; x < 46; x++ {
			i := a.index(4090 + y*64 + x)
			_, isOwn := own[y*46+x]
			if want := a.detail[i] || isOwn; b.detail[i] != want {
				t.Fatalf("detail at %d,%d: %t", x, y, b.detail[i])
			}
		}
	}

	// One refined sample: keep pixel (17,10) takes the underlying floor in its
	// top-right sample only.
	im := edgeReplica(t, raw)
	im.SetNRGBA(2*17+1, 2*10, color.NRGBA{A: 128})
	one, err := convertEdgePNG(raw, im)
	if err != nil {
		t.Fatal(err)
	}
	a, b = edgeRings(t, raw, floor, one, true)
	changed := b.index(4090 + 10*64 + 17)
	for i := range a.pix {
		if i == changed {
			continue
		}
		if a.pix[i] != b.pix[i] || a.valid[i] != b.valid[i] {
			t.Fatalf("unrelated cell %d changed", i)
		}
	}
	if b.pix[changed][0] != a.pix[changed][0] || b.pix[changed][2] != a.pix[changed][2] || b.pix[changed][3] != a.pix[changed][3] ||
		b.pix[changed][1] != floor[20*92+35] || !b.detail[changed] {
		t.Fatalf("refined cell %v (was %v)", b.pix[changed], a.pix[changed])
	}
	// Without a base the mixed cell has no value to keep: the whole edge uses
	// the original path.
	a, b = edgeRings(t, raw, floor, one, false)
	for i := range a.pix {
		if a.pix[i] != b.pix[i] || a.valid[i] != b.valid[i] || a.detail[i] != b.detail[i] {
			t.Fatalf("invalid-cell fallback differs at %d", i)
		}
	}
	// A wrong-size asset is ignored.
	a, b = edgeRings(t, raw, floor, replica[1:], true)
	for i := range a.pix {
		if a.pix[i] != b.pix[i] || a.detail[i] != b.detail[i] {
			t.Fatalf("short asset used at %d", i)
		}
	}
}
