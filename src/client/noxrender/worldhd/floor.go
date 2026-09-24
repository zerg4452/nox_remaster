// Package worldhd validates optional world assets independently of menu assets.
// It does not enable HD presentation or change legacy tile pixels.
package worldhd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"strings"
)

// FloorSpec binds a replacement to the raw bag record, not its non-unique name
// or an exported PNG's hash. Initial support is type 0, 46x46, offset 0, density 2.
type FloorSpec struct {
	ID           int
	Type         int
	SourceSHA256 [32]byte
	PNGSHA256    [32]byte
	Path         string
	LogicalSize  image.Point
	Offset       image.Point
	Density      int
}

// FloorSource must be resolved from the current bag by record index. Mask is
// the decoded original; Raw is the unmodified record payload (before overrides).
type FloorSource struct {
	Type   int
	Raw    []byte
	Mask   image.Image
	Offset image.Point
}

type Floor struct {
	spec   FloorSpec
	pixels *image.NRGBA
}

type Floors struct{ byID map[int]*Floor }

// LoadFloors returns a new, immutable complete set or nil on any error. Callers
// must assign the returned set even on failure, then use the original renderer
// when Lookup returns nil. A failed reload must not retain stale HD assets.
// The filesystem is a trusted asset root (not a symlink security boundary).
func LoadFloors(root fs.FS, specs []FloorSpec, resolve func(int) (FloorSource, error)) (*Floors, error) {
	if root == nil || resolve == nil || len(specs) > 256 {
		return nil, fmt.Errorf("invalid floor loader inputs")
	}
	out := &Floors{byID: make(map[int]*Floor, len(specs))}
	for _, s := range specs {
		if s.ID < 0 || s.Type != 0 || s.LogicalSize != image.Pt(46, 46) || s.Offset != (image.Point{}) || s.Density != 2 || !fs.ValidPath(s.Path) || strings.ContainsAny(s.Path, "\\:") {
			return nil, fmt.Errorf("floor %d: unsupported metadata", s.ID)
		}
		if _, exists := out.byID[s.ID]; exists {
			return nil, fmt.Errorf("floor %d: duplicate record", s.ID)
		}
		src, err := resolve(s.ID)
		if err != nil {
			return nil, fmt.Errorf("floor %d: source: %w", s.ID, err)
		}
		if len(src.Raw) == 0 || sha256.Sum256(src.Raw) != s.SourceSHA256 || src.Type != s.Type || src.Offset != s.Offset || src.Mask == nil || src.Mask.Bounds() != (image.Rectangle{Max: s.LogicalSize}) {
			return nil, fmt.Errorf("floor %d: source identity or geometry mismatch", s.ID)
		}
		f, err := root.Open(s.Path)
		if err != nil {
			return nil, fmt.Errorf("floor %d: PNG: %w", s.ID, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		closeErr := f.Close()
		if readErr != nil || closeErr != nil || len(data) > 1<<20 || sha256.Sum256(data) != s.PNGSHA256 {
			return nil, fmt.Errorf("floor %d: PNG read/size/hash failure", s.ID)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 92 || cfg.Height != 92 {
			return nil, fmt.Errorf("floor %d: PNG geometry failure", s.ID)
		}
		im, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("floor %d: PNG decode: %w", s.ID, err)
		}
		pixels := image.NewNRGBA(image.Rect(0, 0, 92, 92))
		for y := 0; y < 92; y++ {
			for x := 0; x < 92; x++ {
				_, _, _, exactAlpha := im.At(x, y).RGBA()
				c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
				_, _, _, a := src.Mask.At(x/2, y/2).RGBA()
				if (a != 0 && a != 65535) || exactAlpha != a {
					return nil, fmt.Errorf("floor %d: original mask mismatch at %d,%d", s.ID, x, y)
				}
				pixels.SetNRGBA(x, y, c)
			}
		}
		out.byID[s.ID] = &Floor{spec: s, pixels: pixels}
	}
	return out, nil
}

// Lookup requires the current raw-record fingerprint, not just an index that
// could refer to a different bag after a data-set switch. Nil means original.
func (f *Floors) Lookup(id, typ int, rawHash [32]byte) *Floor {
	if f == nil {
		return nil
	}
	v := f.byID[id]
	if v == nil || v.spec.Type != typ || v.spec.SourceSHA256 != rawHash {
		return nil
	}
	return v
}

// NRGBAAt reads density-space pixels without exposing mutable backing storage.
func (f *Floor) NRGBAAt(x, y int) color.NRGBA { return f.pixels.NRGBAAt(x, y) }

// Clip maps an already projected logical anchor and logical clip to paired
// source/destination HD rectangles. It performs no camera/world projection,
// lighting, underlay composition or mutation of the logical game buffer.
func (f *Floor) Clip(anchor image.Point, clip image.Rectangle) (source, destination image.Rectangle) {
	pos := anchor.Add(f.spec.Offset)
	r := (image.Rectangle{Min: pos, Max: pos.Add(f.spec.LogicalSize)}).Intersect(clip)
	if r.Empty() {
		return image.Rectangle{}, image.Rectangle{}
	}
	s := f.spec.Density
	source = image.Rectangle{Min: r.Min.Sub(pos).Mul(s), Max: r.Max.Sub(pos).Mul(s)}
	destination = image.Rectangle{Min: r.Min.Mul(s), Max: r.Max.Mul(s)}
	return
}
