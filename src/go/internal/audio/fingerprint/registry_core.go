package fingerprint

import "fmt"

// registry_core.go — the nine axes cmd/refs/diagnose.go used to hand-write.
//
// Group and Name strings are reproduced BYTE-FOR-BYTE from the pre-registry
// diagnose.go. They are load-bearing: the frozen acceptance baseline in
// cmd/refs/testdata/diagnose_baseline_2026-08-22.json keys on them, as does
// the verdict table transcribed from
// docs/superpowers/specs/2026-08-22-verdict-to-metric-mapping.md. Renaming an
// axis here silently breaks the gate that proves the diagnostic still
// reproduces the listener's own complaints.
//
// Descriptor IDs are new (the hand-written axes had no stable machine key)
// and are what goldens and the calibration tables will key on from now on.

// Core descriptor IDs. Exported so the validation harnesses and cmd/refs can
// reference specific axes without stringly-typed literals scattered around.
const (
	IDNoiseFraction     = "noise.fraction"
	IDNoiseFlatness     = "noise.flatness"
	IDEnvAttack         = "env.attack"
	IDEnvRingT60        = "env.ring_t60"
	IDEnvRelease        = "env.release"
	IDInharmonicity     = "partial.inharmonicity"
	IDSeriesDeviation   = "partial.series_deviation"
	IDDecayT10          = "env.decay_t10"
	IDDecayT20          = "env.decay_t20"
	IDDecayT30          = "env.decay_t30"
	IDEnergySections    = "env.energy_sections"
	IDEnvDecay          = "env.decay"
	IDEnvDecayRate      = "env.decay_rate"
	IDEnvPeakTime       = "env.peak_time"
	IDEnvSwell          = "env.swell"
	IDEnvTotalAudible   = "env.total_audible"
	IDCentroid          = "bright.centroid"
	IDRolloff           = "bright.rolloff"
	IDSustainModulation = "mod.sustain_variation"
)

// Group names, byte-identical to the pre-registry diagnose.go.
const (
	GroupNoise    = "noise / harmonic balance"
	GroupEnvelope = "temporal envelope"
	GroupPartials = "partial structure"
	GroupBright   = "brightness / formant"
	GroupMod      = "modulation"
)

// naSilent is the single reason string used whenever an envelope-derived
// descriptor cannot be measured because the clip carries no audible signal.
const naSilent = "signal is silent"

