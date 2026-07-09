//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestFocusGraph_BypassedStageDimsAndHints is Task 10's honesty gate: the
// focus graph's picture always shows what a knob WOULD do (conceptMotion
// force-shows the LFO wobble independent of the live enable state — see
// conceptStageEnableParam), but the card must SAY when the stage is actually
// bypassed rather than silently implying the sound changes. Selects the LFO
// stage's Rate knob (via the crop_synth_focus_motion scene, the same
// mechanism TestSynthFocusSceneSelectsIntendedKnob uses), pins lfo_depth so
// the underlying curve is identical in both renders, and flips only
// lfo_enabled — the fingerprint must differ (the scrim + OFF hint painted on
// top), proving the honesty scrim is wired into drawSynthFocusGraph.
//
// Lives in a //go:build test file because fingerprintFocus (defined in
// synth_focus_sweep_test.go) relies on the swappable drawRect var / stubbed
// Ebiten fast path.
func TestFocusGraph_BypassedStageDimsAndHints(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := RunScene(g, "crop_synth_focus_motion"); err != nil {
		t.Fatalf("RunScene(crop_synth_focus_motion): %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = g.Update()
	}
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Fatal("no resolved synth instrument")
	}
	sec := dv.synthSelectedSection()
	if sec == nil {
		t.Fatal("no selected synth section (expected LFO)")
	}
	rect := image.Rect(0, 0, 220, 90)

	// Pin lfo_depth to the SAME value in both renders so conceptMotion's own
	// curve is identical — only the honesty scrim (gated on lfo_enabled) may
	// differ between the two fingerprints.
	audio.SetInstrumentParam(inst, "lfo_depth", 0.6)
	audio.SetInstrumentParam(inst, "lfo_enabled", 0)

	// Warm-up render: drawRoundedRect's cornerSpriteCache (drawing.go) is a
	// package-level cache populated lazily on first use per (radius, color)
	// key — its construction goes through the SAME drawRect var the
	// fingerprint intercepts, so an uncached first call paints extra
	// corner-antialiasing rects that a cached repeat call does not. Without
	// this warm-up, "off" vs "on" would spuriously differ by cache-miss vs
	// cache-hit rects instead of by the scrim under test.
	_ = fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthFocusGraph(img, rect, inst) })

	off := fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthFocusGraph(img, rect, inst) })

	audio.SetInstrumentParam(inst, "lfo_depth", 0.6)
	audio.SetInstrumentParam(inst, "lfo_enabled", 1)
	on := fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthFocusGraph(img, rect, inst) })

	if off == on {
		t.Fatalf("bypassed LFO stage must dim the focus graph + show the OFF hint (off fingerprint == on fingerprint = %d)", off)
	}
}
