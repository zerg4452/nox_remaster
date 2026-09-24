package worldhd

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

func fixture(t *testing.T) (FloorSpec, FloorSource, fstest.MapFS) {
	t.Helper()
	mask := image.NewNRGBA(image.Rect(0, 0, 46, 46))
	hd := image.NewNRGBA(image.Rect(0, 0, 92, 92))
	for y := 0; y < 46; y++ {
		for x := 0; x < 46; x++ {
			if x+y < 12 {
				continue
			}
			mask.SetNRGBA(x, y, color.NRGBA{A: 255})
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					hd.SetNRGBA(2*x+dx, 2*y+dy, color.NRGBA{R: uint8(2*x + dx), G: uint8(2*y + dy), B: 17, A: 255})
				}
			}
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, hd); err != nil {
		t.Fatal(err)
	}
	raw := []byte("fixture raw bag payload")
	s := FloorSpec{ID: 9166, SourceSHA256: sha256.Sum256(raw), PNGSHA256: sha256.Sum256(b.Bytes()), Path: "009166.png", LogicalSize: image.Pt(46, 46), Density: 2}
	return s, FloorSource{Raw: raw, Mask: mask}, fstest.MapFS{s.Path: &fstest.MapFile{Data: b.Bytes()}}
}

// fixtureSamples converts the fixture floor, failing the test on rejection.
func fixtureSamples(t *testing.T) []uint16 {
	t.Helper()
	s, src, files := fixture(t)
	dst := make([]uint16, AssetPixels(s))
	if err := ConvertAsset(s, files[s.Path].Data, src, dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestFloorConvertRejects(t *testing.T) {
	for _, name := range []string{"source hash", "source type", "source size", "source offset", "HD hash", "decode", "HD size", "mask", "density", "type", "offset", "logical size", "traversal", "windows path"} {
		t.Run(name, func(t *testing.T) {
			s, src, files := fixture(t)
			data := files[s.Path].Data
			replacePNG := func(im image.Image) {
				var b bytes.Buffer
				if err := png.Encode(&b, im); err != nil {
					t.Fatal(err)
				}
				data = b.Bytes()
				s.PNGSHA256 = sha256.Sum256(data)
			}
			switch name {
			case "source hash":
				src.Raw = []byte("another bag record")
			case "source type":
				src.Type = 1
			case "source size":
				src.Mask = image.NewNRGBA(image.Rect(0, 0, 45, 46))
			case "source offset":
				src.Offset = image.Pt(1, 0)
			case "HD hash":
				s.PNGSHA256 = [32]byte{}
			case "decode":
				data = []byte("not PNG")
				s.PNGSHA256 = sha256.Sum256(data)
			case "HD size":
				replacePNG(image.NewNRGBA(image.Rect(0, 0, 46, 46)))
			case "mask":
				replacePNG(image.NewNRGBA(image.Rect(0, 0, 92, 92)))
			case "density":
				s.Density = 3
			case "type":
				s.Type = 3
			case "offset":
				s.Offset = image.Pt(1, 0)
			case "logical size":
				s.LogicalSize = image.Pt(92, 92)
			case "traversal":
				s.Path = "../009166.png"
			case "windows path":
				s.Path = "C:\\009166.png"
			}
			dst := make([]uint16, AssetPixels(s))
			if err := ConvertAsset(s, data, src, dst); err == nil {
				t.Fatal("invalid floor accepted")
			}
		})
	}
}

func TestFloorRejectsNearlyOpaque16BitAlpha(t *testing.T) {
	s, src, _ := fixture(t)
	im := image.NewNRGBA64(image.Rect(0, 0, 92, 92))
	for y := 0; y < 92; y++ {
		for x := 0; x < 92; x++ {
			_, _, _, a := src.Mask.At(x/2, y/2).RGBA()
			im.SetNRGBA64(x, y, color.NRGBA64{R: 1234, A: uint16(a)})
		}
	}
	im.SetNRGBA64(40, 40, color.NRGBA64{R: 1234, A: 65534})
	var b bytes.Buffer
	if e := png.Encode(&b, im); e != nil {
		t.Fatal(e)
	}
	s.PNGSHA256 = sha256.Sum256(b.Bytes())
	if e := ConvertAsset(s, b.Bytes(), src, make([]uint16, AssetPixels(s))); e == nil {
		t.Fatal("16-bit alpha rounded to 255 and accepted")
	}
}
