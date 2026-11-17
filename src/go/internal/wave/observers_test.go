package wave

import (
	"math"
	"testing"
)

// --- ampToDB helper ---

func TestAmpToDB_Unity(t *testing.T) {
	got := ampToDB(1.0)
	if math.Abs(got) > 1e-9 {
		t.Fatalf("ampToDB(1.0): want 0dB, got %f", got)
	}
}

func TestAmpToDB_Half(t *testing.T) {
	got := ampToDB(0.5)
	want := 20 * math.Log10(0.5) // -6.0206...
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("ampToDB(0.5): want %f, got %f", want, got)
	}
}

func TestAmpToDB_Zero(t *testing.T) {
	got := ampToDB(0.0)
	if got != -math.MaxFloat64 {
		t.Fatalf("ampToDB(0): want -MaxFloat64, got %f", got)
	}
}

// --- PeakRMS Observer ---

func TestPeakRMS_SineFullAmplitude(t *testing.T) {
	// Sine 440Hz amp=1.0, 1 second at 44100.
	// Peak amplitude = 1.0 => PeakDB ~= 0 dB.
	// RMS of a sine = amp / sqrt(2) => RMSDB ~= 20*log10(1/sqrt(2)) ~= -3.0103 dB.
	w := Sine(440, 1.0, 1.0, sr44100)
	obs := NewPeakRMSObserver()
	o := obs.Observe(w)

	if o.Kind != ObsPeakRMS {
		t.Fatalf("Kind: want ObsPeakRMS, got %v", o.Kind)
	}
	// PeakDB should be ~0 dB (allow tiny tolerance for discrete sampling).
	if math.Abs(o.PeakDB) > 0.1 {
		t.Fatalf("PeakDB: want ~0dB, got %f", o.PeakDB)
	}
	// RMSDB should be ~-3.01 dB.
	wantRMS := 20 * math.Log10(1.0/math.Sqrt(2))
	if math.Abs(o.RMSDB-wantRMS) > 0.1 {
		t.Fatalf("RMSDB: want ~%f dB, got %f", wantRMS, o.RMSDB)
	}
	if o.ClipCount != 0 {
		t.Fatalf("ClipCount: want 0, got %d", o.ClipCount)
	}
}

func TestPeakRMS_DC_Half(t *testing.T) {
	// DC at 0.5: peak = 0.5 => PeakDB = 20*log10(0.5) ~= -6.02 dB.
	// RMS of DC = amplitude => RMSDB ~= -6.02 dB.
	w := DC(0.5, 0.1, sr44100)
	obs := NewPeakRMSObserver()
	o := obs.Observe(w)

	wantDB := 20 * math.Log10(0.5)
	if math.Abs(o.PeakDB-wantDB) > 0.01 {
		t.Fatalf("PeakDB: want ~%f, got %f", wantDB, o.PeakDB)
	}
	if math.Abs(o.RMSDB-wantDB) > 0.01 {
		t.Fatalf("RMSDB: want ~%f, got %f", wantDB, o.RMSDB)
	}
}

func TestPeakRMS_Silence(t *testing.T) {
	w := Silence(0.1, sr44100)
	obs := NewPeakRMSObserver()
	o := obs.Observe(w)

	if o.PeakDB > -100 {
		t.Fatalf("PeakDB: want < -100, got %f", o.PeakDB)
	}
	if o.RMSDB > -100 {
		t.Fatalf("RMSDB: want < -100, got %f", o.RMSDB)
	}
}

func TestPeakRMS_ClippingWave(t *testing.T) {
	// Manual wave with 3 samples exceeding +/-1.0: 1.2, -1.5, 1.01.
	w := Wave{
		Samples:    []float64{0.5, 1.2, -1.5, 0.8, 1.01, -0.3},
		SampleRate: sr44100,
		Label:      "clip_test",
	}
	obs := NewPeakRMSObserver()
	o := obs.Observe(w)

	if o.ClipCount != 3 {
		t.Fatalf("ClipCount: want 3, got %d", o.ClipCount)
	}
}

func TestPeakRMS_EmptyWave(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	obs := NewPeakRMSObserver()
	o := obs.Observe(w)

	if o.PeakDB > -100 {
		t.Fatalf("PeakDB: want < -100 for empty, got %f", o.PeakDB)
	}
	if o.RMSDB > -100 {
		t.Fatalf("RMSDB: want < -100 for empty, got %f", o.RMSDB)
	}
}

// --- Envelope Observer ---

