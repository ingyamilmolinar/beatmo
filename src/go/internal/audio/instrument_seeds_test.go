//go:build !test && !js

package audio

import "testing"

// renderSeedInstrument renders one builtin instrument id through its bound recipe
// and returns the buffer. Helper for seed render-smoke tests.
func renderSeedInstrument(t *testing.T, id string, n int) []float32 {
	t.Helper()
	rec := RecipeForInstrument(id)
	if rec == "" {
		t.Fatalf("instrument %q has no recipe binding", id)
	}
	p := RecipeDefaultParams(rec) // includes the seed (baked into recipe defaults)
	buf := make([]float32, n)
	renderModularP(buf, 48000, n, recipeParamsToModular(p))
	return buf
}

func assertAudible(t *testing.T, id string, buf []float32) {
	t.Helper()
	var peak float32
	for _, v := range buf {
		if v != v || v > 8 || v < -8 {
			t.Fatalf("%s: non-finite/exploded sample %v", id, v)
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
		t.Fatalf("%s: silent (peak %v)", id, peak)
	}
}

func TestSeed_BowedStrings_Audible(t *testing.T) {
	for _, id := range []string{"violin", "violin-ensemble", "cello", "cello-warm"} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_BowedStrings_DistinctFromDefault(t *testing.T) {
	def := make([]float32, 48000)
	renderModularP(def, 48000, 48000, defaultModularParams())
	for _, id := range []string{"violin", "cello"} {
		buf := renderSeedInstrument(t, id, 48000)
		diff := 0
		for i := range buf {
			if buf[i] != def[i] {
				diff++
			}
		}
		if diff == 0 {
			t.Fatalf("%s identical to bare modular default — seed not applied", id)
		}
	}
}

func TestSeed_StringsCategory(t *testing.T) {
	for _, id := range []string{"violin", "cello"} {
		if got := synthCategory(id); got != "Strings (Synth)" {
			t.Fatalf("synthCategory(%q)=%q, want Strings (Synth)", id, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 2: Guitars (Karplus-Strong)
// ---------------------------------------------------------------------------

func TestSeed_Guitars_Audible(t *testing.T) {
	for _, id := range []string{
		"guitar-nylon", "guitar-nylon-bright",
		"guitar-steel", "guitar-steel-warm",
		"guitar-electric", "guitar-electric-neck",
	} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_Guitars_NylonDarkerThanSteel(t *testing.T) {
	// Brighter = faster sample-to-sample change → larger Σ|buf[i]-buf[i-1]|.
	nylon := renderSeedInstrument(t, "guitar-nylon", 48000)
	steel := renderSeedInstrument(t, "guitar-steel", 48000)
	var diffNylon, diffSteel float64
	for i := 1; i < len(nylon); i++ {
		d := float64(nylon[i] - nylon[i-1])
		if d < 0 {
			d = -d
		}
		diffNylon += d
		d = float64(steel[i] - steel[i-1])
		if d < 0 {
			d = -d
		}
		diffSteel += d
	}
	if diffSteel <= diffNylon {
		t.Fatalf("expected steel (brighter, diffSteel=%v) > nylon (diffNylon=%v)", diffSteel, diffNylon)
	}
}

// ---------------------------------------------------------------------------
// Task 3: Piano (additive)
// ---------------------------------------------------------------------------

func TestSeed_Piano_Audible(t *testing.T) {
	for _, id := range []string{"piano-grand", "piano-felt"} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_KeysCategory(t *testing.T) {
	for _, id := range []string{"piano-grand", "piano-felt"} {
		if got := synthCategory(id); got != "Keys (Synth)" {
			t.Fatalf("synthCategory(%q)=%q, want Keys (Synth)", id, got)
		}
	}
}

func TestSeed_Piano_DecaysNotSustained(t *testing.T) {
	// Piano has sustain≈0 → tail half should have lower RMS than first half.
	for _, id := range []string{"piano-grand", "piano-felt"} {
		buf := renderSeedInstrument(t, id, 96000) // 2s at 48kHz
		half := len(buf) / 2
		var rmsFirst, rmsTail float64
		for i := 0; i < half; i++ {
			v := float64(buf[i])
			rmsFirst += v * v
		}
		for i := half; i < len(buf); i++ {
			v := float64(buf[i])
			rmsTail += v * v
		}
		rmsFirst /= float64(half)
		rmsTail /= float64(len(buf) - half)
		if rmsTail >= rmsFirst {
			t.Fatalf("%s: tail RMS (%v) >= first-half RMS (%v) — expected percussive decay", id, rmsTail, rmsFirst)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 4: Woodwind
// ---------------------------------------------------------------------------

func TestSeed_Woodwind_Audible(t *testing.T) {
	for _, id := range []string{"flute", "flute-breathy", "oboe", "oboe-full"} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_WindsCategory(t *testing.T) {
	for _, id := range []string{"flute", "oboe", "trumpet", "french-horn"} {
		if got := synthCategory(id); got != "Winds (Synth)" {
			t.Fatalf("synthCategory(%q)=%q, want Winds (Synth)", id, got)
		}
	}
}

func TestSeed_Flute_HasVibrato(t *testing.T) {
	// Flute with LFO vs flute with LFO disabled should differ.
	buf := renderSeedInstrument(t, "flute", 48000)
	// Render bare modular with lfo_enabled:0 to compare.
	noVib := RecipeParams{
		"osc_type":           0,
		"osc_enabled":        1,
		"filter_type":        0,
		"filter_cutoff":      4000,
		"amp_attack":         0.06,
		"amp_decay":          0.1,
		"amp_sustain":        0.80,
		"amp_release":        0.12,
		"lfo_enabled":        0, // vibrato OFF
		"lfo_rate":           5.5,
		"lfo_depth":          0.10,
		"lfo_target":         1,
		"gen1_source":        2,
		"gen1_gain":          0.10,
		"gen1_filt_type":     5,
		"gen1_filt_freq":     2800,
		"gen1_filt_q":        0.7,
		"gen1_env_fast_rate": 0.5,
		"gain":               0.9,
	}
	noVibBuf := make([]float32, 48000)
	renderModularP(noVibBuf, 48000, 48000, recipeParamsToModular(noVib))
	diff := 0
	for i := range buf {
		if buf[i] != noVibBuf[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("flute: vibrato active output identical to no-vibrato — lfo_target not routed")
	}
}

// ---------------------------------------------------------------------------
// Task 5: Brass
// ---------------------------------------------------------------------------

func TestSeed_Brass_Audible(t *testing.T) {
	for _, id := range []string{"trumpet", "trumpet-mellow", "french-horn", "french-horn-loud"} {
		buf := renderSeedInstrument(t, id, 48000)
		assertAudible(t, id, buf)
	}
}

func TestSeed_Brass_TrumpetBrighterThanHorn(t *testing.T) {
	// Trumpet (band-pass formant) is spectrally brighter than the mellow
	// french-horn (low-pass): more high-frequency energy → a larger
	// sample-to-sample slope Σ|buf[i]-buf[i-1]| RELATIVE TO its amplitude.
	//
	// Brightness must be measured independently of loudness: the raw total
	// variation Σ|Δ| scales with both frequency content AND signal level, so a
	// louder-but-darker seed can beat a quieter-but-brighter one on the raw
	// sum. We normalize each seed's total variation by its own rectified
	// amplitude (Σ|buf|) — a standard amplitude-invariant brightness proxy
	// (mean slope per unit level) — so the comparison reflects timbre, not gain.
	trumpet := renderSeedInstrument(t, "trumpet", 48000)
	horn := renderSeedInstrument(t, "french-horn", 48000)
	brightness := func(buf []float32) float64 {
		var tv, sumAbs float64
		for i := range buf {
			a := buf[i]
			if a < 0 {
				a = -a
			}
			sumAbs += float64(a)
			if i > 0 {
				d := float64(buf[i] - buf[i-1])
				if d < 0 {
					d = -d
				}
				tv += d
			}
		}
		if sumAbs == 0 {
			return 0
		}
		return tv / sumAbs
	}
	bTrumpet, bHorn := brightness(trumpet), brightness(horn)
	if bTrumpet <= bHorn {
		t.Fatalf("expected trumpet (brighter, slope/amp=%v) > french-horn (slope/amp=%v)", bTrumpet, bHorn)
	}
}

// ---------------------------------------------------------------------------
// Bass guitar (renamed from synth-bass) + synth bass family
// ---------------------------------------------------------------------------

func TestSynthBassAudible(t *testing.T) {
	for _, id := range []string{"bass-guitar", "bass-acid", "bass-reese", "bass-fm", "bass-808"} {
		buf := renderSeedInstrument(t, id, 48000*2) // 2s at 48kHz
		assertAudible(t, id, buf)
	}
}

func TestSynthBassCategory(t *testing.T) {
	for _, id := range []string{"bass-guitar", "bass-acid", "bass-reese", "bass-fm", "bass-808"} {
		if got := synthCategory(id); got != "Bass (Synth)" {
			t.Fatalf("synthCategory(%s)=%q, want Bass (Synth)", id, got)
		}
	}
}

func TestSynthBassDistinctFromDefault(t *testing.T) {
	def := make([]float32, 48000)
	renderModularP(def, 48000, 48000, defaultModularParams())
	for _, id := range []string{"bass-guitar", "bass-acid", "bass-reese", "bass-fm", "bass-808"} {
		buf := renderSeedInstrument(t, id, 48000)
		diff := 0
		for i := range buf {
			if buf[i] != def[i] {
				diff++
			}
		}
		if diff == 0 {
			t.Fatalf("%s identical to bare modular default — seed not applied", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 7: Modal conga
// ---------------------------------------------------------------------------

func TestSeed_Conga_Audible(t *testing.T) {
	buf := renderSeedInstrument(t, "conga", 48000)
	assertAudible(t, "conga", buf)
}

func TestSeed_Conga_Finite(t *testing.T) {
	buf := renderSeedInstrument(t, "conga", 48000)
	for i, v := range buf {
		if v != v || v > 8 || v < -8 {
			t.Fatalf("conga: non-finite/exploded sample at index %d: %v", i, v)
		}
	}
}

func TestSeed_Conga_Decays(t *testing.T) {
	// Second-half RMS must be less than first-half RMS — it's a percussive hit.
	buf := renderSeedInstrument(t, "conga", 48000) // 1s at 48kHz
	half := len(buf) / 2
	var rmsFirst, rmsTail float64
	for i := 0; i < half; i++ {
		v := float64(buf[i])
		rmsFirst += v * v
	}
	for i := half; i < len(buf); i++ {
		v := float64(buf[i])
		rmsTail += v * v
	}
	rmsFirst /= float64(half)
	rmsTail /= float64(len(buf) - half)
	if rmsTail >= rmsFirst {
		t.Fatalf("conga: tail RMS (%v) >= first-half RMS (%v) — expected percussive decay", rmsTail, rmsFirst)
	}
}

func TestSeed_Conga_Category(t *testing.T) {
	if got := synthCategory("conga"); got != "Percussion (Synth)" {
		t.Fatalf("synthCategory(conga)=%q, want Percussion (Synth)", got)
	}
}

func TestSeed_Conga_DistinctFromDefault(t *testing.T) {
	def := make([]float32, 48000)
	renderModularP(def, 48000, 48000, defaultModularParams())
	buf := renderSeedInstrument(t, "conga", 48000)
	diff := 0
	for i := range buf {
		if buf[i] != def[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("conga: output identical to bare modular default — seed not applied")
	}
}

// TestRideDurationLongerThanHihat verifies the ride config DurationSec (2.5s)
// is longer than the hihat's (0.25s), so the ride ring is not truncated early.
// The ride and hihat are also both audible at the start of their render.
func TestRideDurationLongerThanHihat(t *testing.T) {
	rideCfg := ConfigForInstrument("ride")
	hihatCfg := ConfigForInstrument("hihat")
	if rideCfg.DurationSec <= hihatCfg.DurationSec {
		t.Fatalf("ride DurationSec (%.2f) should be > hihat DurationSec (%.2f)", rideCfg.DurationSec, hihatCfg.DurationSec)
	}

	// Both should be audible (non-silent) at the start.
	const sr = 48000
	rideSamples := int(rideCfg.DurationSec * float64(sr))
	rideBuf := make([]float32, rideSamples)
	renderRideVoice(rideBuf, sr, rideSamples)
	assertAudible(t, "ride", rideBuf[:sr/4]) // first 0.25s

	hihatSamples := int(hihatCfg.DurationSec * float64(sr))
	hihatBuf := make([]float32, hihatSamples)
	renderHiHatVoice(hihatBuf, sr, hihatSamples)
	assertAudible(t, "hihat", hihatBuf)

	t.Logf("ride DurationSec=%.2f > hihat DurationSec=%.2f", rideCfg.DurationSec, hihatCfg.DurationSec)
}
