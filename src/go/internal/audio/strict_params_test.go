//go:build !test && !js

package audio

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestZeroIsARealValue: a knob turned to 0 must sound like 0, not like the C
// default the engine used to swap in for it (Phase C, review focus 1): a hi-hat
// with rattle 0 rattled fully, a plate with tune 0 sounded at tune 1.
func TestZeroIsARealValue(t *testing.T) {
	for _, c := range []struct {
		id, param string
		old       float64 // the value C used to substitute for 0
	}{
		{string(roster.Hihat), "gen1_cym_rattle", 1.0},      // was: 0 → 1.0 (full rattle)
		{string(roster.CongaOrganic), "gen3_cym_tune", 1.0}, // was: 0 → 1.0 (plate at pitch)
		{string(roster.Hihat), "gen1_cym_modes", 1.0},       // was: 0 → 1 mode (now: no plate)
	} {
		zero, okZ := engineCaseRender(c.id, c.param, 0)
		old, okO := engineCaseRender(c.id, c.param, c.old)
		if !okZ || !okO {
			t.Errorf("%s: %s is not settable on its document — the case no longer exercises the engine", c.id, c.param)
			continue
		}
		if equalF32(zero, old) {
			t.Errorf("%s %s=0 renders exactly like %s=%v: 0 is still swapped for the C default",
				c.id, c.param, c.param, c.old)
		}
	}
}

// TestNoFallbackHelpersRemainInC: the engine reads every param directly — no
// NULL-params path, no NaN sentinel, no "0 means default" substitution.
func TestNoFallbackHelpersRemainInC(t *testing.T) {
	root := repoRootForC(t)
	for _, f := range []string{"modular.c", "modular_stages.c", "modular.h", "synth_post.h", "drums.c"} {
		src, err := os.ReadFile(filepath.Join(root, "src", "c", f))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{
			"kp_get(", "mp_get(", "cym_modal_param(", "cym_modal_level(",
			"(gain > 0.0) ? gain : 1.0", "const modular_params zero", "void render_modular(",
			// the POST indirection: per-op enable flags that were always 1 and
			// "default if params is NULL" accessors
			"apply_post_params", "post_config", "sp_decay(",
			// surviving "0 / NaN means something else" substitutions
			"lpAlpha = 0.5", "(modesV == modesV)", "modes == modes", "slotGain",
		} {
			if bytes.Contains(src, []byte(banned)) {
				t.Errorf("src/c/%s still contains %q", f, banned)
			}
		}
	}
}

func equalF32(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return false
		}
	}
	return true
}

// TestFamilyVoiceBeyondCapacityRendersSilence: the engine defends itself when a
// block puts a family voice in a slot that family has no fields for (Phase C
// task C3) — that slot contributes nothing, instead of reading another family's
// memory.
func TestFamilyVoiceBeyondCapacityRendersSilence(t *testing.T) {
	b := initBlockForTest(nil)
	b.set("osc_enabled", 0)
	b.set("gen2_source", 5) // a kick voice in slot 2; kick fields exist for slot 1 only
	buf := make([]float32, 4800)
	renderModularBlock(buf, 48000, len(buf), b)
	for i, v := range buf {
		if v != 0 {
			t.Fatalf("sample %d = %v: a kick in slot 2 rendered sound", i, v)
		}
	}
}

// TestLinkedEngineMatchesTheSchema: the C engine that is actually LINKED was
// compiled against the same param block the Go marshal builds. A header
// regenerated without rebuilding build/libdrums.a (or drums.single.js) leaves
// every later field read at a shifted offset — wrong voices, no error.
func TestLinkedEngineMatchesTheSchema(t *testing.T) {
	if n := compiledModularParamCount(); n != len(modularParamSchema) {
		t.Fatalf("linked C engine has %d params, the schema %d — rebuild build/libdrums.a", n, len(modularParamSchema))
	}
}

// TestSnareVoiceStaysStableAcrossTuneRange: Tune at either end of its range
// must leave a working voice. At 0 the voice's filters were designed at 0 Hz
// (rc = 1/0 → NaN from the first sample); at Max the wire band-pass landed
// above Nyquist and the biquad blew up within ~20 ms. The FINISH stage's
// non-finite guard turns that into silence, so measure the silence.
func TestSnareVoiceStaysStableAcrossTuneRange(t *testing.T) {
	for _, id := range []instrument.ID{roster.Snare, roster.Rimshot, roster.SnareCrack, roster.SnareDeep, roster.SnareAcoustic, roster.Clap, roster.CongaOrganic, roster.TomHighAcoustic} {
		var lo, hi float64
		found := false
		for _, d := range InstrumentParamDefs(id) {
			if d.Name == "gen1_snare_tune" {
				lo, hi, found = d.Min, d.Max, true
			}
		}
		if !found {
			t.Fatalf("%s: no gen1_snare_tune knob", id)
		}
		for _, v := range []float64{lo, hi} {
			buf, ok := engineCaseRender(string(id), "gen1_snare_tune", v)
			if !ok {
				t.Fatalf("%s: gen1_snare_tune is not settable on its document", id)
			}
			zeros := 0
			for _, s := range buf {
				if s == 0 {
					zeros++
				}
			}
			if zeros > len(buf)/2 {
				t.Errorf("%s gen1_snare_tune=%v: %d of %d samples are exact zeros — the voice blew up (non-finite) and was silenced", id, v, zeros, len(buf))
			}
		}
	}
}
