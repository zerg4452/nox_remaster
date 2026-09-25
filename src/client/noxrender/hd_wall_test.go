package noxrender

import (
	"encoding/binary"
	"image"
	"reflect"
	"testing"
	"unsafe"
)

// wallAssetRecord: type-3 wall record 3x2 at offset (1,1) with only skip and
// opaque (op 2) runs, the ops nox_xxx_edgeDraw_480EF0 draws.
func wallAssetRecord() []byte {
	rec := make([]byte, 17)
	binary.LittleEndian.PutUint32(rec[0:], 3)
	binary.LittleEndian.PutUint32(rec[4:], 2)
	binary.LittleEndian.PutUint32(rec[8:], 1)
	binary.LittleEndian.PutUint32(rec[12:], 1)
	rec = append(rec, 2, 3)
	rec = binary.LittleEndian.AppendUint16(rec, 0x7fff)
	rec = binary.LittleEndian.AppendUint16(rec, 0x4a52)
	rec = binary.LittleEndian.AppendUint16(rec, 0x1ce7)
	rec = append(rec, 1, 1, 2, 2)
	rec = binary.LittleEndian.AppendUint16(rec, 0x6318)
	rec = binary.LittleEndian.AppendUint16(rec, 0x3def)
	return rec
}

type wallSpan struct {
	local, pos image.Point
	src        []uint16
}

// drawLitWall emulates the edgeDraw sequence: begin with the image handle,
// then per lit span announce its image-local position (optional) and light it.
func drawLitWall(t *testing.T, rec []byte, override bool, asset []uint16, announce bool, spans []wallSpan) ([]uint16, []uint16, *NoxRender) {
	t.Helper()
	r, img := spriteAssetRender(rec, override, asset, false)
	defer img.Free()
	h := ImageHandle(unsafe.Pointer(&make([]byte, 1)[0]))
	r.Bag.byHandle = map[ImageHandle]*Image{h: img}
	if !r.BeginWorldHDFrame() {
		t.Fatal("begin failed")
	}
	r.BeginWorldWall(h)
	for _, s := range spans {
		light := []uint32{0xc000, 0x9000, 0xffff}
		step := []uint32{0x0100, 0x0400, 0}
		dst := r.pix.Row(s.pos.Y)[s.pos.X : s.pos.X+len(s.src)]
		if announce {
			r.WorldWallSpan(s.local)
		}
		r.LitWallSpan(dst, append([]uint16(nil), s.src...), light, step)
	}
	out := r.EndHDFrame()
	if out == nil {
		t.Fatal("wall frame fell back")
	}
	return append([]uint16(nil), r.pix.Pix...), append([]uint16(nil), out.Pix...), r
}

// Lit opaque walls (4.3-001c): a replica asset reproduces the 2x2 mirror, a
// changed sample reaches exactly its HD pixel with that pixel's light, and
// override data, unannounced spans or spans outside the asset keep the mirror.
func TestWorldHDLitWallAsset(t *testing.T) {
	rec := wallAssetRecord()
	spans := []wallSpan{
		{local: image.Pt(0, 0), pos: image.Pt(2, 1), src: []uint16{0x7fff, 0x4a52, 0x1ce7}},
		{local: image.Pt(1, 1), pos: image.Pt(3, 2), src: []uint16{0x6318, 0x3def}},
		{local: image.Pt(1, 0), pos: image.Pt(4, 3), src: []uint16{0x4a52, 0x1ce7}}, // left-clipped run
	}
	wantLogical, wantHD, _ := drawLitWall(t, rec, false, nil, true, spans)

	logical, hd, _ := drawLitWall(t, rec, false, replicaAsset(rec), true, spans)
	if !reflect.DeepEqual(logical, wantLogical) || !reflect.DeepEqual(hd, wantHD) {
		t.Fatal("replica wall asset changed output")
	}

	// Asset subpixel (3,2) is image-local (1,1): the first pixel of span 2,
	// drawn at logical (3,2) with the span's start light; HD (2*3+1, 2*2) = (7,4).
	asset := replicaAsset(rec)
	asset[2*6+3] = 0x2d6b
	logical, hd, r := drawLitWall(t, rec, false, asset, true, spans)
	want := append([]uint16(nil), wantHD...)
	want[4*r.hd.pix.Stride+7] = litWallPixel(0x2d6b, []uint32{0xc000, 0x9000, 0xffff})
	if want[4*r.hd.pix.Stride+7] == wantHD[4*r.hd.pix.Stride+7] {
		t.Fatal("test sample does not change the pixel")
	}
	if !reflect.DeepEqual(logical, wantLogical) || !reflect.DeepEqual(hd, want) {
		t.Fatal("wall asset sample not used exactly once")
	}

	for name, run := range map[string]func() ([]uint16, []uint16){
		"override": func() ([]uint16, []uint16) {
			base, baseHD, _ := drawLitWall(t, rec, true, nil, true, spans)
			if !reflect.DeepEqual(base, wantLogical) || !reflect.DeepEqual(baseHD, wantHD) {
				t.Fatal("override baseline differs")
			}
			l, h, _ := drawLitWall(t, rec, true, asset, true, spans)
			return l, h
		},
		"unannounced": func() ([]uint16, []uint16) {
			l, h, _ := drawLitWall(t, rec, false, asset, false, spans)
			return l, h
		},
		"outside asset": func() ([]uint16, []uint16) {
			bad := append([]wallSpan(nil), spans...)
			bad[1].local = image.Pt(2, 1) // 2*(2+2) > stride 6
			l, h, _ := drawLitWall(t, rec, false, asset, true, bad)
			return l, h
		},
	} {
		l, h := run()
		if !reflect.DeepEqual(l, wantLogical) || !reflect.DeepEqual(h, wantHD) {
			t.Fatalf("%s: wall asset used", name)
		}
	}

	// Without a client the span is only lit (sub_480860 with no renderer).
	var none *NoxRender
	dst := make([]uint16, 3)
	none.LitWallSpan(dst, []uint16{0x7fff, 0x4a52, 0x1ce7}, []uint32{0xc000, 0x9000, 0xffff}, []uint32{0x0100, 0x0400, 0})
	if dst[0] != litWallPixel(0x7fff, []uint32{0xc000, 0x9000, 0xffff}) {
		t.Fatal("nil renderer did not light the span")
	}
}
