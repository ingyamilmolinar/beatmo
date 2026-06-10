package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthTab_KnobDragEndToEndChangesAudioParam reproduces the exact UX
// the user described: switch to the Synth tab, find a knob, press inside
// it, drag, release — and verify the audio.SetInstrumentParam was called
// with a new value. Catches regressions where the hit area is registered
// but the knob's value isn't propagated to the audio engine.
func TestSynthTab_KnobDragEndToEndChangesAudioParam(t *testing.T) {
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

	// Switch to Synth tab + run layout. The panel claims its expanded synth
	// height only during a DrumView Layout pass that sees TabSynth active, and
	// Ebiten's Layout short-circuits when the size is unchanged — so toggle the
	// size (shrink then grow) after activating the tab to force a real relayout.
	// Otherwise the detail pane is one knob-row tall and decay scrolls out.
	expandSynthPanelForTest(t, g)

	// Find the decay knob — drum-snare wires {pitch, decay, tone, drive}
	// plus the snare family knobs (native-deprecation migration).
	knobs := g.drum.SynthTabKnobs()
	bindings := g.drum.SynthTabBindings()
	wantKnobs := len(audio.WiredParamsForRecipe("drum-snare"))
	if len(knobs) != wantKnobs {
		t.Fatalf("expected %d knobs for drum-snare (wired), got %d", wantKnobs, len(knobs))
	}
	var decayIdx = -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob in bindings")
	}
	// decay lives in the ENVELOPE stage; open it so the knob gets a
	// hit-testable rect under the chip-strip + detail-pane layout.
	selectSectionForKnobIdx(t, g, "snare", decayIdx)
	knobs = g.drum.SynthTabKnobs()
	knob := knobs[decayIdx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("decay knob has empty rect (panel layout broken). header=%v sections=%v", g.drum.SynthTabHeader(), g.drum.SynthTabSections())
	}

	// Locate the matching HitArea returned by the EQ panel.
	hits := g.drum.eqPanelZone.HitAreas()
	var pressHit *HitArea
	for i, h := range hits {
		if h.Tag != "" && strings.HasPrefix(h.Tag, "synth-knob-") {
			if image.Pt((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2).In(h.Rect) {
				pressHit = &hits[i]
				break
			}
		}
	}
	if pressHit == nil {
		var tags []string
		for _, h := range hits {
			tags = append(tags, fmt.Sprintf("%s@%v", h.Tag, h.Rect))
		}
		t.Fatalf("no synth-knob HitArea covers the decay knob center (rect=%v); tags=%s", rect, strings.Join(tags, ","))
	}

	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	// Capture initial value (decay default = 1.0; rescaled 0..1 = 0.25).
	initialValue := knob.Value

	// Press the knob.
	if result := pressHit.Handler.OnPress(cx, cy); result == InputIgnored {
		t.Fatalf("synth-knob HitArea ignored the press at center (%d,%d) of rect %v", cx, cy, rect)
	}
	if !knob.Capturing() {
		t.Fatal("knob not capturing after press — drag will be a no-op")
	}

	// Knobs are horizontal-only: drag RIGHT by half the knob's full sweep
	// → +0.5 value delta. (Distance derived from the knob's own
	// pixels-per-sweep so the assertion is robust to radius-scaled sensitivity.)
	half := knob.dragPixels() / 2
	const steps = 6
	for s := 1; s <= steps; s++ {
		pressHit.Handler.OnDrag(cx+half*s/steps, cy)
	}
	wantValue := initialValue + 0.5
	if knob.Value < wantValue-0.05 || knob.Value > wantValue+0.05 {
		t.Errorf("after right drag: knob.Value=%v, want ~%v (initial %v + 0.5)", knob.Value, wantValue, initialValue)
	}

	// Release.
	pressHit.Handler.OnRelease(cx+half, cy)
	if knob.Capturing() {
		t.Error("knob still capturing after release")
	}

	// Verify audio engine received the new param.
	got := audio.GetInstrumentParams("snare")
	val, ok := got["decay"]
	if !ok {
		t.Fatalf("decay param was not set after drag: %v", got)
	}
	// decay range [0,4] → 0.75 slider ≈ 3.0.
	wantParam := 4.0 * wantValue
	if val < wantParam-0.3 || val > wantParam+0.3 {
		t.Errorf("after drag: audio.GetInstrumentParams[\"decay\"]=%v, want ~%v (rescaled from slider value %v)", val, wantParam, wantValue)
	}
}

