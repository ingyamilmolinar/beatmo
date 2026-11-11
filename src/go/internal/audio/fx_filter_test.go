package audio

import (
	"math"
	"testing"
)

func TestFilterLowpass(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 0, "cutoff": 200, "q": 0.707, "mix": 1})
	// Feed a high-frequency signal (5 kHz sine) through a 200 Hz lowpass.
	freq := 5000.0
	var energy float64
	for i := 0; i < 4410; i++ { // 100ms
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy += out * out
	}
	// The 5 kHz signal should be heavily attenuated by the 200 Hz LP filter.
	avgPower := energy / 4410
	if avgPower > 0.05 {
		t.Errorf("LP filter should attenuate 5kHz signal, avg power=%.6f", avgPower)
	}
}

func TestFilterHighpass(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 1, "cutoff": 5000, "q": 0.707, "mix": 1})
	// Feed a low-frequency signal (100 Hz sine) through a 5 kHz highpass.
	freq := 100.0
	var energy float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy += out * out
	}
	avgPower := energy / 4410
	if avgPower > 0.05 {
		t.Errorf("HP filter should attenuate 100Hz signal, avg power=%.6f", avgPower)
	}
}

func TestFilterBandpass(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 2, "cutoff": 1000, "q": 5, "mix": 1})
	// Feed a 1 kHz sine — it should pass through a 1 kHz bandpass with Q=5.
	freq := 1000.0
	var energy float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy += out * out
	}
	avgPower := energy / 4410
	// Should pass mostly through.
	if avgPower < 0.1 {
		t.Errorf("BP filter should pass 1kHz signal, avg power=%.6f", avgPower)
	}
}

func TestFilterMix(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 0, "cutoff": 200, "q": 0.707, "mix": 0})
	for i := 0; i < 500; i++ {
		f.ProcessSample(0.5)
	}
	out := f.ProcessSample(0.5)
	if math.Abs(out-0.5) > 0.001 {
		t.Errorf("mix=0: expected 0.5, got %.4f", out)
	}
}

func TestFilterReset(t *testing.T) {
	f := newFilter(44100, DefaultParams(EffectFilter))
	for i := 0; i < 5000; i++ {
		f.ProcessSample(0.8)
	}
	f.Reset()
	out := f.ProcessSample(0)
	if math.Abs(out) > 0.01 {
		t.Errorf("after reset: expected ~0, got %.4f", out)
	}
}

func TestFilterSetParam(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 0, "cutoff": 200, "q": 0.707, "mix": 1})
	// Switch to highpass mode.
	f.SetParam("mode", 1)
	f.SetParam("cutoff", 5000)
	// Feed a low-frequency signal — should be attenuated after mode switch.
	freq := 100.0
	var energy float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy += out * out
	}
	avgPower := energy / 4410
	if avgPower > 0.1 {
		t.Errorf("after switching to HP 5kHz: 100Hz avg power should be low, got %.6f", avgPower)
	}
}

func TestFilterSetParamModeRebuilds(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 0, "cutoff": 1000, "q": 0.707, "mix": 1})
	// LP mode: 5kHz signal should be attenuated through 1kHz LP.
	freq := 5000.0
	var energyLP float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energyLP += out * out
	}

	// Switch to HP mode: same 5kHz signal should pass through 1kHz HP.
	f.SetParam("mode", 1)
	f.Reset()
	var energyHP float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energyHP += out * out
	}

	// HP should pass more 5kHz energy than LP.
	if energyHP <= energyLP {
		t.Errorf("HP should pass more 5kHz than LP: LP=%.4f HP=%.4f", energyLP, energyHP)
	}
}

func TestFilterCutoffQRebuild(t *testing.T) {
	f := newFilter(44100, map[string]float64{"mode": 0, "cutoff": 200, "q": 0.707, "mix": 1})
	// 5kHz through 200Hz LP: heavily attenuated.
	freq := 5000.0
	var energy1 float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy1 += out * out
	}

	// Raise cutoff to 10kHz: 5kHz should pass.
	f.SetParam("cutoff", 10000)
	f.Reset()
	var energy2 float64
	for i := 0; i < 4410; i++ {
		in := math.Sin(2 * math.Pi * freq * float64(i) / 44100)
		out := f.ProcessSample(in)
		energy2 += out * out
	}

	if energy2 <= energy1 {
		t.Errorf("higher cutoff should pass more energy: low=%.4f high=%.4f", energy1, energy2)
	}

	// Q change should also rebuild without panic.
	f.SetParam("q", 5)
	out := f.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output after Q change, got %f", out)
	}
}
