//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestAllMelodicInstruments_PitchRangeRendersCleanly is Gap A: a table test
// over every registered melodic instrument at pitches -12, 0, +12.
//
// For each (instrument, pitch) pair it asserts:
//   - finite (no NaN or Inf)
//   - non-silent (peak > 0.01)
//   - bounded (|sample| < 2.0 — generous headroom for summed harmonics)
//
// This guards against a seed change that produces silence or explosion
// at a non-default pitch, which the existing per-instrument seed tests
// do not catch (they render only at the default pitch=0).
func TestAllMelodicInstruments_PitchRangeRendersCleanly(t *testing.T) {
	const sr = 44100
	const n = sr / 4 // 0.25 s is enough for any attack
	pitches := []float64{-12, 0, +12}

	// Collect all melodic instrument IDs (those whose recipe is pitch-aware).
	var melodicIDs []string
	for instID, recipeID := range builtinInstrumentRecipeBindings {
		if pitchAwareRecipe(recipeID) {
			melodicIDs = append(melodicIDs, instID)
		}
	}
	if len(melodicIDs) == 0 {
		t.Fatal("no melodic instruments found — builtinInstrumentRecipeBindings may be empty")
	}

	for _, instID := range melodicIDs {
		recipeID := RecipeForInstrument(instID)
		if recipeID == "" {
			t.Errorf("instrument %q has no recipe binding", instID)
			continue
		}
		r := NewRecipe(recipeID)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil for instrument %q", recipeID, instID)
			continue
		}
		defaults := RecipeDefaultParams(recipeID)

		for _, pitch := range pitches {
			name := instID
			p := pitch
			t.Run(name+"/pitch"+formatPitchLabel(p), func(t *testing.T) {
				params := cloneRecipeParams(defaults)
				params["pitch"] = p
				buf := make([]float32, n)
				r.Render(buf, sr, n, 0, params)

				var peak float32
				for i, v := range buf {
					f := float64(v)
					if math.IsNaN(f) || math.IsInf(f, 0) {
						t.Fatalf("instrument %q at pitch %+.0f: non-finite sample at %d: %v",
							instID, p, i, v)
					}
					a := v
					if a < 0 {
						a = -a
					}
					if a > peak {
						peak = a
					}
				}
				if peak < 0.01 {
					t.Errorf("instrument %q at pitch %+.0f: silent (peak %v)", instID, p, peak)
				}
				if peak >= 2.0 {
					t.Errorf("instrument %q at pitch %+.0f: exploded (peak %v >= 2.0)", instID, p, peak)
				}
			})
		}
	}
}

// formatPitchLabel returns "+12", "0", "-12" etc. for sub-test naming.
func formatPitchLabel(pitch float64) string {
	if pitch > 0 {
		return "+" + floatToIntStr(pitch)
	}
	if pitch < 0 {
		return floatToIntStr(pitch)
	}
	return "0"
}

func floatToIntStr(f float64) string {
	i := int(f)
	s := ""
	if i < 0 {
		s = "-"
		i = -i
	}
	digits := []byte{}
	if i == 0 {
		return s + "0"
	}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return s + string(digits)
}

// TestCelloFormantPreservedVsResample is Gap B (second melodic instrument):
// For cello at pitch=+12, the at-pitch render differs from a naive
// resample-by-+12 of the pitch=0 buffer. Complements the existing violin test.
func TestCelloFormantPreservedVsResample(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("cello") })

	const sr = 44100

	v0, ok0 := tryRecipeVoicePitched("cello", 120, sr, 0)
	if !ok0 || v0 == nil {
		t.Fatal("tryRecipeVoicePitched(cello, pitch=0) returned nil/false")
	}
	buf0 := drainVoiceToBuffer(v0, sr)
	if len(buf0) == 0 {
		t.Fatal("empty cello pitch=0 buffer")
	}

	v12, ok12 := tryRecipeVoicePitched("cello", 120, sr, 12)
	if !ok12 || v12 == nil {
		t.Fatal("tryRecipeVoicePitched(cello, pitch=12) returned nil/false")
	}
	buf12 := drainVoiceToBuffer(v12, sr)
	if len(buf12) == 0 {
		t.Fatal("empty cello pitch=+12 buffer")
	}

	// Resample buf0 at rate=2 (octave up) — the legacy resampling path.
	resampled := resampleBuffer(buf0, 2.0)

	n := len(buf12)
	if len(resampled) < n {
		n = len(resampled)
	}
	var l1dist float64
	for i := 0; i < n; i++ {
		d := float64(buf12[i]) - float64(resampled[i])
		if d < 0 {
			d = -d
		}
		l1dist += d
	}
	if l1dist == 0 {
		t.Error("cello at-pitch render is identical to resample — formant not fixed (re-render has no effect)")
	}
	t.Logf("cello pitch=+12 re-render vs resample L1 dist = %.4f (over %d samples)", l1dist, n)
}

