//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Native coverage for the standalone DSP utility modules (adsr.c,
// wavetable.c, pan.c) through the dsp_units_c.go bridge. Before these tests
// the synth engines only reached the units indirectly (modular.c drives
// wt_osc_tick inline; fmsynth.c reads generated tables), leaving the block
// APIs (adsr_process, wt_osc_process[_fm]) and all of pan.c unexecuted —
// pan.c previously produced NO DATA in `make coverage-c` because nothing
// even pulled it out of libdrums.a.

// ── ADSR ────────────────────────────────────────────────────────────────────

func TestADSRLinearFullEnvelope(t *testing.T) {
	const sr = 44100
	env := newCADSR(sr, 0.001, 0.002, 0.5, 0.002, false)

	if got := env.Tick(); got != 0 {
		t.Fatalf("idle tick = %v, want 0", got)
	}

	env.Trigger()
	// Attack: must reach 1.0 within attack_sec plus slack.
	attackSamples := sr/1000 + 8 // 1 ms of samples plus slack
	peak := float32(0)
	for i := 0; i < attackSamples; i++ {
		v := env.Tick()
		if v > peak {
			peak = v
		}
	}
	if peak < 0.999 {
		t.Fatalf("attack peak = %v, want >= 0.999", peak)
	}

	// Decay: settle to sustain level.
	for i := 0; i < sr*2/1000+8; i++ {
		env.Tick()
	}
	if v := env.Tick(); math.Abs(float64(v)-0.5) > 0.01 {
		t.Fatalf("sustain value = %v, want ~0.5", v)
	}

	// Release: decay to 0 and return to idle.
	env.Release()
	for i := 0; i < sr*2/1000+64; i++ {
		env.Tick()
	}
	if v := env.Tick(); v != 0 {
		t.Fatalf("post-release tick = %v, want 0", v)
	}
}

func TestADSRExponentialFullEnvelope(t *testing.T) {
	const sr = 44100
	env := newCADSR(sr, 0.001, 0.002, 0.5, 0.002, true)
	env.Trigger()

	peak := float32(0)
	for i := 0; i < sr/100; i++ { // 10ms — plenty for a 1ms exp attack
		v := env.Tick()
		if v > peak {
			peak = v
		}
		if v < 0 || v > 1 {
			t.Fatalf("envelope out of range at %d: %v", i, v)
		}
	}
	if peak < 0.999 {
		t.Fatalf("exp attack peak = %v, want >= 0.999", peak)
	}

	env.Release()
	for i := 0; i < sr/100; i++ {
		env.Tick()
	}
	if v := env.Tick(); v != 0 {
		t.Fatalf("exp post-release tick = %v, want 0", v)
	}
}

func TestADSRProcessBlockMatchesTick(t *testing.T) {
	const sr = 44100
	blockEnv := newCADSR(sr, 0.002, 0.005, 0.6, 0.01, false)
	tickEnv := newCADSR(sr, 0.002, 0.005, 0.6, 0.01, false)
	blockEnv.Trigger()
	tickEnv.Trigger()

	block := make([]float32, 512)
	blockEnv.Process(block)
	for i, want := range block {
		if got := tickEnv.Tick(); got != want {
			t.Fatalf("block[%d] = %v, tick = %v — adsr_process must be tick-for-tick identical", i, want, got)
		}
	}

	// Zero-length block is a no-op.
	blockEnv.Process(nil)
}

func TestADSRResetReturnsToIdle(t *testing.T) {
	env := newCADSR(44100, 0.1, 0.1, 0.8, 0.1, false)
	env.Trigger()
	for i := 0; i < 100; i++ {
		env.Tick()
	}
	if env.Value() == 0 {
		t.Fatal("envelope should be mid-attack before reset")
	}
	env.Reset()
	if env.Value() != 0 || env.Stage() != 0 {
		t.Fatalf("after reset: value=%v stage=%d, want 0/ADSR_IDLE", env.Value(), env.Stage())
	}
	if v := env.Tick(); v != 0 {
		t.Fatalf("tick after reset = %v, want 0", v)
	}
}

func TestADSRReleaseFromIdleStaysIdle(t *testing.T) {
	env := newCADSR(44100, 0.01, 0.01, 0.5, 0.01, false)
	env.Release() // never triggered
	if v := env.Tick(); v != 0 {
		t.Fatalf("release-from-idle tick = %v, want 0", v)
	}
}

// ── Wavetable ───────────────────────────────────────────────────────────────

