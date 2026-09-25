package alloc

import "testing"

func TestStatsTracksLiveAllocations(t *testing.T) {
	count0, bytes0 := Stats()
	_, free := Calloc(10, 4)
	if count, bytes := Stats(); count != count0+1 || bytes != bytes0+40 {
		t.Fatalf("after alloc: %d/%d from %d/%d", count, bytes, count0, bytes0)
	}
	free()
	if count, bytes := Stats(); count != count0 || bytes != bytes0 {
		t.Fatalf("after free: %d/%d from %d/%d", count, bytes, count0, bytes0)
	}
}
