package worldhd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/fs"
)

type Manifest struct {
	Version int             `json:"version"`
	Floors  []ManifestFloor `json:"floors"`
}

type ManifestFloor struct {
	ID           int    `json:"id"`
	Type         int    `json:"type"`
	SourceSHA256 string `json:"source_sha256"`
	PNGSHA256    string `json:"png_sha256"`
	File         string `json:"file"`
	Width        int    `json:"logical_width"`
	Height       int    `json:"logical_height"`
	OffsetX      int    `json:"offset_x"`
	OffsetY      int    `json:"offset_y"`
	Density      int    `json:"density"`
}

// ReadManifest reads only the fixed manifest name under a trusted root.
func ReadManifest(root fs.FS) ([]FloorSpec, error) {
	f, err := root.Open("floors.json")
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	ce := f.Close()
	if err != nil {
		return nil, err
	}
	if ce != nil {
		return nil, ce
	}
	if len(data) > 65536 {
		return nil, fmt.Errorf("floor manifest too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var m Manifest
	if err = d.Decode(&m); err != nil {
		return nil, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fmt.Errorf("trailing manifest data")
	}
	if m.Version != 1 || len(m.Floors) == 0 || len(m.Floors) > 256 {
		return nil, fmt.Errorf("unsupported or empty floor manifest")
	}
	var out []FloorSpec
	for _, v := range m.Floors {
		s := FloorSpec{ID: v.ID, Type: v.Type, Path: v.File, LogicalSize: image.Pt(v.Width, v.Height), Offset: image.Pt(v.OffsetX, v.OffsetY), Density: v.Density}
		for _, h := range []struct {
			text string
			dst  *[32]byte
		}{{v.SourceSHA256, &s.SourceSHA256}, {v.PNGSHA256, &s.PNGSHA256}} {
			b, e := hex.DecodeString(h.text)
			if e != nil || len(b) != 32 {
				return nil, fmt.Errorf("floor %d: invalid fingerprint", v.ID)
			}
			copy(h.dst[:], b)
		}
		out = append(out, s)
	}
	return out, nil
}
