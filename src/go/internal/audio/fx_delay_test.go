package audio

import (
	"math"
	"testing"
)

func TestDelayTiming(t *testing.T) {
	sr := 44100
	delayMs := 100.0
	d := newDelay(sr, map[string]float64{"time": delayMs, "feedback": 0, "mix": 1})
	delaySamples := int(math.Round(delayMs * 0.001 * float64(sr)))

	// Send an impulse and scan for the echo within a window around the expected delay.
	d.ProcessSample(1.0)
	var maxOut float64
	maxIdx := 0
	for i := 1; i <= delaySamples+10; i++ {
		out := d.ProcessSample(0)
		if math.Abs(out) > maxOut {
			maxOut = math.Abs(out)
			maxIdx = i
		}
	}
	// The impulse should appear near the expected delay time.
	if maxOut < 0.3 {
		t.Errorf("delay impulse not found: maxOut=%.4f at sample %d (expected near %d)", maxOut, maxIdx, delaySamples)
	}
	if maxIdx < delaySamples-5 || maxIdx > delaySamples+5 {
		t.Errorf("delay impulse at wrong time: sample %d (expected ~%d)", maxIdx, delaySamples)
	}
}

func TestDelayFeedback(t *testing.T) {
	sr := 44100
	delayMs := 50.0
	d := newDelay(sr, map[string]float64{"time": delayMs, "feedback": 0.5, "mix": 1})
	delaySamples := int(math.Round(delayMs * 0.001 * float64(sr)))

	// Send impulse.
	d.ProcessSample(1.0)

	// Scan for the first echo.
	var firstEcho float64
	for i := 1; i <= delaySamples+5; i++ {
		out := d.ProcessSample(0)
		if math.Abs(out) > math.Abs(firstEcho) {
			firstEcho = out
		}
	}

	// Scan for the second echo.
	var secondEcho float64
	for i := 0; i < delaySamples+5; i++ {
		out := d.ProcessSample(0)
		if math.Abs(out) > math.Abs(secondEcho) {
			secondEcho = out
		}
	}

	if math.Abs(firstEcho) < 0.1 {
		t.Fatalf("first echo too quiet: %.4f", firstEcho)
	}
	// Second echo should be smaller than first due to feedback + LP filtering.
	if math.Abs(secondEcho) >= math.Abs(firstEcho) {
		t.Errorf("second echo should be quieter: first=%.4f second=%.4f", firstEcho, secondEcho)
	}
	// Second echo should be non-zero (feedback is working).
	if math.Abs(secondEcho) < 0.01 {
		t.Errorf("second echo too quiet — feedback not working: %.4f", secondEcho)
	}
}

func TestDelayMix(t *testing.T) {
	d := newDelay(44100, map[string]float64{"time": 100, "feedback": 0, "mix": 0})
	// Mix=0 means fully dry.
	for i := 0; i < 500; i++ {
		d.ProcessSample(0.5)
	}
	out := d.ProcessSample(0.5)
	if math.Abs(out-0.5) > 0.01 {
		t.Errorf("mix=0: expected dry signal 0.5, got %.4f", out)
	}
}

func TestDelayReset(t *testing.T) {
	d := newDelay(44100, map[string]float64{"time": 50, "feedback": 0.9, "mix": 1})
	// Fill the delay buffer.
	for i := 0; i < 5000; i++ {
		d.ProcessSample(0.8)
	}
	d.Reset()
	// After reset, buffer should be empty — no echoes.
	out := d.ProcessSample(0)
	if math.Abs(out) > 0.001 {
		t.Errorf("after reset: expected silence, got %.4f", out)
	}
}

func TestDelaySetParamTimeRecalc(t *testing.T) {
	d := newDelay(44100, map[string]float64{"time": 100, "feedback": 0.5, "mix": 1})
	// Send impulse and process through initial delay time.
	d.ProcessSample(1.0)
	for i := 0; i < 5000; i++ {
		d.ProcessSample(0)
	}
	// Change delay time.
	d.SetParam("time", 500)
	d.Reset()
	// Send another impulse with new delay time.
	d.ProcessSample(1.0)
	newDelaySamples := int(math.Round(500 * 0.001 * 44100))
	var maxOut float64
	maxIdx := 0
	for i := 1; i <= newDelaySamples+10; i++ {
		out := d.ProcessSample(0)
		if math.Abs(out) > maxOut {
			maxOut = math.Abs(out)
			maxIdx = i
		}
	}
	// The impulse should appear near the new delay time.
	if maxOut < 0.3 {
		t.Errorf("delay impulse not found after time change: maxOut=%.4f", maxOut)
	}
	if maxIdx < newDelaySamples-5 || maxIdx > newDelaySamples+5 {
		t.Errorf("delay at wrong time after SetParam: sample %d (expected ~%d)", maxIdx, newDelaySamples)
	}
}

func TestDelayZeroSR(t *testing.T) {
	// sr=0 should fallback to 44100 in recalc.
	d := newDelay(0, map[string]float64{"time": 100, "feedback": 0.5, "mix": 1})
	out := d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
