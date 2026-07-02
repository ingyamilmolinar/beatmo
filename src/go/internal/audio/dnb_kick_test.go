//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// renderDnbKickSeed renders the dnbKickSeed config through the modular engine
// (the same recipe path the dnb-kick instrument uses), returning the raw buffer.
func renderDnbKickSeed(sr, samples int) []float32 {
	mp := recipeParamsToModular(dnbKickSeed)
	buf := make([]float32, samples)
	renderModularP(buf, sr, samples, mp)
	return buf
}

func f32toF64(buf []float32) []float64 {
	out := make([]float64, len(buf))
	for i, v := range buf {
		out[i] = float64(v)
	}
	return out
}

func maxMagAt(f []float64, freqs []float64, sr int) float64 {
	m := 0.0
	for _, fr := range freqs {
		if g := goertzelMagnitude(f, fr, sr); g > m {
			m = g
		}
	}
	return m
}

func argmaxMagAt(f []float64, freqs []float64, sr int) float64 {
	best, bestMag := 0.0, -1.0
	for _, fr := range freqs {
		if g := goertzelMagnitude(f, fr, sr); g > bestMag {
			bestMag, best = g, fr
		}
	}
	return best
}

// TestDnbKickSeedIsKickShaped pins the structural identity measured from the
// reference 36010__sandyrb__dnb-kick-003.wav: a deep, dark DnB sub-kick —
// bass-dominated (energy concentrated below ~160 Hz, essentially nothing above
// 400 Hz), fundamental ≈ 60–90 Hz, and a short tail that has largely decayed by
// the back of the buffer. The seed is tuned (A/B vs the reference) to satisfy
// these; the test is the regression net for the kick's identity.
func TestDnbKickSeedIsKickShaped(t *testing.T) {
	const sr = 48000
	const samples = sr / 2 // 0.5 s, matching the reference length
	buf := renderDnbKickSeed(sr, samples)
	f := f32toF64(buf)

	// 1. Non-silent.
	peak := 0.0
	for _, v := range buf {
		if a := math.Abs(float64(v)); a > peak {
			peak = a
		}
	}
	if peak < 0.1 {
		t.Fatalf("dnb-kick is silent/too quiet: peak=%.4f", peak)
	}

	// 2. Bass-dominated and dark: low-band energy dwarfs high-band energy
	// (the reference has ~0 energy above 400 Hz, 1.4% HF click).
	low := maxMagAt(f, []float64{45, 55, 65, 75, 90, 110}, sr)
	high := maxMagAt(f, []float64{800, 1500, 3000, 6000}, sr)
	if low < 4*high {
		t.Fatalf("not a dark sub-kick: low-band mag=%.5f only %.1fx high-band mag=%.5f (want >=4x)", low, low/(high+1e-12), high)
	}

	// 3. Fundamental sits low (reference settles ~60 Hz with a strong sub).
	dom := argmaxMagAt(f, []float64{45, 55, 65, 75, 90, 110, 140, 180}, sr)
	if dom > 120 {
		t.Fatalf("fundamental too high for a DnB sub-kick: dominant=%.0f Hz (want <=120)", dom)
	}

	// 4. Short tail: the back quarter of the buffer has largely decayed
	// relative to the punch (reference is −40 dB by ~98 ms).
	early := rmsRange(buf, 0, samples/10)       // first ~50 ms (the body)
	late := rmsRange(buf, samples*3/4, samples) // last ~125 ms (the tail)
	if early <= 0 {
		t.Fatalf("no onset energy: early rms=%.5f", early)
	}
	if late > 0.2*early {
		t.Fatalf("tail too long for a DnB kick: late rms=%.5f is %.0f%% of early rms=%.5f (want <=20%%)", late, 100*late/early, early)
	}
}

// TestDnbKickPlaybackPathRendersKick is THE regression for the "seeded modular
// instrument renders a generic tone" gotcha. A node triggers playback through
// the tryRecipeVoiceOpts→legacyNewVoice dispatch (RenderInstrumentOneShotRaw
// uses the identical path); because the seed IS the shipped default,
// RecipeDefaultsCustomized is false, so the recipe path is NOT taken and the
// instrument's Render closure must itself bake the kick seed. Before the fix the
// playback path rendered the BARE modular voice (a ~220 Hz sine blip — the
// "high-pitched electronic xylophone" the user heard). This asserts playback is
// the dark, bass-dominated kick, matching the recipe render.
func TestDnbKickPlaybackPathRendersKick(t *testing.T) {
	// Every configurable-KICK-stage instrument is a seeded modular preset and
	// MUST bake its seed in its Render closure (bakedModularRender) — else the
	// node plays the bare ~218 Hz modular voice.
	grid := make([]float64, 0, 200)
	for hz := 30.0; hz <= 400; hz += 2 {
		grid = append(grid, hz)
	}
	for _, id := range []string{"dnb-kick", "kick-electro", "kick-808", "kick-acoustic", "kick-punchy"} {
		buf, sr := RenderInstrumentOneShotRaw(id)
		if len(buf) == 0 {
			t.Fatalf("%s playback produced no samples", id)
		}
		f := f32toF64(buf)
		peak := 0.0
		for _, v := range buf {
			if a := math.Abs(float64(v)); a > peak {
				peak = a
			}
		}
		if peak < 0.1 {
			t.Fatalf("%s playback silent/too quiet: peak=%.4f", id, peak)
		}
		// The DECISIVE check: the dominant frequency must be the kick's sub
		// fundamental (≤~80 Hz), NOT the bare modular voice's ~218 Hz A3 sine (the
		// pre-fix "electronic xylophone"). Scan a fine grid so a low fundamental
		// and 218 Hz are distinguished (a coarse low/high band test passes for both).
		if dom := argmaxMagAt(f, grid, sr); dom > 150 {
			t.Fatalf("%s PLAYBACK dominant freq = %.0f Hz — the bare modular voice (~218 Hz), not the kick. The Render closure must bake the seed.", id, dom)
		}
	}
}

