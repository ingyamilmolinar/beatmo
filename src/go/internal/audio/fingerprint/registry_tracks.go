package fingerprint

// registry_tracks.go — whole-signal statistics as registry rows.
//
// Every row here reads a frame-wise track (tracks.go) reduced by TrackStats
// (trackstats.go). This is the "overall statistically for the entire wave"
// half of the battery, and it is deliberately a small CURATED selection from
// a much larger cross-product: six tracks times twelve statistics is
// seventy-two candidates, and registering all of them would bury the report
// under near-duplicates before the Phase 6 orthogonality analysis has had a
// chance to say which are redundant.
//
// The selection rule applied here: register the statistics that answer a
// question no EXISTING descriptor already answers. Mean centroid is skipped
// because bright.centroid already reports brightness; centroid SLOPE and
// CURVATURE are registered because nothing in the battery could previously
// see a note that brightens as it swells. When Phase 6 measures the
// correlation matrix, this list gets pruned or extended on evidence.

// Whole-signal statistic descriptor IDs.
const (
	IDTrackCentroidSlope     = "track.centroid.slope"
	IDTrackCentroidCurvature = "track.centroid.curvature"
	IDTrackCentroidIQR       = "track.centroid.iqr"
	IDTrackRMSSlope          = "track.rms.slope"
	IDTrackRMSCurvature      = "track.rms.curvature"
	IDTrackRMSMonotonic      = "track.rms.monotonic"
	IDTrackRMSIQR            = "track.rms.iqr"
	IDTrackFlatnessSlope     = "track.flatness.slope"
	IDTrackFlatnessMedian    = "track.flatness.median"
	IDTrackNoiseSlope        = "track.noise.slope"
	IDTrackNoiseP90          = "track.noise.p90"
	IDTrackCrestMedian       = "track.crest.median"
	IDTrackH2MinusH1Slope    = "track.h2_minus_h1.slope"
)

// GroupWholeSignal is the report group for frame-wise whole-signal
// statistics.
const GroupWholeSignal = "whole-signal statistics"

const naNoFrames = "signal too short for frame-wise analysis"

// trackRow registers one (track, statistic) descriptor.
//
// pick receives the already-computed Tracks and returns the track to reduce;
// stat pulls the wanted field out of the reduction. Splitting it this way
// keeps each row a one-liner while the extraction stays type-safe.
func trackRow(id, name, doc string, kind Kind, repr Repr,
	drift, mismatch float64, unit string,
	needsPitch bool,
	pick func(*Tracks) []float64,
	stat func(TrackStats) float64) {

	applies := RequiresAudible
	if needsPitch {
		applies = func(b *Bundle) bool { return RequiresAudible(b) && RequiresPitched(b) }
	}
	DefaultRegistry().Register(Descriptor{
		ID: id, Group: GroupWholeSignal, Name: name,
		Scope: ScopeTrack, Repr: repr, Kind: kind,
		Drift: drift, Mismatch: mismatch, Unit: unit,
		Doc:     doc,
		Applies: applies,
		Extract: func(b *Bundle) Measurement {
			tr := b.Tracks()
			if tr.NumFrames < 2 {
				return Unavailable(naNoFrames)
			}
			if needsPitch && tr.F0Hz <= 0 {
				return Unavailable(naNotPitched)
			}
			return Scalar(stat(StatsOf(pick(tr), tr.HopSec)))
		},
	})
}

