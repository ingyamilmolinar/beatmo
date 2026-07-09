package fingerprint

import (
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Kick-drum fingerprinting: TIME-RESOLVED analysis for tuning a synthesized kick
// against a reference kick WAV.
//
// Motivation: averaged scalar metrics (spectral centroid, flatness, a single
// band split) can HIDE organic complexity. A real "spit"/sputter bug in the kick
// synth was invisible to averaged spectral descriptors — it only showed up when
// the signal was analyzed window-by-window over time. So EVERY metric here is a
// trace over time, and KickDistance compares those traces point-by-point rather
// than collapsing them to scalars. The scalar landmarks (attack-peak time,
// plateau flatness, tail-bloom presence) are derived FROM the traces and carry
// only a supporting weight.
//
// All windowing is done on the peak-normalized signal. Window/hop sizes are
// named constants derived from the sample rate.

// --- window/hop policy (seconds) ---------------------------------------------

const (
	// Envelope: a 20 ms analysis window (>= one period of the lowest kick
	// fundamental ~50 Hz, so intra-cycle RMS ripple is suppressed) advanced by a
	// 5 ms hop (the "~5 ms" time resolution the trace reports). Overlapping the
	// window is deliberate: a bare 5 ms window under-resolves a 50-100 Hz
	// fundamental and its RMS ripples within each cycle, which would fabricate
	// spurious envelope minima/maxima and defeat the tail-bloom detector.
	kickEnvWindowSec = 0.020
	kickEnvHopSec    = 0.005

	// Pitch: ~15 ms window (spec) advanced by a 5 ms hop for fine time resolution
	// through a fast pitch glide.
	kickPitchWindowSec = 0.015
	kickPitchHopSec    = 0.005

	// Bands: ~15 ms non-overlapping window; each window is zero-padded to
	// kickBandFFT for adequate low-frequency bin resolution.
	kickBandWindowSec = 0.015
	kickBandHopSec    = 0.015

	// HF ratio ("spit" detector): a short 10 ms window advanced by 5 ms so a brief
	// sputter shows up as a sharp jump between consecutive windows, while the
	// window is long enough to keep single-cycle leakage from dominating.
	kickHFWindowSec = 0.010
	kickHFHopSec    = 0.005

	// The attack transient of a kick is legitimately broadband (the beater click);
	// that is not "spit". HFRatioMaxJump is therefore measured only after this
	// initial window so a normal attack does not read as a sputter.
	kickHFAttackSkipSec = 0.020

	// A pitch window is treated as voiced only when its RMS is at least this
	// fraction of the loudest window's RMS (≈ −12 dB). Below it the note has
	// decayed into the tail where a dominant-frequency estimate is just noise;
	// the settle pitch is derived from the sustained body, not the dying tail.
	kickVoicedFrac = 0.25

	// FFT sizes for the per-window spectra (both power-of-two; the short windows
	// are zero-padded up to these lengths inside wave.MagnitudeSpectrum).
	kickBandFFT = 4096
	kickHFFFT   = 1024

	// HF cutoff separating "spit" energy from body energy.
	kickHFCutoffHz = 1000.0

	// Pitch scan grid.
	kickPitchLoHz   = 20.0
	kickPitchHiHz   = 400.0
	kickPitchStepHz = 5.0

	// -12 dB envelope level (fraction of peak) bounding the plateau region.
	kickMinus12dB = 0.25118864 // 10^(-12/20)

	// Tail-bloom thresholds.
	kickBloomSigMin  = 0.85 // a "significant" post-peak minimum must dip below this (fraction of peak)
	kickBloomRatio   = 0.15 // the later maximum must reach at least this fraction of the global peak
	kickBloomMinSec  = 0.04 // ...and occur at least this long after the attack peak
	kickDistResample = 64   // common length traces are resampled to before pointwise comparison

	// Fundamental-dominance ("kick vs tom") band split. The LOW band is the kick
	// fundamental region; the MID band is where a tom's inharmonic body modes
	// live. AttackLowMidRatio = energy(LOW)/energy(MID) over the loud onset.
	kickLowBandLoHz = 40.0
	kickLowBandHiHz = 100.0
	kickMidBandLoHz = 100.0
	kickMidBandHiHz = 500.0

	// A window counts toward the LOUD onset (for AttackLowMidRatio) when its RMS
	// is at least this fraction of the loudest window's RMS.
	kickAttackLoudFrac = 0.50

	// Reverb-tail thresholds (fractions of the envelope peak). The tail region
	// begins where the envelope FIRST falls to kickTailStartFrac of peak; the
	// tail is considered "ringing" while it stays above kickTailFloorFrac.
	kickTailStartFrac = 0.10
	kickTailFloorFrac = 0.02
)

// KickBandCount is the number of frequency bands in the per-window band trace.
const KickBandCount = 7

// KickBands returns the fixed 7-band split used by the band-energy trace, in
// ascending order. The top band runs up to Nyquist (clamped at analysis time).
func KickBands() []BandEdge {
	return []BandEdge{
		{Name: "<60", LoHz: 0, HiHz: 60},
		{Name: "60-120", LoHz: 60, HiHz: 120},
		{Name: "120-250", LoHz: 120, HiHz: 250},
		{Name: "250-500", LoHz: 250, HiHz: 500},
		{Name: "500-1000", LoHz: 500, HiHz: 1000},
		{Name: "1000-2000", LoHz: 1000, HiHz: 2000},
		{Name: ">2000", LoHz: 2000, HiHz: 30000},
	}
}

// KickFingerprint holds the time-resolved descriptors of a kick drum.
type KickFingerprint struct {
	SampleRate int

	// --- 1. Envelope trace + landmarks -------------------------------------
	EnvTrace  []float64 // RMS envelope, normalized to peak (0..1)
	EnvHopSec float64   // seconds between EnvTrace samples

	AttackPeakSec   float64 // time of the global envelope peak
	AttackRiseSec   float64 // 10%→90% rise time
	PlateauFlatness float64 // how sustained (flat, non-exponential) the post-attack region is; 0..1, higher = flatter

	TailBloomPresent bool    // a secondary envelope maximum after the initial decay
	TailBloomSec     float64 // time of that bloom relative to the attack peak
	TailBloomRatio   float64 // bloom level / global peak

	// --- 2. Pitch-glide trace ----------------------------------------------
	PitchTrace    []float64 // dominant frequency (Hz) per window, 20-400 Hz
	PitchHopSec   float64
	PitchStartHz  float64 // first window's pitch
	PitchSettleHz float64 // median of the last third
	GlideSec      float64 // time until pitch first comes within 10% of settle

	// --- 3. Per-window band-energy trace -----------------------------------
	BandTrace  [][]float64 // outer = window, inner = KickBandCount energy fractions (sum ~1)
	BandAvg    []float64   // per-band mean over all windows
	BandHopSec float64

	// --- 4. Punch / transient ----------------------------------------------
	Crest          float64   // peak / RMS over the whole signal
	HFRatioTrace   []float64 // fraction of energy above kickHFCutoffHz per short window
	HFRatioHopSec  float64
	HFRatioMaxJump float64 // max |Δ| between consecutive HFRatioTrace windows (smoothness guard)

	// --- 5. Timbre (spectral color + noisiness) ----------------------------
	// The axes that separate a BRIGHT, NOISY real recorded kick from a dull,
	// pure-tonal synth even when band energies + crest match (diagnosed on the
	// sandyrb acoustic ref: centroid 872 vs 99 Hz, flatness 0.086 vs 0.000).
	// Band energies throw away the fine spectral SHAPE; these keep it.
	CentroidTrace  []float64   // spectral centroid (Hz) per body window — brightness over time
	CentroidHopSec float64     //
	FlatnessAvg    float64     // mean spectral flatness (0=pure tone … 1=white noise) over voiced windows
	AttackCentroid float64     // spectral centroid (Hz) of the first ~20 ms (beater-click brightness)
	MFCC           [13]float64 // mel-cepstral timbre fingerprint (perceptual spectral shape)

	// --- 6. Body character (kick-vs-tom) + reverb tail ---------------------
	// Two axes the timbre/band terms miss. (a) Fundamental dominance: a KICK's
	// loud onset is fundamental-dominant (energy in 40–100 Hz >> 100–500 Hz),
	// whereas a TOM's loud onset is MID-dominant (prominent inharmonic body
	// modes at 100–500 Hz). Band fractions alone can't separate them because a
	// tom can share a kick's overall low-frequency weighting once its long ring
	// is averaged in — the discriminator is the ratio at the LOUD onset only.
	// (b) Reverb/echo tail: a recorded kick has a low-level tail that persists
	// well past the main hit; a dry synth kick has none.
	LowMidRatioTrace  []float64 // per body window: energy(40–100 Hz) / energy(100–500 Hz)
	LowMidHopSec      float64   // seconds between LowMidRatioTrace samples
	AttackLowMidRatio float64   // low/mid energy ratio over the LOUD onset windows only (kick≫1, tom<1); PRIMARY kick-vs-tom scalar

	TailRatio       float64 // mean tail-region RMS / peak RMS (tail = after env first falls to 10% of peak); recorded≈0.03–0.05, dry synth≈0
	TailDurationSec float64 // time from the main peak until the envelope RMS last exceeds 2% of peak (how long the tail rings)
	TailFlatness    float64 // spectral flatness of the tail region (reverb is smooth/diffuse → higher)

	// --- 7. Sensory dissonance + per-band decay ----------------------------
	// Two axes every energy/shape metric above is blind to. (a) Roughness: the
	// Plomp–Levelt/Sethares sensory-dissonance of the sustained body — the
	// beating/intermodulation between close partials heard as a "creak"/buzz
	// (a saturating-inharmonic-modes bug was invisible to band/centroid/MFCC).
	// See kick_roughness.go. (b) BandT60: the decay time (T60) of EACH frequency
	// band independently — a real kick's sub rings long while its click dies in
	// ms; the whole-signal envelope collapses that away. See kick_decay.go.
	Roughness float64 // sensory dissonance of the body partials; 0 (consonant/pure) → higher (rough/buzzy)

	BandT60        []float64 // per-band 60 dB decay time (seconds), len KickBandCount; 0 = band too quiet to measure
	BandT60Floored []bool    // per-band: T60 clamped to remaining signal length (decay too slow to reach −60 dB)
	DecayHopSec    float64   // seconds between the per-band decay-envelope samples

	// --- 8. Temporal spectral change (flux + warble) ----------------------
	// Two axes every AVERAGED/STATIC spectral statistic above is blind to: a
	// slow pitch/formant SWEEP or an amplitude WARBLE leaves BandAvg/FlatnessAvg/
	// MFCC unchanged while the ear hears movement. See kick_flux.go.
	SpectralFluxAvg float64 // mean per-frame spectral flux over the sustained (post-attack) region; level-invariant. Steady tone ≈ 0, sweep/creak > 0
	WarbleDepth     float64 // 2–30 Hz amplitude-modulation depth of the mid-band body (log-detrended CoV); 0 = steady, higher = warbling

	// --- 9. Inharmonicity (metallic vs pure) ------------------------------
	// Amplitude-weighted mean mistuning of the body partials from an exact
	// harmonic series of f0 — the axis mode_detune drives. See kick_inharmonicity.go.
	Inharmonicity float64 // 0 = perfectly harmonic, higher = inharmonic/metallic

	// --- 10. MPEG-7 attack descriptors ------------------------------------
	// Perceptual (log-time) transient character + where energy sits in time —
	// axes the linear AttackRiseSec landmark compresses away. See kick_attack.go.
	LogAttackTime       float64 // log10(t_peak − t_start) seconds (NEGATIVE); sharp click ≈ −4, slow swell higher
	TemporalCentroidSec float64 // energy-weighted mean time (seconds); front-loaded → small, long tail → larger
}

// KickAnalyze computes the time-resolved kick fingerprint of w. The signal is
// peak-normalized first (like FromWave), so envelope levels are relative to the
// signal's own peak. A silent input yields a zero fingerprint (hop fields still
// populated).
func KickAnalyze(w wave.Wave) KickFingerprint {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	fp := KickFingerprint{
		SampleRate:    sr,
		EnvHopSec:     kickEnvHopSec,
		PitchHopSec:   kickPitchHopSec,
		BandHopSec:    kickBandHopSec,
		HFRatioHopSec: kickHFWindowSec,
		LowMidHopSec:  kickBandHopSec,
		BandAvg:       make([]float64, KickBandCount),
	}
	if w.PeakSample() < 1e-9 {
		return fp
	}
	w = PeakNormalize(w)
	sr = w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	fp.SampleRate = sr
	samples := w.Samples

	fp.computeEnvelope(samples, sr)
	fp.computePitch(samples, sr)
	fp.computeBands(samples, sr)
	fp.computeHF(samples, sr)
	fp.computeCrest(samples)
	fp.computeTimbre(w, samples, sr)
	fp.computeBodyReverb(w, samples, sr)
	fp.computeRoughness(samples, sr)
	fp.computeDecay(samples, sr)
	fp.computeFlux(samples, sr)
	fp.computeInharmonicity(samples, sr) // after computePitch: uses fp.PitchSettleHz as f0
	fp.computeAttack(samples, sr)
	return fp
}

// computeTimbre fills the spectral-color + noisiness fields: a per-window
// spectral-centroid trace (brightness over time), the mean spectral flatness of
// the voiced windows (noisy vs tonal), the beater-click brightness (attack-
// window centroid), and the 13-coefficient MFCC timbre fingerprint. These are
// what a real recorded kick and a pure-tonal synth diverge on when their band
// energies already agree.
func (fp *KickFingerprint) computeTimbre(w wave.Wave, samples []float64, sr int) {
	win := secToSamples(kickBandWindowSec, sr)
	hop := secToSamples(kickBandHopSec, sr)
	fp.CentroidHopSec = float64(hop) / float64(sr)

	// Peak RMS across windows → a voiced gate so the decayed tail (noise-only,
	// meaningless flatness/centroid) doesn't skew the averages.
	var peakRMS float64
	type wstat struct{ centroid, flatness, rms float64 }
	var stats []wstat
	for start := 0; start < len(samples); start += hop {
		end := start + win
		if end > len(samples) {
			end = len(samples)
		}
		if end-start < 8 {
			break
		}
		seg := samples[start:end]
		mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: sr}, kickBandFFT, wave.WindowHann)
		c := spectralCentroidHz(mag, binHz)
		f := spectralFlatnessOf(mag)
		r := rmsOf(seg)
		if r > peakRMS {
			peakRMS = r
		}
		stats = append(stats, wstat{c, f, r})
	}
	gate := 0.15 * peakRMS
	var fsum float64
	var fn int
	for _, s := range stats {
		fp.CentroidTrace = append(fp.CentroidTrace, s.centroid)
		if s.rms >= gate {
			fsum += s.flatness
			fn++
		}
	}
	if fn > 0 {
		fp.FlatnessAvg = fsum / float64(fn)
	}

	// Attack-window (first ~20 ms) centroid = beater-click brightness.
	aw := secToSamples(0.02, sr)
	if aw > len(samples) {
		aw = len(samples)
	}
	if aw >= 8 {
		mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[:aw], SampleRate: sr}, kickBandFFT, wave.WindowHann)
		fp.AttackCentroid = spectralCentroidHz(mag, binHz)
	}

	fp.MFCC = computeMFCC(w, 13)
}

