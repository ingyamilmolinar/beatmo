package audio

import (
	"encoding/json"
	"math"
)

// No platform dependencies, so the baked buffer is identical on desktop, WASM and the test build.

// trim → reverse → resample(pitch) → normalize → gain → fades. Normalize before gain, or it would cancel the gain.
type SampleEdit struct {
	StartFrac      float64 // [0,1] over the source buffer
	EndFrac        float64 // [0,1] over the source buffer
	TransposeSemis float64 // whole semitones, +/-
	DetuneCents    float64 // cents, +/- (100 cents == 1 semitone)
	GainDB         float32 // output gain in dB
	Fade           bool    // SampleFadeMs fade-in and fade-out (the Sampler's Fade toggle)
	Reverse        bool
	Normalize      bool
}

// The fade length travels as milliseconds; the length itself is Go's.
func SampleEditWire(e SampleEdit) []byte {
	wire := struct {
		SampleEdit
		FadeMs float64
	}{SampleEdit: e}
	if e.Fade {
		wire.FadeMs = SampleFadeMs
	}
	data, _ := json.Marshal(wire)
	return data
}

// The toggle is the only control, so the length is not a setting.
const SampleFadeMs = 5.0

// SetSampleEdit treats an identity edit as a clear.
func (e SampleEdit) isIdentity() bool {
	return e.StartFrac == 0 && e.EndFrac == 1 &&
		e.TransposeSemis == 0 && e.DetuneCents == 0 &&
		e.GainDB == 0 && !e.Fade &&
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

// Fractions clamp to [0,1]; an inverted range is empty.
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

// A positive shift raises pitch and shortens the buffer; the kernel mirrors resampleVoice.
func ResampleSemitones(src []float32, semitones, cents float64) []float32 {
	return resampleByStep(src, math.Pow(2, (semitones+cents/100.0)/12.0))
}

// How a recording at another rate enters either engine, the same Go on both. A Kaiser-windowed sinc cut just below the
// lower Nyquist, so 44.1 kHz neither images nor dulls at 48 kHz and 96 kHz does not alias. round(len × toSR / fromSR)
// samples.
func ResampleToRate(src []float32, fromSR, toSR int) []float32 {
	if fromSR <= 0 || toSR <= 0 || fromSR == toSR {
		return append([]float32(nil), src...)
	}
	n := len(src)
	ratio := float64(toSR) / float64(fromSR)
	out := make([]float32, int(math.Round(float64(n)*ratio)))
	cut := 0.97 * math.Min(1, ratio) // fraction of the source Nyquist kept
	half := rateConvZeroCrossings / cut
	// Tabulated over half the symmetric kernel: evaluating the Bessel window per tap took seconds in the browser.
	table := make([]float64, int(math.Ceil(half*rateConvTableSteps))+2)
	for j := range table {
		x := float64(j) / rateConvTableSteps
		table[j] = cut * sincPi(cut*x) * kaiserWindow(x/half)
	}
	kernel := func(x float64) float64 {
		p := math.Abs(x) * rateConvTableSteps
		j := int(p)
		if j+1 >= len(table) {
			return 0
		}
		return table[j] + (table[j+1]-table[j])*(p-float64(j))
	}
	for i := range out {
		t := float64(i) / ratio
		lo := max(0, int(math.Ceil(t-half)))
		hi := min(n-1, int(math.Floor(t+half)))
		var acc float64
		for k := lo; k <= hi; k++ {
			acc += float64(src[k]) * kernel(float64(k)-t)
		}
		out[i] = float32(acc)
	}
	return out
}

const (
	rateConvZeroCrossings = 16
	rateConvKaiserBeta    = 8.6 // ≈ 90 dB stop band
	rateConvTableSteps    = 512
)

func sincPi(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

// kaiserWindow is the Kaiser window at u ∈ [-1, 1] (0 outside).
func kaiserWindow(u float64) float64 {
	if u <= -1 || u >= 1 {
		return 0
	}
	return besselI0(rateConvKaiserBeta*math.Sqrt(1-u*u)) / besselI0(rateConvKaiserBeta)
}

// besselI0 is the modified Bessel function of the first kind, order 0.
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	q := x * x / 4
	for k := 1; k < 64 && term > sum*1e-17; k++ {
		term *= q / float64(k*k)
		sum += term
	}
	return sum
}

// One function on both platforms, so a WAV at any rate plays at the same pitch and length everywhere. sr ≤ 0 is the
// engine's rate.
func pcmAtEngineRate(pcm []float32, sr, engine int) []float32 {
	if sr <= 0 || sr == engine {
		return pcm
	}
	return ResampleToRate(pcm, sr, engine)
}

// step > 1 shortens / raises pitch. Mirrors resampleVoice.
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

// In place; silence is untouched.
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

// ApplyFades applies a linear fade-in and fade-out of fadeMs each, in place.
func ApplyFades(buf []float32, sampleRate int, fadeMs float64) []float32 {
	n := len(buf)
	fi := min(int(float64(sampleRate)*fadeMs/1000.0), n)
	for i := 0; i < fi; i++ {
		buf[i] *= float32(i) / float32(fi)
	}
	for k := 0; k < fi; k++ {
		buf[n-1-k] *= float32(k) / float32(fi)
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
	if e.Fade {
		out = ApplyFades(out, sampleRate, SampleFadeMs)
	}
	return out
}
