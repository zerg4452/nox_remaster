package opennox

import "testing"

func TestMenuUIProbeRequiresExplicitMenuOptIn(t *testing.T) {
	for _, tc := range []struct {
		value string
		menu  bool
		want  bool
	}{{"", true, false}, {"false", true, false}, {"1", true, false}, {"true", false, false}, {"true", true, true}} {
		if got := menuUIProbeAllowed(tc.value, tc.menu); got != tc.want {
			t.Errorf("opt-in=%q menu=%v: got %v, want %v", tc.value, tc.menu, got, tc.want)
		}
	}
}
