//go:build worldhdassets

package worldhd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noxworld-dev/opennox-lib/bag"
)

// Real edge records (4.3-002a): a 2x replica of every type-1 record converts
// back to the original ops and edge-owned colours.
func TestWorldHDRealEdgeReplicaRoundTrip(t *testing.T) {
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
	checked, own, bit15 := 0, 0, 0
	for i, r := range records {
		if r.Type != 1 {
			continue
		}
		raw, err := r.Raw()
		if err != nil {
			t.Fatal(err)
		}
		ops, err := edgeOps(raw)
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		got, err := convertEdgePNG(raw, edgeReplica(t, raw))
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		colors, hdOps, _ := EdgeAsset(got)
		ownColors := edgeOwnColors(raw)
		for k := range hdOps {
			l := (k/92/2)*46 + (k%92)/2
			want := uint16(ops[l])
			if want == 0 {
				want = EdgeKeep
			}
			if hdOps[k] != want {
				t.Fatalf("record %d sample %d: op %d != %d", i, k, hdOps[k], want)
			}
			if want == EdgeOwn {
				c := ownColors[l]
				if c&0x8000 != 0 {
					bit15++
				}
				if colors[k] != c&0x7fff {
					t.Fatalf("record %d sample %d: colour %#x != %#x", i, k, colors[k], c)
				}
				own++
			}
		}
		// The replica composes exactly like the original stream (4.3-002b).
		if a, b := edgeRings(t, raw, nil, got, true); !tilesPixEqual(a, b) {
			t.Fatalf("record %d: replica ring differs", i)
		}
		checked++
	}
	if checked < 800 || own == 0 {
		t.Fatalf("too few edge records checked: %d (own samples %d)", checked, own)
	}
	t.Logf("real edge replicas: %d round-tripped, %d own samples (%d with bit 15 set)", checked, own, bit15)
}

func tilesPixEqual(a, b *Tiles) bool {
	for i := range a.pix {
		if a.pix[i] != b.pix[i] || a.valid[i] != b.valid[i] {
			return false
		}
	}
	return true
}
