package opennox

// legacyFullscreenMode converts the renderer mode to the legacy GUI's boolean.
func legacyFullscreenMode(mode int) int {
	switch mode {
	case -2, -1, 1, 2:
		return 1
	default:
		return 0
	}
}
