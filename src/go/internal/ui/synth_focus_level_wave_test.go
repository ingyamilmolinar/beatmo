//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthFocusRenderer_EveryGroupExplicit — every param GROUP that ANY
// registered recipe actually uses must have an EXPLICIT entry in
// synthFocusRenderers, so a recipe that introduces a new group can't silently
// inherit the level-wave fallback (which would be wrong for, say, a new
// time-domain stage).
func TestSynthFocusRenderer_EveryGroupExplicit(t *testing.T) {
	groups := map[string]bool{}
	for _, reg := range audio.RecipeRegistrations() {
		if reg == nil {
			continue
		}
		for _, d := range reg.Params {
			if d.Group == audio.SynthHiddenGroup {
				continue
			}
			groups[d.Group] = true
		}
	}
	if len(groups) == 0 {
		t.Fatal("no registered recipe params found — registry empty?")
	}
	for g := range groups {
		if _, ok := synthFocusRenderers[g]; !ok {
			t.Errorf("group %q has no explicit focus renderer", g)
		}
	}
}

// TestSynthFocusRendererForGroup_Total — the resolver is total: every group
// (known or not) yields a non-nil renderer, and unknown groups fall back to the
// level wave (not a misleading domain-specific curve).
func TestSynthFocusRendererForGroup_Total(t *testing.T) {
	for _, g := range []string{"osc", "filter", "env", "fm", "post", "pitchenv", "lfo", "burst", "core", "generic", "voice", "", "totally-unknown-group"} {
		if synthFocusRendererForGroup(g) == nil {
			t.Errorf("group %q resolved to a nil focus renderer", g)
		}
	}
}

// TestConceptFocusLevelWave_AmplitudeTracksValue — the level wave's display
// amplitude scales with the selected knob's value fraction, so a higher gain
// paints a TALLER wave (more filled ink area). The fill runs from the curve to
// the centerline, so a larger amplitude excursion ⇒ more filled rows per column.
func TestConceptFocusLevelWave_AmplitudeTracksValue(t *testing.T) {
	const inst = "focus-levelwave-amp"
	bindModularConceptInst(t, inst)
	def := audio.ParamDef{Name: "gain", Group: "post", Min: 0, Max: 1.5}

	low := countInkForLevelWave(t, inst, def, map[string]float64{"gain": 0.2})
	high := countInkForLevelWave(t, inst, def, map[string]float64{"gain": 1.4})
	if low < 1 {
		t.Fatalf("level wave drew no ink at low value, want >=1")
	}
	if high <= low {
		t.Fatalf("higher gain should paint more ink: low=%d high=%d", low, high)
	}
}

// TestConceptFocusLevelWave_GhostAddsInk — a ghost snapshot at a DIFFERENT
// amplitude draws a faint second curve, so total ink exceeds the no-ghost case.
func TestConceptFocusLevelWave_GhostAddsInk(t *testing.T) {
	const inst = "focus-levelwave-ghost"
	bindModularConceptInst(t, inst)
	def := audio.ParamDef{Name: "gain", Group: "post", Min: 0, Max: 1.5}
	audio.SetInstrumentParam(inst, "gain", 1.4)

	dst := ebiten.NewImage(300, 120)
	r := image.Rect(0, 0, 300, 120)
	no := countNonBgRects(collectFilledRects(t, func() { conceptFocusLevelWave(dst, r, inst, def, nil) }))
	with := countNonBgRects(collectFilledRects(t, func() {
		conceptFocusLevelWave(dst, r, inst, def, map[string]float64{"gain": 0.2})
	}))
	if with <= no {
		t.Errorf("level-wave ghost did not add ink: noGhost=%d withGhost=%d", no, with)
	}
}

// countInkForLevelWave sets the override params on the bound instrument and
// returns the non-background filled ink AREA produced by conceptFocusLevelWave.
// Area (not rect count) is the metric that tracks amplitude: a louder knob makes
// the fill from curve→centerline TALLER, not more numerous.
func countInkForLevelWave(t *testing.T, inst string, def audio.ParamDef, override map[string]float64) int {
	t.Helper()
	for k, v := range override {
		audio.SetInstrumentParam(inst, k, v)
	}
	dst := ebiten.NewImage(300, 120)
	r := image.Rect(0, 0, 300, 120)
	return nonBgInkArea(collectFilledRects(t, func() { conceptFocusLevelWave(dst, r, inst, def, nil) }))
}
