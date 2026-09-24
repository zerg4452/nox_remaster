package opennox

import (
	"fmt"
	"os"
	"unsafe"

	noxcolor "github.com/noxworld-dev/opennox-lib/color"
	"github.com/noxworld-dev/opennox/v1/client/gui"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
)

// A menu-only, explicitly requested diagnostic. It uses real widget dispatch
// and image drawing, never synthetic clicks or a direct HD sprite draw.
func guiAddMenuUIProbe(root *gui.Window) {
	if !menuUIProbeAllowed(os.Getenv("NOX_MENU_UI_PROBE"), nox_client_gui_flag_815132 != 0) {
		return
	}
	g := noxClient.GUI
	var controls []*gui.Window
	counts := make([]int, 5)
	var counter *gui.Window
	panel := g.NewWindowRaw(root, gui.StatusEnabled|gui.StatusAbove|gui.StatusNoFocus, 90, 60, 460, 335,
		func(_ *gui.Window, e gui.WindowEvent) gui.WindowEventResp {
			if e.EventCode() == 0x4007 {
				ptr, _ := e.EventArgsC()
				for i, w := range controls {
					if ptr == uintptr(unsafe.Pointer(w)) {
						// Count even an unexpected disabled event, so the probe
						// cannot hide an input-dispatch regression.
						counts[i]++
						noxrender.Log.Printf("menu-ui-probe input id=%d enabled=%v count=%d", w.ID(), w.Flags.IsEnabled(), counts[i])
						counter.Func94(&gui.StaticTextSetText{Str: fmt.Sprintf("Inputs: %d / %d / %d / %d / %d", counts[0], counts[1], counts[2], counts[3], counts[4])})
					}
				}
			}
			return gui.RawEventResp(1)
		})
	panel.SetID(9900)
	panel.CopyDrawData(root.DrawData())
	panel.SetDraw(func(w *gui.Window, _ *gui.WindowData) int {
		// Legacy option initialization can re-show children after the parse
		// hook. The newest child is drawn first; hide its siblings here before
		// they draw or receive input. Leaving options destroys this whole tree.
		for other := root.Field100(); other != nil; other = other.Prev() {
			if other != w {
				other.Hide()
			}
		}
		p := w.GlobalPos()
		r := g.Render()
		r.DrawRectFilledOpaque(p.X, p.Y, w.Size().X, w.Size().Y, noxcolor.RGB5551Color(12, 18, 24))
		return 1
	})
	g.NewStaticText(panel, 9901, 15, 12, 430, 20, true, false, "MENU UI PROBE - diagnostic only")
	g.NewStaticText(panel, 9902, 15, 40, 430, 20, false, false, "Checkbox: normal / selected / disabled")
	for i, label := range []string{"Normal", "Selected", "Disabled"} {
		w := gui.NewCheckBoxImg(g, panel, uint(9910+i), 25+140*i, 78, 120, 20, label,
			nox_xxx_gLoadImg("UICheckBox"), nox_xxx_gLoadImg("UICheckBoxLit"), nox_xxx_gLoadImg("UICheckBoxDis"))
		if i == 1 {
			w.DrawData().Field0 |= 4
		}
		if i == 2 {
			w.Flags &^= gui.StatusEnabled
		}
		controls = append(controls, w)
	}
	g.NewStaticText(panel, 9903, 15, 120, 430, 20, false, false, "Up: hover/click enabled; disabled must ignore input")
	for i := 0; i < 2; i++ {
		d, free := gui.NewWindowData()
		d.Window = panel
		d.Style = gui.StylePushButton | gui.StyleMouseTrack
		d.SetBackgroundImage(nox_xxx_gLoadImg("DefaultLBUpButton"))
		d.SetSelectedImage(nox_xxx_gLoadImg("DefaultLBUpButtonLit"))
		d.SetHighlightImage(nox_xxx_gLoadImg("DefaultLBUpButtonLit"))
		d.SetDisabledImage(nox_xxx_gLoadImg("DefaultLBUpButtonDis"))
		flags := gui.StatusImage | gui.StatusNoFocus
		if i == 0 {
			flags |= gui.StatusEnabled
		}
		w := gui.NewButtonRaw(g, panel, flags, 75+230*i, 160, 10, 13, d)
		w.SetID(uint(9920 + i))
		free()
		controls = append(controls, w)
	}
	g.NewStaticText(panel, 9904, 25, 185, 180, 20, false, false, "Enabled")
	g.NewStaticText(panel, 9905, 255, 185, 180, 20, false, false, "Disabled")
	counter = g.NewStaticText(panel, 9906, 15, 235, 430, 20, false, false, "Inputs: 0 / 0 / 0 / 0 / 0")
	g.NewStaticText(panel, 9907, 15, 280, 430, 30, false, false, "Use the normal Back button to leave. No settings saved.")
	noxrender.Log.Printf("menu-ui-probe opened: IDs 9910/9911/9912/9920/9921; disabled=9912,9921")
}