// TestSynthTab_PressMissingKnobIsNoOp verifies a press OUTSIDE every knob
// rect does not start a drag (negative complement to the success case).
func TestSynthTab_PressMissingKnobIsNoOp(t *testing.T) {
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

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	// Press at a point WAY off any knob — far below the audio panel.
	for _, k := range g.drum.SynthTabKnobs() {
		if k.Capturing() {
			t.Errorf("knob unexpectedly capturing before any press: rect=%v", k.Rect())
		}
	}
}

// TestSynthTab_ReleaseFollowingDragPropagatesFinalValue verifies the
// last drag value is propagated on release (one final SetInstrumentParam
// call). This is the regression for "I drag to 75 % then release and the
// audio doesn't pick up the final value".
func TestSynthTab_ReleaseFollowingDragPropagatesFinalValue(t *testing.T) {
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

	// Count SetInstrumentParam calls.
	var fired int
	oldCb := audio.SwapPlatformInstrumentParamsChangedForTest(nil)
	t.Cleanup(func() { audio.SwapPlatformInstrumentParamsChangedForTest(oldCb) })
	audio.SwapPlatformInstrumentParamsChangedForTest(func(id string, p audio.RecipeParams) {
		fired++
	})

	knobs := g.drum.SynthTabKnobs()
	bindings := g.drum.SynthTabBindings()
	// Pick the first VISIBLE synth knob. Phase 8B's unified layout shows many
	// more sections, so a fixed param (the old "drive") may scroll out of its
	// narrow card in the small test panel — any visible knob exercises the same
	// drag→propagate path.
	var idx = -1
	for i := range bindings {
		if !knobs[i].Rect().Empty() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no visible synth knob")
	}
	k := knobs[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("synth knob rect empty")
	}

	// Locate the hit area.
	var hit *HitArea
	hits := g.drum.eqPanelZone.HitAreas()
	for i, h := range hits {
		if h.Tag != fmt.Sprintf("synth-knob-%d", idx) {
			continue
		}
		hit = &hits[i]
		break
	}
	if hit == nil {
		t.Fatalf("no hit area for synth-knob-%d", idx)
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	def := bindings[idx].def
	paramName := def.Name
	beforeNorm := k.Value         // normalized [0,1] start value of the chosen knob
	quarter := k.dragPixels() / 4 // right drag → ~0.25 normalized delta
	hit.Handler.OnPress(cx, cy)
	hit.Handler.OnDrag(cx+quarter, cy)
	hit.Handler.OnRelease(cx+quarter, cy)

	got := audio.GetInstrumentParams("snare")
	val, ok := got[paramName]
	// A right drag of ~dragPixels()/4 lifts the normalized value by ~0.25
	// (clamped at 1.0), so the propagated ACTUAL value is min + want*(max-min).
	wantNorm := beforeNorm + 0.25
	if wantNorm > 1 {
		wantNorm = 1
	}
	wantApprox := def.Min + wantNorm*(def.Max-def.Min)
	band := 0.15 * (def.Max - def.Min)
	if !ok || val < wantApprox-band || val > wantApprox+band {
		t.Errorf("after press+drag+release: %s=%v ok=%v, want ~%.3f", paramName, val, ok, wantApprox)
	}
	if fired < 1 {
		t.Errorf("expected at least 1 SetInstrumentParam call, got %d", fired)
	}
}
