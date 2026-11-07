package main

import (
	"testing"
)

// A score's tempo is a TIMELINE: collapsed to one field, an excerpt inherits whichever tempo appears last in the document.

// TestTempoTimelineIsNotCollapsed pins the property that made the bug possible:
// a multi-tempo score must expose more than one tempo, and the excerpt's tempo
// must be allowed to differ from both the opening and the closing mark.
func TestTempoTimelineIsNotCollapsed(t *testing.T) {
	multi := 0
	for _, spec := range templateSpecs() {
		sc := mustScore(t, spec.ScoreID)
		if len(sc.Tempos) < 2 {
			continue
		}
		multi++
		last := sc.Tempos[len(sc.Tempos)-1].BPM
		got := sc.DominantTempo(spec.From, spec.To)
		// Agreement is fine; only inheritance is the failure.
		if at := sc.TempoAt(spec.From); at <= 0 || got <= 0 {
			t.Errorf("%s: tempo timeline gave a non-positive tempo (at=%v dominant=%v)", spec.Stem, at, got)
		}
		_ = last
	}
	if multi == 0 {
		t.Fatal("no multi-tempo score in the corpus — this guard proves nothing")
	}
	t.Logf("%d of the corpus's scores carry more than one tempo marking", multi)
}
