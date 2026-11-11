package ui

// hapticEmit is the package-level seam invoked by the named transport
// haptic helpers. Production builds route through `vibrateMS` (which is
// platform-dispatched via build tags); tests can replace `hapticEmit`
// directly to capture the emitted durations without mocking the JS
// layer.
var hapticEmit = func(ms int) {
	if ms <= 0 {
		return
	}
	if !Profile().IsMobile() {
		return
	}
	platformVibrate(ms)
}

// hapticTransportTap fires an 8 ms vibration appropriate for play/stop.
func hapticTransportTap() { hapticEmit(8) }

// hapticTransportRecord fires a 16 ms vibration for the record-arm action,
// distinguishing it from play/stop by feel.
func hapticTransportRecord() { hapticEmit(16) }
