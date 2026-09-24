package noxrender

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestWorldHDFillMatchesCallback(t *testing.T) {
	for _, rc := range []image.Rectangle{
		image.Rect(1, 1, 4, 2), image.Rect(-2, -1, 3, 2),
		image.Rect(3, 2, 8, 5), image.Rect(-1, -1, 8, 5),
		image.Rect(2, 1, 2, 3), image.Rect(7, 1, 9, 2),
	} {
		for _, value := range []uint16{0, 0x39e7, 0xffff} {
			a, b := hdTestRender(5, 3), hdTestRender(5, 3)
			for _, r := range []*NoxRender{a, b} {
				for i := range r.pix.Pix {
					r.pix.Pix[i] = uint16(i*37 + 9)
				}
				if !r.BeginWorldHDFrame() {
					t.Fatal("begin failed")
				}
				for i := range r.hd.pix.Pix {
					r.hd.pix.Pix[i] = uint16(i*53 + 19)
				}
			}
			logical := append([]uint16(nil), a.pix.Pix...)
			a.hdFillWorldRect(rc, value)
			b.hdRect(rc, func(uint16) uint16 { return value })
			if !reflect.DeepEqual(a.hd.pix.Pix, b.hd.pix.Pix) || !reflect.DeepEqual(a.pix.Pix, logical) {
				t.Fatalf("output or logical buffer changed: rect=%v value=%x", rc, value)
			}
			before := append([]uint16(nil), a.hd.pix.Pix...)
			a.InvalidateHDFrame()
			a.hdFillWorldRect(a.pix.Rect, value^0xffff)
			if !reflect.DeepEqual(a.hd.pix.Pix, before) {
				t.Fatal("inactive HD frame changed")
			}
		}
	}
}

func TestWorldHDHorizontalLineClipping(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		r := hdTestRender(6, 4)
		r.BeginWorldHDFrame()
		for i := range r.hd.pix.Pix {
			r.hd.pix.Pix[i] = uint16(i*71+3) & 0x7fff
		}
		before := append([]uint16(nil), r.hd.pix.Pix...)
		r.Data().SetClip(true)
		// Legacy axis-line clip bounds are inclusive.
		r.Data().SetClipRect2(image.Rect(1, 1, 4, 2))
		r.Data().SetAlphaEnabled(alpha)
		cl := color.RGBA{R: 248, G: 248, B: 248, A: 255}
		r.DrawLineHorizontal(8, 1, -2, cl)
		r.DrawLineHorizontal(0, 3, 5, cl) // fully clipped out
		for y := 0; y < 8; y++ {
			for x := 0; x < 12; x++ {
				i := r.hd.pix.PixOffset(x, y)
				want := before[i]
				if y/2 == 1 && x/2 >= 1 && x/2 <= 4 {
					want = 0x7fff
					if alpha {
						want = SplitColor16(0x7fff).OverAlpha(uint16(r.Data().Alpha()), SplitColor16(before[i])).Make16()
					}
				}
				if r.hd.pix.Pix[i] != want {
					t.Fatalf("alpha=%t pixel=(%d,%d): got %x want %x", alpha, x, y, r.hd.pix.Pix[i], want)
				}
			}
		}
		if r.EndHDFrame() == nil {
			t.Fatal("supported line rejected")
		}
	}
}

func BenchmarkWorldHDOpaqueFill(b *testing.B) {
	for _, callback := range []bool{true, false} {
		name := "direct"
		if callback {
			name = "callback"
		}
		b.Run(name, func(b *testing.B) {
			r := hdTestRender(1280, 1)
			r.BeginWorldHDFrame()
			rc := r.pix.Rect
			op := func(uint16) uint16 { return 0x39e7 }
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if callback {
					r.hdRect(rc, op)
				} else {
					r.hdFillWorldRect(rc, 0x39e7)
				}
			}
		})
	}
}
