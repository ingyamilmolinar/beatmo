//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// synth_armed_instrument_switch_test.go — regression coverage for the
// cross-task lifecycle bug: the armed model has an asymmetry where
// instEditorArmedControl is a per-instID map but instEditorArmedEditor is a
// single shared pointer. Nothing used to reconcile them when the active
// instrument changed via the channel pill (or project load / row
// reassignment): arming a control on instrument A then switching the Synth
// tab to instrument B left instEditorArmedEditor pointing at A's editor,
// which forced buildSynthTab(B)'s schema-defer guard to keep rendering A's
// schema on B's tab — a stuck/frozen Synth tab for B. buildSynthTab now
// reconciles the shared editor to the instrument being built at the top of
// the function, before the schema-defer guard runs.

// TestSynthArmedEditor_ReconcilesOnInstrumentSwitch arms a control on
// instrument A, switches the Synth tab's active instrument to a DIFFERENT
// instrument B (via the real channel-pill path, EQPanelZone.SetActiveChannel)
// and drives a layout pass. It asserts the shared armed editor is no longer
// stale-bound to A (either nil, or rebound to B) and that B's own schema
// (not A's frozen one) is what actually got laid out.
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

	// Switch the Synth tab's active instrument to B via the real channel-pill
	// path, then drive another layout/build pass. This is the exact repro:
	// buildSynthTab(B) must not still see A's stale armed editor.
	dv.eqPanelZone.SetActiveChannel(channel.Of(instB))
	layoutSynthTab(t, g)

	if e := dv.instEditorArmedEditor; e != nil && e.instID == instA {
		t.Fatalf("armed editor is stale-bound to instrument A after switching to B: %+v", e)
	}
	if e := dv.instEditorArmedEditor; e != nil && e.instID != instB {
		t.Fatalf("armed editor bound to neither A nor B (idx=%d instID=%q)", e.idx, e.instID)
	}

	// Prove B's OWN schema actually rendered — not A's frozen one. A control
	// unexpectedly still being reported armed for B (when B never armed
	// anything) would mean the schema-defer guard incorrectly kept firing.
	if _, armed := dv.synthControlArmed(instB); armed {
		t.Fatalf("instrument B must not report an armed control; nothing was armed on B")
	}
	if got, armed := dv.synthControlArmed(instA); !armed || got != idxA {
		t.Fatalf("instrument A's armed map entry should be left untouched (lingering but inert): idx=%d armed=%v", got, armed)
	}
	if dv.synthTabActiveInstrument() != instB {
		t.Fatalf("active instrument should now be B, got %q", dv.synthTabActiveInstrument())
	}
	// The bindings the tab actually laid out must reflect B's recipe schema
	// (drum-snare), not still be frozen to A's (synth-modular) — the
	// observable symptom of the bug: B's tab visually stuck rendering A's
	// controls. Compare against the drum-snare param-name set directly.
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

// TestSynthArmedEditor_SameInstrumentDeferralStillWorks pins that the fix does
// NOT regress the existing same-instrument schema-defer guard: arming a
// control and rebuilding the SAME instrument's tab (e.g. an osc_type toggle
// mid-edit) must still defer the schema swap until the editor disarms.
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

	// Re-layout the SAME instrument's tab repeatedly (mirrors a live edit /
	// osc_type toggle mid-arm): the armed editor must be REBUILT with the
	// SAME instID/idx, never disarmed, as long as the control stays armed.
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
