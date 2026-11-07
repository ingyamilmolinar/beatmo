//go:build !js

package ui

// The DOM overlay exists only in the browser; desktop uses drawLandscapeUnsupported.
func registerLandscapeNoticeLocaleSync() {}
