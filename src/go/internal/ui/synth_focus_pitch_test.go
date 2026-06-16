//go:build test

package ui

import (
	"image"
	"reflect"
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// rendererName resolves a conceptRenderer func value to its fully-qualified
// function name so tests can assert which renderer a router returned without
// comparing func values (Go forbids func == func).
func rendererName(fn conceptRenderer) string {
	if fn == nil {
		return "<nil>"
	}
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

func TestSynthFocusRendererForKnob_RoutesPitchKnobsToPitchWave(t *testing.T) {
	for _, name := range []string{"osc_octave", "osc_detune", "pitch", "tune"} {
		def := audio.ParamDef{Name: name, Group: "osc", Min: 0, Max: 1}
		// Pitch knobs must NOT route to the plain group renderer's shape view.
		got := rendererName(synthFocusRendererForKnob(def))
		if got != rendererName(conceptPitchWave) {
			t.Errorf("knob %s routed to %s, want conceptPitchWave", name, got)
		}
	}
	// A non-pitch knob still routes by group.
	cut := audio.ParamDef{Name: "filter_cutoff", Group: "filter", Min: 0, Max: 1}
	if rendererName(synthFocusRendererForKnob(cut)) != rendererName(conceptFilter) {
		t.Errorf("filter_cutoff should route to conceptFilter")
	}
}

func TestConceptPitchWave_RespondsToPitch(t *testing.T) {
	inst := "focus-pitch-test"
	bindModularConceptInst(t, inst)
	def := audio.ParamDef{Name: "osc_octave", Group: "osc", Min: -2, Max: 2}

	fp := func(v float64) int {
		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, "osc_octave", v)
		rects := collectFilledRects(t, func() {
			conceptPitchWave(ebiten.NewImage(260, 110), image.Rect(0, 0, 260, 110), inst, def, nil)
		})
		h := len(rects)
		for _, r := range rects {
			h = h*31 + r.Rect.Min.Y + r.Rect.Max.X
		}
		return h
	}
	if fp(-2) == fp(2) {
		t.Fatalf("conceptPitchWave identical at octave -2 vs +2 (pitch not reflected)")
	}
}
