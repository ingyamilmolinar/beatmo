//go:build js

package audio

// PrewarmVoice is a no-op on browser: the WASM build already renders voice
// warms off-thread on its render worker (see voice_warm_js.go). Extending
// per-note browser coverage is out of scope for the desktop overdue fix.
func PrewarmVoice(id string, pitch float64) {}

// VoiceCached always reports false on browser (the desktop prewarmer's cache
// probe is not used on the WASM render-worker path).
func VoiceCached(id string, pitch float64) bool { return false }

// IsMelodicInstrument is a no-op false on browser (neighborhood warming is a
// desktop path; the browser warms via its render worker).
func IsMelodicInstrument(id string) bool { return false }
