package ui

import (
	"testing"
	"time"
)

// A highlight dropped because hlCh is full is never painted though its audio fired; that cosmetic drop must not
// trip highlight_vs_audio, while the same state NOT dropped still fires.
func TestParityHighlightDroppedBeatExempt(t *testing.T) {
	assertDefaultParityState(t)
	g := buildInflightHighlightGame(t, time.Now().Add(-500*time.Millisecond)) // stale: past grace
	g.parityWatch = parityWatchPanic

	const abs = 139
	// The sequencer dropped this beat's highlight (hlCh was full).
	g.markHighlightDropped(0, abs)

	g.parityScan("test-dropped-highlight")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			t.Fatalf("highlight_vs_audio fired for a beat whose highlight was deliberately dropped (cosmetic, under load): %+v", m)
		}
	}
}
