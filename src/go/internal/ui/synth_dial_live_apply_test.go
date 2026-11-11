package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// openMelodicSynthRoot builds a melodic row with many node pitches (so the
// prewarmer enumerates them) and opens the Synth tab's category root on it.
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

// multiTermDial returns a root dial on inst whose top end moves at least two
// of its params — the shape whose live apply could publish half-applied
// documents.
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

// One dial detent is ONE edit of the sound: every document the engine hears
// about (each binding change the voice prewarmer renders) must be the
// detent's final document, never a half-applied one with only some of the
// dial's params moved.
//
// Why it matters: the prewarmer renders every binding change at every pitch
// the row plays (± the predictive neighbourhood) into a bounded, drop-on-
// overflow pool. A dial wrote its N params one SetInstrumentParam at a time,
// so each detent queued N documents × every pitch; the intermediate ones —
// sounds nobody will ever hear — filled the queue first and the final one was
// dropped, so playback kept serving an older cached voice while the dial
// turned ("I hear no difference in real time").
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
