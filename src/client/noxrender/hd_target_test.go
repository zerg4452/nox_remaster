package noxrender

import (
	"encoding/binary"
	"image"
	"reflect"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"

	"github.com/noxworld-dev/opennox/v1/legacy/common/alloc"
)

func hdTestImage(typ, width int, stream ...byte) Image16 {
	data := make([]byte, 17+len(stream))
	binary.LittleEndian.PutUint32(data, uint32(width))
	binary.LittleEndian.PutUint32(data[4:], 1)
	copy(data[17:], stream)
	return NewRawImage16(typ, data)
}

func hdTestRender(w, h int) *NoxRender {
	r := NewRender(nil)
	r.SetData(newRenderData(w, h))
	r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, w, h)))
	return r
}

// Catches accidental logical-buffer replacement, asset mutation and lost detail.
func TestHDFramePreservesBuffers(t *testing.T) {
	r := hdTestRender(2, 1)
	logical := r.PixBuffer()
	logical.Pix[0] = 0x7fff
	bg := noximage.NewImage16(image.Rect(0, 0, 6, 3))
	bg.Pix[0], bg.Pix[1], bg.Pix[2] = 0x7c00, 0x001f, 0x03e0
	want := append([]uint16(nil), bg.Pix...)
	if !r.BeginHDFrame(bg) {
		t.Fatal("valid 3x background rejected")
	}
	out := r.EndHDFrame()
	if out == nil || !reflect.DeepEqual(out.Pix, want) {
		t.Fatal("HD background detail lost")
	}
	out.Pix[0] = 0
	if !reflect.DeepEqual(bg.Pix, want) {
		t.Fatal("output aliases background asset")
	}
	if r.PixBuffer() != logical || logical.Pix[0] != 0x7fff {
		t.Fatal("logical buffer changed")
	}
	if r.EndHDFrame() != nil {
		t.Fatal("ended frame exposed twice")
	}
}

// Catches stale output after failed starts and incorrect dimension acceptance.
func TestHDFrameRejectsInvalidBackground(t *testing.T) {
	for _, rc := range []image.Rectangle{
		{}, image.Rect(0, 0, 4, 2), image.Rect(0, 0, 6, 2), image.Rect(1, 0, 7, 3),
	} {
		r := hdTestRender(2, 1)
		if !r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 2, 1))) {
			t.Fatal("valid 1x background rejected")
		}
		if r.BeginHDFrame(noximage.NewImage16(rc)) || r.EndHDFrame() != nil {
			t.Fatalf("invalid background %v retained output", rc)
		}
	}
	r := hdTestRender(2, 1)
	if r.BeginHDFrame(nil) || r.EndHDFrame() != nil {
		t.Fatal("nil background accepted")
	}
	if NewRender(nil).BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 1, 1))) {
		t.Fatal("missing logical buffer accepted")
	}
}

func TestHDFrameInvalidationAndBufferReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		r := hdTestRender(2, 1)
		bg := noximage.NewImage16(image.Rect(0, 0, 6, 3))
		if !r.BeginHDFrame(bg) {
			t.Fatal("valid background rejected")
		}
		if replace {
			r.SetPixBuffer(noximage.NewImage16(image.Rect(0, 0, 2, 1)))
		} else {
			r.InvalidateHDFrame()
		}
		if r.EndHDFrame() != nil {
			t.Fatal("invalidated frame still available")
		}
		bg.Pix[1] = 0x001f
		if !r.BeginHDFrame(bg) {
			t.Fatal("could not start fresh frame")
		}
		if out := r.EndHDFrame(); out == nil || out.Pix[1] != 0x001f {
			t.Fatal("fresh frame retained old data")
		}
	}
}

