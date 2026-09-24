package noxrender

import (
	"encoding/binary"
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/bag"
	"github.com/noxworld-dev/opennox-lib/noximage"
)

var menuTestSizes = map[string]image.Point{
	"MNMchck1": {12, 12}, "MNMchck2": {12, 12}, "MNMchckG": {12, 12},
	"MNMmnup1": {10, 13}, "MNMmnup2": {10, 13}, "MNMmnupG": {10, 13},
}

func menuTestAssets() map[string]*image.NRGBA {
	out := make(map[string]*image.NRGBA)
	for name, sz := range menuTestSizes {
		im := image.NewNRGBA(image.Rectangle{Max: sz.Mul(3)})
		im.SetNRGBA(1, 0, color.NRGBA{R: 255, A: 255})
		im.SetNRGBA(2, 0, color.NRGBA{G: 255, A: 255})
		im.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
		im.SetNRGBA(1, 1, color.NRGBA{A: 255})
		im.SetNRGBA(2, 1, color.NRGBA{R: 203, G: 57, B: 149, A: 255})
		im.SetNRGBA(sz.X*3-1, sz.Y*3-1, color.NRGBA{R: 255, B: 255, A: 255})
		// Distinct state pixels catch a lookup that always selects one asset.
		states := map[string]color.NRGBA{
			"MNMchck1": {R: 255, A: 255}, "MNMchck2": {G: 255, A: 255}, "MNMchckG": {B: 255, A: 255},
			"MNMmnup1": {R: 255, G: 255, A: 255}, "MNMmnup2": {R: 255, B: 255, A: 255}, "MNMmnupG": {G: 255, B: 255, A: 255},
		}
		im.SetNRGBA(3, 1, states[name])
		out[name] = im
	}
	return out
}

func menuTestRegister(t *testing.T, r *NoxRender, assets map[string]*image.NRGBA) error {
	t.Helper()
	setter, ok := any(r).(interface {
		SetHDMenuSprites(map[string]*image.NRGBA) error
	})
	if !ok {
		t.Fatal("renderer has no HD menu sprite registration")
	}
	return setter.SetHDMenuSprites(assets)
}

func menuTestImage(name string, sz image.Point) *Image {
	data := make([]byte, 17)
	binary.LittleEndian.PutUint32(data, uint32(sz.X))
	binary.LittleEndian.PutUint32(data[4:], uint32(sz.Y))
	for y := 0; y < sz.Y; y++ {
		data = append(data, 2, byte(sz.X))
		for x := 0; x < sz.X; x++ {
			data = append(data, 0xff, 0x7f)
		}
	}
	im := &Image{bag: &bag.ImageRec{}, cdata: data}
	im.bag.Name = name
	im.bag.Type = 3
	return im
}

func menuTestBegin(t *testing.T, r *NoxRender, scale int) {
	t.Helper()
	bg := noximage.NewImage16(image.Rectangle{Max: r.pix.Size().Mul(scale)})
	for i := range bg.Pix {
		bg.Pix[i] = 0x4210
	}
	if !r.BeginHDFrame(bg) {
		t.Fatal("begin failed")
	}
}

// Catches partial registration, stale assets after rejection, and input aliasing.
func TestHDMenuRegistration(t *testing.T) {
	for _, mode := range []string{"valid", "nil", "empty", "missing", "extra", "wrong-key", "nil-image", "size", "origin", "alpha", "short-pix", "stride"} {
		t.Run(mode, func(t *testing.T) {
			r := hdTestRender(20, 20)
			if err := menuTestRegister(t, r, menuTestAssets()); err != nil {
				t.Fatal(err)
			}
			assets := menuTestAssets()
			switch mode {
			case "nil":
				assets = nil
			case "empty":
				assets = map[string]*image.NRGBA{}
			case "missing":
				delete(assets, "MNMmnupG")
			case "extra":
				assets["other"] = assets["MNMchck1"]
			case "wrong-key":
				assets["mnmchck1"] = assets["MNMchck1"]
				delete(assets, "MNMchck1")
			case "nil-image":
				assets["MNMmnupG"] = nil
			case "size":
				assets["MNMmnupG"] = image.NewNRGBA(image.Rect(0, 0, 30, 38))
			case "origin":
				assets["MNMmnupG"] = image.NewNRGBA(image.Rect(1, 1, 31, 40))
			case "alpha":
				assets["MNMmnupG"].Pix[3] = 128
			case "short-pix":
				assets["MNMmnupG"].Pix = nil
			case "stride":
				assets["MNMmnupG"].Stride = -1
			}
			err := menuTestRegister(t, r, assets)
			valid := mode == "valid"
			if (err == nil) != (valid || mode == "nil" || mode == "empty") {
				t.Fatalf("unexpected registration result: %v", err)
			}
			if valid {
				assets["MNMchck1"].SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
				delete(assets, "MNMchck1")
			}
			menuTestBegin(t, r, 3)
			r.DrawImage16(menuTestImage("MNMchck1.pcx", image.Pt(12, 12)), image.Point{})
			out := r.EndHDFrame()
			if out == nil {
				t.Fatal("fallback invalidated frame")
			}
			want := uint16(0x7fff)
			if valid {
				want = 0x7c00
			}
			if out.Pix[1] != want {
				t.Fatalf("got %#x want %#x", out.Pix[1], want)
			}
		})
	}
}

