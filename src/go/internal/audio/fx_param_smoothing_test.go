package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const smoothSR = 44100

// maxAbsDelta returns the maximum absolute sample-to-sample difference.
func maxAbsDelta(samples []float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	m := 0.0
	for i := 1; i < len(samples); i++ {
		d := math.Abs(samples[i] - samples[i-1])
		if d > m {
			m = d
		}
	}
	return m
}

// sineAt returns a sample of a sine wave at the given index.
func sineAt(freq, amp float64, i int) float64 {
	return amp * math.Sin(2*math.Pi*freq*float64(i)/float64(smoothSR))
}

// The change lands at the signal's peak, where a discontinuity is most detectable.
func peakIdx(freq float64, minSamples int) int {
	period := float64(smoothSR) / freq
	cycles := math.Ceil(float64(minSamples) / period)
	return int(math.Round((cycles + 0.25) * period))
}

// An unsmoothed mix jumps from the dry level (~0.3) to the saturated wet level (~0.995) in one sample, far beyond the
// sine's natural step (~0.019).
func TestDistortionMixNoZipperNoise(t *testing.T) {
	const freq = 440.0
	const amp = 0.3

	d := newDistortion(smoothSR, map[string]float64{"drive": 10, "tone": 4000, "mix": 0})

	changeAt := peakIdx(freq, 2000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = d.ProcessSample(sineAt(freq, amp, i))
	}

	// Change mix mid-stream without reset.
	d.SetParam("mix", 1.0)

	for i := changeAt; i < total; i++ {
		output[i] = d.ProcessSample(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// A ~5 ms ramp keeps the step near the sine's own; an unsmoothed one exceeds 10×.
	if ratio > 4.0 {
		t.Errorf("distortion mix 0->1 caused discontinuity: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"parameter change needs smoothing to avoid zipper noise",
			changeDelta, ratio, steadyDelta)
	}
}

// An instant time change moves the read pointer to an unrelated sample: a click.
func TestDelayTimeNoPopOnChange(t *testing.T) {
	const freq = 441.0 // non-round to avoid phase alignment with delay times
	const amp = 0.3

	d := newDelay(smoothSR, map[string]float64{"time": 110, "feedback": 0.3, "mix": 1.0})

	changeAt := peakIdx(freq, 20000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = d.ProcessSample(sineAt(freq, amp, i))
	}

	// Change delay time — read pointer jumps to unrelated buffer position.
	d.SetParam("time", 370)

	for i := changeAt; i < total; i++ {
		output[i] = d.ProcessSample(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[5000 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// Crossfaded read pointers stay smooth; a jump pops far past 3×.
	if ratio > 3.0 {
		t.Errorf("delay time 110->370ms caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 3.0x)\n"+
			"delay time change needs crossfade to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// A fresh biquad with zeroed state outputs ~0 where the old one output the full sine: a massive spike.
func TestFilterCutoffNoBiquadTransient(t *testing.T) {
	const freq = 440.0
	const amp = 0.5

	f := newFilter(smoothSR, map[string]float64{"mode": 0, "cutoff": 10000, "q": 0.707, "mix": 1})

	changeAt := peakIdx(freq, 2000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = f.ProcessSample(sineAt(freq, amp, i))
	}

	// Drastically lower cutoff — rebuilds biquad with zeroed state.
	f.SetParam("cutoff", 200)

	for i := changeAt; i < total; i++ {
		output[i] = f.ProcessSample(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// A zeroed biquad spikes past 10×.
	if ratio > 5.0 {
		t.Errorf("filter cutoff 10kHz->200Hz caused transient: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 5.0x)\n"+
			"biquad coefficient change needs interpolation to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// Swapping the chain in one sample jumps from the distorted level (~0.995) to dry (~0.3).
func TestEffectToggleNoCrossfadePop(t *testing.T) {
	resetChains(t)
	const freq = 440.0
	const amp = 0.3

	id := "smooth-toggle"
	ch := InstrumentChannel(channel.Of(instrument.ID(id)))

	AddInsertEffect(instrument.ID(id), EffectDistortion, map[string]float64{"drive": 10, "tone": 4000, "mix": 1})

	changeAt := peakIdx(freq, 2000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = ch.ProcessSampleLocal(sineAt(freq, amp, i))
	}

	// Disable the effect — processor chain swapped atomically.
	ToggleInsertEffect(instrument.ID(id), 0, false)

	for i := changeAt; i < total; i++ {
		output[i] = ch.ProcessSampleLocal(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// tanh makes steep zero crossings, so the ratio is modest (~4×) while the absolute pop is large.
	if ratio > 3.0 {
		t.Errorf("effect toggle off caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 3.0x)\n"+
			"toggling needs crossfade to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// A mix jump halves the dry signal instantly and adds the accumulated tail at once: a click far beyond the sine step.
func TestReverbMixNoClickOnChange(t *testing.T) {
	const freq = 440.0
	const amp = 0.3

	r := newReverb(smoothSR, map[string]float64{"room": 0.5, "damping": 0.5, "mix": 0})

	changeAt := peakIdx(freq, 5000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = r.ProcessSample(sineAt(freq, amp, i))
	}

	// At mix 0 the combs still accumulate.
	r.SetParam("mix", 0.5)

	for i := changeAt; i < total; i++ {
		output[i] = r.ProcessSample(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// Unsmoothed, the jump exceeds 4×.
	if ratio > 4.0 {
		t.Errorf("reverb mix 0->0.5 caused click: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"mix parameter needs smoothing to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// A depth jump reallocates (zeroes) the buffer and moves the read position hundreds of samples, dropping the wet
// signal.
func TestChorusDepthNoPopOnChange(t *testing.T) {
	const freq = 441.0
	const amp = 0.3

	c := newChorus(smoothSR, map[string]float64{"rate": 1.5, "depth": 0, "mix": 0.8})

	changeAt := peakIdx(freq, 5000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = c.ProcessSample(sineAt(freq, amp, i))
	}

	// Increasing depth reallocates buffer (zeroed) and jumps read position.
	c.SetParam("depth", 15)

	for i := changeAt; i < total; i++ {
		output[i] = c.ProcessSample(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	if ratio > 4.0 {
		t.Errorf("chorus depth 0->15ms caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"depth change needs smoothing to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}
