package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// RenderDocForGraph (one per platform) renders a document exactly as a trigger of its id plays it, Sampler edit
// included; the -tags test version is a toy voice for UI plumbing only. It BLOCKS until the render lands: call it off
// the UI goroutine.

// Two renders with the same key are the same audio. No tempo: a note sounds the same at every tempo.
func GraphRenderKey(id instrument.ID, d instrument.Doc, pitch float64) uint64 {
	h := SoundSignature(d)
	h ^= mix64(math.Float64bits(pitch))
	if e, ok := SampleEditFor(id); ok {
		h ^= hashSampleEdit(e)
	}
	return h
}

// mix64 is the splitmix64 finaliser (spreads the pitch bits).
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// The flattened block and render settings, never id, name, origin or notes: anything derived from the sound alone (a
// graph render, the dial relevance verdicts) is reused across documents that sound the same.
func SoundSignature(d instrument.Doc) uint64 {
	return instrument.BlockHash(instrument.Flatten(d, BuiltinModuleRegistry())) ^ renderMetaHash(d)
}
