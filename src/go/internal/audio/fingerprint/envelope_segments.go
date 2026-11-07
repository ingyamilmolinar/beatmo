package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// General ADSR-style segmentation from the wave's OWN measured envelope (a harp, a hi-hat and an oboe share no fixed
// boundaries, unlike a kick). Degenerate cases (silence, no decay, no sustain) return honest flags rather than
// misleading numbers.
const (
	// Fine enough for a fast percussive attack, coarse enough to smooth intra-cycle ripple down to ~50 Hz.
	envSegHopSec = 0.010

	// About -40 dB: catches a slow bowed onset, rejects noise-floor jitter.
	envAttackOnsetFrac = 0.01

	// About -60 dB: below it the envelope is inaudible (silence, TotalAudibleSec).
	envNoiseFloorFrac = 0.001

	// 60 dB below the peak, matching kickDecayDropFrac.
	envRingT60DropFrac = 0.001

	// Long enough to average a few hops (no false flatness from one noisy sample), short enough to place the plateau
	// within ~60 ms.
	envSlopeWindowSec = 0.06

	// Even a slow real decay is far steeper; a plateau hovers near 0 dB/s.
	envSustainFlatDBPerSec = 6.0

	// Release starts at the first frame below this share of the sustain reference that NEVER recovers above it: the
	// "never recovers" guard keeps a tremolo dip from reading as release, so the threshold need not be conservative.
	envReleaseFloorFrac = 0.90

	// Frozen to this first window, so the threshold cannot chase a falling signal down the release ramp.
	envSustainRefWinSec = 0.10
)

// EnvelopeSegments holds the general-purpose ADSR segmentation of one wave.
type EnvelopeSegments struct {
	SampleRate int
	HopSec     float64

	// Every other field is zero then, never a misleading number.
	Silent bool

	// Onset → the end of the INITIAL RISE, never → the global peak: tremolo, a bow swell or beating put the maximum
	// mid-note.
	AttackSec float64
	// Equal to PeakSec for a hit; earlier for anything that swells.
	AttackPeakSec float64
	// Decay, ring T60 and release are defined from it.
	PeakSec   float64
	PeakLevel float64 // envelope RMS value at the peak (<=1.0; less than the true sample peak since it is a block RMS, not a sample max)

	DecaySec          float64 // peak -> sustain plateau onset (or, absent a plateau, peak -> last-audible frame)
	DecayRateDBPerSec float64 // OLS slope of 20*log10(env) over the decay region; negative = decaying
	NoSustainPlateau  bool    // true when the post-peak envelope never flattens into a plateau before falling to the noise floor (a purely percussive hit)

	SustainSec       float64 // duration of the detected plateau, 0 when NoSustainPlateau
	SustainLevel     float64 // median envelope level in the sustain region, as a fraction of peak
	SustainVariation float64 // std/mean of the sustain-region envelope: amplitude micro-variation ("liveliness"); ~0 for a dead-flat ("organ") sustain

	ReleaseSec float64 // sustain-region end -> last audible frame (0 when no release was detected, e.g. the clip simply stops mid-sustain)

	RingT60Sec          float64 // time from peak to fall 60dB below peak (ring-out / last-crossing, same method as kick_decay.go's bandT60)
	RingT60Extrapolated bool    // true when the clip ends before reaching -60dB: RingT60Sec is then a LOWER BOUND, not a measured value

	// SHALLOW thresholds: seconds from the peak to fall 10, 20 and 30 dB. Every reference is truncated, so a ringing
	// note ends before -60 dB and T60 clamps to the clip length on both sides; these are reached inside the clip and
	// measure the audible decay.
	DecayT10Sec float64
	DecayT20Sec float64
	DecayT30Sec float64
	// The clip ended first, so the value is a LOWER BOUND, never to be mistaken for a measurement.
	DecayT10Extrapolated bool
	DecayT20Extrapolated bool
	DecayT30Extrapolated bool

	// Each equal slice of the audible span's energy share, summing to 1 (empty when Silent): WHERE the energy sits,
	// which two notes sharing a T20 can still differ on.
	EnergySections []float64

	// The OLS slope from the peak to the LAST sample, whatever plateau lies between: when both ring T60s are extrapolated
	// it is the unclamped fallback, since a bell's strike transient can trip the plateau detector and leave
	// DecayRateDBPerSec describing only its first ~100 ms. 0 when the peak is the last sample.
	OverallDecayRateDBPerSec float64

	TotalAudibleSec float64 // span from the first to the last frame at or above envNoiseFloorFrac
}

