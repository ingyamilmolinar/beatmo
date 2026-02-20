//go:build !test && !js

package audio

import (
	"log"
	"os"
)

// bypassNormalize skips peak normalization when set via BYPASS_NORMALIZE=1.
// Use this to test if normalization is causing distortion.
var bypassNormalize = os.Getenv("BYPASS_NORMALIZE") == "1"

func init() {
	if bypassNormalize {
		log.Println("[AUDIO DEBUG] BYPASS_NORMALIZE=1 - Skipping peak normalization")
	}
}

// amplitudeForInstrument returns the amplitude scalar for the given instrument.
// Uses the shared InstrumentConfigs as the single source of truth.
func amplitudeForInstrument(id string) float32 {
	return ConfigForInstrument(id).Amplitude
}

// normalizeAndScale applies peak normalization followed by amplitude scaling
// to match the WASM audio processing pipeline in audio.js.
// This ensures cross-platform audio parity.
func normalizeAndScale(buf []float32, instrumentID string) {
	if bypassNormalize {
		// Just apply amplitude scaling, no normalization
		amp := amplitudeForInstrument(instrumentID)
		for i := range buf {
			buf[i] *= amp
		}
		applyFadeOut(buf)
		return
	}

	// 1. Find peak
	var peak float32
	for _, s := range buf {
		a := s
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}

	// 2. Normalize to peak=1.0
	if peak > 0 {
		inv := 1.0 / peak
		for i := range buf {
			buf[i] *= inv
		}
	}

	// 3. Scale by instrument amplitude
	amp := amplitudeForInstrument(instrumentID)
	for i := range buf {
		buf[i] *= amp
	}

	// 4. Apply fade-out to prevent click at end of sample
	applyFadeOut(buf)
}

// applyFadeOut applies a short exponential fade to the last portion of the buffer
// to prevent clicks when the voice ends. Fades the last ~5ms to zero.
func applyFadeOut(buf []float32) {
	if len(buf) == 0 {
		return
	}

	// Fade duration: ~5ms at 44100Hz = ~220 samples
	// Use a shorter fade for shorter buffers
	fadeSamples := len(buf) / 20 // 5% of buffer length
	if fadeSamples < 64 {
		fadeSamples = 64
	}
	if fadeSamples > 512 {
		fadeSamples = 512 // Cap at ~12ms
	}
	if fadeSamples > len(buf) {
		fadeSamples = len(buf)
	}

	startIdx := len(buf) - fadeSamples
	for i := 0; i < fadeSamples; i++ {
		// Linear fade from 1.0 to 0.0
		t := float32(i) / float32(fadeSamples)
		fade := 1.0 - t
		buf[startIdx+i] *= fade
	}
}

// ExportVoice returns a Voice for the given instrument ID, or nil if not found.
// This is used for cross-platform audio comparison tests.
func ExportVoice(id string, bpm, sampleRate int) Voice {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return nil
	}
	return inst.NewVoice(bpm, sampleRate)
}

// AmplitudeForInstrumentExport returns the amplitude for the given instrument ID.
// This is exported for cross-platform comparison tests.
func AmplitudeForInstrumentExport(id string) float64 {
	return float64(amplitudeForInstrument(id))
}
