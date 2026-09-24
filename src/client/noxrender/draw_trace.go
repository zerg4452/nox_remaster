package noxrender

import (
	"runtime"
	"strings"
)

// BeginDrawTrace enables per-frame diagnostic counters on the render thread.
func (r *NoxRender) BeginDrawTrace() { r.drawTrace = make(map[string]uint64) }

// EndDrawTrace disables counters and returns the sampled frame's summary.
func (r *NoxRender) EndDrawTrace() map[string]uint64 {
	counts := r.drawTrace
	r.drawTrace = nil
	return counts
}

func (r *NoxRender) traceDraw(key string) {
	if r.drawTrace != nil {
		r.drawTrace[key]++
	}
}

// Count buffer-access call sites, not writes. Direct r.pix/C writes need
// explicit instrumentation; absence from this summary does not prove coverage.
func (r *NoxRender) traceBufferAccess() {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return
	}
	f := runtime.FuncForPC(pc)
	if f == nil {
		return
	}
	name := f.Name()
	r.traceDraw("buffer:" + name[strings.LastIndexByte(name, '.')+1:])
}
