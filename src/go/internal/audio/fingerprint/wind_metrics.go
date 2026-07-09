package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// wind_metrics.go — onset-aligned attack analysis, spectral irregularity and
// formant-band energies. These target the axes the sustain-window Fingerprint
// cannot see for wind instruments:
//
//   - AutoSegment picks the LOUDEST window, which lands mid-note, so
//     Fingerprint.AttackTimeSec measured on it describes level ripple, not the
//     note's attack. OnsetSegment aligns to the true tone onset instead (a 10%
//     threshold with a 1% backtrack, so a breath/inhale preroll at a few % of
//     peak — present in real wind recordings — does not trigger it).
//   - Winds bloom: the spectral centroid RISES through the attack as upper
//     harmonics arrive later than lower ones. WindAttack captures the centroid
//     trajectory anchored at the onset.
//   - Real horns have a jagged harmonic envelope (tonehole-lattice ripple);
//     naive saw+LP patches are too smooth. SpectralIrregularity (Krimphoff)
//     quantifies that jaggedness.
//   - The sax identity lives in fixed formant regions (bari: knee ~450-900 Hz,
//     tract band ~0.9-2 kHz, brilliance 2-8 kHz). FormantBandEnergies measures
//     the energy split across those bands.

// WindAttack holds onset-aligned attack descriptors for a sustained
// (wind/brass/bowed) note.
type WindAttack struct {
	OnsetSec         float64    // position of the detected tone onset in the input wave
	AttackTimeSec    float64    // 10% → 90% RMS rise time measured from the onset
	AttackCentroids  [4]float64 // spectral centroid at 25 / 50 / 100 / 300 ms after onset (Hz)
	CentroidSlopeHzS float64    // OLS slope of the centroid trajectory over the first 300 ms (Hz/sec)
	TemporalCentroid float64    // energy-weighted mean time of the whole note (sec, from onset)
}

// windAttackFFT is the STFT frame size used for the attack centroid
// trajectory: 2048 samples ≈ 43 ms at 48 kHz — short enough to localize the
// early attack, long enough to resolve a 123 Hz fundamental's harmonics.
const windAttackFFT = 2048

// OnsetSegment returns the sub-wave starting at the detected tone onset (up to
// durSec seconds) plus the onset time in seconds. Detection: the RMS envelope
// must reach 10% of its peak (so a breath preroll at a few % of peak does not
// trigger), then the start is backtracked to the last frame below 1% of peak
// (so the measured window still contains the full rise). Returns the input
// unchanged with onset 0 when it is silent or too short.
func OnsetSegment(w wave.Wave, durSec float64) (wave.Wave, float64) {
	const hop = 256
	env := RMSEnvelope(w, hop, 0)
	if len(env) == 0 || w.SampleRate <= 0 {
		return w, 0
	}
	peak := 0.0
	for _, v := range env {
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 {
		return w, 0
	}
	trigger := -1
	for i, v := range env {
		if v >= 0.10*peak {
			trigger = i
			break
		}
	}
	if trigger < 0 {
		return w, 0
	}
	// Backtrack down the rising edge to its valley: walk back while the
	// envelope keeps (roughly) descending and stays above the breath/silence
	// floor (1.5% of peak). A fixed low threshold alone fails on recordings
	// whose breath preroll hovers around 1-2% of peak.
	start := trigger
	for start > 0 && env[start] >= 0.015*peak && env[start-1] <= env[start]*1.05 {
		start--
	}
	startSample := start * hop
	end := len(w.Samples)
	if want := float64(w.SampleRate) * durSec; !math.IsInf(want, 1) &&
		startSample+int(want) < end {
		end = startSample + int(want)
	}
	out := make([]float64, end-startSample)
	copy(out, w.Samples[startSample:end])
	return wave.Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label},
		float64(startSample) / float64(w.SampleRate)
}

