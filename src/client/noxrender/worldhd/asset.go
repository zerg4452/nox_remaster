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
// (op 5). HD coverage must equal the original coverage: uncovered pixels need
// alpha 0, opaque ones alpha 255. Indexed (op 4) and op 6 runs are not
// supported yet (4.2-M6b), so such records are rejected and stay original.
// FloorSpec/FloorSource describe any record type here, not only floors.
const MaxAssetPNG = 8 << 20

// AssetPixels is the number of samples ConvertAsset writes for s.
func AssetPixels(s FloorSpec) int {
	return s.LogicalSize.X * s.LogicalSize.Y * s.Density * s.Density
}

// Per logical pixel coverage used for conversion (op&0xF values, 0 = uncovered).
const (
	covNone   = 0
	covOpaque = 2
	covAlpha  = 5
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
			case 5:
				c = covAlpha
			case 4, 6:
				return nil, fmt.Errorf("unsupported run op %d", op)
			default:
				return nil, fmt.Errorf("invalid run op %d", op)
			}
			if n == 0 || (op != 1 && len(pix) < 2*n) {
				return nil, errors.New("invalid run length")
			}
			if op != 1 {
				pix = pix[2*n:]
			}
			for i := x; i < x+n && i < w; i++ {
				cov[y*w+i] = c
			}
			x += n
		}
	}
	return cov, nil
}
