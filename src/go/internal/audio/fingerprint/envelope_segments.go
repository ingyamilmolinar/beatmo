package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// envelope_segments.go — GENERAL-PURPOSE ADSR-style envelope segmentation.
//
// Motivation: a listener explicitly asked for this ("split it up into
// multiple sections and also get duration of attack, release, peak, etc").
// KickSegmentAnalyze (kick_segments.go) does something similar but is kick-
// specific: it uses FIXED perceptual time boundaries (attack ends at a fixed
// 35ms, body at a fixed 150ms) because a kick's coarse structure is roughly
// time-stationary across instruments. A harp, a hihat, an oboe and a bassoon
// have wildly different envelope shapes, so this analyzer instead SEGMENTS
// the wave from its own measured envelope: onset, peak, decay-to-plateau,
// sustain (with a micro-variation measure -- a dead-flat sustain is the
// "sounds like an organ" tell), release, and a −60dB ring-out time (RingT60).
//
// Every threshold below is a named, documented constant rather than a magic
// number, and every degenerate case (silence, a clip that never decays, a
// percussive hit with no sustain) returns an honest flag (Silent,
// NoSustainPlateau, RingT60Extrapolated) rather than a misleading number.
const (
	// envSegHopSec is the RMS envelope block size: fine enough to resolve a
	// fast percussive attack, coarse enough to suppress intra-cycle RMS
	// ripple for fundamentals down to ~50 Hz (a 10ms block spans 5 cycles at
	// 500Hz and still resolves attacks an order of magnitude faster than a
	// typical struck/plucked instrument's).
	envSegHopSec = 0.010

	// envAttackOnsetFrac (~ -40dB of peak) marks "sound has audibly begun":
	// low enough to catch a slow bowed/blown onset, high enough to reject
	// noise-floor jitter before the real attack.
	envAttackOnsetFrac = 0.01

	// envNoiseFloorFrac (~ -60dB of peak) is the floor below which the
	// envelope is treated as inaudible -- used for silence detection and for
	// TotalAudibleSec.
	envNoiseFloorFrac = 0.001

	// envRingT60DropFrac = 10^(-60/20): the classic T60 definition (60dB
	// decay from peak), matching kickDecayDropFrac's convention exactly.
	envRingT60DropFrac = 0.001

	// envSlopeWindowSec is the local window used to test whether the
	// post-peak envelope has "flattened" into a sustain plateau: long enough
	// to average over a few envelope hops (avoiding false flatness from a
	// single noisy sample), short enough to localize the plateau onset to
	// within ~60ms.
	envSlopeWindowSec = 0.06

	// envSustainFlatDBPerSec: a post-peak local window whose |slope| falls
	// below this magnitude is judged to have entered a sustain plateau. A
	// genuinely decaying tail (even a slow one, e.g. -20dB/s) is far steeper
	// than this; an actual sustained plateau hovers near 0dB/s modulo
	// micro-variation.
	envSustainFlatDBPerSec = 6.0

	// envReleaseFloorFrac: release is judged to have begun at the first
	// envelope frame that falls below this fraction of the sustain
	// REFERENCE level (median of the first ~100ms of the detected plateau)
	// AND never recovers above that threshold again for the rest of the
	// clip (a "permanent" test). Close to 1.0 so release onset is caught
	// promptly rather than only once the signal has fallen deep into the
	// release ramp; the "never recovers" guard is what keeps a legitimate
	// sustain micro-variation (tremolo/beating dip that later recovers)
	// from being mistaken for release onset, so the threshold itself does
	// not need to be conservative.
	envReleaseFloorFrac = 0.90

	// envSustainRefWinSec is the window (from sustainStart) used to estimate
	// the sustain reference level for release detection. Frozen to this
	// initial window -- NOT expanded as more of the clip is scanned -- so
	// the release threshold cannot "chase" a declining signal down through
	// the release ramp itself (an expanding-window median would keep
	// lowering the threshold as release samples entered the window,
	// systematically delaying detection until near the very end of the clip).
	envSustainRefWinSec = 0.10
)

