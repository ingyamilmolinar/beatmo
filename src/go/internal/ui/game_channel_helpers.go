package ui

import "sync/atomic"

// Never blocks; drops the oldest item when full.
func sendLatest[T any](ch chan T, v T, drops *atomic.Int64) {
	select {
	case ch <- v:
		return
	default:
		if drops != nil {
			drops.Add(1)
		}
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

// Only the newest BPM matters, so keep dropping until it is in the channel.
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
