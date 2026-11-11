//go:build !test && !js

package audio

import (
	"math"
	"slices"
	"testing"
)

// The C stages after the voice (fx_host.c effects, mix_dsp.c EQ and soft
// clip) are the one implementation both platforms run: desktop through cgo
// in 64-frame blocks (or whatever an oto Read hands the mixer), the browser's
// beatmo-dsp worklet in 128-frame render quanta. These tests pin the
// properties that make that true.

// stageSignal is a deterministic second of everything a stage can meet: a
// log chirp, a noise burst, decaying hits, silence, a full-scale square.
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

// effectParamSets: the defaults, every parameter at its minimum, at its
// maximum, and at the middle of its range.
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

// TestEveryEffectIsBlockSizeInvariant: an effect's output never depends on
// how the stream is cut into blocks, so desktop's 64-frame blocks (or an odd
// oto Read) and the browser's 128-frame quanta compute the same samples.
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

// TestEffectParamNamesMatchTheRegistry: every parameter the FX panel offers
// reaches the C init, and the C init reads nothing the registry does not
// define (the browser orders values by the same C list).
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

// TestEffectCreateFailurePassesThrough: an effect the C engine cannot build
// (unknown type here; on the browser's fixed heap, memory running out)
// passes its input through — never silence, never NaN.
func TestEffectCreateFailurePassesThrough(t *testing.T) {
	e := newCEffect(EffectType("no-such-effect"), EngineSampleRate, nil)
	in := stageSignal(EngineSampleRate)[:512]
	out := runInBlocks(e, in, 64)
	if !slices.Equal(in, out) {
		t.Fatal("a refused effect must pass its input through unchanged")
	}
}

// TestSetKeepsState: a live param edit retunes the running effect (its delay
// line keeps the echoes already in it) — the browser applies the same C
// ifx_set to its live instance.
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

// eqCases are the EQ shapes the UI can make (buildFullEQBands): flat, boosts
// and cuts, the shelves at their extremes, high-/low-pass, a muted band,
// muted + both filters, everything muted.
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

// TestHPFAndMutedBandKeepEveryFilter: with a band muted the graphic section
// becomes a band split, and the high-/low-pass stay in series around it (they
// used to be counted as two extra bands of the split, which dropped both and
// slid every band onto the wrong frequencies).
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

// TestEQMatchesItsResponse: the C chain is the filter the EQ panel draws —
// a steady sine comes out at |H(f)| within 0.05 dB.
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

// TestEQEditKeepsFilterState: a band drag (same shape, new gain) retunes the
// running filters; a mute toggle (new shape) is refused so a fresh EQ is
// built — on both platforms.
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

// TestSoftClipIsContinuousAndBounded: identity up to the knee, never a jump
// (the old T·tanh(x/T) curve fell from 0.9 to 0.685 at the knee), never past
// full scale, odd, and NaN never reaches the device.
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

// TestEQResetClearsEveryFilter: after Reset (audio.ResetDSPState) silence in is
// silence out, on the plain chain and the band split alike; without it the
// filters still ring.
func TestEQResetClearsEveryFilter(t *testing.T) {
	const sr = EngineSampleRate
	silence := make([]float32, 256)
	for _, name := range []string{"shaped", "muted-band"} {
		prog := BuildEQProgram(sr, eqCases()[name])
		ringing := newCEQ(prog)
		runInBlocks(ringing, stageSignal(sr)[:sr/4], 128)
		if slices.Equal(runInBlocks(ringing, silence, 128), silence) {
			t.Fatalf("%s: the EQ never rings, so this test cannot see a reset", name)
		}
		e := newCEQ(prog)
		runInBlocks(e, stageSignal(sr)[:sr/4], 128)
		e.Reset()
		if out := runInBlocks(e, silence, 128); !slices.Equal(out, silence) {
			t.Errorf("%s: after Reset a filter kept its history (first sample %g)", name, out[0])
		}
	}
}

// TestEQUpdateRefusesAMovedMute: muting a different band is a new topology,
// even with the same band count, so it must rebuild rather than retune.
func TestEQUpdateRefusesAMovedMute(t *testing.T) {
	const sr = EngineSampleRate
	bands := eqCases()["muted-band"]
	moved := slices.Clone(bands)
	from := slices.IndexFunc(moved, func(b EQBand) bool { return b.Muted })
	if from < 0 {
		t.Fatal("the muted-band case mutes no band")
	}
	to := (from + 1) % len(moved)
	moved[from].Muted, moved[to].Muted = false, true
	live := newCEQ(BuildEQProgram(sr, bands))
	if live.update(BuildEQProgram(sr, moved)) {
		t.Error("moving the mute to another band was applied in place")
	}
}

// TestEQRejectsAnUnknownStage: a program the runner does not understand
// builds no EQ, and the channel passes audio through untouched.
func TestEQRejectsAnUnknownStage(t *testing.T) {
	e := newCEQ([]float64{1, 99})
	if e.h != nil {
		t.Fatal("a program with stage kind 99 built an EQ")
	}
	if got := e.ProcessSample(0.5); got != 0.5 {
		t.Errorf("an EQ that failed to build changed the signal: 0.5 -> %g", got)
	}
}
