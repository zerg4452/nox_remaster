//go:build !server

package legacy

/*
extern unsigned int nox_menu_trace_enabled;
extern unsigned int nox_menu_trace_edge_calls;
extern unsigned int nox_menu_trace_background_calls;
*/
import "C"

// BeginMenuDrawTrace and EndMenuDrawTrace are render-thread-only diagnostics.
func BeginMenuDrawTrace() {
	C.nox_menu_trace_edge_calls = 0
	C.nox_menu_trace_background_calls = 0
	C.nox_menu_trace_enabled = 1
}

func EndMenuDrawTrace() (edge, background uint32) {
	C.nox_menu_trace_enabled = 0
	return uint32(C.nox_menu_trace_edge_calls), uint32(C.nox_menu_trace_background_calls)
}
