package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Every noise source injects pre-filter, so a bowed string cannot produce the broadband onset bite the references show;
// bite_ratio shows whether one survived the voice filter.

const (
	onsetFrac        = 0.1 // onset = first frame/sample at this fraction of the max
	onsetBiteWinSec  = 0.030
	onsetRefStartSec = 0.300
	onsetRefEndSec   = 0.600
	onsetFlatWinSec  = 0.020
)

func onsetFrame(tr *Tracks) int {
	mx := 0.0
	for _, v := range tr.RMS {
		mx = math.Max(mx, v)
	}
	for i, v := range tr.RMS {
		if v >= onsetFrac*mx {
			return i
		}
	}
	return 0
}

// Broadband energy (RMS² × flatness) in the first 30 ms over 300–600 ms in: a bow bite or stick crack drives it well
// above 1.
func OnsetBiteRatio(tr *Tracks) (float64, bool) {
	if tr.NumFrames < 4 || tr.HopSec <= 0 {
		return 0, false
	}
	o := onsetFrame(tr)
	win := func(fromSec, toSec float64) (float64, bool) {
		a := o + int(fromSec/tr.HopSec)
		b := o + int(toSec/tr.HopSec)
		if b <= a {
			b = a + 1
		}
		if a >= tr.NumFrames || b > tr.NumFrames {
			// Not clamped: averaging fewer frames than the window claims would fabricate a number; unmeasurable instead.
			return 0, false
		}
		sum := 0.0
		for i := a; i < b; i++ {
			sum += tr.RMS[i] * tr.RMS[i] * tr.Flatness[i]
		}
		return sum / float64(b-a), true
	}
	bite, ok1 := win(0, onsetBiteWinSec)
	ref, ok2 := win(onsetRefStartSec, onsetRefEndSec)
	if !ok1 || !ok2 || !(ref > 1e-12) {
		return 0, false
	}
	v := bite / ref
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func onsetSample(w wave.Wave) int {
	pk := w.PeakSample()
	for i, v := range w.Samples {
		if math.Abs(v) >= onsetFrac*pk {
			return i
		}
	}
	return 0
}

// 20 ms from the onset, zero-padded to 2048: noise-led attacks read high, a clean pluck low.
func OnsetFlatness(w wave.Wave) (float64, bool) {
	if w.Len() < 2048 {
		return 0, false
	}
	o := onsetSample(w)
	n := int(onsetFlatWinSec * float64(w.SampleRate))
	if o+n > w.Len() {
		return 0, false
	}
	seg := make([]float64, 2048)
	copy(seg, w.Samples[o:o+n])
	mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: w.SampleRate}, 2048, wave.WindowHann)
	return spectralFlatnessOf(mag), true
}

// 20–60 ms minus 200–320 ms after onset, in cents: positive started sharp. ok only when both windows detect within an
// octave of f0, the same guard RequiresPitched uses, so a spurious detection cannot fabricate a glide.
func OnsetPitchGlideCents(w wave.Wave, f0 float64) (float64, bool) {
	if f0 <= 0 {
		return 0, false
	}
	o := onsetSample(w)
	slice := func(fromSec, toSec float64) (float64, bool) {
		a := o + int(fromSec*float64(w.SampleRate))
		b := o + int(toSec*float64(w.SampleRate))
		if b > w.Len() {
			return 0, false
		}
		f := DetectF0(wave.Wave{Samples: w.Samples[a:b], SampleRate: w.SampleRate})
		if f <= 0 || f < f0/1.9 || f > f0*1.9 {
			return 0, false
		}
		return f, true
	}
	early, ok1 := slice(0.020, 0.060)
	late, ok2 := slice(0.200, 0.320)
	if !ok1 || !ok2 || !(late > 0) {
		return 0, false
	}
	v := 1200 * math.Log2(early/late)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// ±1 bin zeroed around every frequency in freqs.
func residualCentroidNotched(mag []float64, binHz float64, freqs []float64) float64 {
	if binHz <= 0 || len(freqs) == 0 {
		return spectralCentroidHz(mag, binHz)
	}
	res := make([]float64, len(mag))
	copy(res, mag)
	for _, f := range freqs {
		if f <= 0 {
			continue
		}
		c := int(f/binHz + 0.5)
		if c >= len(res) {
			continue
		}
		for d := -1; d <= 1; d++ {
			if j := c + d; j >= 0 && j < len(res) {
				res[j] = 0
			}
		}
	}
	return spectralCentroidHz(res, binHz)
}

// Where the NON-harmonic energy lives; the plain centroid when f0 or binHz is unusable.
func ResidualCentroidHz(mag []float64, binHz, f0 float64) float64 {
	if f0 <= 0 || binHz <= 0 {
		return spectralCentroidHz(mag, binHz)
	}
	var freqs []float64
	for k := 1; ; k++ {
		f := f0 * float64(k)
		if int(f/binHz+0.5) >= len(mag) {
			break
		}
		freqs = append(freqs, f)
	}
	return residualCentroidNotched(mag, binHz, freqs)
}

// Notches at the tracked modes instead of k·f0, for signals with no reliable fundamental: multiples of an unreliable
// f0 are the hi-hat bug RequiresPitched exists to prevent.
func ResidualCentroidAtModes(mag []float64, binHz float64, modeHz []float64) float64 {
	return residualCentroidNotched(mag, binHz, modeHz)
}

// Both tracks running out the clip first gets its own, more precise message.
const noReasonNoiseDecay = "no fundamental, or the tonal track never decayed 20 dB"

// Residual T20 over harmonic T20: above 1 the noise outlives the tone. reason names why when ok is false.
func NoiseDecayVsTone(tr *Tracks) (ratio float64, ok bool, reason string) {
	if tr.NumFrames < 4 || tr.F0Hz <= 0 || tr.HopSec <= 0 {
		return 0, false, noReasonNoiseDecay
	}
	resid := make([]float64, tr.NumFrames)
	tone := make([]float64, tr.NumFrames)
	pr, pt := 0, 0
	for i := range resid {
		e := tr.RMS[i] * tr.RMS[i]
		resid[i] = e * tr.NoiseFraction[i]
		tone[i] = e * (1 - tr.NoiseFraction[i])
		if resid[i] > resid[pr] {
			pr = i
		}
		if tone[i] > tone[pt] {
			pt = i
		}
	}
	// decayThreshold works on amplitude-like envelopes; pass sqrt(energy).
	sq := func(v []float64) []float64 {
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = math.Sqrt(x)
		}
		return out
	}
	rs, tsv := sq(resid), sq(tone)
	tr20, rExtrap := decayThreshold(rs, pr, rs[pr], 20, tr.HopSec)
	tt20, tExtrap := decayThreshold(tsv, pt, tsv[pt], 20, tr.HopSec)
	if rExtrap && tExtrap {
		// Neither T20 was observed, so their ratio would compare two lower bounds and look measured.
		return 0, false, "both tracks still above -20 dB at clip end"
	}
	if !(tt20 > 1e-12) {
		return 0, false, noReasonNoiseDecay
	}
	v := tr20 / tt20
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false, noReasonNoiseDecay
	}
	return v, true, ""
}
