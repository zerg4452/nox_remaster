package worldhd

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"strings"
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
		s, err := specFromManifest(v)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func specFromManifest(v ManifestFloor) (FloorSpec, error) {
	s := FloorSpec{ID: v.ID, Type: v.Type, Path: v.File, LogicalSize: image.Pt(v.Width, v.Height), Offset: image.Pt(v.OffsetX, v.OffsetY), Density: v.Density}
	for _, h := range []struct {
		text string
		dst  *[32]byte
	}{{v.SourceSHA256, &s.SourceSHA256}, {v.PNGSHA256, &s.PNGSHA256}} {
		b, e := hex.DecodeString(h.text)
		if e != nil || len(b) != 32 {
			return FloorSpec{}, fmt.Errorf("floor %d: invalid fingerprint", v.ID)
		}
		copy(h.dst[:], b)
	}
	return s, nil
}

// Manifest v2 splits entries into shard files listed by index.json so the
// full asset set can grow past the v1 single-file limits. Entries keep the v1
// fields; per-asset type/geometry checks happen in ConvertAsset, so one bad
// asset is rejected alone while a malformed index rejects the whole set.
const (
	maxIndexBytes  = 1 << 16
	maxShards      = 1024
	maxShardBytes  = 16 << 20
	maxIndexAssets = 200000
)

type assetIndex struct {
	Version int      `json:"version"`
	Shards  []string `json:"shards"`
}

type assetShard struct {
	Version int             `json:"version"`
	Assets  []ManifestFloor `json:"assets"`
}

func decodeStrict(root fs.FS, name string, limit int64, out any) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("%s too large", name)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("%s: trailing data", name)
	}
	return nil
}

func validAssetPath(p string) bool { return fs.ValidPath(p) && !strings.ContainsAny(p, "\\:") }

// ReadAssetIndex returns specs by record ID from index.json (v2), or from the
// v1 floors.json when no v2 index exists.
func ReadAssetIndex(root fs.FS) (map[int]FloorSpec, error) {
	var idx assetIndex
	err := decodeStrict(root, "index.json", maxIndexBytes, &idx)
	var specs []FloorSpec
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if specs, err = ReadManifest(root); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if idx.Version != 2 || len(idx.Shards) == 0 || len(idx.Shards) > maxShards {
			return nil, fmt.Errorf("unsupported or empty asset index")
		}
		for _, name := range idx.Shards {
			if !validAssetPath(name) {
				return nil, fmt.Errorf("invalid shard path %q", name)
			}
			var sh assetShard
			if err = decodeStrict(root, name, maxShardBytes, &sh); err != nil {
				return nil, err
			}
			if sh.Version != 2 {
				return nil, fmt.Errorf("%s: unsupported version", name)
			}
			for _, v := range sh.Assets {
				s, err := specFromManifest(v)
				if err != nil {
					return nil, err
				}
				s.Shard = name
				specs = append(specs, s)
			}
		}
	}
	if len(specs) > maxIndexAssets {
		return nil, fmt.Errorf("asset index too large")
	}
	out := make(map[int]FloorSpec, len(specs))
	for _, s := range specs {
		if _, dup := out[s.ID]; dup {
			return nil, fmt.Errorf("asset %d: duplicate record", s.ID)
		}
		out[s.ID] = s
	}
	return out, nil
}