func TestWavetableGenerationShapes(t *testing.T) {
	const length = 1024
	for _, tc := range []struct {
		name  string
		shape wtShape
	}{
		{"sine", wtSine}, {"saw", wtSaw}, {"square", wtSquare}, {"triangle", wtTriangle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newCWavetable(tc.shape, length, 16)
			defer w.Free()
			if w.Length() != length {
				t.Fatalf("length = %d, want %d", w.Length(), length)
			}
			// Guard point: table[length] == table[0].
			if w.At(length) != w.At(0) {
				t.Fatalf("guard point %v != table[0] %v", w.At(length), w.At(0))
			}
			// Non-trivial signal with sane amplitude.
			var peak float64
			for i := 0; i < length; i++ {
				v := math.Abs(float64(w.At(i)))
				if v > peak {
					peak = v
				}
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("table[%d] not finite", i)
				}
			}
			if peak < 0.5 || peak > 1.5 {
				t.Fatalf("peak = %v, want in [0.5, 1.5]", peak)
			}
		})
	}
}

func TestWtOscProcessFrequencyAccuracy(t *testing.T) {
	const (
		sr   = 44100
		freq = 441.0 // exactly 100 samples per cycle
	)
	w := newCWavetable(wtSine, 4096, 0)
	defer w.Free()
	o := newCWtOsc(w, freq, sr)
	defer o.Free()

	out := make([]float32, sr) // 1 second
	o.Process(out)

	// Count positive-going zero crossings: expect ~freq.
	crossings := 0
	for i := 1; i < len(out); i++ {
		if out[i-1] < 0 && out[i] >= 0 {
			crossings++
		}
	}
	if math.Abs(float64(crossings)-freq) > 1 {
		t.Fatalf("zero crossings = %d, want ~%v", crossings, freq)
	}
}

func TestWtOscSetFreqPreservesPhase(t *testing.T) {
	const sr = 44100
	w := newCWavetable(wtSine, 4096, 0)
	defer w.Free()
	o := newCWtOsc(w, 220, sr)
	defer o.Free()

	warm := make([]float32, 333)
	o.Process(warm)

	incBefore := o.PhaseInc()
	o.SetFreq(440, sr)
	if got := o.PhaseInc(); math.Abs(got-2*incBefore) > 1e-9 {
		t.Fatalf("phase_inc after doubling freq = %v, want %v", got, 2*incBefore)
	}

	// sr<=0 / nil-table guards: no change, no crash.
	o.SetFreq(880, 0)
	if got := o.PhaseInc(); math.Abs(got-2*incBefore) > 1e-9 {
		t.Fatalf("phase_inc changed on sr=0 set_freq: %v", got)
	}
}

func TestWtOscInitZeroSampleRate(t *testing.T) {
	w := newCWavetable(wtSine, 256, 0)
	defer w.Free()
	o := newCWtOsc(w, 440, 0) // sampleRate <= 0 → phase_inc = 0
	defer o.Free()
	if o.PhaseInc() != 0 {
		t.Fatalf("phase_inc = %v, want 0 for sr=0", o.PhaseInc())
	}
	out := make([]float32, 64)
	o.Process(out)
	// Frozen phase: every sample reads table[0].
	for i, v := range out {
		if v != w.At(0) {
			t.Fatalf("out[%d] = %v, want frozen table[0] = %v", i, v, w.At(0))
		}
	}
}

func TestWtOscProcessFM(t *testing.T) {
	const sr = 44100
	w := newCWavetable(wtSine, 4096, 0)
	defer w.Free()

	// Unmodulated FM render (freq_mod = 0) must equal plain process.
	oA := newCWtOsc(w, 441, sr)
	defer oA.Free()
	oB := newCWtOsc(w, 441, sr)
	defer oB.Free()
	plain := make([]float32, 2048)
	fmOut := make([]float32, 2048)
	zeroMod := make([]float32, 2048)
	oA.Process(plain)
	oB.ProcessFM(zeroMod, fmOut, sr)
	for i := range plain {
		if plain[i] != fmOut[i] {
			t.Fatalf("zero-mod FM diverges at %d: %v vs %v", i, fmOut[i], plain[i])
		}
	}

	// Strong negative modulation exercises the phase-wrap-below-zero branch.
	oC := newCWtOsc(w, 100, sr)
	defer oC.Free()
	negMod := make([]float32, 2048)
	for i := range negMod {
		negMod[i] = -5000
	}
	out := make([]float32, 2048)
	oC.ProcessFM(negMod, out, sr)
	for i, v := range out {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("FM out[%d] not finite", i)
		}
	}
}

