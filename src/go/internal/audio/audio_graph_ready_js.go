//go:build js

package audio

import "syscall/js"

// EnsureAudioGraphReady forces the browser AudioContext into existence and
// drains audio.js's pendingChannelOps queue (channel volume/pan/EQ/insert-FX
// node construction deferred while no context existed).
//
// Called from the playback readiness gate (import.go's g.beginWarmup path)
// so a circuit imported BEFORE the audio-unlock gesture builds its whole
// channel/FX graph now, while the progress toast is already up and Play is
// already disabled — a stall here is invisible. Without this, the deferred
// graph construction would burst at the very first triggered note instead.
func EnsureAudioGraphReady() {
	fn := js.Global().Get("flushPendingChannelOps")
	if !fn.Truthy() {
		return
	}
	fn.Invoke()
}
