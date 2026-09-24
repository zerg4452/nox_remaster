package noxrender

import "testing"

func TestMenuOutputFadeSnapshotReadOnly(t *testing.T) {
	r := new(NoxRender)
	calls := 0
	r.fade.arr[0] = fade{key: FadeOutScreenKey, flags: fadeActive | fadeMenu, remaining: 7, doneFunc: func() { calls++ }, drawFunc: func(*fade) { calls++ }}
	a := r.FadeStates()
	if a[0].Key != int(FadeOutScreenKey) || a[0].Flags != int(fadeActive|fadeMenu) || a[0].Remaining != 7 {
		t.Fatalf("snapshot=%+v", a)
	}
	a[0].Remaining = 0
	if r.FadeStates()[0].Remaining != 7 || calls != 0 {
		t.Fatal("snapshot mutated fade or ran a callback")
	}
}
