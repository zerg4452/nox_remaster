//go:build worldhdassets

package worldhd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
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
	set, e := LoadFloors(os.DirFS(dir), specs, func(id int) (FloorSource, error) {
		if id >= len(records) {
			return FloorSource{}, fmt.Errorf("missing record %d", id)
		}
		r := records[id]
		raw, e := r.Raw()
		if e != nil {
			return FloorSource{}, e
		}
		decoded, e := pcx.Decode(bytes.NewReader(raw), byte(r.Type))
		if e != nil {
			return FloorSource{}, e
		}
		return FloorSource{Type: int(r.Type), Raw: raw, Mask: decoded.Image, Offset: decoded.Point}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range specs {
		if set.Lookup(s.ID, 0, s.SourceSHA256) == nil {
			t.Fatalf("floor %d not selected", s.ID)
		}
		t.Logf("floor %d: raw identity + recorded candidate hash + 8464 alpha pixels verified", s.ID)
	}
	if set.Lookup(9166, 0, sha256.Sum256([]byte("wrong source"))) != nil {
		t.Fatal("wrong bag selected")
	}
}
