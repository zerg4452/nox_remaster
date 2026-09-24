package worldhd

import (
	"encoding/json"
	"fmt"
	"testing"
	"testing/fstest"
)

func TestManifestValidation(t *testing.T) {
	s, _, _ := fixture(t)
	m := Manifest{Version: 1, Floors: []ManifestFloor{{ID: s.ID, SourceSHA256: fmt.Sprintf("%x", s.SourceSHA256), PNGSHA256: fmt.Sprintf("%x", s.PNGSHA256), File: s.Path, Width: 46, Height: 46, Density: 2}}}
	raw, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	read := func(data []byte) ([]FloorSpec, error) {
		return ReadManifest(fstest.MapFS{"floors.json": &fstest.MapFile{Data: data}})
	}
	specs, e := read(raw)
	if e != nil || len(specs) != 1 || specs[0] != s {
		t.Fatal("manifest identity changed", e)
	}
	for _, bad := range [][]byte{[]byte(`{"version":2,"floors":[]}`), []byte(`{"version":1,"floors":[],"typo":true}`), append(append([]byte(nil), raw...), []byte(` {}`)...), []byte(`{"version":1,"floors":[{"id":9166,"source_sha256":"bad"}]}`), make([]byte, 65537)} {
		if _, e := read(bad); e == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
