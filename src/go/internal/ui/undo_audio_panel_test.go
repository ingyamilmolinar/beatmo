//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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
			z.SetActiveChannel("main")

			g.undoManager.OnExternalLoad()
			before := g.undoCapture()
			depth0 := len(g.undoManager.undo)

			tc.mutate(t, g)

			assertOneUndoStepRoundTrip(t, g, tc.name, before, depth0)
		})
	}
}

// firstStageEnableParam returns a "*_enabled" stage-toggle param for the recipe
// whose shipped default is non-zero (so toggling it OFF produces an exported
// synth_params delta). ok=false means the recipe has no such param.
func firstStageEnableParam(recipe string) (string, bool) {
	shipped := audio.RecipeShippedDefaults(recipe)
	for name, def := range shipped {
		if strings.HasSuffix(name, "_enabled") && def != 0 {
			return name, true
		}
	}
	return "", false
}

// TestUndoSynthStageToggle drives the REAL synth stage enable/disable handler
// (toggleSynthStage — what the per-stage enable pill invokes) and proves it
// records a single reversible undo step. Stage enables live in synth_params, so
// the snapshot can restore them.
func TestUndoSynthStageToggle(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	instID := g.drum.Rows[0].Instrument
	recipe := audio.RecipeForInstrument(instID)
	param, ok := firstStageEnableParam(recipe)
	if !ok {
		t.Skipf("recipe %q has no stage-enable param", recipe)
	}

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	g.drum.toggleSynthStage(instID, param)

	assertOneUndoStepRoundTrip(t, g, "synth-stage-toggle", before, depth0)
}

// TestUndoSynthNumericEntry drives the REAL synth knob numeric-entry path: open
// the shared value editor over a knob readout (openSynthParamEditor) and commit
// a typed value through the production editor commit. The typed commit mutates
// synth_params and must record a single reversible undo step.
func TestUndoSynthNumericEntry(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	instID := g.drum.Rows[0].Instrument
	recipe := audio.RecipeForInstrument(instID)

	// Find a knob binding whose typed value produces a document change.
	idx := -1
	var newText string
	for i, b := range g.drum.SynthTabBindings() {
		def := b.def
		if strings.HasSuffix(def.Name, "_enabled") || def.Max <= def.Min {
			continue
		}
		shipped := audio.RecipeShippedDefaults(recipe)
		cur, ok := shipped[def.Name]
		if !ok {
			continue
		}
		// Aim for a value clearly off the shipped default, inside [Min,Max].
		target := cur + (def.Max-def.Min)*0.25
		if target > def.Max || target <= cur {
			target = cur - (def.Max-def.Min)*0.25
		}
		if target < def.Min || target == cur {
			continue
		}
		idx = i
		newText = formatParamForEntry(def, target)
		break
	}
	if idx < 0 {
		t.Skip("no editable synth param produced a document change")
	}

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	g.drum.openSynthParamEditor(idx, instID)
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatal("synth param editor did not open")
	}
	g.drum.paramEditor.ti.SetText(newText)
	g.drum.paramEditor.commit()

	assertOneUndoStepRoundTrip(t, g, "synth-numeric-entry", before, depth0)
}

// TestSynthKnobDragRecordsExactlyOneUndoStep is the flagship "one event per
// drag" proof: it drives a full press→drag(multi-frame)→release knob gesture
// through the REAL production input path (SetInputForTest → g.Update →
// HitIndex → synthKnobHitAdapter) and asserts the entire gesture collapses into
// EXACTLY ONE undo step, committed on release — never one-per-frame.
func TestSynthKnobDragRecordsExactlyOneUndoStep(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.knobdrag", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate knob rect

	k := g.drum.SynthTabKnobs()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatalf("knob %d rect empty after draw", idx)
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := cx, cy
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	// Establish the undo baseline at the moment of press.
	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	pressed = true
	g.Update() // OnPress

	// Drag across several frames — each frame mutates live audio but must NOT
	// record an undo step. The synth knob axis-locks to VALUE mode only on a
	// horizontal-dominant drag (vertical is a section scroll), so move in +x.
	for f := 1; f <= 5; f++ {
		mouseX = cx + f*10 // move right = turn the knob up
		g.Update()
		if steps := len(g.undoManager.undo) - depth0; steps != 0 {
			t.Fatalf("undo step recorded mid-drag at frame %d (got %d, want 0 until release)", f, steps)
		}
	}

	pressed = false
	g.Update() // OnRelease → single commit

	after := g.undoCapture()
	if string(after) == string(before) {
		t.Fatal("knob drag did not change the exported document")
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("knob drag recorded %d undo steps, want exactly 1 (one event per gesture)", steps)
	}
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("knob drag: Undo not byte-identical to pre-drag document")
	}
	_ = hooks.EventInstrumentParamsCommitted
}
