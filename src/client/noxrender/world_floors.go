package noxrender

import (
	"io/fs"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// hdTilePrefetch bounds the floor/edge replacements requested as soon as the
// asset set is bound (design D6); beyond it they load on first draw.
const hdTilePrefetch = 64 << 20

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
	b.worldHD = newHDAssetLoader(root, specs, newHDAssetCache(hdAssetBudget))
	total := 0
	for id, s := range specs {
		if (s.Type == 0 || s.Type == 1) && id >= 0 && id < len(b.byIndex) {
			total += assetBytes(s)
		}
	}
	if total <= hdTilePrefetch {
		for id, s := range specs {
			if (s.Type == 0 || s.Type == 1) && id >= 0 && id < len(b.byIndex) {
				b.worldHD.Request(b.byIndex[id])
			}
		}
	}
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

// PrefetchWorldHD queues this bag's images by record ID for loading ahead of
// drawing (4.3-001a); IDs outside the bag or without a replacement are ignored.
func (b *RenderSprites) PrefetchWorldHD(ids []int) {
	if b.worldHD == nil {
		return
	}
	imgs := make([]*Image, 0, len(ids))
	for _, id := range ids {
		if id >= 0 && id < len(b.byIndex) {
			imgs = append(imgs, b.byIndex[id])
		}
	}
	b.worldHD.Prefetch(imgs)
}

// BeginWorldHDAssets starts an asset frame and installs finished conversions.
func (b *RenderSprites) BeginWorldHDAssets() { b.worldHD.BeginFrame() }