// spectralCentroidHz is the magnitude-weighted mean frequency of a spectrum.
func spectralCentroidHz(mag []float64, binHz float64) float64 {
	if binHz <= 0 {
		return 0
	}
	num, den := 0.0, 0.0
	for i, m := range mag {
		num += float64(i) * binHz * m
		den += m
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// spectralFlatnessOf is geometric-mean / arithmetic-mean of the magnitude
// spectrum: ~0 for a pure tone, →1 for white noise (the "noisiness" axis).
func spectralFlatnessOf(mag []float64) float64 {
	if len(mag) == 0 {
		return 0
	}
	var logSum, sum float64
	n := 0
	for _, m := range mag {
		if m <= 0 {
			continue
		}
		logSum += math.Log(m)
		sum += m
		n++
	}
	if n == 0 || sum == 0 {
		return 0
	}
	geo := math.Exp(logSum / float64(n))
	arith := sum / float64(n)
	return geo / arith
}

// --- envelope + landmarks -----------------------------------------------------

func (fp *KickFingerprint) computeEnvelope(samples []float64, sr int) {
	win := secToSamples(kickEnvWindowSec, sr)
	hop := secToSamples(kickEnvHopSec, sr)
	fp.EnvHopSec = float64(hop) / float64(sr)

	env := kickRMSEnvelope(samples, win, hop)
	if len(env) == 0 {
		return
	}

	peakIdx, peakVal := argmax(env)
	if peakVal > 0 {
		for i := range env {
			env[i] /= peakVal
		}
	}
	fp.EnvTrace = env
	fp.AttackPeakSec = float64(peakIdx) * fp.EnvHopSec

	// ---- 10%→90% rise time (before the peak) ----
	first10, first90 := -1, -1
	for i := 0; i <= peakIdx && i < len(env); i++ {
		if first10 < 0 && env[i] >= 0.10 {
			first10 = i
		}
		if first10 >= 0 && first90 < 0 && env[i] >= 0.90 {
			first90 = i
		}
	}
	if first10 >= 0 && first90 >= first10 {
		fp.AttackRiseSec = float64(first90-first10) * fp.EnvHopSec
	}

	// ---- PlateauFlatness ----
	// Definition: over the region from the attack peak down to the -12 dB point,
	// compute the coefficient of variation (std/mean) of the envelope and report
	// PlateauFlatness = clamp(1 - CoV, 0, 1). A pure exponential decay drops from
	// peak to a quarter of peak across this region → large spread → high CoV →
	// LOW flatness. A sustained plateau stays near-constant → CoV ≈ 0 → HIGH
	// flatness. (The exponential-fit variant is NOT used because fitting an
	// exponential to a constant also yields near-zero residual, so it cannot tell
	// a plateau from a decay — CoV can.)
	endIdx := len(env) - 1
	for i := peakIdx; i < len(env); i++ {
		if env[i] <= kickMinus12dB {
			endIdx = i
			break
		}
	}
	if endIdx-peakIdx+1 >= 3 {
		region := env[peakIdx : endIdx+1]
		mean := meanOf(region)
		if mean > 1e-9 {
			cov := stdOf(region, mean) / mean
			fp.PlateauFlatness = clamp1(1 - cov)
		}
	}

	// ---- TailBloom ----
	// Rule: after the attack peak, find the first "significant" local minimum
	// (a local min that dips below kickBloomSigMin·peak). Then take the largest
	// local maximum after that minimum. If that later maximum is >= kickBloomRatio
	// of the global peak AND occurs > kickBloomMinSec after the attack peak, a
	// tail bloom is present. (env is normalized so the global peak == 1.)
	minIdx := -1
	for i := peakIdx + 1; i < len(env)-1; i++ {
		if env[i] < env[i-1] && env[i] <= env[i+1] && env[i] < kickBloomSigMin {
			minIdx = i
			break
		}
	}
	if minIdx >= 0 {
		bIdx, bVal := -1, 0.0
		for i := minIdx + 1; i < len(env); i++ {
			if env[i] > bVal {
				bVal = env[i]
				bIdx = i
			}
		}
		if bIdx >= 0 {
			tsec := float64(bIdx-peakIdx) * fp.EnvHopSec
			if bVal >= kickBloomRatio && tsec > kickBloomMinSec {
				fp.TailBloomPresent = true
				fp.TailBloomSec = tsec
				fp.TailBloomRatio = bVal
			}
		}
	}
}

// --- pitch glide --------------------------------------------------------------

func (fp *KickFingerprint) computePitch(samples []float64, sr int) {
	win := secToSamples(kickPitchWindowSec, sr)
	hop := secToSamples(kickPitchHopSec, sr)
	fp.PitchHopSec = float64(hop) / float64(sr)
	if win < 4 {
		return
	}
	// Compute a dominant-frequency estimate AND a window RMS for every window.
	// The full trace (including decayed tail windows) is retained so the trace's
	// index→time mapping stays intact; PitchStartHz / PitchSettleHz / GlideSec are
	// derived only from the VOICED windows so the dead tail (where a
	// dominant-frequency estimate is just noise) cannot skew the settle pitch.
	var rms []float64
	peakRMS := 0.0
	for start := 0; start+win <= len(samples); start += hop {
		seg := samples[start : start+win]
		f := kickDominantPitch(seg, sr, kickPitchLoHz, kickPitchHiHz, kickPitchStepHz)
		fp.PitchTrace = append(fp.PitchTrace, f)
		r := rmsOf(seg)
		rms = append(rms, r)
		if r > peakRMS {
			peakRMS = r
		}
	}
	if len(fp.PitchTrace) == 0 {
		return
	}

	// Collect voiced window indices.
	floor := kickVoicedFrac * peakRMS
	var voiced []int
	for i, r := range rms {
		if r >= floor {
			voiced = append(voiced, i)
		}
	}
	if len(voiced) == 0 {
		// Degenerate (flat) — fall back to the whole trace.
		for i := range fp.PitchTrace {
			voiced = append(voiced, i)
		}
	}

	fp.PitchStartHz = fp.PitchTrace[voiced[0]]
	fp.PitchSettleHz = medianLastThirdVoiced(fp.PitchTrace, voiced)

	settle := fp.PitchSettleHz
	if settle > 0 {
		for _, i := range voiced {
			if math.Abs(fp.PitchTrace[i]-settle) <= 0.10*settle {
				fp.GlideSec = float64(i) * fp.PitchHopSec
				break
			}
		}
	}
}

// --- band-energy trace --------------------------------------------------------

func (fp *KickFingerprint) computeBands(samples []float64, sr int) {
	win := secToSamples(kickBandWindowSec, sr)
	hop := secToSamples(kickBandHopSec, sr)
	fp.BandHopSec = float64(hop) / float64(sr)
	bands := KickBands()

	for start := 0; start < len(samples); start += hop {
		end := start + win
		if end > len(samples) {
			end = len(samples)
		}
		if end-start < 8 {
			break
		}
		seg := wave.Wave{Samples: samples[start:end], SampleRate: sr}
		fp.BandTrace = append(fp.BandTrace, kickBandFractions(seg, bands, kickBandFFT))
	}

	fp.BandAvg = make([]float64, KickBandCount)
	for _, row := range fp.BandTrace {
		for i := 0; i < KickBandCount && i < len(row); i++ {
			fp.BandAvg[i] += row[i]
		}
	}
	if n := len(fp.BandTrace); n > 0 {
		for i := range fp.BandAvg {
			fp.BandAvg[i] /= float64(n)
		}
	}
}

// --- HF ratio (spit detector) -------------------------------------------------

func (fp *KickFingerprint) computeHF(samples []float64, sr int) {
	win := secToSamples(kickHFWindowSec, sr)
	hop := secToSamples(kickHFHopSec, sr)
	fp.HFRatioHopSec = float64(hop) / float64(sr)

	// Full windows only: a ragged final partial window has a different length and
	// FFT support, which fabricates a spurious jump at the end of the trace.
	for start := 0; start+win <= len(samples); start += hop {
		seg := wave.Wave{Samples: samples[start : start+win], SampleRate: sr}
		fp.HFRatioTrace = append(fp.HFRatioTrace, kickHFRatio(seg, kickHFCutoffHz, kickHFFFT))
	}
	// MaxJump over the body only (skip the attack transient — see
	// kickHFAttackSkipSec). A smoothly-varying HFRatio ⇒ small jump; a spitty
	// kick oscillates ⇒ large jump.
	skip := int(kickHFAttackSkipSec / fp.HFRatioHopSec)
	if skip < 1 {
		skip = 1
	}
	for i := skip; i < len(fp.HFRatioTrace); i++ {
		if j := math.Abs(fp.HFRatioTrace[i] - fp.HFRatioTrace[i-1]); j > fp.HFRatioMaxJump {
			fp.HFRatioMaxJump = j
		}
	}
}

func (fp *KickFingerprint) computeCrest(samples []float64) {
	if len(samples) == 0 {
		return
	}
	peak, sumSq := 0.0, 0.0
	for _, s := range samples {
		if a := math.Abs(s); a > peak {
			peak = a
		}
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(len(samples)))
	if rms > 1e-12 {
		fp.Crest = peak / rms
	}
}

// --- body character (kick-vs-tom) + reverb tail -------------------------------

// computeBodyReverb fills the fundamental-dominance and reverb-tail fields.
//
// Fundamental dominance: over each body window (same window/hop as the band
// trace) it takes the ratio energy(40–100 Hz)/energy(100–500 Hz). A kick's LOUD
// onset is fundamental-dominant (ratio ≫ 1); a tom's loud onset is mid-dominant
// (ratio < 1). AttackLowMidRatio aggregates that ratio over the loud onset only
// (windows whose RMS ≥ kickAttackLoudFrac of the peak window) as a SUM-of-energy
// ratio, which is more stable than averaging per-window ratios that blow up when
// a window's mid energy is near zero.
//
// Reverb tail: using the same RMS envelope as computeEnvelope, it locates the
// tail region (everything after the envelope first falls to 10 % of peak) and
// reports the tail's mean level relative to peak (TailRatio), how long it keeps
// ringing above 2 % of peak (TailDurationSec), and the tail's spectral flatness
// (reverb is smooth/diffuse). A dry synth kick reads ~0 on all three.
func (fp *KickFingerprint) computeBodyReverb(w wave.Wave, samples []float64, sr int) {
	// ---- fundamental dominance (kick vs tom) ----
	win := secToSamples(kickBandWindowSec, sr)
	hop := secToSamples(kickBandHopSec, sr)
	fp.LowMidHopSec = float64(hop) / float64(sr)

	type lmwin struct{ low, mid, rms float64 }
	var wins []lmwin
	peakRMS := 0.0
	for start := 0; start < len(samples); start += hop {
		end := start + win
		if end > len(samples) {
			end = len(samples)
		}
		if end-start < 8 {
			break
		}
		seg := samples[start:end]
		mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: sr}, kickBandFFT, wave.WindowHann)
		low := bandEnergy(mag, binHz, kickLowBandLoHz, kickLowBandHiHz)
		mid := bandEnergy(mag, binHz, kickMidBandLoHz, kickMidBandHiHz)
		r := rmsOf(seg)
		if r > peakRMS {
			peakRMS = r
		}
		ratio := 0.0
		if mid > 1e-20 {
			ratio = low / mid
		}
		fp.LowMidRatioTrace = append(fp.LowMidRatioTrace, ratio)
		wins = append(wins, lmwin{low, mid, r})
	}
	// AttackLowMidRatio: summed-energy ratio over the loud onset windows.
	loud := kickAttackLoudFrac * peakRMS
	var lowSum, midSum float64
	for _, wn := range wins {
		if wn.rms >= loud {
			lowSum += wn.low
			midSum += wn.mid
		}
	}
	if midSum > 1e-20 {
		fp.AttackLowMidRatio = lowSum / midSum
	}

	// ---- reverb / echo tail ----
	ewin := secToSamples(kickEnvWindowSec, sr)
	ehop := secToSamples(kickEnvHopSec, sr)
	env := kickRMSEnvelope(samples, ewin, ehop)
	if len(env) == 0 {
		return
	}
	peakIdx, peakVal := argmax(env)
	if peakVal <= 1e-12 {
		return
	}
	envHop := float64(ehop) / float64(sr)

	// Tail start: first index after the peak where env falls to 10 % of peak.
	tailStart := -1
	for i := peakIdx; i < len(env); i++ {
		if env[i] <= kickTailStartFrac*peakVal {
			tailStart = i
			break
		}
	}
	if tailStart >= 0 && tailStart < len(env) {
		fp.TailRatio = meanOf(env[tailStart:]) / peakVal
	}

	// Tail duration: from the main peak to the LAST window above 2 % of peak.
	lastIdx := peakIdx
	for i := peakIdx; i < len(env); i++ {
		if env[i] > kickTailFloorFrac*peakVal {
			lastIdx = i
		}
	}
	fp.TailDurationSec = float64(lastIdx-peakIdx) * envHop

	// Tail flatness: spectral flatness of the tail-region samples (reuse the
	// shared helper). Only when the tail region has enough support for an FFT.
	if tailStart >= 0 {
		s0 := tailStart * ehop
		if s0 < len(samples) && len(samples)-s0 >= 8 {
			mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[s0:], SampleRate: sr}, kickBandFFT, wave.WindowHann)
			fp.TailFlatness = spectralFlatnessOf(mag)
		}
	}
}

