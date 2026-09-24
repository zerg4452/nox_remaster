//go:build !server

package opennox

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
)

func loadMenuHDSprites(backgroundPath string) (map[string]*image.NRGBA, error) {
	assets := make(map[string]*image.NRGBA, 6)
	for _, spec := range []struct {
		name string
		w, h int
	}{
		{"MNMchck1", 36, 36}, {"MNMchck2", 36, 36}, {"MNMchckG", 36, 36},
		{"MNMmnup1", 30, 39}, {"MNMmnup2", 30, 39}, {"MNMmnupG", 30, 39},
	} {
		im, err := loadMenuHDSprite(filepath.Join(filepath.Dir(backgroundPath), "menu-hd-ui", spec.name+".png"), spec.w, spec.h)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.name, err)
		}
		assets[spec.name] = im
	}
	return assets, nil
}

func loadMenuHDSprite(path string, width, height int) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width != width || cfg.Height != height {
		return nil, fmt.Errorf("menu HD UI sprite must be %dx%d", width, height)
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	im, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	// Validate before reducing 16-bit PNG channels to NRGBA's eight bits.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a != 0 && a != 0xffff {
				return nil, errors.New("menu HD UI sprite must have binary alpha")
			}
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(out, out.Rect, im, im.Bounds().Min, draw.Src)
	return out, nil
}

func loadMenuHDBackground(path string) (*noximage.Image16, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width != 1920 || cfg.Height != 1440 {
		return nil, errors.New("menu HD background must be 1920x1440")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	im, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	for y := 0; y < 1440; y++ {
		for x := 0; x < 1920; x++ {
			_, _, _, a := im.At(x, y).RGBA()
			if a != 0xffff {
				return nil, errors.New("menu HD background must be opaque")
			}
		}
	}
	out := noximage.NewImage16(im.Bounds())
	draw.Draw(out, out.Rect, im, im.Bounds().Min, draw.Src)
	return out, nil
}

type menuHDTarget struct {
	background *noximage.Image16
	ready      *noximage.Image16
	open       bool
	candidate  bool
}

func (m *menuHDTarget) begin(r *noxrender.NoxRender) {
	r.InvalidateHDFrame()
	m.ready = nil
	m.candidate = false
	m.open = true
}
func (m *menuHDTarget) drawBackground(r *noxrender.NoxRender, id int, pos image.Point, menu bool) {
	if !m.open || m.background == nil || id != 98 || pos != (image.Point{}) || !menu || r.PixBufferRect() != image.Rect(0, 0, 640, 480) {
		return
	}
	m.candidate = r.BeginHDFrame(m.background)
}
func (m *menuHDTarget) end(r *noxrender.NoxRender, edge bool) {
	if !m.open {
		return
	}
	m.open = false
	if edge {
		r.InvalidateHDFrame()
	}
	m.ready = r.EndHDFrame()
}
func (m *menuHDTarget) take() *noximage.Image16 {
	out := m.ready
	m.ready = nil
	return out
}
