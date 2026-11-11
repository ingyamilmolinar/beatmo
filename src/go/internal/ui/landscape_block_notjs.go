//go:build !js

package ui

// registerLandscapeNoticeLocaleSync is a no-op off the browser: the DOM overlay
// only exists in the WASM/browser build. Desktop landscape is handled by the
// in-engine drawLandscapeUnsupported notice instead.
func registerLandscapeNoticeLocaleSync() {}
