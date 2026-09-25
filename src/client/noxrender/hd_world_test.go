package noxrender

import (
	"image"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

func TestWorldHDDetailAndOpaqueOcclusion(t *testing.T) {
	r := hdTestRender(3, 2)
	logical := r.PixBuffer()
	logical.Pix[0] = 42
	if !r.BeginWorldHDFrame() {
		t.Fatal("begin failed")
	}
	samples := [4]uint16{1, 2, 3, 4}
	r.WorldFloorPixel(logical.Pix[:1], samples, true)
	if logical.Pix[0] != 42 {
		t.Fatal("logical pixels changed")
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if r.hd.pix.Pix[r.hd.pix.PixOffset(x, y)] != samples[y*2+x] {
				t.Fatal("detail lost")
			}
		}
	}
	logical.Pix[0] = 9
	r.MirrorWorldOpaque(logical.Pix[:1])
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			if r.hd.pix.Pix[r.hd.pix.PixOffset(x, y)] != 9 {
				t.Fatal("wall did not occlude floor")
			}
		}
	}
	r.PixBuffer()
	if r.EndHDFrame() != nil {
		t.Fatal("unhandled write did not veto HD")
	}
	r.BeginWorldHDFrame()
	r.RejectWorldHD("unsupported")
	r.BeginWorldHDFrame()
	if ok, _, reason := r.WorldHDStatus(); !ok || reason != "" {
		t.Fatal("old fallback leaked")
	}
}

func TestWorldFloorSpanMatchesPixelWrites(t *testing.T) {
	perPixel, perSpan := hdTestRender(5, 3), hdTestRender(5, 3)
	for _, r := range []*NoxRender{perPixel, perSpan} {
		for i := range r.pix.Pix {
			r.pix.Pix[i] = uint16(i*13 + 7)
		}
		if !r.BeginWorldHDFrame() {
			t.Fatal("begin failed")
		}
	}
	a := perPixel.pix.Row(1)[1:4]
	b := perSpan.pix.Row(1)[1:4]
	span := perSpan.BeginWorldFloorSpan(b)
	for i := range a {
		samples := [4]uint16{uint16(i + 1), uint16(i + 11), uint16(i + 21), uint16(i + 31)}
		perPixel.WorldFloorPixel(a[i:i+1], samples, i%2 == 0)
		span.Set(i, samples, i%2 == 0)
	}
	if !reflect.DeepEqual(perPixel.pix.Pix, perSpan.pix.Pix) || !reflect.DeepEqual(perPixel.hd.pix.Pix, perSpan.hd.pix.Pix) || perPixel.hd.detail != perSpan.hd.detail {
		t.Fatal("span changed logical pixels, HD output, or detail count")
	}
	perSpan.BeginWorldFloorSpan(perSpan.pix.Pix[4:6])
	if out := perSpan.EndHDFrame(); out != nil {
		t.Fatal("cross-row span did not reject HD")
	}
	r := hdTestRender(2, 1)
	if !r.BeginWorldHDFrame() {
		t.Fatal("begin failed")
	}
	valid := r.BeginWorldFloorSpan(r.pix.Row(0))
	before := append([]uint16(nil), r.hd.pix.Pix...)
	valid.Set(2, [4]uint16{1, 2, 3, 4}, true)
	valid.Set(0, [4]uint16{5, 6, 7, 8}, true)
	if out := r.EndHDFrame(); out != nil || !reflect.DeepEqual(before, r.hd.pix.Pix) {
		t.Fatal("rejected span still produced or changed HD output")
	}
}

func TestWorldFloorSpanRowsMatchSet(t *testing.T) {
	perSet, perRows := hdTestRender(5, 3), hdTestRender(5, 3)
	for _, r := range []*NoxRender{perSet, perRows} {
		for i := range r.pix.Pix {
			r.pix.Pix[i] = uint16(i*13 + 7)
		}
		if !r.BeginWorldHDFrame() {
			t.Fatal("begin failed")
		}
	}
	a := perSet.BeginWorldFloorSpan(perSet.pix.Row(1)[1:4])
	b := perRows.BeginWorldFloorSpan(perRows.pix.Row(1)[1:4])
	top, bottom, ok := b.Rows()
	if !ok || len(top) != 6 || len(bottom) != 6 {
		t.Fatal("valid span rows unavailable")
	}
	detail := 0
	for i := 0; i < 3; i++ {
		samples := [4]uint16{uint16(i + 1), uint16(i + 11), uint16(i + 21), uint16(i + 31)}
		a.Set(i, samples, i%2 == 0)
		top[i*2], top[i*2+1] = samples[0], samples[1]
		bottom[i*2], bottom[i*2+1] = samples[2], samples[3]
		if i%2 == 0 {
			detail++
		}
	}
	b.AddDetail(detail)
	if !reflect.DeepEqual(perSet.pix.Pix, perRows.pix.Pix) || !reflect.DeepEqual(perSet.hd.pix.Pix, perRows.hd.pix.Pix) || perSet.hd.detail != perRows.hd.detail {
		t.Fatal("rows writer changed logical pixels, HD output, or detail count")
	}
	rejected := perRows.BeginWorldFloorSpan(perRows.pix.Pix[4:6])
	if _, _, ok := rejected.Rows(); ok {
		t.Fatal("cross-row span exposed rows")
	}
	if _, _, ok := b.Rows(); ok {
		t.Fatal("span exposed rows after the frame was rejected")
	}
}

