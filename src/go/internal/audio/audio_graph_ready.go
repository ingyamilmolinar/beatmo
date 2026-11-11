//go:build !js

package audio

// EnsureAudioGraphReady is a no-op on desktop: channel/insert-FX processors
// are constructed eagerly (see SetInsertEffects), there is no browser-style
// AudioContext-gated deferral queue to drain. Exists so callers (the warm-up
// gate, import.go) can call it unconditionally on every platform. See
// audio_graph_ready_js.go for the browser implementation.
func EnsureAudioGraphReady() {}
