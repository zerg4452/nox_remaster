//go:build !server

package opennox

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
)

var menuUITestSizes = map[string]image.Point{
	"MNMchck1": {36, 36}, "MNMchck2": {36, 36}, "MNMchckG": {36, 36},
	"MNMmnup1": {30, 39}, "MNMmnup2": {30, 39}, "MNMmnupG": {30, 39},
}

func menuUITestFiles(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, sz := range menuUITestSizes {
		im := image.NewNRGBA(image.Rectangle{Max: sz})
		im.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".png"), b.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches accepting an incomplete set or decoding an unbounded/wrong-sized PNG.
func TestMenuHDUIValidation(t *testing.T) {
	for _, mode := range []string{"valid", "missing-folder", "missing-file", "corrupt-header", "corrupt-pixels", "wrong-size", "alpha", "alpha16-low", "alpha16-high"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "menu-hd-ui")
			if mode != "missing-folder" {
				menuUITestFiles(t, dir)
			}
			path := filepath.Join(dir, "MNMmnupG.png")
			switch mode {
			case "missing-file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "corrupt-header":
				if err := os.WriteFile(path, []byte("not PNG"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt-pixels":
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				// Preserve signature and IHDR so DecodeConfig succeeds, Decode fails.
				if err := os.WriteFile(path, data[:33], 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong-size", "alpha":
				sz := image.Pt(30, 39)
				if mode == "wrong-size" {
					sz = image.Pt(30, 38)
				}
				im := image.NewNRGBA(image.Rectangle{Max: sz})
				if mode == "alpha" {
					im.Pix[3] = 127
				}
				var b bytes.Buffer
				if err := png.Encode(&b, im); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			case "alpha16-low", "alpha16-high":
				im := image.NewNRGBA64(image.Rect(0, 0, 30, 39))
				a := uint16(1)
				if mode == "alpha16-high" {
					a = 65534
				}
				im.SetNRGBA64(0, 0, color.NRGBA64{R: 65535, A: a})
				var b bytes.Buffer
				if err := png.Encode(&b, im); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, b.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
			}
			assets, err := loadMenuHDSprites(filepath.Join(root, "background.png"))
			if mode != "valid" {
				if err == nil || assets != nil {
					t.Fatalf("invalid set exposed: count=%d err=%v", len(assets), err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) != 6 {
				t.Fatalf("count=%d", len(assets))
			}
			for name, sz := range menuUITestSizes {
				im := assets[name]
				if im == nil || im.Bounds() != (image.Rectangle{Max: sz}) || im.NRGBAAt(0, 0).A != 0 || im.NRGBAAt(1, 0) != (color.NRGBA{R: 255, A: 255}) {
					t.Fatalf("decoded asset %s differs", name)
				}
			}
			if err := noxrender.NewRender(nil).SetHDMenuSprites(assets); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Exercise the reviewed candidate through the production directory convention
// in a temporary directory; never assume it has been deployed beside a background.
func TestMenuHDUIReviewedCandidate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "menu-hd-ui")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name := range menuUITestSizes {
		path := filepath.Join("..", "..", "..", "assets", "work", "menu-ui-release-01", "review-02", name+".png")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			t.Skip("reviewed assets not present in this source checkout")
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".png"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := loadMenuHDSprites(filepath.Join(root, "background.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 6 {
		t.Fatalf("candidate count=%d", len(assets))
	}
	if err := noxrender.NewRender(nil).SetHDMenuSprites(assets); err != nil {
		t.Fatal(err)
	}
	for _, names := range [][3]string{{"MNMchck1", "MNMchck2", "MNMchckG"}, {"MNMmnup1", "MNMmnup2", "MNMmnupG"}} {
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				if reflect.DeepEqual(assets[names[i]].Pix, assets[names[j]].Pix) {
					t.Fatalf("states %s and %s identical", names[i], names[j])
				}
			}
		}
	}
}

func TestMenuHDBackgroundValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "background.png")
	if _, err := loadMenuHDBackground(path); err == nil {
		t.Fatal("missing file accepted")
	}
	for _, tc := range []struct {
		w, h   int
		opaque bool
		valid  bool
	}{
		{640, 480, true, false}, {1920, 1440, false, false}, {1920, 1440, true, true},
	} {
		im := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
		if tc.opaque {
			for i := 3; i < len(im.Pix); i += 4 {
				im.Pix[i] = 255
			}
		}
		im.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, im); err != nil {
			t.Fatal(err)
		}
		f.Close()
		got, err := loadMenuHDBackground(path)
		if tc.valid {
			if err != nil {
				t.Fatal(err)
			}
			if got.Pix[0] != 0x7c00 {
				t.Fatal("color conversion changed")
			}
		} else if err == nil {
			t.Fatal("invalid background accepted")
		}
	}
}

func TestMenuHDFrameLifetime(t *testing.T) {
	r := noxrender.NewRender(nil)
	d, free := noxrender.NewRenderData()
	defer free()
	r.SetData(d)
	r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, 640, 480)))
	m := menuHDTarget{background: noximage.NewImage16(image.Rect(0, 0, 1920, 1440))}
	for _, tc := range []struct {
		id                int
		pos               image.Point
		menu, edge, valid bool
	}{
		{98, image.Point{}, true, false, true}, {99, image.Point{}, true, false, false},
		{98, image.Pt(1, 0), true, false, false}, {98, image.Point{}, false, false, false},
		{98, image.Point{}, true, true, false},
	} {
		m.begin(r)
		m.drawBackground(r, tc.id, tc.pos, tc.menu)
		m.end(r, tc.edge)
		if (m.take() != nil) != tc.valid {
			t.Fatalf("frame eligibility %+v", tc)
		}
		if m.take() != nil {
			t.Fatal("frame consumed twice")
		}
	}
	m.begin(r)
	m.drawBackground(r, 98, image.Point{}, true)
	m.end(r, false)
	m.begin(r)
	m.end(r, false)
	if m.take() != nil {
		t.Fatal("stale previous background presented")
	}
	m.drawBackground(r, 98, image.Point{}, true)
	m.end(r, false)
	if m.take() != nil {
		t.Fatal("outside-frame callback accepted")
	}
}
