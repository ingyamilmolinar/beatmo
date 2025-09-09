package ui

import (
	"sync/atomic"
	"testing"
	"time"
)

// While the user performs heavy UI operations (resizing, layout, etc.),
// audio playback must remain responsive and decoupled from the UI loop.
// This test queues sounds on a tight interval while hammering Update() with
// length changes and verifies that play callbacks continue to arrive without
// long gaps.
func TestAudioDecoupledDuringHeavyUIOperations(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Capture play callbacks with timestamps.
	const N = 50
	plays := make(chan time.Time, N)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays <- time.Now() })

	// Producer: queue a sound every 5ms for ~250ms.
	stop := make(chan struct{})
	var sent int32
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.Now().Add(250 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				g.queueSound("snare", 1)
				atomic.AddInt32(&sent, 1)
				if time.Now().After(deadline) {
					close(stop)
					return
				}
			}
		}
	}()

	// UI hammer: repeatedly change length and call Update. This simulates
	// interactive resizing and layout churn.
	hammer := time.NewTimer(250 * time.Millisecond)
	for {
		select {
		case <-stop:
			goto done
		case <-hammer.C:
			goto done
		default:
			g.drum.lenIncPressed = true
			_ = g.Update()
			g.drum.lenDecPressed = true
			_ = g.Update()
		}
	}
done:
	// Allow a small drain window for queued events to reach playFn.
	time.Sleep(50 * time.Millisecond)

	// Collect timestamps and verify we received a reasonable number quickly.
	var times []time.Time
	for {
		select {
		case ts := <-plays:
			times = append(times, ts)
		default:
			goto collected
		}
	}
collected:
	if len(times) < 10 {
		t.Fatalf("insufficient plays during UI work: %d", len(times))
	}
	// Check that gaps remain bounded (no long stalls due to UI operations).
	// We sent roughly every 5ms; allow a generous 40ms bound for scheduler
	// variability in CI.
	maxGap := time.Duration(0)
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap > maxGap {
			maxGap = gap
		}
	}
	if maxGap > 40*time.Millisecond {
		t.Fatalf("audio stalled: max gap %v > 40ms", maxGap)
	}
}
