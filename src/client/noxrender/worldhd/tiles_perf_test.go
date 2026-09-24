package worldhd

import "testing"

// The pre-optimization expressions are the compatibility reference, including
// uint32 wraparound for signed/out-of-range light values.
func legacyTileLight(c uint16, r, g, b int) uint16 {
	rr := uint16((uint32(r)*uint32((c>>7)&248))>>16) & 248
	gg := uint16((uint32(g)*uint32((c>>2)&248))>>16) & 248
	bb := uint16((uint32(b)*uint32(c&31))>>13) & 248
	return rr<<7 | gg<<2 | bb>>3
}

func legacyTilePixel(t *Tiles, index int, original uint16) ([4]uint16, bool) {
	if !t.Ready() {
		return [4]uint16{original, original, original, original}, false
	}
	i := index % len(t.pix)
	if i < 0 {
		i += len(t.pix)
	}
	if !t.valid[i] {
		return [4]uint16{original, original, original, original}, false
	}
	return t.pix[i], t.detail[i]
}

func TestTilesLightLegacyEquivalence(t *testing.T) {
	lights := [][3]int{
		{0, 0, 0}, {1, 255, 256}, {4095, 4096, 8191},
		{32767, 32768, 32769}, {65535, 65536, 65537},
		{131071, 100003, 98765}, {-1, -65536, -1234567},
		{0x7fffffff, -0x80000000, 0x12345678},
		{0x01000001, 0x20000001, 0x40000001},
	}
	// Every 16-bit input, including the otherwise ignored high bit.
	for _, light := range lights {
		for c := 0; c <= 0xffff; c++ {
			got := Light(uint16(c), light[0], light[1], light[2])
			want := legacyTileLight(uint16(c), light[0], light[1], light[2])
			if got != want {
				t.Fatalf("color=%x light=%v: got %x want %x", c, light, got, want)
			}
		}
	}
	// Vary the fixed-point fractional bits as well as channel intensities.
	for light := -65537; light <= 131073; light++ {
		for channel := 0; channel < 32; channel++ {
			c := uint16(channel<<10 | ((channel+11)&31)<<5 | ((channel + 23) & 31))
			if Light(c, light, light+17, light-29) != legacyTileLight(c, light, light+17, light-29) {
				t.Fatalf("fractional light mismatch: %x/%d", c, light)
			}
		}
	}
}

func TestTilesPixelFastIndexEquivalence(t *testing.T) {
	for _, size := range []int{1, 7, 64} {
		ring := NewTiles(size, 3)
		for i := range ring.pix {
			ring.pix[i] = [4]uint16{uint16(i), uint16(i + 37), uint16(i + 91), uint16(i + 151)}
			ring.valid[i] = i%3 != 0
			ring.detail[i] = i%2 == 0
		}
		check := func(index int) {
			t.Helper()
			got, detail := ring.Pixel(index, 0x739a)
			want, wantDetail := legacyTilePixel(ring, index, 0x739a)
			if got != want || detail != wantDetail {
				t.Fatalf("size=%d index=%d: got %v/%t want %v/%t", size, index, got, detail, want, wantDetail)
			}
		}
		for i := -3*len(ring.pix) - 1; i <= 3*len(ring.pix)+1; i++ {
			check(i)
		}
		check(-0x80000000)
		check(0x7fffffff)
		ring.Invalidate()
		check(0)
		check(-1)
	}
	var ring *Tiles
	if got, detail := ring.Pixel(3, 19); detail || got != [4]uint16{19, 19, 19, 19} {
		t.Fatal("nil ring fallback changed")
	}
}

var tileLightingBenchmarkSink uint16

func BenchmarkTilesLookupAndLight(b *testing.B) {
	for _, optimized := range []bool{false, true} {
		name := "legacy"
		if optimized {
			name = "optimized"
		}
		b.Run(name, func(b *testing.B) {
			ring := NewTiles(1280, 2)
			for i := range ring.pix {
				ring.pix[i] = [4]uint16{uint16(i * 11), uint16(i * 23), uint16(i * 37), uint16(i * 53)}
				ring.valid[i], ring.detail[i] = true, true
			}
			var total uint16
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for x := 0; x < 1280; x++ {
					var samples [4]uint16
					if optimized {
						samples, _ = ring.Pixel(x, 0)
					} else {
						samples, _ = legacyTilePixel(ring, x, 0)
					}
					for _, c := range samples {
						if optimized {
							total ^= Light(c, 20000+x*17, 23000+x*13, 28000+x*7)
						} else {
							total ^= legacyTileLight(c, 20000+x*17, 23000+x*13, 28000+x*7)
						}
					}
				}
			}
			tileLightingBenchmarkSink = total
		})
	}
}
