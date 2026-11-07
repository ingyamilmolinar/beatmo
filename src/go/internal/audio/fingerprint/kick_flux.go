package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// An averaged spectrum hides movement: a sweep, a tremolo or a creak. Flux is the level-invariant, half-wave-rectified
// frame-to-frame change over the sustain (the onset is excluded); warble is the std of the detrended log mid-band
// envelope, which removes an exponential decay exactly and brackets 2–30 Hz below the frame Nyquist.

const (
	// The pitched body, above the sub rumble and below the click.
	kickWarbleMidLoHz = 120.0
	kickWarbleMidHiHz = 600.0

	// The onset frame's flux is huge by construction and the click broadband, so both measures start here, the segment
	// analyser's attack boundary.
	kickFluxAttackSkipSec = segAttackEndMs / 1000 // 35 ms

	// Bring the small raw ranges to the order of the other KickDistance terms.
	kickFluxDistScale   = 5.0
	kickWarbleDistScale = 2.0
)

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

// The linear log trend removes an exponential decay exactly, leaving the 2–30 Hz oscillation. 0 without enough
// positive frames.
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
