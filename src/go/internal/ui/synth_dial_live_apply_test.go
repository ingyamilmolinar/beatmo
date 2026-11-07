package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// Many node pitches, so the prewarmer enumerates them.
func openMelodicSynthRoot(t *testing.T) (*Game, string) {
	t.Helper()
	g := newMelodicCircuitGame(t, 0, 5, -2, 7, -5, 12)
	inst := testMelodicInst
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.SetActiveChannel(channel.Of(inst))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	if got := dv.resolveSynthInstrument(dv.synthTabActiveInstrument()); got != inst {
		t.Fatalf("Synth tab opened on %q, want %q", got, inst)
	}
	return g, string(inst)
}

// The top end moves at least two params, the shape that could publish half-applied documents.
func multiTermDial(t *testing.T, dv *DrumView, inst string) (string, synthdial.Resolved) {
	t.Helper()
	anchor := audio.MergedInstrumentParamsRO(instrument.ID(inst))
	for _, cg := range dv.instEditorMacros {
		for _, d := range cg.Dials {
			out := d.Turn(anchor, 1)
			moved := 0
			for p, v := range out {
				if v != anchor[p] {
					moved++
				}
			}
			if moved >= 2 {
				return cg.ID, d
			}
		}
	}
	t.Fatalf("%q has no root dial that moves two params", inst)
	return "", synthdial.Resolved{}
}

// A detent is ONE edit: the prewarmer renders each binding change at every played pitch into a bounded
// drop-on-overflow pool, so half-applied documents would fill it and the final sound would be dropped.
func TestSynthDialDetentPublishesOnlyItsFinalDocument(t *testing.T) {
	g, inst := openMelodicSynthRoot(t)
	dv := g.drum
	card, d := multiTermDial(t, dv, inst)

	var mu sync.Mutex
	var warmed []instrument.Doc
	g.voicePrewarmer.warmParamsFn = func(id instrument.ID, _ float64, doc instrument.Doc) {
		if id != instrument.ID(inst) {
			return
		}
		mu.Lock()
		warmed = append(warmed, doc)
		mu.Unlock()
	}

	h := &synthDialHitAdapter{dv: dv, instID: instrument.ID(inst), card: card, dial: d.Dial.ID}
	h.OnPress(100, 300)
	h.OnDrag(100, 300-synthDialDragSpanPx/2) // one detent to the top end
	final := audio.MergedInstrumentParamsRO(instrument.ID(inst))
	h.OnRelease(100, 300-synthDialDragSpanPx/2)

	mu.Lock()
	defer mu.Unlock()
	if len(warmed) == 0 {
		t.Fatal("the detent warmed nothing: the prewarmer never heard about the new sound")
	}
	reg := audio.BuiltinModuleRegistry()
	halfApplied := 0
	for _, doc := range warmed {
		vals := instrument.FieldValues(doc, reg)
		for _, term := range d.Terms {
			if vals[term.Param] != final[term.Param] {
				halfApplied++
				break
			}
		}
	}
	if halfApplied > 0 {
		t.Errorf("%s/%s on %q: %d of %d warmed documents were half-applied (a dial param not yet at its detent value) — "+
			"a detent must publish one document, not one per param", card, d.Dial.ID, inst, halfApplied, len(warmed))
	}
}
