//go:build worldhdassets

package noxrender

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"
	"time"
	"unsafe"
)

// waitWorldHDAsset drives asset frames until im is installed (a miss requests it).
func waitWorldHDAsset(t *testing.T, b *RenderSprites, im *Image) []uint16 {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		b.BeginWorldHDAssets()
		if pix := b.WorldHDAsset(im); pix != nil {
			return pix
		}
		if time.Now().After(deadline) {
			t.Fatal("floor asset not installed")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWorldFloorRuntimeBindings(t *testing.T) {
	root := os.Getenv("NOX_WORLDHD_TEST_PROJECT")
	if root == "" {
		t.Fatal("missing project root")
	}
	b := new(RenderSprites)
	b.init()
	if e := b.readVideobag(filepath.Join(filepath.Dir(root), "Nox/video.bag")); e != nil {
		t.Fatal(e)
	}
	defer b.Free()
	assets := os.DirFS(filepath.Join(root, "assets/work/group4-runtime-20260914-001"))
	if e := b.SetWorldHDAssets(assets); e != nil {
		t.Fatal(e)
	}
	// Binding alone loads nothing; floors are prefetched per map (4.3-001b).
	if len(b.worldHD.pending) != 0 || b.WorldHDPrefetchQueued() != 0 || b.worldHD.cache.bytes != 0 {
		t.Fatal("binding started loading before a map")
	}
	var handles []ImageHandle
	for id := 9160; id <= 9168; id++ {
		// Test handles; Image.C needs the engine's handle table.
		h := ImageHandle(unsafe.Pointer(&make([]byte, 1)[0]))
		b.byHandle[h] = b.ImageByIndex(id)
		handles = append(handles, h)
	}
	b.PrefetchWorldHD(handles)
	im := b.ImageByIndex(9166)
	if b.WorldHDAssetCount() != 9 || len(waitWorldHDAsset(t, b, im)) != 92*92 || b.WorldHDAsset(&Image{bag: im.bag}) != nil {
		t.Fatal("wrong bag-instance binding")
	}
	for b.WorldHDPrefetchQueued() != 0 {
		b.BeginWorldHDAssets()
	}
	waitIdle(t, b.worldHD)
	for id := 9160; id <= 9168; id++ {
		if b.worldHD.cache.byImage[b.ImageByIndex(id)] == nil {
			t.Fatalf("floor %d was not prefetched", id)
		}
		// Requests read the record without caching img.raw, which would
		// otherwise disable an image's override data.
		if b.ImageByIndex(id).raw != nil {
			t.Fatalf("floor %d request cached the raw record", id)
		}
	}
	if e := b.SetWorldHDAssets(fstest.MapFS{}); e == nil || b.WorldHDAsset(im) != nil || b.WorldHDAssetCount() != 0 {
		t.Fatal("failed reload retained stale HD")
	}
	if e := b.SetWorldHDAssets(assets); e != nil {
		t.Fatal(e)
	}
	waitWorldHDAsset(t, b, im)
	// Load/unload leak check, not the planned in-game 20 scene round trips.
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 20; i++ {
		if e := b.SetWorldHDAssets(nil); e != nil {
			t.Fatal(e)
		}
		if b.WorldHDAsset(im) != nil {
			t.Fatal("unload retained binding")
		}
		if e := b.SetWorldHDAssets(assets); e != nil {
			t.Fatal(e)
		}
		waitWorldHDAsset(t, b, im)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > 32<<20 {
		t.Fatalf("asset reload growth %d exceeds 32MiB", growth)
	}
	t.Logf("20 asset reloads: stabilized Go heap delta=%d bytes (not in-game scene memory)", growth)
}
