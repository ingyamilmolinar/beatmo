package fingerprint

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// Opens GroupOnset and extends GroupNoise from a later file, a pure addition to the frozen core order.

const (
	IDOnsetBite             = "onset.bite_ratio"
	IDOnsetFlatness         = "onset.flatness"
	IDOnsetPitchGlide       = "onset.pitch_glide_cents"
	IDNoiseResidualCentroid = "noise.residual_centroid_hz"
	IDNoiseDecayVsTone      = "noise.decay_vs_tone"
)

const GroupOnset = "onset character"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDOnsetBite, Group: GroupOnset, Name: "onset bite",
		Scope: ScopeSegment, Repr: ReprTime, Kind: KindRatio,
		Drift: 2.0, Mismatch: 4.0,
		Doc: "Broadband energy (frame energy x flatness) in the first 30 ms after onset, relative " +
			"to the same quantity 300-600 ms in. The bow bite, the tonguing 'ttah', the " +
			"stick crack. Needs no fundamental. The shipped bowed strings read near 1 " +
			"(flat) against references that spike — the 'slightly fluty' verdict.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetBiteRatio(b.Tracks())
			if !ok {
				return Unavailable("clip shorter than 600 ms after onset")
			}
			return Scalar(v)
		},
	})
	r.Register(Descriptor{
		ID: IDOnsetFlatness, Group: GroupOnset, Name: "onset flatness",
		Scope: ScopeSegment, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Spectral flatness of the first 20 ms. Noise-led (clap, snare crack) is high; a " +
			"clean pluck or a soft bow start is low.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetFlatness(b.Wave())
			if !ok {
				return Unavailable("clip too short for a 20 ms onset window")
			}
			return Scalar(v)
		},
	})
	r.Register(Descriptor{
		ID: IDOnsetPitchGlide, Group: GroupOnset, Name: "onset pitch glide",
		Scope: ScopeSegment, Repr: ReprTime, Kind: KindRate, Unit: "cents",
		Drift: 10, Mismatch: 30,
		Doc: "Pitch 20-60 ms after onset minus pitch at 200-320 ms, in cents. A plucked string " +
			"starts sharp and settles; a brass lip slurs in; a synth sits dead on pitch.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetPitchGlideCents(b.Wave(), b.Fingerprint().F0Hz)
			if !ok {
				return Unavailable("pitch not detectable in both onset windows")
			}
			return Scalar(v)
		},
	})
	// Not gated on RequiresPitched alone: claps and snares are exactly what this axis is for, and notching k·f0 of an
	// unreliable f0 is the hi-hat bug. Without a reliable f0 it notches the tracked modes; with neither f0 nor ≥ 2 strong
	// modes there is nothing to notch.
	r.Register(Descriptor{
		ID: IDNoiseResidualCentroid, Group: GroupNoise, Name: "residual centroid",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Drift: 15, Mismatch: 40,
		Doc: "Centroid of the spectrum with +/-1 bin removed around every harmonic partial (a " +
			"reliably pitched signal) or every strong tracked mode (an unpitched " +
			"bar/plate/membrane): WHERE the non-tonal energy lives once whatever real " +
			"periodic structure IS there has been notched out. Falls back to the mode-based " +
			"notch rather than notching multiples of a fabricated fundamental -- the " +
			"hi-hat bug applied to this axis -- so a clap or snare (spuriously " +
			"low-confidence f0) still gets a real notch at ITS OWN detected resonances. " +
			"Hiss is high, snare wire and rattle are mid. The snare verdict ('breathy, " +
			"fuzzy, hissy') was the wrong KIND of noise, not too much of it.",
		Applies: func(b *Bundle) bool { return RequiresAudible(b) && (RequiresPitched(b) || RequiresModes(b)) },
		Extract: func(b *Bundle) Measurement {
			mag, binHz := wave.MagnitudeSpectrum(sustainWindow(b.Wave()), 16384, wave.WindowHann)
			switch {
			case RequiresPitched(b):
				return Scalar(ResidualCentroidHz(mag, binHz, b.Fingerprint().F0Hz))
			case RequiresModes(b):
				modes := b.Modes().Strong(modeStrongWithin)
				freqs := make([]float64, len(modes))
				for i, m := range modes {
					freqs[i] = m.Hz
				}
				return Scalar(ResidualCentroidAtModes(mag, binHz, freqs))
			default:
				return Unavailable("no reliable fundamental and fewer than two strong tracked modes: nothing to notch out")
			}
		},
	})
	r.Register(Descriptor{
		ID: IDNoiseDecayVsTone, Group: GroupNoise, Name: "noise decay vs tone",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindRatio,
		Drift: 1.5, Mismatch: 3.0,
		Doc: "T20 of the residual (noise) energy track divided by T20 of the harmonic energy " +
			"track. Above 1 the noise outlives the tone ('noisy throughout' — the clap " +
			"verdict); below 1 the noise belongs to the onset. REQUIRES a reliable f0: unlike " +
			"residual centroid, the residual/harmonic SPLIT here comes from " +
			"Tracks.NoiseFraction, which is only meaningful when the harmonic-series " +
			"assumption behind it holds (the same reason noise.fraction requires it).",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			v, ok, reason := NoiseDecayVsTone(b.Tracks())
			if !ok {
				return Unavailable(reason)
			}
			return Scalar(v)
		},
	})
}
