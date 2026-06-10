package ui

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// sampler_realtime.go — keeps the Sampler tab's waveform in sync with the live
// synth signal. The displayed buffer is captured from the source instrument's
// rendered one-shot; when the user edits that instrument's synth params (on the
// Synth tab or via the bridge) the capture would otherwise go stale until a
// manual Preview / tab switch. ensureSamplerLoaded re-renders whenever the
// source signature here diverges from the one recorded at capture time.

// samplerSourceSignatureFn returns an opaque fingerprint of an instrument's
// current synth signal. The Sampler re-captures whenever it changes. The
// default fingerprints the live per-instrument param overlay (the values the
// Synth-tab knobs write through audio.SetInstrumentParam); swappable in tests so
// the re-capture trigger can be exercised without the real renderer (which, in
// the test build, ignores params).
var samplerSourceSignatureFn = func(instID string) uint64 {
	// The descriptor signature is folded in so an externally-changed sample
	// edit (import, startup rehydration) re-syncs the open editor, not just
	// live param edits.
	return recipeParamsSignature(audio.GetInstrumentParams(instID)) ^
		audio.SampleEditSignature(instID)
}

// SwapSamplerSourceSignatureFnForTest installs a test fake for the source
// signature provider and returns the previous value, mirroring
// SwapSamplerCaptureFnForTest / SwapSamplerAuditionFnForTest.
func SwapSamplerSourceSignatureFnForTest(fn func(string) uint64) func(string) uint64 {
	prev := samplerSourceSignatureFn
	if fn != nil {
		samplerSourceSignatureFn = fn
	}
	return prev
}

// recipeParamsSignature hashes a parameter overlay into a stable, order-
// independent fingerprint. Two overlays with the same name→value pairs produce
// the same value regardless of map iteration order; any added, removed, or
// changed value produces a different one.
func recipeParamsSignature(p audio.RecipeParams) uint64 {
	if len(p) == 0 {
		return 0
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := fnv.New64a()
	var buf [8]byte
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(p[k]))
		_, _ = h.Write(buf[:])
	}
	return h.Sum64()
}
