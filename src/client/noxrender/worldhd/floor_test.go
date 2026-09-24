package worldhd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
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

func TestFloorSelectionAndClipping(t *testing.T) {
	s, src, files := fixture(t)
	set, err := LoadFloors(files, []FloorSpec{s}, func(id int) (FloorSource, error) {
		if id != 9166 {
			t.Fatal(id)
		}
		return src, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f := set.Lookup(9166, 0, s.SourceSHA256)
	if f == nil {
		t.Fatal("valid replacement not selected")
	}
	if set.Lookup(9167, 0, s.SourceSHA256) != nil || set.Lookup(9166, 1, s.SourceSHA256) != nil || set.Lookup(9166, 0, [32]byte{}) != nil {
		t.Fatal("unmatched source selected HD")
	}
	// Four different output texels survive within one logical pixel.
	for dy := 0; dy < 2; dy++ {
		for dx := 0; dx < 2; dx++ {
			want := color.NRGBA{R: uint8(40 + dx), G: uint8(40 + dy), B: 17, A: 255}
			if got := f.NRGBAAt(40+dx, 40+dy); got != want {
				t.Fatalf("detail lost: %v != %v", got, want)
			}
		}
	}
	if f.NRGBAAt(0, 0).A != 0 {
		t.Fatal("transparent original became opaque")
	}
	for _, tc := range []struct {
		name           string
		anchor         image.Point
		clip, src, dst image.Rectangle
	}{
		{"full", image.Pt(10, 20), image.Rect(0, 0, 1280, 720), image.Rect(0, 0, 92, 92), image.Rect(20, 40, 112, 132)},
		{"left top", image.Pt(-3, -5), image.Rect(0, 0, 1280, 720), image.Rect(6, 10, 92, 92), image.Rect(0, 0, 86, 82)},
		{"right bottom", image.Pt(1260, 700), image.Rect(0, 0, 1280, 720), image.Rect(0, 0, 40, 40), image.Rect(2520, 1400, 2560, 1440)},
		{"outside", image.Pt(-100, -100), image.Rect(0, 0, 1280, 720), image.Rectangle{}, image.Rectangle{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := f.Clip(tc.anchor, tc.clip)
			if a != tc.src || b != tc.dst {
				t.Fatalf("got %v %v", a, b)
			}
		})
	}
	// Changes to caller-owned metadata and mask cannot mutate the loaded pixels.
	src.Mask.(*image.NRGBA).SetNRGBA(20, 20, color.NRGBA{})
	files[s.Path].Data[0] = 0
	if f.NRGBAAt(40, 40).A != 255 {
		t.Fatal("loaded texture aliases caller")
	}
}

func TestFloorRejectsInvalidSets(t *testing.T) {
	for _, name := range []string{"missing", "source hash", "source type", "source size", "source offset", "HD hash", "decode", "HD size", "mask", "density", "type", "offset", "logical size", "traversal", "windows path", "duplicate", "resolver failure"} {
		t.Run(name, func(t *testing.T) {
			s, src, files := fixture(t)
			var resolveErr error
			specs := []FloorSpec{s}
			replacePNG := func(im image.Image) {
				var b bytes.Buffer
				if err := png.Encode(&b, im); err != nil {
					t.Fatal(err)
				}
				files[s.Path].Data = b.Bytes()
				s.PNGSHA256 = sha256.Sum256(b.Bytes())
			}
			switch name {
			case "missing":
				delete(files, s.Path)
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
				files[s.Path].Data = []byte("not PNG")
				s.PNGSHA256 = sha256.Sum256(files[s.Path].Data)
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
			case "duplicate":
				specs = append(specs, s)
			case "resolver failure":
				resolveErr = fmt.Errorf("missing bag record")
			}
			specs[0] = s
			set, err := LoadFloors(files, specs, func(int) (FloorSource, error) { return src, resolveErr })
			if err == nil || set != nil {
				t.Fatal("invalid set accepted, or partial registration leaked")
			}
			if set.Lookup(9166, 0, s.SourceSHA256) != nil {
				t.Fatal("failed load did not select original")
			}
		})
	}
}

func TestFloorRejectsNearlyOpaque16BitAlpha(t *testing.T) {
	s, src, files := fixture(t)
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
	files[s.Path].Data = b.Bytes()
	s.PNGSHA256 = sha256.Sum256(b.Bytes())
	if set, e := LoadFloors(files, []FloorSpec{s}, func(int) (FloorSource, error) { return src, nil }); e == nil || set != nil {
		t.Fatal("16-bit alpha rounded to 255 and accepted")
	}
}
