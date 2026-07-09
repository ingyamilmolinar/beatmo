//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSynthFullNoteWindowMs_TracksEnvelope(t *testing.T) {
	const inst = "fullnote-window-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	audio.SetInstrumentParam(inst, "amp_attack", 0.01)
	audio.SetInstrumentParam(inst, "amp_decay", 0.10)
	audio.SetInstrumentParam(inst, "amp_release", 0.05)
	short := synthFullNoteWindowMs(inst)

	audio.SetInstrumentParam(inst, "amp_decay", 0.60)
	long := synthFullNoteWindowMs(inst)

	if !(long > short) {
		t.Fatalf("window should grow with decay: short=%d long=%d", short, long)
	}
	if short < 120 || long > 1000 {
		t.Fatalf("window out of clamp range: short=%d long=%d", short, long)
	}
}

func TestSynthFullNoteWindowMs_Clamps(t *testing.T) {
	const inst = "fullnote-window-clamp-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	audio.SetInstrumentParam(inst, "amp_attack", 0.001)
	audio.SetInstrumentParam(inst, "amp_decay", 0.001)
	audio.SetInstrumentParam(inst, "amp_release", 0.001)
	if got := synthFullNoteWindowMs(inst); got != 120 {
		t.Fatalf("tiny envelope must clamp to 120ms, got %d", got)
	}
	audio.SetInstrumentParam(inst, "amp_decay", 5.0)
	if got := synthFullNoteWindowMs(inst); got != 1000 {
		t.Fatalf("huge envelope must clamp to 1000ms, got %d", got)
	}
}

func TestSynthFullNoteWindowMs_NonModularFallback(t *testing.T) {
	// Bespoke drum recipe: no visible amp_attack -> decay-based fallback, still in range.
	const inst = "fullnote-window-drum-test"
	audio.BindInstrumentToRecipe(inst, "drum-kick-punchy")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	got := synthFullNoteWindowMs(inst)
	if got < 120 || got > 1000 {
		t.Fatalf("fallback window out of range: %d", got)
	}
}

func TestPcmMinMaxColumns(t *testing.T) {
	pcm := []float64{0, 1, -1, 0.5, -0.5, 0, 0.25, -0.25}
	mins, maxs := pcmMinMaxColumns(pcm, 4)
	if len(mins) != 4 || len(maxs) != 4 {
		t.Fatalf("want 4 columns, got %d/%d", len(mins), len(maxs))
	}
	if maxs[0] != 1 || mins[0] != 0 {
		t.Fatalf("col0 should span [0,1], got [%g,%g]", mins[0], maxs[0])
	}
	if mins[1] != -1 {
		t.Fatalf("col1 min should be -1, got %g", mins[1])
	}
}

func TestDrawSynthFullNote_RespondsToAmpDecay(t *testing.T) {
	const inst = "fullnote-draw-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	audio.SetInstrumentParam(inst, "env_enabled", 1)

	rect := image.Rect(0, 0, 260, 90)
	render := func(m *synthMirror) int {
		return fingerprintFocus(t, func(img *ebiten.Image) { drawSynthFullNote(img, rect, m) })
	}

	m := &synthMirror{}
	audio.SetInstrumentParam(inst, "amp_decay", 0.05)
	m.renderNowFull(inst, "h-short", func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
	short := render(m)

	audio.SetInstrumentParam(inst, "amp_decay", 1.2)
	m.renderNowFull(inst, "h-long", func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
	long := render(m)

	if short == long {
		t.Fatalf("full-note card identical for 0.05s vs 1.2s decay — envelope not depicted")
	}
}
