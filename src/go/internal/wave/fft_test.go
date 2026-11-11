package wave

import (
	"math"
	"testing"
)

// --- nextPow2 helper ---

func TestNextPow2(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{7, 8},
		{8, 8},
		{9, 16},
		{1023, 1024},
		{1024, 1024},
		{1025, 2048},
	}
	for _, tt := range tests {
		got := nextPow2(tt.in)
		if got != tt.want {
			t.Errorf("nextPow2(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// --- FFT: single frequency (440Hz sine) ---

func TestFFTObserver_SingleFrequency(t *testing.T) {
	// 440Hz sine, 1 second at 44100Hz, FFT size 1024.
	// Bin resolution = 44100/1024 ≈ 43.07 Hz.
	// Expect peak bin near 440Hz (within one bin width ~43Hz).
	w := Sine(440, 1.0, 1.0, sr44100)
	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	if o.Kind != ObsFFT {
		t.Fatalf("Kind: want ObsFFT, got %v", o.Kind)
	}

	// Should have 513 bins (N/2 + 1 for real FFT).
	wantBins := 1024/2 + 1
	if len(o.Bins) != wantBins {
		t.Fatalf("Bins length: want %d, got %d", wantBins, len(o.Bins))
	}
	if len(o.FreqBins) != wantBins {
		t.Fatalf("FreqBins length: want %d, got %d", wantBins, len(o.FreqBins))
	}

	// BinHz should be 44100/1024 ≈ 43.07.
	wantBinHz := float64(sr44100) / 1024.0
	if math.Abs(o.BinHz-wantBinHz) > 0.01 {
		t.Fatalf("BinHz: want %f, got %f", wantBinHz, o.BinHz)
	}

	// PeakFreq should be near 440Hz (within 50Hz).
	if math.Abs(o.PeakFreq-440) > 50 {
		t.Fatalf("PeakFreq: want ~440Hz, got %fHz", o.PeakFreq)
	}

	// FreqBins[0] should be 0, FreqBins[1] should be ~BinHz.
	if o.FreqBins[0] != 0 {
		t.Fatalf("FreqBins[0]: want 0, got %f", o.FreqBins[0])
	}
	if math.Abs(o.FreqBins[1]-wantBinHz) > 0.01 {
		t.Fatalf("FreqBins[1]: want %f, got %f", wantBinHz, o.FreqBins[1])
	}
}

// --- FFT: two frequencies (440Hz + 1000Hz) ---

func TestFFTObserver_TwoFrequencies(t *testing.T) {
	// Sum of 440Hz + 1000Hz sines.
	w1 := Sine(440, 0.5, 1.0, sr44100)
	w2 := Sine(1000, 0.5, 1.0, sr44100)
	samples := make([]float64, len(w1.Samples))
	for i := range samples {
		samples[i] = w1.Samples[i] + w2.Samples[i]
	}
	w := Wave{Samples: samples, SampleRate: sr44100, Label: "two_freq"}

	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	// Find the two highest bins.
	if len(o.Bins) < 2 {
		t.Fatalf("not enough bins")
	}

	// Find top-2 peak indices.
	type peak struct {
		idx int
		mag float64
	}
	var top1, top2 peak
	top1.mag = -math.MaxFloat64
	top2.mag = -math.MaxFloat64
	for i, db := range o.Bins {
		if db > top1.mag {
			top2 = top1
			top1 = peak{i, db}
		} else if db > top2.mag {
			top2 = peak{i, db}
		}
	}

	freq1 := o.FreqBins[top1.idx]
	freq2 := o.FreqBins[top2.idx]
	// Sort so freq1 < freq2.
	if freq1 > freq2 {
		freq1, freq2 = freq2, freq1
	}

	if math.Abs(freq1-440) > 50 {
		t.Fatalf("first peak: want ~440Hz, got %fHz", freq1)
	}
	if math.Abs(freq2-1000) > 50 {
		t.Fatalf("second peak: want ~1000Hz, got %fHz", freq2)
	}
}

// --- FFT: silence (all bins < -80dB) ---

func TestFFTObserver_Silence(t *testing.T) {
	w := Silence(1.0, sr44100)
	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	for i, db := range o.Bins {
		if db > -80 {
			t.Fatalf("Silence bin[%d] = %fdB, want < -80dB", i, db)
		}
	}
}

// --- FFT: DC signal (bin[0] high, others low) ---

func TestFFTObserver_DC(t *testing.T) {
	w := DC(0.5, 1.0, sr44100)
	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	// bin[0] (DC component) should be the dominant peak.
	// Hann window attenuates DC, so the level is reduced; but it should
	// still be well above the noise floor and be the highest bin.
	if o.Bins[0] < -20 {
		t.Fatalf("DC bin[0] = %fdB, want > -20dB", o.Bins[0])
	}

	// bin[0] must be the peak.
	if o.PeakFreq != 0 {
		t.Fatalf("DC PeakFreq: want 0Hz (DC), got %fHz", o.PeakFreq)
	}

	// Bins far from DC should be well below bin[0]. The Hann window
	// causes spectral leakage into the first few adjacent bins, so we
	// only check bins beyond index 4. Those should be at least 30dB
	// below the DC bin.
	for i := 4; i < len(o.Bins); i++ {
		if o.Bins[i] > o.Bins[0]-30 {
			t.Fatalf("DC bin[%d] = %fdB, want < %fdB (bin[0]-30dB)", i, o.Bins[i], o.Bins[0]-30)
		}
	}
}

// --- FFT: Parseval's theorem ---

func TestFFTObserver_ParsevalTheorem(t *testing.T) {
	// Time-domain energy ≈ frequency-domain energy (within factor of 3
	// due to Hann windowing).
	w := Sine(440, 1.0, 1.0, sr44100)
	fftSize := 1024

	// Use the last fftSize samples (same as what the observer does).
	start := len(w.Samples) - fftSize
	segment := w.Samples[start : start+fftSize]

	// Time-domain energy (with Hann window applied).
	var timeEnergy float64
	for i := 0; i < fftSize; i++ {
		hannVal := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize)))
		s := segment[i] * hannVal
		timeEnergy += s * s
	}

	// Frequency-domain: observe, then convert dB bins back to magnitude
	// and compute energy. The observer stores normalized magnitudes:
	//   bins_dB[k] = 20*log10(|X[k]| / N)
	// So the unnormalized DFT magnitude is: |X[k]| = 10^(dB/20) * N.
	// Parseval's theorem: sum(|x[n]|^2) = (1/N) * sum(|X[k]|^2).
	// Because the input is real, bins 1..N/2-1 are doubled (positive +
	// negative frequency). Bin 0 (DC) and bin N/2 (Nyquist) appear once.
	obs := NewFFTObserver(fftSize)
	o := obs.Observe(w)

	var freqEnergy float64
	for i, db := range o.Bins {
		normMag := math.Pow(10, db/20.0) // normalized magnitude
		unnormMag := normMag * float64(fftSize)
		power := unnormMag * unnormMag
		if i > 0 && i < len(o.Bins)-1 {
			power *= 2 // mirror for negative frequencies
		}
		freqEnergy += power
	}
	// Parseval's: sum(|x[n]|^2) = (1/N) * sum(|X[k]|^2)
	freqEnergy /= float64(fftSize)

	ratio := freqEnergy / timeEnergy
	if ratio < 1.0/3.0 || ratio > 3.0 {
		t.Fatalf("Parseval ratio: want 0.33..3.0, got %f (time=%f, freq=%f)",
			ratio, timeEnergy, freqEnergy)
	}
}

