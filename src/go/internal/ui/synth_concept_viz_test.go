//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// conceptVizRect is the standard target rect used by the concept-viz tests.
func conceptVizRect() image.Rectangle { return image.Rect(0, 0, 120, 60) }

// bindModularConceptInst points a throwaway instrument id at the unified
// modular recipe (which exposes the full param surface — fm_op*_depth,
// lfo_rate, drive, gain, pitchenv_*) and resets its params on cleanup.
func bindModularConceptInst(t *testing.T, instID string) {
	t.Helper()
	audio.BindInstrumentToRecipe(instID, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })
}

// countNonBgRects counts filled drawRect calls whose color is NOT the
// scope-background wash. The scope-bg fill is chrome; everything else is
// "ink" the renderer drew to depict the concept.
func countNonBgRects(rects []drawnRect) int {
	bg := color.RGBAModel.Convert(WithAlpha(genColorVizScopeBg, AlphaOverlay)).(color.RGBA)
	n := 0
	for _, r := range rects {
		if r.Color != bg {
			n++
		}
	}
	return n
}

// nonBgInkArea sums the filled pixel area of every non-background rect. This is
// the right "how much did the renderer draw" metric when a knob makes an
// existing mark BIGGER (e.g. a taller FM bar) rather than adding more marks —
// counting rects alone would miss that growth.
func nonBgInkArea(rects []drawnRect) int {
	bg := color.RGBAModel.Convert(WithAlpha(genColorVizScopeBg, AlphaOverlay)).(color.RGBA)
	area := 0
	for _, r := range rects {
		if r.Color != bg {
			area += r.Rect.Dx() * r.Rect.Dy()
		}
	}
	return area
}

// TestConceptRendererForGroup_TotalCoverage — every param group resolves to a
// non-nil concept renderer (the registry is total).
func TestConceptRendererForGroup_TotalCoverage(t *testing.T) {
	groups := []string{"osc", "filter", "env", "fm", "post", "pitch", "lfo", "burst", "core", "generic", "voice", ""}
	for _, g := range groups {
		if conceptRendererForGroup(g) == nil {
			t.Errorf("group %q has no concept renderer", g)
		}
	}
}

// TestEveryRecipeGroupHasExplicitConceptRenderer — the real discipline guard:
// every param GROUP that ANY registered recipe actually uses must have an
// EXPLICIT entry in conceptRenderers. The old switch had a `default: conceptOsc`
// that silently painted the oscillator wave under unrelated knobs (the 44 "core"
// + 9 "generic" params), so a new group would inherit a misleading picture with
// no test failure. This enumerates the live groups so that can't happen again.
func TestEveryRecipeGroupHasExplicitConceptRenderer(t *testing.T) {
	live := map[string]string{} // group -> example "recipe.param"
	for recipeID, reg := range audio.RecipeRegistrations() {
		if reg == nil {
			continue
		}
		for _, d := range reg.Params {
			// Hidden params (Phase-8A migrated drum/FM stage knobs) are never laid
			// out as a visible knob, so they never reach a concept renderer —
			// exclude their group from the guard.
			if d.Group == audio.SynthHiddenGroup {
				continue
			}
			if _, seen := live[d.Group]; !seen {
				live[d.Group] = recipeID + "." + d.Name
			}
		}
	}
	if len(live) == 0 {
		t.Fatal("no registered recipe params found — registry empty?")
	}
	for group, example := range live {
		if _, ok := conceptRenderers[group]; !ok {
			t.Errorf("param group %q (e.g. %s) has no EXPLICIT entry in conceptRenderers — "+
				"it would silently fall back to the value bar. Add an intended renderer.",
				group, example)
		}
	}
}

// TestConceptValueBar_FillScalesWithValue — the per-knob value bar (core /
// generic / voice groups) fills proportionally to the knob's value within its
// [Min,Max] range, so a higher value paints more fill ink.
func TestConceptValueBar_FillScalesWithValue(t *testing.T) {
	const inst = "concept-valbar-test"
	bindModularConceptInst(t, inst)
	def := audio.ParamDef{Name: "gain", Min: 0, Max: 1.5}
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()

	audio.SetInstrumentParam(inst, "gain", 0.1)
	lowInk := nonBgInkArea(collectFilledRects(t, func() { conceptValueBar(dst, r, inst, def, nil) }))
	if lowInk < 1 {
		t.Fatalf("conceptValueBar drew no non-bg ink at low value, want >=1")
	}

	audio.SetInstrumentParam(inst, "gain", 1.4)
	highInk := nonBgInkArea(collectFilledRects(t, func() { conceptValueBar(dst, r, inst, def, nil) }))

	if highInk <= lowInk {
		t.Errorf("value-bar fill did not grow with value: low=%d high=%d", lowInk, highInk)
	}
}

