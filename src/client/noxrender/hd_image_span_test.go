package noxrender

import (
	"fmt"
	"image"
	"reflect"
	"testing"
)

// The pre-batching implementation is the reference for all blend operations.
func worldImageSpanScalar(r *NoxRender, pos image.Point, src []byte, n int, fn drawOp16Func) {
	scale := r.hd.scale
	for y := 0; y < scale; y++ {
		row := r.hd.pix.Row(pos.Y*scale + y)
		for x := 0; x < n; x++ {
			for dx := 0; dx < scale; dx++ {
				_, _ = fn(row[(pos.X+x)*scale+dx:], src[2*x:], 1)
			}
		}
	}
}

func TestWorldHDImageSpanBlendEquivalence(t *testing.T) {
	ops := []struct {
		name string
		fn   func(*NoxRender) drawOp16Func
	}{
		{"opaque", func(r *NoxRender) drawOp16Func { return pixOpSrc }},
		{"multiply", func(r *NoxRender) drawOp16Func { return r.pixOpSrcMultiply }},
		{"alpha50", func(r *NoxRender) drawOp16Func { return r.pixOpOverAlpha50 }},
		{"alpha", func(r *NoxRender) drawOp16Func { return r.pixOpOverAlpha }},
		{"multiply-alpha50", func(r *NoxRender) drawOp16Func { return r.pixOpOverMultiplyAlpha50 }},
		{"multiply-alpha", func(r *NoxRender) drawOp16Func { return r.pixOpOverMultiplyAlpha }},
		{"4444", func(r *NoxRender) drawOp16Func { return r.pixOpOver4444 }},
		{"4444-multiply", func(r *NoxRender) drawOp16Func { return r.pixOpOver4444Multiply }},
		{"4444-alpha", func(r *NoxRender) drawOp16Func { return r.pixOpOver4444Alpha }},
		{"colorize", func(r *NoxRender) drawOp16Func { return r.pixOpSrcColorize }},
		{"premult", func(r *NoxRender) drawOp16Func { return r.pixBlendPremult }},
	}
	for _, op := range ops {
		for _, alpha := range []byte{0, 127, 255} {
			t.Run(fmt.Sprintf("%s/alpha%d", op.name, alpha), func(t *testing.T) {
				a, b := hdTestRender(130, 3), hdTestRender(130, 3)
				for _, r := range []*NoxRender{a, b} {
					r.Data().SetAlpha(alpha)
					r.Data().SetColorMultA(Color16{R: 173, G: 89, B: 231})
					r.colors.revTable = make([]byte, 1<<16)
					for i := range r.colors.revTable {
						r.colors.revTable[i] = byte(i*17 + 11)
					}
					for i := range r.pix.Pix {
						r.pix.Pix[i] = uint16(i*41 + 3)
					}
					if !r.BeginWorldHDFrame() {
						t.Fatal("begin failed")
					}
					// Different backgrounds at every subpixel catch copying one
					// blended result over the other density samples.
					for i := range r.hd.pix.Pix {
						r.hd.pix.Pix[i] = uint16(i*73+19) & 0x7fff
					}
				}
				logical := append([]uint16(nil), a.pix.Pix...)
				// Grow and then reuse scratch storage for shorter spans.
				for _, n := range []int{1, 3, 23, 127, 2} {
					src := make([]byte, n*2+4)
					for i := range src {
						src[i] = byte(i*53 + n)
					}
					before := append([]byte(nil), src...)
					pos := image.Pt(2, 1)
					a.hdImageSpan(pos, src, n, op.fn(a))
					worldImageSpanScalar(b, pos, src, n, op.fn(b))
					if !reflect.DeepEqual(a.hd.pix.Pix, b.hd.pix.Pix) {
						t.Fatalf("HD output differs for span length %d", n)
					}
					if !reflect.DeepEqual(src, before) || !reflect.DeepEqual(a.pix.Pix, logical) {
						t.Fatal("source or logical buffer changed")
					}
				}
			})
		}
	}
}

// The texel-reuse loops must match the previous per-pixel closure formulas,
// including repeated texels over different destinations and a span restart.
func TestWorldHDPixOpTexelReuseEquivalence(t *testing.T) {
	ops := []struct {
		name string
		fn   func(*NoxRender) drawOp16Func
		ref  func(mul Color16) drawU16Func
	}{
		{"multiply", func(r *NoxRender) drawOp16Func { return r.pixOpSrcMultiply }, func(mul Color16) drawU16Func {
			return func(_ uint16, c2 uint16) uint16 { return SplitColor16(c2).Mult(mul).Make16() }
		}},
		{"multiply-alpha50", func(r *NoxRender) drawOp16Func { return r.pixOpOverMultiplyAlpha50 }, func(mul Color16) drawU16Func {
			return func(old uint16, c2 uint16) uint16 { return SplitColor16(c2).Mult(mul).Over(SplitColor16(old)).Make16() }
		}},
	}
	// Every texel appears twice in a row, like a density-2 expanded span.
	const n = 1 << 17
	src := make([]byte, n*2)
	for i := 0; i < n; i++ {
		c := uint16(i / 2)
		src[2*i], src[2*i+1] = byte(c), byte(c>>8)
	}
	for _, op := range ops {
		for _, mul := range []Color16{{R: 173, G: 89, B: 231}, {R: 255, G: 255, B: 255}, {R: 0, G: 128, B: 1}} {
			r := hdTestRender(1, 1)
			r.Data().SetColorMultA(mul)
			got := make([]uint16, n)
			for i := range got {
				got[i] = uint16(i*73 + 19)
			}
			want := append([]uint16(nil), got...)
			// Split once in the middle of a repeated pair to exercise a fresh cache.
			d, s := op.fn(r)(got, src, 3)
			op.fn(r)(d, s, n-3)
			r.drawOpU16(want, src, n, op.ref(mul))
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s mul=%v differs from per-pixel reference", op.name, mul)
			}
		}
	}
}

func BenchmarkWorldHDImageSpan(b *testing.B) {
	for _, scalar := range []bool{true, false} {
		name := "batched"
		if scalar {
			name = "scalar"
		}
		b.Run(name, func(b *testing.B) {
			const n = 127
			r := hdTestRender(n, 1)
			r.Data().SetColorMultA(Color16{R: 173, G: 89, B: 231})
			r.BeginWorldHDFrame()
			src := make([]byte, n*2)
			for i := range src {
				src[i] = byte(i*53 + 7)
			}
			fn := r.pixOpOverMultiplyAlpha50
			r.hdImageSpan(image.Point{}, src, n, fn)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if scalar {
					worldImageSpanScalar(r, image.Point{}, src, n, fn)
				} else {
					r.hdImageSpan(image.Point{}, src, n, fn)
				}
			}
		})
	}
}