// Catches color-key reconstruction (black is opaque) and clipping drift.
func TestHDImageOpaqueSkipAndClip(t *testing.T) {
	for _, tc := range []struct {
		x    int
		want []uint16
	}{
		{0, []uint16{0, 0x7fff, 0x001f}},
		{-1, []uint16{0x7fff, 0x001f, 0x7fff}},
		{1, []uint16{0x7fff, 0, 0x7fff}},
	} {
		for _, scale := range []int{1, 3} {
			r := hdTestRender(3, 1)
			r.Data().SetClip(true)
			r.Data().SetClipRect(image.Rect(0, 0, 3, 1))
			for i := range r.pix.Pix {
				r.pix.Pix[i] = 0x7fff
			}
			bg := noximage.NewImage16(image.Rect(0, 0, 3*scale, scale))
			for i := range bg.Pix {
				bg.Pix[i] = 0x7fff
			}
			if !r.BeginHDFrame(bg) {
				t.Fatal("start failed")
			}
			r.DrawImage16(hdTestImage(3, 3, 2, 1, 0, 0, 1, 1, 2, 1, 0x1f, 0), image.Pt(tc.x, 0))
			out := r.EndHDFrame()
			if out == nil {
				t.Fatal("supported image invalidated frame")
			}
			if !reflect.DeepEqual(r.pix.Pix, tc.want) {
				t.Fatalf("logical pixels: %v", r.pix.Pix)
			}
			for y := 0; y < scale; y++ {
				for x := 0; x < 3*scale; x++ {
					if got := out.Pix[out.PixOffset(x, y)]; got != tc.want[x/scale] {
						t.Fatalf("scale=%d offset=%d (%d,%d)=%#x want=%#x", scale, tc.x, x, y, got, tc.want[x/scale])
					}
				}
			}
		}
	}
}

// Catches applying the logical result to all subpixels instead of blending
// with each subpixel's own background, and wrapping instead of saturating.
func TestHDImageParticleUsesSubpixelBackground(t *testing.T) {
	r := hdTestRender(1, 1)
	bg := noximage.NewImage16(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		copy(bg.Row(y), []uint16{0, 0x001f, 0x7c00})
	}
	if !r.BeginHDFrame(bg) {
		t.Fatal("start failed")
	}
	r.DrawImage16(hdTestImage(8, 1, 7, 1, 0, 0xf8), image.Point{})
	out := r.EndHDFrame()
	if out == nil {
		t.Fatal("particle invalidated frame")
	}
	for y := 0; y < 3; y++ {
		if !reflect.DeepEqual(out.Row(y), []uint16{0x7c00, 0x7c1f, 0x7c00}) {
			t.Fatalf("particle row %d: %v", y, out.Row(y))
		}
	}
	if r.pix.Pix[0] != 0x7c00 {
		t.Fatal("logical particle changed")
	}
}

func TestHDImageRejectsUnsupportedState(t *testing.T) {
	for _, mode := range []string{"type", "alpha", "multiply", "colorize", "interlace", "op5"} {
		t.Run(mode, func(t *testing.T) {
			r := hdTestRender(1, 1)
			img := hdTestImage(3, 1, 2, 1, 0, 0x7c)
			switch mode {
			case "type":
				img = NewRawImage16(63, nil)
			case "alpha":
				r.Data().SetAlphaEnabled(true)
			case "multiply":
				r.Data().SetMultiply14(1)
			case "colorize":
				r.Data().SetColorize17(1)
			case "interlace":
				r.interlacing = true
			case "op5":
				img = hdTestImage(3, 1, 5, 1, 0xff, 0xff)
			}
			if !r.BeginHDFrame(noximage.NewImage16(image.Rect(0, 0, 3, 3))) {
				t.Fatal("start failed")
			}
			r.DrawImage16(img, image.Point{})
			if r.EndHDFrame() != nil {
				t.Fatal("unsupported image exposed HD output")
			}
		})
	}
}

// Catches HD targets left on the Go heap, leaked when their size changes, or
// reallocated when the size is unchanged.
func TestResizeHDTargetCHeap(t *testing.T) {
	count0, bytes0 := alloc.Stats()
	a := resizeHDTarget(nil, image.Rect(0, 0, 6, 3))
	if count, bytes := alloc.Stats(); count != count0+1 || bytes != bytes0+6*3*2 {
		t.Fatalf("first target not on the C heap: count %d->%d bytes %d->%d", count0, count, bytes0, bytes)
	}
	if b := resizeHDTarget(a, a.Rect); b != a {
		t.Fatal("same-size target reallocated")
	}
	b := resizeHDTarget(a, image.Rect(0, 0, 4, 2))
	if count, bytes := alloc.Stats(); count != count0+1 || bytes != bytes0+4*2*2 {
		t.Fatalf("old target not freed: count %d->%d bytes %d->%d", count0, count, bytes0, bytes)
	}
	if b.Stride != 4 || len(b.Pix) != 8 || b.Pix[7] != 0 {
		t.Fatalf("geometry stride=%d len=%d", b.Stride, len(b.Pix))
	}
	alloc.FreeSlice(b.Pix)
}
