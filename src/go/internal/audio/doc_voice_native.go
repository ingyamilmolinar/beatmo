//go:build !test && !js

package audio

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// doc_voice_native.go — THE native synth voice (spec §8): an instrument
// document, flattened, rendered, then its voice fx, its gate and the
// instrument's normalisation, then the Sampler edit.

// docInstrument is every synth instrument in the instrument table.
type docInstrument struct{ id instrument.ID }

func (d docInstrument) NewVoice(sampleRate int) Voice {
	buf, _, ok := docVoiceBuffer(d.id, sampleRate, 0, false, false, originDispatch)
	if !ok {
		return nil
	}
	return &cVoice{buf: buf}
}

// ensureDoc makes sure id has a project document without copying one when it
// already does (the dispatch hot path).
func ensureDoc(id instrument.ID) bool {
	if HasInstrumentDoc(id) {
		return true
	}
	_, ok := EnsureInstrumentDoc(id)
	return ok
}

// docMelodic reports whether id's document renders at the node pitch.
func docMelodic(id instrument.ID) bool {
	return ensureDoc(id) && instrumentDocMelodic(id)
}

var (
	pitchIdxOnce sync.Once
	pitchIdx     int
)

// pitchFieldIndex is the block index of the modular "pitch" field.
func pitchFieldIndex() int {
	pitchIdxOnce.Do(func() { pitchIdx, _ = BuiltinModuleRegistry().FieldIndex("pitch") })
	return pitchIdx
}

// liveVoiceKey is the cache key of id's CURRENT document, computed from the
// stored block and hash with no allocation (the dispatch hot path). It equals
// docVoiceKey(id, current document).
func liveVoiceKey(id instrument.ID, sampleRate int, pitch float64, pitched, ignoreEdit bool) (voiceCacheKey, bool) {
	block, hash, salt, ok := docBlockForDispatch(id)
	if !ok {
		return voiceCacheKey{}, false
	}
	key := voiceCacheKey{instrumentID: id, sampleRate: sampleRate, paramsHash: hash}
	if pitched {
		key.paramsHash = instrument.BlockHashWith(block, pitchFieldIndex(), pitch) ^ salt
		key.pitch = roundPitchForCache(pitch)
	}
	if !ignoreEdit {
		if edit, ok := SampleEditFor(id); ok {
			key.paramsHash ^= hashSampleEdit(edit)
		}
	}
	return key, true
}

func effectSlotsOf(fx []instrument.Module) []EffectSlot {
	out := make([]EffectSlot, len(fx))
	for i, m := range fx {
		out[i] = EffectSlot{Type: EffectType(m.Type[len("fx."):]), Enabled: m.On, Params: map[string]float64(m.Values)}
	}
	return out
}

// docVoiceKey flattens d (pitch written for a pitched render) and returns the
// cache key dispatch uses, plus the block.
func docVoiceKey(id instrument.ID, d instrument.Doc, sampleRate int, pitch float64, pitched, ignoreEdit bool) (voiceCacheKey, []float64) {
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	key := voiceCacheKey{instrumentID: id, sampleRate: sampleRate}
	if pitched {
		block[pitchFieldIndex()] = pitch
		key.pitch = roundPitchForCache(pitch)
	}
	key.paramsHash = instrument.BlockHash(block) ^ renderMetaHash(d)
	if !ignoreEdit {
		if edit, ok := SampleEditFor(id); ok {
			key.paramsHash ^= hashSampleEdit(edit)
		}
	}
	return key, block
}

// docVoiceBuffer renders (or fetches from the voice cache) the voice of id's
// project document. The returned buffer is shared read-only with the cache.
func docVoiceBuffer(id instrument.ID, sampleRate int, pitch float64, pitched, ignoreEdit bool, origin renderOrigin) ([]float32, voiceCacheKey, bool) {
	if !ensureDoc(id) {
		return nil, voiceCacheKey{}, false
	}
	key, ok := liveVoiceKey(id, sampleRate, pitch, pitched, ignoreEdit)
	if !ok {
		return nil, voiceCacheKey{}, false
	}
	if buf, ok := globalVoiceCache.Get(key); ok {
		recordVoiceCacheHit()
		return buf, key, true
	}
	if pitched && origin == originDispatch && dispatchMustNotRender.Load() {
		if v, ok := serveBestAvailable(id, pitch); ok {
			return v.(*cVoice).buf, key, true
		}
		// Nothing cached at all — render below: a late first note beats a
		// silent one (user decision), bounded to the instrument's first trigger.
	}
	d, ok := InstrumentDoc(id)
	if !ok {
		return nil, voiceCacheKey{}, false
	}
	buf := renderDocIntoCache(id, d, sampleRate, pitch, pitched, ignoreEdit, origin)
	return buf, key, true
}

