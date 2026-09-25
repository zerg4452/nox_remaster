//go:build !server

package opennox

import (
	"image"
	"reflect"
	"testing"
	"unsafe"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/server"
)

func TestWorldHDPresentationPreservesDrawnFloor(t *testing.T) {
	oldWorld, oldMenu := worldHD, menuHD
	defer func() { worldHD, menuHD = oldWorld, oldMenu }()
	r := noxrender.NewRender(nil)
	logical := noximage.NewImage16(image.Rect(0, 0, 2, 1))
	r.SetPixBuffer(logical)
	c := &Client{r: &NoxRender{NoxRender: r}}
	worldHD.open = r.BeginWorldHDFrame()
	r.WorldFloorPixel(logical.Pix[:1], [4]uint16{1, 2, 3, 4}, true)
	// This is the later presentation/menu entry, after the world was drawn.
	c.beginMenuHD()
	c.endWorldHD()
	if worldHD.ready == nil {
		t.Fatal("presentation discarded the drawn world")
	}
	if got := worldHD.ready.Pix; got[0] != 1 || got[1] != 2 || got[4] != 3 || got[5] != 4 {
		t.Fatalf("HD samples were replaced: %v", got)
	}
	if logical.Pix[0] != 0 {
		t.Fatal("HD output changed logical pixels")
	}
	worldHD.open = r.BeginWorldHDFrame()
	r.RejectWorldHD("test unsupported writer")
	c.endWorldHD()
	if worldHD.ready != nil || worldHD.status != "fallback: test unsupported writer" {
		t.Fatalf("fallback accepted or original reason lost: %q", worldHD.status)
	}
}

// Wall prefetch (4.3-001d) takes every sprite of each used wall definition
// once and skips unknown definitions and empty slots.
func TestWorldHDWallSprites(t *testing.T) {
	var defs [2]server.WallDef
	a, b, c := new(byte), new(byte), new(byte)
	defs[0].Sprite8432[0][0][0] = unsafe.Pointer(a)
	defs[0].Sprite8432[3][14][15] = unsafe.Pointer(b)
	defs[1].Sprite8432[2][7][1] = unsafe.Pointer(c)
	def := func(i int) *server.WallDef {
		if i < 0 || i >= len(defs) {
			return nil
		}
		return &defs[i]
	}
	walls := []*server.Wall{{Tile1: 0}, {Tile1: 5}, {Tile1: 0}, {Tile1: 1}}
	got := wallSpriteHandles(walls, def)
	want := []noxrender.ImageHandle{noxrender.ImageHandle(a), noxrender.ImageHandle(b), noxrender.ImageHandle(c)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if wallSpriteHandles(nil, def) != nil {
		t.Fatal("no walls must prefetch nothing")
	}
}