// renderKickVariantForTest renders the source==5 kick voice at the given variant
// with a representative punchy param set, returning the raw buffer (no clamp —
// recipeParamsToModular is direct).
func renderKickVariantForTest(variant float64, extra RecipeParams) []float32 {
	p := RecipeParams{
		"kick_enabled": 1,
		"gen1_source":  5, "gen1_freq_mode": 0, "gen1_freq": 1, "gen1_gain": 1,
		"osc_enabled": 0, "env_enabled": 0, "filter_enabled": 0, "drive_enabled": 0,
		"voice_freq_hz": 50, "gen1_kick_variant": variant,
		"gen1_kick_h2": 0.4, "gen1_kick_h3": 0.2, "gen1_kick_h4": 0.1,
		"gen1_kick_env0": 6, "gen1_kick_env1": 11,
		"gen1_kick_pe_amt": 2.0, "gen1_kick_pe_rate": 50,
		"gen1_kick_click": 0.5, "gen1_kick_noise": 0.2,
		"gen1_kick_fade": 6, "gen1_kick_attack": 1.5, "gen1_kick_sat": 2.0,
	}
	for k, v := range extra {
		p[k] = v
	}
	buf := make([]float32, 48000/2)
	renderModularP(buf, 48000, len(buf), recipeParamsToModular(p))
	return buf
}

func crestOf(buf []float32) float64 {
	peak, sum := 0.0, 0.0
	for _, v := range buf {
		a := math.Abs(float64(v))
		if a > peak {
			peak = a
		}
		sum += float64(v) * float64(v)
	}
	return peak / (math.Sqrt(sum/float64(len(buf))) + 1e-12)
}

// TestKickVariant5LayeredHasPunchAndCleanTail is THE driver for the layered-kick
// DSP: variant 5 must produce a much higher CREST (the sharp transient = punch)
// than the saturation-flattened existing variants (~3), via a post-distortion
// click spike — AND its grit must decay with the body (the late tail is NOT a
// sustained bright hiss).
func TestKickVariant5LayeredHasPunchAndCleanTail(t *testing.T) {
	const sr = 48000
	base := crestOf(renderKickVariantForTest(0, nil))
	hybrid := renderKickVariantForTest(5, nil)
	hc := crestOf(hybrid)
	if hc < 5.0 {
		t.Fatalf("variant 5 (layered) crest=%.2f, want >=5 (the post-distortion punch); base variant crest=%.2f", hc, base)
	}
	// Grit must decay with the body: the late tail (last 30%) must not be a
	// bright hiss — its HF (>2 kHz) energy must be a small fraction of its total.
	f := f32toF64(hybrid)
	lateLo := len(f) * 70 / 100
	late := f[lateLo:]
	hf := goertzelMagnitude(late, 3000, sr) + goertzelMagnitude(late, 6000, sr) + goertzelMagnitude(late, 9000, sr)
	lo := goertzelMagnitude(late, 50, sr) + goertzelMagnitude(late, 100, sr)
	if hf > 0.5*lo {
		t.Fatalf("variant 5 late tail is hissy: HF=%.5f vs LF=%.5f (grit must decay with the body)", hf, lo)
	}
}

// windowHFProxy returns the per-8ms-window high-frequency energy fraction (a
// first-difference high-pass proxy) over the kick BODY (0..bodyMs). A "spitty"
// kick sputters — its HF fraction swings violently window-to-window; a smooth
// kick's HF fraction changes gradually.
func windowHFProxy(buf []float32, sr, bodyMs int) []float64 {
	win := sr * 8 / 1000
	var out []float64
	for i := 0; i+win <= len(buf) && i < sr*bodyMs/1000; i += win {
		var hf, tot float64
		for j := i + 1; j < i+win; j++ {
			d := float64(buf[j] - buf[j-1])
			hf += d * d
			tot += float64(buf[j]) * float64(buf[j])
		}
		out = append(out, hf/(tot+1e-12))
	}
	return out
}

