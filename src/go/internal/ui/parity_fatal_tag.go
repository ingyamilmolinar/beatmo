//go:build parityfatal

package ui

// Build with -tags parityfatal (e.g. GOFLAGS=-tags=parityfatal make test-browser)
// to make the logical parity checker PANIC on the first mismatch on every
// platform — including WASM, where it is log-only by default (game.go's
// browser init). Browser tests have no single launch / serve / build
// chokepoint (each file builds and serves its own page), but every WASM build
// inherits GOFLAGS, so this is the one switch that reaches all of them.
//
// Runs after game.go's init()s: a package's init functions run in file-name
// order, and "parity_fatal_tag.go" sorts after "game.go".
func init() {
	parityFatalEnabled.Store(true)
	parityWatchDefault = parityWatchPanic
}