func init() {
	// --- brightness over time -------------------------------------------------

	trackRow(IDTrackCentroidSlope, "centroid slope",
		"OLS slope of the per-frame spectral centroid, in Hz/sec over the WHOLE "+
			"signal. Positive means the note brightens as it plays. This is the cue "+
			"that took four failed trumpet attempts to find: the real instrument "+
			"blooms 1382->2070 Hz across the note, and a mid-sustain snapshot sees "+
			"none of it.",
		KindRate, ReprSpectrum, 150, 500, "Hz/s", false,
		func(t *Tracks) []float64 { return t.Centroid },
		func(s TrackStats) float64 { return s.Slope })

	trackRow(IDTrackCentroidCurvature, "centroid curvature",
		"Quadratic coefficient of the centroid trajectory (Hz/sec^2). Separates a "+
			"steady brightening ramp from an accelerating bloom or an onset blat that "+
			"settles — three shapes that can share one slope.",
		KindRate, ReprSpectrum, 500, 2000, "Hz/s²", false,
		func(t *Tracks) []float64 { return t.Centroid },
		func(s TrackStats) float64 { return s.Curvature })

	trackRow(IDTrackCentroidIQR, "centroid spread",
		"Interquartile range of the per-frame centroid (Hz). How much the brightness "+
			"wanders, robust to the one-off outlier frame that inflates a plain range.",
		KindHz, ReprSpectrum, 0, 0, "Hz", false,
		func(t *Tracks) []float64 { return t.Centroid },
		func(s TrackStats) float64 { return s.IQR })

	// --- amplitude shape over time --------------------------------------------
	//
	// These three together are the direct fix for the documented failure where
	// a metric's MAGNITUDE matched while its CHARACTER was opposite. The
	// violin's std/mean was tuned to 0.40 against a real 0.64 and still sounded
	// wrong, because the real variation is a slow bow-stroke shape and the
	// synth's was fast chaos. IQR captures the magnitude; slope and curvature
	// capture the shape; monotonic fraction captures gesture-vs-chaos.

	trackRow(IDTrackRMSSlope, "loudness slope",
		"OLS slope of the per-frame RMS envelope over the whole signal. Positive is "+
			"a crescendo, negative a decay. The trumpet's 500 ms swell is this axis.",
		KindRate, ReprTime, 0.05, 0.2, "1/s", false,
		func(t *Tracks) []float64 { return t.RMS },
		func(s TrackStats) float64 { return s.Slope })

	trackRow(IDTrackRMSCurvature, "loudness curvature",
		"Quadratic coefficient of the RMS envelope. Distinguishes a linear fade from "+
			"an exponential decay or an accelerating swell at the same overall slope.",
		KindRate, ReprTime, 0.1, 0.4, "1/s²", false,
		func(t *Tracks) []float64 { return t.RMS },
		func(s TrackStats) float64 { return s.Curvature })

	trackRow(IDTrackRMSMonotonic, "loudness gesture",
		"Fraction of consecutive RMS frames moving in the majority direction. Near 1 "+
			"is a shaped gesture (a bow stroke, a decay); near 0.5 is chaos. Two "+
			"signals with identical amplitude VARIANCE separate cleanly here — which "+
			"is exactly the discrimination std/mean could not make.",
		KindFraction, ReprTime, 0, 0, "", false,
		func(t *Tracks) []float64 { return t.RMS },
		func(s TrackStats) float64 { return s.MonotonicFrac })

	trackRow(IDTrackRMSIQR, "loudness spread",
		"Interquartile range of the per-frame RMS. The magnitude half of the "+
			"magnitude/shape split, robust to outlier frames.",
		KindFraction, ReprTime, 0, 0, "", false,
		func(t *Tracks) []float64 { return t.RMS },
		func(s TrackStats) float64 { return s.IQR })

	// --- noise character over time ---------------------------------------------

	trackRow(IDTrackFlatnessSlope, "flatness slope",
		"OLS slope of per-frame spectral flatness (1/sec). NEGATIVE means the note "+
			"starts noisy and cleans up — the breath/bow/tonguing attack signature. "+
			"Real winds and brass run ~0.5-0.8 noise at onset and settle to ~0.02; a "+
			"synth with constant noise measures ~0 here however well its average matches.",
		KindRate, ReprSpectrum, 0.02, 0.1, "1/s", false,
		func(t *Tracks) []float64 { return t.Flatness },
		func(s TrackStats) float64 { return s.Slope })

	trackRow(IDTrackFlatnessMedian, "flatness median",
		"Median per-frame spectral flatness over the whole signal. The whole-signal "+
			"counterpart to noise.flatness, which measures one mid-sustain window; a "+
			"large gap between the two means the noise character is not stationary.",
		KindFraction, ReprSpectrum, 0, 0, "", false,
		func(t *Tracks) []float64 { return t.Flatness },
		func(s TrackStats) float64 { return s.Median })

	trackRow(IDTrackNoiseSlope, "noise-fraction slope",
		"OLS slope of the per-frame noise fraction (1/sec). The attack-noise decay "+
			"the trumpet work measured directly (0.6 -> 0.02 across the note).",
		KindRate, ReprSpectrum, 0.05, 0.2, "1/s", true,
		func(t *Tracks) []float64 { return t.NoiseFraction },
		func(s TrackStats) float64 { return s.Slope })

	trackRow(IDTrackNoiseP90, "peak noise fraction",
		"90th-percentile per-frame noise fraction — how noisy the NOISIEST part of "+
			"the note gets. A whole-signal average hides a violently noisy 50 ms "+
			"onset; this does not.",
		KindFraction, ReprSpectrum, 0, 0, "", true,
		func(t *Tracks) []float64 { return t.NoiseFraction },
		func(s TrackStats) float64 { return s.P90 })

	// --- transient character over time -------------------------------------------

	trackRow(IDTrackCrestMedian, "crest median",
		"Median per-frame crest factor (peak/RMS). High throughout means a spiky, "+
			"impulsive waveform; near 1.4 is a steady sine. Measured per frame so a "+
			"single transient cannot dominate the whole-signal figure.",
		KindRatio, ReprTime, 0, 0, "", false,
		func(t *Tracks) []float64 { return t.Crest },
		func(s TrackStats) float64 { return s.Median })

	// --- spectral bloom ------------------------------------------------------------

	DefaultRegistry().Register(Descriptor{
		ID: IDTrackH2MinusH1Slope, Group: GroupWholeSignal, Name: "harmonic bloom",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindRate,
		Drift: 1.0, Mismatch: 4.0, Unit: "dB/s",
		Doc: "OLS slope of (H2 - H1) in dB/sec across the whole note: does the second " +
			"harmonic grow RELATIVE to the fundamental as the note plays? This is " +
			"'brightness tracks loudness' measured on the harmonic ladder itself " +
			"rather than inferred from the centroid, and it is level-invariant by " +
			"construction — an overall crescendo moves H1 and H2 together and cancels, " +
			"so only a genuine change in harmonic BALANCE registers.",
		Applies: func(b *Bundle) bool { return RequiresAudible(b) && RequiresPitched(b) },
		Extract: func(b *Bundle) Measurement {
			tr := b.Tracks()
			if tr.NumFrames < 2 {
				return Unavailable(naNoFrames)
			}
			if tr.F0Hz <= 0 {
				return Unavailable(naNotPitched)
			}
			diff := make([]float64, tr.NumFrames)
			for i := range diff {
				diff[i] = tr.Harmonics[1][i] - tr.Harmonics[0][i]
			}
			return Scalar(StatsOf(diff, tr.HopSec).Slope)
		},
	})
}