// Map loads switch between the menu and world targets; each keeps its buffer.
func TestWorldHDBuffersReusedAcrossModes(t *testing.T) {
	r := hdTestRender(4, 3)
	for i := range r.pix.Pix {
		r.pix.Pix[i] = uint16(i + 1)
	}
	bg := noximage.NewImage16(image.Rect(0, 0, 12, 9))
	for i := range bg.Pix {
		bg.Pix[i] = 0x7000 + uint16(i)
	}
	var world, menu *noximage.Image16
	for i := 0; i < 3; i++ {
		if !r.BeginWorldHDFrame() {
			t.Fatal("world begin failed")
		}
		out := r.EndHDFrame()
		if world != nil && out != world {
			t.Fatal("world target reallocated")
		}
		world = out
		if out.Pix[out.PixOffset(3, 1)] != r.pix.Pix[r.pix.PixOffset(1, 0)] {
			t.Fatal("reused world target not refreshed from the logical buffer")
		}
		if !r.BeginHDFrame(bg) {
			t.Fatal("menu begin failed")
		}
		out = r.EndHDFrame()
		if menu != nil && out != menu {
			t.Fatal("menu target reallocated")
		}
		menu = out
		if out == world || !reflect.DeepEqual(out.Pix, bg.Pix) {
			t.Fatal("menu target shared with world or not refreshed")
		}
		r.pix.Pix[r.pix.PixOffset(1, 0)] += 3
	}
}

func TestWorldHDInitialCopyAllRows(t *testing.T) {
	r := hdTestRender(7, 3)
	logical := r.PixBuffer()
	for i := range logical.Pix {
		logical.Pix[i] = uint16(i*17 + 1)
	}
	if !r.BeginWorldHDFrame() {
		t.Fatal("begin failed")
	}
	for y := 0; y < 6; y++ {
		for x := 0; x < 14; x++ {
			if got, want := r.hd.pix.Row(y)[x], logical.Row(y / 2)[x/2]; got != want {
				t.Fatalf("initial copy (%d,%d): got %d want %d", x, y, got, want)
			}
		}
	}
}

func TestWorldHDSpriteOperations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		typ    int
		stream []byte
	}{
		{"opaque", 3, []byte{2, 2, 31, 0, 0, 124}},
		{"indexed", 4, []byte{4, 2, 128, 255}},
		{"translucent", 5, []byte{5, 2, 0xff, 0xf8, 0, 0x88}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, clip := range []bool{false, true} {
				a := hdTestRender(3, 2)
				b := hdTestRender(3, 2)
				for _, r := range []*NoxRender{a, b} {
					for i := range r.pix.Pix {
						r.pix.Pix[i] = 0x1234
					}
					r.Data().SetMaterial(0, image.White)
					if clip {
						r.Data().SetClip(true)
						r.Data().SetClipRect(image.Rect(1, 0, 3, 2))
					}
				}
				a.BeginWorldHDFrame()
				im := hdTestImage(tc.typ, 2, tc.stream...)
				a.DrawImage16(im, image.Point{})
				b.DrawImage16(im, image.Point{})
				if !reflect.DeepEqual(a.pix.Pix, b.pix.Pix) {
					t.Fatal("logical state changed")
				}
				out := a.EndHDFrame()
				if out == nil {
					t.Fatal("supported operation fell back")
				}
				for y := 0; y < 2; y++ {
					for x := 0; x < 3; x++ {
						for dy := 0; dy < 2; dy++ {
							for dx := 0; dx < 2; dx++ {
								if out.Pix[out.PixOffset(2*x+dx, 2*y+dy)] != b.pix.Pix[b.pix.PixOffset(x, y)] {
									t.Fatal("sprite output mismatch", x, y, dx, dy)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestWorldHDPrimitiveOperations(t *testing.T) {
	for _, draw := range []func(*NoxRender){
		func(r *NoxRender) { r.DrawLine(image.Pt(1, 1), image.Pt(6, 5), image.White) },
		func(r *NoxRender) { r.DrawLineAlpha(image.Pt(1, 1), image.Pt(5, 6), image.White) },
		func(r *NoxRender) { r.DrawCircleOpaque(4, 4, 2, image.White) },
		func(r *NoxRender) { r.DrawCircleAlpha(4, 4, 2, image.White) },
		func(r *NoxRender) { r.drawRectFilledAlpha(1, 1, 4, 4) },
	} {
		a, b := hdTestRender(8, 8), hdTestRender(8, 8)
		for _, r := range []*NoxRender{a, b} {
			for i := range r.pix.Pix {
				r.pix.Pix[i] = 0x5678
			}
		}
		a.BeginWorldHDFrame()
		draw(a)
		draw(b)
		if !reflect.DeepEqual(a.pix.Pix, b.pix.Pix) {
			t.Fatal("logical primitive changed")
		}
		out := a.EndHDFrame()
		if out == nil {
			t.Fatal("supported primitive rejected")
		}
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				for dy := 0; dy < 2; dy++ {
					for dx := 0; dx < 2; dx++ {
						if out.Pix[out.PixOffset(x*2+dx, y*2+dy)] != b.pix.Pix[b.pix.PixOffset(x, y)] {
							t.Fatal("primitive mismatch", x, y)
						}
					}
				}
			}
		}
	}
}
