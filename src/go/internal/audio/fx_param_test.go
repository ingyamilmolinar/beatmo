//go:build test || js

package audio

import "testing"

// fx_param_test.go covers SetParam routing and clamp behavior on the
// Go-side FX implementations (transientShaper, waveshaper) plus
// smoothParam.snap(). These structs only exist under `test || js`,
// matching their definitions.

func TestTransientShaperSetParamRoutesToSmoothParams(t *testing.T) {
	tr := newTransient(44100, nil)

	tr.SetParam("attack", 50)
	if got := tr.attack.target; got != 50 {
		t.Errorf("attack.target=%v after SetParam(attack, 50), want 50", got)
	}
	tr.SetParam("sustain", 75)
	if got := tr.sustain.target; got != 75 {
		t.Errorf("sustain.target=%v after SetParam(sustain, 75), want 75", got)
	}
	tr.SetParam("speed", 25)
	if got := tr.speed.target; got != 25 {
		t.Errorf("speed.target=%v after SetParam(speed, 25), want 25", got)
	}
}

func TestTransientShaperSetParamClampsAndIgnoresUnknown(t *testing.T) {
	tr := newTransient(44100, nil)

	// attack/sustain clamp 0..200
	tr.SetParam("attack", 9999)
	if got := tr.attack.target; got != 200 {
		t.Errorf("attack.target=%v after SetParam(attack, 9999), want 200", got)
	}
	tr.SetParam("sustain", -10)
	if got := tr.sustain.target; got != 0 {
		t.Errorf("sustain.target=%v after SetParam(sustain, -10), want 0", got)
	}

	// speed clamps 1..50
	tr.SetParam("speed", 0)
	if got := tr.speed.target; got != 1 {
		t.Errorf("speed.target=%v after SetParam(speed, 0), want 1", got)
	}
	tr.SetParam("speed", 100)
	if got := tr.speed.target; got != 50 {
		t.Errorf("speed.target=%v after SetParam(speed, 100), want 50", got)
	}

	// Unknown param is a no-op — leave previous targets intact.
	prevAttack := tr.attack.target
	prevSustain := tr.sustain.target
	prevSpeed := tr.speed.target
	tr.SetParam("nonsense", 42)
	if tr.attack.target != prevAttack || tr.sustain.target != prevSustain || tr.speed.target != prevSpeed {
		t.Errorf("unknown param mutated state: attack=%v sustain=%v speed=%v",
			tr.attack.target, tr.sustain.target, tr.speed.target)
	}
}

func TestTransientShaperResetClearsEnvelopes(t *testing.T) {
	tr := newTransient(44100, nil)
	// Drive the envelopes nonzero by processing a sample.
	tr.ProcessSample(0.9)
	if tr.fastEnv == 0 && tr.slowEnv == 0 {
		t.Fatalf("ProcessSample did not advance envelopes; test setup wrong")
	}
	tr.Reset()
	if tr.fastEnv != 0 || tr.slowEnv != 0 {
		t.Errorf("Reset left fastEnv=%v slowEnv=%v, want both zero", tr.fastEnv, tr.slowEnv)
	}
}

func TestWaveshaperSetParamRoutesAndClamps(t *testing.T) {
	w := newWaveshaper(44100, nil)

	// curve clamps 0..3
	w.SetParam("curve", 2)
	if w.curve != 2 {
		t.Errorf("curve=%v after SetParam(curve, 2), want 2", w.curve)
	}
	w.SetParam("curve", 99)
	if w.curve != 3 {
		t.Errorf("curve=%v after SetParam(curve, 99), want 3 (upper clamp)", w.curve)
	}
	w.SetParam("curve", -1)
	if w.curve != 0 {
		t.Errorf("curve=%v after SetParam(curve, -1), want 0 (lower clamp)", w.curve)
	}

	// drive clamps 1..20
	w.SetParam("drive", 10)
	if w.drive != 10 {
		t.Errorf("drive=%v after SetParam(drive, 10), want 10", w.drive)
	}
	w.SetParam("drive", 0)
	if w.drive != 1 {
		t.Errorf("drive=%v after SetParam(drive, 0), want 1", w.drive)
	}

	// mix clamps 0..1
	w.SetParam("mix", 0.5)
	if w.mix != 0.5 {
		t.Errorf("mix=%v after SetParam(mix, 0.5), want 0.5", w.mix)
	}
	w.SetParam("mix", 5)
	if w.mix != 1 {
		t.Errorf("mix=%v after SetParam(mix, 5), want 1", w.mix)
	}

	// unknown param is no-op
	prev := *w
	w.SetParam("???", 999)
	if *w != prev {
		t.Errorf("unknown param mutated waveshaper: before=%+v after=%+v", prev, *w)
	}
}

func TestWaveshaperResetIsNoOp(t *testing.T) {
	// Waveshaper is stateless — Reset must not panic and must not change params.
	w := newWaveshaper(44100, map[string]float64{"curve": 2, "drive": 5, "mix": 0.7})
	prev := *w
	w.Reset()
	if *w != prev {
		t.Errorf("Reset mutated stateless waveshaper: before=%+v after=%+v", prev, *w)
	}
}

func TestSmoothParamSnapBypassesRamp(t *testing.T) {
	// Construct a smoother with a long smoothing time so a single tick()
	// would barely move; snap() must hop over that ramp entirely.
	p := newSmoothParam(0, 44100, 100 /*ms*/)
	p.set(1.0)

	// One tick should leave us nowhere near the target due to long smooth time.
	pre := p.tick()
	if pre > 0.5 {
		t.Fatalf("test premise broken: tick after set(1) jumped to %v in one sample", pre)
	}

	p.snap(0)
	if p.value() != 0 {
		t.Errorf("value() after snap(0)=%v, want 0", p.value())
	}
	if p.target != 0 {
		t.Errorf("target after snap(0)=%v, want 0", p.target)
	}

	// snap() must take effect on the next tick too — current is now exactly 0,
	// and target is 0, so tick() returns 0.
	if got := p.tick(); got != 0 {
		t.Errorf("tick after snap(0)=%v, want 0", got)
	}
}
