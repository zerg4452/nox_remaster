//go:build !server

package opennox

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/legacy"
	"github.com/noxworld-dev/opennox/v1/server"
)

var worldHD struct {
	open           bool
	ready          *noximage.Image16
	status         string
	frames, active uint64
	tiles, matched uint64
}

func init() {
	onClientMapLoaded = prefetchWorldHDMap
	legacy.WorldHDWallBegin = func(img unsafe.Pointer) {
		if noxClient != nil {
			noxClient.r.BeginWorldWall(noxrender.ImageHandle(img))
		}
	}
	legacy.WorldHDWallSpan = func(x, y int) {
		if noxClient != nil {
			noxClient.r.WorldWallSpan(image.Pt(x, y))
		}
	}
	legacy.WorldHDTileOpaque = func(index int, src []uint16) {
		if noxClient != nil {
			noxClient.tiles.hd.Opaque(index, src)
		}
	}
	legacy.WorldHDTile = func(x, y int, base, edge unsafe.Pointer) {
		worldHD.tiles++
		c := noxClient
		if c == nil || !c.tiles.hd.Ready() {
			return
		}
		b := &c.r.Bag
		im := b.AsImage(noxrender.ImageHandle(base))
		if im == nil || im.Type() != 0 {
			c.tiles.hd.Invalidate()
			return
		}
		c.r.CensusImage(im)
		hd := b.WorldHDAsset(im)
		if hd != nil {
			worldHD.matched++
		}
		anchor := c.tiles.hd.Width*(int(legacy.Get_dword_5d4594_3798840())+y-int(legacy.Get_dword_5d4594_3798824())) + int(legacy.Get_dword_5d4594_3798836()) + x - int(legacy.Get_dword_5d4594_3798820())
		if edge == nil {
			c.tiles.hd.Base(anchor, im.Pixdata(), hd)
		} else {
			e := b.AsImage(noxrender.ImageHandle(edge))
			if e == nil || e.Type() != 1 {
				c.tiles.hd.Invalidate()
				return
			}
			c.r.CensusImage(e)
			c.tiles.hd.Edge(anchor, im.Pixdata(), e.Pixdata(), hd, b.WorldHDAsset(e))
		}
	}
	legacy.WorldHDUnsupported = func(kind int) {
		if noxClient == nil {
			return
		}
		if kind == 1 {
			noxClient.tiles.hd.Invalidate()
		}
		noxClient.r.RejectWorldHD(fmt.Sprintf("legacy path %d", kind))
	}
}

// prefetchWorldHDMap requests the HD floors, edges (4.3-001b) and walls
// (4.3-001d) of the loaded map while it is still loading (design D6).
func prefetchWorldHDMap() {
	c := noxClient
	if c == nil || c.r.Bag.WorldHDAssetCount() == 0 {
		return
	}
	c.r.Bag.ResetWorldHDUsage()
	imgs := legacy.WorldHDMapTileImages()
	handles := make([]noxrender.ImageHandle, len(imgs))
	for i, p := range imgs {
		handles[i] = noxrender.ImageHandle(p)
	}
	var walls []noxrender.ImageHandle
	if noxServer != nil {
		walls = wallSpriteHandles(noxServer.Walls.All(), noxServer.Walls.DefByInd)
	}
	added := c.r.Bag.PrefetchWorldHD(append(handles, walls...))
	noxrender.Log.Printf("world-hd prefetch map tiles=%d walls=%d requested=%d waiting=%d", len(handles), len(walls), added, c.r.Bag.WorldHDPrefetchQueued())
}

// wallSpriteHandles returns every sprite (all directions, variations and
// states) of the wall definitions the walls use, each definition once.
func wallSpriteHandles(walls []*server.Wall, def func(int) *server.WallDef) []noxrender.ImageHandle {
	var out []noxrender.ImageHandle
	seen := make(map[int]bool)
	for _, wl := range walls {
		i := int(wl.Tile1)
		if seen[i] {
			continue
		}
		seen[i] = true
		d := def(i)
		if d == nil {
			continue
		}
		for _, states := range d.Sprite8432 {
			for _, dirs := range states {
				for _, p := range dirs {
					if p != nil {
						out = append(out, noxrender.ImageHandle(p))
					}
				}
			}
		}
	}
	return out
}

func (c *Client) beginWorldHD() {
	worldHD.open = false
	worldHD.ready = nil
	memoryDiagTick()
	c.r.Bag.BeginWorldHDAssets()
	if nox_client_gui_flag_815132 != 0 || c.r.Bag.WorldHDAssetCount() == 0 || c.r.PixBufferRect() != image.Rect(0, 0, 1280, 720) {
		return
	}
	worldHD.open = c.r.BeginWorldHDFrame()
	// In-game text uses Scale2x glyphs (4.4-004 B1); menu frames keep replicas.
	c.r.SetHDGlyphSmoothing(true)
}

func (c *Client) endWorldHD() {
	if !worldHD.open {
		return
	}
	worldHD.open = false
	worldHD.frames++
	active, detail, reason := c.r.WorldHDStatus()
	if active && detail == 0 {
		c.r.RejectWorldHD("no HD floor samples")
		active = false
		reason = "no HD floor samples"
	}
	worldHD.ready = c.r.EndHDFrame()
	status := "active"
	if !active || worldHD.ready == nil {
		worldHD.ready = nil
		status = "fallback: " + reason
	} else {
		worldHD.active++
	}
	if status != worldHD.status {
		noxrender.Log.Printf("world-hd status=%s detail=%d active=%d frames=%d tiles=%d matched=%d ring_ready=%t", status, detail, worldHD.active, worldHD.frames, worldHD.tiles, worldHD.matched, c.tiles.hd.Ready())
		worldHD.status = status
	}
	// The menu target was opened before GUI dispatch, but did not own this frame.
	menuHD.open = false
}
