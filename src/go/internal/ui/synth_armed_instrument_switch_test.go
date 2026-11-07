//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The armed control is per instrument but the editor is one shared pointer, so building another instrument's tab
// must first reconcile the editor, or that tab stays stuck on the previous instrument's schema.

// Through the real channel-pill path: the editor is nil or rebound to B, and B's own schema is laid out.
func TestSynthArmedEditor_ReconcilesOnInstrumentSwitch(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	const instA = "ut-chip-modular" // bound to "synth-modular" in newModularSynthTabGame
	const instB = "ut-switch-snare"

	// Second row/instrument with a DIFFERENT recipe/schema than A.
	dv.AddRow()
	dv.Rows[1].Instrument = instB
	_ = audio.InstallInstrumentDocForTest(instB, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(instB) })

	// Activate A on the Synth tab and arm one of its controls.
	dv.eqPanelZone.SetActiveChannel(channel.Of(instA))
	layoutSynthTab(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs for instrument A")
	}
	idxA := sec.knobIdxs[0]
	dv.armSynthControl(instA, idxA)

	if got, armed := dv.synthControlArmed(instA); !armed || got != idxA {
		t.Fatalf("instrument A control not armed as expected: idx=%d armed=%v", got, armed)
	}
	if dv.instEditorArmedEditor == nil || dv.instEditorArmedEditor.instID != instA {
		t.Fatalf("expected shared armed editor bound to A after arming; got %+v", dv.instEditorArmedEditor)
	}
	aBindingsLen := len(dv.instEditorBindings)
	if aBindingsLen == 0 {
		t.Fatal("instrument A produced no bindings after layout")
	}

	// buildSynthTab(B) must not still see A's editor.
	dv.eqPanelZone.SetActiveChannel(channel.Of(instB))
	layoutSynthTab(t, g)

	if e := dv.instEditorArmedEditor; e != nil && e.instID == instA {
		t.Fatalf("armed editor is stale-bound to instrument A after switching to B: %+v", e)
	}
	if e := dv.instEditorArmedEditor; e != nil && e.instID != instB {
		t.Fatalf("armed editor bound to neither A nor B (idx=%d instID=%q)", e.idx, e.instID)
	}

	// B never armed anything.
	if _, armed := dv.synthControlArmed(instB); armed {
		t.Fatalf("instrument B must not report an armed control; nothing was armed on B")
	}
	if got, armed := dv.synthControlArmed(instA); !armed || got != idxA {
		t.Fatalf("instrument A's armed map entry should be left untouched (lingering but inert): idx=%d armed=%v", got, armed)
	}
	if dv.synthTabActiveInstrument() != instB {
		t.Fatalf("active instrument should now be B, got %q", dv.synthTabActiveInstrument())
	}
	// B's drum-snare schema, not A's frozen synth-modular one.
	if len(dv.instEditorBindings) == 0 {
		t.Fatal("instrument B produced no bindings after switch — Synth tab appears frozen/empty")
	}
	snareNames := map[string]bool{}
	for _, p := range audio.InstrumentParamDefs(roster.Snare) {
		snareNames[p.Name] = true
	}
	if len(snareNames) == 0 {
		t.Fatal("snare has no params to compare against")
	}
	for _, b := range dv.instEditorBindings {
		if !snareNames[b.def.Name] {
			t.Fatalf("binding %q after switching to B is not part of drum-snare's schema — schema still frozen to A (synth-modular)?", b.def.Name)
		}
	}
}

// Rebuilding the SAME instrument (an osc_type toggle mid-edit) still defers the schema swap until disarm.
func TestSynthArmedEditor_SameInstrumentDeferralStillWorks(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	const instA = "ut-chip-modular"

	dv.eqPanelZone.SetActiveChannel(channel.Of(instA))
	layoutSynthTab(t, g)
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Fatal("no selected synth section with knobs for instrument A")
	}
	idxA := sec.knobIdxs[0]
	dv.armSynthControl(instA, idxA)

	if dv.instEditorArmedEditor == nil || dv.instEditorArmedEditor.instID != instA {
		t.Fatalf("expected armed editor bound to A; got %+v", dv.instEditorArmedEditor)
	}
	prevIdx := dv.instEditorArmedEditor.idx

	// Rebuilt with the same instID/idx, never disarmed.
	for i := 0; i < 3; i++ {
		dv.eqPanelZone.SetActiveChannel(channel.Of(instA))
		layoutSynthTab(t, g)
		if dv.instEditorArmedEditor == nil {
			t.Fatalf("iteration %d: armed editor was cleared for the SAME instrument mid-edit", i)
		}
		if dv.instEditorArmedEditor.instID != instA || dv.instEditorArmedEditor.idx != prevIdx {
			t.Fatalf("iteration %d: armed editor drifted: got instID=%q idx=%d, want instID=%q idx=%d",
				i, dv.instEditorArmedEditor.instID, dv.instEditorArmedEditor.idx, instA, prevIdx)
		}
	}
	if got, armed := dv.synthControlArmed(instA); !armed || got != idxA {
		t.Fatalf("instrument A control should still be armed at idx=%d after re-layouts: idx=%d armed=%v", idxA, got, armed)
	}
}
