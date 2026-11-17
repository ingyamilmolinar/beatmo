package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// onset.go — what happens in the first tens of milliseconds, and how the
// noise part of a note behaves against its tonal part.
//
// The guide records that every noise source in the modular pipeline injects
// PRE-filter, so a bowed string cannot produce the broadband bite the
// references show (0.20 at onset dropping to ~0.01). onset.bite_ratio is the
// axis that shows whether a bite survived the voice filter.

const (
	onsetFrac        = 0.1 // onset = first frame/sample at this fraction of the max
	onsetBiteWinSec  = 0.030
	onsetRefStartSec = 0.300
	onsetRefEndSec   = 0.600
	onsetFlatWinSec  = 0.020
)

// onsetFrame returns the index of the first Tracks frame whose RMS reaches
// onsetFrac of the track's peak RMS.
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

// OnsetBiteRatio compares the broadband energy (frame RMS^2 * spectral
// flatness) in the first 30 ms after onset to the same quantity 300-600 ms
// in. A noise burst at the attack (a bow bite, a stick crack) drives this
// well above 1; a clean onset with no burst stays near 1.
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
			// Do NOT clamp: a clip too short to cover the full window would
			// silently average over fewer frames than the window claims,
			// fabricating a number the Doc's "300-600 ms" description does
			// not actually describe. Report unmeasurable instead.
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

// onsetSample returns the index of the first time-domain sample whose
// absolute value reaches onsetFrac of the wave's peak.
func onsetSample(w wave.Wave) int {
	pk := w.PeakSample()
	for i, v := range w.Samples {
		if math.Abs(v) >= onsetFrac*pk {
			return i
		}
	}
	return 0
}

// OnsetFlatness is the spectral flatness of a 2048-point spectrum built from
// 20 ms of signal starting at the onset sample, zero-padded to 2048.
// Noise-led attacks (a clap, a snare crack) read high; a clean pluck or a
// soft bow start reads low.
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

// OnsetPitchGlideCents is the pitch detected 20-60 ms after onset minus the
// pitch detected 200-320 ms after onset, in cents. Positive means the note
// started sharp of where it settles (a plucked string glide-down, a brass
// lip slurring in); negative means it started flat. ok is false unless both
// windows produce a detection within an octave of the reference f0 -- the
// same octave-guard RequiresPitched relies on, so a spurious detection in
// either window cannot fabricate a glide.
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

// residualCentroidNotched is the shared core for ResidualCentroidHz and
// ResidualCentroidAtModes: zero out a +/-1 bin window around every frequency
// in freqs, then take the centroid of whatever remains.
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

// ResidualCentroidHz is the spectral centroid of mag with a +/-1 bin window
// around every k*f0 harmonic zeroed out: where the NON-harmonic energy
// lives. When f0 or binHz is not usable, it falls back to the plain
// centroid (nothing to notch out).
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

// ResidualCentroidAtModes is ResidualCentroidHz's f0-free sibling: it notches
// +/-1 bin around each of the given mode frequencies directly, rather than
// around k*f0 multiples of a fundamental. Use this whenever the signal has
// no reliable fundamental (RequiresPitched is false) — placing notches at
// multiples of an unreliable f0 is exactly the hi-hat bug RequiresPitched
// exists to prevent (a spurious 56.5 Hz "fundamental" on a cymbal), and this
// axis needs the same protection every harmonic-series descriptor already
// has. modeHz is expected to come from a ModeSet's tracked, amplitude-real
// modes (b.Modes().Strong(modeStrongWithin)), not a guessed harmonic ladder.
func ResidualCentroidAtModes(mag []float64, binHz float64, modeHz []float64) float64 {
	return residualCentroidNotched(mag, binHz, modeHz)
}

// noReasonNoiseDecay is the reason reported when NoiseDecayVsTone fails for
// any cause OTHER than both tracks running out the clip before -20 dB —
// that specific cause gets its own, more precise message.
const noReasonNoiseDecay = "no fundamental, or the tonal track never decayed 20 dB"

// NoiseDecayVsTone is the T20 (20 dB decay time) of the residual (noise)
// energy track divided by the T20 of the harmonic (tonal) energy track.
// Above 1, the noise outlives the tone; below 1, the noise belongs to the
// onset and the tone rings on past it.
//
// reason is only meaningful when ok is false; it names WHY, so the registry
// can report an honest "not measurable" rather than a generic one.
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
		// Both tracks were still above -20 dB when the clip ran out: neither
		// T20 was actually observed, so their ratio would compare two lower
		// bounds against each other and report a number that looks measured
		// but isn't.
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
