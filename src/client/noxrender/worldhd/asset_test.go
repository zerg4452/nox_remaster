package worldhd

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

// Floors: covered samples are the RGB555 of the PNG texel (the value the former
// NRGBA floor path produced), uncovered samples are 0.
func TestWorldHDConvertAssetMatchesFloorPath(t *testing.T) {
	s, _, files := fixture(t)
	dst := fixtureSamples(t)
	im, err := png.Decode(bytes.NewReader(files[s.Path].Data))
	if err != nil {
		t.Fatal(err)
	}
	for y := 0; y < 92; y++ {
		for x := 0; x < 92; x++ {
			c := im.(*image.NRGBA).NRGBAAt(x, y)
			want := uint16(0)
			if c.A != 0 {
				want = uint16(c.R&248)<<7 | uint16(c.G&248)<<2 | uint16(c.B)>>3
			}
			if dst[y*92+x] != want {
				t.Fatalf("sample %d,%d: %#x != %#x", x, y, dst[y*92+x], want)
			}
		}
	}
}

// spriteRecord: 3x2 RLE, row0 = skip, op2, op5; row1 = op7 x2, skip.
func spriteRecord(ops ...byte) []byte {
	raw := make([]byte, 17)
	binary.LittleEndian.PutUint32(raw[0:], 3)
	binary.LittleEndian.PutUint32(raw[4:], 2)
	binary.LittleEndian.PutUint32(raw[8:], uint32(0xFFFFFFFE)) // offset -2
	binary.LittleEndian.PutUint32(raw[12:], 5)
	if len(ops) == 0 {
		ops = []byte{1, 2, 5, 7, 1}
	}
	counts := []byte{1, 1, 1, 2, 1}
	for i, op := range ops {
		raw = append(raw, op, counts[i])
		if op != 1 {
			raw = append(raw, make([]byte, 2*int(counts[i]))...)
		}
	}
	return raw
}

func spriteFixture(t *testing.T, raw []byte, edit func(*image.NRGBA)) (FloorSpec, []byte, FloorSource) {
	t.Helper()
	cov := [][]byte{{0, 2, 5}, {2, 2, 0}}
	im := image.NewNRGBA(image.Rect(0, 0, 6, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			switch cov[y/2][x/2] {
			case 2:
				im.SetNRGBA(x, y, color.NRGBA{R: uint8(40*x + 7), G: uint8(50*y + 9), B: 200, A: 255})
			case 5:
				im.SetNRGBA(x, y, color.NRGBA{R: 0xA0, G: 0x50, B: 0x30, A: uint8(0x40 * (x - 3 + 2*y))})
			}
		}
	}
	if edit != nil {
		edit(im)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	s := FloorSpec{ID: 14000, Type: 3, SourceSHA256: sha256.Sum256(raw), PNGSHA256: sha256.Sum256(b.Bytes()), Path: "014000.png", LogicalSize: image.Pt(3, 2), Offset: image.Pt(-2, 5), Density: 2}
	return s, b.Bytes(), FloorSource{Type: 3, Raw: raw}
}

func TestWorldHDConvertAssetSpriteRuns(t *testing.T) {
	s, data, src := spriteFixture(t, spriteRecord(), nil)
	dst := make([]uint16, AssetPixels(s))
	if err := ConvertAsset(s, data, src, dst); err != nil {
		t.Fatal(err)
	}
	im, _ := png.Decode(bytes.NewReader(data))
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			c := im.(*image.NRGBA).NRGBAAt(x, y)
			var want uint16
			switch {
			case x/2 == 2 && y/2 == 0: // op5 -> RGBA4444
				want = uint16(c.R&0xf0)<<8 | uint16(c.G&0xf0)<<4 | uint16(c.B&0xf0) | uint16(c.A&0xf0)>>4
			case c.A == 255: // op2/op7 -> RGB555
				want = uint16(c.R&248)<<7 | uint16(c.G&248)<<2 | uint16(c.B)>>3
			}
			if dst[y*6+x] != want {
				t.Fatalf("sample %d,%d: %#x != %#x", x, y, dst[y*6+x], want)
			}
		}
	}
}

