package fingerprint

const (
	IDPartialBandwidth    = "partial.bandwidth_cents"
	IDPartialIndependence = "partial.independence"
)

const GroupAliveness = "partial decorrelation"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDPartialBandwidth, Group: GroupAliveness, Name: "partial bandwidth",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "cents",
		Drift: 5, Mismatch: 15,
		Doc: "Mean excess -3 dB width of the strong partials on a 32768-point spectrum of the " +
			"sustain, in cents, after subtracting the window's own main lobe. A locked sine is " +
			"0; a real harmonic is a band of slightly detuned components. The PADsynth " +
			"argument, measured. Requires >= 0.5 s of sustain: the 32768-point analysis needs " +
			"~0.74 s at 44.1kHz to resolve, and a shorter plateau cannot support it.",
		Applies: func(b *Bundle) bool {
			return RequiresModes(b) && RequiresSustain(b) && b.Segments().SustainSec >= 0.5
		},
		Extract: func(b *Bundle) Measurement {
			v, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(modeStrongWithin))
			if !ok {
				return Unavailable("sustain window shorter than 32768 samples, or fewer than two partials had a resolvable width")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDPartialIndependence, Group: GroupAliveness, Name: "partial independence",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "1 minus the mean pairwise correlation of the strong partials' detrended level " +
			"tracks. Phase-locked additive synthesis under one envelope reads ~0; partials " +
			"that breathe on their own read higher. This is 'sterile' vs 'alive' as a number.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := PartialIndependence(b.Modes().Strong(modeStrongWithin))
			if !ok {
				return Unavailable("no pair of partials shared eight frames")
			}
			return Scalar(v)
		},
	})
}
