//go:build worldhdassets

package worldhd

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/noxworld-dev/opennox-lib/bag"
	noxcolor "github.com/noxworld-dev/opennox-lib/color"
)

// Real sprite records (4.2-M6): a 2x replica PNG of every sampled type 3/5
// record with only op 1/2/5/7 runs must convert back to the original samples.
func TestWorldHDRealSpriteReplicaRoundTrip(t *testing.T) {
	root := os.Getenv("NOX_WORLDHD_TEST_PROJECT")
	if root == "" {
		t.Fatal("set NOX_WORLDHD_TEST_PROJECT to the remaster project")
	}
	f, err := bag.Open(filepath.Join(filepath.Dir(root), "Nox/video.bag"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := f.Images()
	if err != nil {
		t.Fatal(err)
	}
	checked, skipped, bit15 := 0, 0, 0
	for i, r := range records {
		if (r.Type != 3 && r.Type != 5) || i%10 != 0 {
			continue
		}
		raw, err := r.Raw()
		if err != nil {
			t.Fatal(err)
		}
		w, h := int(int32(binary.LittleEndian.Uint32(raw[0:]))), int(int32(binary.LittleEndian.Uint32(raw[4:])))
		if len(raw) < 17 || w <= 0 || h <= 0 || w > 4096 || h > 4096 {
			skipped++
			continue
		}
		want := make([]uint16, 4*w*h)
		im := image.NewNRGBA(image.Rect(0, 0, 2*w, 2*h))
		pix, ok := raw[17:], true
		for y := 0; y < h && ok; y++ {
			for x := 0; x < w && ok; {
				if len(pix) < 2 {
					ok = false
					break
				}
				op, n := pix[0]&0xF, int(pix[1])
				pix = pix[2:]
				if op != 1 && op != 2 && op != 5 && op != 7 || n == 0 || op != 1 && len(pix) < 2*n {
					ok = false
					break
				}
				for j := 0; j < n && op != 1 && x+j < w; j++ {
					v := binary.LittleEndian.Uint16(pix[2*j:])
					var c color.NRGBA
					if op == 5 {
						c = noxcolor.RGBA4444(v).ColorNRGBA()
					} else {
						if v&0x8000 != 0 {
							bit15++
						}
						v &= 0x7fff // RGB555 output carries no bit 15
						c = noxcolor.RGBA5551(v).ColorNRGBA()
						c.A = 255
					}
					for d := 0; d < 4; d++ {
						px, py := 2*(x+j)+d%2, 2*y+d/2
						im.SetNRGBA(px, py, c)
						want[py*2*w+px] = v
					}
				}
				if op != 1 {
					pix = pix[2*n:]
				}
				x += n
			}
		}
		if !ok {
			skipped++
			continue
		}
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		off := image.Pt(int(int32(binary.LittleEndian.Uint32(raw[8:]))), int(int32(binary.LittleEndian.Uint32(raw[12:]))))
		s := FloorSpec{ID: i, Type: int(r.Type), SourceSHA256: sha256.Sum256(raw), PNGSHA256: sha256.Sum256(b.Bytes()), Path: "x.png", LogicalSize: image.Pt(w, h), Offset: off, Density: 2}
		got := make([]uint16, AssetPixels(s))
		if err := ConvertAsset(s, b.Bytes(), FloorSource{Type: int(r.Type), Raw: raw}, got); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		for k := range got {
			if got[k] != want[k] {
				t.Fatalf("record %d sample %d: %#x != %#x", i, k, got[k], want[k])
			}
		}
		checked++
	}
	if checked < 1000 {
		t.Fatalf("too few records checked: %d", checked)
	}
	t.Logf("real sprite replicas: %d round-tripped, %d skipped (op4/op6 or malformed), %d opaque samples had bit 15 set", checked, skipped, bit15)
}
