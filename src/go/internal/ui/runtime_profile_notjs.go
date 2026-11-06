//go:build !js

package ui

// newRuntimeProfile returns the desktop profile with env overrides applied.
func newRuntimeProfile() *RuntimeProfile {
	p := desktopRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}
