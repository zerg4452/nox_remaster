package noxrender

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/noxworld-dev/opennox-lib/bag"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// loaderFixture: one 1x1 opaque (op 2) type-3 record and its 2x2 HD PNG.
func loaderFixture(t *testing.T) (raw, data []byte, spec func(id int, path string) worldhd.FloorSpec) {
	t.Helper()
	raw = make([]byte, 17)
	binary.LittleEndian.PutUint32(raw[0:], 1)
	binary.LittleEndian.PutUint32(raw[4:], 1)
	raw = append(raw, 2, 1, 0x34, 0x12)
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for i := 0; i < 4; i++ {
		im.SetNRGBA(i%2, i/2, color.NRGBA{R: uint8(8 * i), G: 0x80, B: 0xF8, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	data = b.Bytes()
	return raw, data, func(id int, path string) worldhd.FloorSpec {
		return worldhd.FloorSpec{ID: id, Type: 3, SourceSHA256: sha256.Sum256(raw), PNGSHA256: sha256.Sum256(data), Path: path, LogicalSize: image.Pt(1, 1), Density: 2}
	}
}

// countingAlloc tracks C buffers so tests can prove nothing leaks.
type countingAlloc struct {
	mu            sync.Mutex
	allocs, frees int
}

func (c *countingAlloc) alloc(n int) ([]uint16, func(), bool) {
	pix, free, ok := allocHDPixels(n)
	if !ok {
		return nil, nil, false
	}
	c.mu.Lock()
	c.allocs++
	c.mu.Unlock()
	return pix, func() {
		c.mu.Lock()
		c.frees++
		c.mu.Unlock()
		free()
	}, true
}

func (c *countingAlloc) balanced() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.allocs == c.frees
}

func waitIdle(t *testing.T, l *hdAssetLoader) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for len(l.pending) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("loader did not finish")
		}
		time.Sleep(time.Millisecond)
		l.BeginFrame()
	}
}

func testImage(id int, raw []byte) *Image {
	return &Image{bag: &bag.ImageRec{Index: id, Type: 3}, raw: raw}
}

func TestWorldHDAssetLoader(t *testing.T) {
	raw, data, spec := loaderFixture(t)
	bad := spec(2, "b.png")
	bad.PNGSHA256 = [32]byte{}
	specs := map[int]worldhd.FloorSpec{1: spec(1, "a.png"), 2: bad}
	files := fstest.MapFS{"a.png": {Data: data}, "b.png": {Data: data}}
	ca := &countingAlloc{}
	l := newHDAssetLoader(files, specs, newHDAssetCache(hdAssetBudget))
	l.alloc = ca.alloc
	a, b, none := testImage(1, raw), testImage(2, raw), testImage(3, raw)
	for _, im := range []*Image{a, b, none, a} {
		l.Request(im)
	}
	if l.stats.Requested != 2 || len(l.pending) != 2 {
		t.Fatalf("requests %+v pending %d", l.stats, len(l.pending))
	}
	waitIdle(t, l)
	got := l.cache.Lookup(a)
	if got == nil || len(got.pix) != 4 {
		t.Fatal("valid asset not installed")
	}
	for i, v := range got.pix {
		if want := uint16(noxRGB555(uint8(8*i), 0x80, 0xF8)); v != want {
			t.Fatalf("sample %d: %#x != %#x", i, v, want)
		}
	}
	if _, ok := l.rejected[b]; !ok || l.stats.Failed != 1 || l.cache.Lookup(b) != nil {
		t.Fatal("invalid asset not rejected")
	}
	for _, im := range []*Image{a, b} {
		l.Request(im) // installed / rejected: no new work
	}
	if l.stats.Requested != 2 || l.inflight != 0 {
		t.Fatalf("stats %+v inflight %d", l.stats, l.inflight)
	}

	// Byte and job limits drop the request without reserving anything.
	l.cache.Clear()
	l.maxInflight = assetBytes(specs[1]) - 1
	l.Request(a)
	l.maxInflight, l.maxPending = hdLoaderInflight, 0
	l.Request(a)
	if l.stats.Dropped != 2 || len(l.pending) != 0 || l.inflight != 0 {
		t.Fatalf("limits: %+v pending %d inflight %d", l.stats, len(l.pending), l.inflight)
	}
	l.maxPending = hdLoaderQueue

	// Close with work in flight frees every buffer and ignores later requests.
	l.Request(a)
	l.Close()
	l.Request(a)
	l.BeginFrame()
	if !ca.balanced() || l.cache.bytes != 0 || len(l.pending) != 0 || l.inflight != 0 {
		t.Fatalf("close leaked: allocs %d frees %d cache %d pending %d", ca.allocs, ca.frees, l.cache.bytes, len(l.pending))
	}
}

func noxRGB555(r, g, b uint8) uint16 { return uint16(r&248)<<7 | uint16(g&248)<<2 | uint16(b)>>3 }

// Many requests against a tiny budget: evictions, re-requests and a close
// while busy must keep the budget and leave no buffer behind.
func TestWorldHDAssetLoaderStress(t *testing.T) {
	raw, data, spec := loaderFixture(t)
	specs := map[int]worldhd.FloorSpec{}
	var images []*Image
	for id := 0; id < 300; id++ {
		specs[id] = spec(id, "a.png")
		images = append(images, testImage(id, raw))
	}
	ca := &countingAlloc{}
	l := newHDAssetLoader(fstest.MapFS{"a.png": {Data: data}}, specs, newHDAssetCache(10*assetBytes(specs[0])))
	l.alloc = ca.alloc
	for frame := 0; frame < 200; frame++ {
		l.BeginFrame()
		for i := 0; i < 20; i++ {
			im := images[(frame*7+i*13)%len(images)]
			l.Request(im)
			l.cache.Lookup(im)
		}
		if l.cache.bytes > l.cache.budget || l.inflight > l.maxInflight || len(l.pending) > l.maxPending {
			t.Fatal("limit exceeded")
		}
		// Let workers catch up periodically so installs and evictions happen
		// regardless of scheduling; requests still race with running workers.
		if frame%10 == 9 {
			waitIdle(t, l)
		}
	}
	if l.cache.stats.Installs == 0 || l.cache.stats.Evictions == 0 {
		t.Fatalf("stress did not exercise the cache: %+v", l.cache.stats)
	}
	l.Close()
	if !ca.balanced() || l.cache.bytes != 0 {
		t.Fatalf("leak after stress: allocs %d frees %d", ca.allocs, ca.frees)
	}
}