// bandEnergy sums |mag|² over the FFT bins covering [loHz, hiHz], DC excluded.
func bandEnergy(mag []float64, binHz, loHz, hiHz float64) float64 {
	if binHz <= 0 {
		return 0
	}
	lo := int(loHz / binHz)
	if lo < 1 { // skip DC
		lo = 1
	}
	hi := int(hiHz / binHz)
	if hi >= len(mag) {
		hi = len(mag) - 1
	}
	sum := 0.0
	for k := lo; k <= hi; k++ {
		sum += mag[k] * mag[k]
	}
	return sum
}

// --- distance -----------------------------------------------------------------

// KickWeights sets the relative contribution of each KickDistance sub-term. The
// three trace terms (EnvShape, Pitch, Bands) intentionally carry the most weight
// so the metric stays honest — no single averaged scalar can dominate.
type KickWeights struct {
	EnvShape      float64
	Pitch         float64
	Bands         float64
	Landmarks     float64
	Crest         float64
	Smoothness    float64
	Timbre        float64
	LowMid        float64
	Reverb        float64
	Roughness     float64
	Decay         float64
	Flux          float64
	Inharmonicity float64
	Attack        float64
}

// DefaultKickWeights weights the point-by-point trace terms above the scalar
// terms. A tuner may pass its own weights to KickDistanceWeighted.
var DefaultKickWeights = KickWeights{
	EnvShape:   2.0,  // envelope shape over time — the primary organic-complexity signal
	Pitch:      1.5,  // pitch-glide trajectory
	Bands:      1.5,  // per-window spectral balance
	Landmarks:  1.0,  // attack-peak time, plateau flatness, tail-bloom presence/time
	Crest:      0.5,  // scalar punch
	Smoothness: 0.75, // HF-ratio trace diff + excess-jump penalty
	Timbre:     2.0,  // spectral color (MFCC + centroid) + noisiness — bright/noisy REAL vs dull/pure synth
	LowMid:     1.5,  // fundamental-dominance (kick vs tom) — pulls a re-tune away from mid-dominant/tom-like
	Reverb:     1.0,  // reverb/echo tail presence + duration + diffuseness
	Roughness:  1.0,  // sensory dissonance (buzz/creak) of the body — a supporting axis; kept below the trace/timbre terms
	Decay:      0.75, // per-band decay-time (T60) mismatch — how each region rings out; below Roughness by design

	Flux:          0.75, // spectral flux (sweep/creak) + warble (2–30 Hz tremolo) — supporting axis, below the trace/timbre terms
	Inharmonicity: 0.75, // partial-mistuning (metallic vs pure) gap — supporting axis
	Attack:        0.75, // MPEG-7 log-attack-time + temporal-centroid gap — supporting axis
}

