package fingerprint

// registry_timbre.go — vibrato, formants and MFCC, reconnected.
//
// Same argument as registry_family.go: computeVibrato, computeSpectralEnvelope
// and computeMFCC all run on EVERY Fingerprint already, their results are
// stored on it, and until now nothing read them. The battery was paying for
// three analyzers and reporting none of them.
//
// These also close the gap where Descriptor.Repr declared ReprCepstral and
// ReprMel with no descriptor anywhere using either -- a taxonomy that named
// domains the report could not actually reach.

// Timbre descriptor IDs.
const (
	IDVibratoRate    = "vibrato.rate_hz"
	IDVibratoExtent  = "vibrato.extent_cents"
	IDVibratoJitter  = "vibrato.jitter"
	IDVibratoAMDepth = "vibrato.am_depth"
	IDFormant1       = "formant.f1_hz"
	IDFormant2       = "formant.f2_hz"
	IDFormantBridge  = "formant.bridge_hill_hz"
	IDMFCCProfile    = "cepstral.mfcc"
)

// Timbre group names.
const (
	GroupVibrato  = "vibrato"
	GroupFormants = "formants / body resonance"
	GroupCepstral = "cepstral timbre"
)

const (
	naNoVibrato = "no vibrato detected (band power below the noise floor, or extent under 2 cents)"
	naNoFormant = "no formant peak found in this band"
)