func TestWorldHDConvertAssetRejects(t *testing.T) {
	for _, name := range []string{"indexed op4", "op6", "outside coverage", "opaque not opaque", "record size", "record offset", "source hash", "source type", "png hash", "png size", "density", "path", "destination", "type"} {
		t.Run(name, func(t *testing.T) {
			raw := spriteRecord()
			var edit func(*image.NRGBA)
			switch name {
			case "indexed op4":
				raw = spriteRecord(1, 4, 5, 7, 1)
			case "op6":
				raw = spriteRecord(1, 6, 5, 7, 1)
			case "outside coverage":
				edit = func(im *image.NRGBA) { im.SetNRGBA(0, 0, color.NRGBA{A: 1}) }
			case "opaque not opaque":
				edit = func(im *image.NRGBA) { im.SetNRGBA(2, 0, color.NRGBA{R: 1, A: 254}) }
			}
			s, data, src := spriteFixture(t, raw, edit)
			dst := make([]uint16, AssetPixels(s))
			switch name {
			case "record size":
				s.LogicalSize = image.Pt(3, 3)
				dst = make([]uint16, AssetPixels(s))
			case "record offset":
				s.Offset = image.Pt(0, 5)
			case "source hash":
				src.Raw = append(append([]byte(nil), src.Raw...), 0)
			case "source type":
				src.Type = 4
			case "png hash":
				s.PNGSHA256 = [32]byte{}
			case "png size":
				var b bytes.Buffer
				if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 4, 4))); err != nil {
					t.Fatal(err)
				}
				data = b.Bytes()
				s.PNGSHA256 = sha256.Sum256(data)
			case "density":
				s.Density = 1
			case "path":
				s.Path = "../x.png"
			case "destination":
				dst = dst[1:]
			case "type":
				s.Type, src.Type = 9, 9
			}
			if err := ConvertAsset(s, data, src, dst); err == nil {
				t.Fatal("invalid asset accepted")
			}
		})
	}
}

func TestWorldHDAssetIndex(t *testing.T) {
	s, _, _ := fixture(t)
	entry := func(id int) ManifestFloor {
		return ManifestFloor{ID: id, Type: 0, SourceSHA256: fmt.Sprintf("%x", s.SourceSHA256), PNGSHA256: fmt.Sprintf("%x", s.PNGSHA256), File: fmt.Sprintf("%06d.png", id), Width: 46, Height: 46, Density: 2}
	}
	js := func(v any) *fstest.MapFile {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return &fstest.MapFile{Data: b}
	}
	fsys := fstest.MapFS{
		"index.json": js(assetIndex{Version: 2, Shards: []string{"a.json", "sub/b.json"}}),
		"a.json":     js(assetShard{Version: 2, Assets: []ManifestFloor{entry(1), entry(2)}}),
		"sub/b.json": js(assetShard{Version: 2, Assets: []ManifestFloor{entry(3)}}),
	}
	got, err := ReadAssetIndex(fsys)
	if err != nil || len(got) != 3 || got[3].Path != "000003.png" || got[1].SourceSHA256 != s.SourceSHA256 {
		t.Fatalf("v2 index: %v %v", got, err)
	}
	// v1 fallback when index.json is absent.
	v1 := fstest.MapFS{"floors.json": js(Manifest{Version: 1, Floors: []ManifestFloor{entry(9166)}})}
	if got, err := ReadAssetIndex(v1); err != nil || len(got) != 1 || got[9166].ID != 9166 {
		t.Fatalf("v1 fallback: %v %v", got, err)
	}
	for name, edit := range map[string]func(fstest.MapFS){
		"duplicate across shards": func(m fstest.MapFS) { m["sub/b.json"] = js(assetShard{Version: 2, Assets: []ManifestFloor{entry(2)}}) },
		"shard traversal":         func(m fstest.MapFS) { m["index.json"] = js(assetIndex{Version: 2, Shards: []string{"../a.json"}}) },
		"missing shard":           func(m fstest.MapFS) { delete(m, "a.json") },
		"unknown field":           func(m fstest.MapFS) { m["a.json"] = &fstest.MapFile{Data: []byte(`{"version":2,"assets":[],"x":1}`)} },
		"bad fingerprint": func(m fstest.MapFS) {
			m["a.json"] = &fstest.MapFile{Data: []byte(`{"version":2,"assets":[{"id":1,"source_sha256":"bad"}]}`)}
		},
		"index version": func(m fstest.MapFS) { m["index.json"] = js(assetIndex{Version: 1, Shards: []string{"a.json"}}) },
		"empty index":   func(m fstest.MapFS) { m["index.json"] = js(assetIndex{Version: 2}) },
	} {
		t.Run(name, func(t *testing.T) {
			m := fstest.MapFS{}
			for k, v := range fsys {
				m[k] = &fstest.MapFile{Data: append([]byte(nil), v.Data...)}
			}
			edit(m)
			if _, err := ReadAssetIndex(m); err == nil {
				t.Fatal("invalid index accepted")
			}
		})
	}
}
