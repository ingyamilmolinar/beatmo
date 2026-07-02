package synthmatch

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// resolveRender builds the merged recipe params (defaults + instrument params +
// overrides + pitch) and the sample count for instID. Shared by the production
// and test renderers so param resolution is identical in both build modes.
func resolveRender(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (recipeID string, merged audio.RecipeParams, samples int, err error) {
	recipeID = audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return "", nil, 0, fmt.Errorf("synthmatch: no recipe for instrument %q", instID)
	}
	merged = audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	if merged == nil {
		merged = audio.RecipeParams{}
	}
	for k, v := range overrides {
		merged[k] = v
	}
	merged["pitch"] = pitch
	if durSec <= 0 {
		durSec = audio.ConfigForInstrument(instID).DurationSec
	}
	samples = int(float64(sr) * durSec)
	if samples < 1 {
		return "", nil, 0, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	return recipeID, merged, samples, nil
}
