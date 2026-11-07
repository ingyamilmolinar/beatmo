package fingerprint

// The family analysers (snare, piano, wind attack, irregularity, kick segments) had full DSP and no consumer;
// registering them is coverage at no DSP cost. Every row is gated on measured signal character, never an
// instrument→family table that goes stale.

// Family descriptor IDs.
const (
	IDSnareBodyFund      = "snare.body_fund_hz"
	IDSnareToneNoise     = "snare.tone_noise_ratio"
	IDSnareLowBody       = "snare.low_body_frac"
	IDSnareHighBuzz      = "snare.high_buzz_frac"
	IDSnareBodyDecay     = "snare.body_decay_ms"
	IDSnareBuzzDecay     = "snare.buzz_decay_ms"
	IDPianoInharmonicity = "piano.inharmonicity_b"
	IDPianoTwoStage      = "piano.two_stage_ratio"
	IDPianoDecayFast     = "piano.decay_fast_db_s"
	IDPianoDecaySlow     = "piano.decay_slow_db_s"
	IDPianoStretch       = "piano.stretch_cents"
	IDWindAttackTime     = "wind.attack_time_sec"
	IDWindCentroidSlope  = "wind.attack_centroid_slope"
	IDWindCentroidTraj   = "wind.attack_centroid_traj"
	IDWindTemporalCent   = "wind.temporal_centroid"
	IDSpectralIrregular  = "partial.spectral_irregularity"
)

// Family group names.
const (
	GroupSnare = "percussive body"
	GroupPiano = "struck string"
	GroupWind  = "wind attack"
)

const (
	naNotPercussive = "signal is not percussive (it establishes a sustain plateau)"
	naNotPitched    = "no reliable fundamental detected"
)

