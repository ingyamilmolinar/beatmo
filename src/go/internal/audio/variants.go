//go:build !test && !js

package audio

import (
	"math"
	"time"
)

// cRenderer matches the Miniaudio C render helpers in drums_c.go.
type cRenderer func(buf []float32, sampleRate, samples int)

// CVariantInstrument is a generic wrapper around a C-rendered buffer plus
// optional post-processing. It allows us to scale to multiple instrument
// "flavours" (kick-1, snare-2, etc.) without duplicating wiring.
type CVariantInstrument struct {
	Render      cRenderer
	RenderParam cParamRenderer                      // optional: parameterized renderer
	Beats       float64                             // base duration in beats
	Post        func(buf []float32, sampleRate int) // optional in-place DSP
	Name        string                              // debug label
}

// NewVoice generates a voice by asking the C renderer to fill a buffer, then
// applying any post-processors. Duration is expressed as a fraction of a beat
// so the same instrument scales sensibly across BPM changes.
// Uses voice cache keyed on (name, bpm, sampleRate) with round-robin selection
// across N pre-rendered variants to avoid the "machine gun effect".
func (v CVariantInstrument) NewVoice(bpm, sampleRate int) Voice {
	if v.Beats <= 0 {
		v.Beats = 0.5
	}
	key := voiceCacheKey{instrumentID: v.Name, bpm: bpm, sampleRate: sampleRate}

	// Try cache hit first (returns round-robin selected variant).
	if buf, ok := globalVoiceCache.Get(key); ok {
		// If not all variants are rendered yet, render one more in the background.
		if !globalVoiceCache.IsFull(key) {
			v.renderAndCache(key, bpm, sampleRate)
		}
		return &cVoice{buf: buf}
	}

	// Cache miss: render and cache the first variant.
	buf := v.renderAndCache(key, bpm, sampleRate)
	return &cVoice{buf: buf}
}

// renderAndCache renders a single voice buffer and stores it in the cache.
// Returns a clone of the buffer for immediate use.
func (v CVariantInstrument) renderAndCache(key voiceCacheKey, bpm, sampleRate int) []float32 {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * v.Beats * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	if samples < 1 {
		samples = 1
	}
	buf := make([]float32, samples)
	if v.Render != nil {
		v.Render(buf, sampleRate, samples)
	}
	if v.Post != nil {
		v.Post(buf, sampleRate)
	}
	baseID := v.Name
	if len(baseID) > 2 && (baseID[len(baseID)-2:] == "-1" || baseID[len(baseID)-2:] == "-2") {
		baseID = baseID[:len(baseID)-2]
	}
	normalizeAndScale(buf, baseID)
	globalVoiceCache.Put(key, buf)
	// Return a clone since the cache stores its own copy.
	clone := make([]float32, len(buf))
	copy(clone, buf)
	return clone
}

// NewVoiceWithParams generates a voice with custom synth parameters.
// If params are default or no parameterized renderer exists, falls back to
// the standard NewVoice path.
func (v CVariantInstrument) NewVoiceWithParams(bpm, sampleRate int, params SynthParams) Voice {
	if params.IsDefault() || v.RenderParam == nil {
		return v.NewVoice(bpm, sampleRate)
	}
	if v.Beats <= 0 {
		v.Beats = 0.5
	}
	key := voiceCacheKey{instrumentID: v.Name, bpm: bpm, sampleRate: sampleRate, synthParams: params}
	if buf, ok := globalVoiceCache.Get(key); ok {
		if !globalVoiceCache.IsFull(key) {
			v.renderParamAndCache(key, bpm, sampleRate, params)
		}
		return &cVoice{buf: buf}
	}
	buf := v.renderParamAndCache(key, bpm, sampleRate, params)
	return &cVoice{buf: buf}
}

// renderParamAndCache renders with synth params and caches.
func (v CVariantInstrument) renderParamAndCache(key voiceCacheKey, bpm, sampleRate int, params SynthParams) []float32 {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * v.Beats * float64(time.Second))
	samples := int(float64(sampleRate) * dur.Seconds())
	if samples < 1 {
		samples = 1
	}
	buf := make([]float32, samples)
	v.RenderParam(buf, sampleRate, samples, params)
	if v.Post != nil {
		v.Post(buf, sampleRate)
	}
	baseID := v.Name
	if len(baseID) > 2 && (baseID[len(baseID)-2:] == "-1" || baseID[len(baseID)-2:] == "-2") {
		baseID = baseID[:len(baseID)-2]
	}
	normalizeAndScale(buf, baseID)
	globalVoiceCache.Put(key, buf)
	clone := make([]float32, len(buf))
	copy(clone, buf)
	return clone
}

// ---- simple DSP helpers for variant flavours ----

// crushBits reduces effective bit depth for a simple lofi character.
func crushBits(buf []float32, bits int) {
	if bits <= 0 || bits >= 23 {
		return
	}
	steps := float32(uint32(1) << uint(bits))
	inv := 1.0 / steps
	for i := range buf {
		x := buf[i]
		buf[i] = float32(math.Round(float64(x*steps))) * inv
	}
}

// softClip applies a lightweight tanh based soft clipping.
func softClip(buf []float32, drive, outGain float32) {
	if drive <= 0 {
		return
	}
	for i := range buf {
		x := float64(buf[i] * drive)
		buf[i] = float32(math.Tanh(x) * float64(outGain))
	}
}

// hpFilter applies a simple one-pole high-pass filter.
func hpFilter(buf []float32, sampleRate int, cutoffHz float64) {
	if cutoffHz <= 0 || sampleRate <= 0 {
		return
	}
	rc := 1.0 / (2 * math.Pi * cutoffHz)
	dt := 1.0 / float64(sampleRate)
	alpha := rc / (rc + dt)
	var prevIn, prevOut float64
	for i := range buf {
		x := float64(buf[i])
		y := alpha * (prevOut + x - prevIn)
		prevIn, prevOut = x, y
		buf[i] = float32(y)
	}
}

// lpFilter applies a simple one-pole low-pass filter.
func lpFilter(buf []float32, sampleRate int, cutoffHz float64) {
	if cutoffHz <= 0 || sampleRate <= 0 {
		return
	}
	rc := 1.0 / (2 * math.Pi * cutoffHz)
	dt := 1.0 / float64(sampleRate)
	alpha := dt / (rc + dt)
	var prev float64
	for i := range buf {
		x := float64(buf[i])
		prev += alpha * (x - prev)
		buf[i] = float32(prev)
	}
}

// gateTail hard-gates the tail after a given fraction of the buffer.
func gateTail(buf []float32, cutoffFrac float64) {
	if cutoffFrac <= 0 || cutoffFrac >= 1 {
		return
	}
	start := int(float64(len(buf)) * cutoffFrac)
	for i := start; i < len(buf); i++ {
		buf[i] = 0
	}
}
