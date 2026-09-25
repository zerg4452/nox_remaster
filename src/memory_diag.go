//go:build !server

package opennox

import (
	"os"
	"runtime"
	"time"

	"github.com/noxworld-dev/opennox/v1/client/noxrender"
	"github.com/noxworld-dev/opennox/v1/legacy/common/alloc"
)

// Memory diagnostics for the 4.2-M7 gate: with NOX_MEMORY_DIAG set, log the
// Go heap, tracked C allocations, interned original image data and the world
// HD cache at every client map read and once a minute, so process growth can
// be attributed. It never triggers a GC or changes state.
var memoryDiag = struct {
	on   bool
	last time.Time
}{on: os.Getenv("NOX_MEMORY_DIAG") != ""}

func init() {
	if memoryDiag.on {
		onClientMapRead = func() { logMemoryDiag("map-read") }
	}
}

func logMemoryDiag(reason string) {
	if !memoryDiag.on {
		return
	}
	memoryDiag.last = time.Now()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	cCount, cBytes := alloc.Stats()
	var hdBytes, hdAssets int
	var pixBytes, pixCount int64
	if noxClient != nil {
		hdBytes, hdAssets, pixBytes, pixCount = noxClient.r.Bag.MemoryStats()
	}
	noxrender.Log.Printf("memory reason=%s go_heap_alloc=%d go_heap_inuse=%d go_heap_idle=%d go_heap_released=%d go_sys=%d go_next_gc=%d go_num_gc=%d c_alloc_count=%d c_alloc_bytes=%d pixdata_bytes=%d pixdata_count=%d hd_cache_bytes=%d hd_assets=%d",
		reason, m.HeapAlloc, m.HeapInuse, m.HeapIdle, m.HeapReleased, m.Sys, m.NextGC, m.NumGC, cCount, cBytes, pixBytes, pixCount, hdBytes, hdAssets)
}

// memoryDiagTick logs once a minute from the frame loop.
func memoryDiagTick() {
	if memoryDiag.on && time.Since(memoryDiag.last) >= time.Minute {
		logMemoryDiag("periodic")
	}
}
