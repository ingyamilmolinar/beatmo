//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every melodic instrument at -12, 0 and +12 must be finite, non-silent and bounded: the per-instrument tests
// render only at pitch 0, where a seed change that silences or explodes another pitch never shows.
func TestAllMelodicInstruments_PitchRangeRendersCleanly(t *testing.T) {
	const sr = 44100
	const n = sr / 4 // 0.25 s is enough for any attack
	pitches := []float64{-12, 0, +12}

	// Collect all melodic instrument IDs (render.melodic in their documents).
	var melodicIDs []string
	for _, instID := range roster.All {
		if IsMelodicInstrument(instID) {
			melodicIDs = append(melodicIDs, string(instID))
		}
	}
	if len(melodicIDs) == 0 {
		t.Fatal("no melodic instruments found")
	}

	for _, instID := range melodicIDs {
		for _, pitch := range pitches {
			name := instID
			p := pitch
			t.Run(name+"/pitch"+formatPitchLabel(p), func(t *testing.T) {
				buf := renderFieldsForTest(t, instrument.ID(instID), sr, n, map[string]float64{"pitch": p})

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

// At +12 an at-pitch render differs from resampling the pitch-0 buffer.
func TestCelloFormantPreservedVsResample(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams(roster.Cello) })

	const sr = 44100

	v0, ok0 := tryRecipeVoicePitched(roster.Cello, sr, 0)
	if !ok0 || v0 == nil {
		t.Fatal("tryRecipeVoicePitched(cello) returned nil/false")
	}
	buf0 := drainVoiceToBuffer(v0, sr)
	if len(buf0) == 0 {
		t.Fatal("empty cello pitch=0 buffer")
	}

	v12, ok12 := tryRecipeVoicePitched(roster.Cello, sr, 12)
	if !ok12 || v12 == nil {
		t.Fatal("tryRecipeVoicePitched(cello) returned nil/false")
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

// More filtenv_amt opens the filter wider in the attack; with filtenv_enabled 0 the output is byte-identical to none.
func TestFiltEnvMonotonicBrightening(t *testing.T) {
	const sr, n = 48000, 48000

	// First-difference energy over the first 2000 samples: a large delta is HF energy.
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
		p.set("osc_type", 1) // saw — harmonically rich
		p.set("env_enabled", 0)
		p.set("filter_enabled", 1)
		p.set("filter_type", 0)     // LP
		p.set("filter_cutoff", 800) // low so amt has room to open
		p.set("filtenv_enabled", enabled)
		p.set("filtenv_amt", amt)
		p.set("filtenv_decay", 0.15) // fast decay so the window is meaningful
		buf := make([]float32, n)
		renderModularBlock(buf, sr, n, p)
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
	baseline.set("osc_type", 1)
	baseline.set("env_enabled", 0)
	baseline.set("filter_enabled", 1)
	baseline.set("filter_type", 0)
	baseline.set("filter_cutoff", 800)
	baseBuf := make([]float32, n)
	renderModularBlock(baseBuf, sr, n, baseline)

	offBuf := renderFiltEnv(0, 5.0) // enabled=0; amt should be ignored
	for i := range offBuf {
		if offBuf[i] != baseBuf[i] {
			t.Fatalf("filtenv_enabled=0 not byte-identical to baseline at sample %d: %v vs %v",
				i, offBuf[i], baseBuf[i])
		}
	}
}

// Two consecutive renders of every shipped document are identical and non-silent.
func TestAllInstrumentDefaultsUnchanged(t *testing.T) {
	const sr = 44100
	const n = sr / 8 // 0.125 s

	for _, id := range roster.All {
		recipeID := id
		t.Run(string(recipeID), func(t *testing.T) {
			buf1 := renderFieldsForTest(t, recipeID, sr, n, nil)
			buf2 := renderFieldsForTest(t, recipeID, sr, n, nil)

			// Deterministic: two renders with same params must be identical.
			for i := range buf1 {
				if buf1[i] != buf2[i] {
					t.Fatalf("%q: non-deterministic render at sample %d (%v vs %v)",
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
				t.Errorf("%q rendered silence from its document (peak %v)", recipeID, peak)
			}

			// Finite.
			for i, v := range buf1 {
				f := float64(v)
				if math.IsNaN(f) || math.IsInf(f, 0) {
					t.Fatalf("%q: non-finite sample at %d: %v", recipeID, i, v)
				}
			}
		})
	}
}
