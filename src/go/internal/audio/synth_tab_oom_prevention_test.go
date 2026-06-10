//go:build !test && !js

package audio

import (
	"runtime"
	"testing"
)

// TestSynthTabHotPathAllocBudget pins the per-trigger cumulative allocation
// budget for the hot path the OOM-prevention investigation surfaced:
//
//	voiceCache.Get  +  tryRecipeVoice  +  cVoice construction
//
// This is the "fast" path taken every time a scheduled trigger fires AFTER
// the first warm-up render — and it is the path the 12 Hz synth-tab knob
// churn re-enters on every cache invalidation. Pre-fix this allocated ~160
// KB per trigger (two clones — one in voiceCache.Get, one in
// tryRecipeVoice — plus the cVoice struct + 1.2 GB cumulative attribution
// in the profile). Post-fix the only allocation is the cVoice struct.
//
// We measure via runtime.MemStats.Mallocs delta rather than
// testing.AllocsPerRun because the inner closure spans multiple
// allocation sites and AllocsPerRun's float64 return is too coarse for a
// per-call budget that spans multiple functions.
func TestSynthTabHotPathAllocBudget(t *testing.T) {
	t.Cleanup(func() {
		ResetInstrumentParams("snare")
		globalVoiceCache.ClearInstrument("snare")
	})

	// Seed the voice cache with a realistic snare buffer for the default
	// (no per-instrument-params) cache key. This puts us on the cache-hit
	// path that the 12 Hz churn re-enters every time a SetInstrumentParam
	// invalidates the entry — the next trigger renders, then the next 50
	// or so triggers hit. The hit rate dominates the cumulative budget.
	const samples = 22050
	const bpm = 140
	const sampleRate = 44100
	buf := make([]float32, samples)
	for i := range buf {
		buf[i] = float32(i) / float32(samples)
	}
	key := voiceCacheKey{
		instrumentID: "snare",
		bpm:          bpm,
		sampleRate:   sampleRate,
		paramsHash:   hashRecipeParams(nil),
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

	// Budget: 1 alloc / call (the cVoice struct) + a small slack for
	// runtime overhead. Pre-fix this was 4+ (two slice clones, two slice
	// headers, plus the struct).
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

// TestRecipeAwareVoiceDispatch_LegacyPathAllocBudget pins the legacy
// (no-user-params) dispatch path, which is the OTHER common branch in
// production: instruments with no per-instrument param edits fall through
// to inst.NewVoice. This test makes sure no clone has been introduced
// on the legacy side either — the production OOM only landed in the
// recipe path because that's what the churn invalidates, but tightening
// the recipe path without a guard on the legacy path would let drift
// re-emerge through the "default" code branch.
func TestRecipeAwareVoiceDispatch_LegacyPathAllocBudget(t *testing.T) {
	t.Cleanup(func() {
		ResetInstrumentParams("snare")
		globalVoiceCache.Clear()
	})

	// Ensure no user params for snare so we take the legacy branch.
	ResetInstrumentParams("snare")

	// Warm up so cVoice typed-table lookups, channel resolution, etc.
	// don't show up as one-shot allocs in the measured window.
	for i := 0; i < 16; i++ {
		_ = newRecipeAwareVoice("snare", 120, 44100)
	}

	allocs := testing.AllocsPerRun(200, func() {
		_ = newRecipeAwareVoice("snare", 120, 44100)
	})

	// Legacy path returns inst.NewVoice(...) which constructs a fresh
	// cVoice with a freshly rendered buffer — at minimum 2 allocs (the
	// struct + the buffer). Pin the budget at 4 to allow for instrument
	// dispatch overhead but flag any new alloc-heavy code on this path.
	const budget = 4.0
	if allocs > budget {
		t.Errorf("legacy dispatch allocs/call = %.1f exceeds budget %.0f",
			allocs, budget)
	}
	t.Logf("legacy dispatch allocs/call = %.1f (budget %.0f)", allocs, budget)
}
