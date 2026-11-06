//go:build noscenes

package ui

// sceneCatalog is empty in a `-tags noscenes` build (the shipped WASM, `make
// wasm`): the catalog is screenshot/test tooling, so it is compiled only into
// the desktop binary, `go test` and `make wasm-dev`. See scene_registry.go.
var sceneCatalog []Scene
