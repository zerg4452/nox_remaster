package worldhd

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"

	noxcolor "github.com/noxworld-dev/opennox-lib/color"
)

// ConvertAsset turns one verified PNG replacement into density-2 16-bit samples
// (row-major, stride LogicalSize.X*2) in the encoding the original draw op reads:
// RGB555 for floors and opaque sprite runs (op 2/7), RGBA4444 for alpha runs
// (op 5) and the 8-bit shade for indexed runs (op 4, 4.2-M6b), whose colour
// slot still comes from the original run. HD coverage must equal the original
// coverage: uncovered pixels need alpha 0, opaque and indexed ones alpha 255,
// and indexed ones must be grey (R=G=B, the shade). Records with op 6 runs are
// rejected and stay original.
// FloorSpec/FloorSource describe any record type here, not only floors.
const MaxAssetPNG = 8 << 20

// AssetPixels is the number of samples ConvertAsset writes for s. Edges
// (type 1) carry two planes: colours, then per-sample edge ops (EdgeAsset).
func AssetPixels(s FloorSpec) int {
	n := s.LogicalSize.X * s.LogicalSize.Y * s.Density * s.Density
	if s.Type == 1 {
		n *= 2
	}
	return n
}

// Per logical pixel coverage used for conversion (op&0xF values, 0 = uncovered).
const (
	covNone    = 0
	covOpaque  = 2
	covIndexed = 4
	covAlpha   = 5
)

func ConvertAsset(s FloorSpec, data []byte, src FloorSource, dst []uint16) error {
	w, h := s.LogicalSize.X, s.LogicalSize.Y
	if s.ID < 0 || s.Density != 2 || w <= 0 || h <= 0 || w > 4096 || h > 4096 || !validAssetPath(s.Path) {
		return fmt.Errorf("asset %d: unsupported metadata", s.ID)
	}
	if len(dst) != AssetPixels(s) {
		return fmt.Errorf("asset %d: destination size", s.ID)
	}
	if len(src.Raw) == 0 || sha256.Sum256(src.Raw) != s.SourceSHA256 || src.Type != s.Type {
		return fmt.Errorf("asset %d: source identity mismatch", s.ID)
	}
	cov, err := sourceCoverage(s, src)
	if err != nil {
		return fmt.Errorf("asset %d: %w", s.ID, err)
	}
	if len(data) > MaxAssetPNG || sha256.Sum256(data) != s.PNGSHA256 {
		return fmt.Errorf("asset %d: PNG size/hash failure", s.ID)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width != 2*w || cfg.Height != 2*h {
		return fmt.Errorf("asset %d: PNG geometry failure", s.ID)
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("asset %d: PNG decode: %w", s.ID, err)
	}
	if s.Type == 1 {
		return convertEdge(s.ID, im, cov, dst)
	}
	b := im.Bounds().Min
	for y := 0; y < 2*h; y++ {
		for x := 0; x < 2*w; x++ {
			px := im.At(b.X+x, b.Y+y)
			_, _, _, a := px.RGBA()
			c := color.NRGBAModel.Convert(px).(color.NRGBA)
			var v uint16
			switch cov[(y/2)*w+x/2] {
			case covNone:
				if a != 0 {
					return fmt.Errorf("asset %d: pixel outside original coverage at %d,%d", s.ID, x, y)
				}
			case covOpaque:
				if a != 0xffff {
					return fmt.Errorf("asset %d: opaque pixel not opaque at %d,%d", s.ID, x, y)
				}
				v = uint16(noxcolor.RGB5551Color(c.R, c.G, c.B))
			case covAlpha:
				v = uint16(noxcolor.RGBA4444Color(c.R, c.G, c.B, c.A))
			case covIndexed:
				if a != 0xffff || c.R != c.G || c.R != c.B {
					return fmt.Errorf("asset %d: indexed pixel not opaque grey at %d,%d", s.ID, x, y)
				}
				v = uint16(c.R)
			}
			dst[y*2*w+x] = v
		}
	}
	return nil
}

func sourceCoverage(s FloorSpec, src FloorSource) ([]byte, error) {
	w, h := s.LogicalSize.X, s.LogicalSize.Y
	switch s.Type {
	case 0:
		if s.LogicalSize != image.Pt(46, 46) || s.Offset != (image.Point{}) || src.Offset != s.Offset ||
			src.Mask == nil || src.Mask.Bounds() != (image.Rectangle{Max: s.LogicalSize}) {
			return nil, errors.New("floor geometry mismatch")
		}
		cov := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				switch _, _, _, a := src.Mask.At(x, y).RGBA(); a {
				case 0:
				case 0xffff:
					cov[y*w+x] = covOpaque
				default:
					return nil, errors.New("partial floor mask")
				}
			}
		}
		return cov, nil
	case 1:
		if s.LogicalSize != image.Pt(46, 46) || s.Offset != (image.Point{}) {
			return nil, errors.New("edge geometry mismatch")
		}
		return edgeOps(src.Raw)
	case 3, 4, 5, 6:
		return runCoverage(s, src.Raw)
	}
	return nil, errors.New("unsupported record type")
}