func TestEnvelope_DecayingSine(t *testing.T) {
	// Generate a decaying sine: 1.0 * exp(-10*t) * sin(2*pi*440*t)
	// for 0.5 seconds at 44100 Hz.
	dur := 0.5
	n := int(dur * float64(sr44100))
	samples := make([]float64, n)
	for i := range samples {
		tSec := float64(i) / float64(sr44100)
		samples[i] = math.Exp(-10*tSec) * math.Sin(2*math.Pi*440*tSec)
	}
	w := Wave{Samples: samples, SampleRate: sr44100, Label: "decaying_sine"}

	obs := NewEnvelopeObserver(5.0, 50.0) // attackMs=5, releaseMs=50
	o := obs.Observe(w)

	if o.Kind != ObsEnvelope {
		t.Fatalf("Kind: want ObsEnvelope, got %v", o.Kind)
	}
	if len(o.Envelope) != n {
		t.Fatalf("Envelope length: want %d, got %d", n, len(o.Envelope))
	}

	// After ~50ms of attack settling, envelope should be tracking the amplitude.
	// At t=0.05s, exp(-10*0.05) ~= 0.607. Envelope should be above 0.5.
	idx50ms := int(0.05 * float64(sr44100))
	if o.Envelope[idx50ms] < 0.3 {
		t.Fatalf("Envelope at 50ms: want > 0.3, got %f", o.Envelope[idx50ms])
	}

	// Near the end (t=0.45s), exp(-10*0.45) ~= 0.011. Envelope should be low.
	idx450ms := int(0.45 * float64(sr44100))
	if o.Envelope[idx450ms] > 0.1 {
		t.Fatalf("Envelope at 450ms: want < 0.1, got %f", o.Envelope[idx450ms])
	}
}

func TestEnvelope_Silence(t *testing.T) {
	w := Silence(0.1, sr44100)
	obs := NewEnvelopeObserver(5.0, 50.0)
	o := obs.Observe(w)

	for i, v := range o.Envelope {
		if v != 0 {
			t.Fatalf("Envelope[%d]: want 0 for silence, got %f", i, v)
		}
	}
}

func TestEnvelope_EmptyWave(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	obs := NewEnvelopeObserver(5.0, 50.0)
	o := obs.Observe(w)

	if len(o.Envelope) != 0 {
		t.Fatalf("Envelope length: want 0 for empty wave, got %d", len(o.Envelope))
	}
}

// --- ZeroCrossing Observer ---

func TestZeroCrossing_Sine440(t *testing.T) {
	// A 440 Hz sine crosses zero twice per cycle => ~880 crossings/sec.
	w := Sine(440, 1.0, 1.0, sr44100)
	obs := NewZeroCrossingObserver()
	o := obs.Observe(w)

	if o.Kind != ObsZeroCrossing {
		t.Fatalf("Kind: want ObsZeroCrossing, got %v", o.Kind)
	}

	wantRate := 880.0
	tolerance := wantRate * 0.05 // 5%
	if math.Abs(o.ZCRate-wantRate) > tolerance {
		t.Fatalf("ZCRate: want ~%f (+/- 5%%), got %f", wantRate, o.ZCRate)
	}
}

func TestZeroCrossing_DC(t *testing.T) {
	// Constant signal: zero crossings = 0.
	w := DC(0.5, 0.1, sr44100)
	obs := NewZeroCrossingObserver()
	o := obs.Observe(w)

	if o.ZeroCrossings != 0 {
		t.Fatalf("ZeroCrossings: want 0 for DC, got %d", o.ZeroCrossings)
	}
	if o.ZCRate != 0 {
		t.Fatalf("ZCRate: want 0 for DC, got %f", o.ZCRate)
	}
}

func TestZeroCrossing_Noise(t *testing.T) {
	// White noise should have a high zero-crossing rate (> 5000/sec).
	w := Noise(42, 1.0, 1.0, sr44100)
	obs := NewZeroCrossingObserver()
	o := obs.Observe(w)

	if o.ZCRate < 5000 {
		t.Fatalf("ZCRate: want > 5000 for noise, got %f", o.ZCRate)
	}
}

func TestZeroCrossing_EmptyWave(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	obs := NewZeroCrossingObserver()
	o := obs.Observe(w)

	if o.ZeroCrossings != 0 {
		t.Fatalf("ZeroCrossings: want 0 for empty, got %d", o.ZeroCrossings)
	}
	if o.ZCRate != 0 {
		t.Fatalf("ZCRate: want 0 for empty, got %f", o.ZCRate)
	}
}

func TestZeroCrossing_SingleSample(t *testing.T) {
	w := Wave{Samples: []float64{0.5}, SampleRate: sr44100}
	obs := NewZeroCrossingObserver()
	o := obs.Observe(w)

	if o.ZeroCrossings != 0 {
		t.Fatalf("ZeroCrossings: want 0 for single sample, got %d", o.ZeroCrossings)
	}
}