func init() {
	r := DefaultRegistry()

	// Body and buzz bands are measured separately: a whole-signal envelope averages a snare's bottom with its wire snap,
	// which decay at very different rates.

	snareRow := func(id, name, doc string, kind Kind, get func(SnareMetrics) float64) {
		r.Register(Descriptor{
			ID: id, Group: GroupSnare, Name: name,
			Scope: ScopeBand, Repr: ReprSpectrum, Kind: kind,
			Doc:     doc,
			Applies: RequiresPercussive,
			Extract: func(b *Bundle) Measurement {
				if !RequiresPercussive(b) {
					return Unavailable(naNotPercussive)
				}
				return Scalar(get(b.Snare()))
			},
		})
	}

	snareRow(IDSnareBodyFund, "body fundamental",
		"SnareMetrics.BodyFundHz: the dominant body partial — the drum's tuning. "+
			"Distinct from Fingerprint.F0Hz, which assumes a harmonic series a snare does not have.",
		KindHz, func(m SnareMetrics) float64 { return m.BodyFundHz })

	snareRow(IDSnareToneNoise, "tone/noise ratio",
		"SnareMetrics.ToneNoiseRatio: tonal energy / total over the body window. "+
			"1 = pure tone, 0 = pure noise. Measured on the BODY window specifically, "+
			"so the wire buzz cannot mask a hollow-sounding shell.",
		KindFraction, func(m SnareMetrics) float64 { return m.ToneNoiseRatio })

	snareRow(IDSnareLowBody, "low body fraction",
		"SnareMetrics.LowBodyFrac: 80-400 Hz energy fraction — the 'fat bottom'. "+
			"The rimshot complaint ('real is full, deep, lower') is this axis.",
		KindFraction, func(m SnareMetrics) float64 { return m.LowBodyFrac })

	snareRow(IDSnareHighBuzz, "high buzz fraction",
		"SnareMetrics.HighBuzzFrac: 2k-10k Hz energy fraction — the wire 'snap'.",
		KindFraction, func(m SnareMetrics) float64 { return m.HighBuzzFrac })

	snareRow(IDSnareBodyDecay, "body decay",
		"SnareMetrics.BodyDecayMs: low-band -20dB decay time. Reported separately "+
			"from the buzz decay because a real snare's shell rings long while its "+
			"wires die fast; a single whole-signal decay collapses that away.",
		KindSeconds, func(m SnareMetrics) float64 { return m.BodyDecayMs / 1000 })

	snareRow(IDSnareBuzzDecay, "buzz decay",
		"SnareMetrics.BuzzDecayMs: high-band -20dB decay time. See body decay.",
		KindSeconds, func(m SnareMetrics) float64 { return m.BuzzDecayMs / 1000 })

	// Stiff-string inharmonicity (an exactly harmonic stack reads electronic) and double decay: what no general
	// descriptor captures.

	pianoRow := func(id, name, doc string, kind Kind, get func(PianoMetrics) float64) {
		r.Register(Descriptor{
			ID: id, Group: GroupPiano, Name: name,
			Scope: ScopeWhole, Repr: ReprSpectrum, Kind: kind,
			Doc:     doc,
			Applies: RequiresPitched,
			Extract: func(b *Bundle) Measurement {
				if !RequiresPitched(b) {
					return Unavailable(naNotPitched)
				}
				return Scalar(get(b.Piano()))
			},
		})
	}

	pianoRow(IDPianoInharmonicity, "string stiffness B",
		"PianoMetrics.InharmonicityB: the stiff-string stretch coefficient, where 0 "+
			"means exactly harmonic (i.e. electronic). Different from "+
			"partial.inharmonicity, which measures unsigned deviation: B captures the "+
			"PROGRESSIVE sharpening that is specifically a struck-string signature.",
		KindTinyFraction, func(m PianoMetrics) float64 { return m.InharmonicityB })

	pianoRow(IDPianoTwoStage, "double-decay ratio",
		"PianoMetrics.TwoStageRatio: early decay rate / late decay rate. Above ~1.5 "+
			"is the piano double-decay (prompt sound then aftersound); ~1 is a single "+
			"exponential, which is what a synth envelope produces by default.",
		KindRatio, func(m PianoMetrics) float64 { return m.TwoStageRatio })

	pianoRow(IDPianoDecayFast, "prompt decay rate",
		"PianoMetrics.DecayFastDBs: early decay rate in dB/s — the prompt sound.",
		KindDB, func(m PianoMetrics) float64 { return m.DecayFastDBs })

	pianoRow(IDPianoDecaySlow, "aftersound decay rate",
		"PianoMetrics.DecaySlowDBs: late decay rate in dB/s — the aftersound.",
		KindDB, func(m PianoMetrics) float64 { return m.DecaySlowDBs })

	r.Register(Descriptor{
		ID: IDPianoStretch, Group: GroupPiano, Name: "partial stretch",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		// In CENTS, the vector's unit: KindFraction's defaults would call two curves 0.1 cent apart a drift. 5 cents is near
		// audibility, 20 unambiguously out of tune.
		Drift: 5, Mismatch: 20, Unit: "cents",
		Doc: "PianoMetrics.StretchCents: per-partial stretch versus an exact n*f0 series, " +
			"in cents. A VECTOR measurement — the shape of the stretch curve is the " +
			"signature, not any single partial's offset, so it is compared " +
			"component-wise rather than reduced to a scalar first.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naNotPitched)
			}
			st := b.Piano().StretchCents
			if len(st) == 0 {
				return Unavailable("no partials measured for stretch")
			}
			return Vector(st, nil)
		},
	})

	// A wind note's identity lives in its ATTACK TRAJECTORY (swelling and brightening together), not a mid-sustain
	// snapshot.

	windRow := func(id, name, doc string, kind Kind, get func(WindAttack) float64) {
		r.Register(Descriptor{
			ID: id, Group: GroupWind, Name: name,
			Scope: ScopeSegment, Repr: ReprTime, Kind: kind,
			Doc:     doc,
			Applies: RequiresAudible,
			Extract: func(b *Bundle) Measurement {
				if !RequiresAudible(b) {
					return Unavailable(naSilent)
				}
				return Scalar(get(b.WindAttack()))
			},
		})
	}

	windRow(IDWindAttackTime, "attack rise time",
		"WindAttack.AttackTimeSec: 10%->90% RMS rise measured from the DETECTED "+
			"onset rather than from sample 0, so leading silence in a reference clip "+
			"cannot inflate it.",
		KindSeconds, func(m WindAttack) float64 { return m.AttackTimeSec })

	windRow(IDWindCentroidSlope, "attack brightening",
		"WindAttack.CentroidSlopeHzS: OLS slope of the spectral centroid over the "+
			"first 300 ms. POSITIVE means the note brightens as it swells — the "+
			"'brightness tracks loudness' cue that defines real brass and reeds. A "+
			"static synth tone measures ~0 here however well its steady spectrum matches.",
		KindHz, func(m WindAttack) float64 { return m.CentroidSlopeHzS })

	windRow(IDWindTemporalCent, "temporal centroid",
		"WindAttack.TemporalCentroid: energy-weighted mean time of the whole note. "+
			"Small = front-loaded (a struck/plucked shape); larger = a swell.",
		KindSeconds, func(m WindAttack) float64 { return m.TemporalCentroid })

	r.Register(Descriptor{
		ID: IDWindCentroidTraj, Group: GroupWind, Name: "attack centroid trajectory",
		Scope: ScopeSegment, Repr: ReprSpectrum, Kind: KindHz,
		// In HERTZ: KindHz's defaults are percentages and cannot apply to an absolute reduction; these are those percentages
		// at a ~2 kHz attack centroid.
		Drift: 200, Mismatch: 800, Unit: "Hz",
		Doc: "WindAttack.AttackCentroids: spectral centroid at 25 / 50 / 100 / 300 ms " +
			"after onset. A VECTOR, because the SHAPE of the brightening is the cue — " +
			"a note that jumps bright immediately and one that blooms over 300 ms can " +
			"share the same slope while sounding nothing alike.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			if !RequiresAudible(b) {
				return Unavailable(naSilent)
			}
			c := b.WindAttack().AttackCentroids
			return Vector([]float64{c[0], c[1], c[2], c[3]},
				[]string{"25ms", "50ms", "100ms", "300ms"})
		},
	})

	// --- spectral irregularity (partial structure) ----------------------------

	r.Register(Descriptor{
		ID: IDSpectralIrregular, Group: GroupPartials, Name: "spectral irregularity",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Doc: "SpectralIrregularity over Fingerprint.Partials: how jagged the harmonic " +
			"ladder is from one partial to the next. A smooth monotonic roll-off (a " +
			"filtered saw) scores low; a real reed's alternating strong/weak partials " +
			"score high. Orthogonal to the centroid, which a jagged and a smooth " +
			"spectrum can share exactly.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naNotPitched)
			}
			return Scalar(SpectralIrregularity(b.Fingerprint().Partials))
		},
	})
}