// TestConceptValueBar_GhostTickAddsInk — a ghost snapshot with a DIFFERENT value
// for the knob adds a faint "before" tick, so total ink exceeds the no-ghost case.
func TestConceptValueBar_GhostTickAddsInk(t *testing.T) {
	const inst = "concept-valbar-ghost-test"
	bindModularConceptInst(t, inst)
	def := audio.ParamDef{Name: "gain", Min: 0, Max: 1.5}
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	audio.SetInstrumentParam(inst, "gain", 0.5)

	noGhost := countNonBgRects(collectFilledRects(t, func() { conceptValueBar(dst, r, inst, def, nil) }))
	withGhost := countNonBgRects(collectFilledRects(t, func() {
		conceptValueBar(dst, r, inst, def, map[string]float64{"gain": 1.4})
	}))
	if withGhost <= noGhost {
		t.Errorf("value-bar ghost tick did not add ink: noGhost=%d withGhost=%d", noGhost, withGhost)
	}
}

// TestConceptFM_DrawsInkAndScalesWithDepth — conceptFM draws at least one
// non-background bar, and more FM depth produces more ink.
func TestConceptFM_DrawsInkAndScalesWithDepth(t *testing.T) {
	const inst = "concept-fm-test"
	bindModularConceptInst(t, inst)
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()

	// Low depth.
	audio.SetInstrumentParam(inst, "fm_op2_depth", 0.2)
	lowRects := collectFilledRects(t, func() { conceptFM(dst, r, inst, audio.ParamDef{}, nil) })
	if countNonBgRects(lowRects) < 1 {
		t.Fatalf("conceptFM drew no non-bg bars at low depth, want >=1")
	}
	lowInk := nonBgInkArea(lowRects)

	// High depth. More FM depth makes the bar TALLER (more filled area), not
	// necessarily more bars — so compare ink AREA, not rect count.
	audio.SetInstrumentParam(inst, "fm_op2_depth", 8.0)
	highRects := collectFilledRects(t, func() { conceptFM(dst, r, inst, audio.ParamDef{}, nil) })
	highInk := nonBgInkArea(highRects)

	if highInk <= lowInk {
		t.Errorf("conceptFM ink area did not grow with depth: low=%d high=%d", lowInk, highInk)
	}
}

// TestConceptMotion_LFORateAddsWiggles — raising lfo_rate increases the number
// of derivative sign-changes in motionCurveSamples (a faster LFO = more
// oscillations across the window).
func TestConceptMotion_LFORateAddsWiggles(t *testing.T) {
	const inst = "concept-motion-test"
	bindModularConceptInst(t, inst)

	// Disable the pitch-env so the curve is dominated by the LFO wobble, and
	// enable the LFO so it contributes.
	audio.SetInstrumentParam(inst, "lfo_enabled", 1)
	audio.SetInstrumentParam(inst, "lfo_depth", 1.0)
	audio.SetInstrumentParam(inst, "pitchenv_amt", 0)

	const n = 256
	signChanges := func() int {
		s := motionCurveSamples(inst, n)
		changes := 0
		prev := 0
		for i := 1; i < len(s); i++ {
			d := s[i] - s[i-1]
			sign := 0
			switch {
			case d > 0:
				sign = 1
			case d < 0:
				sign = -1
			}
			if sign != 0 && prev != 0 && sign != prev {
				changes++
			}
			if sign != 0 {
				prev = sign
			}
		}
		return changes
	}

	audio.SetInstrumentParam(inst, "lfo_rate", 2)
	slow := signChanges()
	audio.SetInstrumentParam(inst, "lfo_rate", 30)
	fast := signChanges()

	if fast <= slow {
		t.Errorf("motionCurveSamples wiggles did not grow with lfo_rate: slow=%d fast=%d", slow, fast)
	}

	// conceptMotion must also draw something non-trivial.
	dst := ebiten.NewImage(120, 60)
	rects := collectFilledRects(t, func() { conceptMotion(dst, conceptVizRect(), inst, audio.ParamDef{}, nil) })
	if countNonBgRects(rects) < 1 {
		t.Errorf("conceptMotion drew no non-bg ink")
	}
}

