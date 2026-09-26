package noxrender

import (
	"io/fs"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// SetWorldHDAssets replaces the world HD asset set bound to this exact bag. It
// must run on the render thread, outside drawing. A rejected index leaves no
// assets, and the previous set's buffers are freed first.
func (b *RenderSprites) SetWorldHDAssets(root fs.FS) error {
	b.worldHD.Close()
	b.worldHD = nil
	if root == nil {
		return nil
	}
	specs, err := worldhd.ReadAssetIndex(root)
	if err != nil {
		return err
	}
	// Floors and edges are prefetched per map when it loads (PrefetchWorldHD).
	b.worldHD = newHDAssetLoader(root, specs, newHDAssetCache(hdAssetBudget))
	return nil
}

func (b *RenderSprites) WorldHDAssetCount() int {
	if b.worldHD == nil {
		return 0
	}
	return len(b.worldHD.specs)
}

// WorldHDAsset returns the installed density-2 samples for a bag-owned image,
// requesting them on a miss. A handle from another bag or an external image
// never matches, and hashing never occurs per draw.
func (b *RenderSprites) WorldHDAsset(im *Image) []uint16 {
	if b.worldHD == nil || im == nil || im.bag == nil {
		return nil
	}
	if id := im.bag.Index; id < 0 || id >= len(b.byIndex) || b.byIndex[id] != im {
		return nil
	}
	if a := b.worldHD.cache.Lookup(im); a != nil {
		if !a.noted {
			b.worldHD.noteUse(a)
		}
		return a.pix
	}
	b.worldHD.Request(im)
	return nil
}

// MemoryStats reports HD cache usage and original image data copied to C
// memory by Pixdata (diagnostics).
func (b *RenderSprites) MemoryStats() (hdBytes, hdAssets int, pixBytes, pixCount int64) {
	if b.worldHD != nil {
		hdBytes, hdAssets = b.worldHD.cache.bytes, len(b.worldHD.cache.byImage)
	}
	return hdBytes, hdAssets, pixdataInterned.bytes, pixdataInterned.count
}

// PrefetchWorldHD queues this bag's images for loading ahead of drawing
// (4.3-001) and returns how many were queued; handles of other images or
// without a replacement are ignored.
func (b *RenderSprites) PrefetchWorldHD(handles []ImageHandle) int {
	if b.worldHD == nil {
		return 0
	}
	imgs := make([]*Image, 0, len(handles))
	for _, h := range handles {
		if im := b.byHandle[h]; im != nil && im.bag != nil && im.bag.Index >= 0 && im.bag.Index < len(b.byIndex) && b.byIndex[im.bag.Index] == im {
			imgs = append(imgs, im)
		}
	}
	return b.worldHD.Prefetch(imgs)
}

// WorldHDPrefetchQueued is the number of prefetched images not yet requested.
func (b *RenderSprites) WorldHDPrefetchQueued() int { return b.worldHD.PrefetchQueued() }

// ResetWorldHDUsage starts the per-map HD use log ("world-hd use") over; call
// when a map is loaded.
func (b *RenderSprites) ResetWorldHDUsage() {
	if b.worldHD != nil {
		b.worldHD.resetUse()
	}
}

// BeginWorldHDAssets starts an asset frame and installs finished conversions.
func (b *RenderSprites) BeginWorldHDAssets() { b.worldHD.BeginFrame() }