// ComputeWindAttack measures onset-aligned attack descriptors on the FULL note
// wave w (not a pre-segmented sustain window).
func ComputeWindAttack(w wave.Wave) WindAttack {
	var wa WindAttack
	if w.SampleRate <= 0 || len(w.Samples) == 0 {
		return wa
	}
	seg, onset := OnsetSegment(w, math.Inf(1))
	wa.OnsetSec = onset
	if len(seg.Samples) < windAttackFFT {
		return wa
	}
	sr := seg.SampleRate

	// 10→90% rise from the onset.
	const hop = 256
	env := RMSEnvelope(seg, hop, 0)
	peak := 0.0
	for _, v := range env {
		if v > peak {
			peak = v
		}
	}
	if peak <= 0 {
		return wa
	}
	first10, first90 := -1, -1
	for i, v := range env {
		if first10 < 0 && v >= 0.10*peak {
			first10 = i
		}
		if first10 >= 0 && v >= 0.90*peak {
			first90 = i
			break
		}
	}
	if first10 >= 0 && first90 >= first10 {
		wa.AttackTimeSec = float64(first90-first10) * float64(hop) / float64(sr)
	}

	// Centroid trajectory: frames centered at 25/50/100/300 ms after onset.
	marks := []float64{0.025, 0.050, 0.100, 0.300}
	var ts, cs []float64
	for i, m := range marks {
		center := int(m * float64(sr))
		lo := center - windAttackFFT/2
		if lo < 0 {
			lo = 0
		}
		hi := lo + windAttackFFT
		if hi > len(seg.Samples) {
			break
		}
		frame := wave.Wave{Samples: seg.Samples[lo:hi], SampleRate: sr}
		mag, binHz := wave.MagnitudeSpectrum(frame, windAttackFFT, wave.WindowHann)
		c := SpectralCentroid(mag, binHz)
		wa.AttackCentroids[i] = c
		ts = append(ts, m)
		cs = append(cs, c)
	}
	if len(ts) >= 2 {
		wa.CentroidSlopeHzS = olsSlope(ts, cs)
	}

	// Temporal centroid of the whole note from the onset (MPEG-7).
	var num, den float64
	for i, v := range env {
		t := float64(i*hop) / float64(sr)
		num += t * v
		den += v
	}
	if den > 0 {
		wa.TemporalCentroid = num / den
	}
	return wa
}

// SpectralIrregularity is Krimphoff's jaggedness measure over the harmonic
// amplitude envelope: the mean absolute deviation (in dB) of each partial from
// the 3-point moving average of its neighbours. Zero-amplitude partials at the
// tail are excluded. A smooth 1/n saw envelope scores near 0; real horns with
// tonehole-lattice ripple score several dB.
func SpectralIrregularity(partials [16]float64) float64 {
	// Find the last non-negligible partial so trailing zeros don't dilute.
	last := 0
	peak := 0.0
	for _, p := range partials {
		if p > peak {
			peak = p
		}
	}
	if peak <= 0 {
		return 0
	}
	for i, p := range partials {
		if p > peak*1e-3 {
			last = i
		}
	}
	if last < 2 {
		return 0
	}
	db := make([]float64, last+1)
	for i := 0; i <= last; i++ {
		db[i] = 20 * math.Log10(math.Max(partials[i], peak*1e-6))
	}
	sum := 0.0
	n := 0
	for i := 1; i < last; i++ {
		avg := (db[i-1] + db[i] + db[i+1]) / 3
		sum += math.Abs(db[i] - avg)
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// WindFormantBandsHz are the default band edges for baritone-register winds:
// below the bore knee, the knee/first-formant band, the tract-influenced band,
// and the brilliance band.
var WindFormantBandsHz = []float64{0, 450, 900, 2000, 8000}

// FormantBandEnergies returns each band's fraction of the total energy
// (magnitude²) within [bands[0], bands[len-1]]. bands are ascending Hz edges;
// the result has len(bands)-1 entries summing to 1 (all zeros for silence).
func FormantBandEnergies(w wave.Wave, bands []float64) []float64 {
	nb := len(bands) - 1
	out := make([]float64, nb)
	if nb < 1 || w.SampleRate <= 0 || len(w.Samples) == 0 {
		return out
	}
	mag, binHz := wave.MagnitudeSpectrum(w, 16384, wave.WindowHann)
	total := 0.0
	for b, m := range mag {
		hz := float64(b) * binHz
		if hz < bands[0] || hz >= bands[nb] {
			continue
		}
		e := m * m
		total += e
		for k := 0; k < nb; k++ {
			if hz >= bands[k] && hz < bands[k+1] {
				out[k] += e
				break
			}
		}
	}
	if total > 0 {
		for k := range out {
			out[k] /= total
		}
	}
	return out
}
