//go:build !server

package opennox

import (
	"image"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
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
