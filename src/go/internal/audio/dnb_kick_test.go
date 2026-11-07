//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// RAW, the path every note takes.
func renderDnbKickSeed(t testing.TB, sr, samples int) []float32 {
	return renderFieldsForTest(t, roster.DnbKick, sr, samples, nil)
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

// A deep, dark DnB sub kick: energy below ~160 Hz, almost nothing above 400 Hz, fundamental 60–90 Hz, a tail largely
// gone by the end of the buffer.
func TestDnbKickSeedIsKickShaped(t *testing.T) {
	const sr = 48000
	const samples = sr / 2 // 0.5 s, matching the reference length
	buf := renderDnbKickSeed(t, sr, samples)
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

	// Low-band energy dwarfs high-band energy.
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

	// The back quarter has largely decayed relative to the punch.
	early := rmsRange(buf, 0, samples/10)       // first ~50 ms (the body)
	late := rmsRange(buf, samples*3/4, samples) // last ~125 ms (the tail)
	if early <= 0 {
		t.Fatalf("no onset energy: early rms=%.5f", early)
	}
	if late > 0.2*early {
		t.Fatalf("tail too long for a DnB kick: late rms=%.5f is %.0f%% of early rms=%.5f (want <=20%%)", late, 100*late/early, early)
	}
}

// A node's playback must render the instrument's document, the dark sub kick, never the bare modular voice (a
// ~220 Hz blip).
func TestDnbKickPlaybackPathRendersKick(t *testing.T) {
	// A fine grid tells a low fundamental from 218 Hz.
	grid := make([]float64, 0, 200)
	for hz := 30.0; hz <= 400; hz += 2 {
		grid = append(grid, hz)
	}
	for _, id := range []instrument.ID{roster.DnbKick, roster.KickElectro, roster.Kick808, roster.KickAcoustic, roster.KickPunchy, roster.ZgumpKick, roster.RawKick,
		roster.KickFat, roster.KickBoomBap, roster.KickBrutal, roster.KickRock, roster.KickWood, roster.KickAiry, roster.KickOrganicTight, roster.KickMuted, roster.KickMutedRoom} {
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
		// The dominant frequency must be the sub fundamental, not the bare voice's A3; a coarse band test passes both.
		if dom := argmaxMagAt(f, grid, sr); dom > 150 {
			t.Fatalf("%s PLAYBACK dominant freq = %.0f Hz — the bare modular voice (~218 Hz), not the kick. The Render closure must bake the seed.", id, dom)
		}
	}
}

// A representative punchy param set on a raw engine block.
func renderKickVariantForTest(t testing.TB, variant float64, extra RecipeParams) []float32 {
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
	renderModularBlock(buf, 48000, len(buf), initBlockForTest(p))
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

// Variant 5 needs a much higher crest than the saturation-flattened variants (punch), and its grit must decay with
// the body rather than leave a bright hiss.
func TestKickVariant5LayeredHasPunchAndCleanTail(t *testing.T) {
	const sr = 48000
	base := crestOf(renderKickVariantForTest(t, 0, nil))
	hybrid := renderKickVariantForTest(t, 5, nil)
	hc := crestOf(hybrid)
	if hc < 5.0 {
		t.Fatalf("variant 5 (layered) crest=%.2f, want >=5 (the post-distortion punch); base variant crest=%.2f", hc, base)
	}
	// The late tail's HF energy is a small fraction of its total.
	f := f32toF64(hybrid)
	lateLo := len(f) * 70 / 100
	late := f[lateLo:]
	hf := goertzelMagnitude(late, 3000, sr) + goertzelMagnitude(late, 6000, sr) + goertzelMagnitude(late, 9000, sr)
	lo := goertzelMagnitude(late, 50, sr) + goertzelMagnitude(late, 100, sr)
	if hf > 0.5*lo {
		t.Fatalf("variant 5 late tail is hissy: HF=%.5f vs LF=%.5f (grit must decay with the body)", hf, lo)
	}
}

// A sputtering kick's HF fraction swings window to window; a smooth one changes gradually.
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

// Grit run THROUGH the distortion was multiplied by the body wave and fluttered; additive air is smooth even at an
// organic level. So the invariant is smoothness, not darkness; a floor skips inaudible windows.
func TestKickVariant5BodyNotSpitty(t *testing.T) {
	const sr = 48000
	// High air stresses the guard: noise fused into the distortion would sputter loudly.
	buf := renderKickVariantForTest(t, 5, RecipeParams{
		"voice_freq_hz": 45, "gen1_kick_noise": 0.30, "gen1_kick_sat": 1.2,
		"gen1_kick_click": 0.30, "gen1_kick_pe_amt": 0.9, "gen1_kick_pe_rate": 35,
		"gen1_kick_h2": 0.22, "gen1_kick_h3": 0.02, "gen1_kick_env0": 12,
		"gen1_kick_env1": 12, "gen1_kick_attack": 1.5, "gen1_kick_fade": 4,
	})
	hf := windowHFProxy(buf, sr, 72)
	// Windows 0-1 are the attack. Flutter is an UPWARD jump; a clean decay only steps down, so only upward jumps count.
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

func TestKickEnabledGatesVoice(t *testing.T) {
	const sr = 48000
	const samples = sr / 4
	// The document always writes 1, so the engine's gate is driven in the block directly.
	d, _ := ShippedDoc(roster.DnbKick)
	base := modularBlock(instrument.Flatten(d, BuiltinModuleRegistry()))
	render := func(enabled float64) float32 {
		b := base.clone()
		b.set("kick_enabled", enabled)
		buf := make([]float32, samples)
		renderModularBlock(buf, sr, samples, b)
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

// Through its project document.
func TestDnbKickInstrumentWiredAndKickShaped(t *testing.T) {
	const sr, samples = 48000, sr / 2
	buf := renderFieldsForTest(t, roster.DnbKick, sr, samples, nil)
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
