package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// --- synthetic signal generators with known ground truth ----------------------

// expDecaySine renders an exponentially-decaying pure sine at f0 Hz.
// tau is the decay time constant (seconds): amplitude ∝ e^{-t/tau}.
func expDecaySine(sr int, dur, f0, tau float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = math.Exp(-t/tau) * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// beatingPair renders the sum of two detuned decaying sines (f1 & f2). The
// second is delayed by delaySec (onset offset). The amplitude of the sum beats
// at |f1-f2| Hz, producing a secondary envelope "bloom" one beat-period after
// the attack. Both partials share the same decay time constant tau.
func beatingPair(sr int, dur, f1, f2, a1, a2, tau, delaySec float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		v := a1 * math.Exp(-t/tau) * math.Sin(2*math.Pi*f1*t)
		if t >= delaySec {
			td := t - delaySec
			v += a2 * math.Exp(-td/tau) * math.Sin(2*math.Pi*f2*td)
		}
		s[i] = v
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// chirpThenSteady renders a linear pitch glide from fStart to fEnd over
// glideSec, then a steady tone at fEnd for steadySec. Phase is integrated so
// the waveform is continuous.
func chirpThenSteady(sr int, glideSec, steadySec, fStart, fEnd float64) wave.Wave {
	n := int((glideSec + steadySec) * float64(sr))
	s := make([]float64, n)
	phase := 0.0
	twoPiSR := 2 * math.Pi / float64(sr)
	for i := range s {
		t := float64(i) / float64(sr)
		var f float64
		if t < glideSec {
			f = fStart + (fEnd-fStart)*(t/glideSec)
		} else {
			f = fEnd
		}
		phase += twoPiSR * f
		s[i] = math.Sin(phase)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// toneWithHFBurst renders a clean low sine with a short burst of deterministic
// high-frequency content injected in the middle (a "spit"/sputter artifact).
func toneWithHFBurst(sr int, dur, f0, burstStart, burstDur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.6 * math.Sin(2*math.Pi*f0*t)
		if t >= burstStart && t < burstStart+burstDur {
			// Deterministic HF content (5 kHz + 7 kHz).
			s[i] += 0.6 * (math.Sin(2*math.Pi*5000*t) + math.Sin(2*math.Pi*7000*t))
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// plateauThenDrop renders a signal whose amplitude envelope holds a plateau
// (constant) for holdSec then drops off exponentially — the anti-exponential
// reference for PlateauFlatness.
func plateauThenDrop(sr int, dur, f0, holdSec, tau float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		amp := 1.0
		if t > holdSec {
			amp = math.Exp(-(t - holdSec) / tau)
		}
		s[i] = amp * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// midModeDecay renders a decaying tone whose energy sits mostly in the MID band
// (100–500 Hz) — a stand-in for a tom's prominent inharmonic body mode. A small
// low partial is added so the low band is not exactly zero (like a real drum).
func midModeDecay(sr int, dur, fMid, tau float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		e := math.Exp(-t / tau)
		s[i] = e * (0.9*math.Sin(2*math.Pi*fMid*t) + 0.05*math.Sin(2*math.Pi*55*t))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// reverbTail renders a fast-decaying main hit at f0 (whose own tail is
// negligible) plus a long, low-level added tail (a slow-decaying sine out to
// tailDur seconds) — the reverb reference. tailAmp is the tail level relative to
// the main hit's peak.
func reverbTail(sr int, dur, f0, tailAmp, tailTau, tailDur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		v := math.Exp(-t/0.03) * math.Sin(2*math.Pi*f0*t) // fast main hit
		if t < tailDur {
			v += tailAmp * math.Exp(-t/tailTau) * math.Sin(2*math.Pi*f0*t)
		}
		s[i] = v
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// --- tests --------------------------------------------------------------------

func TestKickAnalyze_ExpDecay(t *testing.T) {
	const sr = 44100
	w := expDecaySine(sr, 0.5, 80, 0.15)
	fp := KickAnalyze(w)

	// Pure exponential decay is NOT a plateau → flatness LOW.
	if fp.PlateauFlatness >= 0.75 {
		t.Errorf("PlateauFlatness = %.3f, want < 0.75 for exponential decay", fp.PlateauFlatness)
	}
	// A plateau signal must score clearly higher (sanity of the discriminator).
	plat := KickAnalyze(plateauThenDrop(sr, 0.5, 80, 0.3, 0.05))
	if plat.PlateauFlatness <= fp.PlateauFlatness+0.1 {
		t.Errorf("plateau flatness %.3f should exceed exp flatness %.3f by a margin",
			plat.PlateauFlatness, fp.PlateauFlatness)
	}
	// No secondary bloom in a monotonic decay.
	if fp.TailBloomPresent {
		t.Errorf("TailBloomPresent = true, want false for monotonic exp decay (sec=%.3f ratio=%.3f)",
			fp.TailBloomSec, fp.TailBloomRatio)
	}
	// Settle pitch ~ 80 Hz.
	if fp.PitchSettleHz < 70 || fp.PitchSettleHz > 95 {
		t.Errorf("PitchSettleHz = %.1f, want ~80", fp.PitchSettleHz)
	}
	// Crest modest.
	if fp.Crest < 2 || fp.Crest > 6 {
		t.Errorf("Crest = %.2f, want modest [2,6]", fp.Crest)
	}
}

func TestKickAnalyze_BeatingTailBloom(t *testing.T) {
	const sr = 44100
	// 80 & 71 Hz → Δf = 9 Hz → beat period ≈ 111 ms.
	w := beatingPair(sr, 0.5, 80, 71, 0.9, 0.8, 0.6, 0.005)
	fp := KickAnalyze(w)

	if !fp.TailBloomPresent {
		t.Fatalf("TailBloomPresent = false, want true for beating pair")
	}
	// Beat period ≈ 111 ms; allow generous tolerance.
	if fp.TailBloomSec < 0.07 || fp.TailBloomSec > 0.16 {
		t.Errorf("TailBloomSec = %.3f s, want ≈0.111 (beat period)", fp.TailBloomSec)
	}
	if fp.TailBloomRatio < 0.15 {
		t.Errorf("TailBloomRatio = %.3f, want >= 0.15", fp.TailBloomRatio)
	}
}

func TestKickAnalyze_PitchGlide(t *testing.T) {
	const sr = 44100
	w := chirpThenSteady(sr, 0.04, 0.36, 200, 80)
	fp := KickAnalyze(w)

	if fp.PitchStartHz < 140 {
		t.Errorf("PitchStartHz = %.1f, want high (~200)", fp.PitchStartHz)
	}
	if fp.PitchSettleHz < 70 || fp.PitchSettleHz > 95 {
		t.Errorf("PitchSettleHz = %.1f, want ~80", fp.PitchSettleHz)
	}
	if fp.PitchStartHz <= fp.PitchSettleHz+40 {
		t.Errorf("expected clear downward glide: start %.1f settle %.1f", fp.PitchStartHz, fp.PitchSettleHz)
	}
	if fp.GlideSec < 0.01 || fp.GlideSec > 0.09 {
		t.Errorf("GlideSec = %.3f s, want ≈0.04", fp.GlideSec)
	}
}

func TestKickAnalyze_HFSmoothness(t *testing.T) {
	const sr = 44100
	spitty := KickAnalyze(toneWithHFBurst(sr, 0.4, 80, 0.18, 0.02))
	clean := KickAnalyze(expDecaySine(sr, 0.4, 80, 5.0)) // near-steady clean tone

	if spitty.HFRatioMaxJump < 0.2 {
		t.Errorf("spitty HFRatioMaxJump = %.3f, want large (>0.2)", spitty.HFRatioMaxJump)
	}
	if clean.HFRatioMaxJump > 0.1 {
		t.Errorf("clean HFRatioMaxJump = %.3f, want small (<0.1)", clean.HFRatioMaxJump)
	}
	if spitty.HFRatioMaxJump <= clean.HFRatioMaxJump {
		t.Errorf("spitty (%.3f) should exceed clean (%.3f)", spitty.HFRatioMaxJump, clean.HFRatioMaxJump)
	}
}

func TestKickDistance_IdentityAndSeparation(t *testing.T) {
	const sr = 44100
	exp := KickAnalyze(expDecaySine(sr, 0.5, 80, 0.15))
	beat := KickAnalyze(beatingPair(sr, 0.5, 80, 71, 0.9, 0.8, 0.6, 0.005))

	if d := KickDistance(exp, exp); d.Total > 1e-9 {
		t.Errorf("KickDistance(x,x).Total = %g, want ~0", d.Total)
	}
	if d := KickDistance(beat, beat); d.Total > 1e-9 {
		t.Errorf("KickDistance(beat,beat).Total = %g, want ~0", d.Total)
	}

	d := KickDistance(exp, beat)
	if d.Total <= 0.1 {
		t.Errorf("KickDistance(exp,beat).Total = %g, want clearly > 0.1", d.Total)
	}
	// Symmetry of the total.
	dr := KickDistance(beat, exp)
	if math.Abs(d.Total-dr.Total) > 1e-9 {
		t.Errorf("distance not symmetric: %g vs %g", d.Total, dr.Total)
	}
	// The trace/landmark terms (organic-complexity capture) should carry the
	// distance — not the scalar Crest term.
	traceLandmark := DefaultKickWeights.EnvShape*d.EnvShape +
		DefaultKickWeights.Landmarks*d.Landmarks
	crestContrib := DefaultKickWeights.Crest * d.Crest
	if traceLandmark <= crestContrib {
		t.Errorf("expected trace/landmark terms (%.3f) to dominate Crest (%.3f)", traceLandmark, crestContrib)
	}
	if d.Landmarks <= 0 {
		t.Errorf("Landmarks distance = %g, want > 0 (tail-bloom presence differs)", d.Landmarks)
	}
}

func TestKickBands_Shape(t *testing.T) {
	bands := KickBands()
	if len(bands) != KickBandCount {
		t.Fatalf("KickBands len = %d, want %d", len(bands), KickBandCount)
	}
	// Band fractions of a single window should sum ~1.
	fp := KickAnalyze(expDecaySine(44100, 0.3, 80, 0.15))
	if len(fp.BandAvg) != KickBandCount {
		t.Fatalf("BandAvg len = %d, want %d", len(fp.BandAvg), KickBandCount)
	}
	sum := 0.0
	for _, v := range fp.BandAvg {
		sum += v
	}
	if sum < 0.9 || sum > 1.1 {
		t.Errorf("BandAvg sum = %.3f, want ~1", sum)
	}
	// An 80 Hz kick's dominant band is 60-120 Hz (index 1).
	dom := 0
	for i, v := range fp.BandAvg {
		if v > fp.BandAvg[dom] {
			dom = i
		}
	}
	if dom != 1 {
		t.Errorf("dominant band = %d (%s), want 1 (60-120 Hz)", dom, bands[dom].Name)
	}
}

func TestKickAnalyze_LowMidKickVsTom(t *testing.T) {
	const sr = 44100
	// Kick: pure 60 Hz decaying sine → fundamental-dominant loud onset.
	kick := KickAnalyze(expDecaySine(sr, 0.4, 60, 0.08))
	// Tom: dominated by a 250 Hz inharmonic mid mode → mid-dominant loud onset.
	tom := KickAnalyze(midModeDecay(sr, 0.4, 250, 0.08))

	if kick.AttackLowMidRatio <= 1 {
		t.Errorf("kick AttackLowMidRatio = %.3f, want > 1 (fundamental-dominant)", kick.AttackLowMidRatio)
	}
	if tom.AttackLowMidRatio >= 1 {
		t.Errorf("tom AttackLowMidRatio = %.3f, want < 1 (mid-dominant)", tom.AttackLowMidRatio)
	}
	if kick.AttackLowMidRatio <= tom.AttackLowMidRatio {
		t.Errorf("kick (%.3f) should exceed tom (%.3f) in AttackLowMidRatio",
			kick.AttackLowMidRatio, tom.AttackLowMidRatio)
	}
	if len(kick.LowMidRatioTrace) == 0 {
		t.Errorf("LowMidRatioTrace empty")
	}
	// The LowMid distance term must clearly separate kick from tom.
	d := KickDistance(kick, tom)
	if d.LowMid <= 0.5 {
		t.Errorf("LowMid distance = %.3f, want clearly > 0.5 for kick-vs-tom", d.LowMid)
	}
}

func TestKickAnalyze_ReverbTail(t *testing.T) {
	const sr = 44100
	// Dry: fast decaying exponential, no added tail.
	dry := KickAnalyze(expDecaySine(sr, 0.5, 60, 0.03))
	// Reverberant: same fast hit + a low-level, slow-decaying tail out to 300 ms.
	wet := KickAnalyze(reverbTail(sr, 0.5, 60, 0.05, 0.8, 0.30))

	if dry.TailRatio > 0.02 {
		t.Errorf("dry TailRatio = %.4f, want ~0 (< 0.02)", dry.TailRatio)
	}
	if wet.TailRatio <= dry.TailRatio {
		t.Errorf("wet TailRatio (%.4f) should exceed dry (%.4f)", wet.TailRatio, dry.TailRatio)
	}
	if wet.TailRatio < 0.01 {
		t.Errorf("wet TailRatio = %.4f, want a present tail (>= 0.01)", wet.TailRatio)
	}
	// Dry tail rings out fast; wet tail persists to ~300 ms.
	if dry.TailDurationSec > 0.2 {
		t.Errorf("dry TailDurationSec = %.3f, want short (< 0.2)", dry.TailDurationSec)
	}
	if wet.TailDurationSec < 0.25 {
		t.Errorf("wet TailDurationSec = %.3f, want ~0.3", wet.TailDurationSec)
	}
	if wet.TailDurationSec <= dry.TailDurationSec {
		t.Errorf("wet duration (%.3f) should exceed dry (%.3f)", wet.TailDurationSec, dry.TailDurationSec)
	}
	// The Reverb distance term must clearly separate dry from reverberant.
	d := KickDistance(dry, wet)
	if d.Reverb <= 0 {
		t.Errorf("Reverb distance = %.4f, want clearly > 0 for dry-vs-reverberant", d.Reverb)
	}
}

func TestKickDistance_BodyReverbIdentity(t *testing.T) {
	const sr = 44100
	wet := KickAnalyze(reverbTail(sr, 0.5, 60, 0.03, 0.12, 0.30))
	tom := KickAnalyze(midModeDecay(sr, 0.4, 250, 0.08))

	if d := KickDistance(wet, wet); d.LowMid != 0 || d.Reverb != 0 || d.Total > 1e-9 {
		t.Errorf("KickDistance(x,x): LowMid=%g Reverb=%g Total=%g, want all ~0", d.LowMid, d.Reverb, d.Total)
	}
	// Symmetry of the new terms.
	d1 := KickDistance(wet, tom)
	d2 := KickDistance(tom, wet)
	if math.Abs(d1.LowMid-d2.LowMid) > 1e-9 || math.Abs(d1.Reverb-d2.Reverb) > 1e-9 {
		t.Errorf("new terms not symmetric: LowMid %g/%g Reverb %g/%g",
			d1.LowMid, d2.LowMid, d1.Reverb, d2.Reverb)
	}
}
