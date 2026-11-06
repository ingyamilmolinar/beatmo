package fingerprint

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func ratioTone(sr int, dur, f0 float64, ratios []float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		for _, r := range ratios {
			s[i] += (0.3 / float64(len(ratios))) * math.Sin(2*math.Pi*f0*r*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestModeHarmonicity_BarVsString(t *testing.T) {
	bar := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2.76, 5.40}), "bar")
	str := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2, 3, 4, 5}), "string")
	hb, ok := ModeHarmonicity(bar.Modes())
	if !ok || hb > 0.4 {
		t.Fatalf("bar harmonicity want < 0.4, got %.2f ok=%v", hb, ok)
	}
	hs, ok := ModeHarmonicity(str.Modes())
	if !ok || hs < 0.9 {
		t.Fatalf("string harmonicity want > 0.9, got %.2f", hs)
	}
}

func TestModeRatioDistanceCents(t *testing.T) {
	if d := ModeRatioDistanceCents([]float64{1, 2, 3}, []float64{1, 2, 3}); d != 0 {
		t.Fatalf("identical want 0, got %.2f", d)
	}
	// 2.76 vs 3.00 is 144 cents; symmetric mean over 3 modes.
	d := ModeRatioDistanceCents([]float64{1, 2.76, 5.40}, []float64{1, 3, 5.40})
	if d < 40 || d > 60 {
		t.Fatalf("want ~48 cents (144/3), got %.1f", d)
	}
}

func TestModeBeating_TwoCloseSines(t *testing.T) {
	sr := 44100
	n := int(2.0 * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tt := float64(i) / float64(sr)
		s[i] = 0.3*math.Sin(2*math.Pi*880*tt) + 0.3*math.Sin(2*math.Pi*884*tt)
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "beat")
	rate, depth, ok := ModeBeating(b.Modes())
	if !ok {
		t.Fatal("beating pair not found")
	}
	if math.Abs(rate-4) > 1.0 {
		t.Fatalf("beating rate want 4 Hz, got %.2f", rate)
	}
	if depth < 0.3 {
		t.Fatalf("equal-amplitude pair should beat deeply, got depth %.2f", depth)
	}
}

// TestModeSelfBeat_RippleEstimate is the dedicated unit test for the
// per-mode ripple estimator, exercised directly on a synthetic dB Track
// (bypassing the FFT/tracker entirely) so the rate/depth math is pinned in
// isolation. amp(t) = 1 + m*sin(2*pi*f*t) has depth exactly m (its max is
// 1+m, its min is 1-m, and (max-min)/(max+min) = 2m/2 = m) and ripples at
// exactly f Hz.
func TestModeSelfBeat_RippleEstimate(t *testing.T) {
	const hopSec = 0.01
	const f = 3.0
	const m = 0.3
	n := 300 // 3 seconds at hopSec
	track := make([]float64, n)
	for i := range track {
		tt := float64(i) * hopSec
		amp := 1 + m*math.Sin(2*math.Pi*f*tt)
		track[i] = 20 * math.Log10(amp)
	}
	rate, depth, ok := modeSelfBeat(Mode{Track: track}, hopSec)
	if !ok {
		t.Fatal("expected a ripple to be detected")
	}
	if math.Abs(rate-f) > 0.3 {
		t.Fatalf("rate want ~%.1f, got %.3f", f, rate)
	}
	if math.Abs(depth-m) > 0.05 {
		t.Fatalf("depth want ~%.2f, got %.3f", m, depth)
	}
}

// TestModeSelfBeat_NoRippleWhenSteady is the negative case for the same
// estimator: a perfectly constant track has no ripple to report.
func TestModeSelfBeat_NoRippleWhenSteady(t *testing.T) {
	const hopSec = 0.01
	track := make([]float64, 300)
	for i := range track {
		track[i] = -6 // constant level, no modulation
	}
	if _, _, ok := modeSelfBeat(Mode{Track: track}, hopSec); ok {
		t.Fatal("a steady track must not report a ripple")
	}
}

