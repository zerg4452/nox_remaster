//go:build !server

package opennox

import (
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/legacy"
	"os"
)

// Opt-in diagnostics deployed only in the isolated debug executable. Sampling
// bounds overhead/log volume; zero calls never establishes complete coverage.
var menuDrawTraceEnabled = os.Getenv("NOX_MENU_DRAW_TRACE") == "true"
var menuDrawTraceFrames uint64

func (c *Client) beginMenuDrawTrace() bool {
	if !menuDrawTraceEnabled || nox_client_gui_flag_815132 == 0 {
		return false
	}
	menuDrawTraceFrames++
	if menuDrawTraceFrames > 1200 || (menuDrawTraceFrames-1)%60 != 0 {
		return false
	}
	c.r.BeginDrawTrace()
	legacy.BeginMenuDrawTrace()
	return true
}

func (c *Client) endMenuDrawTrace() {
	edge, background := legacy.EndMenuDrawTrace()
	counts := c.r.EndDrawTrace()
	noxrender.Log.Printf("menu-draw-trace frame=%d background_calls=%d c_edge_calls=%d counts=%v", menuDrawTraceFrames, background, edge, counts)
}
