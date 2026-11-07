//go:build !test && !js

package audio

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The cache-hit path every trigger after a warm-up takes, re-entered on every knob-drag invalidation: the only
// allocation may be the cVoice struct. Measured by MemStats.Mallocs: AllocsPerRun is too coarse across several
// functions.
func TestSynthTabHotPathAllocBudget(t *testing.T) {
	t.Cleanup(func() {
		ResetInstrumentParams(roster.Snare)
		globalVoiceCache.ClearInstrument(roster.Snare)
	})

	// The cache-hit path dominates the cumulative budget.
	const samples = 22050
	const sampleRate = 44100
	buf := make([]float32, samples)
	for i := range buf {
		buf[i] = float32(i) / float32(samples)
	}
	key := voiceCacheKey{
		instrumentID: roster.Snare,
		sampleRate:   sampleRate,
		paramsHash:   0,
	}
	globalVoiceCache.Put(key, buf)

	// Warm up — first call may pay sticky JIT / interface table costs.
	for i := 0; i < 16; i++ {
		v, _ := globalVoiceCache.Get(key)
		_ = &cVoice{buf: v}
	}

	// Force a GC before measuring so the Mallocs counter is steady.
	runtime.GC()
	var pre, post runtime.MemStats
	runtime.ReadMemStats(&pre)

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		v, _ := globalVoiceCache.Get(key)
		_ = &cVoice{buf: v}
	}
	runtime.ReadMemStats(&post)

	mallocs := post.Mallocs - pre.Mallocs
	bytes := post.TotalAlloc - pre.TotalAlloc
	perCall := float64(mallocs) / float64(iterations)
	bytesPerCall := float64(bytes) / float64(iterations)

	// The cVoice struct plus runtime slack.
	const allocBudget = 1.2
	const bytesBudget = 32.0 // cVoice = ptr + int = 24 bytes; +slack for alignment

	if perCall > allocBudget {
		t.Errorf("synth-tab hot-path allocs/call = %.2f exceeds budget %.1f "+
			"— a clone or other alloc has re-entered the hot path. "+
			"Check voiceCache.Get and tryRecipeVoice for re-introduced "+
			"make+copy calls; the per-trigger contract is SHARED backing "+
			"array (cVoice never writes to v.buf).",
			perCall, allocBudget)
	}
	if bytesPerCall > bytesBudget {
		t.Errorf("synth-tab hot-path bytes/call = %.1f exceeds budget %.1f "+
			"— at the 22 Hz steady-state production trigger rate, %d bytes/call "+
			"projects to %.1f MB/min of pure allocator churn",
			bytesPerCall, bytesBudget, int(bytesPerCall),
			bytesPerCall*22*60/1e6)
	}
	t.Logf("synth-tab hot path: %.2f allocs/call %.1f bytes/call "+
		"(budgets: %.1f, %.1f) — pre-fix was ~4 allocs / ~%d bytes per "+
		"trigger (160 KB clone)", perCall, bytesPerCall, allocBudget,
		bytesBudget, samples*4*2)
}

// The other common branch: guarding only the recipe path would let a clone creep back through the default one.
func TestRecipeAwareVoiceDispatch_LegacyPathAllocBudget(t *testing.T) {
	t.Cleanup(func() {
		ResetInstrumentParams(roster.Snare)
		globalVoiceCache.Clear()
	})

	// Ensure no user params for snare so we take the legacy branch.
	ResetInstrumentParams(roster.Snare)

	// So one-shot lookups do not land in the measured window.
	for i := 0; i < 16; i++ {
		_ = newRecipeAwareVoice(roster.Snare, 44100)
	}

	allocs := testing.AllocsPerRun(200, func() {
		_ = newRecipeAwareVoice(roster.Snare, 44100)
	})

	// At least the struct and its buffer; 4 leaves room for dispatch overhead.
	const budget = 4.0
	if allocs > budget {
		t.Errorf("legacy dispatch allocs/call = %.1f exceeds budget %.0f",
			allocs, budget)
	}
	t.Logf("legacy dispatch allocs/call = %.1f (budget %.0f)", allocs, budget)
}
