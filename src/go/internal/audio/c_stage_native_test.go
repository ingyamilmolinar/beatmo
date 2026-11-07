//go:build !test && !js

package audio

import (
	"math"
	"slices"
	"testing"
)

// The C stages after the voice are the one implementation both platforms run, desktop in 64-frame blocks (or
// whatever an oto Read hands over) and the browser worklet in 128-frame quanta.

// A chirp, a noise burst, decaying hits, silence and a full-scale square.
func stageSignal(sr int) []float32 {
	n := sr
	out := make([]float32, n)
	noise := noiseBurst(n)
	for i := range out {
		t := float64(i) / float64(sr)
		var v float64
		switch {
		case t < 0.3:
			f := 40 * math.Pow(400, t/0.3)
			v = 0.6 * math.Sin(2*math.Pi*f*t)
		case t < 0.45:
			v = 0.5 * float64(noise[i])
		case t < 0.7:
			k := math.Mod(t-0.45, 0.05)
			v = 0.9 * math.Exp(-k*80) * math.Sin(2*math.Pi*220*k)
		case t < 0.8:
			v = 0
		default:
			v = 0.95
			if math.Mod(t, 0.004) < 0.002 {
				v = -0.95
			}
		}
		out[i] = float32(v)
	}
	return out
}

// Defaults, every param at its minimum, its maximum and its midpoint.
func effectParamSets(t EffectType) map[string]map[string]float64 {
	reg := EffectRegistrations()[t]
	sets := map[string]map[string]float64{"default": DefaultParams(t), "min": {}, "max": {}, "mid": {}}
	for _, p := range reg.Params {
		sets["min"][p.Name] = p.Min
		sets["max"][p.Name] = p.Max
		sets["mid"][p.Name] = (p.Min + p.Max) / 2
	}
	return sets
}

func runInBlocks(proc BlockProcessor, in []float32, block int) []float32 {
	out := make([]float32, len(in))
	for off := 0; off < len(in); off += block {
		n := min(block, len(in)-off)
		proc.ProcessBlockBuf(in[off:off+n], out[off:off+n], n)
	}
	return out
}

// So desktop's blocks (or an odd oto Read) and the browser's quanta compute the same samples.
func TestEveryEffectIsBlockSizeInvariant(t *testing.T) {
	const sr = EngineSampleRate
	in := stageSignal(sr)
	for _, et := range EffectTypes {
		for name, params := range effectParamSets(et) {
			ref := runInBlocks(newCEffect(et, sr, params), in, len(in))
			for _, block := range []int{1, 64, 97, 128} {
				got := runInBlocks(newCEffect(et, sr, params), in, block)
				for i := range ref {
					if got[i] != ref[i] && !(got[i] != got[i] && ref[i] != ref[i]) {
						t.Errorf("%s/%s: block %d differs at sample %d: %g vs %g (one pass)", et, name, block, i, got[i], ref[i])
						break
					}
				}
			}
		}
	}
}

// The browser orders values by the same C list.
func TestEffectParamNamesMatchTheRegistry(t *testing.T) {
	for _, et := range EffectTypes {
		var reg []string
		for _, p := range EffectRegistrations()[et].Params {
			reg = append(reg, p.Name)
		}
		c := append([]string(nil), effectParamNames(et)...)
		slices.Sort(reg)
		slices.Sort(c)
		if !slices.Equal(reg, c) {
			t.Errorf("%s: registry params %v, C init params %v", et, reg, c)
		}
	}
}

// Unknown type here; on the browser's fixed heap, memory running out. Never silence, never NaN.
func TestEffectCreateFailurePassesThrough(t *testing.T) {
	e := newCEffect(EffectType("no-such-effect"), EngineSampleRate, nil)
	in := stageSignal(EngineSampleRate)[:512]
	out := runInBlocks(e, in, 64)
	if !slices.Equal(in, out) {
		t.Fatal("a refused effect must pass its input through unchanged")
	}
}

// The delay line keeps its echoes; the browser applies the same ifx_set to its live instance.
func TestSetKeepsState(t *testing.T) {
	const sr = EngineSampleRate
	in := stageSignal(sr)
	p := map[string]float64{"time": 120, "feedback": 0.6, "mix": 0.5}
	live := newCEffect(EffectDelay, sr, p)
	runInBlocks(live, in[:sr/2], 64)
	live.SetParam("mix", 0.8)
	tailLive := runInBlocks(live, make([]float32, sr/4), 64) // silence in: only echoes out
	var energy float64
	for _, x := range tailLive {
		energy += float64(x) * float64(x)
	}
	if energy == 0 {
		t.Fatal("after a param edit the delay lost the echoes it held: its state was reset")
	}
}

func eqRun(prog []float64, in []float32, block int) []float32 {
	return runInBlocks(newCEQ(prog), in, block)
}

// TestEQIsBlockSizeInvariant covers the chain, the band split and silence.
func TestEQIsBlockSizeInvariant(t *testing.T) {
	const sr = EngineSampleRate
	in := stageSignal(sr)
	for name, bands := range eqCases() {
		prog := BuildEQProgram(sr, bands)
		ref := eqRun(prog, in, len(in))
		for _, block := range []int{1, 64, 97, 128, 300} {
			got := eqRun(prog, in, block)
			if !slices.Equal(got, ref) {
				t.Errorf("%s: block %d output differs from one pass", name, block)
			}
		}
	}
}

