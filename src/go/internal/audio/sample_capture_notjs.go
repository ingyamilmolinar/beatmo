//go:build !test && !js

package audio

// RenderInstrumentOneShot renders the active recipe-aware voice for id into a
// fresh []float32 by draining it once. It is the deterministic "From Synth"
// capture source for the Sampler tab — unlike LatestVoiceSample it does not
// depend on the instrument having been played yet.
func RenderInstrumentOneShot(id string) ([]float32, int) {
	return renderInstrumentOneShotOpts(id, false)
}

// RenderInstrumentOneShotRaw renders ignoring any non-destructive sample-edit
// descriptor. The Sampler editor loads through this so it shows the UN-edited
// source waveform and overlays the saved trim/pitch/gain itself — capturing
// through the edited path would double-apply the edit.
func RenderInstrumentOneShotRaw(id string) ([]float32, int) {
	return renderInstrumentOneShotOpts(id, true)
}

func renderInstrumentOneShotOpts(id string, ignoreEdit bool) ([]float32, int) {
	sr := SampleRate()
	b := bpm
	if b <= 0 {
		b = 120
	}
	v, ok := tryRecipeVoiceOpts(id, b, sr, ignoreEdit)
	if !ok {
		v = legacyNewVoice(id, b, sr)
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
