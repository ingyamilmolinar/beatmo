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

// On a synthetic dB track, bypassing the FFT: amp = 1 + m·sin(2πft) has depth exactly m and ripples at exactly f.
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

// A global 5 Hz tremolo moves every steady partial identically, so every mode ripples at the same rate: common-mode
// modulation, not beating.
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

// Only the second partial ripples, so there is no shared rate and it is beating.
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

// A decaying partial (linear in dB) with a ripple on top, built directly so the ripple's dB shape is exact rather
// than averaged by an analysis window close to its period. A constant-envelope tone makes detrending a no-op, so
// only this shape catches a regression to counting raw-mean crossings.
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

// -12 dB/s with a 5 Hz / 0.35 ripple: without detrending it read rate 1.40 / depth 0.91.
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

// A decay alone, however steep, is never a ripple.
func TestModeBeating_DecayingPartialNoRippleIsOK(t *testing.T) {
	track := decayingRippleTrack(0.02, -12, 0, 0, 100)
	if _, _, ok := modeSelfBeat(Mode{Track: track}, 0.02); ok {
		t.Fatal("a plain decaying track with no ripple must not report beating")
	}
}

func TestModeRows_AppearForUnpitchedSignal(t *testing.T) {
	// The mode rows apply on RequiresModes alone, whatever DetectF0's confidence.
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

// The nearest-neighbour gap measures mode ABSENCE, not mistuning: a missing partial borrows a far neighbour and scores
// worse than a badly tuned one (the approved group's p50 was 520 cents). Printed, excluded from ranking.
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
	// Calibrating thresholds nobody may rank on hides the declared values.
	if _, ok := LoadThresholdFile()[IDModeRatios]; ok {
		t.Error("mode.ratios is Demoted but still has a thresholds.json entry — regenerate the calibration")
	}
}
