package noxrender

import "testing"

type cacheProbe struct{ freed map[*Image]int }

func (p *cacheProbe) asset(img *Image, bytes int) *hdAsset {
	return &hdAsset{img: img, bytes: bytes, free: func() { p.freed[img]++ }}
}

func (p *cacheProbe) check(t *testing.T, c *hdAssetCache, want ...*Image) {
	t.Helper()
	if len(c.byImage) != len(want) {
		t.Fatalf("installed %d, want %d", len(c.byImage), len(want))
	}
	sum, n := 0, 0
	for a := c.head; a != nil; a = a.next {
		if n >= len(want) || a.img != want[n] || c.byImage[a.img] != a {
			t.Fatalf("LRU order mismatch at %d", n)
		}
		sum += a.bytes
		n++
	}
	if n != len(want) || sum != c.bytes || c.bytes > c.budget {
		t.Fatalf("bytes %d (sum %d) over budget %d or list length %d", c.bytes, sum, c.budget, n)
	}
}

func TestWorldHDAssetCacheBudget(t *testing.T) {
	p := &cacheProbe{freed: map[*Image]int{}}
	a, b, c, d := &Image{}, &Image{}, &Image{}, &Image{}
	cache := newHDAssetCache(100)
	cache.BeginFrame()
	for _, im := range []*Image{a, b, c} {
		if !cache.Install(p.asset(im, 30)) {
			t.Fatal("asset within budget rejected")
		}
	}
	p.check(t, cache, c, b, a)

	// Next frame: a is drawn, so b is now least recently used and evicted.
	cache.BeginFrame()
	if cache.Lookup(a) == nil || cache.Lookup(d) != nil {
		t.Fatal("lookup")
	}
	if !cache.Install(p.asset(d, 30)) || p.freed[b] != 1 || p.freed[a]+p.freed[c] != 0 {
		t.Fatalf("expected only b evicted: %v", p.freed)
	}
	p.check(t, cache, d, a, c)

	// Everything drawn this frame is protected: an install that cannot fit is
	// rejected and freed, and the budget holds.
	for _, im := range []*Image{a, c, d} {
		cache.Lookup(im)
	}
	e := &Image{}
	if cache.Install(p.asset(e, 30)) || p.freed[e] != 1 {
		t.Fatal("install evicted current-frame assets or leaked the rejected buffer")
	}
	p.check(t, cache, d, c, a)

	// Oversized and empty assets are rejected; replacing an image frees the old buffer.
	cache.BeginFrame()
	big := &Image{}
	if cache.Install(p.asset(big, 101)) || cache.Install(p.asset(&Image{}, 0)) || p.freed[big] != 1 {
		t.Fatal("invalid size accepted")
	}
	if !cache.Install(p.asset(a, 40)) || p.freed[a] != 1 {
		t.Fatal("replacement did not free old buffer")
	}
	p.check(t, cache, a, d, c)
	if s := cache.stats; s.Installs != 5 || s.Rejected != 3 || s.Evictions != 1 || s.Hits != 4 || s.Misses != 1 {
		t.Fatalf("stats %+v", s)
	}

	cache.Clear()
	p.check(t, cache)
	for _, im := range []*Image{a, c, d} {
		if p.freed[im] == 0 {
			t.Fatal("clear leaked a buffer")
		}
	}
	if p.freed[a] != 2 || p.freed[c] != 1 || p.freed[d] != 1 {
		t.Fatalf("double free or leak: %v", p.freed)
	}
}

func TestWorldHDAssetCacheCMemory(t *testing.T) {
	pix, free, ok := allocHDPixels(92 * 92)
	if !ok || len(pix) != 92*92 || pix[0] != 0 || pix[len(pix)-1] != 0 {
		t.Fatal("C allocation")
	}
	pix[len(pix)-1] = 0x7fff
	cache := newHDAssetCache(hdAssetBudget)
	im := &Image{}
	if !cache.Install(&hdAsset{img: im, pix: pix, free: free, bytes: 2 * len(pix)}) {
		t.Fatal("install")
	}
	if a := cache.Lookup(im); a == nil || a.pix[len(pix)-1] != 0x7fff {
		t.Fatal("lookup lost pixels")
	}
	cache.Clear() // frees through alloc; a double free would panic
	if _, _, ok := allocHDPixels(0); ok {
		t.Fatal("zero allocation accepted")
	}
}
