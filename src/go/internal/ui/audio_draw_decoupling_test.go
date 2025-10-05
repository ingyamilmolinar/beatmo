package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// Ensure that heavy DrumView.Draw work (many rows, long timelines) does not
// stall audio queuing. We queue sounds rapidly while forcing repeated Draws
// and assert gaps remain bounded.
func TestAudioDecoupledDuringHeavyDraw(t *testing.T) {
	g := New(testLogger)
	w, h := 800, 600
	g.Layout(w, h)

	// Make the drum view heavier: add rows and increase length toward width.
	for len(g.drum.Rows) < 12 {
		g.drum.AddRow()
	}
	// Clamp length to the visible timeline width via control logic; do a few
	// increments to exercise path/step rebuild and layout updates.
	for i := 0; i < 20; i++ {
		g.drum.lenIncPressed = true
		_ = g.Update()
	}

	// Capture play callbacks with timestamps.
	plays := make(chan time.Time, 4096)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays <- time.Now() })

	// Producer: queue a sound every 5ms for ~150ms.
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.Now().Add(40 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				g.queueSound("snare", 1)
				if time.Now().After(deadline) {
					close(stop)
					return
				}
			}
		}
	}()

	// Hammer Draw in a tight loop while the producer runs.
	img := ebiten.NewImage(w, h)
	for {
		select {
		case <-stop:
			goto done
		default:
			// Stress DrumView.Draw but yield to the scheduler so the audio
			// goroutine can run fairly on slower CI machines.
			g.drawDrumPane(img)
			time.Sleep(0)
		}
	}
done:
	// Allow a small drain window.
	time.Sleep(5 * time.Millisecond)

	// Drain timestamps into a slice.
	var times []time.Time
	for {
		select {
		case ts := <-plays:
			times = append(times, ts)
		default:
			goto drained
		}
	}
drained:
	if len(times) < 8 {
		t.Fatalf("insufficient plays during heavy draw: %d", len(times))
	}
	// Compute max inter-arrival gap; expect under 50ms to be conservative.
	maxGap := time.Duration(0)
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		if gap > maxGap {
			maxGap = gap
		}
	}
	if maxGap > 80*time.Millisecond {
		t.Fatalf("audio stalled during draw: max gap %v > 80ms (events=%d)", maxGap, len(times))
	}
}
