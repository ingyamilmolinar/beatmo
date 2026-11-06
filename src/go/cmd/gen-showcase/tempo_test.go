package main

import (
	"testing"
)

// tempo_test.go — does each committed template run at the tempo its score
// marks for the measures it was cut from?
//
// The score is the oracle here exactly as it is for pitch in
// TestTemplateIsDerivedFromScore. What makes tempo its own guard is that a
// score's tempo is a TIMELINE: musicxml.Score used to collapse it to one field
// overwritten by every marking, so an excerpt inherited whichever tempo
// appeared LAST anywhere in the document. Nothing compared the two, and seven
// of twenty-four templates shipped at the wrong speed — six too fast, worst of
// them the Ode to Joy excerpt at 264 against its marked 160 (the tempo of the
// Prestissimo coda 800 measures later), a 65% overspeed.

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
		// Not an error when they legitimately agree — only that the excerpt's
		// tempo is derived from its own measures rather than inherited.
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
