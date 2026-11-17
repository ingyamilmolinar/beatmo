package fingerprint

import (
	"fmt"
	"math"
)

const (
	IDDecayPerMode     = "decay.per_mode"
	IDDecaySlopeVsFreq = "decay.slope_vs_freq"
	IDBandT60          = "band.t60"
)

const GroupDecayResolved = "resolved decay"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDDecayPerMode, Group: GroupDecayResolved, Name: "decay per mode",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "dB/s",
		Drift: 6, Mismatch: 15,
		Doc: "Decay rate of each strong mode, fitted from its own level track after its peak. " +
			"Compared as mean absolute difference over the modes both sides share by rank. " +
			"A harp's 'let it ring' and a nylon-vs-steel 'metallic' verdict both live here: " +
			"the question is not how fast the note dies but how fast EACH partial dies.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rates, labels, ok := PerModeDecay(b.Modes(), 8)
			if !ok {
				return Unavailable("no strong mode had three frames of decay to fit")
			}
			return Vector(rates, labels)
		},
		Compare: func(ref, synth Measurement, drift, mismatch float64) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "per-mode decay unavailable on one side"}
			}
			n := len(ref.Values)
			if len(synth.Values) < n {
				n = len(synth.Values)
			}
			if n == 0 {
				return Delta{OK: false, NA: "no shared modes"}
			}
			sum := 0.0
			for i := 0; i < n; i++ {
				sum += math.Abs(ref.Values[i] - synth.Values[i])
			}
			d := sum / float64(n)
			// Declared thresholds are already in this comparator's units
			// (dB/s), so they pass straight through.
			// The display already carries the value in dB/s; passing it
			// through verbatim avoids appending a second rendering of the
			// same number.
			return reducedDeltaDisplay(d, fmt.Sprintf("mean|Δ| %.1f dB/s", d), drift, mismatch)
		},
	})

	r.Register(Descriptor{
		ID: IDDecaySlopeVsFreq, Group: GroupDecayResolved, Name: "decay slope vs frequency",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "dB/s per kHz",
		Drift: 3, Mismatch: 8,
		Doc: "How much faster the high modes die than the low ones. Nylon and wood: strongly " +
			"negative (bright attack, warm sustain). Steel and metal: near zero (the highs " +
			"ring as long as the fundamental). The centroid can match while this differs — " +
			"which is exactly the 'metallic acoustic, not nylon' verdict recorded in the guide.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := DecaySlopeVsFreq(b.Modes())
			if !ok {
				return Unavailable("fewer than three strong modes with a fitted decay")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDBandT60, Group: GroupDecayResolved, Name: "band T60",
		Scope: ScopeBand, Repr: ReprSpectrum, Kind: KindSeconds,
		Drift: 1.4, Mismatch: 2.0,
		Doc: "T60 per octave band, derived from the fitted decay RATE of the visible tail " +
			"rather than the time a threshold was crossed, so a 3 s clip of a 10 s ring " +
			"still reports ~10 s instead of a clamped 3 s on both sides. A band is only " +
			"fitted when its own peak is within 30 dB of the loudest band's peak in the " +
			"SAME signal -- otherwise an OLS slope over near-silence would measure FFT " +
			"leakage/noise floor, not a real decay (unfittable bands report the sentinel " +
			"-1, never a real T60, and never NaN). Compared as the MEAN |log2(ref/synth)| " +
			"over bands measurable on both sides -- the worst individual band is still " +
			"surfaced by name in the comparison's display text, since a single badly-wrong " +
			"band is exactly the kind of localized defect ring T60 could never show. " +
			"This replaces ring T60 in every ranking.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			bd := BandT60(b.STFT(trackFFTSize, trackHop))
			vals := make([]float64, len(bd.T60Sec))
			any := false
			for i := range vals {
				if bd.OK[i] {
					vals[i] = bd.T60Sec[i]
					any = true
				} else {
					vals[i] = bandT60Unfit
				}
			}
			if !any {
				return Unavailable("no octave band had a fittable decay")
			}
			return Vector(vals, octaveBandLabels)
		},
		Compare: func(ref, synth Measurement, drift, mismatch float64) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "band T60 unavailable on one side"}
			}
			sum, signed := 0.0, 0.0
			n := 0
			worstAbs, worstLabel := 0.0, ""
			for i := range ref.Values {
				if i >= len(synth.Values) || ref.Values[i] < 0 || synth.Values[i] < 0 {
					continue
				}
				l := math.Log2(synth.Values[i] / ref.Values[i])
				sum += math.Abs(l)
				signed += l
				n++
				if math.Abs(l) > worstAbs {
					worstAbs = math.Abs(l)
					if i < len(octaveBandLabels) {
						worstLabel = octaveBandLabels[i]
					}
				}
			}
			if n == 0 {
				return Delta{OK: false, NA: "no octave band measurable on both sides"}
			}
			mean := sum / float64(n)
			label := fmt.Sprintf("mean %.2fx (worst %s %.2fx)", math.Pow(2, mean), worstLabel, math.Pow(2, worstAbs))
			// This descriptor DECLARES its thresholds as T60 RATIOS (1.4x
			// drift, 2.0x mismatch -- readable, and the unit the scale
			// harness perturbs in) but COMPARES mean|log2 ratio|, so the
			// pair is converted into the working domain here. The literals
			// 0.5 / 1.0 that used to sit in this call were exactly
			// log2(1.4) / log2(2.0); spelling the conversion out is what
			// lets calibration reach this row at all.
			d := reducedDeltaDisplay(mean, label, math.Log2(drift), math.Log2(mismatch))
			// Report the magnitude back in the DECLARED units so calibration
			// and this comparator agree on what a threshold means.
			d.Mag, d.MagOK = math.Pow(2, mean), true
			if signed/float64(n) > 0 {
				d.Direction = "high"
			} else {
				d.Direction = "low"
			}
			if d.Verdict == VerdictOK {
				d.Direction = ""
			}
			return d
		},
	})
}