// The entire output uses independent RGB5551 literals, including opaque black.
// Transparent HD pixels must not contain the original white sprite underneath.
func TestHDMenuActualImageAndClip(t *testing.T) {
	for name, sz := range menuTestSizes {
		for _, pos := range []image.Point{{2, 2}, {-1, 0}, {19, 19}, {24, 24}} {
			for _, clipped := range []bool{false, true} {
				t.Run(name+pos.String()+map[bool]string{true: "clip", false: "bounds"}[clipped], func(t *testing.T) {
					r, base := hdTestRender(24, 24), hdTestRender(24, 24)
					clip := image.Rect(0, 0, 24, 24)
					if clipped {
						clip = image.Rect(2, 2, 21, 21)
					}
					for _, rr := range []*NoxRender{r, base} {
						rr.p.SetClip(clipped || pos != image.Pt(2, 2))
						rr.p.SetClipRect(clip)
					}
					if err := menuTestRegister(t, r, menuTestAssets()); err != nil {
						t.Fatal(err)
					}
					im := menuTestImage(name+".pcx", sz)
					header := append([]byte(nil), im.cdata...)
					logical, handle := r.pix, im.h
					calls := 0
					r.HookImageDrawXxx = func(image.Point, image.Point) { calls++ }
					menuTestBegin(t, r, 3)
					r.DrawImage16(im, pos)
					base.DrawImage16(im, pos)
					if calls != 1 || r.pix != logical || im.h != handle || !reflect.DeepEqual(im.cdata, header) || !reflect.DeepEqual(r.pix.Pix, base.pix.Pix) {
						t.Fatal("original draw, metadata or logical buffer changed")
					}
					out := r.EndHDFrame()
					if out == nil {
						t.Fatal("valid sprite invalidated frame")
					}
					colors := map[image.Point]uint16{{1, 0}: 0x7c00, {2, 0}: 0x03e0, {0, 1}: 0x001f, {1, 1}: 0, {2, 1}: 0x64f2, sz.Mul(3).Sub(image.Pt(1, 1)): 0x7c1f}
					colors[image.Pt(3, 1)] = map[string]uint16{"MNMchck1": 0x7c00, "MNMchck2": 0x03e0, "MNMchckG": 0x001f, "MNMmnup1": 0x7fe0, "MNMmnup2": 0x7c1f, "MNMmnupG": 0x03ff}[name]
					for y := 0; y < 72; y++ {
						for x := 0; x < 72; x++ {
							want := uint16(0x4210)
							if c, ok := colors[image.Pt(x, y).Sub(pos.Mul(3))]; ok && image.Pt(x/3, y/3).In(clip) {
								want = c
							}
							if got := out.Pix[out.PixOffset(x, y)]; got != want {
								t.Fatalf("(%d,%d)=%#x want %#x", x, y, got, want)
							}
						}
					}
				})
			}
		}
	}
}

func TestHDMenuOriginalPaths(t *testing.T) {
	for _, mode := range []string{"scale1", "inactive", "raw", "no-bag", "wrong-name", "wrong-case", "prefix"} {
		t.Run(mode, func(t *testing.T) {
			r := hdTestRender(20, 20)
			if err := menuTestRegister(t, r, menuTestAssets()); err != nil {
				t.Fatal(err)
			}
			im := menuTestImage("MNMchck1.pcx", image.Pt(12, 12))
			var src Image16 = im
			scale := 3
			switch mode {
			case "scale1":
				scale = 1
			case "raw":
				src = NewRawImage16(3, im.cdata)
			case "no-bag":
				im.bag = nil
				im.typ = 3
			case "wrong-name":
				im.bag.Name = "MNMbase1.pcx"
			case "wrong-case":
				im.bag.Name = "mnmchck1.pcx"
			case "prefix":
				im.bag.Name = "dir/MNMchck1.pcx"
			}
			if mode != "inactive" {
				menuTestBegin(t, r, scale)
			}
			r.DrawImage16(src, image.Point{})
			if r.pix.Pix[0] != 0x7fff {
				t.Fatal("logical original missing")
			}
			out := r.EndHDFrame()
			if mode == "inactive" {
				if out != nil {
					t.Fatal("inactive output")
				}
				return
			}
			if out == nil || out.Pix[0] != 0x7fff || out.Pix[1] != 0x7fff {
				t.Fatal("original span not mirrored")
			}
		})
	}
}

