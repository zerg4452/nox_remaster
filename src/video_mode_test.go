package opennox

import "testing"

func TestLegacyFullscreenMode(t *testing.T) {
	for _, tc := range []struct {
		mode int
		want int
	}{
		{-4, 0}, {-3, 0}, {-2, 1}, {-1, 1},
		{0, 0}, {1, 1}, {2, 1},
	} {
		if got := legacyFullscreenMode(tc.mode); got != tc.want {
			t.Errorf("mode %d: got %d, want %d", tc.mode, got, tc.want)
		}
	}
}