func init() {
	r := DefaultRegistry()

	// --- noise / harmonic balance ------------------------------------------
	//
	// Noise QUANTITY and noise CHARACTER are deliberately two descriptors,
	// not one. The mapping doc's snare correction is the reason: the synth
	// measured MORE noise energy (fraction 0.815 -> 0.933) but FAR more
	// tonal noise (flatness 0.599 -> 0.204). Broadband noise reads as snare
	// wires; band-limited coloured noise reads as hiss. When these two
	// disagree, the disagreement IS the diagnosis — averaging them away
	// would have destroyed the finding.

	r.Register(Descriptor{
		ID: IDNoiseFraction, Group: GroupNoise, Name: "noise fraction",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Doc: "Fingerprint.NoiseRatio: 1 - harmonic energy / total energy, using narrow " +
			"+/-1-bin notches at each k*f0. Preferred over NoiseProfile.ResidualRatio, " +
			"whose wider per-harmonic tolerance bands fold real noise into 'harmonic' " +
			"and empirically under-detect breathiness on winds (oboe). REQUIRES a " +
			"reliable f0: the notches are placed at multiples of it, so on an unpitched " +
			"source they land on arbitrary bins and the result is noise, not a " +
			"measurement. The corpus hi-hat is the worked case -- F0 detection returns " +
			"56.5 Hz for a cymbal, and this axis then claimed the synth had MORE noise " +
			"than the reference (0.9995 vs 0.9187) while the listener heard it as too " +
			"TONAL. spectral flatness covers noise character on those signals; it needs " +
			"no f0 at all.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			return Scalar(b.Fingerprint().NoiseRatio)
		},
	})

	r.Register(Descriptor{
		ID: IDNoiseFlatness, Group: GroupNoise, Name: "spectral flatness",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Doc: "Fingerprint.SpectralFlatness: geometric mean / arithmetic mean of the " +
			"sustain-window magnitude spectrum. Measures noise CHARACTER (broadband vs " +
			"coloured) where noise.fraction measures QUANTITY; the two are reported " +
			"side by side because their disagreement is itself diagnostic.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement { return Scalar(b.Fingerprint().SpectralFlatness) },
	})

	// --- temporal envelope --------------------------------------------------

	r.Register(Descriptor{
		ID: IDEnvAttack, Group: GroupEnvelope, Name: "attack",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "EnvelopeSegments.AttackSec: onset (first crossing of 1% of peak) to the " +
			"envelope peak. Uses the general per-signal segmentation rather than " +
			"Fingerprint.AttackTimeSec's fixed 10%->90% rule, because a slow bowed or " +
			"blown onset never presents a clean 10/90 knee.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			s := b.Segments()
			if s.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(s.AttackSec)
		},
	})

	r.Register(Descriptor{
		ID: IDEnvRingT60, Group: GroupEnvelope, Name: "ring T60",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Compound: true,
		Demoted:  true,
		Doc: "EnvelopeSegments.RingT60Sec: peak to -60dB. COMPOUND measurement — it " +
			"also carries RingT60Extrapolated and OverallDecayRateDBPerSec, because " +
			"when BOTH clips still ring above -60dB at their end a raw ratio compares " +
			"two equally-CLAMPED lower bounds and reports a vacuous 'ok' (tubular-bells " +
			"read exactly the clip length on both sides for an instrument the listener " +
			"called not even close). The custom comparator escalates to decay RATE in " +
			"that case. DEMOTED 2026-09-04: every corpus clip is truncated by " +
			"convert.max_sec, so anything that rings clamps at the clip end on BOTH " +
			"sides and compares as fine (AUC 0.19, anti-correlated). Use band.t60, " +
			"which fits the visible tail's slope instead of clamping.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			s := b.Segments()
			if s.Silent {
				return Unavailable(naSilent)
			}
			return Vector(
				[]float64{s.RingT60Sec, boolToFloat(s.RingT60Extrapolated), s.OverallDecayRateDBPerSec},
				[]string{"ring_t60_sec", "extrapolated", "overall_decay_db_per_sec"})
		},
		Compare: compareRingT60,
	})

	r.Register(Descriptor{
		ID: IDEnvRelease, Group: GroupEnvelope, Name: "release",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "EnvelopeSegments.ReleaseSec: sustain-region end to the last audible " +
			"frame. Zero when no release was detected (the clip simply stops " +
			"mid-sustain), which the seconds comparator reports as a mismatch rather " +
			"than as agreement.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			s := b.Segments()
			if s.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(s.ReleaseSec)
		},
	})

	// --- temporal envelope: decay measured in SECTIONS ----------------------
	//
	// ring T60 above is kept for continuity but is close to useless on this
	// corpus and known to be so: every reference is truncated by its own fetch
	// config, so anything that rings hits the clip end long before -60 dB and
	// both sides report a clamped lower bound. Validated against the listener
	// verdicts it scored AUC 0.21 (95% CI 0.00-0.47) -- confidently
	// ANTI-correlated with perceived quality.
	//
	// These four are what the listener actually asked for, in their words:
	// "split it up into multiple sections and also get duration of attack,
	// release, peak". The shallow thresholds are reached inside the clip so
	// they MEASURE, and the section profile says where the energy sits.

	decayRow := func(id, name string, dbDown int, get func(EnvelopeSegments) (float64, bool)) {
		r.Register(Descriptor{
			ID: id, Group: GroupEnvelope, Name: name,
			Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
			Doc: fmt.Sprintf("EnvelopeSegments.DecayT%dSec: seconds from the envelope peak to fall "+
				"%d dB below it, by LAST crossing so a tremolo or beating dip does not end the "+
				"decay early. Preferred over ring T60, which every truncated reference clamps to "+
				"its own clip length. On the instruments the listener rejected for decay this "+
				"saw the whole problem where T60 saw none: harp T20 0.48 ref vs 1.56 synth, "+
				"tubular-bells T10 1.28 ref vs 0.35 synth.", dbDown, dbDown),
			Applies: RequiresAudible,
			Extract: func(b *Bundle) Measurement {
				seg := b.Segments()
				if seg.Silent {
					return Unavailable(naSilent)
				}
				v, extrapolated := get(seg)
				if extrapolated {
					return Unavailable(fmt.Sprintf(
						"clip ended before -%d dB was reached (still above it at %.2fs), so this "+
							"is a lower bound rather than a measured decay time", dbDown, v))
				}
				return Scalar(v)
			},
		})
	}

	decayRow(IDDecayT10, "decay T10", 10, func(s EnvelopeSegments) (float64, bool) {
		return s.DecayT10Sec, s.DecayT10Extrapolated
	})
	decayRow(IDDecayT20, "decay T20", 20, func(s EnvelopeSegments) (float64, bool) {
		return s.DecayT20Sec, s.DecayT20Extrapolated
	})
	decayRow(IDDecayT30, "decay T30", 30, func(s EnvelopeSegments) (float64, bool) {
		return s.DecayT30Sec, s.DecayT30Extrapolated
	})

	r.Register(Descriptor{
		ID: IDEnergySections, Group: GroupEnvelope, Name: "energy sections",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindFraction,
		// Thresholds on the mean absolute difference of four shares that sum to
		// 1. A 0.05 mean shift moves roughly a twentieth of the note's power
		// between sections -- audible as a different envelope shape; 0.15 is a
		// different instrument's shape entirely.
		Drift: 0.05, Mismatch: 0.15,
		Doc: "EnvelopeSegments.EnergySections: the share of the note's total energy in each " +
			"quarter of its audible span. The 'split it into sections' view -- it answers " +
			"WHERE the energy sits, which no single duration can. A real harp puts 96% in the " +
			"first quarter and fades; the synth put 72% there and spread the rest, and the two " +
			"can still share a T20 while sounding nothing alike.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			if len(seg.EnergySections) == 0 {
				return Unavailable("audible span too short to split into sections")
			}
			return Vector(seg.EnergySections, []string{"q1", "q2", "q3", "q4"})
		},
	})

	// --- temporal envelope: the remaining named durations --------------------
	//
	// The listener asked for "duration of attack, release, peak, etc" and the
	// first pass registered attack, release, the shallow decay thresholds and
	// the section profile -- but DecaySec, DecayRateDBPerSec, PeakSec and
	// TotalAudibleSec were computed on every analysis and read by nothing.
	// (Found by TestNoUnreadMetricFields, which exists because this is the
	// fourth time an analyzer in this package has turned out to have no
	// consumer.)

	r.Register(Descriptor{
		ID: IDEnvDecay, Group: GroupEnvelope, Name: "decay",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "EnvelopeSegments.DecaySec: peak to the onset of the sustain plateau, or to the " +
			"last audible frame when there is no plateau. The D of ADSR, which none of the " +
			"other envelope axes measure: attack ends at the peak, release starts at the end " +
			"of sustain, and the decay between them was the missing segment.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(seg.DecaySec)
		},
	})

	r.Register(Descriptor{
		ID: IDEnvDecayRate, Group: GroupEnvelope, Name: "decay rate",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindRate,
		Drift: 3.0, Mismatch: 12.0, Unit: "dB/s",
		Doc: "EnvelopeSegments.DecayRateDBPerSec: OLS slope of the envelope in dB/s over the " +
			"decay region only. Complements the decay DURATION: two notes can take the same " +
			"time to reach the plateau while one falls steeply and levels off and the other " +
			"slides down evenly.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			if seg.DecaySec <= 0 {
				return Unavailable("no decay region (the peak is the last audible frame)")
			}
			return Scalar(seg.DecayRateDBPerSec)
		},
	})

	r.Register(Descriptor{
		ID: IDEnvPeakTime, Group: GroupEnvelope, Name: "peak time",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "EnvelopeSegments.PeakSec: when the note is at its loudest, measured from the " +
			"start. Distinct from attack now that attack ends at the INITIAL rise rather than " +
			"at the global maximum -- for a note that swells, these are different moments, and " +
			"the gap between them is env.swell.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(seg.PeakSec)
		},
	})

	r.Register(Descriptor{
		ID: IDEnvSwell, Group: GroupEnvelope, Name: "swell",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "PeakSec - AttackPeakSec: how long AFTER its onset peak the note goes on getting " +
			"louder. Zero for anything that peaks and decays; large for a note whose loudest " +
			"moment arrives late. This axis only became expressible once attack stopped being " +
			"measured to the global maximum -- before that the swell WAS the attack, which is " +
			"how the shipped cello came to report a 1.62 s attack on a 2 s note. A swell is " +
			"not automatically wrong (a bowed crescendo is one); it is wrong when the " +
			"reference does not have one.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			d := seg.PeakSec - seg.AttackPeakSec
			if d < 0 {
				d = 0
			}
			return Scalar(d)
		},
	})

	r.Register(Descriptor{
		ID: IDEnvTotalAudible, Group: GroupEnvelope, Name: "total audible",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindSeconds,
		Doc: "EnvelopeSegments.TotalAudibleSec: first to last frame above the noise floor -- " +
			"how long the note actually sounds, as opposed to how long the clip is. The " +
			"denominator every section and duration axis is implicitly relative to.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			seg := b.Segments()
			if seg.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(seg.TotalAudibleSec)
		},
	})

	// --- partial structure ---------------------------------------------------
	//
	// Two descriptors asking DIFFERENT questions, both kept deliberately.
	// Inharmonicity asks "how far is each partial from its OWN nearest
	// integer multiple of f0" — a tuned bar's ~4x overtone scores near zero
	// there, since 4 IS an integer. SeriesDeviation asks "does the found
	// partial SEQUENCE look like a consecutive series at all" — the actual
	// string-vs-bar/tube discriminator behind the kalimba complaint.

	r.Register(Descriptor{
		ID: IDInharmonicity, Group: GroupPartials, Name: "inharmonicity",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindTinyFraction,
		Doc: "Fingerprint.Inharmonicity: mean fractional deviation of each measured " +
			"partial from its nearest integer multiple of f0. Near 0 for a clean " +
			"string or wind tone; 0.005-0.03 for a genuinely inharmonic struck bar or " +
			"tube. Blind to non-CONSECUTIVE series — see partial series deviation. " +
			"Requires a reliable f0: every partial is located relative to it.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			return Scalar(b.Fingerprint().Inharmonicity)
		},
	})

	r.Register(Descriptor{
		ID: IDSeriesDeviation, Group: GroupPartials, Name: "partial series deviation",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindSeriesDeviation,
		Doc: "PartialRatios.SeriesDeviation: how far the k-th strongest-by-frequency " +
			"partial sits from the k-th harmonic. A string's partials are 1,2,3,4 " +
			"(near 0); a tuned bar's are 1,4,9 (rank 2 sits at 4x, large deviation); a " +
			"bell/tube's are dense and non-integer. This is the discriminator neither " +
			"inharmonicity measure provides. Requires a reliable f0: the ratios are " +
			"measured against it.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			return Scalar(b.Fingerprint().PartialRatioProfile.SeriesDeviation)
		},
	})

	// --- brightness / formant -------------------------------------------------

	r.Register(Descriptor{
		ID: IDCentroid, Group: GroupBright, Name: "centroid",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Doc: "Fingerprint.SpectralCentroid: magnitude-weighted mean frequency of the " +
			"sustain window — the single best scalar proxy for perceived brightness.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement { return Scalar(b.Fingerprint().SpectralCentroid) },
	})

	r.Register(Descriptor{
		ID: IDRolloff, Group: GroupBright, Name: "rolloff",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Doc: "Fingerprint.SpectralRolloff: the frequency below which 85% of the " +
			"spectral energy lies. Complements centroid by describing where the " +
			"spectrum ENDS rather than where its mass sits.",
		Applies: AlwaysApplies,
		Extract: func(b *Bundle) Measurement { return Scalar(b.Fingerprint().SpectralRolloff) },
	})

	// --- modulation -------------------------------------------------------------

	r.Register(Descriptor{
		ID: IDSustainModulation, Group: GroupMod, Name: "sustain modulation",
		Scope: ScopeWhole, Repr: ReprTime, Kind: KindFraction,
		Doc: "EnvelopeSegments.SustainVariation: std/mean of the sustain-region " +
			"envelope. A dead-flat sustain is the 'sounds like an organ' tell. NOTE " +
			"this measures modulation MAGNITUDE only; magnitude can match while the " +
			"character is opposite (a slow bow stroke vs fast chaos both measure ~0.5) " +
			"— the modulation-spectrum descriptors are what separate those.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			s := b.Segments()
			if s.Silent {
				return Unavailable(naSilent)
			}
			return Scalar(s.SustainVariation)
		},
	})
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// ringT60 compound-measurement component indices.
const (
	ringT60Sec          = 0
	ringT60Extrapolated = 1
	ringT60DecayRate    = 2
)

