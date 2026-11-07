//go:build !js

package audio

// Desktop builds processors eagerly with no deferral queue; it exists so callers need no platform check.
func EnsureAudioGraphReady() {}