// --- FFT: short wave (< FFT size, zero-padded) ---

func TestFFTObserver_ShortWave(t *testing.T) {
	// Only 100 samples of 440Hz sine, but FFT size 1024.
	// Should zero-pad and still produce correct bin count.
	w := Sine(440, 1.0, 100.0/float64(sr44100), sr44100) // 100 samples
	if w.Len() != 100 {
		t.Fatalf("precondition: want 100 samples, got %d", w.Len())
	}

	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	wantBins := 1024/2 + 1
	if len(o.Bins) != wantBins {
		t.Fatalf("short wave Bins length: want %d, got %d", wantBins, len(o.Bins))
	}
	if len(o.FreqBins) != wantBins {
		t.Fatalf("short wave FreqBins length: want %d, got %d", wantBins, len(o.FreqBins))
	}

	// BinHz should still be based on sample rate and FFT size.
	wantBinHz := float64(sr44100) / 1024.0
	if math.Abs(o.BinHz-wantBinHz) > 0.01 {
		t.Fatalf("BinHz: want %f, got %f", wantBinHz, o.BinHz)
	}
}

// --- FFT: size rounds up to power of 2 ---

func TestFFTObserver_RoundsUpSize(t *testing.T) {
	// Requesting size 1000 should round up to 1024.
	w := Sine(440, 1.0, 1.0, sr44100)
	obs := NewFFTObserver(1000)
	o := obs.Observe(w)

	// 1024/2 + 1 = 513 bins.
	wantBins := 1024/2 + 1
	if len(o.Bins) != wantBins {
		t.Fatalf("rounded Bins length: want %d, got %d", wantBins, len(o.Bins))
	}
}

// --- FFT: uses last N samples when wave is longer ---

func TestFFTObserver_UsesLastSamples(t *testing.T) {
	// Construct a wave: first half silence, second half 440Hz sine.
	// FFT size 1024 should use the last 1024 samples (all sine).
	silence := Silence(0.5, sr44100)
	sine := Sine(440, 1.0, 0.5, sr44100)
	samples := make([]float64, len(silence.Samples)+len(sine.Samples))
	copy(samples, silence.Samples)
	copy(samples[len(silence.Samples):], sine.Samples)
	w := Wave{Samples: samples, SampleRate: sr44100, Label: "half_sine"}

	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	// Peak should be near 440Hz since the last 1024 samples are sine.
	if math.Abs(o.PeakFreq-440) > 50 {
		t.Fatalf("PeakFreq from tail: want ~440Hz, got %fHz", o.PeakFreq)
	}
}

func TestMagnitudeSpectrum_SinePeakBin(t *testing.T) {
	sr := 44100
	w := Sine(1000, 1.0, 0.2, sr) // exported testgen
	mag, binHz := MagnitudeSpectrum(w, 16384, WindowHann)
	if len(mag) != 16384/2+1 {
		t.Fatalf("got %d bins, want %d", len(mag), 16384/2+1)
	}
	peak, peakIdx := 0.0, 0
	for i, m := range mag {
		if m > peak {
			peak, peakIdx = m, i
		}
	}
	gotHz := float64(peakIdx) * binHz
	if math.Abs(gotHz-1000) > binHz*2 {
		t.Errorf("peak at %.1f Hz, want ~1000 (binHz=%.2f)", gotHz, binHz)
	}
}

// --- FFT: empty wave ---

func TestFFTObserver_EmptyWave(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	obs := NewFFTObserver(1024)
	o := obs.Observe(w)

	// Should still return correct number of bins (all very low dB).
	wantBins := 1024/2 + 1
	if len(o.Bins) != wantBins {
		t.Fatalf("empty wave Bins length: want %d, got %d", wantBins, len(o.Bins))
	}
	for i, db := range o.Bins {
		if db > -80 {
			t.Fatalf("empty wave bin[%d] = %fdB, want < -80dB", i, db)
		}
	}
}
