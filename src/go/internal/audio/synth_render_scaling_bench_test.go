//go:build !test && !js

package audio

import (
	"fmt"
	"testing"
	"time"
)

// TestOrganRenderScalingAndBreakdown answers two questions with evidence:
//  1. Is organ render cost linear in the render-window (sample count)? If so,
//     the Beats=2.0 (1.0 s) window is the direct lever on the stall time.
//  2. Where does the full newRecipeAwareVoicePitched cost go beyond the raw C
//     render — MergeRecipeDefaults (per-trigger key/param marshaling) and
//     normalizeAndScale (a full-buffer pass)?
func TestOrganRenderScalingAndBreakdown(t *testing.T) {
	const (
		sr = 44100
		id = "organ-church"
	)
	recipeID := RecipeForInstrument(id)
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		t.Fatalf("no recipe for %s", id)
	}

	// (1) Cost vs render window.
	fmt.Printf("\n-- organ-church render cost vs window size --\n%-12s %10s %12s\n", "window(s)", "samples", "render(ms)")
	for _, sec := range []float64{0.25, 0.5, 1.0, 2.0} {
		samples := int(sec * sr)
		merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
		merged["pitch"] = 5.0
		buf := make([]float32, samples)
		// warm the CPU/caches once
		recipe.Render(buf, sr, samples, 0, merged)
		best := time.Hour
		for i := 0; i < 5; i++ {
			for j := range buf {
				buf[j] = 0
			}
			s := time.Now()
			recipe.Render(buf, sr, samples, 0, merged)
			if d := time.Since(s); d < best {
				best = d
			}
		}
		fmt.Printf("%-12.2f %10d %12.2f\n", sec, samples, float64(best.Nanoseconds())/1e6)
	}

	// (2) Full-path breakdown at the natural window.
	samples, _ := instrumentDurationSamples(id, 120, sr)
	buf := make([]float32, samples)

	timeIt := func(n int, fn func()) time.Duration {
		best := time.Hour
		for i := 0; i < n; i++ {
			s := time.Now()
			fn()
			if d := time.Since(s); d < best {
				best = d
			}
		}
		return best
	}

	mergeCost := timeIt(50, func() {
		m := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
		m["pitch"] = 5.0
		_ = hashRecipeParams(m)
	})
	merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
	merged["pitch"] = 5.0
	renderCost := timeIt(5, func() {
		for j := range buf {
			buf[j] = 0
		}
		recipe.Render(buf, sr, samples, 0, merged)
	})
	normCost := timeIt(50, func() { normalizeAndScale(buf, id) })

	fmt.Printf("\n-- full newRecipeAwareVoicePitched cost breakdown (natural %d-sample window) --\n", samples)
	fmt.Printf("MergeRecipeDefaults+hash : %8.3f ms   (paid EVERY trigger, even cache hits)\n", ms(mergeCost))
	fmt.Printf("recipe.Render (C DSP)    : %8.3f ms   (paid on cache MISS only)\n", ms(renderCost))
	fmt.Printf("normalizeAndScale        : %8.3f ms   (paid on cache MISS only)\n", ms(normCost))
	fmt.Printf("=> a cache MISS stalls the dispatch goroutine ~%.1f ms; a cache HIT costs ~%.3f ms (merge+lookup)\n\n",
		ms(mergeCost)+ms(renderCost)+ms(normCost), ms(mergeCost))
}

func ms(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }
