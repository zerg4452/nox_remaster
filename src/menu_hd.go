//go:build !server

package opennox

import (
	"image"
	"os"

	"github.com/noxworld-dev/opennox/v1/client/gui"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/legacy"
)

var menuHD menuHDTarget
var menuHDLoaded bool
var menuHDLastStatus string

func init() {
	legacy.MenuHDBackground = func(win *gui.Window, pos image.Point) {
		if !noxClient.r.ConsumeFullFrameImage() {
			noxClient.r.InvalidateHDFrame()
			return
		}
		menuHD.drawBackground(noxClient.r.NoxRender, int(win.ID()), pos, nox_client_gui_flag_815132 != 0)
	}
}

func (c *Client) beginMenuHD() {
	// DrawFunc already mirrored the world. Do not replace its target before HUD.
	if worldHD.open {
		return
	}
	if !menuHDLoaded {
		menuHDLoaded = true
		if path := os.Getenv("NOX_MENU_HD_BACKGROUND"); path != "" {
			bg, err := loadMenuHDBackground(path)
			if err != nil {
				noxrender.Log.Printf("menu-hd background rejected: %v", err)
			} else {
				menuHD.background = bg
				sprites, err := loadMenuHDSprites(path)
				if err != nil {
					_ = c.r.SetHDMenuSprites(nil)
				} else {
					err = c.r.SetHDMenuSprites(sprites)
				}
				if err != nil {
					noxrender.Log.Printf("menu-hd UI rejected: %v", err)
				}
			}
		}
	}
	menuHD.begin(c.r.NoxRender)
	legacy.BeginMenuHDGuard()
}

func (c *Client) endMenuHD() {
	menuHD.end(c.r.NoxRender, legacy.EndMenuHDGuard())
	if menuHD.background == nil {
		return
	}
	status := "inactive"
	if menuHD.candidate {
		status = "fallback"
	}
	if menuHD.ready != nil {
		status = "active"
	}
	if status != menuHDLastStatus {
		noxrender.Log.Printf("menu-hd status=%s", status)
		menuHDLastStatus = status
	}
}
