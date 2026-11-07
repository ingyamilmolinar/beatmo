//go:build js

package audio

import "syscall/js"

// Builds the deferred channel/FX graph during the readiness gate, while Play is disabled and the toast is up, so a
// circuit imported before the unlock gesture does not burst that construction at its first note.
func EnsureAudioGraphReady() {
	fn := js.Global().Get("flushPendingChannelOps")
	if !fn.Truthy() {
		return
	}
	fn.Invoke()
}
