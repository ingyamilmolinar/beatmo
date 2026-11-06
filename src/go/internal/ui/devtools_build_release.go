//go:build release

package ui

// devToolsBuild: the shipped build (`make wasm`, WASM_RELEASE_TAGS) hides the
// developer surfaces. Their code is still compiled in — only hidden.
var devToolsBuild = false
