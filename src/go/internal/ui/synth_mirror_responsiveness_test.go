//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthMirror_RenderLiveHashGated guards that the real-time mirror render
// (called every frame from drawSynthTab) only does work when a knob actually
// changed — a frame with no change is a cheap hash compare, and a change moves
// the prior wave to the ghost.
func TestSynthMirror_RenderLiveHashGated(t *testing.T) {
	// renderLive is synchronous and needs no pool — build the struct directly so
	// this test never touches the shared "synth.preview" pool (releasing it would
	// disturb sibling mirror tests).
	m := &synthMirror{}
	renders := 0
	render := func(string) []float64 { renders++; return []float64{1, 2} }
	m.renderLive("x", "h1", render)
	m.renderLive("x", "h1", render) // same hash → no work
	if renders != 1 {
		t.Fatalf("hash-gated renderLive should render once for an unchanged hash, got %d", renders)
	}
	m.renderLive("x", "h2", render) // changed → render + demote prior to ghost
	if renders != 2 {
		t.Fatalf("expected 2 renders after a hash change, got %d", renders)
	}
	if len(m.ghostForTest()) != 2 {
		t.Fatalf("prior wave should become the ghost, got len %d", len(m.ghostForTest()))
	}
}

func pcmEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSynthMirrorRespondsToTimbreKnobs is the UI-level guard that the "Your
// sound" wave re-renders when a TIMBRE knob changes (osc shape, filter, drive,
// FM) — the knobs that change the wave SHAPE the cycle-normalized trace shows.
// (Pitch / envelope / gain affect time or level, not the cycle shape, and have
// their own per-knob pictures; the audio-level
// TestRenderInstrumentPreview_RespondsToEveryStageParam covers all params.)
func TestSynthMirrorRespondsToTimbreKnobs(t *testing.T) {
	g := setupModularSynthGame(t)
	dv := g.drum
	const inst = "modular"

	render := func() []float64 {
		dv.requestSynthMirror(inst)
		dv.synthMirror.drainForTest()
		return append([]float64(nil), dv.synthMirror.pcmForTest()...)
	}
	t.Cleanup(func() {
		if dv.synthMirror != nil {
			dv.synthMirror.closeForTest()
		}
		audio.ResetInstrumentParams(inst)
	})

	cases := []struct {
		name, enable, param string
		value               float64
	}{
		{"osc shape", "", "osc_type", 1},
		{"filter cutoff", "filter_enabled", "filter_cutoff", 250},
		{"drive", "drive_enabled", "drive", 0.85},
		{"fm depth", "fm_enabled", "fm_op2_depth", 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			audio.ResetInstrumentParams(inst)
			if c.enable != "" {
				audio.SetInstrumentParam(inst, c.enable, 1)
			}
			base := render()
			if len(base) == 0 {
				t.Fatal("mirror produced no PCM")
			}
			audio.SetInstrumentParam(inst, c.param, c.value)
			changed := render()
			if pcmEqual(base, changed) {
				t.Errorf("mirror did not re-render when %s (%s=%v) changed", c.name, c.param, c.value)
			}
		})
	}
}
