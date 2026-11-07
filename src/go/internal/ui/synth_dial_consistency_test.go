package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// One knob, one meaning: a dial position writes the same param value on every instrument, never clipped by an
// instrument's range or capped per instrument, so an edit means the same thing wherever it lands.
func TestDialSpansAreTheSameOnEveryInstrument(t *testing.T) {
	checked := 0
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		merged := audio.MergedInstrumentParamsRO(id)
		voice := 0.0
		if d, ok := audio.InstrumentDoc(id); ok {
			voice = d.Render.Seconds
		}
		for _, cg := range buildMacroGroupsForVoice(schema, merged, voice) {
			for _, d := range cg.Dials {
				for _, lt := range d.Terms {
					checked++
					if lt.From == nil || lt.To == nil {
						t.Errorf("%s/%s term %s: from/to must be declared (an omitted end defaults to each instrument's own range)", d.Dial.ID, cg.ID, lt.Param)
						continue
					}
					from, to := *lt.From, *lt.To
					if lt.Span.From != from || lt.Span.To != to {
						t.Errorf("%s: %s/%s term %s spans %g→%g there, declared %g→%g: the instrument's range clips the knob", id, cg.ID, d.Dial.ID, lt.Param, lt.Span.From, lt.Span.To, from, to)
						continue
					}
					lo, hi := math.Min(from, to), math.Max(from, to)
					if lt.Range.Min > lo || lt.Range.Max < hi {
						t.Errorf("%s: %s/%s term %s: range [%g, %g] does not hold the span %g→%g (a turn would clamp)", id, cg.ID, d.Dial.ID, lt.Param, lt.Range.Min, lt.Range.Max, from, to)
					}
				}
			}
		}
	}
	if checked < 500 {
		t.Fatalf("only %d live terms across the roster: the enumeration is broken", checked)
	}
}