// w need not be normalised; it is peak-normalised here.
func AnalyzeEnvelopeSegments(w wave.Wave) EnvelopeSegments {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out := EnvelopeSegments{SampleRate: sr}
	if w.PeakSample() < 1e-9 {
		out.Silent = true
		return out
	}
	w = PeakNormalize(w)
	sr = w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out.SampleRate = sr

	hop := secToSamples(envSegHopSec, sr)
	if hop < 1 {
		hop = 1
	}
	hopSec := float64(hop) / float64(sr)
	out.HopSec = hopSec

	env := RMSEnvelope(w, hop, 0)
	n := len(env)
	if n == 0 {
		out.Silent = true
		return out
	}

	peakIdx, peakVal := argmax(env)
	if peakVal < envNoiseFloorFrac {
		out.Silent = true
		return out
	}
	out.PeakLevel = peakVal
	out.PeakSec = float64(peakIdx) * hopSec

	// The onset crossing is the same either way; the attack ENDS at the first qualifying local maximum, not the global one.
	onsetThresh := envAttackOnsetFrac * peakVal
	onsetIdx := 0
	for i := 0; i <= peakIdx; i++ {
		if env[i] >= onsetThresh {
			onsetIdx = i
			break
		}
	}
	attackPeakIdx := initialRisePeak(env, onsetIdx, peakIdx, hopSec)
	out.AttackSec = float64(attackPeakIdx-onsetIdx) * hopSec
	out.AttackPeakSec = float64(attackPeakIdx) * hopSec

	// ---- Total audible span -------------------------------------------------
	floorAbs := envNoiseFloorFrac * peakVal
	firstAudible, lastAudible := -1, -1
	for i, v := range env {
		if v >= floorAbs {
			if firstAudible < 0 {
				firstAudible = i
			}
			lastAudible = i
		}
	}
	if firstAudible >= 0 {
		out.TotalAudibleSec = float64(lastAudible-firstAudible+1) * hopSec
	}

	// ---- Decay: peak -> sustain plateau onset (slope flattens) -----------
	slopeWin := int(math.Round(envSlopeWindowSec / hopSec))
	if slopeWin < 2 {
		slopeWin = 2
	}
	sustainStart := -1
	for i := peakIdx; i+slopeWin <= n; i++ {
		xs := make([]float64, 0, slopeWin)
		ys := make([]float64, 0, slopeWin)
		ok := true
		for j := 0; j < slopeWin; j++ {
			v := env[i+j]
			if v < floorAbs {
				ok = false
				break
			}
			xs = append(xs, float64(j)*hopSec)
			ys = append(ys, 20*math.Log10(v))
		}
		if !ok {
			continue
		}
		if math.Abs(olsSlope(xs, ys)) < envSustainFlatDBPerSec {
			sustainStart = i
			break
		}
	}

	var decayEndIdx int
	if sustainStart >= 0 {
		decayEndIdx = sustainStart
	} else {
		out.NoSustainPlateau = true
		if lastAudible > peakIdx {
			decayEndIdx = lastAudible
		} else {
			decayEndIdx = peakIdx
		}
	}
	out.DecaySec = float64(decayEndIdx-peakIdx) * hopSec
	if decayEndIdx > peakIdx {
		var xs, ys []float64
		for i := peakIdx; i <= decayEndIdx; i++ {
			v := env[i]
			if v < 1e-12 {
				continue
			}
			xs = append(xs, float64(i-peakIdx)*hopSec)
			ys = append(ys, 20*math.Log10(v))
		}
		out.DecayRateDBPerSec = olsSlope(xs, ys)
	}

	// ---- Sustain + release: only meaningful when a plateau was found -----
	if sustainStart >= 0 {
		refWin := int(math.Round(envSustainRefWinSec / hopSec))
		if refWin < 1 {
			refWin = 1
		}
		refEnd := sustainStart + refWin
		if refEnd > n {
			refEnd = n
		}
		referenceLevel := median(env[sustainStart:refEnd])
		releaseThresh := envReleaseFloorFrac * referenceLevel

		releaseStart := n // default: sustain runs to the end of the clip (no release detected)
		for i := sustainStart; i < n; i++ {
			if env[i] < releaseThresh {
				recovers := false
				for k := i; k < n; k++ {
					if env[k] >= releaseThresh {
						recovers = true
						break
					}
				}
				if !recovers {
					releaseStart = i
					break
				}
			}
		}
		out.SustainSec = float64(releaseStart-sustainStart) * hopSec
		sustainRegion := env[sustainStart:releaseStart]
		if len(sustainRegion) > 0 {
			out.SustainLevel = median(sustainRegion) / peakVal
			mean, std := meanStd(sustainRegion)
			if mean > 1e-12 {
				out.SustainVariation = std / mean
			}
		}
		if releaseStart < n {
			out.ReleaseSec = float64(n-1-releaseStart) * hopSec
		}
	}

	// ---- Overall decay rate: peak -> end of clip, ignoring any plateau ---
	if n-1 > peakIdx {
		var xs, ys []float64
		for i := peakIdx; i < n; i++ {
			v := env[i]
			if v < 1e-12 {
				continue
			}
			xs = append(xs, float64(i-peakIdx)*hopSec)
			ys = append(ys, 20*math.Log10(v))
		}
		out.OverallDecayRateDBPerSec = olsSlope(xs, ys)
	}

	// ---- RingT60: ring-out (last-crossing) time from peak to -60dB -------
	ringThresh := peakVal * envRingT60DropFrac
	lastAbove := peakIdx
	for i := peakIdx; i < n; i++ {
		if env[i] > ringThresh {
			lastAbove = i
		}
	}
	if lastAbove >= n-1 {
		out.RingT60Sec = float64(n-1-peakIdx) * hopSec
		out.RingT60Extrapolated = true
	} else {
		out.RingT60Sec = float64(lastAbove-peakIdx) * hopSec
	}

	// ---- Shallow decay thresholds + energy sections -------------------------
	out.DecayT10Sec, out.DecayT10Extrapolated = decayThreshold(env, peakIdx, peakVal, 10, hopSec)
	out.DecayT20Sec, out.DecayT20Extrapolated = decayThreshold(env, peakIdx, peakVal, 20, hopSec)
	out.DecayT30Sec, out.DecayT30Extrapolated = decayThreshold(env, peakIdx, peakVal, 30, hopSec)
	out.EnergySections = energySections(env, firstAudible, lastAudible)

	return out
}

