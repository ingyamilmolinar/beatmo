package ui

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The waveform is a capture of the instrument's one-shot; without this signature it would go stale after a
// synth edit until a manual Preview or tab switch.

// Fingerprints the live param overlay. A var so tests can exercise the re-capture without the real renderer,
// which ignores params in the test build.
var samplerSourceSignatureFn = func(instID instrument.ID) uint64 {
	// The sample-edit signature is folded in so an external edit (import, startup rehydration) re-syncs too.
	return recipeParamsSignature(audio.GetInstrumentParams(instID)) ^
		audio.SampleEditSignature(instID)
}

func SwapSamplerSourceSignatureFnForTest(fn func(instrument.ID) uint64) func(instrument.ID) uint64 {
	prev := samplerSourceSignatureFn
	if fn != nil {
		samplerSourceSignatureFn = fn
	}
	return prev
}

// Order-independent: the same pairs give the same value whatever the map iteration order.
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