func init() {
	r := DefaultRegistry()

	// --- vibrato ------------------------------------------------------------
	//
	// A bowed or sung note without vibrato reads as synthetic no matter how
	// well its steady spectrum matches, and vibrato has FOUR independent
	// parameters that a listener hears separately: how fast, how wide, how
	// regular, and whether loudness moves with pitch. Collapsing them would
	// lose exactly the distinctions being tuned -- a 4 Hz and a 7 Hz vibrato
	// at equal extent sound nothing alike.

	vibratoRow := func(id, name, doc string, kind Kind, drift, mismatch float64, unit string,
		get func(VibratoFingerprint) float64) {
		r.Register(Descriptor{
			ID: id, Group: GroupVibrato, Name: name,
			Scope: ScopeSegment, Repr: ReprModulation, Kind: kind,
			Drift: drift, Mismatch: mismatch, Unit: unit,
			Doc:     doc,
			Applies: RequiresPitched,
			Extract: func(b *Bundle) Measurement {
				if !RequiresPitched(b) {
					return Unavailable(naUnreliableF0(b))
				}
				v := b.Fingerprint().Vibrato
				if v == nil || !v.Detected {
					return Unavailable(naNoVibrato)
				}
				return Scalar(get(*v))
			},
		})
	}

	vibratoRow(IDVibratoRate, "vibrato rate",
		"VibratoFingerprint.RateHz: dominant modulation rate of the F0 track. Real "+
			"string and vocal vibrato sits at 4-7 Hz; an LFO left at 2 Hz reads as a "+
			"wobble and at 9 Hz as a bleat, and neither is a matter of DEPTH.",
		KindHz, 0, 0, "Hz", func(v VibratoFingerprint) float64 { return v.RateHz })

	vibratoRow(IDVibratoExtent, "vibrato extent",
		"VibratoFingerprint.ExtentCents: half the p95-p5 pitch excursion, in cents. "+
			"How WIDE the vibrato is, independent of how fast. Thresholds in cents: 15 "+
			"is a noticeable widening, 50 is a different playing style.",
		KindFraction, 15, 50, "cents", func(v VibratoFingerprint) float64 { return v.ExtentCents })

	vibratoRow(IDVibratoJitter, "vibrato jitter",
		"VibratoFingerprint.Jitter: cycle-to-cycle rate irregularity (std/mean of the "+
			"period). A human player's vibrato is never metronomic; an LFO's is exactly "+
			"metronomic and measures ~0 here. This is the 'sounds like an LFO, not a "+
			"player' axis that rate and extent both miss.",
		KindFraction, 0, 0, "", func(v VibratoFingerprint) float64 { return v.Jitter })

	vibratoRow(IDVibratoAMDepth, "vibrato AM depth",
		"VibratoFingerprint.AMDepth: amplitude modulation synchronous with the pitch "+
			"vibrato. On a real bowed string the two move together because bow speed "+
			"drives both; pure frequency modulation with no amplitude component is a "+
			"synthesis tell.",
		KindFraction, 0, 0, "", func(v VibratoFingerprint) float64 { return v.AMDepth })

	// --- formants / body resonance -------------------------------------------
	//
	// Formants are FIXED resonances of the instrument body: they stay put as
	// the played pitch moves, which is precisely why they read as "this is a
	// violin" rather than "this is a note". A spectral centroid cannot see
	// them -- it moves with the pitch.

	formantRow := func(id, name, doc string, get func(SpectralEnvelope) float64) {
		r.Register(Descriptor{
			ID: id, Group: GroupFormants, Name: name,
			Scope: ScopeBand, Repr: ReprSpectrum, Kind: KindHz,
			Unit:    "Hz",
			Doc:     doc,
			Applies: RequiresAudible,
			Extract: func(b *Bundle) Measurement {
				if !RequiresAudible(b) {
					return Unavailable(naSilent)
				}
				env := b.Fingerprint().SpectralEnv
				if env == nil {
					return Unavailable(naNoFormant)
				}
				hz := get(*env)
				if hz <= 0 {
					return Unavailable(naNoFormant)
				}
				return Scalar(hz)
			},
		})
	}

	formantRow(IDFormant1, "first formant",
		"SpectralEnvelope.Formants[0].Hz: the strongest peak of the smoothed spectral "+
			"envelope. A BODY resonance, fixed as the played pitch moves -- which is why "+
			"it carries instrument identity in a way the pitch-following centroid cannot.",
		func(e SpectralEnvelope) float64 {
			if len(e.Formants) < 1 {
				return 0
			}
			return e.Formants[0].Hz
		})

	formantRow(IDFormant2, "second formant",
		"SpectralEnvelope.Formants[1].Hz: the second-strongest envelope peak. The "+
			"F1/F2 PAIR is what distinguishes vowel-like timbres from each other; either "+
			"alone is ambiguous.",
		func(e SpectralEnvelope) float64 {
			if len(e.Formants) < 2 {
				return 0
			}
			return e.Formants[1].Hz
		})

	formantRow(IDFormantBridge, "bridge hill",
		"SpectralEnvelope.BridgeHillHz: the dominant peak in 1.8-3.3 kHz, the canonical "+
			"violin bridge-hill region. Its presence and placement is a documented "+
			"marker of bowed-string body response; a subtractive synth patch has no "+
			"reason to produce one.",
		func(e SpectralEnvelope) float64 { return e.BridgeHillHz })

	// --- cepstral -------------------------------------------------------------

	r.Register(Descriptor{
		ID: IDMFCCProfile, Group: GroupCepstral, Name: "MFCC profile",
		Scope: ScopeWhole, Repr: ReprCepstral, Kind: KindFraction,
		// Thresholds in MFCC-coefficient units, the vector's own scale. The
		// coefficients are log-energy-derived and run to roughly +/-20 on c0
		// and a few units on the rest, so the KindFraction defaults would flag
		// every pair. 1.5 mean absolute deviation across the profile is an
		// audible timbre difference; 5 is a different instrument.
		Drift: 1.5, Mismatch: 5,
		Doc: "Fingerprint.MFCC: the first 13 Mel-frequency cepstral coefficients of the " +
			"sustain window -- the standard perceptual timbre vector, and the only " +
			"descriptor in the battery reading the CEPSTRAL domain. Compared " +
			"component-wise rather than reduced, because the profile SHAPE is the " +
			"signature. Deliberately kept alongside the individual spectral axes rather " +
			"than replacing them: MFCC says THAT two timbres differ, the named axes say " +
			"HOW, and only the latter is actionable when tuning a synth.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			if !RequiresAudible(b) {
				return Unavailable(naSilent)
			}
			m := b.Fingerprint().MFCC
			return Vector(m[:], nil)
		},
	})
}
