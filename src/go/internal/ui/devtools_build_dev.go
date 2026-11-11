//go:build !release

package ui

// devToolsBuild: a developer build (`make run`, `make wasm-dev`, serve-lan,
// the screenshot binary, every test build) carries the developer surfaces.
var devToolsBuild = true
