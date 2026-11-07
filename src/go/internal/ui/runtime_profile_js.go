//go:build js

package ui

// Overrides arrive through window.__beatmoEnv, the same path desktop uses.
func newRuntimeProfile() *RuntimeProfile {
	p := browserRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}