// KickDistanceBreakdown holds the per-term and total weighted kick distance.
// Lower is more similar. Total == 0 for identical fingerprints. Symmetric.
type KickDistanceBreakdown struct {
	EnvShape   float64 // resampled EnvTrace, mean |Δ| (both are 0..1 normalized)
	Pitch      float64 // resampled PitchTrace log-Hz diff + settle/glide terms
	Bands      float64 // resampled per-window per-band fraction diff
	Landmarks  float64 // attack-peak time + plateau flatness + tail-bloom presence/time
	Crest      float64 // relative crest difference
	Smoothness float64 // HF-ratio trace diff + |Δ max-jump|
	Timbre     float64 // MFCC (color) + centroid-trace (brightness) + flatness (noisiness)
	LowMid     float64 // fundamental-dominance: |log Δ AttackLowMidRatio| + resampled LowMidRatioTrace diff
	Reverb     float64 // reverb tail: scaled |Δ TailRatio| + |Δ TailDurationSec| + |Δ TailFlatness|
	Roughness  float64 // sensory-dissonance gap: |Δ Roughness| × kickRoughDistScale
	Decay      float64 // per-band T60 mismatch: mean over measurable bands of |log((a+eps)/(b+eps))|

	Flux          float64 // temporal spectral change: |Δ SpectralFluxAvg|×scale + |Δ WarbleDepth|×scale
	Inharmonicity float64 // partial-mistuning gap: |Δ Inharmonicity| × kickInharmDistScale
	Attack        float64 // MPEG-7: |Δ LogAttackTime| + |log ratio of TemporalCentroidSec|

	Total float64 // weighted sum
}

