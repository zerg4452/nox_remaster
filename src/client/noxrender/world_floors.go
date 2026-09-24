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

// BeginWorldHDAssets starts an asset frame and installs finished conversions.
func (b *RenderSprites) BeginWorldHDAssets() { b.worldHD.BeginFrame() }
