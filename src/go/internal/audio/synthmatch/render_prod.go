//go:build !test

package synthmatch

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// RenderInstrument renders instID at the given pitch (semitones from A3=220Hz)
// for durSec (0 = instrument default) at sr, returning a mono wave.Wave (float64).
func RenderInstrument(instID string, pitch float64, sr int, durSec float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, nil)
}

// RenderWithParams is RenderInstrument with extra param overrides applied on top
// of the merged recipe defaults (and "pitch") before rendering.
//
// Production build: calls recipe.Render directly (the C DSP path) for
// bit-accurate audio. No normalization, no fallback — silence is a valid result.
func RenderWithParams(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	recipeID, merged, samples, err := resolveRender(instID, pitch, sr, durSec, overrides)
	if err != nil {
		return wave.Wave{}, err
	}
	recipe := audio.NewRecipe(recipeID)
	if recipe == nil {
		return wave.Wave{}, fmt.Errorf("synthmatch: NewRecipe(%q) nil", recipeID)
	}
	buf := make([]float32, samples)
	recipe.Render(buf, sr, samples, 0, merged) // raw output; NO normalization, NO fallback
	out := make([]float64, samples)
	for i, s := range buf {
		out[i] = float64(s)
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: instID}, nil
}