// Well above block-RMS ripple on the way up, well below a real tremolo or bow-swell dip.
const attackRiseDipFrac = 0.12

// So a momentary notch is not a dip.
const attackRiseMinSec = 0.03

// The first local maximum followed by a sustained dip of attackRiseDipFrac or, once within that of the global peak,
// by a sustained fall of any depth: a plateau's ripple is not more rising. With neither (a hit), the global peak.
func initialRisePeak(env []float64, onsetIdx, globalPeak int, hopSec float64) int {
	if globalPeak <= onsetIdx || globalPeak >= len(env) {
		return globalPeak
	}
	minDipFrames := int(math.Round(attackRiseMinSec / math.Max(hopSec, 1e-9)))
	if minDipFrames < 1 {
		minDipFrames = 1
	}

	nearTop := env[globalPeak] * (1 - attackRiseDipFrac)
	runningMax := env[onsetIdx]
	maxIdx := onsetIdx
	below, under := 0, 0
	for i := onsetIdx + 1; i <= globalPeak; i++ {
		if env[i] >= runningMax {
			runningMax = env[i]
			maxIdx = i
			below, under = 0, 0
			continue
		}
		under++
		if runningMax >= nearTop && under >= minDipFrames {
			return maxIdx // a plateau's ripple, not more rising
		}
		if env[i] < runningMax*(1-attackRiseDipFrac) {
			below++
			if below >= minDipFrames {
				return maxIdx
			}
			continue
		}
		below = 0
	}
	return globalPeak
}

// Enough to tell a front-loaded pluck from an even sustain from a swell, few enough that each slice holds real energy
// on a 2 s clip.
const energySectionCount = 4

// LAST crossing, like RingT60Sec: tremolo or a beating bell dips below the threshold before it has finished
// decaying.
func decayThreshold(env []float64, peakIdx int, peakVal, dbDown, hopSec float64) (sec float64, extrapolated bool) {
	if peakIdx >= len(env)-1 || peakVal <= 0 {
		return 0, true
	}
	thresh := peakVal * math.Pow(10, -dbDown/20)
	lastAbove := peakIdx
	for i := peakIdx; i < len(env); i++ {
		if env[i] > thresh {
			lastAbove = i
		}
	}
	if lastAbove >= len(env)-1 {
		// Still above the threshold at the last frame: the clip ran out first.
		return float64(len(env)-1-peakIdx) * hopSec, true
	}
	return float64(lastAbove-peakIdx) * hopSec, false
}

// Energy, so shares add up; over the AUDIBLE span, so leading or trailing silence cannot shift the profile.
func energySections(env []float64, firstAudible, lastAudible int) []float64 {
	if firstAudible < 0 || lastAudible < firstAudible {
		return nil
	}
	span := lastAudible - firstAudible + 1
	if span < energySectionCount {
		return nil
	}
	out := make([]float64, energySectionCount)
	var total float64
	for i := firstAudible; i <= lastAudible; i++ {
		e := env[i] * env[i]
		k := (i - firstAudible) * energySectionCount / span
		if k >= energySectionCount {
			k = energySectionCount - 1
		}
		out[k] += e
		total += e
	}
	if total <= 0 {
		return nil
	}
	for i := range out {
		out[i] /= total
	}
	return out
}
