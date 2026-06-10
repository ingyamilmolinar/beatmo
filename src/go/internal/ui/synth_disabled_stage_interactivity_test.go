package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// synth_disabled_stage_interactivity_test.go pins the spec's confirmed UX
// choice for disabled stages (design doc § UI unification): "dimmed but
// interactive" — a user may pre-tweak a bypassed stage's knobs, the value
// persists and takes effect when the stage is re-enabled, and the enable pill
// stays visible (bright) so re-enabling is obvious. Previously this behavior
// existed only as code + comment (drawSynthSection); these tests make it a
// contract.

// newDisabledFilterSnareGame builds a full game on the Synth tab with the
// snare instrument's FILTER stage bypassed (filter_enabled = 0).
func newDisabledFilterSnareGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	audio.SetInstrumentParam("snare", "filter_enabled", 0)
	// Open the Synth tab at production height and select the FILTER stage so its
	// detail pane (knobs + enable pill + bypass scrim) is laid out. The
	// per-card pill is gone in the chip-strip redesign — the enable pill lives
	// in the detail header of the SELECTED stage, and only the selected stage's
	// knobs get non-empty rects.
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	g.drum.setSelectedSynthSection("snare", synthSectionFilter)
	layoutSynthTab(t, g)
	return g
}

// TestSynthTab_DisabledStageKnobsRemainInteractive drags a knob on the
// BYPASSED filter stage and asserts (a) the drag still writes the param,
// (b) the value survives re-enabling the stage, and (c) the enable pill stays
// laid out (visible) while the stage is off.
func TestSynthTab_DisabledStageKnobsRemainInteractive(t *testing.T) {
	g := newDisabledFilterSnareGame(t)
	restore := SwapSynthAuditionFnForTest(func(string) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if synthStageEnabled("snare", "filter_enabled") {
		t.Fatal("precondition: filter stage should be bypassed")
	}
	// (c) the pill survives the bypass — it must stay tappable to re-enable.
	// FILTER is the selected stage, so its pill lives in the detail header.
	if g.drum.synthDetailEnablePillRect().Empty() {
		t.Fatal("FILTER enable pill rect is empty while the stage is bypassed (pill must stay visible)")
	}

	// Find the filter_cutoff knob.
	bindings := g.drum.SynthTabBindings()
	knobs := g.drum.SynthTabKnobs()
	idx := -1
	for i, b := range bindings {
		if b.def.Name == "filter_cutoff" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no filter_cutoff knob in bindings")
	}
	knob := knobs[idx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("filter_cutoff knob has empty rect (scrolled out / layout broken)")
	}
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	before := knob.Value

	// (a) press + drag on the DIMMED knob via the same input path a live knob
	// uses. The scrim is visual only — no hit-area suppression.
	if !g.drum.handleSynthTabInput(cx, cy, true, "snare") {
		t.Fatal("press on a dimmed (bypassed-stage) knob was not handled")
	}
	half := knob.dragPixels() / 2
	const steps = 6
	for s := 1; s <= steps; s++ {
		g.drum.handleSynthTabInput(cx+half*s/steps, cy, true, "snare")
	}
	g.drum.handleSynthTabInput(cx+half, cy, false, "snare") // release
	if knob.Value <= before {
		t.Fatalf("dimmed knob did not move: value %v -> %v", before, knob.Value)
	}
	got, ok := audio.GetInstrumentParams("snare")["filter_cutoff"]
	if !ok {
		t.Fatal("filter_cutoff was not written while the stage was bypassed (pre-tweak must persist)")
	}

	// (b) re-enable the stage: the pre-tweaked value must survive and be the
	// effective merged value.
	audio.SetInstrumentParam("snare", "filter_enabled", 1)
	merged := audio.MergeRecipeDefaults("drum-snare", audio.GetInstrumentParams("snare"))
	if merged["filter_cutoff"] != got {
		t.Fatalf("pre-tweaked filter_cutoff lost on re-enable: merged %v, want %v", merged["filter_cutoff"], got)
	}
}

// TestSynthTab_DisabledStageScrimDrawn asserts the bypass scrim is painted
// over the SELECTED (FILTER) stage's detail-pane knob area (below the detail
// header band) when the stage is off, and NOT painted when the stage is on.
// In the chip-strip redesign the scrim moved from the per-card body to the
// detail pane: drawSynthDetailPane emits a WithAlpha(Surface1, AlphaStrong)
// filled rounded rect spanning the pane below dv.instEditorDetailHeaderH.
func TestSynthTab_DisabledStageScrimDrawn(t *testing.T) {
	g := newDisabledFilterSnareGame(t)

	scrimColor := WithAlpha(TokenSurface1(), AlphaStrong)
	countScrims := func() int {
		detail := g.drum.instEditorDetailR
		// The scrim covers the knob area below the detail header band.
		body := image.Rect(detail.Min.X, detail.Min.Y+g.drum.instEditorDetailHeaderH, detail.Max.X, detail.Max.Y)
		n := 0
		orig := drawRoundedRect
		drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
			if filled && c == scrimColor && r.In(body) && r.Dx() > body.Dx()/2 {
				n++
			}
			orig(dst, r, c, radius, filled)
		}
		defer func() { drawRoundedRect = orig }()
		scratch := ebiten.NewImage(1280, 720)
		g.drum.eqPanelZone.Draw(scratch)
		return n
	}

	if got := countScrims(); got == 0 {
		t.Error("no bypass scrim drawn over the disabled FILTER stage's detail knob area")
	}

	// Re-enable → the scrim must disappear.
	audio.SetInstrumentParam("snare", "filter_enabled", 1)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if got := countScrims(); got != 0 {
		t.Errorf("bypass scrim still drawn (%d) after re-enabling the FILTER stage", got)
	}
}