// compareRingT60 reproduces cmd/refs/diagnose.go's ringT60Axis exactly.
//
// When BOTH sides' T60 are extrapolated lower bounds, comparing them is
// vacuous, so it escalates to the post-peak decay RATE, which IS measurable
// inside the clip. When only one side is extrapolated the ordinary ratio
// still carries signal (harp reads 1.9x short even with one side biased
// toward it), but the note says which value is a bound rather than a
// measurement.
func compareRingT60(ref, synth Measurement, drift, mismatch float64) Delta {
	if !ref.OK {
		return Delta{NA: ref.NA}
	}
	if !synth.OK {
		return Delta{NA: synth.NA}
	}
	// The compound measurement carries three components by construction. A
	// short vector means the extractor and this comparator have drifted apart;
	// report that rather than panicking mid-report and taking every other axis
	// down with it.
	if len(ref.Values) <= ringT60DecayRate || len(synth.Values) <= ringT60DecayRate {
		return Delta{NA: fmt.Sprintf(
			"malformed ring-T60 measurement: need %d components, got ref=%d synth=%d",
			ringT60DecayRate+1, len(ref.Values), len(synth.Values))}
	}
	refExtrap := ref.Values[ringT60Extrapolated] >= 0.5
	synthExtrap := synth.Values[ringT60Extrapolated] >= 0.5

	if refExtrap && synthExtrap {
		v, dir := ClassifyDecayRate(
			ref.Values[ringT60DecayRate], synth.Values[ringT60DecayRate], drift, mismatch)
		// The escalation compares IMPLIED T60s, so the magnitude it weighs is
		// their folded ratio -- the same unit the seconds path below reports,
		// which is what lets one calibrated threshold serve both paths.
		magT60, magOK := decayRateFoldedT60Ratio(
			ref.Values[ringT60DecayRate], synth.Values[ringT60DecayRate])
		return Delta{
			Verdict:   v,
			Direction: dir,
			Signed:    synth.Values[ringT60Sec] - ref.Values[ringT60Sec],
			Norm:      decayRateNorm(ref.Values[ringT60DecayRate], synth.Values[ringT60DecayRate], mismatch),
			Mag:       magT60, MagOK: magOK,
			Display: fmt.Sprintf("rate %.1f -> %.1f dB/s",
				ref.Values[ringT60DecayRate], synth.Values[ringT60DecayRate]),
			OK: true,
			NA: "", // measurable, just via a different quantity
		}
	}
	return compareScalar(KindSeconds, ref.Values[ringT60Sec], synth.Values[ringT60Sec], drift, mismatch)
}

