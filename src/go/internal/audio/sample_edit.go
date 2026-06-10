package audio

import "math"

// Pure []float32 sample-editing transforms used by the Sampler tab. They have
// no platform dependencies (no build tag) so they run identically on desktop,
// WASM, and the -tags test fast path, and the baked buffer is deterministic
// across all three.

// SampleEdit describes the non-destructive edit applied to a source buffer.
// BakeSample renders it into a final buffer in a fixed order:
//
//	trim -> reverse -> resample(pitch) -> normalize -> gain -> fades
//
// Normalize runs before gain so the gain knob trims the level after the peak
// has been brought to 0 dBFS (gain-then-normalize would cancel the gain).
type SampleEdit struct {
	StartFrac      float64 // [0,1] over the source buffer
	EndFrac        float64 // [0,1] over the source buffer
	TransposeSemis float64 // whole semitones, +/-
	DetuneCents    float64 // cents, +/- (100 cents == 1 semitone)
	GainDB         float32 // output gain in dB
	FadeInMs       float64
	FadeOutMs      float64
	Reverse        bool
	Normalize      bool
}

// isIdentity reports whether the edit is a render-time no-op: the full
// [0,1] range kept and every transform at its neutral value. Storing an
// identity edit as a descriptor would only waste a cache-key perturbation,
// so SetSampleEdit treats it as a clear.
func (e SampleEdit) isIdentity() bool {
	return e.StartFrac == 0 && e.EndFrac == 1 &&
		e.TransposeSemis == 0 && e.DetuneCents == 0 &&
		e.GainDB == 0 && e.FadeInMs == 0 && e.FadeOutMs == 0 &&
		!e.Reverse && !e.Normalize
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// TrimSample returns the [startFrac,endFrac] slice of src as a fresh buffer.
// Fractions clamp to [0,1]; an inverted range yields an empty buffer.
func TrimSample(src []float32, startFrac, endFrac float64) []float32 {
	n := len(src)
	if n == 0 {
		return []float32{}
	}
	s := int(clamp01(startFrac) * float64(n))
	e := min(int(clamp01(endFrac)*float64(n)), n)
	if s >= e {
		return []float32{}
	}
	out := make([]float32, e-s)
	copy(out, src[s:e])
	return out
}

// ReverseSample reverses buf in place and returns it.
func ReverseSample(buf []float32) []float32 {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return buf
}

// ResampleSemitones linearly resamples src by the given pitch shift. A positive
// shift raises pitch and shortens the buffer (it is played faster); the
// interpolation kernel mirrors resampleVoice in engine_resample.go.
func ResampleSemitones(src []float32, semitones, cents float64) []float32 {
	return resampleByStep(src, math.Pow(2, (semitones+cents/100.0)/12.0))
}

// ResampleToRate linearly resamples src from fromSR to toSR. Used to bring a
// loaded WAV to the engine sample rate so native playback (which assumes the
// engine rate) plays at the correct speed. Invalid or equal rates pass through.
func ResampleToRate(src []float32, fromSR, toSR int) []float32 {
	if fromSR <= 0 || toSR <= 0 || fromSR == toSR {
		return append([]float32(nil), src...)
	}
	return resampleByStep(src, float64(fromSR)/float64(toSR))
}

// resampleByStep walks src at the given fractional step using linear
// interpolation (step > 1 shortens / raises pitch). Mirrors resampleVoice in
// engine_resample.go.
func resampleByStep(src []float32, step float64) []float32 {
	n := len(src)
	if n == 0 {
		return []float32{}
	}
	if step <= 0 {
		step = 1
	}
	outN := max(int(float64(n-1)/step)+1, 1)
	out := make([]float32, outN)
	for i := range out {
		pos := float64(i) * step
		i0 := int(pos)
		if i0 >= n-1 {
			out[i] = src[n-1]
			continue
		}
		frac := float32(pos - float64(i0))
		out[i] = src[i0] + (src[i0+1]-src[i0])*frac
	}
	return out
}

// ApplyGainDB scales buf by the linear gain corresponding to db, in place.
func ApplyGainDB(buf []float32, db float32) []float32 {
	if db == 0 {
		return buf
	}
	lin := float32(math.Pow(10, float64(db)/20.0))
	for i := range buf {
		buf[i] *= lin
	}
	return buf
}

// NormalizePeak scales buf so its peak magnitude reaches 0 dBFS, in place.
// Silence is left untouched.
func NormalizePeak(buf []float32) []float32 {
	var peak float32
	for _, v := range buf {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	if peak == 0 {
		return buf
	}
	g := 1.0 / peak
	for i := range buf {
		buf[i] *= g
	}
	return buf
}

// ApplyFades applies linear fade-in/out of the given millisecond lengths, in place.
func ApplyFades(buf []float32, sampleRate int, fadeInMs, fadeOutMs float64) []float32 {
	n := len(buf)
	fi := min(int(float64(sampleRate)*fadeInMs/1000.0), n)
	for i := 0; i < fi; i++ {
		buf[i] *= float32(i) / float32(fi)
	}
	fo := min(int(float64(sampleRate)*fadeOutMs/1000.0), n)
	for k := 0; k < fo; k++ {
		buf[n-1-k] *= float32(k) / float32(fo)
	}
	return buf
}

// BakeSample renders e against src into a final, self-contained buffer.
func BakeSample(src []float32, sampleRate int, e SampleEdit) []float32 {
	out := TrimSample(src, e.StartFrac, e.EndFrac)
	if e.Reverse {
		out = ReverseSample(out)
	}
	if e.TransposeSemis != 0 || e.DetuneCents != 0 {
		out = ResampleSemitones(out, e.TransposeSemis, e.DetuneCents)
	}
	if e.Normalize {
		out = NormalizePeak(out)
	}
	if e.GainDB != 0 {
		out = ApplyGainDB(out, e.GainDB)
	}
	if e.FadeInMs > 0 || e.FadeOutMs > 0 {
		out = ApplyFades(out, sampleRate, e.FadeInMs, e.FadeOutMs)
	}
	return out
}
