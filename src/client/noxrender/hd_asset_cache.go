package noxrender

import (
	"unsafe"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
	"github.com/noxworld-dev/opennox/v1/legacy/common/alloc"
)

// hdAssetBudget caps the bytes of all installed world HD assets (4.2 design D4).
const hdAssetBudget = 256 << 20

// hdAsset is one density-2 16-bit replacement bound to a bag image. Its pixels
// live in C memory so the Go GC target does not grow with the cache (D3).
type hdAsset struct {
	img        *Image
	spec       worldhd.FloorSpec
	pix        []uint16
	free       func()
	bytes      int
	used       uint64
	noted      bool // first draw of this map already counted (noteUse)
	prev, next *hdAsset
}

type hdAssetStats struct {
	Installs, Evictions, Rejected, Hits, Misses int64
}

// hdAssetCache is a byte-budget LRU owned by the render thread. Evicted and
// rejected buffers are freed immediately; assets drawn in the current frame
// are never evicted, so an install that cannot fit is rejected instead and the
// budget is a hard cap.
type hdAssetCache struct {
	budget, bytes int
	frame         uint64
	byImage       map[*Image]*hdAsset
	head, tail    *hdAsset // head = most recently used
	stats         hdAssetStats
}

func newHDAssetCache(budget int) *hdAssetCache {
	return &hdAssetCache{budget: budget, byImage: make(map[*Image]*hdAsset)}
}

// allocHDPixels returns zeroed C memory for n samples, or ok=false if the
// allocation failed.
func allocHDPixels(n int) (pix []uint16, free func(), ok bool) {
	if n <= 0 {
		return nil, nil, false
	}
	ptr, free := alloc.Calloc(n, 2)
	if ptr == nil {
		free()
		return nil, nil, false
	}
	return unsafe.Slice((*uint16)(ptr), n), free, true
}

func (c *hdAssetCache) BeginFrame() { c.frame++ }

// Lookup returns the installed asset for img and marks it used in this frame.
func (c *hdAssetCache) Lookup(img *Image) *hdAsset {
	a := c.byImage[img]
	if a == nil {
		c.stats.Misses++
		return nil
	}
	c.stats.Hits++
	a.used = c.frame
	c.unlink(a)
	c.pushFront(a)
	return a
}

// Install takes ownership of a. It replaces an asset for the same image and
// evicts least recently used assets not drawn in this frame to stay in budget.
func (c *hdAssetCache) Install(a *hdAsset) bool {
	if a.bytes <= 0 || a.bytes > c.budget {
		a.free()
		c.stats.Rejected++
		return false
	}
	if old := c.byImage[a.img]; old != nil {
		c.remove(old)
	}
	for c.bytes+a.bytes > c.budget && c.tail != nil && c.tail.used != c.frame {
		c.remove(c.tail)
		c.stats.Evictions++
	}
	if c.bytes+a.bytes > c.budget {
		a.free()
		c.stats.Rejected++
		return false
	}
	c.byImage[a.img] = a
	c.bytes += a.bytes
	c.pushFront(a)
	c.stats.Installs++
	return true
}

// Clear frees every asset, e.g. when the bag is replaced or HD is disabled.
func (c *hdAssetCache) Clear() {
	for c.head != nil {
		c.remove(c.head)
	}
}

func (c *hdAssetCache) remove(a *hdAsset) {
	c.unlink(a)
	delete(c.byImage, a.img)
	c.bytes -= a.bytes
	a.free()
	a.pix, a.free = nil, nil
}

func (c *hdAssetCache) pushFront(a *hdAsset) {
	a.prev, a.next = nil, c.head
	if c.head != nil {
		c.head.prev = a
	}
	c.head = a
	if c.tail == nil {
		c.tail = a
	}
}

func (c *hdAssetCache) unlink(a *hdAsset) {
	if a.prev != nil {
		a.prev.next = a.next
	} else if c.head == a {
		c.head = a.next
	}
	if a.next != nil {
		a.next.prev = a.prev
	} else if c.tail == a {
		c.tail = a.prev
	}
	a.prev, a.next = nil, nil
}
