//go:build test || js

package audio

import "math"

// tape implements a tape saturation effect with asymmetric soft-clipping,
// a one-pole warmth filter, and wow/flutter modulated delay for analog
// tape character. Drive controls saturation intensity, warmth rolls off
// highs, and wow/flutter add slow/fast pitch modulation respectively.
type tape struct {
	drive   smoothParam // 1-10: saturation amount
	warmth  smoothParam // 0-1: LP filter amount (higher = darker)
	wow     smoothParam // 0-1: slow pitch wobble depth
	flutter smoothParam // 0-1: fast pitch wobble depth
	mix     smoothParam // 0-1: wet/dry

	sr           int
	lpY1         float64   // one-pole LP filter state
	buf          []float64 // circular delay buffer
	pos          int       // write position
	wowPhase     float64   // LFO phase 0..2π (0.5 Hz)
	flutterPhase float64   // LFO phase 0..2π (6 Hz)
}

func newTape(sr int, params map[string]float64) *tape {
	t := &tape{sr: sr}
	t.drive = newSmoothParam(clampf(params["drive"], 1, 10), sr, defaultSmoothTimeMs)
	t.warmth = newSmoothParam(clampf(params["warmth"], 0, 1), sr, defaultSmoothTimeMs)
	t.wow = newSmoothParam(clampf(params["wow"], 0, 1), sr, defaultSmoothTimeMs)
	t.flutter = newSmoothParam(clampf(params["flutter"], 0, 1), sr, defaultSmoothTimeMs)
	t.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)

	// Buffer: max wow (2ms) + max flutter (0.5ms) = 2.5ms max excursion.
	// Double for safety margin plus extra headroom.
	srf := float64(sr)
	if srf <= 0 {
		srf = 44100
	}
	bufSize := int(0.005*srf) + 8
	if bufSize < 16 {
		bufSize = 16
	}
	t.buf = make([]float64, bufSize)
	return t
}

func (t *tape) ProcessSample(x float64) float64 {
	drv := t.drive.tick()
	wrm := t.warmth.tick()
	wowVal := t.wow.tick()
	flutterVal := t.flutter.tick()
	mixVal := t.mix.tick()

	srf := float64(t.sr)
	if srf <= 0 {
		srf = 44100
	}

	// 1. Saturation: asymmetric soft-clip, level-preserving.
	//    y = tanh(drive * x) / tanh(drive)
	tanhDrv := math.Tanh(drv)
	var wet float64
	if tanhDrv > 1e-12 {
		wet = math.Tanh(drv*x) / tanhDrv
	} else {
		wet = x
	}

	// 2. Warmth filter: one-pole lowpass on saturated signal.
	//    Cutoff = 2000 + (1-warmth)*18000 Hz
	cutoff := 2000.0 + (1.0-wrm)*18000.0
	a := math.Exp(-2.0 * math.Pi * cutoff / srf)
	t.lpY1 = wet*(1.0-a) + t.lpY1*a
	wet = t.lpY1

	// 3 & 4. Wow + Flutter via modulated delay line.
	if wowVal > 1e-9 || flutterVal > 1e-9 {
		bufLen := len(t.buf)

		// Write to circular buffer.
		t.buf[t.pos] = wet

		// Wow LFO: 0.5 Hz, max excursion = wow * 0.002 * sr samples.
		wowExcursion := wowVal * 0.002 * srf
		wowOffset := math.Sin(t.wowPhase) * wowExcursion

		// Flutter LFO: 6 Hz, max excursion = flutter * 0.0005 * sr samples.
		flutterExcursion := flutterVal * 0.0005 * srf
		flutterOffset := math.Sin(t.flutterPhase) * flutterExcursion

		totalOffset := wowOffset + flutterOffset

		// Read with linear interpolation.
		readF := float64(t.pos) - totalOffset
		for readF < 0 {
			readF += float64(bufLen)
		}
		idx0 := int(readF) % bufLen
		idx1 := (idx0 + 1) % bufLen
		frac := readF - math.Floor(readF)
		wet = t.buf[idx0]*(1.0-frac) + t.buf[idx1]*frac

		// Advance LFOs.
		t.wowPhase += 2.0 * math.Pi * 0.5 / srf
		if t.wowPhase >= 2.0*math.Pi {
			t.wowPhase -= 2.0 * math.Pi
		}
		t.flutterPhase += 2.0 * math.Pi * 6.0 / srf
		if t.flutterPhase >= 2.0*math.Pi {
			t.flutterPhase -= 2.0 * math.Pi
		}

		// Advance write position.
		t.pos++
		if t.pos >= bufLen {
			t.pos = 0
		}
	}

	// 6. Mix: output = input*(1-mix) + wet*mix
	return x*(1.0-mixVal) + wet*mixVal
}

func (t *tape) Reset() {
	t.lpY1 = 0
	for i := range t.buf {
		t.buf[i] = 0
	}
	t.pos = 0
	t.wowPhase = 0
	t.flutterPhase = 0
}

func (t *tape) SetParam(name string, value float64) {
	switch name {
	case "drive":
		t.drive.set(clampf(value, 1, 10))
	case "warmth":
		t.warmth.set(clampf(value, 0, 1))
	case "wow":
		t.wow.set(clampf(value, 0, 1))
	case "flutter":
		t.flutter.set(clampf(value, 0, 1))
	case "mix":
		t.mix.set(clampf(value, 0, 1))
	}
}
