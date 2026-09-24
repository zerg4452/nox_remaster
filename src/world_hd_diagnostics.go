//go:build !server

package opennox

import (
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/noxworld-dev/opennox-lib/noximage"
	"github.com/noxworld-dev/opennox/v1/client/noxrender"
)

var worldPerf = struct {
	path            string
	target          menuPerfTarget
	sample          menuPerfSample
	done, presented bool
	heapStart       uint64
}{path: os.Getenv("NOX_WORLD_PERF_OUTPUT")}
var worldCapture = struct {
	path string
	done bool
}{path: os.Getenv("NOX_WORLD_CAPTURE")}

func writeWorldReport(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}

// Wait timing belongs to the interval consumed by the next beginWorldPerf.
// It includes any loop hooks serviced while waiting, not just scheduler delay.
func observeWorldPerfWait(elapsed, requested time.Duration) {
	if worldPerf.path == "" || worldPerf.done || worldPerf.target.started.IsZero() {
		return
	}
	worldPerf.sample.WaitNS += int64(elapsed)
	if requested < 0 || worldPerf.sample.WaitRequestedNS < 0 {
		worldPerf.sample.WaitRequestedNS = -1
	} else {
		worldPerf.sample.WaitRequestedNS += int64(requested)
	}
}

func finishWorldPerf(err error) {
	worldPerf.done = true
	worldPerfWait = nil
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	status := "complete"
	msg := ""
	wantHD := os.Getenv("NOX_WORLD_HD_FLOORS") != ""
	for _, s := range worldPerf.target.samples {
		if s.HD != wantHD || s.Width != 1280*(1+bool2int(wantHD)) || s.Height != 720*(1+bool2int(wantHD)) {
			err = errors.New("output mode changed or HD fell back during measurement")
			break
		}
	}
	if err != nil {
		status = "failed"
		msg = err.Error()
	}
	report := map[string]any{"status": status, "error": msg, "expected_hd": wantHD, "warmup_ns": int64(10 * time.Second), "measurement_ns": int64(worldPerf.target.elapsed), "heap_start": worldPerf.heapStart, "heap_end": mem.HeapAlloc, "samples": worldPerf.target.samples, "note": "single in-game measurement; requires 3 original and 3 HD runs plus separate scene-memory checks"}
	report["limiter_timing"] = "v1: limiter_wait_ns measures the whole limiter call including loop hooks; limiter_requested_ns is local-host requested sleep (0=no requested sleep, -1=unknown non-host rate target). Frame interval minus limiter elapsed is remaining work, not pure render time."
	if e := writeWorldReport(worldPerf.path, report); e != nil {
		noxrender.Log.Printf("world-perf report failed: %v", e)
	} else {
		noxrender.Log.Printf("world-perf status=%s samples=%d error=%s", status, len(worldPerf.target.samples), msg)
	}
}

func (c *Client) beginWorldPerf() func() {
	if worldPerf.path == "" || worldPerf.done {
		return nil
	}
	if worldPerf.target.started.IsZero() {
		if !filepath.IsAbs(worldPerf.path) {
			worldPerf.done = true
			noxrender.Log.Println("world-perf rejected relative path")
			return nil
		}
		start, e := menuOutputSignal(worldPerf.path + ".start")
		if e != nil {
			finishWorldPerf(e)
			return nil
		}
		if !start {
			return nil
		}
		if !useFrameLimit || menuPerf.path != "" || menuOutput.dir != "" || menuDrawTraceEnabled || worldCapture.path != "" {
			finishWorldPerf(errors.New("world measurement requires limiter on and other diagnostics off"))
			return nil
		}
		if _, e = os.Lstat(worldPerf.path); !os.IsNotExist(e) {
			worldPerf.done = true
			noxrender.Log.Println("world-perf output already exists or inaccessible")
			return nil
		}
		worldPerf.target.started = time.Now()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		worldPerf.heapStart = m.HeapAlloc
		worldPerfWait = observeWorldPerfWait
	}
	if nox_client_gui_flag_815132 != 0 {
		finishWorldPerf(errors.New("left game during measurement"))
		return nil
	}
	if !worldPerf.target.last.IsZero() && !worldPerf.presented {
		finishWorldPerf(errors.New("game frame not presented"))
		return nil
	}
	done, e := worldPerf.target.Next(time.Now(), worldPerf.sample)
	if e != nil || done {
		finishWorldPerf(e)
		return nil
	}
	worldPerf.sample = menuPerfSample{}
	worldPerf.presented = false
	start := time.Now()
	return func() { worldPerf.sample.RenderNS = int64(time.Since(start)) }
}

func (c *Client) observeWorldOutput(im *noximage.Image16, hd bool) {
	if worldPerf.path != "" && !worldPerf.done && !worldPerf.target.started.IsZero() {
		sz := c.Seat.ScreenSize()
		worldPerf.sample.HD = hd
		worldPerf.sample.Width = im.Rect.Dx()
		worldPerf.sample.Height = im.Rect.Dy()
		worldPerf.sample.WindowWidth = sz.X
		worldPerf.sample.WindowHeight = sz.Y
		worldPerf.sample.Filtering = c.Win.GetFiltering()
		worldPerf.presented = true
	}
	if worldCapture.path == "" || worldCapture.done || nox_client_gui_flag_815132 != 0 {
		return
	}
	if !filepath.IsAbs(worldCapture.path) {
		worldCapture.done = true
		return
	}
	start, e := menuOutputSignal(worldCapture.path + ".start")
	if e != nil {
		worldCapture.done = true
		return
	}
	if !start {
		return
	}
	worldCapture.done = true
	if e = os.Mkdir(worldCapture.path, 0700); e != nil {
		noxrender.Log.Printf("world-capture rejected: %v", e)
		return
	}
	for name, pix := range map[string]*noximage.Image16{"logical.png": noxPixBuffer.img, "selected.png": im} {
		f, err := os.OpenFile(filepath.Join(worldCapture.path, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			e = err
			break
		}
		err = png.Encode(f, pix)
		ce := f.Close()
		if err != nil {
			e = err
			break
		}
		if ce != nil {
			e = ce
			break
		}
	}
	_, detail, reason := c.r.WorldHDStatus()
	msg := ""
	if e != nil {
		msg = e.Error()
	}
	e = writeWorldReport(filepath.Join(worldCapture.path, "capture.json"), map[string]any{"hd": hd, "detail_samples": detail, "fallback_reason": reason, "error": msg, "output_width": im.Rect.Dx(), "output_height": im.Rect.Dy(), "world_frames": worldHD.frames, "world_active_frames": worldHD.active})
	noxrender.Log.Printf("world-capture hd=%t detail=%d image_error=%s report_error=%v", hd, detail, msg, e)
}
