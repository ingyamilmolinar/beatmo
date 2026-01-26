package ui

// sendLatest writes v to ch, dropping an existing item if the buffer is full.
// It never blocks and will silently discard v if the channel remains full.
func sendLatest[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}

// sendLatestBPM aggressively ensures v is left in the channel by dropping
// existing items until a non-blocking send succeeds. This is used for BPM
// updates where only the most recent value matters and tests expect that the
// final requested BPM is applied even if the audio layer was busy.
func sendLatestBPM(ch chan int, v int) {
	for i := 0; i < 8; i++ { // bounded attempts to avoid long spins
		select {
		case ch <- v:
			return
		default:
		}
		// Drop one pending value if any.
		select {
		case <-ch:
		default:
		}
	}
	// Best-effort final attempt; ok if it drops due to extreme contention.
	select {
	case ch <- v:
	default:
	}
}