// runCoverage reads the RLE record the way the legacy drawer does: a 17-byte
// header, then per row (op, count) runs until the row width is reached; pixels
// past the width and trailing bytes are ignored as in drawing.
func runCoverage(s FloorSpec, raw []byte) ([]byte, error) {
	if len(raw) < 17 {
		return nil, errors.New("short record header")
	}
	w, h := int(int32(binary.LittleEndian.Uint32(raw[0:]))), int(int32(binary.LittleEndian.Uint32(raw[4:])))
	off := image.Pt(int(int32(binary.LittleEndian.Uint32(raw[8:]))), int(int32(binary.LittleEndian.Uint32(raw[12:]))))
	if image.Pt(w, h) != s.LogicalSize || off != s.Offset {
		return nil, errors.New("record geometry mismatch")
	}
	cov := make([]byte, w*h)
	pix := raw[17:]
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			if len(pix) < 2 {
				return nil, errors.New("truncated run")
			}
			op, n := pix[0]&0xF, int(pix[1])
			pix = pix[2:]
			var c byte
			switch op {
			case 1:
			case 2, 7:
				c = covOpaque
			case 4:
				c = covIndexed
			case 5:
				c = covAlpha
			case 6:
				return nil, fmt.Errorf("unsupported run op %d", op)
			default:
				return nil, fmt.Errorf("invalid run op %d", op)
			}
			size := 2 * n
			switch op {
			case 1:
				size = 0
			case 4:
				size = n
			}
			if n == 0 || len(pix) < size {
				return nil, errors.New("invalid run length")
			}
			pix = pix[size:]
			for i := x; i < x+n && i < w; i++ {
				cov[y*w+i] = c
			}
			x += n
		}
	}
	return cov, nil
}

// Edge assets (4.3-002, type 1). The PNG alpha of each HD sample selects the
// edge op it takes: 0 keeps the floor below (op 1), 128 copies the edge's
// underlying floor (op 3), 255 is an edge-owned colour (op 2, RGB). A sample
// may use the op of its own logical pixel or of a 4-neighbour, so HD only
// refines the original outline by half a pixel; outside the original rows and
// the tile diamond everything stays keep.
const (
	EdgeKeep  = 1
	EdgeOwn   = 2
	EdgeFloor = 3
)

// EdgeSamples is the size of one EdgeAsset plane (46x46 logical, density 2).
const EdgeSamples = 92 * 92

// EdgeAsset splits ConvertAsset output of a type-1 record into RGB555 colours
// (valid where ops is EdgeOwn) and per-sample ops. ok=false for a wrong size.
func EdgeAsset(pix []uint16) (colors, ops []uint16, ok bool) {
	if len(pix) != 2*EdgeSamples {
		return nil, nil, false
	}
	return pix[:EdgeSamples], pix[EdgeSamples:], true
}

// edgeOps reads a packed edge stream like Tiles.Edge into per logical pixel
// ops of the 46x46 tile square (0 = outside the drawn rows or the diamond).
func edgeOps(raw []byte) ([]byte, error) {
	if len(raw) < 2 || raw[0] > raw[1] || raw[1] >= 46 {
		return nil, errors.New("invalid edge rows")
	}
	ops := make([]byte, 46*46)
	pos := 2
	for y := int(raw[0]); y <= int(raw[1]); y++ {
		start, n, _ := tileRow(y)
		for x := 0; x < n; {
			if pos+2 > len(raw) {
				return nil, errors.New("truncated edge")
			}
			op, count := raw[pos], int(raw[pos+1])
			pos += 2
			if count == 0 || count > n-x || op < EdgeKeep || op > EdgeFloor || (op == EdgeOwn && pos+2*count > len(raw)) {
				return nil, errors.New("invalid edge run")
			}
			for j := 0; j < count; j++ {
				ops[y*46+start+x+j] = op
			}
			if op == EdgeOwn {
				pos += 2 * count
			}
			x += count
		}
	}
	if pos != len(raw) {
		return nil, errors.New("trailing edge data")
	}
	return ops, nil
}

func convertEdge(id int, im image.Image, ops []byte, dst []uint16) error {
	colors, out, _ := EdgeAsset(dst)
	// Outside pixels count as keep for their neighbours.
	opAt := func(x, y int) byte {
		if x < 0 || y < 0 || x >= 46 || y >= 46 || ops[y*46+x] == 0 {
			return EdgeKeep
		}
		return ops[y*46+x]
	}
	b := im.Bounds().Min
	for y := 0; y < 92; y++ {
		for x := 0; x < 92; x++ {
			c := color.NRGBAModel.Convert(im.At(b.X+x, b.Y+y)).(color.NRGBA)
			var op byte
			switch c.A {
			case 0:
				op = EdgeKeep
			case 128:
				op = EdgeFloor
			case 255:
				op = EdgeOwn
			default:
				return fmt.Errorf("asset %d: edge alpha %d at %d,%d", id, c.A, x, y)
			}
			lx, ly := x/2, y/2
			if ops[ly*46+lx] == 0 {
				if op != EdgeKeep {
					return fmt.Errorf("asset %d: edge sample outside original rows at %d,%d", id, x, y)
				}
			} else if op != opAt(lx, ly) && op != opAt(lx-1, ly) && op != opAt(lx+1, ly) && op != opAt(lx, ly-1) && op != opAt(lx, ly+1) {
				return fmt.Errorf("asset %d: edge op %d not at or beside the original at %d,%d", id, op, x, y)
			}
			i := y*92 + x
			out[i], colors[i] = uint16(op), 0
			if op == EdgeOwn {
				colors[i] = uint16(noxcolor.RGB5551Color(c.R, c.G, c.B))
			}
		}
	}
	return nil
}
