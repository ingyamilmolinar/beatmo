//go:build js

package audio

// PrewarmVoice is a no-op on browser: the WASM build already renders voice
// warms off-thread on its render worker (see voice_warm_js.go). Extending
// per-note browser coverage is out of scope for the desktop overdue fix.
func PrewarmVoice(id string, pitch float64) {}

// PrewarmVoiceParams is a no-op on browser, mirroring PrewarmVoice: the WASM
// build already renders voice warms off-thread on its render worker, and
// internal/ui's voicePrewarmer (build-tag-free) needs this symbol to resolve
// for every GOOS/GOARCH combination it compiles under.
func PrewarmVoiceParams(id string, pitch float64, params RecipeParams) {}

// VoiceCached always reports false on browser (the desktop prewarmer's cache
// probe is not used on the WASM render-worker path).
func VoiceCached(id string, pitch float64) bool { return false }

// IsMelodicInstrument is a no-op false on browser (neighborhood warming is a
// desktop path; the browser warms via its render worker).
func IsMelodicInstrument(id string) bool { return false }

// SetDispatchMustNotRender is a no-op on browser (the WASM build renders voice
// warms off-thread on its render worker; there is no dispatch-goroutine render
// path here to gate).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op on browser, mirroring SetDispatchMustNotRender.
func SetAsyncRenderSubmit(f func(id string, pitch float64, params RecipeParams)) {}
