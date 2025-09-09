//go:build !js

package ui

// selectJSONAsync uses the desktop picker and calls cb when finished.
func selectJSONAsync(cb func([]byte, error)) {
	go func() { d, e := selectJSON(); cb(d, e) }()
}