// KickDistance compares two kick fingerprints using DefaultKickWeights.
func KickDistance(a, b KickFingerprint) KickDistanceBreakdown {
	return KickDistanceWeighted(a, b, DefaultKickWeights)
}

// KickDistanceWeighted compares two kick fingerprints with explicit weights.
func KickDistanceWeighted(a, b KickFingerprint, w KickWeights) KickDistanceBreakdown {
	const L = kickDistResample
	var d KickDistanceBreakdown

	// EnvShape: resample both normalized envelopes to a common length and take
	// the mean absolute pointwise difference.
	d.EnvShape = meanAbsDiff(resampleLinear(a.EnvTrace, L), resampleLinear(b.EnvTrace, L))

	// Pitch: pointwise |log2(ratio)| over the resampled traces (octave distance),
	// plus a relative settle term and a scaled glide-time term.
	d.Pitch = kickPitchDistance(a, b, L)

	// Bands: per-window, per-band fraction difference (NOT just BandAvg — averaged
	// bands hide time drift). Each band series is resampled over the window axis.
	d.Bands = kickBandDistance(a, b, L)

	// Landmarks: derived scalars.
	d.Landmarks = kickLandmarkDistance(a, b)

	// Crest: relative difference.
	d.Crest = relDiff(a.Crest, b.Crest)

	// Smoothness: HF-ratio trace difference plus the excess-jump penalty.
	d.Smoothness = meanAbsDiff(resampleLinear(a.HFRatioTrace, L), resampleLinear(b.HFRatioTrace, L)) +
		math.Abs(a.HFRatioMaxJump-b.HFRatioMaxJump)

	// Timbre: MFCC color + centroid brightness trajectory + noisiness.
	d.Timbre = kickTimbreDistance(a, b, L)

	// LowMid: fundamental-dominance (kick-vs-tom). The scalar AttackLowMidRatio
	// difference in log-space (the +0.2 floor keeps a tom's near-zero ratio from
	// blowing the log up) plus the pointwise ratio-trace difference. This is the
	// term that pulls a re-tune away from tom-like (mid-dominant) toward kick-like.
	d.LowMid = math.Abs(math.Log((a.AttackLowMidRatio+0.2)/(b.AttackLowMidRatio+0.2))) +
		meanAbsDiff(resampleLinear(a.LowMidRatioTrace, L), resampleLinear(b.LowMidRatioTrace, L))

	// Reverb: tail presence + duration + diffuseness. TailRatio/TailFlatness are
	// small (a few %) so the ratio term is scaled ×8; TailDurationSec ×2.
	d.Reverb = 8*math.Abs(a.TailRatio-b.TailRatio) +
		2*math.Abs(a.TailDurationSec-b.TailDurationSec) +
		math.Abs(a.TailFlatness-b.TailFlatness)

	// Roughness: absolute sensory-dissonance gap, scaled to a comparable range.
	d.Roughness = math.Abs(a.Roughness-b.Roughness) * kickRoughDistScale

	// Decay: mean per-band |log ratio| of T60 (scale-robust). Bands where either
	// side is unmeasurable (T60 ≤ 0 — too quiet to fit a decay) are skipped.
	d.Decay = kickDecayDistance(a.BandT60, b.BandT60)

	// Flux: temporal spectral change. The sustained-flux gap (sweep/creak) plus the
	// warble-depth gap (2–30 Hz tremolo), each scaled to a comparable range.
	d.Flux = kickFluxDistScale*math.Abs(a.SpectralFluxAvg-b.SpectralFluxAvg) +
		kickWarbleDistScale*math.Abs(a.WarbleDepth-b.WarbleDepth)

	// Inharmonicity: absolute partial-mistuning gap, scaled up (raw range ~0–0.1).
	d.Inharmonicity = math.Abs(a.Inharmonicity-b.Inharmonicity) * kickInharmDistScale

	// Attack: MPEG-7 log-attack-time gap (already log-domain) + the temporal-centroid
	// gap as a log ratio (scale-robust; the +eps floor guards a near-zero centroid).
	d.Attack = math.Abs(a.LogAttackTime-b.LogAttackTime) +
		math.Abs(math.Log((a.TemporalCentroidSec+kickAttackEps)/(b.TemporalCentroidSec+kickAttackEps)))

	d.Total = w.EnvShape*d.EnvShape + w.Pitch*d.Pitch + w.Bands*d.Bands +
		w.Landmarks*d.Landmarks + w.Crest*d.Crest + w.Smoothness*d.Smoothness +
		w.Timbre*d.Timbre + w.LowMid*d.LowMid + w.Reverb*d.Reverb +
		w.Roughness*d.Roughness + w.Decay*d.Decay +
		w.Flux*d.Flux + w.Inharmonicity*d.Inharmonicity + w.Attack*d.Attack
	return d
}