// The shapes buildFullEQBands can produce.
func eqCases() map[string][]EQBand {
	graphic := func(gains []float64, muted map[int]bool) []EQBand {
		var out []EQBand
		for i, d := range defaultBandDefs {
			k := EQPeaking
			if i == 0 {
				k = EQLowShelf
			}
			if i == len(defaultBandDefs)-1 {
				k = EQHighShelf
			}
			out = append(out, EQBand{Kind: k, Freq: math.Sqrt(d.loHz * d.hiHz), Q: 1.414, GainDB: gains[i], Muted: muted[i]})
		}
		return out
	}
	flat := make([]float64, 10)
	shaped := []float64{12, -6, 3, 0, -12, 6, 0, -3, 9, -12}
	hpf := EQBand{Kind: EQHighpass, Freq: 180, Q: 0.707}
	lpf := EQBand{Kind: EQLowpass, Freq: 9000, Q: 0.707}
	all := map[int]bool{}
	for i := range 10 {
		all[i] = true
	}
	return map[string][]EQBand{
		"flat":           graphic(flat, nil),
		"shaped":         graphic(shaped, nil),
		"shaped+filters": append(append([]EQBand{hpf}, graphic(shaped, nil)...), lpf),
		"muted-band":     graphic(shaped, map[int]bool{3: true}),
		"muted+filters":  append(append([]EQBand{hpf}, graphic(shaped, map[int]bool{3: true, 7: true})...), lpf),
		"all-muted":      graphic(shaped, all),
	}
}

// With a band muted the graphic section becomes a band split, and the high/low-pass stay in series around it rather
// than counting as bands of the split.
func TestHPFAndMutedBandKeepEveryFilter(t *testing.T) {
	prog := BuildEQProgram(EngineSampleRate, eqCases()["muted+filters"])
	if prog[0] != 3 {
		t.Fatalf("want 3 stages (high-pass, band split, low-pass), got %v", prog[0])
	}
	if prog[1] != eqStageBiquad || prog[7] != eqStageSplit || prog[8] != 10 {
		t.Fatalf("stage layout %v, want biquad then a 10-band split", prog[:9])
	}
	if prog[len(prog)-6] != eqStageBiquad {
		t.Fatal("the low-pass must be the last stage")
	}
	if p := BuildEQProgram(EngineSampleRate, eqCases()["all-muted"]); p[0] != 1 || p[1] != eqStageSilence {
		t.Fatalf("all bands muted must be one silence stage, got %v", p)
	}
}

// A steady sine comes out at |H(f)| within 0.05 dB.
func TestEQMatchesItsResponse(t *testing.T) {
	const sr = EngineSampleRate
	for _, name := range []string{"flat", "shaped", "shaped+filters"} {
		bands := eqCases()[name]
		for _, f := range []float64{40, 125, 500, 2000, 6000, 14000} {
			want := ComputeFreqResponse(sr, bands, 2, f, f*1.0000001)[0].GainDB
			n := sr
			in := make([]float32, n)
			for i := range in {
				in[i] = float32(0.25 * math.Sin(2*math.Pi*f*float64(i)/sr))
			}
			out := eqRun(BuildEQProgram(sr, bands), in, 128)
			got := 20 * math.Log10(rms32(out[n/2:])/rms32(in[n/2:]))
			if math.Abs(got-want) > 0.05 {
				t.Errorf("%s at %.0f Hz: %.3f dB, response says %.3f dB", name, f, got, want)
			}
		}
	}
}

func rms32(x []float32) float64 {
	var s float64
	for _, v := range x {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(len(x)))
}

// Same shape, new gain retunes the running filters; a new shape (mute) is refused, so a fresh EQ is built.
func TestEQEditKeepsFilterState(t *testing.T) {
	const sr = EngineSampleRate
	in := stageSignal(sr)
	shaped := eqCases()["shaped"]
	live := newCEQ(BuildEQProgram(sr, shaped))
	runInBlocks(live, in[:sr/2], 128)
	moved := slices.Clone(shaped)
	moved[2].GainDB = -9
	if !live.update(BuildEQProgram(sr, moved)) {
		t.Fatal("a gain move must retune the live EQ in place")
	}
	cont := runInBlocks(live, in[sr/2:sr/2+256], 128)
	fresh := eqRun(BuildEQProgram(sr, moved), in[sr/2:sr/2+256], 128)
	if slices.Equal(cont, fresh) {
		t.Fatal("the retuned EQ restarted from silence: its filter state was dropped")
	}
	if live.update(BuildEQProgram(sr, eqCases()["muted-band"])) {
		t.Fatal("a new topology (a muted band) must not be applied in place")
	}
}

// Identity to the knee, no jump there, never past full scale, odd, and NaN never reaches the device.
func TestSoftClipIsContinuousAndBounded(t *testing.T) {
	const h = 1e-4
	prev := -8.0
	buf := []float64{-8}
	softClipBlock(buf)
	prevY := buf[0]
	for x := -8 + h; x <= 8; x += h {
		b := []float64{x, -x}
		softClipBlock(b)
		y := b[0]
		if math.Abs(y) > 1 {
			t.Fatalf("soft clip(%v) = %v is past full scale", x, y)
		}
		if y-prevY < -1e-15 || y-prevY > (x-prev)*1.000001 {
			t.Fatalf("soft clip jumps between %v and %v: %v → %v", prev, x, prevY, y)
		}
		if b[1] != -y {
			t.Fatalf("soft clip is not odd at %v", x)
		}
		if math.Abs(x) <= SoftClipThreshold && y != x {
			t.Fatalf("soft clip(%v) = %v below the knee", x, y)
		}
		prev, prevY = x, y
	}
	nan := []float64{math.NaN()}
	softClipBlock(nan)
	if nan[0] != 0 {
		t.Fatalf("soft clip(NaN) = %v, want 0", nan[0])
	}
}
