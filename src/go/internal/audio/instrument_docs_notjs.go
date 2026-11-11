//go:build !js

package audio

// PushAllInstrumentDocsToPlatform is a no-op off the browser: native renders
// read the store directly.
func PushAllInstrumentDocsToPlatform() {}