// kickDecayDistance is the mean over measurable bands of the absolute log-ratio of
// per-band T60 (a scale-robust ratio distance). A band is measurable only when
// BOTH fingerprints report a positive T60 for it (a band too quiet to fit a decay
// carries T60 ≤ 0 and is skipped). Returns 0 when no band is jointly measurable.
func kickDecayDistance(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	sum := 0.0
	cnt := 0
	for i := 0; i < n; i++ {
		if a[i] <= 0 || b[i] <= 0 {
			continue
		}
		sum += math.Abs(math.Log((a[i] + kickDecayEps) / (b[i] + kickDecayEps)))
		cnt++
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

// kickTimbreDistance measures spectral-color + noisiness divergence: a scaled
// MFCC L2 (dropping C0 = loudness), the per-window spectral-centroid trajectory
// in octaves (brightness over time), the attack-window centroid (beater-click
// brightness), and the flatness (noisy-vs-tonal) gap. These are the axes a real
// recorded kick and a pure-tonal synth diverge on when band energies already
// agree, so this term is what pulls tuning toward the reference's actual color.
func kickTimbreDistance(a, b KickFingerprint, L int) float64 {
	// MFCC coeffs 1..12 (skip loudness), scaled to ~unit range.
	mfcc := 0.0
	for i := 1; i < 13; i++ {
		d := (a.MFCC[i] - b.MFCC[i]) / 20.0
		mfcc += d * d
	}
	mfcc = math.Sqrt(mfcc / 12.0)

	// Centroid trajectory: octave distance per window (log-frequency; +25 Hz
	// floor avoids blowups on near-silent windows).
	ca := resampleLinear(a.CentroidTrace, L)
	cb := resampleLinear(b.CentroidTrace, L)
	cent := 0.0
	for i := 0; i < L; i++ {
		cent += math.Abs(math.Log2((ca[i] + 25) / (cb[i] + 25)))
	}
	if L > 0 {
		cent /= float64(L)
	}

	// Attack-click brightness (octaves).
	atk := math.Abs(math.Log2((a.AttackCentroid + 25) / (b.AttackCentroid + 25)))

	// Noisiness (flatness) gap, scaled up (values are small, ~0–0.2).
	flat := math.Abs(a.FlatnessAvg-b.FlatnessAvg) * 4

	return mfcc + cent + 0.5*atk + flat
}

func kickPitchDistance(a, b KickFingerprint, L int) float64 {
	pa := resampleLinear(a.PitchTrace, L)
	pb := resampleLinear(b.PitchTrace, L)
	sum, n := 0.0, 0
	for i := 0; i < L; i++ {
		if pa[i] > 1 && pb[i] > 1 {
			sum += math.Abs(math.Log2(pa[i] / pb[i]))
			n++
		}
	}
	trace := 0.0
	if n > 0 {
		trace = sum / float64(n)
	}
	settle := relDiff(a.PitchSettleHz, b.PitchSettleHz)
	glide := math.Abs(a.GlideSec-b.GlideSec) / 0.1 // scale by 100 ms
	return trace + 0.5*settle + 0.25*glide
}

func kickBandDistance(a, b KickFingerprint, L int) float64 {
	sum, cnt := 0.0, 0
	for band := 0; band < KickBandCount; band++ {
		sa := resampleLinear(bandColumn(a.BandTrace, band), L)
		sb := resampleLinear(bandColumn(b.BandTrace, band), L)
		for i := 0; i < L; i++ {
			sum += math.Abs(sa[i] - sb[i])
			cnt++
		}
	}
	if cnt == 0 {
		return 0
	}
	return sum / float64(cnt)
}

func kickLandmarkDistance(a, b KickFingerprint) float64 {
	peak := math.Abs(a.AttackPeakSec-b.AttackPeakSec) / 0.1 // scale by 100 ms
	plateau := math.Abs(a.PlateauFlatness - b.PlateauFlatness)
	bloom := 0.0
	switch {
	case a.TailBloomPresent != b.TailBloomPresent:
		bloom = 1.0
	case a.TailBloomPresent: // both present
		bloom = math.Abs(a.TailBloomSec-b.TailBloomSec)/0.2 + math.Abs(a.TailBloomRatio-b.TailBloomRatio)
	}
	return peak + plateau + bloom
}

// --- low-level helpers --------------------------------------------------------

// kickRMSEnvelope computes an overlapping RMS envelope: window win samples,
// advanced by hop samples. Unlike RMSEnvelope in signal.go (window == hop), this
// allows win > hop so a low fundamental's intra-cycle ripple is smoothed while
// the trace keeps fine time resolution.
func kickRMSEnvelope(samples []float64, win, hop int) []float64 {
	if win < 1 {
		win = 1
	}
	if hop < 1 {
		hop = 1
	}
	var env []float64
	for start := 0; start < len(samples); start += hop {
		end := start + win
		if end > len(samples) {
			end = len(samples)
		}
		if end <= start {
			break
		}
		sum := 0.0
		for _, s := range samples[start:end] {
			sum += s * s
		}
		env = append(env, math.Sqrt(sum/float64(end-start)))
	}
	return env
}

// kickDominantPitch returns the frequency (Hz) with maximum Goertzel power over
// [loHz, hiHz] scanned at stepHz. A Hann window is applied first to reduce
// spectral leakage. Goertzel (a single-bin DFT) is used rather than an FFT so
// the scan grid can be finer than an FFT bin — and it does not duplicate the
// shared FFT in internal/wave.
func kickDominantPitch(samples []float64, sr int, loHz, hiHz, stepHz float64) float64 {
	n := len(samples)
	if n < 4 || sr <= 0 || stepHz <= 0 {
		return 0
	}
	seg := make([]float64, n)
	fn := float64(n)
	for i, s := range samples {
		coeff := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/fn)) // periodic Hann
		seg[i] = s * coeff
	}
	best, bestF := -1.0, 0.0
	for f := loHz; f <= hiHz; f += stepHz {
		if p := kickGoertzelPower(seg, f, sr); p > best {
			best = p
			bestF = f
		}
	}
	return bestF
}

