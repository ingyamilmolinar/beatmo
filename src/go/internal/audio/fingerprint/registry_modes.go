package fingerprint

import (
	"fmt"
	"math"
)

// Built on the f0-free mode tracker, gated only on RequiresModes, so percussive and unpitched instruments get them too.

const (
	IDModeAnchorHz       = "mode.anchor_hz"
	IDModeRatios         = "mode.ratios"
	IDModeCountPerOctave = "mode.count_per_octave"
	IDModeHarmonicity    = "mode.harmonicity"
	IDModeBeatingRate    = "mode.beating_rate_hz"
	IDModeBeatingDepth   = "mode.beating_depth"
)

const GroupModes = "modal structure"

const (
	modeRatioCount        = 12
	modeHarmonicTolFrac   = 0.03
	modeBeatingMaxGapHz   = 25.0 // ceiling on a reported ripple rate
	modeBeatingMinDepth   = 0.05 // floor on a reported ripple depth
	modeBeatingShareTolHz = 0.3  // two modes' ripple rates are "the same" within this
	modeCountLoHz         = 100.0
	modeCountHiHz         = 8000.0
)

// The larger of the two directions, so spurious extra modes cost as much as missing ones.
func ModeRatioDistanceCents(ref, synth []float64) float64 {
	one := func(a, b []float64) float64 {
		if len(a) == 0 || len(b) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, x := range a {
			best := math.Inf(1)
			for _, y := range b {
				if c := math.Abs(1200 * math.Log2(y/x)); c < best {
					best = c
				}
			}
			sum += best
		}
		return sum / float64(len(a))
	}
	return math.Max(one(ref, synth), one(synth, ref))
}

// The same math on values already in CENTS, the descriptor's declared unit: the threshold-scale probe shifts
// components in that unit, and an 8-cent shift added to a raw ratio of 1.0 would be a ~9× pitch jump.
func centsNearestGap(ref, synth []float64) float64 {
	one := func(a, b []float64) float64 {
		if len(a) == 0 || len(b) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, x := range a {
			best := math.Inf(1)
			for _, y := range b {
				if c := math.Abs(y - x); c < best {
					best = c
				}
			}
			sum += best
		}
		return sum / float64(len(a))
	}
	return math.Max(one(ref, synth), one(synth, ref))
}

func ModeHarmonicity(ms *ModeSet) (float64, bool) {
	a := ms.AnchorHz()
	strong := ms.Strong(modeStrongWithin)
	if a <= 0 || len(strong) < 2 {
		return 0, false
	}
	var total, harmonic float64
	for _, m := range strong {
		e := math.Pow(10, m.PeakDB/10)
		total += e
		r := m.Hz / a
		if math.Abs(r-math.Round(r)) <= modeHarmonicTolFrac*math.Round(r) {
			harmonic += e
		}
	}
	if total <= 0 {
		return 0, false
	}
	return harmonic / total, true
}

func ModeCountPerOctave(ms *ModeSet) (float64, bool) {
	strong := ms.Strong(modeStrongWithin)
	if len(strong) < 2 {
		return 0, false
	}
	n := 0
	for _, m := range strong {
		if m.Hz >= modeCountLoHz && m.Hz <= modeCountHiHz {
			n++
		}
	}
	octaves := math.Log2(modeCountHiHz / modeCountLoHz)
	return float64(n) / octaves, true
}

