//go:build test

package ui

import (
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// assertOneUndoStepRoundTrip runs the shared 3-property undo assertion against a
// game whose pre-action document baseline has just been established: the action
// (1) changes the exported document, (2) records EXACTLY ONE undo step, and
// (3) Undo restores the pre-action bytes and Redo restores the post-action bytes.
// before/depth0 are captured by the caller right before invoking the action so
// only the action under test is measured.
func assertOneUndoStepRoundTrip(t *testing.T, g *Game, name string, before []byte, depth0 int) {
	t.Helper()
	after := g.undoCapture()
	if string(after) == string(before) {
		t.Fatalf("%s did not change the exported document", name)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("%s recorded %d undo steps, want exactly 1 (atomic)", name, steps)
	}
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatalf("%s: Undo not byte-identical to pre-action document", name)
	}
	normalize := func(b []byte) string {
		g.undoManager.restoring = true
		_ = g.Import(b)
		out := g.undoCapture()
		g.undoManager.restoring = false
		return string(out)
	}
	wantRedo := normalize(after)
	g.undoManager.Redo()
	if got := g.undoCapture(); string(got) != wantRedo {
		t.Fatalf("%s: Redo did not restore the post-action document", name)
	}
}

// TestUndoEQFilterAndMuteControls drives the REAL EQ-tab toggle handlers
// (toggleHPF / toggleLPF / toggleEQBandMute) — the production methods the HP/LP
// and band-mute buttons invoke — and proves each records a single reversible
// undo step. These mutate exported master-EQ state (hpf_enabled / lpf_enabled /
// band_muted) live via applyEQ, so the snapshot can restore them.
func TestUndoEQFilterAndMuteControls(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, g *Game)
	}{
		{"hpf-toggle", func(t *testing.T, g *Game) { g.drum.toggleHPF() }},
		{"lpf-toggle", func(t *testing.T, g *Game) { g.drum.toggleLPF() }},
		{"band-mute", func(t *testing.T, g *Game) { g.drum.toggleEQBandMute(3) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGameForUndo(t)
			z := g.drum.eqPanelZone
			if len(z.bandGainsDB) <= 3 {
				t.Skip("eq band gains not initialised")
			}
			z.SetActiveChannel(channel.Master)

			g.undoManager.OnExternalLoad()
			before := g.undoCapture()
			depth0 := len(g.undoManager.undo)

			tc.mutate(t, g)

			assertOneUndoStepRoundTrip(t, g, tc.name, before, depth0)
		})
	}
}

// firstStageEnableParam returns a "*_enabled" stage-toggle param for instID's
// document whose value is non-zero (so toggling it OFF changes the exported
// instrument document) AND that is actually toggleable — a sole-source
// generator stage (e.g. OSC on a pure-oscillator voice) is greyed out and
// non-interactive, so toggleSynthStage no-ops on it (see StageDisableSilences).
// Iterates in deterministic (sorted) order so the choice is stable across runs.
// ok=false means the instrument has no toggleable stage param.
func firstStageEnableParam(instID string) (string, bool) {
	shipped := audio.MergedInstrumentParamsRO(instrument.ID(instID))
	names := make([]string, 0, len(shipped))
	for name := range shipped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, "_enabled") || shipped[name] == 0 {
			continue
		}
		if audio.StageDisableSilences(instrument.ID(instID), name) {
			continue // sole-source generator: non-interactive, cannot be toggled
		}
		return name, true
	}
	return "", false
}

// TestUndoSynthStageToggle drives the REAL synth stage enable/disable handler
// (toggleSynthStage — what the per-stage enable pill invokes) and proves it
// records a single reversible undo step. Stage enables live in the instrument document, so
// the snapshot can restore them.
func TestUndoSynthStageToggle(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	instID := g.drum.Rows[0].Instrument
	param, ok := firstStageEnableParam(string(instID))
	if !ok {
		t.Skipf("instrument %q has no stage-enable param", instID)
	}

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	g.drum.toggleSynthStage(instID, param)

	assertOneUndoStepRoundTrip(t, g, "synth-stage-toggle", before, depth0)
}

// TestSynthControlWheelRecordsExactlyOneUndoStep proves the Task-9 ARMED wheel
// nudge (ArmedControlEditor's value lane) collapses a whole spin into EXACTLY
// ONE undo step, committed once the debounce elapses, through the real
// production adapter (synthControlHitAdapter.OnWheel while armed).
func TestSynthControlWheelRecordsExactlyOneUndoStep(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.knobwheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	k := dv.SynthTabControls()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatalf("knob %d rect empty after draw", idx)
	}

	// Arm the control (Task 9: wheel edits require the control to be armed
	// first — a not-armed wheel now scrolls the row instead of nudging the
	// value).
	dv.armSynthControl(instID, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	vx, vy := e.ValueLaneRect().Min.X+2, r.Min.Y+r.Dy()/2

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	for f := 1; f <= 5; f++ {
		dv.eqPanelZone.Invalidate()
		if res := h.OnWheel(vx, vy, +1); res != InputConsumed {
			t.Fatalf("armed wheel notch %d should be consumed, got %v", f, res)
		}
		g.Update()
		if steps := len(g.undoManager.undo) - depth0; steps != 0 {
			t.Fatalf("undo step recorded mid-spin at notch %d (got %d, want 0 until debounce)", f, steps)
		}
	}
	// Settle the kinetic barrel so the value lane actually crosses detents
	// (SHARED FACTS: ImpulseNotch only resolves into a value change via Tick).
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	for i := 0; i < wheelNudgeCommitFrames+4; i++ {
		g.Update()
	}
	after := g.undoCapture()
	if string(after) == string(before) {
		t.Fatal("wheel spin did not change the exported document")
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("wheel spin recorded %d undo steps, want exactly 1", steps)
	}
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("wheel spin: Undo not byte-identical to pre-spin document")
	}
	_ = hooks.EventInstrumentParamsCommitted
}
