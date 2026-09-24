//go:build !server

package opennox

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/legacy"
)

var worldHD struct {
	open           bool
	ready          *noximage.Image16
	status         string
	frames, active uint64
	tiles, matched uint64
}

func init() {
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
		hd := b.WorldFloor(im)
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
			c.tiles.hd.Edge(anchor, im.Pixdata(), e.Pixdata(), hd)
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

func (c *Client) beginWorldHD() {
	worldHD.open = false
	worldHD.ready = nil
	if nox_client_gui_flag_815132 != 0 || c.r.Bag.WorldFloorCount() == 0 || c.r.PixBufferRect() != image.Rect(0, 0, 1280, 720) {
		return
	}
	worldHD.open = c.r.BeginWorldHDFrame()
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
