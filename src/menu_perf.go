//go:build !server

package opennox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
)

var menuPerf = struct {
	path                          string
	prepared, finished, presented bool
	target                        menuPerfTarget
	sample                        menuPerfSample
	heapStart, sysStart           uint64
}{path: os.Getenv("NOX_MENU_PERF_OUTPUT")}

func init() {
	if menuPerf.path != "" {
		menuPerfWait = func(dt time.Duration) {
			if !menuPerf.target.started.IsZero() && !menuPerf.finished {
				menuPerf.sample.WaitNS += int64(dt)
			}
		}
	}
}

func finishMenuPerf(err error) {
	menuPerf.finished = true
	menuPerfWait = nil
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	report := struct {
		Status        string           `json:"status"`
		Error         string           `json:"error,omitempty"`
		WarmupNS      int64            `json:"warmup_ns"`
		MeasurementNS int64            `json:"measurement_ns"`
		HeapStart     uint64           `json:"heap_start"`
		HeapEnd       uint64           `json:"heap_end"`
		SysStart      uint64           `json:"go_sys_start"`
		SysEnd        uint64           `json:"go_sys_end"`
		Samples       []menuPerfSample `json:"samples"`
	}{Status: "complete", WarmupNS: int64(10 * time.Second), MeasurementNS: int64(menuPerf.target.elapsed), HeapStart: menuPerf.heapStart, HeapEnd: mem.Alloc, SysStart: menuPerf.sysStart, SysEnd: mem.Sys, Samples: menuPerf.target.samples}
	if err != nil {
		report.Status = "failed"
		report.Error = err.Error()
	}
	b, writeErr := json.Marshal(report)
	if writeErr == nil {
		var f *os.File
		f, writeErr = os.OpenFile(menuPerf.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if writeErr == nil {
			writeErr = menuOutputWrite(f, b)
			closeErr := f.Close()
			if writeErr == nil {
				writeErr = closeErr
			}
		}
	}
	if err != nil {
		noxrender.Log.Printf("menu-perf failed: %v", err)
	}
	if writeErr != nil {
		noxrender.Log.Printf("menu-perf result write failed: %v", writeErr)
	} else {
		noxrender.Log.Printf("menu-perf status=%s samples=%d elapsed=%s", report.Status, len(report.Samples), menuPerf.target.elapsed)
	}
}

func (c *Client) beginMenuPerf() func() {
	if menuPerf.path == "" || menuPerf.finished {
		return nil
	}
	if !menuPerf.prepared {
		menuPerf.prepared = true
		if !filepath.IsAbs(menuPerf.path) {
			menuPerf.finished = true
			noxrender.Log.Println("menu-perf rejected: absolute output path required")
			return nil
		}
		if _, err := os.Lstat(menuPerf.path); !os.IsNotExist(err) {
			menuPerf.finished = true
			noxrender.Log.Println("menu-perf rejected: output exists or is inaccessible")
			return nil
		}
		if menuOutput.dir != "" || menuDrawTraceEnabled || !useFrameLimit {
			finishMenuPerf(errors.New("menu performance requires capture/trace off and frame limit on"))
			return nil
		}
		f, err := os.OpenFile(menuPerf.path+".pending", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			finishMenuPerf(err)
			return nil
		}
		if err = f.Close(); err != nil {
			finishMenuPerf(err)
			return nil
		}
	}
	if menuPerf.target.started.IsZero() {
		start, err := menuOutputSignal(menuPerf.path + ".start")
		if err != nil {
			finishMenuPerf(err)
			return nil
		}
		if !start {
			return nil
		}
		menuPerf.target.started = time.Now()
	}
	now := time.Now()
	if nox_client_gui_flag_815132 == 0 {
		finishMenuPerf(errors.New("left menu during performance run"))
		return nil
	}
	if !menuPerf.target.last.IsZero() && !menuPerf.presented {
		finishMenuPerf(errors.New("menu frame was not presented"))
		return nil
	}
	if menuPerf.target.last.Sub(menuPerf.target.started) >= 10*time.Second && len(menuPerf.target.samples) == 0 {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		menuPerf.heapStart, menuPerf.sysStart = mem.Alloc, mem.Sys
	}
	done, err := menuPerf.target.Next(now, menuPerf.sample)
	if err != nil || done {
		finishMenuPerf(err)
		return nil
	}
	menuPerf.sample = menuPerfSample{}
	menuPerf.presented = false
	drawStart := time.Now()
	return func() { menuPerf.sample.RenderNS = int64(time.Since(drawStart)) }
}

func (c *Client) observeMenuPerfOutput(im *noximage.Image16, hd bool) {
	if menuPerf.path == "" || menuPerf.finished || menuPerf.target.started.IsZero() {
		return
	}
	sz := c.Seat.ScreenSize()
	menuPerf.sample.HD = hd
	menuPerf.sample.Width, menuPerf.sample.Height = im.Rect.Dx(), im.Rect.Dy()
	menuPerf.sample.WindowWidth, menuPerf.sample.WindowHeight = sz.X, sz.Y
	menuPerf.sample.Filtering = c.Win.GetFiltering()
	menuPerf.presented = true
}
