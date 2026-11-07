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

// before/depth0 are captured just before the action, so only it is measured.
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

// The production toggle handlers write the exported master EQ live through applyEQ, so the snapshot can restore it.
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

// Non-zero and actually toggleable: a sole-source generator stage is greyed out and toggleSynthStage no-ops on it.
// Sorted, so the choice is stable; ok=false when none exists.
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

// The per-stage enable pill's handler; stage enables live in the instrument document.
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

// A whole spin of the armed value lane is ONE step, committed when the debounce elapses, through the real
// synthControlHitAdapter.OnWheel.
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

	// An unarmed wheel scrolls the row instead of changing the value.
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
	// An impulse notch turns into a value change only through Tick.
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
