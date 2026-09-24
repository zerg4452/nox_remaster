package noxrender

import (
	"encoding/binary"
	"image"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/bag"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// spriteAssetRecord: type-3 record 4x3 at offset (1,1) mixing skip, opaque
// (op 2/7) and 4444 (op 5) runs with distinct pixel values.
func spriteAssetRecord() []byte {
	rec := make([]byte, 17)
	binary.LittleEndian.PutUint32(rec[0:], 4)
	binary.LittleEndian.PutUint32(rec[4:], 3)
	binary.LittleEndian.PutUint32(rec[8:], 1)
	binary.LittleEndian.PutUint32(rec[12:], 1)
	v := uint16(0x0421)
	run := func(op byte, n int) {
		rec = append(rec, op, byte(n))
		if op != 1 {
			for i := 0; i < n; i++ {
				v += 0x0843
				rec = binary.LittleEndian.AppendUint16(rec, v)
			}
		}
	}
	run(1, 1)
	run(2, 2)
	run(5, 1)
	run(7, 3)
	run(1, 1)
	run(2, 1)
	run(5, 2)
	run(1, 1)
	return rec
}

// replicaAsset repeats every original sample 2x2 (uncovered pixels stay 0).
func replicaAsset(rec []byte) []uint16 {
	w, h := int(binary.LittleEndian.Uint32(rec[0:])), int(binary.LittleEndian.Uint32(rec[4:]))
	out := make([]uint16, 4*w*h)
	pix := rec[17:]
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			op, n := pix[0]&0xF, int(pix[1])
			pix = pix[2:]
			for i := 0; i < n && op != 1; i++ {
				v := binary.LittleEndian.Uint16(pix[2*i:])
				for d := 0; d < 4; d++ {
					out[(2*y+d/2)*2*w+2*(x+i)+d%2] = v
				}
			}
			if op != 1 {
				pix = pix[2*n:]
			}
			x += n
		}
	}
	return out
}

// spriteAssetRender binds the record to a bag-owned image, optionally as
// override data, and installs asset (nil = none) for it.
func spriteAssetRender(rec []byte, override bool, asset []uint16, clip bool) (*NoxRender, *Image) {
	r := hdTestRender(6, 5)
	img := &Image{c: &r.Bag, bag: &bag.ImageRec{Index: 0, Type: 3}}
	if override {
		img.override = rec
	} else {
		img.raw = rec
	}
	r.Bag.byIndex = []*Image{img}
	if asset != nil {
		cache := newHDAssetCache(hdAssetBudget)
		cache.Install(&hdAsset{img: img, pix: asset, free: func() {}, bytes: 2 * len(asset)})
		r.Bag.worldHD = &hdAssetLoader{cache: cache, specs: map[int]worldhd.FloorSpec{}, pending: map[*Image]int{}, rejected: map[*Image]struct{}{}}
	}
	for i := range r.pix.Pix {
		r.pix.Pix[i] = 0x1234
	}
	r.Data().SetMaterial(0, image.White)
	if clip {
		r.Data().SetClip(true)
		r.Data().SetClipRect(image.Rect(3, 2, 6, 5))
	}
	return r, img
}

func drawSpriteHD(t *testing.T, r *NoxRender, img *Image) ([]uint16, []uint16) {
	t.Helper()
	defer img.Free()
	if !r.BeginWorldHDFrame() {
		t.Fatal("begin failed")
	}
	r.DrawImage16(img, image.Pt(1, 0))
	out := r.EndHDFrame()
	if out == nil {
		t.Fatal("sprite frame fell back")
	}
	return append([]uint16(nil), r.pix.Pix...), append([]uint16(nil), out.Pix...)
}

func TestWorldHDSpriteAsset(t *testing.T) {
	rec := spriteAssetRecord()
	for _, clip := range []bool{false, true} {
		base, baseImg := spriteAssetRender(rec, false, nil, clip)
		wantLogical, wantHD := drawSpriteHD(t, base, baseImg)

		// A replica asset must reproduce the 2x path exactly.
		r, img := spriteAssetRender(rec, false, replicaAsset(rec), clip)
		logical, hd := drawSpriteHD(t, r, img)
		if !reflect.DeepEqual(logical, wantLogical) || !reflect.DeepEqual(hd, wantHD) {
			t.Fatalf("clip=%t: replica asset changed output", clip)
		}

		// A distinct sample reaches exactly its HD pixel. Image-local (2,1) is
		// an op-7 pixel drawn at screen (1+1+2, 0+1+1) = (4,2), visible in both
		// clip modes; asset subpixel (5,3) maps to HD (9,5).
		asset := replicaAsset(rec)
		asset[3*8+5] = 0x7c1f
		r, img = spriteAssetRender(rec, false, asset, clip)
		logical, hd = drawSpriteHD(t, r, img)
		want := append([]uint16(nil), wantHD...)
		want[5*r.hd.pix.Stride+9] = 0x7c1f
		if !reflect.DeepEqual(logical, wantLogical) || !reflect.DeepEqual(hd, want) {
			t.Fatalf("clip=%t: asset sample not used exactly once", clip)
		}

		// Override data is not the record the asset was verified against.
		base, baseImg = spriteAssetRender(rec, true, nil, clip)
		_, wantHD = drawSpriteHD(t, base, baseImg)
		r, img = spriteAssetRender(rec, true, asset, clip)
		if _, hd = drawSpriteHD(t, r, img); !reflect.DeepEqual(hd, wantHD) {
			t.Fatalf("clip=%t: asset applied to override data", clip)
		}
		if r.hd.sprite != nil {
			t.Fatal("sprite asset leaked past the draw")
		}
	}
}