func TestHDMenuInvalidation(t *testing.T) {
	for _, mode := range []string{"type8", "type-flags", "size", "offset-x", "offset-y", "alpha", "multiply", "colorize", "flag16", "interlace", "trim", "op5", "clipped-op5", "hook", "stale"} {
		t.Run(mode, func(t *testing.T) {
			r := hdTestRender(20, 20)
			if err := menuTestRegister(t, r, menuTestAssets()); err != nil {
				t.Fatal(err)
			}
			im := menuTestImage("MNMchck1.pcx", image.Pt(12, 12))
			switch mode {
			case "type8":
				im.bag.Type = 8
			case "type-flags":
				im.bag.Type = 0x43
			case "size":
				im = menuTestImage("MNMchck1.pcx", image.Pt(11, 12))
			case "offset-x":
				binary.LittleEndian.PutUint32(im.cdata[8:], 1)
			case "offset-y":
				binary.LittleEndian.PutUint32(im.cdata[12:], 1)
			case "alpha":
				r.p.SetAlphaEnabled(true)
			case "multiply":
				r.p.SetMultiply14(1)
			case "colorize":
				r.p.SetColorize17(1)
			case "flag16":
				r.p.SetFlag16(true)
			case "interlace":
				r.SetInterlacing(true, 0)
			case "trim":
				r.Set_dword_5d4594_3799484(1)
			case "op5", "clipped-op5":
				im.cdata[17] = 5
			case "hook":
				r.HookImageDrawXxx = func(image.Point, image.Point) { r.InvalidateHDFrame() }
			}
			if mode == "clipped-op5" {
				r.p.SetClip(true)
				r.p.SetClipRect(image.Rect(1, 0, 11, 11))
			}
			menuTestBegin(t, r, 3)
			if mode == "stale" {
				r.InvalidateHDFrame()
			}
			r.DrawImage16(im, image.Point{})
			if r.EndHDFrame() != nil {
				t.Fatal("unsupported/stale frame exposed")
			}
			// Deferred replacement may not write even into an invalid borrowed buffer.
			if r.hd.pix.Pix[1] == 0x7c00 {
				t.Fatal("replacement written after invalidation")
			}
			r.HookImageDrawXxx = nil
			r.SetData(newRenderData(20, 20))
			r.SetInterlacing(false, 0)
			r.Set_dword_5d4594_3799484(0)
			menuTestBegin(t, r, 3)
			r.DrawImage16(menuTestImage("MNMchck1.pcx", image.Pt(12, 12)), image.Point{})
			if out := r.EndHDFrame(); out == nil || out.Pix[1] != 0x7c00 {
				t.Fatal("fresh frame did not recover")
			}
		})
	}
}

func TestHDMenuCheckboxOrderAndSuppressionScope(t *testing.T) {
	r := hdTestRender(20, 20)
	assets := menuTestAssets()
	assets["MNMchck2"].SetNRGBA(1, 0, color.NRGBA{})
	assets["MNMchck2"].SetNRGBA(2, 0, color.NRGBA{B: 255, A: 255})
	if err := menuTestRegister(t, r, assets); err != nil {
		t.Fatal(err)
	}
	menuTestBegin(t, r, 3)
	calls := 0
	r.HookImageDrawXxx = func(image.Point, image.Point) { calls++ }
	r.DrawImage16(menuTestImage("MNMchck1.pcx", image.Pt(12, 12)), image.Point{})
	r.DrawImage16(menuTestImage("MNMchck2.pcx", image.Pt(12, 12)), image.Point{})
	r.DrawImage16(hdTestImage(3, 1, 2, 1, 0xe0, 3), image.Pt(15, 0))
	out := r.EndHDFrame()
	if out == nil || calls != 3 || out.Pix[0] != 0x4210 || out.Pix[1] != 0x7c00 || out.Pix[2] != 0x001f || out.Pix[45] != 0x03e0 {
		t.Fatal("checkbox order, transparency, or scoped suppression changed")
	}
}