// kickGoertzelPower returns the |X(f)|² power of samples at an arbitrary
// (non-bin-aligned) frequency via the Goertzel recurrence.
func kickGoertzelPower(samples []float64, freq float64, sr int) float64 {
	if len(samples) == 0 || sr <= 0 {
		return 0
	}
	w := 2 * math.Pi * freq / float64(sr)
	coeff := 2 * math.Cos(w)
	s1, s2 := 0.0, 0.0
	for _, x := range samples {
		s0 := x + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}
	p := s1*s1 + s2*s2 - coeff*s1*s2
	if p < 0 {
		p = 0
	}
	return p
}

// kickBandFractions returns the fraction of spectral energy (mag², DC excluded)
// falling in each band, normalized to sum ~1 across bands.
func kickBandFractions(seg wave.Wave, bands []BandEdge, fftSize int) []float64 {
	mag, binHz := wave.MagnitudeSpectrum(seg, fftSize, wave.WindowHann)
	out := make([]float64, len(bands))
	if binHz <= 0 {
		return out
	}
	total := 0.0
	for i, b := range bands {
		lo := int(b.LoHz / binHz)
		if lo < 1 { // skip DC
			lo = 1
		}
		hi := int(b.HiHz / binHz)
		if hi >= len(mag) {
			hi = len(mag) - 1
		}
		sum := 0.0
		for k := lo; k <= hi; k++ {
			sum += mag[k] * mag[k]
		}
		out[i] = sum
		total += sum
	}
	if total > 1e-20 {
		for i := range out {
			out[i] /= total
		}
	}
	return out
}