// Per-mode ripple with common-mode rejection: a rate shared by EVERY rippling strong mode is a tremolo on the whole
// output, not beating, which comes from two partials and shows on only some modes or on one merged mode. One strong
// mode qualifies outright (the merged-pair case); the deepest qualifying ripple wins.
func ModeBeating(ms *ModeSet) (rateHz, depth float64, ok bool) {
	strong := ms.Strong(modeStrongWithin)
	if len(strong) == 0 {
		return 0, 0, false
	}

	rates := make([]float64, len(strong))
	depths := make([]float64, len(strong))
	valid := make([]bool, len(strong))
	for i, m := range strong {
		rates[i], depths[i], valid[i] = modeSelfBeat(m, ms.HopSec)
	}

	best := -1
	for i := range strong {
		if !valid[i] {
			continue
		}
		if len(strong) > 1 {
			sharedByAll := true
			for j := range strong {
				if j == i {
					continue
				}
				if !valid[j] || math.Abs(rates[j]-rates[i]) > modeBeatingShareTolHz {
					sharedByAll = false
					break
				}
			}
			if sharedByAll {
				continue // global tremolo: every other mode ripples at the same rate
			}
		}
		if best < 0 || depths[i] > depths[best] {
			best = i
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return rates[best], depths[best], true
}

// Detrended first: crossings of a decaying track's raw mean undercount cycles and gave each mode a different false rate,
// letting a real tremolo dodge common-mode rejection. Duration comes from the span of present frames, so gaps do not
// inflate the rate; only rates up to modeBeatingMaxGapHz, above which hop sampling aliases.
func modeSelfBeat(m Mode, hopSec float64) (rateHz, depth float64, ok bool) {
	var idx, dbs []float64
	for i, db := range m.Track {
		if math.IsNaN(db) {
			continue
		}
		idx = append(idx, float64(i))
		dbs = append(dbs, db)
	}
	if len(dbs) < 6 || hopSec <= 0 {
		return 0, 0, false
	}

	meanX, meanY := 0.0, 0.0
	for i := range idx {
		meanX += idx[i]
		meanY += dbs[i]
	}
	n := float64(len(idx))
	meanX /= n
	meanY /= n
	slope := olsSlope(idx, dbs)
	intercept := meanY - slope*meanX

	lin := make([]float64, len(idx))
	for i := range idx {
		trend := intercept + slope*idx[i]
		lin[i] = math.Pow(10, (dbs[i]-trend)/20)
	}

	mean := 0.0
	for _, v := range lin {
		mean += v
	}
	mean /= float64(len(lin))
	if mean <= 0 {
		return 0, 0, false
	}

	lo, hi := lin[0], lin[0]
	crossings := 0
	above := lin[0] >= mean
	for _, v := range lin {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
		if nowAbove := v >= mean; nowAbove != above {
			crossings++
			above = nowAbove
		}
	}
	if hi <= 0 || crossings < 2 {
		return 0, 0, false
	}
	depth = (hi - lo) / (hi + lo)
	if depth < modeBeatingMinDepth {
		return 0, 0, false
	}
	span := idx[len(idx)-1] - idx[0]
	if span <= 0 {
		return 0, 0, false
	}
	durationSec := span * hopSec
	rateHz = (float64(crossings) / 2) / durationSec
	if rateHz <= 0 || rateHz > modeBeatingMaxGapHz {
		return 0, 0, false
	}
	return rateHz, depth, true
}

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDModeAnchorHz, Group: GroupModes, Name: "mode anchor",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Drift: 5, Mismatch: 15,
		Doc: "Frequency of the LOWEST spectral mode within 20 dB of the loudest — the pitch grid " +
			"every mode ratio is measured against. Independent of DetectF0, so it applies to " +
			"bells, bars, drums and cymbals. A large delta here means the two sides are on " +
			"different grids (the tubular-bells synth was measured at 4x the reference's " +
			"anchor before this row existed) and every ratio comparison below is suspect.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			if a := b.Modes().AnchorHz(); a > 0 {
				return Scalar(a)
			}
			return Unavailable(naNoModes)
		},
	})

	r.Register(Descriptor{
		ID: IDModeRatios, Group: GroupModes, Name: "mode ratios",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "cents",
		Drift: 30, Mismatch: 80,
		Demoted: true,
		Doc: "DEMOTED 2026-09-04: the symmetric nearest-mode gap this row compares measures " +
			"mode ABSENCE, not mistuning — when the synth simply lacks a mode the reference " +
			"has, the nearest surviving neighbour supplies the gap, so a missing partial and " +
			"a badly mistuned one are indistinguishable and the missing one scores larger. " +
			"The good group's own p50 is 520 cents (calibration would have set the drift " +
			"threshold there), which is nearly half an octave: no listener called those " +
			"instruments mistuned by half an octave, so the quantity is not measuring what " +
			"its name says. It stays measured and PRINTED because the ratio list beside it " +
			"is the most legible view of partial structure the report has; it is excluded " +
			"from every ranking. " +
			"The first twelve strong modes divided by the anchor, stored in CENTS above it " +
			"and compared as the symmetric mean nearest-neighbour gap. This is 'where do the " +
			"overtones sit' without assuming a harmonic series — a plucked string reads " +
			"0,1200,1902…, a marimba bar 0,2400,3900…, a tube bell 0,1200,1471,1911,2537… " +
			"The kalimba complaint ('sounds like a plucked string, needs the wooden marimba " +
			"feel') is this row. Stored as cents rather than raw ratios (1, 2.76, 5.40, …) so " +
			"the Vector's unit matches its declared Unit: a raw-ratio payload would let a " +
			"generic cents-scale perturbation of one component (as in " +
			"TestDescriptorThresholdsAreOnTheRightScale) land as a multi-octave pitch jump " +
			"instead of the intended few-cent nudge.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rs := b.Modes().Ratios(modeRatioCount)
			if len(rs) < 2 {
				return Unavailable(naNoModes)
			}
			cents := make([]float64, len(rs))
			labels := make([]string, len(rs))
			for i, r := range rs {
				cents[i] = 1200 * math.Log2(r)
				labels[i] = fmt.Sprintf("m%d", i+1)
			}
			return Vector(cents, labels)
		},
		Compare: func(ref, synth Measurement, drift, mismatch float64) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "mode ratios unavailable on one side"}
			}
			d := centsNearestGap(ref.Values, synth.Values)
			// Declared thresholds are already in cents; the display already carries the value.
			return reducedDeltaDisplay(d, fmt.Sprintf("mean gap %.0f cents", d), drift, mismatch)
		},
	})

	r.Register(Descriptor{
		ID: IDModeCountPerOctave, Group: GroupModes, Name: "mode density",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRatio,
		Drift: 1.4, Mismatch: 2.0,
		Doc: "Strong modes per octave over 100 Hz–8 kHz. A bell or cymbal is dense, a string or " +
			"pipe sparse. 'Metallic complexity' in the listener's words.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := ModeCountPerOctave(b.Modes())
			if !ok {
				return Unavailable(naNoModes)
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDModeHarmonicity, Group: GroupModes, Name: "mode harmonicity",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Share of strong-mode ENERGY sitting within 3% of an integer multiple of the " +
			"anchor. ~1.0 for strings and pipes, low for bars, bells and membranes. This is the " +
			"wooden-vs-string axis the kalimba needed: its inharmonicity already matched the " +
			"reference (0.010 vs 0.008) while its mode SEQUENCE did not.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := ModeHarmonicity(b.Modes())
			if !ok {
				return Unavailable(naNoModes)
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDModeBeatingRate, Group: GroupModes, Name: "beating rate",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "Hz",
		Drift: 1, Mismatch: 4,
		Doc: "Ripple rate of the deepest STRONG MODE whose own amplitude wobbles at a rate the " +
			"OTHER strong modes do not all share — two real partials beating against each " +
			"other (either resolved as separate modes, or, when they are closer together than " +
			"the analysis FFT can resolve, merged into ONE mode whose single dB track still " +
			"carries the beat as periodic amplitude modulation) mark only some modes this way, " +
			"never all of them at once. A GLOBAL tremolo — one LFO on the whole instrument's " +
			"output — modulates every partial identically, so every strong mode ripples at the " +
			"SAME rate; that shared-rate signature is deliberately rejected here as tremolo, " +
			"not beating (a single strong mode has no 'other' to share with, so it qualifies " +
			"unconditionally on its own ripple). N/A when no mode has a qualifying, " +
			"non-common-mode ripple — the tubular-bells reference has one; a plain additive " +
			"tone or a tremolo'd pad does not.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rate, _, ok := ModeBeating(b.Modes())
			if !ok {
				return Unavailable("no per-mode ripple that is not common to every mode")
			}
			return Scalar(rate)
		},
	})

	r.Register(Descriptor{
		ID: IDModeBeatingDepth, Group: GroupModes, Name: "beating depth",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Modulation depth of that same mode's own amplitude ripple — see mode.beating_rate_hz " +
			"for the mechanism and why a global tremolo (every strong mode rippling at the same " +
			"rate) is rejected rather than reported here. The tubular-bells verdict — 'a " +
			"resonant wavy metallic ring that prolongs and waves in time' — is a deep, slow, " +
			"NON-common-mode beat; a clean additive tone, or an instrument whose only amplitude " +
			"motion is a whole-signal tremolo, has none.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			_, depth, ok := ModeBeating(b.Modes())
			if !ok {
				return Unavailable("no per-mode ripple that is not common to every mode")
			}
			return Scalar(depth)
		},
	})
}
