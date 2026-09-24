package noxrender

import (
	"encoding/binary"
	"fmt"
	"image"
	"path"
	"strings"

	noxcolor "github.com/noxworld-dev/opennox-lib/color"
	"github.com/noxworld-dev/opennox-lib/noximage"
)

func hdMenuSpriteSize(name string) image.Point {
	switch name {
	case "MNMchck1", "MNMchck2", "MNMchckG":
		return image.Pt(12, 12)
	case "MNMmnup1", "MNMmnup2", "MNMmnupG":
		return image.Pt(10, 13)
	default:
		return image.Point{}
	}
}

// SetHDMenuSprites replaces the complete menu-only asset set. A rejected set
// clears the previous registration; retained pixels never alias the caller.
func (r *NoxRender) SetHDMenuSprites(assets map[string]*image.NRGBA) error {
	r.hd.menuSprites = nil
	if len(assets) == 0 {
		return nil
	}
	if len(assets) != 6 {
		return fmt.Errorf("menu HD UI requires all six sprites")
	}
	out := make(map[string]*noximage.Image16, 6)
	for name, im := range assets {
		sz := hdMenuSpriteSize(name).Mul(3)
		if sz == (image.Point{}) || im == nil || im.Rect != (image.Rectangle{Max: sz}) {
			return fmt.Errorf("invalid menu HD UI sprite %q bounds", name)
		}
		if im.Stride < sz.X*4 || int64(len(im.Pix)) < int64(sz.Y-1)*int64(im.Stride)+int64(sz.X*4) {
			return fmt.Errorf("invalid menu HD UI sprite %q storage", name)
		}
		converted := noximage.NewImage16(im.Rect)
		for y := 0; y < sz.Y; y++ {
			for x := 0; x < sz.X; x++ {
				c := im.NRGBAAt(x, y)
				if c.A != 0 && c.A != 255 {
					return fmt.Errorf("menu HD UI sprite %q must have binary alpha", name)
				}
				converted.Pix[converted.PixOffset(x, y)] = noxcolor.RGBA5551Color(c.R, c.G, c.B, c.A).Color16()
			}
		}
		out[name] = converted
	}
	r.hd.menuSprites = out
	return nil
}

// Eligibility uses the actual bag name and decoder header, not an image index
// or GUI state. Capture clipping before the original draw runs its hooks.
func (r *NoxRender) hdMenuSprite(img Image16, pos image.Point) (*noximage.Image16, image.Rectangle) {
	if !r.hd.active || r.hd.scale != 3 {
		return nil, image.Rectangle{}
	}
	im, ok := img.(*Image)
	if !ok || im.bag == nil {
		return nil, image.Rectangle{}
	}
	name := strings.TrimSuffix(im.bag.Name, path.Ext(im.bag.Name))
	sz := hdMenuSpriteSize(name)
	if sz == (image.Point{}) {
		return nil, image.Rectangle{}
	}
	data := im.Pixdata()
	if im.Type() != 3 || len(data) < 17 ||
		binary.LittleEndian.Uint32(data) != uint32(sz.X) || binary.LittleEndian.Uint32(data[4:]) != uint32(sz.Y) ||
		binary.LittleEndian.Uint32(data[8:]) != 0 || binary.LittleEndian.Uint32(data[12:]) != 0 ||
		r.p == nil || r.p.IsAlphaEnabled() || r.p.Multiply14() || r.p.Colorize17() || r.p.Flag16() || r.interlacing || r.dword_5d4594_3799484 != 0 {
		r.InvalidateHDFrame()
		return nil, image.Rectangle{}
	}
	clip := (image.Rectangle{Min: pos, Max: pos.Add(sz)}).Intersect(r.pix.Rect)
	if r.p.Clip() {
		clip = clip.Intersect(r.p.ClipRect())
	}
	return r.hd.menuSprites[name], clip
}

func (r *NoxRender) drawHDMenuSprite(im *noximage.Image16, pos image.Point, clip image.Rectangle) {
	for y := clip.Min.Y * 3; y < clip.Max.Y*3; y++ {
		dst := r.hd.pix.Row(y)
		// Image16.Row bounds y by stride, which rejects the bottom of 30x39 assets.
		src := im.Pix[im.PixOffset(0, y-pos.Y*3):]
		for x := clip.Min.X * 3; x < clip.Max.X*3; x++ {
			c := src[x-pos.X*3]
			if c&0x8000 == 0 {
				dst[x] = c
			}
		}
	}
}
