package audio

import (
	"math"
	"testing"
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

// peakIdx returns a sample index where a sine wave at freq is near its positive
// peak, after at least minSamples of warmup. This ensures the parameter change
// occurs when the signal is at maximum amplitude, making discontinuities most
// detectable.
func peakIdx(freq float64, minSamples int) int {
	period := float64(smoothSR) / freq
	cycles := math.Ceil(float64(minSamples) / period)
	return int(math.Round((cycles + 0.25) * period))
}

// TestDistortionMixNoZipperNoise verifies that changing the distortion mix
// parameter mid-stream does not cause an audible discontinuity.
//
// Without parameter smoothing, mix jumps from 0 to 1 in a single sample,
// causing the output to leap from the dry signal level (~0.3) to the saturated
// wet level (~0.995 via tanh(10*0.3)). This amplitude jump of ~0.7 far exceeds
// the natural sample-to-sample variation of the sine wave (~0.019).
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
	// With smoothing (~5ms ramp), the per-sample mix change is tiny, keeping
	// the transition delta close to the natural sine delta (ratio ~1.0).
	// Without smoothing, the ratio exceeds 10x.
	if ratio > 4.0 {
		t.Errorf("distortion mix 0->1 caused discontinuity: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"parameter change needs smoothing to avoid zipper noise",
			changeDelta, ratio, steadyDelta)
	}
}

// TestDelayTimeNoPopOnChange verifies that changing the delay time mid-stream
// does not cause an audible pop from the read-pointer jump.
//
// Without smoothing, changing delay time instantly moves the read position in
// the circular buffer to a completely different location, reading an unrelated
// sample value and producing a sharp click.
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
	// With crossfaded read pointers, the transition stays smooth.
	// Without crossfade, the pointer jump produces a pop far exceeding 3x.
	if ratio > 3.0 {
		t.Errorf("delay time 110->370ms caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 3.0x)\n"+
			"delay time change needs crossfade to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// TestFilterCutoffNoBiquadTransient verifies that changing the filter cutoff
// mid-stream does not cause a transient spike from biquad state discontinuity.
//
// Without smoothing, rebuildBiquad() creates a fresh biquad with zeroed state
// (x1=x2=y1=y2=0), discarding the old biquad's running state. The new 200Hz LP
// filter's first output is nearly zero (b0*x where b0 is tiny for a 200Hz LP),
// while the previous output was the full-amplitude sine. This produces a massive
// transient spike.
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
	// With coefficient interpolation or crossfade, the transition is gradual.
	// Without it, the zeroed biquad state creates a spike exceeding 10x.
	if ratio > 5.0 {
		t.Errorf("filter cutoff 10kHz->200Hz caused transient: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 5.0x)\n"+
			"biquad coefficient change needs interpolation to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// TestEffectToggleNoCrossfadePop verifies that disabling an effect mid-stream
// does not cause a pop from the abrupt processor chain swap.
//
// Without crossfading, ToggleInsertEffect atomically replaces the channel's
// processor chain via replaceProcessors. The output jumps from the heavily
// distorted signal (~0.995 peak) to the raw dry signal (~0.3 peak) in one
// sample — a level shift of ~0.7.
func TestEffectToggleNoCrossfadePop(t *testing.T) {
	resetChains(t)
	const freq = 440.0
	const amp = 0.3

	id := "smooth-toggle"
	ch := InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, map[string]float64{"drive": 10, "tone": 4000, "mix": 1})

	changeAt := peakIdx(freq, 2000)
	total := changeAt + 500

	output := make([]float64, total)
	for i := 0; i < changeAt; i++ {
		output[i] = ch.ProcessSampleLocal(sineAt(freq, amp, i))
	}

	// Disable the effect — processor chain swapped atomically.
	ToggleInsertEffect(id, 0, false)

	for i := changeAt; i < total; i++ {
		output[i] = ch.ProcessSampleLocal(sineAt(freq, amp, i))
	}

	steadyDelta := maxAbsDelta(output[200 : changeAt-50])
	changeDelta := math.Abs(output[changeAt] - output[changeAt-1])

	if steadyDelta < 1e-10 {
		t.Fatal("steady-state signal is silent")
	}

	ratio := changeDelta / steadyDelta
	// With a crossfade envelope, the transition is smooth.
	// Without it, the output level jumps from distorted to dry.
	// Note: the distorted signal itself has high deltas at zero crossings
	// (tanh saturation creates steep transitions), so the ratio is modest
	// (~4x) but the absolute pop is large (~0.7).
	if ratio > 3.0 {
		t.Errorf("effect toggle off caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 3.0x)\n"+
			"toggling needs crossfade to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// TestReverbMixNoClickOnChange verifies that changing the reverb mix from 0
// to 0.5 mid-stream does not cause a click.
//
// At mix=0 the dry signal passes through at full amplitude. When mix jumps to
// 0.5, the dry component instantly halves (from 0.3 to 0.15 at peak) while the
// reverb tail (which has been accumulating internally) is suddenly added. The
// abrupt dry-level drop alone causes a discontinuity of ~0.15, far exceeding
// the natural sine delta of ~0.019.
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

	// At mix=0 combs accumulate but don't contribute. Changing mix suddenly
	// halves the dry component and adds accumulated wet energy.
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
	// With smoothed mix, the dry/wet blend transitions gradually.
	// Without it, the amplitude jump exceeds 4x.
	if ratio > 4.0 {
		t.Errorf("reverb mix 0->0.5 caused click: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"mix parameter needs smoothing to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}

// TestChorusDepthNoPopOnChange verifies that changing chorus depth mid-stream
// does not cause a pop.
//
// When depth increases from 0 to 15ms, the buffer is reallocated (zeroed) and
// the modulated read position jumps from 1 sample to up to ~662 samples back.
// The wet signal drops to zero (from the empty buffer) while the dry component
// continues, causing a level shift that manifests as a pop.
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
	// With smoothed depth change, the modulation range transitions gradually.
	// Without it, the buffer reset and read-position jump cause a pop.
	if ratio > 4.0 {
		t.Errorf("chorus depth 0->15ms caused pop: "+
			"change delta=%.6f is %.1fx steady delta=%.6f (max 4.0x)\n"+
			"depth change needs smoothing to avoid click",
			changeDelta, ratio, steadyDelta)
	}
}