// EnvelopeSegments holds the general-purpose ADSR segmentation of one wave.
type EnvelopeSegments struct {
	SampleRate int
	HopSec     float64

	// Silent is true when the wave carries no audible signal (peak envelope
	// below envNoiseFloorFrac, or the wave is empty). Every other field is
	// zero-valued in that case -- never a misleading number.
	Silent bool

	// AttackSec is onset (first crossing of envAttackOnsetFrac*peak) -> the
	// end of the INITIAL RISE. Deliberately NOT onset -> global peak: a
	// sustained tone carrying tremolo, a bow swell, or two beating partials
	// has its global maximum in the middle of the note, and measuring to it
	// reports the swell as the attack. The shipped cello and violin read
	// 1.62 s and 1.70 s that way, against references at 0.09-0.18 s.
	AttackSec float64
	// AttackPeakSec is when that initial rise tops out. Equal to PeakSec for a
	// percussive hit; earlier for anything that swells afterwards.
	AttackPeakSec float64
	// PeakSec is the time of the GLOBAL envelope maximum. Kept as-is: a late
	// global peak is a real property of the signal and several downstream
	// measurements (decay, ring T60, release) are defined from it.
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

	// DecayT10/T20/T30 are the SHALLOW decay thresholds: seconds from the peak
	// to fall 10, 20 and 30 dB below it.
	//
	// They exist because T60 is unusable on this corpus. Every reference is
	// truncated by its own fetch config (convert.max_sec, typically 2-3 s), so
	// anything that rings ends the clip long before -60 dB and both sides
	// report a clamped lower bound pinned to the clip length -- which compares
	// as "about the same" no matter how different the decays actually are.
	// Validated against the listener verdicts, ring T60 scored AUC 0.21
	// (95% CI 0.00-0.47): confidently ANTI-correlated with perceived quality.
	//
	// The shallow thresholds are reached well inside the clip, so they measure
	// instead of clamping, and they describe the portion of the decay that is
	// actually audible. On the two instruments the listener rejected for their
	// decay, T60 saw nothing and these saw the whole problem:
	//
	//	harp           T20 ref 0.48 vs synth 1.56  (3.3x too long)
	//	tubular-bells  T10 ref 1.28 vs synth 0.35  (3.7x too fast)
	DecayT10Sec float64
	DecayT20Sec float64
	DecayT30Sec float64
	// *Extrapolated marks a threshold the clip ended before reaching, so the
	// corresponding value is a LOWER BOUND. Same discipline as
	// RingT60Extrapolated: a bound must never be mistaken for a measurement.
	DecayT10Extrapolated bool
	DecayT20Extrapolated bool
	DecayT30Extrapolated bool

	// EnergySections is the share of total energy in each equal slice of the
	// audible span -- the "split it up into multiple sections" view of a note's
	// shape. Sums to 1; empty when Silent.
	//
	// It answers a question no single duration can: WHERE the energy sits. A
	// real harp puts 96% of its energy in the first quarter and fades; the
	// synth put 72% there and spread the rest. Two notes can share a T20 and
	// still have completely different shapes, and this is what separates them.
	EnergySections []float64

	// OverallDecayRateDBPerSec is the OLS slope of 20*log10(env) from PEAK
	// to the LAST SAMPLE OF THE CLIP -- unlike DecayRateDBPerSec (peak ->
	// sustain-plateau ONSET only), this always spans the full post-peak
	// signal regardless of any plateau found in between. It exists
	// specifically so a RingT60 comparison still has a real, unclamped
	// number to fall back on when BOTH sides' RingT60Sec are extrapolated
	// lower bounds (both still above -60dB at clip end): a bell/tube's
	// fast initial strike-transient decay can trip the plateau detector
	// (its slope locally flattens as the decay eases into a long, slowly-
	// decaying "hum" tail), leaving DecayRateDBPerSec describing only the
	// first ~100ms rather than the audible ring. 0 when peak is the last
	// sample (nothing to measure).
	OverallDecayRateDBPerSec float64

	TotalAudibleSec float64 // span from the first to the last frame at or above envNoiseFloorFrac
}

// AnalyzeEnvelopeSegments segments w's amplitude envelope into onset / peak /
// decay / sustain / release / ring-out. w need not be pre-normalized -- this
// function peak-normalizes internally (mirroring KickSegmentAnalyze).
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

	// ---- Attack: onset -> end of the INITIAL RISE --------------------------
	//
	// The onset search still scans to the global peak (the first crossing of
	// the onset threshold is the same frame either way), but the attack ENDS
	// at the first local maximum after it, not at the global one.
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

// attackRiseDipFrac is how far the envelope must fall below a candidate local
// maximum before that maximum counts as the end of the initial rise.
//
// Set well above block-RMS ripple so ordinary jitter on the way up does not
// terminate the attack early, and well below the depth of a real tremolo or
// bow swell so a genuine dip does. A 12% drop is far more than the envelope
// wobbles while rising and far less than the ~70% dip the shipped cello shows
// between its onset peak and its mid-note trough.
const attackRiseDipFrac = 0.12

// attackRiseMinSec is the minimum time the envelope must spend below a
// candidate maximum for the dip to be real rather than a momentary notch.
const attackRiseMinSec = 0.03

// initialRisePeak returns the index where the onset rise tops out: the first
// local maximum between onset and the global peak that is followed by a
// sustained dip of at least attackRiseDipFrac — or, once the rise is already
// within attackRiseDipFrac of the global peak, by a sustained fall of any
// depth: from there on the envelope is a plateau whose ripple (a bow,
// vibrato) is not more rising. Without that second rule a plateau that never
// dips 12 % ended its "attack" at its loudest ripple crest, anywhere in the
// note (TestAttackSec_EndsWhereARipplingPlateauBegins).
//
// When neither exists -- a percussive hit, or any signal that simply rises
// to its loudest point and decays -- this returns globalPeak unchanged, so the
// measurement is identical to the previous behaviour for every signal the
// previous behaviour handled correctly.
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
				// The envelope dipped meaningfully and stayed down: the rise
				// that preceded it was the attack.
				return maxIdx
			}
			continue
		}
		below = 0
	}
	return globalPeak
}

// energySectionCount is how many equal slices the audible span is split into.
//
// Four is a deliberate compromise: enough to tell a front-loaded pluck from an
// even sustain from a swell, few enough that each slice still holds a
// statistically meaningful amount of energy on a 2 s clip. Finer slicing buys
// resolution the corpus's clip lengths cannot support.
const energySectionCount = 4

// decayThreshold returns the seconds from the peak until the envelope falls
// dbDown below it, and whether the clip ended first.
//
// Uses a LAST-crossing rule, matching RingT60Sec: a signal that dips below the
// threshold and comes back up (tremolo, a beating bell) has not finished
// decaying at the first dip, and reporting that first dip would understate the
// ring badly on exactly the instruments this measurement exists for.
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

// energySections splits the audible span into energySectionCount equal slices
// and returns each slice's share of the total energy.
//
// Energy (squared envelope) rather than amplitude, so the shares are additive
// and a slice's number means "this fraction of the note's power lives here".
// Measured over the AUDIBLE span rather than the whole buffer so leading or
// trailing silence in a reference clip cannot shift the profile.
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
