package fingerprint

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// SongFingerprint is a whole-segment musical descriptor of a recording.
type SongFingerprint struct {
	SampleRate           int
	DurationSec          float64
	TempoBPM             float64
	TempoConfidence      float64
	OnsetTimes           []float64
	Chroma               [12]float64
	Key, Mode            int
	KeyConfidence        float64
	PrimaryHz            float64
	Harmonics            []HarmonicPeak
	IntervalHistogram    [12]float64
	IntonationCents      float64
	Bands                []float64
	TonalPercussiveRatio float64
}

// SongFingerprintOf computes all whole-segment musical descriptors of w under cfg.
func SongFingerprintOf(w wave.Wave, cfg AnalysisConfig) SongFingerprint {
	fp := SongFingerprint{SampleRate: w.SampleRate, DurationSec: w.Duration()}

	mag, binHz := wave.MagnitudeSpectrum(w, cfg.FFTSize, wave.WindowHann)
	fp.Bands = BandEnergies(mag, binHz, cfg.Bands)
	fp.TonalPercussiveRatio = TonalPercussiveRatio(w, cfg)
	fp.Chroma = Chroma(mag, binHz)
	fp.Key, fp.Mode, fp.KeyConfidence = DetectKey(fp.Chroma, cfg)
	fp.Harmonics = HarmonicProfile(mag, binHz, 8)
	if len(fp.Harmonics) > 0 {
		fp.PrimaryHz = fp.Harmonics[0].Hz
	}
	fp.IntervalHistogram = IntervalHistogram(fp.Chroma)
	fp.IntonationCents = IntonationCents(mag, binHz)

	env, fhz := OnsetEnvelope(w, cfg)
	fp.OnsetTimes = OnsetTimes(env, fhz, cfg)
	fp.TempoBPM, fp.TempoConfidence = DetectTempo(env, fhz, cfg)
	return fp
}