// TestConceptOsc_GhostAddsInk — conceptOsc draws a faint "before" wave when the
// pre-drag oscillator params differ from the live ones.
func TestConceptOsc_GhostAddsInk(t *testing.T) {
	const inst = "concept-osc-ghost-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "osc_type", 0) // sine
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	no := countNonBgRects(collectFilledRects(t, func() { conceptOsc(dst, r, inst, audio.ParamDef{}, nil) }))
	ghost := conceptMergedParams(inst)
	ghost["osc_type"] = 2 // square — a clearly different shape
	with := countNonBgRects(collectFilledRects(t, func() { conceptOsc(dst, r, inst, audio.ParamDef{}, ghost) }))
	if with <= no {
		t.Errorf("conceptOsc ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// TestConceptEnvelope_GhostAddsInk — conceptEnvelope overlays a faint "before"
// ADSR when the pre-drag envelope params differ.
func TestConceptEnvelope_GhostAddsInk(t *testing.T) {
	const inst = "concept-env-ghost-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "amp_decay", 0.2)
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	no := countNonBgRects(collectFilledRects(t, func() { conceptEnvelope(dst, r, inst, audio.ParamDef{}, nil) }))
	ghost := conceptMergedParams(inst)
	ghost["amp_decay"] = 1.8
	ghost["amp_sustain"] = 0.1
	with := countNonBgRects(collectFilledRects(t, func() { conceptEnvelope(dst, r, inst, audio.ParamDef{}, ghost) }))
	if with <= no {
		t.Errorf("conceptEnvelope ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// TestConceptFilter_GhostAddsInk — conceptFilter overlays a faint "before"
// response curve when the pre-drag filter params differ.
func TestConceptFilter_GhostAddsInk(t *testing.T) {
	const inst = "concept-filter-ghost-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "filter_cutoff", 14000)
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	no := countNonBgRects(collectFilledRects(t, func() { conceptFilter(dst, r, inst, audio.ParamDef{}, nil) }))
	ghost := conceptMergedParams(inst)
	ghost["filter_cutoff"] = 400
	with := countNonBgRects(collectFilledRects(t, func() { conceptFilter(dst, r, inst, audio.ParamDef{}, ghost) }))
	if with <= no {
		t.Errorf("conceptFilter ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// TestConceptFM_GhostAddsInk — conceptFM overlays a faint "before" WAVEFORM when
// the pre-drag value of the SELECTED knob differs. (Updated for the new contract:
// conceptFM now renders the FM output waveform, and its ghost is keyed on the
// selected knob's pre-drag value — `def.Name` + `ghost[def.Name]` — not the old
// depth-bar markers driven by a whole params snapshot.)
func TestConceptFM_GhostAddsInk(t *testing.T) {
	const inst = "concept-fm-ghost-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "fm_op2_depth", 1.0)
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	def := audio.ParamDef{Name: "fm_op2_ratio", Group: "fm", Min: 0.5, Max: 8}
	no := countNonBgRects(collectFilledRects(t, func() { conceptFM(dst, r, inst, def, nil) }))
	ghost := map[string]float64{"fm_op2_ratio": 7.0}
	with := countNonBgRects(collectFilledRects(t, func() { conceptFM(dst, r, inst, def, ghost) }))
	if with <= no {
		t.Errorf("conceptFM ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// TestConceptMotion_GhostAddsInk — conceptMotion overlays a faint "before"
// modulation curve when the pre-drag motion params differ.
func TestConceptMotion_GhostAddsInk(t *testing.T) {
	const inst = "concept-motion-ghost-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "lfo_enabled", 1)
	audio.SetInstrumentParam(inst, "lfo_depth", 1.0)
	audio.SetInstrumentParam(inst, "lfo_rate", 4)
	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()
	no := countNonBgRects(collectFilledRects(t, func() { conceptMotion(dst, r, inst, audio.ParamDef{}, nil) }))
	ghost := conceptMergedParams(inst)
	ghost["lfo_rate"] = 24
	with := countNonBgRects(collectFilledRects(t, func() { conceptMotion(dst, r, inst, audio.ParamDef{}, ghost) }))
	if with <= no {
		t.Errorf("conceptMotion ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// TestConceptPost_GhostAddsInk — conceptPost draws a non-empty curve, and a
// ghost snapshot with a different drive adds a second (faint) curve, so total
// ink is strictly greater than the no-ghost case.
func TestConceptPost_GhostAddsInk(t *testing.T) {
	const inst = "concept-post-test"
	bindModularConceptInst(t, inst)
	audio.SetInstrumentParam(inst, "drive", 0.2)
	audio.SetInstrumentParam(inst, "gain", 1.0)

	dst := ebiten.NewImage(120, 60)
	r := conceptVizRect()

	noGhost := collectFilledRects(t, func() { conceptPost(dst, r, inst, audio.ParamDef{}, nil) })
	noGhostInk := countNonBgRects(noGhost)
	if noGhostInk < 1 {
		t.Fatalf("conceptPost drew %d non-bg rects without ghost, want >=1", noGhostInk)
	}

	ghost := map[string]float64{"drive": 0.95, "gain": 1.0}
	withGhost := collectFilledRects(t, func() { conceptPost(dst, r, inst, audio.ParamDef{}, ghost) })
	withGhostInk := countNonBgRects(withGhost)

	if withGhostInk <= noGhostInk {
		t.Errorf("conceptPost ghost did not add ink: noGhost=%d withGhost=%d", noGhostInk, withGhostInk)
	}
}
