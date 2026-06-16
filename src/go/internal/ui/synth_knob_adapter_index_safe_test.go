package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// expandSynthPanelForTest activates the Synth tab and forces the audio panel
// to its expanded synth height (>= 240 px) so the detail pane is several knob
// rows tall. The panel only claims that height during a DrumView Layout pass
// that observes TabSynth active; Ebiten's Layout no-ops when the size is
// unchanged, so we toggle the framebuffer size (shrink then grow) after the
// tab is active to force a genuine relayout.
func expandSynthPanelForTest(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Layout(640, 480)
	g.Layout(1280, 720)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

// selectSectionForKnobIdx opens the synth detail stage that owns the given
// knob index (so the knob gets a non-empty, hit-testable rect under the new
// chip-strip + expand-one-detail-pane layout), then re-lays-out the panel.
// instID is the row's resolved instrument id (e.g. "snare", "modular").
func selectSectionForKnobIdx(t *testing.T, g *Game, instID string, knobIdx int) {
	t.Helper()
	dv := g.drum
	owner := synthSectionID(-1)
	for _, s := range dv.instEditorSections {
		for _, k := range s.knobIdxs {
			if k == knobIdx {
				owner = s.id
				break
			}
		}
		if owner != -1 {
			break
		}
	}
	if owner == -1 {
		t.Fatalf("no section owns knob index %d (sections=%v)", knobIdx, dv.instEditorSections)
	}
	dv.setSelectedSynthSection(instID, owner)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	// With the fewer-knobs-per-row layout (capped columns + taller cells) a
	// stage's later knobs can land on a scrolled-off row. Scroll the target
	// knob into view — local index = its position within the owning section —
	// then re-layout so its rect is populated, mirroring a user scrolling to it.
	for _, s := range dv.instEditorSections {
		if s.id != owner {
			continue
		}
		for local, k := range s.knobIdxs {
			if k == knobIdx {
				dv.sectionGrid(owner).ScrollToIndex(local)
				g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
				return
			}
		}
	}
}

// synthBindingIdxByName returns the binding index of the first knob whose
// param name matches, or fails the test.
func synthBindingIdxByName(t *testing.T, g *Game, name string) int {
	t.Helper()
	for i, b := range g.drum.SynthTabBindings() {
		if b.def.Name == name {
			return i
		}
	}
	t.Fatalf("no %q knob in synth bindings", name)
	return -1
}

// TestSynthKnobAdapter_PropagateIsIndexSafe is the defense-in-depth guard for
// the "changing the generator silences the soloed instrument" bug, independent
// of buildSynthTab's schema-swap deferral.
//
// The synth knob drag handler is captured BY INDEX; the tree routes continued
// OnDrag to it without re-testing hit areas. If the binding slice were ever
// rebuilt mid-gesture (historically the bespoke→modular schema swap; today a
// recipe rebind from any other source), the captured index would point at a
// DIFFERENT param and the drag would scramble it — silencing the voice when
// the foreign param is a gain/enable. The buildSynthTab deferral prevents the
// swap; this test proves the adapter ALSO self-defends: once captured on a
// knob, it refuses to drive any param whose name drifts, even if the binding
// is swapped out from under it.
func TestSynthKnobAdapter_PropagateIsIndexSafe(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	expandSynthPanelForTest(t, g)

	// Locate the decay knob (any grid knob exercises the latch). Decay lives in
	// the ENVELOPE stage — open that stage so the knob gets a hit-testable rect.
	genIdx := synthBindingIdxByName(t, g, "decay")
	selectSectionForKnobIdx(t, g, "snare", genIdx)
	knob := g.drum.SynthTabKnobs()[genIdx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatal("decay knob has empty rect")
	}
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	adapter := &synthKnobHitAdapter{dv: g.drum, idx: genIdx, instID: "snare"}

	// Capture the drag on decay.
	if adapter.OnPress(cx, cy) == InputIgnored {
		t.Fatal("decay knob ignored the press")
	}
	if adapter.capturedParam != "decay" {
		t.Fatalf("OnPress did not latch the captured param name (got %q, want decay)", adapter.capturedParam)
	}

	// Simulate the worst case the deferral normally prevents: the binding slice
	// is rebuilt under the captured drag so this index now names a FOREIGN
	// modular param. (A real schema swap also rebuilds the knob slice, but we
	// only need the binding drift to exercise the index-safety guard.)
	g.drum.instEditorBindings[genIdx] = instParamBinding{def: audio.ParamDef{
		Name: "fm_op1_ratio", Min: 0, Max: 4, Default: 1,
	}}

	// Drive the drag. Without the name latch this would write fm_op1_ratio
	// (a foreign param) and could silence the voice. The guard must no-op.
	adapter.OnDrag(cx+40, cy)
	adapter.OnDrag(cx+80, cy)
	adapter.OnRelease(cx+80, cy)

	got := audio.GetInstrumentParams("snare")
	if _, scrambled := got["fm_op1_ratio"]; scrambled {
		t.Errorf("captured-by-index drag wrote a FOREIGN param after the binding drifted: fm_op1_ratio=%v (full=%v); propagateIfStable must no-op when the param name drifts from the captured name", got["fm_op1_ratio"], got)
	}
}

// TestSynthKnobAdapter_PropagateWritesStableParam is the positive control: when
// the binding at the captured index keeps its name, the drag DOES drive that
// param. This proves the index-safety guard discriminates by name and does not
// break the normal (non-swapping) drag path.
func TestSynthKnobAdapter_PropagateWritesStableParam(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	expandSynthPanelForTest(t, g)

	genIdx := synthBindingIdxByName(t, g, "decay")
	selectSectionForKnobIdx(t, g, "snare", genIdx)
	knob := g.drum.SynthTabKnobs()[genIdx]
	rect := knob.Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	adapter := &synthKnobHitAdapter{dv: g.drum, idx: genIdx, instID: "snare"}
	if adapter.OnPress(cx, cy) == InputIgnored {
		t.Fatal("decay knob ignored the press")
	}
	// Binding name stays decay (no swap). Drag right to move it off default.
	adapter.OnDrag(cx+40, cy)
	adapter.OnRelease(cx+40, cy)

	if _, ok := audio.GetInstrumentParams("snare")["decay"]; !ok {
		t.Fatal("stable-binding drag did not write decay — the index-safety guard must not block the normal path")
	}
}
