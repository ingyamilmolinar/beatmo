package main

import "github.com/ingyamilmolinar/tunkul/internal/audio"

// main invokes audio.Play without ensuring the JS side is initialized.
// When run in a JS/wasm environment without the audio bridge loaded,
// the call should be a no-op rather than a panic.
func main() {
	audio.Play("snare", 0)
}
