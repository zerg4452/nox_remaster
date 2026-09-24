package noxrender

import (
	"bytes"
	"fmt"
	"io/fs"

	"github.com/noxworld-dev/opennox-lib/noximage/pcx"
	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// SetWorldFloors replaces the binding against this exact bag instance. It must
// run on the render thread, outside drawing. Rejected reloads clear old bindings.
func (b *RenderSprites) SetWorldFloors(root fs.FS) error {
	b.worldFloors = nil
	if root == nil {
		return nil
	}
	specs, err := worldhd.ReadManifest(root)
	if err != nil {
		return err
	}
	set, err := worldhd.LoadFloors(root, specs, func(id int) (worldhd.FloorSource, error) {
		if id < 0 || id >= len(b.byIndex) {
			return worldhd.FloorSource{}, fmt.Errorf("bag record %d out of range", id)
		}
		im := b.byIndex[id]
		if im == nil || im.bag == nil {
			return worldhd.FloorSource{}, fmt.Errorf("missing original record %d", id)
		}
		raw, e := im.bag.Raw()
		if e != nil {
			return worldhd.FloorSource{}, e
		}
		decoded, e := pcx.Decode(bytes.NewReader(raw), byte(im.Type()))
		if e != nil {
			return worldhd.FloorSource{}, e
		}
		return worldhd.FloorSource{Type: im.Type(), Raw: raw, Mask: decoded.Image, Offset: decoded.Point}, nil
	})
	if err != nil {
		return err
	}
	out := make(map[*Image]*worldhd.Floor, len(specs))
	for _, s := range specs {
		out[b.byIndex[s.ID]] = set.Lookup(s.ID, s.Type, s.SourceSHA256)
	}
	b.worldFloors = out
	return nil
}

// WorldFloor uses the bag-owned image instance, so a handle from another bag or
// an external image cannot reuse an ID's binding. Hashing never occurs per draw.
func (b *RenderSprites) WorldFloor(im *Image) *worldhd.Floor { return b.worldFloors[im] }
func (b *RenderSprites) WorldFloorCount() int                { return len(b.worldFloors) }