// renderDocIntoCache renders d's voice for id and stores it under its key
// (no-op returning the cached buffer when already present).
func renderDocIntoCache(id instrument.ID, d instrument.Doc, sampleRate int, pitch float64, pitched, ignoreEdit bool, origin renderOrigin) []float32 {
	key, block := docVoiceKey(id, d, sampleRate, pitch, pitched, ignoreEdit)
	if buf, ok := globalVoiceCache.Get(key); ok {
		return buf
	}
	start := time.Now()
	buf := renderDocBlock(d, block, sampleRate)
	recordVoiceRender(time.Since(start), origin)
	if !ignoreEdit {
		if edit, ok := SampleEditFor(id); ok {
			buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
		}
	}
	globalVoiceCache.Put(key, buf)
	return buf
}

// renderDocBlock is the voice buffer of d given its already-flattened block:
// C render → document FX → gate tail → peak-normalise × d's amplitude. The
// ONE implementation behind the voice cache and RenderAsShipped.
func renderDocBlock(d instrument.Doc, block []float64, sampleRate int) []float32 {
	n := VoiceFrames(d.Render.Seconds, sampleRate)
	buf := make([]float32, n)
	renderModularBlock(buf, sampleRate, n, modularBlock(block))
	if len(d.FX) > 0 {
		applyDefaultFX(buf, sampleRate, effectSlotsOf(d.FX))
	}
	if d.Render.GateTail > 0 {
		gateTail(buf, d.Render.GateTail)
	}
	normalizeAndScaleAmp(buf, float32(d.Render.Amplitude))
	return buf
}

// renderDocVoice flattens d (writing pitch when pitched, exactly as
// docVoiceKey does) and renders its voice buffer. No cache, no globals.
func renderDocVoice(d instrument.Doc, sampleRate int, pitch float64, pitched bool) []float32 {
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	if pitched {
		block[pitchFieldIndex()] = pitch
	}
	return renderDocBlock(d, block, sampleRate)
}

// RenderAsShipped returns the samples a trigger of document d at (pitch,
// duration 1) puts into the mix — RenderAsPlayed for a document that need not
// be registered anywhere. Melodic documents render AT pitch and play at rate
// 1; everything else renders at its own pitch and is resampled by
// 2^(pitch/12) through the same resampleVoice playedVoice uses. Writes no
// global state, so analysis tools may call it from several goroutines.
// Guard: TestRenderAsShippedMatchesPlayback.
func RenderAsShipped(d instrument.Doc, sampleRate int, pitch float64) []float64 {
	var buf []float32
	rate := 1.0
	if d.Render.Melodic {
		buf = renderDocVoice(d, sampleRate, pitch, true)
	} else {
		buf = renderDocVoice(d, sampleRate, 0, false)
		rate = pow2(pitch / 12.0)
	}
	v := &resampleVoice{src: &cVoice{buf: buf}, step: rate, buf: make([]float64, 0, len(buf)+2)}
	out := make([]float64, 0, len(buf))
	for {
		s, done := v.Sample()
		if done {
			return out
		}
		out = append(out, s)
	}
}

// RenderInstrumentWithFields renders id's document with fields set through it
// (see InstrumentDocWithFields): n RAW samples at sr — the C voice exactly as
// the engine produces it, before playback normalization and amplitude. The
// render path every tool that "renders an instrument with overrides" uses.
func RenderInstrumentWithFields(id instrument.ID, sr, n int, fields map[string]float64) ([]float32, error) {
	d, err := InstrumentDocWithFields(id, fields)
	if err != nil {
		return nil, err
	}
	return RenderDocRaw(d, sr, n), nil
}

// RenderDocRaw is the RAW C render of a document that need not be registered
// anywhere (n samples at sr, no FX / gate / normalisation) — the
// cross-platform reference's form (cmd/export_audio's document cases).
func RenderDocRaw(d instrument.Doc, sr, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, modularBlock(instrument.Flatten(d, BuiltinModuleRegistry())))
	return buf
}

// gateTail hard-gates the tail after a given fraction of the buffer (a
// document's render.gate_tail; 0 or >= 1 = no gate).
func gateTail(buf []float32, cutoffFrac float64) {
	if cutoffFrac <= 0 || cutoffFrac >= 1 {
		return
	}
	start := int(float64(len(buf)) * cutoffFrac)
	for i := start; i < len(buf); i++ {
		buf[i] = 0
	}
}
