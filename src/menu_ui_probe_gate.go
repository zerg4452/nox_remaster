package opennox

// The diagnostic must never replace in-game options or appear by default.
func menuUIProbeAllowed(value string, menu bool) bool {
	return value == "true" && menu
}