// TestKickVariant5BodyNotSpitty is the ANTI-SPIT invariant. The true "spit" was
// AM-FLUTTER: the grit noise ran THROUGH the distortion and was multiplied by the
// body wave, so the body's high-frequency content swung wildly window-to-window
// (per-window HF proxy jumping 0.0005↔0.021, adjacent ratios >20×, up to 1315×).
// The decoupled ADDITIVE air is smooth — even carrying the reference's organic
// air level (~2% in 1-3 kHz), the body's per-window HF holds ~0.016-0.019, an
// adjacent ratio of ~1.2×. So the invariant is SMOOTHNESS (no flutter), not an
// absolute darkness cap: organic air is fine, sputtering air is the spit. A floor
// skips inaudible noise-floor windows whose ratios are meaningless.
func TestKickVariant5BodyNotSpitty(t *testing.T) {
	const sr = 48000
	// High air (0.30) to STRESS the guard — if a change re-fused the noise into
	// the distortion, this much noise would sputter loudly.
	buf := renderKickVariantForTest(5, RecipeParams{
		"voice_freq_hz": 45, "gen1_kick_noise": 0.30, "gen1_kick_sat": 1.2,
		"gen1_kick_click": 0.30, "gen1_kick_pe_amt": 0.9, "gen1_kick_pe_rate": 35,
		"gen1_kick_h2": 0.22, "gen1_kick_h3": 0.02, "gen1_kick_env0": 12,
		"gen1_kick_env1": 12, "gen1_kick_attack": 1.5, "gen1_kick_fade": 4,
	})
	hf := windowHFProxy(buf, sr, 72)
	// Windows 0-1 (0-16 ms) are the attack transient. Flutter's signature is an
	// UPWARD spike — a quiet body window followed by a bright one (the fused-grit
	// spit swung 0.0005→0.021, jumping UP >20×). A smooth attack-decayed air only
	// steps DOWN (monotonic), so only upward jumps count. (An adjacent-ratio that
	// also counted downward steps false-positives on a fast, clean decay.)
	worst := 1.0
	for i := 2; i < len(hf); i++ {
		if hf[i] < 1e-4 || hf[i-1] < 1e-4 {
			continue
		}
		if r := hf[i] / hf[i-1]; r > worst {
			worst = r // >1 only when this window is BRIGHTER than the previous
		}
	}
	if worst > 4.0 {
		t.Fatalf("variant 5 body FLUTTERS (spit): worst UPWARD HF jump=%.1f× (want <=4×) — a bright window after a quiet one = noise sputtering with the body. per-window HF=%v", worst, hf)
	}
}

// TestKickEnabledGatesVoice drives the KICK stage's enable pill: with the kick
// gen-slot active (source==5), kick_enabled<0.5 silences the voice and
// kick_enabled>=0.5 renders it. This is what makes the Synth-tab KICK enable
// pill meaningful (it toggles kick_enabled).
func TestKickEnabledGatesVoice(t *testing.T) {
	const sr = 48000
	const samples = sr / 4
	render := func(enabled float64) float32 {
		p := RecipeParams{}
		for k, v := range dnbKickSeed {
			p[k] = v
		}
		p["kick_enabled"] = enabled
		buf := make([]float32, samples)
		renderModularP(buf, sr, samples, recipeParamsToModular(p))
		peak := float32(0)
		for _, v := range buf {
			if a := v; a > peak {
				peak = a
			} else if -a > peak {
				peak = -a
			}
		}
		return peak
	}
	on := render(1)
	off := render(0)
	if on < 0.1 {
		t.Fatalf("kick_enabled=1 should render the kick: peak=%.4f", on)
	}
	if off > 1e-6 {
		t.Fatalf("kick_enabled=0 should silence the kick: peak=%.6f", off)
	}
}

// TestDnbKickInstrumentWiredAndKickShaped drives the instrument/recipe wiring:
// the dnb-kick instrument binds to the synth-modular-kick-dnb recipe, which
// registers and renders the same kick-shaped, bass-dominated signal as the seed.
func TestDnbKickInstrumentWiredAndKickShaped(t *testing.T) {
	const want = "synth-modular-kick-dnb"
	if got := RecipeForInstrument("dnb-kick"); got != want {
		t.Fatalf("dnb-kick recipe binding = %q, want %q", got, want)
	}
	r := NewRecipe(want)
	if r == nil {
		t.Fatalf("recipe %q not registered", want)
	}
	const sr, samples = 48000, sr / 2
	buf := make([]float32, samples)
	r.Render(buf, sr, samples, 0, MergeRecipeDefaults(want, nil))
	f := f32toF64(buf)

	peak := 0.0
	for _, v := range buf {
		if a := math.Abs(float64(v)); a > peak {
			peak = a
		}
	}
	if peak < 0.1 {
		t.Fatalf("dnb-kick recipe render silent/too quiet: peak=%.4f", peak)
	}
	low := maxMagAt(f, []float64{45, 55, 65, 75, 90, 110}, sr)
	high := maxMagAt(f, []float64{800, 1500, 3000, 6000}, sr)
	if low < 4*high {
		t.Fatalf("dnb-kick recipe not a dark sub-kick: low=%.5f only %.1fx high=%.5f", low, low/(high+1e-12), high)
	}
}
