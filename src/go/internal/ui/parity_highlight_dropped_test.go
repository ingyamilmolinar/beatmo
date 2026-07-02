package ui

import (
	"testing"
	"time"
)

// Fix C: when the sequencer legitimately DROPS a highlight (hlCh full under
// load — the select/default in game_sequencer_schedule.go), the UI will never
// paint that beat, so highlightedBeats stays empty for it. The audio still
// fired, so highlight_vs_audio would otherwise panic on a purely cosmetic,
// expected drop. A dropped beat must be exempt from highlight_vs_audio.
//
// Counterpart to TestParityHighlightInFlightGraceExpired (same stale state, NOT
// dropped) which must STILL fire — proving the exemption is precise, not a
// blanket silencer.
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
