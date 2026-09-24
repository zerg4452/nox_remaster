package legacy

/*
extern unsigned int nox_menu_hd_guard;
extern unsigned int nox_menu_hd_edge;
*/
import "C"
import (
	"image"
	"unsafe"

	"github.com/noxworld-dev/opennox/v1/client/gui"
)

var MenuHDBackground func(*gui.Window, image.Point)

//export nox_menu_hd_background
func nox_menu_hd_background(win unsafe.Pointer, x, y C.int) {
	if MenuHDBackground != nil && win != nil {
		MenuHDBackground((*gui.Window)(win), image.Pt(int(x), int(y)))
	}
}

func BeginMenuHDGuard()    { C.nox_menu_hd_edge = 0; C.nox_menu_hd_guard = 1 }
func EndMenuHDGuard() bool { C.nox_menu_hd_guard = 0; return C.nox_menu_hd_edge != 0 }
