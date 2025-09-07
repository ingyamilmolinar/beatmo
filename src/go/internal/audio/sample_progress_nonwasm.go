//go:build !js && !test

package audio

// SampleLoadProgress returns (loaded,total) for embedded sample registration.
// Non-WASM builds either do not use this or load samples via desktop APIs.
// Return zeros to indicate no special loading phase.
func SampleLoadProgress() (int, int) { return 0, 0 }