// decayRateNorm mirrors ratioNormAndDisplay's normalization for the decay-rate
// escalation path, so a rate-derived verdict ranks against every other
// descriptor on the same Norm>=1-means-mismatch scale.
// decayRateFoldedT60Ratio returns the folded ratio of the T60s the two decay
// rates imply, and whether it is a measurement at all. It is false when either
// side shows no decay trend, where the classifier asserts a verdict from the
// asymmetry rather than from a ratio -- a sentinel, not a magnitude, and so
// not something threshold calibration may average.
func decayRateFoldedT60Ratio(refRate, synthRate float64) (float64, bool) {
	refAbs, synthAbs := absF(refRate), absF(synthRate)
	if refAbs < decayRateFlatEps || synthAbs < decayRateFlatEps {
		return 0, false
	}
	folded := refAbs / synthAbs
	if folded < 1 {
		folded = 1 / folded
	}
	return folded, true
}

func decayRateNorm(refRate, synthRate, mismatchRatio float64) float64 {
	refAbs, synthAbs := absF(refRate), absF(synthRate)
	if refAbs < decayRateFlatEps || synthAbs < decayRateFlatEps {
		if refAbs < decayRateFlatEps && synthAbs < decayRateFlatEps {
			return 0
		}
		return 1
	}
	folded := refAbs / synthAbs
	if folded < 1 {
		folded = 1 / folded
	}
	return folded / mismatchRatio
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
