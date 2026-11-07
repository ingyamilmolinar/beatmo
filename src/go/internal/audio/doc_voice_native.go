//go:build !test && !js

package audio

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// THE native synth voice: a document flattened and rendered, then its voice fx, gate and normalisation, then the
// Sampler edit.

// docInstrument is every synth instrument in the instrument table.
type docInstrument struct{ id instrument.ID }

func (d docInstrument) NewVoice(sampleRate int) Voice {
	buf, _, ok := docVoiceBuffer(d.id, sampleRate, 0, false, false, originDispatch)
	if !ok {
		return nil
	}
	return &cVoice{buf: buf}
}

// Without copying one when it exists: the dispatch hot path.
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

// From the stored block and hash, with no allocation (the hot path); equals docVoiceKey(id, current document).
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

// The pitch is written for a pitched render.
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

// The buffer is shared read-only with the cache.
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
		// A late first note beats a silent one (user decision); it costs only the instrument's first trigger.
	}
	d, ok := InstrumentDoc(id)
	if !ok {
		return nil, voiceCacheKey{}, false
	}
	buf := renderDocIntoCache(id, d, sampleRate, pitch, pitched, ignoreEdit, origin)
	return buf, key, true
}

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

// C render → document FX → gate tail → peak-normalise × amplitude: the ONE implementation behind the cache and
// RenderAsShipped.
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

// Flattens exactly as docVoiceKey does. No cache, no globals.
func renderDocVoice(d instrument.Doc, sampleRate int, pitch float64, pitched bool) []float32 {
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	if pitched {
		block[pitchFieldIndex()] = pitch
	}
	return renderDocBlock(d, block, sampleRate)
}

// What a trigger of d at (pitch, duration 1) puts into the mix, for a document registered nowhere: melodic renders
// AT pitch at rate 1, the rest resampled by 2^(pitch/12) through playedVoice's resampleVoice. Writes no global
// state, so tools may call it from several goroutines.
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

// RAW samples, before normalisation and amplitude: the path every override render uses.
func RenderInstrumentWithFields(id instrument.ID, sr, n int, fields map[string]float64) ([]float32, error) {
	d, err := InstrumentDocWithFields(id, fields)
	if err != nil {
		return nil, err
	}
	return RenderDocRaw(d, sr, n), nil
}

// No FX, gate or normalisation: the cross-platform reference's form.
func RenderDocRaw(d instrument.Doc, sr, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, modularBlock(instrument.Flatten(d, BuiltinModuleRegistry())))
	return buf
}

// 0 or >= 1 = no gate.
func gateTail(buf []float32, cutoffFrac float64) {
	if cutoffFrac <= 0 || cutoffFrac >= 1 {
		return
	}
	start := int(float64(len(buf)) * cutoffFrac)
	for i := start; i < len(buf); i++ {
		buf[i] = 0
	}
}