// TestModeBeating_RejectsGlobalTremolo is the critical tremolo-conflation
// regression: a vibraphone-like tone with three STEADY partials (1, 4.0,
// 10.7 x f0) under a single GLOBAL 5 Hz amplitude tremolo modulates every
// partial identically, so every strong mode's own dB track ripples at the
// same rate. That is common-mode amplitude modulation of the whole
// instrument, not two partials beating against each other, and must be
// rejected.
func TestModeBeating_RejectsGlobalTremolo(t *testing.T) {
	sr := 44100
	dur := 2.0
	n := int(dur * float64(sr))
	s := make([]float64, n)
	f0 := 300.0
	ratios := []float64{1, 4.0, 10.7}
	for i := range s {
		tt := float64(i) / float64(sr)
		trem := 1 + 0.35*math.Sin(2*math.Pi*5*tt)
		v := 0.0
		for _, r := range ratios {
			v += (0.3 / float64(len(ratios))) * math.Sin(2*math.Pi*f0*r*tt)
		}
		s[i] = trem * v
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "vibraphone-tremolo")
	rate, depth, ok := ModeBeating(b.Modes())
	if ok && depth >= 0.05 {
		t.Fatalf("global tremolo must be rejected as common-mode, got rate=%.2f depth=%.2f ok=%v", rate, depth, ok)
	}
}

// TestModeBeating_DetectsPerPartialRipple is the positive counterpart: two
// STEADY partials (1, 2.76 x f0) where ONLY the second carries its own 3 Hz
// amplitude ripple. This is NOT common-mode (the first partial has no
// ripple at all to share the rate with), so it must be detected.
func TestModeBeating_DetectsPerPartialRipple(t *testing.T) {
	sr := 44100
	dur := 2.0
	n := int(dur * float64(sr))
	s := make([]float64, n)
	f0 := 220.0
	for i := range s {
		tt := float64(i) / float64(sr)
		p1 := math.Sin(2 * math.Pi * f0 * tt)
		ripple := 1 + 0.4*math.Sin(2*math.Pi*3*tt)
		p2 := ripple * math.Sin(2*math.Pi*f0*2.76*tt)
		s[i] = 0.3*p1 + 0.3*p2
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "partial-ripple")
	rate, depth, ok := ModeBeating(b.Modes())
	if !ok {
		t.Fatal("expected the second partial's own ripple to be detected")
	}
	if math.Abs(rate-3) > 0.5 {
		t.Fatalf("rate want ~3 Hz, got %.2f", rate)
	}
	if depth < 0.2 {
		t.Fatalf("depth want >= 0.2, got %.2f", depth)
	}
}

// decayingRippleTrack builds a synthetic dB Track directly (bypassing the
// FFT/tracker, same style as TestModeSelfBeat_RippleEstimate) representing a
// partial decaying at decayDBPerSec (matching Task 1's twoDecayingModes
// shape, amp(t) = 10^(rate*t/20), i.e. a LINEAR trend in dB) with an
// amplitude ripple of rippleHz/rippleDepth riding on top of the decay:
// dB(t) = decayDBPerSec*t + 20*log10(1 + rippleDepth*sin(2*pi*rippleHz*t)).
//
// This is the regression pin for the real bug the detrending fix addressed.
// Every prior beating test uses a constant-envelope tone, which makes
// modeSelfBeat's OLS detrending a no-op — none of them would have caught a
// regression to the pre-fix "count crossings of the raw global mean"
// behaviour, which undercounts cycles once a decaying tail drops well below
// that mean (measured on the real vibraphone synth as a false ~2 Hz instead
// of the true ~5 Hz tremolo). Constructing the Track directly, rather than
// going through a real waveform + STFT, keeps the ripple's dB-domain shape
// exact rather than partially averaged away by the analysis window (whose
// ~0.186s duration is close to a 5 Hz ripple's own 0.2s period) — measuring
// the DETRENDING logic in isolation is the point of this test, not
// re-verifying the FFT/tracker path (already covered by the audio-level
// beating tests above).
func decayingRippleTrack(hopSec, decayDBPerSec, rippleHz, rippleDepth float64, n int) []float64 {
	track := make([]float64, n)
	for i := range track {
		t := float64(i) * hopSec
		track[i] = decayDBPerSec * t
		if rippleDepth != 0 {
			track[i] += 20 * math.Log10(1+rippleDepth*math.Sin(2*math.Pi*rippleHz*t))
		}
	}
	return track
}

