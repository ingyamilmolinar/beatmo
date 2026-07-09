package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_flux.go — SPECTRAL FLUX + WARBLE (temporal spectral change).
//
// Motivation: every existing kick descriptor that looks at the spectrum does so
// on a per-window basis and then AVERAGES (BandAvg, FlatnessAvg, MFCC) or reads a
// single landmark. A STATIC spectral shape can hide temporal misbehaviour: a slow
// pitch/formant SWEEP, an amplitude WARBLE (tremolo), or a periodic "creak" all
// leave the averaged spectrum unchanged while the ear hears movement. Spectral
// flux measures how much the spectrum CHANGES frame-to-frame; warble measures the
// low-rate (2–30 Hz) MODULATION of the body energy. Together they catch sweeps
// and wobbles that the energy/shape statistics miss.
//
//   - SpectralFluxAvg: the mean, over the SUSTAINED (post-attack) region, of the
//     per-frame spectral flux = the half-wave-rectified (positive-change-only) L2
//     norm of the bin-to-bin magnitude difference between consecutive frames,
//     normalized by the current frame's total magnitude (so it is LEVEL-invariant).
//     The onset frame has enormous flux by definition (silence → full spectrum), so
//     the attack transient is excluded — warble/creak lives in the sustain. A steady
//     tone → ~0; a frequency sweep → clearly positive.
//
//   - WarbleDepth: the depth of the 2–30 Hz amplitude modulation of the body. The
//     mid-band (120–600 Hz) amplitude envelope is taken per frame over the
//     post-attack region; its LOG is detrended by removing the linear
//     least-squares fit (an exponential decay is exactly linear in log-amplitude,
//     so this removes the natural decay while leaving oscillation), and WarbleDepth
//     is the standard deviation of the residual — a log-domain coefficient of
//     variation. It is band-limited below by the linear detrend (kills DC + the
//     slow decay trend) and above by the frame Nyquist (~1/(2·hop) ≈ 33 Hz), which
//     brackets the 2–30 Hz warble band. A clean decaying tone → ~0; the same tone
//     with an 8 Hz tremolo → clearly higher.

const (
	// Mid-band used for the warble amplitude envelope: the pitched body of the
	// kick, above the sub-fundamental rumble and below the click brightness.
	kickWarbleMidLoHz = 120.0
	kickWarbleMidHiHz = 600.0

	// The attack transient is excluded from BOTH flux and warble (the onset frame's
	// flux is huge by construction, and the click is broadband). Everything from
	// this time on is the "sustain" where warble/creak lives. Reuses the same
	// perceptual attack boundary as the segment analyzer (kick_segments.go).
	kickFluxAttackSkipSec = segAttackEndMs / 1000 // 35 ms

	// Distance scales bring the small raw ranges (flux ~0–0.3, warble ~0–0.5) up to
	// the same order as the other KickDistance sub-terms.
	kickFluxDistScale   = 5.0
	kickWarbleDistScale = 2.0
)

// computeFlux fills fp.SpectralFluxAvg + fp.WarbleDepth. Mirrors the computeX
// pattern in kick_metrics.go: one pass of per-window spectra, flux accumulated
// between consecutive frames, a mid-band amplitude envelope accumulated for the
// warble measure.
func (fp *KickFingerprint) computeFlux(samples []float64, sr int) {
	win := secToSamples(kickBandWindowSec, sr)
	hop := secToSamples(kickBandHopSec, sr)
	if win < 8 {
		return
	}

	var prev []float64
	var fluxes, fluxTimes []float64 // per-transition flux + the current frame's start time
	var ampEnv, ampTimes []float64  // mid-band amplitude per frame + start time

	for start := 0; start+win <= len(samples); start += hop {
		seg := wave.Wave{Samples: samples[start : start+win], SampleRate: sr}
		mag, binHz := wave.MagnitudeSpectrum(seg, kickBandFFT, wave.WindowHann)
		tsec := float64(start) / float64(sr)

		ampEnv = append(ampEnv, math.Sqrt(bandEnergy(mag, binHz, kickWarbleMidLoHz, kickWarbleMidHiHz)))
		ampTimes = append(ampTimes, tsec)

		if prev != nil {
			var num, tot float64
			n := len(mag)
			if len(prev) < n {
				n = len(prev)
			}
			for k := 1; k < n; k++ { // skip DC
				if d := mag[k] - prev[k]; d > 0 { // half-wave rectified
					num += d * d
				}
				tot += mag[k]
			}
			f := 0.0
			if tot > 1e-20 {
				f = math.Sqrt(num) / tot // level-invariant
			}
			fluxes = append(fluxes, f)
			fluxTimes = append(fluxTimes, tsec)
		}
		prev = mag
	}

	// SpectralFluxAvg over the post-attack transitions only.
	var fsum float64
	var fn int
	for i, f := range fluxes {
		if fluxTimes[i] < kickFluxAttackSkipSec {
			continue
		}
		fsum += f
		fn++
	}
	if fn > 0 {
		fp.SpectralFluxAvg = fsum / float64(fn)
	}

	fp.WarbleDepth = warbleDepth(ampEnv, ampTimes, kickFluxAttackSkipSec)
}

// warbleDepth returns the log-domain coefficient of variation of the mid-band
// amplitude envelope over the post-attack region, after removing its linear
// least-squares trend (which removes an exponential decay exactly, since a decay
// is linear in log-amplitude). The residual std captures the 2–30 Hz oscillatory
// modulation (tremolo/creak) while being blind to the natural decay. Requires a
// handful of positive-amplitude frames; returns 0 otherwise.
func warbleDepth(env, times []float64, skipSec float64) float64 {
	var logs, ts []float64
	for i, e := range env {
		if times[i] < skipSec || e <= 1e-12 {
			continue
		}
		logs = append(logs, math.Log(e))
		ts = append(ts, times[i])
	}
	if len(logs) < 4 {
		return 0
	}
	n := float64(len(logs))
	var st, sy, stt, sty float64
	for i := range logs {
		st += ts[i]
		sy += logs[i]
		stt += ts[i] * ts[i]
		sty += ts[i] * logs[i]
	}
	// Linear least-squares fit  log(e) = m·t + c.
	m, c := 0.0, sy/n
	if denom := n*stt - st*st; math.Abs(denom) > 1e-20 {
		m = (n*sty - st*sy) / denom
		c = (sy - m*st) / n
	}
	var ss float64
	for i := range logs {
		r := logs[i] - (m*ts[i] + c)
		ss += r * r
	}
	return math.Sqrt(ss / n)
}
