//go:build !test && !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// The Sampler's deterministic "From Synth" source: unlike LatestVoiceSample it needs no prior trigger.
func RenderInstrumentOneShot(id instrument.ID) ([]float32, int) {
	return renderInstrumentOneShotOpts(id, false)
}

// The Sampler shows the UN-edited source and overlays the saved edit itself; capturing edited would apply it twice.
func RenderInstrumentOneShotRaw(id instrument.ID) ([]float32, int) {
	return renderInstrumentOneShotOpts(id, true)
}

func renderInstrumentOneShotOpts(id instrument.ID, ignoreEdit bool) ([]float32, int) {
	sr := SampleRate()
	// Sampler tab capture — UI goroutine, not audioLoop dispatch.
	v, ok := tryRecipeVoiceOptsOrigin(id, sr, ignoreEdit, originPrewarm)
	if !ok {
		v = legacyNewVoice(id, sr)
	}
	if v == nil {
		return nil, sr
	}
	const maxSeconds = 4
	maxSamples := sr * maxSeconds
	out := make([]float32, 0, sr/2)
	if bv, ok := v.(BlockVoice); ok {
		block := make([]float64, 1024)
		for len(out) < maxSamples {
			n, done := bv.SampleBlock(block)
			for i := 0; i < n; i++ {
				out = append(out, float32(block[i]))
			}
			if done {
				break
			}
		}
	} else {
		for len(out) < maxSamples {
			s, done := v.Sample()
			out = append(out, float32(s))
			if done {
				break
			}
		}
	}
	return out, sr
}
