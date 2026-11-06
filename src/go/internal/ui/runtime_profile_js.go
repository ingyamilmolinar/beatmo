//go:build js

package ui

// newRuntimeProfile returns the browser profile with env overrides applied.
// Overrides arrive through window.__beatmoEnv (populated into go.env by the
// JS bootstrap in index.html), so this is the same path desktop uses. The
// bespoke window.__beatmoProfileOverride reader that used to live here is
// retired — see applyEnvProfileOverride in runtime_profile_env.go.
func newRuntimeProfile() *RuntimeProfile {
	p := browserRuntimeProfile()
	applyEnvProfileOverride(p)
	return p
}
