//go:build worldhdassets

package worldhd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noxworld-dev/opennox-lib/bag"
	"github.com/noxworld-dev/opennox-lib/noximage/pcx"
)

// Explicit opt-in: proprietary originals and generated art are not test fixtures
// in the engine repository. No game process or output files are created here.
func TestCaveHardBrownAssets(t *testing.T) {
	root := os.Getenv("NOX_WORLDHD_TEST_PROJECT")
	if root == "" {
		t.Fatal("set NOX_WORLDHD_TEST_PROJECT to the remaster project")
	}
	readJSON := func(path string, out any) {
		t.Helper()
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, out); e != nil {
			t.Fatal(e)
		}
	}
	var original struct {
		Rows []struct {
			ID  int    `json:"image_id"`
			Key string `json:"typed_raw_key"`
		}
	}
	var produced struct {
		Rows []struct {
			ID   int    `json:"image_id"`
			Hash string `json:"output_sha256"`
		}
	}
	readJSON(filepath.Join(root, "assets/work/group4-inventory-20260910-008a/decode-report.json"), &original)
	dir := filepath.Join(root, "assets/work/group4-family-20260914-004/final-review")
	readJSON(filepath.Join(dir, "family-report.json"), &produced)
	hash := func(s string) (out [32]byte) {
		t.Helper()
		b, e := hex.DecodeString(s)
		if e != nil || len(b) != 32 {
			t.Fatalf("invalid hash %q", s)
		}
		copy(out[:], b)
		return
	}
	keys := map[int]string{}
	for _, r := range original.Rows {
		keys[r.ID] = r.Key
	}
	var specs []FloorSpec
	for _, r := range produced.Rows {
		if r.ID < 9160 || r.ID > 9168 || !strings.HasPrefix(keys[r.ID], "0:") {
			t.Fatalf("unexpected floor %d", r.ID)
		}
		specs = append(specs, FloorSpec{ID: r.ID, SourceSHA256: hash(strings.TrimPrefix(keys[r.ID], "0:")), PNGSHA256: hash(r.Hash), Path: fmt.Sprintf("%06d.png", r.ID), LogicalSize: image.Pt(46, 46), Density: 2})
	}
	if len(specs) != 9 {
		t.Fatal("incomplete family")
	}
	f, e := bag.Open(filepath.Join(filepath.Dir(root), "Nox/video.bag"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	records, e := f.Images()
	if e != nil {
		t.Fatal(e)
	}
	source := func(id int) FloorSource {
		t.Helper()
		if id >= len(records) {
			t.Fatalf("missing record %d", id)
		}
		r := records[id]
		raw, e := r.Raw()
		if e != nil {
			t.Fatal(e)
		}
		decoded, e := pcx.Decode(bytes.NewReader(raw), byte(r.Type))
		if e != nil {
			t.Fatal(e)
		}
		return FloorSource{Type: int(r.Type), Raw: raw, Mask: decoded.Image, Offset: decoded.Point}
	}
	for _, s := range specs {
		data, e := os.ReadFile(filepath.Join(dir, s.Path))
		if e != nil {
			t.Fatal(e)
		}
		dst := make([]uint16, AssetPixels(s))
		if e := ConvertAsset(s, data, source(s.ID), dst); e != nil {
			t.Fatal(e)
		}
		// Covered samples are the RGB555 of the PNG texel, as the former NRGBA
		// floor path produced; ConvertAsset already enforced the 8464-pixel mask.
		im, e := png.Decode(bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		for y := 0; y < 92; y++ {
			for x := 0; x < 92; x++ {
				if c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA); c.A != 0 {
					if want := uint16(c.R&248)<<7 | uint16(c.G&248)<<2 | uint16(c.B)>>3; dst[y*92+x] != want {
						t.Fatalf("floor %d sample %d,%d: %#x != %#x", s.ID, x, y, dst[y*92+x], want)
					}
				}
			}
		}
		t.Logf("floor %d: raw identity + recorded candidate hash + 8464 alpha pixels + samples verified", s.ID)
	}
	wrong := source(9166)
	wrong.Raw = []byte("wrong source")
	data, _ := os.ReadFile(filepath.Join(dir, specs[0].Path))
	if ConvertAsset(specs[0], data, wrong, make([]uint16, AssetPixels(specs[0]))) == nil {
		t.Fatal("wrong bag record accepted")
	}
}
