//go:build !server

package opennox

import (
	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"os"
)

var menuOutput = menuOutputCapture{dir: os.Getenv("NOX_MENU_OUTPUT_CAPTURE")}

func init() {
	closeMenuOutputCapture = func() {
		if err := menuOutput.Close(); err != nil {
			noxrender.Log.Printf("menu-output capture failed: %v", err)
		}
	}
}

func (c *Client) observeMenuOutput(im *noximage.Image16, hd bool) {
	if menuOutput.dir == "" || menuOutput.finished {
		return
	}
	var fade [4]menuOutputFade
	for i, f := range c.r.FadeStates() {
		fade[i] = menuOutputFade{f.Key, f.Flags, f.Remaining}
	}
	if err := menuOutput.Output(im, hd, fade); err != nil {
		noxrender.Log.Printf("menu-output capture failed: %v", err)
	}
}