// TestModeBeating_DecayingPartialWithRipple pins the detrending fix: a
// partial decaying at -12 dB/s carrying a 5 Hz/depth-0.35 amplitude ripple
// must still be detected. The coordinator's reported pre-fix failure mode on
// this exact shape was rate=1.40/depth=0.91 (wrong, from counting crossings
// of the raw undetrended track) vs rate=5.06/depth=0.36 (correct, with
// detrending) — reproduced here as rate~5.05/depth~0.36.
func TestModeBeating_DecayingPartialWithRipple(t *testing.T) {
	track := decayingRippleTrack(0.02, -12, 5.0, 0.35, 100)
	rate, depth, ok := modeSelfBeat(Mode{Track: track}, 0.02)
	if !ok {
		t.Fatal("expected a ripple on a decaying partial to be detected")
	}
	if math.Abs(rate-5) > 0.5 {
		t.Fatalf("rate want ~5 Hz, got %.3f", rate)
	}
	if depth < 0.25 || depth > 0.45 {
		t.Fatalf("depth want in [0.25, 0.45], got %.3f", depth)
	}
}

// TestModeBeating_DecayingPartialNoRippleIsOK is the negative counterpart:
// the same decay shape with NO ripple must not report beating. Without
// detrending removing the decay trend first, a monotonically falling track
// crosses its own global mean at most once, which already fails
// modeSelfBeat's crossings>=2 floor — this pins that a decay alone, however
// steep, is never mistaken for a ripple.
func TestModeBeating_DecayingPartialNoRippleIsOK(t *testing.T) {
	track := decayingRippleTrack(0.02, -12, 0, 0, 100)
	if _, _, ok := modeSelfBeat(Mode{Track: track}, 0.02); ok {
		t.Fatal("a plain decaying track with no ripple must not report beating")
	}
}

func TestModeRows_AppearForUnpitchedSignal(t *testing.T) {
	// A bar tone whose DetectF0 confidence is irrelevant: the mode rows must
	// apply purely on RequiresModes.
	b := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2.76, 5.40}), "bar")
	ids := map[string]bool{}
	for _, d := range DefaultRegistry().Applicable(b) {
		ids[d.ID] = true
	}
	for _, id := range []string{IDModeAnchorHz, IDModeRatios, IDModeCountPerOctave, IDModeHarmonicity} {
		if !ids[id] {
			t.Errorf("%s must apply to a two-mode bar tone", id)
		}
	}
}

// TestModeRatiosIsDemoted pins I6's ruling.
//
// The row compares the symmetric mean nearest-neighbour gap between two mode
// ladders. That quantity answers "how far is each mode from SOME mode on the
// other side", which is a measure of mode ABSENCE, not of mistuning: when the
// synth simply lacks a partial the reference has, the nearest surviving
// neighbour supplies the gap and a missing mode scores LARGER than a badly
// mistuned one. The corpus proved it — calibration over the listener-approved
// group put the p50 at 520 cents, nearly half an octave, on instruments
// nobody described as mistuned at all.
//
// It stays registered and printed (the ratio list beside it is the clearest
// view of partial structure the report has) and is excluded from ranking.
func TestModeRatiosIsDemoted(t *testing.T) {
	d, ok := DefaultRegistry().ByID(IDModeRatios)
	if !ok {
		t.Fatal("mode.ratios is not registered")
	}
	if !d.Demoted {
		t.Fatal("mode.ratios must be registered with Demoted: true — its nearest-mode gap " +
			"measures mode absence, not mistuning")
	}
	if !strings.Contains(d.Doc, "DEMOTED") {
		t.Error("a demoted descriptor's Doc must say so, and say why")
	}
	// A demoted descriptor must not carry a calibrated entry: calibrating
	// thresholds nobody is allowed to rank on buys nothing and hides the
	// declared values. (thresholds_embedded_test.go gates this globally; this
	// assertion names the row so the failure points at the demotion.)
	if _, ok := LoadThresholdFile()[IDModeRatios]; ok {
		t.Error("mode.ratios is Demoted but still has a thresholds.json entry — regenerate the calibration")
	}
}
