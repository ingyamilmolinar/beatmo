package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// graph_render.go — the audio the Synth tab's graphs are drawn from
// (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md §4.3).
//
// RenderDocForGraph (platform files) renders an instrument DOCUMENT — the live
// one, or a hypothetical one (a dial turned to an end) — exactly the way a
// trigger of instrument id plays it on this platform, Sampler edit included:
//   - desktop: the voice-cache render (C → document FX → gate tail →
//     peak-normalise × amplitude → Sampler edit), graph_render_native.go;
//   - browser: the playback render worker through window.renderDocPCM, which
//     renders like ensureRenderedSample (graph_render_wasm.go);
//   - -tags test: a deterministic toy voice for UI plumbing tests only
//     (graph_render_teststub.go) — never linked into a real binary.
// It BLOCKS until the render lands: call it off the UI goroutine.

// GraphRenderKey identifies a graph render: the document's flattened block +
// render metadata, the pitch and id's Sampler edit. Two renders with the same
// key are the same audio. There is no tempo: a note sounds the same at every
// tempo.
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

// SoundSignature identifies what a document SOUNDS like: its flattened param
// block and its render settings (note length, amplitude, gate tail, melodic,
// document FX) — never its id, name, origin or notes. Two documents with one
// signature render the same note, so anything derived from the sound alone
// (a graph render, the dial relevance verdicts) is reused across them.
func SoundSignature(d instrument.Doc) uint64 {
	return instrument.BlockHash(instrument.Flatten(d, BuiltinModuleRegistry())) ^ renderMetaHash(d)
}