// TestFiltEnvMonotonicBrightening is Gap C:
// With filtenv_enabled:1, increasing filtenv_amt should produce MORE
// high-frequency energy in the attack window (the filter opens wider).
// And with filtenv_enabled:0 the output is byte-identical to no-filtenv.
func TestFiltEnvMonotonicBrightening(t *testing.T) {
	const sr, n = 48000, 48000

	// hfAttackEnergy measures high-frequency energy in the first 2000 samples
	// via first-difference squared sum (large delta = HF energy).
	hfAttackEnergy := func(buf []float32) float64 {
		end := 2000
		if end > len(buf) {
			end = len(buf)
		}
		var e float64
		for i := 1; i < end; i++ {
			d := float64(buf[i] - buf[i-1])
			e += d * d
		}
		return e
	}

	renderFiltEnv := func(enabled, amt float64) []float32 {
		p := defaultModularParams()
		p.OscType = 1 // saw — harmonically rich
		p.EnvEnabled = 0
		p.FilterEnabled = 1
		p.FilterType = 0     // LP
		p.FilterCutoff = 800 // low so amt has room to open
		p.FiltEnvEnabled = enabled
		p.FiltEnvAmt = amt
		p.FiltEnvDecay = 0.15 // fast decay so the window is meaningful
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}

	// Monotonic: amt1 < amt2 < amt3 should produce increasing HF energy in attack.
	buf1 := renderFiltEnv(1, 0.5)
	buf2 := renderFiltEnv(1, 2.0)
	buf3 := renderFiltEnv(1, 5.0)

	e1 := hfAttackEnergy(buf1)
	e2 := hfAttackEnergy(buf2)
	e3 := hfAttackEnergy(buf3)
	t.Logf("filtenv_amt=0.5 HF attack energy: %v", e1)
	t.Logf("filtenv_amt=2.0 HF attack energy: %v", e2)
	t.Logf("filtenv_amt=5.0 HF attack energy: %v", e3)

	if !(e2 > e1) {
		t.Errorf("filtenv_amt=2.0 (e=%v) not brighter than amt=0.5 (e=%v)", e2, e1)
	}
	if !(e3 > e2) {
		t.Errorf("filtenv_amt=5.0 (e=%v) not brighter than amt=2.0 (e=%v)", e3, e2)
	}

	// Identity: filtenv_enabled:0 must be byte-identical to baseline (no filtenv params set).
	baseline := defaultModularParams()
	baseline.OscType = 1
	baseline.EnvEnabled = 0
	baseline.FilterEnabled = 1
	baseline.FilterType = 0
	baseline.FilterCutoff = 800
	baseBuf := make([]float32, n)
	renderModularP(baseBuf, sr, n, baseline)

	offBuf := renderFiltEnv(0, 5.0) // enabled=0; amt should be ignored
	for i := range offBuf {
		if offBuf[i] != baseBuf[i] {
			t.Fatalf("filtenv_enabled=0 not byte-identical to baseline at sample %d: %v vs %v",
				i, offBuf[i], baseBuf[i])
		}
	}
}

// TestAllInstrumentDefaultsUnchanged is Gap D:
// Every builtin instrument rendered with all-default recipe params must produce
// the SAME buffer on two consecutive calls (deterministic) and must not be
// silent. This guards against a seed change silently wiping an instrument's
// defaults so the next render plays a different sound.
func TestAllInstrumentDefaultsUnchanged(t *testing.T) {
	const sr = 44100
	const n = sr / 8 // 0.125 s

	for _, id := range RecipeOrder() {
		recipeID := id
		t.Run(recipeID, func(t *testing.T) {
			r := NewRecipe(recipeID)
			if r == nil {
				t.Fatalf("NewRecipe(%q) returned nil", recipeID)
			}
			defaults := RecipeDefaultParams(recipeID)

			buf1 := make([]float32, n)
			buf2 := make([]float32, n)
			r.Render(buf1, sr, n, 0, defaults)
			r.Render(buf2, sr, n, 0, defaults)

			// Deterministic: two renders with same params must be identical.
			for i := range buf1 {
				if buf1[i] != buf2[i] {
					t.Fatalf("recipe %q: non-deterministic render at sample %d (%v vs %v)",
						recipeID, i, buf1[i], buf2[i])
				}
			}

			// Not silent.
			var peak float32
			for _, v := range buf1 {
				a := v
				if a < 0 {
					a = -a
				}
				if a > peak {
					peak = a
				}
			}
			if peak < 1e-6 {
				t.Errorf("recipe %q with default params rendered silence (peak %v)", recipeID, peak)
			}

			// Finite.
			for i, v := range buf1 {
				f := float64(v)
				if math.IsNaN(f) || math.IsInf(f, 0) {
					t.Fatalf("recipe %q: non-finite sample at %d: %v", recipeID, i, v)
				}
			}
		})
	}
}

// TestRegistryF_EveryBindingInstrumentIDIsInBuiltinList is Gap F (complementary):
// Every key in builtinInstrumentRecipeBindings must appear in BuiltinInstrumentIDs.
// Catches cases where a new binding is added to the recipe map but the generated
// instrument ID list is not regenerated.
func TestRegistryF_EveryBindingInstrumentIDIsInBuiltinList(t *testing.T) {
	builtinSet := make(map[string]bool, len(BuiltinInstrumentIDs))
	for _, id := range BuiltinInstrumentIDs {
		builtinSet[id] = true
	}
	for instID := range builtinInstrumentRecipeBindings {
		if !builtinSet[instID] {
			t.Errorf("instrument %q is in builtinInstrumentRecipeBindings but NOT in BuiltinInstrumentIDs", instID)
		}
	}
}