// kickHFRatio returns the fraction of spectral energy (DC excluded) above cutHz.
func kickHFRatio(seg wave.Wave, cutHz float64, fftSize int) float64 {
	mag, binHz := wave.MagnitudeSpectrum(seg, fftSize, wave.WindowHann)
	if binHz <= 0 {
		return 0
	}
	cut := int(cutHz / binHz)
	total, hf := 0.0, 0.0
	for k := 1; k < len(mag); k++ {
		e := mag[k] * mag[k]
		total += e
		if k >= cut {
			hf += e
		}
	}
	if total < 1e-20 {
		return 0
	}
	return hf / total
}

// resampleLinear resamples src to exactly n points via linear interpolation.
// An empty src yields zeros; a single-element src is held constant.
func resampleLinear(src []float64, n int) []float64 {
	out := make([]float64, n)
	if n <= 0 {
		return out
	}
	switch len(src) {
	case 0:
		return out
	case 1:
		for i := range out {
			out[i] = src[0]
		}
		return out
	}
	if n == 1 {
		out[0] = src[0]
		return out
	}
	last := float64(len(src) - 1)
	for i := 0; i < n; i++ {
		pos := float64(i) * last / float64(n-1)
		lo := int(pos)
		frac := pos - float64(lo)
		if lo+1 < len(src) {
			out[i] = src[lo]*(1-frac) + src[lo+1]*frac
		} else {
			out[i] = src[len(src)-1]
		}
	}
	return out
}

// bandColumn extracts the time series of one band across all windows.
func bandColumn(trace [][]float64, band int) []float64 {
	out := make([]float64, len(trace))
	for i, row := range trace {
		if band < len(row) {
			out[i] = row[band]
		}
	}
	return out
}

func secToSamples(sec float64, sr int) int {
	n := int(sec * float64(sr))
	if n < 1 {
		n = 1
	}
	return n
}

func argmax(xs []float64) (int, float64) {
	idx, val := 0, math.Inf(-1)
	for i, v := range xs {
		if v > val {
			val = v
			idx = i
		}
	}
	if len(xs) == 0 {
		return 0, 0
	}
	return idx, val
}

func meanOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range xs {
		sum += v
	}
	return sum / float64(len(xs))
}

func stdOf(xs []float64, mean float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range xs {
		d := v - mean
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)))
}

func meanAbsDiff(a, b []float64) float64 {
	n := len(a)
	if n == 0 || len(b) != n {
		return 0
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += math.Abs(a[i] - b[i])
	}
	return sum / float64(n)
}

// relDiff is a symmetric relative difference in [0,1): |a-b|/(|a|+|b|).
func relDiff(a, b float64) float64 {
	denom := math.Abs(a) + math.Abs(b)
	if denom < 1e-12 {
		return 0
	}
	return math.Abs(a-b) / denom
}

func rmsOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range xs {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(xs)))
}

// medianLastThirdVoiced returns the median pitch over the last third of the
// voiced-window indices.
func medianLastThirdVoiced(trace []float64, voiced []int) float64 {
	if len(voiced) == 0 {
		return 0
	}
	start := 2 * len(voiced) / 3
	if start >= len(voiced) {
		start = len(voiced) - 1
	}
	seg := make([]float64, 0, len(voiced)-start)
	for _, i := range voiced[start:] {
		if i < len(trace) {
			seg = append(seg, trace[i])
		}
	}
	if len(seg) == 0 {
		return 0
	}
	sort.Float64s(seg)
	m := len(seg)
	if m%2 == 0 {
		return (seg[m/2-1] + seg[m/2]) / 2
	}
	return seg[m/2]
}
