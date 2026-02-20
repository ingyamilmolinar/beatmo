//go:build test || js

package audio

import "math"

// transientShaper implements a dual-envelope transient detection algorithm that
// independently controls the gain of the attack (transient) and sustain portions
// of a signal. A fast envelope tracks rapid changes (transients) while a slow
// envelope captures the overall sustain level. The difference between the two
// determines how much of the attack vs sustain gain to apply.
//
// Parameters:
//   - attack:  0-200 (percentage mapped to 0-2 gain; 100 = unity)
//   - sustain: 0-200 (percentage mapped to 0-2 gain; 100 = unity)
//   - speed:   1-50 ms (controls fast envelope time constant)
type transientShaper struct {
	attack  smoothParam // 0-200 percentage -> 0-2 gain
	sustain smoothParam // 0-200 percentage -> 0-2 gain
	speed   smoothParam // 1-50 ms

	sr              int
	fastEnv         float64 // fast envelope state
	slowEnv         float64 // slow envelope state
	fastCoef        float64 // fast attack coefficient
	fastReleaseCoef float64 // fast release coefficient
	slowCoef        float64 // slow (fixed 100ms) coefficient
}

func newTransient(sr int, params map[string]float64) *transientShaper {
	t := &transientShaper{sr: sr}

	attackVal := params["attack"]
	if attackVal == 0 {
		attackVal = 100
	}
	sustainVal := params["sustain"]
	if sustainVal == 0 {
		sustainVal = 100
	}
	speedVal := params["speed"]
	if speedVal == 0 {
		speedVal = 10
	}

	t.attack = newSmoothParam(clampf(attackVal, 0, 200), sr, defaultSmoothTimeMs)
	t.sustain = newSmoothParam(clampf(sustainVal, 0, 200), sr, defaultSmoothTimeMs)
	t.speed = newSmoothParam(clampf(speedVal, 1, 50), sr, defaultSmoothTimeMs)
	t.recalcCoefs()
	return t
}

func (t *transientShaper) recalcCoefs() {
	sr := float64(t.sr)
	if sr <= 0 {
		sr = 44100
	}
	speedMs := t.speed.value()
	if speedMs <= 0 {
		speedMs = 10
	}
	// Fast envelope attack: time constant = speed_ms
	t.fastCoef = math.Exp(-1.0 / (speedMs * 0.001 * sr))
	// Fast envelope release: slightly slower (3x the attack time)
	t.fastReleaseCoef = math.Exp(-1.0 / (speedMs * 0.003 * sr))
	// Slow envelope: fixed 100ms time constant
	t.slowCoef = math.Exp(-1.0 / (0.1 * sr))
}

func (t *transientShaper) ProcessSample(x float64) float64 {
	attackPct := t.attack.tick()
	sustainPct := t.sustain.tick()
	speedVal := t.speed.tick()

	// Recompute coefficients when speed changes.
	sr := float64(t.sr)
	if sr <= 0 {
		sr = 44100
	}
	if speedVal <= 0 {
		speedVal = 10
	}
	t.fastCoef = math.Exp(-1.0 / (speedVal * 0.001 * sr))
	t.fastReleaseCoef = math.Exp(-1.0 / (speedVal * 0.003 * sr))

	absX := math.Abs(x)

	// Fast envelope: fast attack, moderate release.
	if absX > t.fastEnv {
		t.fastEnv = t.fastCoef*t.fastEnv + (1-t.fastCoef)*absX
	} else {
		t.fastEnv = t.fastReleaseCoef * t.fastEnv
	}

	// Slow envelope: tracks sustain level.
	t.slowEnv = t.slowCoef*t.slowEnv + (1-t.slowCoef)*absX

	// Transient amount: how much the fast envelope exceeds the slow.
	denom := t.slowEnv
	if denom < 1e-10 {
		denom = 1e-10
	}
	transientAmount := clampf((t.fastEnv-t.slowEnv)/denom, 0, 1)

	// Map percentage (0-200) to gain (0-2).
	attackGain := attackPct / 100.0
	sustainGain := sustainPct / 100.0

	// Blend attack and sustain gains based on transient detection.
	gain := attackGain*transientAmount + sustainGain*(1-transientAmount)

	return x * gain
}

func (t *transientShaper) Reset() {
	t.fastEnv = 0
	t.slowEnv = 0
}

func (t *transientShaper) SetParam(name string, value float64) {
	switch name {
	case "attack":
		t.attack.set(clampf(value, 0, 200))
	case "sustain":
		t.sustain.set(clampf(value, 0, 200))
	case "speed":
		t.speed.set(clampf(value, 1, 50))
	}
}