// ── Send-delay init branches ────────────────────────────────────────────────

func TestSendDelayInitDampingBranches(t *testing.T) {
	const sr = 44100

	// dampingHz <= 0 → lpCoeff = 1 (no filtering). Only reachable by direct
	// init: the production path (initSendEffects) always passes 3 kHz.
	undamped := newCSendDelay(sr/10, 0.3, 0, sr)
	defer undamped.Free()
	if got := undamped.LPCoeff(); got != 1 {
		t.Fatalf("lpCoeff with damping 0 = %v, want 1", got)
	}

	damped := newCSendDelay(sr/10, 0.3, 3000, sr)
	defer damped.Free()
	if got := damped.LPCoeff(); got <= 0 || got >= 1 {
		t.Fatalf("lpCoeff with 3 kHz damping = %v, want in (0, 1)", got)
	}

	// The undamped line must still delay an impulse.
	buf := make([]float32, sr/2)
	buf[0] = 1
	undamped.Process(buf)
	var tailEnergy float64
	for _, v := range buf[1:] {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatal("delay output not finite")
		}
		tailEnergy += float64(v) * float64(v)
	}
	if tailEnergy == 0 {
		t.Fatal("delay produced no echo")
	}
}

// ── Pan ─────────────────────────────────────────────────────────────────────

func TestPanEqualPowerLaw(t *testing.T) {
	p := newCPan()

	// Center: equal gains at sin(45°) ≈ 0.7071 (equal-power, -3 dB).
	p.Set(0)
	l, r := p.Gains()
	if math.Abs(float64(l)-math.Sqrt2/2) > 1e-3 || math.Abs(float64(r)-math.Sqrt2/2) > 1e-3 {
		t.Fatalf("center gains = %v/%v, want ~0.7071 each", l, r)
	}

	// Hard left: all left, no right.
	p.Set(-1)
	l, r = p.Gains()
	if l < 0.999 || r > 1e-3 {
		t.Fatalf("hard-left gains = %v/%v, want 1/0", l, r)
	}

	// Hard right.
	p.Set(1)
	l, r = p.Gains()
	if r < 0.999 || l > 1e-3 {
		t.Fatalf("hard-right gains = %v/%v, want 0/1", l, r)
	}

	// Equal-power invariant across the sweep: l²+r² == 1.
	for pan := -1.0; pan <= 1.0; pan += 0.125 {
		p.Set(pan)
		l, r = p.Gains()
		if pw := float64(l)*float64(l) + float64(r)*float64(r); math.Abs(pw-1) > 1e-3 {
			t.Fatalf("pan %v: l²+r² = %v, want 1", pan, pw)
		}
	}
}

func TestPanProcessMonoToLR(t *testing.T) {
	p := newCPan()
	p.Set(-0.5)
	l, r := p.Gains()

	in := []float32{1, -1, 0.5, 0.25}
	left := make([]float32, len(in))
	right := make([]float32, len(in))
	// pan_process_mono_to_lr ACCUMULATES — pre-seed to verify.
	left[0], right[0] = 0.1, 0.2
	p.ProcessMonoToLR(in, left, right)

	if math.Abs(float64(left[0])-(0.1+float64(l))) > 1e-6 {
		t.Fatalf("left[0] = %v, want accumulate 0.1 + %v", left[0], l)
	}
	if math.Abs(float64(right[0])-(0.2+float64(r))) > 1e-6 {
		t.Fatalf("right[0] = %v, want accumulate 0.2 + %v", right[0], r)
	}
	for i := 1; i < len(in); i++ {
		if math.Abs(float64(left[i])-float64(in[i])*float64(l)) > 1e-6 {
			t.Fatalf("left[%d] = %v, want %v", i, left[i], in[i]*l)
		}
	}
}

func TestPanProcessMonoToInterleaved(t *testing.T) {
	p := newCPan()
	p.Set(0.5)
	l, r := p.Gains()

	in := []float32{1, -0.5}
	out := make([]float32, len(in)*2)
	p.ProcessMonoToInterleaved(in, out)

	want := []float32{l, r, -0.5 * l, -0.5 * r}
	for i := range want {
		if math.Abs(float64(out[i])-float64(want[i])) > 1e-6 {
			t.Fatalf("out[%d] = %v, want %v", i, out[i], want[i])
		}
	}
}
