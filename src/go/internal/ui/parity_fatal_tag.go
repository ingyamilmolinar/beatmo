//go:build parityfatal

package ui

// -tags parityfatal makes the parity checker PANIC on every platform, WASM included; every WASM build inherits
// GOFLAGS, the one switch that reaches every browser test. Runs after game.go's init (file-name order).
func init() {
	parityFatalEnabled.Store(true)
	parityWatchDefault = parityWatchPanic
}
