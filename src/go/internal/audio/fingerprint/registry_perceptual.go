package fingerprint

import "math"

// Each threshold is anchored on a corpus pair where the listener wrote the word, so crossing it means what the word
// meant.

// Perceptual descriptor IDs.
const (
	IDBrightnessRatio  = "timbre.brightness_ratio"
	IDHarmonicSlope    = "timbre.harmonic_slope"
	IDTristimulus      = "timbre.tristimulus"
	IDNoisePersistence = "noise.persistence"
	IDAirBandFlatness  = "noise.air_band"
)

// GroupPerceptual is the report group for these.
const GroupPerceptual = "perceptual character"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDBrightnessRatio, Group: GroupPerceptual, Name: "brightness ratio",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRatio,
		// 2.0× is the gap the listener called "needs to be much fuller and warmer".
		Drift: 1.4, Mismatch: 2.0,
		Doc: "SpectralCentroid / F0Hz -- where the energy sits, in multiples of the note's " +
			"OWN pitch. The warm/cool axis. bright.centroid compares absolute hertz and so " +
			"conflates timbre with pitch (a note an octave up always measures 'brighter'); " +
			"validated against the listener verdicts that axis scored AUC 0.36, " +
			"ANTI-correlated. Normalising by f0 is what makes this a timbre measurement. " +
			"Worked cases: synth cello 21.6 vs ref 11.1 (\"much fuller and warmer\"), synth " +
			"oboe 14.4 vs ref 6.9 (\"needs warmness\").",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			v := BrightnessRatio(b.Fingerprint())
			if v <= 0 {
				return Unavailable("no fundamental to normalise the centroid by")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDHarmonicSlope, Group: GroupPerceptual, Name: "harmonic slope",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate,
		// References span -6 to -15 dB/oct: 2 is a noticeably different roll-off, 6 another instrument class.
		Drift: 2.0, Mismatch: 6.0, Unit: "dB/oct",
		Doc: "OLS slope of partial level in dB against log2 of harmonic number: how fast the " +
			"harmonic series rolls off. The complement to brightness ratio -- the centroid " +
			"says WHERE the energy sits, this says how quickly it falls away, and a spectrum " +
			"can be dark on average while carrying a few loud high partials. Steep negative " +
			"is dark and fundamental-dominant; shallow is bright and rich.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			v := HarmonicSlope(b.Fingerprint().Partials)
			if math.IsNaN(v) {
				return Unavailable("fewer than three partials carry energy; a slope through two points is not a measurement")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDTristimulus, Group: GroupPerceptual, Name: "tristimulus",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		// Three shares summing to 1: 0.25 is a different balance entirely.
		Drift: 0.08, Mismatch: 0.25,
		Doc: "Fingerprint.Tristimulus: the share of partial energy in the FUNDAMENTAL (T1), " +
			"in partials 2-4 (T2), and in partials 5+ (T3) -- the classic three-band timbre " +
			"balance, computed on every fingerprint and until now read by nothing outside " +
			"the composite distance. Compared component-wise because the BALANCE is the " +
			"signature: the reference viola is overwhelmingly fundamental (T1 0.91) while " +
			"the synth puts almost nothing there (T1 0.05) and everything in the upper bands.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			if !RequiresPitched(b) {
				return Unavailable(naUnreliableF0(b))
			}
			t := b.Fingerprint().Tristimulus
			return Vector([]float64{t[0], t[1], t[2]}, []string{"T1 fundamental", "T2 partials 2-4", "T3 partials 5+"})
		},
	})

	r.Register(Descriptor{
		ID: IDNoisePersistence, Group: GroupPerceptual, Name: "noise persistence",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindRatio,
		// References: strings 0.04–0.25, winds 0.34–0.43; shipped bowed strings ~0.99. 4× is string versus wind.
		Drift: 2.0, Mismatch: 4.0,
		Doc: "Late-half mean noise fraction / early-half mean: does the noise STAY, or does it " +
			"belong to the onset? This is the wind-vs-string axis and the airy-vs-dry axis at " +
			"once, because acoustically they are one question -- a blown instrument is excited " +
			"by turbulent air the whole time it sounds, a bowed or plucked string has a noisy " +
			"attack that settles into a clean tone. A whole-signal noise AVERAGE cannot " +
			"separate them: the same total noise spread evenly or piled at the onset reads " +
			"identically. Every shipped bowed string measures ~0.99 (constant noise) against " +
			"references at 0.04-0.25, and the listener called two of them \"a wind instrument, " +
			"like a flute instead of a string\" and the third \"too airy\".",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			tr := b.Tracks()
			if tr.NumFrames < 4 {
				return Unavailable(naNoFrames)
			}
			if tr.F0Hz <= 0 {
				return Unavailable(naUnreliableF0(b))
			}
			v, ok := NoisePersistence(tr.NoiseFraction)
			if !ok {
				return Unavailable("too little onset noise to form a ratio against")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDAirBandFlatness, Group: GroupPerceptual, Name: "air-band flatness",
		Scope: ScopeBand, Repr: ReprSpectrum, Kind: KindFraction,
		Doc: "Spectral flatness ABOVE 2 kHz: is there broadband hiss sitting on top of the " +
			"tone? noise.flatness measures the whole spectrum, where the strong low partials " +
			"every pitched instrument has dominate the figure and a breathy note scores much " +
			"like a clean one. Restricting to the air band asks the narrower question the word " +
			"'airy' actually means.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			st := b.STFT(4096, 1024)
			if len(st.Frames) == 0 {
				return Unavailable(naNoFrames)
			}
			// Averaged over frames, so it describes the note, not one window.
			avg := make([]float64, len(st.Frames[0]))
			for _, f := range st.Frames {
				for i := range avg {
					if i < len(f) {
						avg[i] += f[i]
					}
				}
			}
			for i := range avg {
				avg[i] /= float64(len(st.Frames))
			}
			v, ok := AirBandFlatness(avg, st.BinHz)
			if !ok {
				return Unavailable("no spectral content above the 2 kHz air band")
			}
			return Scalar(v)
		},
	})
}
