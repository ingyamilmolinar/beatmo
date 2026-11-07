package fingerprint

// Joins GroupMod from a later file, a pure addition that leaves the frozen core axes in order.

const (
	IDModAMRate        = "mod.am.rate_hz"
	IDModAMDepth       = "mod.am.depth"
	IDModAMBands       = "mod.am.bands"
	IDModAMFlatness    = "mod.am.flatness"
	IDModCentroidDepth = "mod.centroid.depth"
)

const naModShort = "clip too short for a modulation spectrum (needs 32 frames)"

// Past the frame-count gate, the hop rate still leaves no bin in 0.5–30 Hz: a resolution collapse, reported as such.
const naModCollapsed = "clip's hop rate leaves no usable frequency bin across 0.5–30 Hz (a resolution collapse, not a frame-count shortfall)"

// A decay's curvature, which a linear detrend cannot remove, piles into the lowest bin and wins the peak; reporting
// that bin as "the rate" dresses up an analysis artefact, which is why the axis scored 0/11 on the preference
// pairs.
const naModRateFloor = "dominant modulation sits in the lowest usable bin — envelope leakage, not a rate"

func modApplies(b *Bundle) bool { return RequiresAudible(b) && b.Tracks().NumFrames >= modMinFrames }

func amSpectrum(b *Bundle) (ModSpectrum, bool) {
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	return ms, ms.OK
}

// Past the frame gate an !OK result is either the flagged collapse or a degenerate track (near-zero mean).
func modNAReason(ms ModSpectrum) string {
	if ms.resolutionCollapsed {
		return naModCollapsed
	}
	return naModShort
}

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDModAMRate, Group: GroupMod, Name: "loudness modulation rate",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindRate, Unit: "Hz",
		Drift: 1, Mismatch: 3,
		Doc: "Dominant rate of the detrended loudness track's spectrum over 0.5–30 Hz. A bow " +
			"stroke sits near 1 Hz, vibrato/tremolo 4–7 Hz, a bell's beat 1–4 Hz, chaos nowhere. " +
			"NOT MEASURABLE when the peak lands on the LOWEST usable bin: the detrend removes a " +
			"straight line and an exponential decay is not one, so a decay-only note's residual " +
			"envelope curvature wins that bin on every clip and would report the analysis " +
			"window's own floor frequency as a modulation rate.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(modNAReason(ms))
			}
			if ms.rateAtFloorBin {
				return Unavailable(naModRateFloor)
			}
			return Scalar(ms.RateHz)
		},
	})
	r.Register(Descriptor{
		ID: IDModAMDepth, Group: GroupMod, Name: "loudness modulation depth",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15,
		Doc: "Share of modulation power at the dominant rate, scaled by the loudness track's " +
			"coefficient of variation: how much the note moves, and how much of that movement " +
			"is ONE shape. The tubular-bells 'waves in time' verdict is a high value here.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(modNAReason(ms))
			}
			return Scalar(ms.Depth)
		},
	})
	r.Register(Descriptor{
		ID: IDModAMBands, Group: GroupMod, Name: "modulation bands",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Energy shares of the loudness modulation spectrum in gesture (0.5–2 Hz), " +
			"vibrato (2–8 Hz) and warble (8–30 Hz). This is the discriminator the program " +
			"doc names as missing: a slow bow stroke and fast chaos can have identical " +
			"variance and opposite band profiles.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(modNAReason(ms))
			}
			return Vector(ms.Bands[:], []string{"gesture", "vibrato", "warble"})
		},
	})
	r.Register(Descriptor{
		ID: IDModAMFlatness, Group: GroupMod, Name: "modulation flatness",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Spectral flatness of the loudness modulation spectrum. A shaped gesture is " +
			"peaky (low); random amplitude noise is flat (high). 'Wobbles like a machine' " +
			"versus 'breathes'.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(modNAReason(ms))
			}
			return Scalar(ms.Flatness)
		},
	})
	r.Register(Descriptor{
		ID: IDModCentroidDepth, Group: GroupMod, Name: "brightness modulation depth",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15,
		Doc: "Same measurement on the spectral-centroid track: does the brightness move with " +
			"the note? A real flute's vibrato swings the centroid ~40%; a synth LFO on " +
			"amplitude alone leaves it still.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			tr := b.Tracks()
			ms := ModulationSpectrum(tr.Centroid, tr.HopSec)
			if !ms.OK {
				return Unavailable(modNAReason(ms))
			}
			return Scalar(ms.Depth)
		},
	})
}
