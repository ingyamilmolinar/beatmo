package ui

// A var so tests capture the durations without mocking the JS layer.
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

// 16 ms tells record-arm apart from play/stop by feel.
func hapticTransportRecord() { hapticEmit(16) }
