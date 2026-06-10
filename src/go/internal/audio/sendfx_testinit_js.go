//go:build js && wasm && !test

package audio

import "testing"

// ensureSendFXForTest is a no-op under the JS+WASM build path. WASM unit
// tests don't exercise the send-fx C code directly (WebAudio handles it on
// the JS side); the bridge tests live in src/js/*.browser.test.js.
func ensureSendFXForTest(_ *testing.T) {}
