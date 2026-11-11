# Wave Toolkit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a composable waveform observability and transformation toolkit with live monitoring, static inspection, and an expandable tabbed UI panel.

**Architecture:** Four layers — (1) pure data types in `wave` package, (2) sources/transforms/observers with C bridges, (3) analyzer service goroutine with lock-free publishing, (4) Ebiten UI renderers. Audio thread only memcopies; all analysis runs on a dedicated goroutine.

**Tech Stack:** Go 1.23, C (FFT), CGo, Ebiten v2, atomic pointer swap for lock-free state

**Spec:** `docs/superpowers/specs/2026-04-03-wave-toolkit-design.md`

---

## File Structure

### New Files (Layer 1 — `src/go/internal/wave/`)

| File | Responsibility |
|------|----------------|
| `wave.go` | Wave struct, Source/Transform/Observer interfaces, Chain/Observe/Pipeline compositors |
| `testgen.go` | Deterministic wave generators for testing: Sine, Silence, Square, Impulse, DC, Noise |
| `wave_test.go` | Wave helpers + chain compositor tests |
| `observers.go` | PeakRMS, Envelope, ZeroCrossing observers (pure Go) |
| `observers_test.go` | Observer tests with deterministic inputs and known expected outputs |
| `fft.go` | Go FFT implementation (used in all builds — the wave package is pure Go) |
| `fft_test.go` | FFT correctness tests (known frequencies, Parseval's theorem) |
| `transforms.go` | Normalize, Trim, Window, Resample transforms |
| `transforms_test.go` | Transform tests with deterministic waves |

### New Files (Layer 2 — audio bridges, `src/go/internal/wave/`)

| File | Responsibility |
|------|----------------|
| `source_render.go` | RenderSource — calls C synth, returns Wave. Build tag `!test && !js` |
| `source_render_stub.go` | Stub RenderSource for tests. Build tag `test \|\| js` |
| `effect_transform.go` | EffectTransform — wraps C insert effects as Transform. Build tag `!test && !js` |
| `effect_transform_stub.go` | Stub EffectTransform for tests. Build tag `test \|\| js` |

> **Note:** `source_render.go`, `source_render_stub.go`, `effect_transform.go`, and `effect_transform_stub.go` are listed here for completeness but are **not covered by tasks in this plan**. They enable on-demand offline rendering and effect preview — features that build on the core toolkit but aren't required for the initial waveform inspection + real-time monitoring deliverables. Add these as follow-up tasks.

### New Files (Layer 3 — `src/go/internal/analyzer/`)

| File | Responsibility |
|------|----------------|
| `ring.go` | SPSC ring buffer for float64 slices |
| `ring_test.go` | Ring buffer correctness, overflow, concurrent SPSC |
| `state.go` | State, ChannelMetrics, InstrumentMetrics, CaptureBuffer types |
| `service.go` | Analyzer goroutine, ingest, observer dispatch, atomic state publish |
| `service_test.go` | Service integration tests (push data → verify published state) |

### New Files (Layer 4 — `src/go/internal/ui/`)

| File | Responsibility |
|------|----------------|
| `panel_tab.go` | PanelTab enum, tab bar rendering, expand/collapse state |
| `panel_tab_test.go` | Tab switching, expand/collapse, layout tests |
| `render_waveform.go` | Enhanced waveform renderer reading from analyzer.State |
| `render_spectrum.go` | FFT spectrum bar renderer |
| `render_meters.go` | Meter bridge renderer (all instruments) |

### Modified Files

| File | Change |
|------|--------|
| `src/go/internal/audio/engine_stop.go` | Add analyzer push calls (~6 lines) |
| `src/go/internal/audio/engine_mixer.go` | Add trigger notification (~4 lines) |
| `src/go/internal/audio/engine.go` | Init analyzer service (~5 lines) |
| `src/go/internal/ui/eq_panel_zone.go` | Replace `waveformMode bool` with tab system |
| `src/go/internal/ui/drumview_draw_waveform.go` | Route through new renderer |
| `Makefile` | Add `wave` and `analyzer` packages to test targets |

### Test Commands

```bash
# Wave package (pure Go, always runs with test tag)
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v

# Analyzer package (pure Go service layer)
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v

# UI tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -v -run <TestName>

# All fast tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 60s ./...
```

---

## Task 1: Wave Struct + Deterministic Test Generators

**Files:**
- Create: `src/go/internal/wave/wave.go`
- Create: `src/go/internal/wave/testgen.go`
- Create: `src/go/internal/wave/wave_test.go`

This task establishes the foundation: the `Wave` type, interfaces, and deterministic wave generators that every subsequent test will use.

- [ ] **Step 1: Write tests for Wave struct helpers and test generators**

```go
// src/go/internal/wave/wave_test.go
package wave

import (
	"math"
	"testing"
)

func TestWaveDuration(t *testing.T) {
	w := Wave{Samples: make([]float64, 44100), SampleRate: 44100, Label: "1s"}
	if got := w.Duration(); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("Duration() = %v, want 1.0", got)
	}
}

func TestWaveDurationEmpty(t *testing.T) {
	w := Wave{SampleRate: 44100}
	if got := w.Duration(); got != 0 {
		t.Fatalf("Duration() = %v, want 0", got)
	}
}

func TestWavePeakSample(t *testing.T) {
	w := Wave{Samples: []float64{0.1, -0.5, 0.3, 0.8, -0.2}, SampleRate: 44100}
	if got := w.PeakSample(); math.Abs(got-0.8) > 1e-9 {
		t.Fatalf("PeakSample() = %v, want 0.8", got)
	}
}

func TestWavePeakSampleNegative(t *testing.T) {
	w := Wave{Samples: []float64{0.1, -0.9, 0.3}, SampleRate: 44100}
	if got := w.PeakSample(); math.Abs(got-0.9) > 1e-9 {
		t.Fatalf("PeakSample() = %v, want 0.9", got)
	}
}

func TestWaveSlice(t *testing.T) {
	w := Wave{Samples: make([]float64, 44100), SampleRate: 44100, Label: "full"}
	// Slice 100ms-200ms = samples 4410-8820
	s := w.Slice(100, 200)
	if len(s.Samples) != 4410 {
		t.Fatalf("Slice len = %d, want 4410", len(s.Samples))
	}
	if s.SampleRate != 44100 {
		t.Fatalf("Slice SampleRate = %d, want 44100", s.SampleRate)
	}
}

func TestWaveSliceClamp(t *testing.T) {
	w := Wave{Samples: make([]float64, 4410), SampleRate: 44100}
	s := w.Slice(0, 1000) // 1000ms but only 100ms of data
	if len(s.Samples) != 4410 {
		t.Fatalf("Slice clamped len = %d, want 4410", len(s.Samples))
	}
}

func TestWaveClone(t *testing.T) {
	w := Wave{Samples: []float64{1, 2, 3}, SampleRate: 44100, Label: "orig"}
	c := w.Clone()
	c.Samples[0] = 99
	if w.Samples[0] != 1 {
		t.Fatal("Clone mutated original")
	}
	if c.Label != "orig" {
		t.Fatalf("Clone Label = %q, want %q", c.Label, "orig")
	}
}

func TestSineGenerator(t *testing.T) {
	w := Sine(440, 1.0, 0.01, 44100) // 10ms of 440Hz sine at amplitude 1.0
	if len(w.Samples) != 441 {
		t.Fatalf("Sine len = %d, want 441", len(w.Samples))
	}
	// At 440Hz, one full cycle is 44100/440 ≈ 100.23 samples.
	// First sample should be 0 (sin(0)), sample at ~25 should be near +1 (sin(π/2)).
	if math.Abs(w.Samples[0]) > 1e-9 {
		t.Fatalf("Sine first sample = %v, want 0", w.Samples[0])
	}
	// Peak should be close to amplitude
	if w.PeakSample() < 0.99 {
		t.Fatalf("Sine peak = %v, want ~1.0", w.PeakSample())
	}
}

func TestSilenceGenerator(t *testing.T) {
	w := Silence(0.1, 44100) // 100ms
	if len(w.Samples) != 4410 {
		t.Fatalf("Silence len = %d, want 4410", len(w.Samples))
	}
	for i, s := range w.Samples {
		if s != 0 {
			t.Fatalf("Silence sample[%d] = %v, want 0", i, s)
		}
	}
}

func TestSquareGenerator(t *testing.T) {
	w := Square(100, 0.5, 0.01, 44100) // 100Hz square at 0.5 amplitude
	// First half-cycle should be positive, second negative
	// 44100/100 = 441 samples per cycle, half = 220.5
	positiveCount := 0
	for _, s := range w.Samples {
		if s > 0 {
			positiveCount++
		}
	}
	// Roughly half should be positive
	ratio := float64(positiveCount) / float64(len(w.Samples))
	if ratio < 0.4 || ratio > 0.6 {
		t.Fatalf("Square positive ratio = %v, want ~0.5", ratio)
	}
	// Amplitude should be exactly 0.5
	if math.Abs(w.PeakSample()-0.5) > 1e-9 {
		t.Fatalf("Square peak = %v, want 0.5", w.PeakSample())
	}
}

func TestImpulseGenerator(t *testing.T) {
	w := Impulse(0.8, 10, 0.01, 44100) // impulse at sample 10
	if len(w.Samples) != 441 {
		t.Fatalf("Impulse len = %d, want 441", len(w.Samples))
	}
	if w.Samples[10] != 0.8 {
		t.Fatalf("Impulse at index 10 = %v, want 0.8", w.Samples[10])
	}
	// All other samples should be 0
	for i, s := range w.Samples {
		if i != 10 && s != 0 {
			t.Fatalf("Impulse sample[%d] = %v, want 0", i, s)
		}
	}
}

func TestDCGenerator(t *testing.T) {
	w := DC(0.3, 0.01, 44100) // constant 0.3 for 10ms
	if len(w.Samples) != 441 {
		t.Fatalf("DC len = %d, want 441", len(w.Samples))
	}
	for i, s := range w.Samples {
		if s != 0.3 {
			t.Fatalf("DC sample[%d] = %v, want 0.3", i, s)
		}
	}
}

func TestNoiseGeneratorDeterministic(t *testing.T) {
	w1 := Noise(42, 1.0, 0.01, 44100)
	w2 := Noise(42, 1.0, 0.01, 44100)
	if len(w1.Samples) != len(w2.Samples) {
		t.Fatal("Noise generators with same seed produced different lengths")
	}
	for i := range w1.Samples {
		if w1.Samples[i] != w2.Samples[i] {
			t.Fatalf("Noise sample[%d] differs with same seed", i)
		}
	}
	// Different seed should differ
	w3 := Noise(99, 1.0, 0.01, 44100)
	differs := false
	for i := range w1.Samples {
		if w1.Samples[i] != w3.Samples[i] {
			differs = true
			break
		}
	}
	if !differs {
		t.Fatal("Different seeds produced identical noise")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v`
Expected: Compilation failure — package `wave` does not exist.

- [ ] **Step 3: Implement Wave struct, interfaces, and helpers**

```go
// src/go/internal/wave/wave.go
package wave

import "math"

// Wave is the fundamental data unit — a buffer of samples with metadata.
type Wave struct {
	Samples    []float64
	SampleRate int
	Label      string
}

// Duration returns the wave's duration in seconds.
func (w Wave) Duration() float64 {
	if w.SampleRate == 0 || len(w.Samples) == 0 {
		return 0
	}
	return float64(len(w.Samples)) / float64(w.SampleRate)
}

// PeakSample returns the maximum absolute sample value.
func (w Wave) PeakSample() float64 {
	peak := 0.0
	for _, s := range w.Samples {
		a := math.Abs(s)
		if a > peak {
			peak = a
		}
	}
	return peak
}

// Slice extracts a time range (in milliseconds) as a new Wave. Clamps to bounds.
func (w Wave) Slice(startMs, endMs float64) Wave {
	start := int(startMs * 0.001 * float64(w.SampleRate))
	end := int(endMs * 0.001 * float64(w.SampleRate))
	if start < 0 {
		start = 0
	}
	if end > len(w.Samples) {
		end = len(w.Samples)
	}
	if start >= end {
		return Wave{SampleRate: w.SampleRate, Label: w.Label + "/slice"}
	}
	out := make([]float64, end-start)
	copy(out, w.Samples[start:end])
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label + "/slice"}
}

// Clone returns a deep copy of the wave.
func (w Wave) Clone() Wave {
	out := make([]float64, len(w.Samples))
	copy(out, w.Samples)
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label}
}

// Len returns the number of samples.
func (w Wave) Len() int { return len(w.Samples) }

// --- Interfaces ---

// Source produces a Wave.
type Source interface {
	Render() Wave
}

// Transform takes a Wave, returns a new Wave. Must not mutate the input.
type Transform interface {
	Apply(w Wave) Wave
}

// Observer reads a Wave non-destructively, returns metrics.
type Observer interface {
	Observe(w Wave) Observation
}

// ObsKind identifies which fields of Observation are populated.
type ObsKind int

const (
	ObsFFT          ObsKind = iota
	ObsPeakRMS
	ObsEnvelope
	ObsZeroCrossing
)

// Observation is the result of an Observer. Each observer populates the fields
// relevant to its kind.
type Observation struct {
	Kind          ObsKind
	Spectrum      []float64 // FFT magnitudes in dB (ObsFFT)
	FreqBins      []float64 // Hz labels for each spectrum bin (ObsFFT)
	PeakDB        float64   // peak level in dB (ObsPeakRMS)
	RMSDB         float64   // RMS level in dB (ObsPeakRMS)
	ClipCount     int       // samples exceeding ±1.0 (ObsPeakRMS)
	Envelope      []float64 // amplitude envelope (ObsEnvelope)
	ZeroCrossRate float64   // crossings per second (ObsZeroCrossing)
}

// --- Compositors ---

// Chain applies transforms in order to a source and returns the resulting Wave.
func Chain(src Source, transforms ...Transform) Wave {
	w := src.Render()
	for _, t := range transforms {
		w = t.Apply(w)
	}
	return w
}

// Observe runs multiple observers on a Wave and returns all observations.
func Observe(w Wave, observers ...Observer) []Observation {
	results := make([]Observation, len(observers))
	for i, o := range observers {
		results[i] = o.Observe(w)
	}
	return results
}

// Pipeline renders a source, applies transforms, and runs observers.
func Pipeline(src Source, transforms []Transform, observers []Observer) (Wave, []Observation) {
	w := Chain(src, transforms...)
	obs := Observe(w, observers...)
	return w, obs
}

// WaveSource wraps a Wave as a Source (useful for chaining a pre-existing buffer).
type WaveSource struct{ W Wave }

func (s WaveSource) Render() Wave { return s.W }
```

- [ ] **Step 4: Implement deterministic test wave generators**

```go
// src/go/internal/wave/testgen.go
package wave

import (
	"math"
	"math/rand"
)

// Sine generates a sine wave with the given frequency (Hz), amplitude, duration (seconds),
// and sample rate. Phase starts at 0. Deterministic for the same inputs.
func Sine(freq, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	inc := 2 * math.Pi * freq / float64(sampleRate)
	for i := range samples {
		samples[i] = amplitude * math.Sin(inc*float64(i))
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "sine"}
}

// Silence generates a buffer of zeros.
func Silence(durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	return Wave{Samples: make([]float64, n), SampleRate: sampleRate, Label: "silence"}
}

// Square generates a band-unlimited square wave. +amplitude for the first half
// of each cycle, -amplitude for the second half.
func Square(freq, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	period := float64(sampleRate) / freq
	for i := range samples {
		pos := math.Mod(float64(i), period)
		if pos < period/2 {
			samples[i] = amplitude
		} else {
			samples[i] = -amplitude
		}
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "square"}
}

// Impulse generates a single impulse at the given sample index. All other samples are 0.
func Impulse(amplitude float64, sampleIndex int, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	if sampleIndex >= 0 && sampleIndex < n {
		samples[sampleIndex] = amplitude
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "impulse"}
}

// DC generates a constant-amplitude wave.
func DC(amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	for i := range samples {
		samples[i] = amplitude
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "dc"}
}

// Noise generates deterministic pseudo-random noise using the given seed.
// Values are in [-amplitude, +amplitude].
func Noise(seed int64, amplitude, durationSec float64, sampleRate int) Wave {
	n := int(durationSec * float64(sampleRate))
	samples := make([]float64, n)
	rng := rand.New(rand.NewSource(seed))
	for i := range samples {
		samples[i] = (rng.Float64()*2 - 1) * amplitude
	}
	return Wave{Samples: samples, SampleRate: sampleRate, Label: "noise"}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v`
Expected: All tests PASS.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/wave/wave.go src/go/internal/wave/testgen.go src/go/internal/wave/wave_test.go
git commit -m "feat(wave): add Wave struct, interfaces, compositors, and deterministic test generators"
```

---

## Task 2: PeakRMS Observer

**Files:**
- Create: `src/go/internal/wave/observers.go`
- Create: `src/go/internal/wave/observers_test.go`

- [ ] **Step 1: Write failing tests for PeakRMS observer**

```go
// src/go/internal/wave/observers_test.go
package wave

import (
	"math"
	"testing"
)

const float64Epsilon = 1e-6

func TestPeakRMSObserverSine(t *testing.T) {
	// A 440Hz sine at amplitude 1.0 for 100ms.
	// RMS of a sine wave = amplitude / sqrt(2) ≈ 0.7071
	// Peak = 1.0
	// PeakDB = 20*log10(1.0) = 0 dB
	// RMSDB = 20*log10(0.7071) ≈ -3.01 dB
	w := Sine(440, 1.0, 0.1, 44100)
	obs := NewPeakRMSObserver().Observe(w)
	if obs.Kind != ObsPeakRMS {
		t.Fatalf("Kind = %v, want ObsPeakRMS", obs.Kind)
	}
	if math.Abs(obs.PeakDB-0.0) > 0.1 {
		t.Fatalf("PeakDB = %v, want ~0.0", obs.PeakDB)
	}
	if math.Abs(obs.RMSDB-(-3.01)) > 0.1 {
		t.Fatalf("RMSDB = %v, want ~-3.01", obs.RMSDB)
	}
	if obs.ClipCount != 0 {
		t.Fatalf("ClipCount = %d, want 0", obs.ClipCount)
	}
}

func TestPeakRMSObserverDC(t *testing.T) {
	// DC at 0.5: Peak = 0.5, RMS = 0.5
	// PeakDB = 20*log10(0.5) ≈ -6.02 dB
	// RMSDB = -6.02 dB
	w := DC(0.5, 0.01, 44100)
	obs := NewPeakRMSObserver().Observe(w)
	if math.Abs(obs.PeakDB-(-6.02)) > 0.1 {
		t.Fatalf("PeakDB = %v, want ~-6.02", obs.PeakDB)
	}
	if math.Abs(obs.RMSDB-(-6.02)) > 0.1 {
		t.Fatalf("RMSDB = %v, want ~-6.02", obs.RMSDB)
	}
}

func TestPeakRMSObserverSilence(t *testing.T) {
	w := Silence(0.01, 44100)
	obs := NewPeakRMSObserver().Observe(w)
	if obs.PeakDB > -100 {
		t.Fatalf("PeakDB = %v, want < -100 (silence)", obs.PeakDB)
	}
	if obs.RMSDB > -100 {
		t.Fatalf("RMSDB = %v, want < -100 (silence)", obs.RMSDB)
	}
}

func TestPeakRMSObserverClipping(t *testing.T) {
	// Create a wave with clipping samples (exceeding ±1.0)
	w := Wave{Samples: []float64{0.5, 1.2, -1.5, 0.8, 1.01, -0.3}, SampleRate: 44100, Label: "clip"}
	obs := NewPeakRMSObserver().Observe(w)
	if obs.ClipCount != 3 { // 1.2, -1.5, 1.01
		t.Fatalf("ClipCount = %d, want 3", obs.ClipCount)
	}
}

func TestPeakRMSObserverEmpty(t *testing.T) {
	w := Wave{SampleRate: 44100}
	obs := NewPeakRMSObserver().Observe(w)
	if obs.PeakDB > -100 {
		t.Fatalf("PeakDB for empty wave = %v, want < -100", obs.PeakDB)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run PeakRMS`
Expected: Compilation failure — `NewPeakRMSObserver` not defined.

- [ ] **Step 3: Implement PeakRMS observer**

```go
// src/go/internal/wave/observers.go
package wave

import "math"

// peakRMSObserver computes peak level, RMS level (both in dB), and clip count.
type peakRMSObserver struct{}

// NewPeakRMSObserver returns an observer that computes peak and RMS levels.
func NewPeakRMSObserver() Observer { return peakRMSObserver{} }

func (peakRMSObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n == 0 {
		return Observation{Kind: ObsPeakRMS, PeakDB: -math.MaxFloat64, RMSDB: -math.MaxFloat64}
	}
	var peak, sumSq float64
	var clips int
	for _, s := range w.Samples {
		a := math.Abs(s)
		if a > peak {
			peak = a
		}
		if a > 1.0 {
			clips++
		}
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(n))
	return Observation{
		Kind:      ObsPeakRMS,
		PeakDB:    ampToDB(peak),
		RMSDB:     ampToDB(rms),
		ClipCount: clips,
	}
}

// ampToDB converts a linear amplitude to decibels. Returns -infinity for 0.
func ampToDB(amp float64) float64 {
	if amp <= 0 {
		return -math.MaxFloat64
	}
	return 20 * math.Log10(amp)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run PeakRMS`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/wave/observers.go src/go/internal/wave/observers_test.go
git commit -m "feat(wave): add PeakRMS observer with clip detection"
```

---

## Task 3: Envelope and ZeroCrossing Observers

**Files:**
- Modify: `src/go/internal/wave/observers.go`
- Modify: `src/go/internal/wave/observers_test.go`

- [ ] **Step 1: Write failing tests for Envelope and ZeroCrossing**

Append to `observers_test.go`:

```go
func TestEnvelopeObserverDecayingSignal(t *testing.T) {
	// Create a decaying sine: amplitude starts at 1.0 and decays to ~0 over 100ms
	w := Sine(440, 1.0, 0.1, 44100)
	for i := range w.Samples {
		decay := math.Exp(-10.0 * float64(i) / float64(w.SampleRate))
		w.Samples[i] *= decay
	}
	obs := NewEnvelopeObserver(1.0, 50.0).Observe(w)
	if obs.Kind != ObsEnvelope {
		t.Fatalf("Kind = %v, want ObsEnvelope", obs.Kind)
	}
	if len(obs.Envelope) != len(w.Samples) {
		t.Fatalf("Envelope len = %d, want %d", len(obs.Envelope), len(w.Samples))
	}
	// Envelope should start high and decay
	if obs.Envelope[0] < 0.5 {
		t.Fatalf("Envelope start = %v, want > 0.5", obs.Envelope[0])
	}
	last := obs.Envelope[len(obs.Envelope)-1]
	if last > 0.1 {
		t.Fatalf("Envelope end = %v, want < 0.1 (decayed)", last)
	}
}

func TestEnvelopeObserverSilence(t *testing.T) {
	w := Silence(0.01, 44100)
	obs := NewEnvelopeObserver(1.0, 50.0).Observe(w)
	for i, v := range obs.Envelope {
		if v != 0 {
			t.Fatalf("Envelope[%d] = %v, want 0 for silence", i, v)
		}
	}
}

func TestZeroCrossingObserverSine(t *testing.T) {
	// 440Hz sine crosses zero twice per cycle = 880 crossings/sec
	w := Sine(440, 1.0, 0.1, 44100)
	obs := NewZeroCrossingObserver().Observe(w)
	if obs.Kind != ObsZeroCrossing {
		t.Fatalf("Kind = %v, want ObsZeroCrossing", obs.Kind)
	}
	// Allow 5% tolerance
	if math.Abs(obs.ZeroCrossRate-880) > 44 {
		t.Fatalf("ZeroCrossRate = %v, want ~880", obs.ZeroCrossRate)
	}
}

func TestZeroCrossingObserverDC(t *testing.T) {
	w := DC(0.5, 0.1, 44100)
	obs := NewZeroCrossingObserver().Observe(w)
	if obs.ZeroCrossRate != 0 {
		t.Fatalf("ZeroCrossRate for DC = %v, want 0", obs.ZeroCrossRate)
	}
}

func TestZeroCrossingObserverNoise(t *testing.T) {
	// White noise should have a high zero-crossing rate
	w := Noise(42, 1.0, 0.1, 44100)
	obs := NewZeroCrossingObserver().Observe(w)
	// White noise at 44.1kHz typically crosses ~14000-15000 times/sec
	if obs.ZeroCrossRate < 5000 {
		t.Fatalf("ZeroCrossRate for noise = %v, want > 5000", obs.ZeroCrossRate)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run "Envelope|ZeroCross"`
Expected: Compilation failure — `NewEnvelopeObserver`, `NewZeroCrossingObserver` not defined.

- [ ] **Step 3: Implement Envelope and ZeroCrossing observers**

Append to `observers.go`:

```go
// envelopeObserver follows the amplitude envelope with configurable attack/release.
type envelopeObserver struct {
	attackMs  float64
	releaseMs float64
}

// NewEnvelopeObserver returns an observer that tracks the amplitude envelope.
// attackMs controls how fast the envelope rises; releaseMs controls how fast it falls.
func NewEnvelopeObserver(attackMs, releaseMs float64) Observer {
	return envelopeObserver{attackMs: attackMs, releaseMs: releaseMs}
}

func (e envelopeObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n == 0 {
		return Observation{Kind: ObsEnvelope}
	}
	env := make([]float64, n)
	sr := float64(w.SampleRate)
	attackCoef := 1.0 - math.Exp(-1.0/(e.attackMs*0.001*sr))
	releaseCoef := 1.0 - math.Exp(-1.0/(e.releaseMs*0.001*sr))

	var current float64
	for i, s := range w.Samples {
		a := math.Abs(s)
		if a > current {
			current += attackCoef * (a - current)
		} else {
			current += releaseCoef * (a - current)
		}
		env[i] = current
	}
	return Observation{Kind: ObsEnvelope, Envelope: env}
}

// zeroCrossingObserver counts sign changes in the waveform.
type zeroCrossingObserver struct{}

// NewZeroCrossingObserver returns an observer that computes the zero-crossing rate.
func NewZeroCrossingObserver() Observer { return zeroCrossingObserver{} }

func (zeroCrossingObserver) Observe(w Wave) Observation {
	n := len(w.Samples)
	if n < 2 {
		return Observation{Kind: ObsZeroCrossing}
	}
	crossings := 0
	for i := 1; i < n; i++ {
		if (w.Samples[i-1] >= 0) != (w.Samples[i] >= 0) {
			crossings++
		}
	}
	rate := float64(crossings) / w.Duration()
	return Observation{Kind: ObsZeroCrossing, ZeroCrossRate: rate}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run "Envelope|ZeroCross"`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/wave/observers.go src/go/internal/wave/observers_test.go
git commit -m "feat(wave): add Envelope and ZeroCrossing observers"
```

---

## Task 4: Pure Go FFT + FFT Observer

**Files:**
- Create: `src/go/internal/wave/fft.go`
- Create: `src/go/internal/wave/fft_test.go`

The wave package is pure Go (no build tags). We implement an in-place radix-2 Cooley-Tukey FFT here. The C FFT in `src/c/fft.c` is a future optimization used by the analyzer service only — this Go FFT is the canonical implementation used by the `FFTObserver`.

- [ ] **Step 1: Write failing tests for FFT observer**

```go
// src/go/internal/wave/fft_test.go
package wave

import (
	"math"
	"testing"
)

func TestFFTObserverSingleFrequency(t *testing.T) {
	// 440Hz sine — FFT should show a clear peak at 440Hz
	w := Sine(440, 1.0, 0.1, 44100)
	obs := NewFFTObserver(1024).Observe(w)
	if obs.Kind != ObsFFT {
		t.Fatalf("Kind = %v, want ObsFFT", obs.Kind)
	}
	if len(obs.Spectrum) != 512 {
		t.Fatalf("Spectrum len = %d, want 512", len(obs.Spectrum))
	}
	if len(obs.FreqBins) != 512 {
		t.Fatalf("FreqBins len = %d, want 512", len(obs.FreqBins))
	}
	// Find the peak bin
	peakBin := 0
	peakVal := obs.Spectrum[0]
	for i, v := range obs.Spectrum {
		if v > peakVal {
			peakVal = v
			peakBin = i
		}
	}
	peakFreq := obs.FreqBins[peakBin]
	// Peak should be near 440Hz (within one bin width: 44100/1024 ≈ 43Hz)
	if math.Abs(peakFreq-440) > 50 {
		t.Fatalf("Peak frequency = %v Hz (bin %d), want ~440 Hz", peakFreq, peakBin)
	}
}

func TestFFTObserverTwoFrequencies(t *testing.T) {
	// Sum of 440Hz + 1000Hz — should show two peaks
	w1 := Sine(440, 0.5, 0.1, 44100)
	w2 := Sine(1000, 0.5, 0.1, 44100)
	combined := Wave{Samples: make([]float64, len(w1.Samples)), SampleRate: 44100, Label: "dual"}
	for i := range combined.Samples {
		combined.Samples[i] = w1.Samples[i] + w2.Samples[i]
	}
	obs := NewFFTObserver(1024).Observe(combined)

	// Find the two highest bins
	type binVal struct {
		idx int
		val float64
	}
	top := [2]binVal{{-1, -math.MaxFloat64}, {-1, -math.MaxFloat64}}
	for i, v := range obs.Spectrum {
		if v > top[0].val {
			top[1] = top[0]
			top[0] = binVal{i, v}
		} else if v > top[1].val {
			top[1] = binVal{i, v}
		}
	}
	freq0 := obs.FreqBins[top[0].idx]
	freq1 := obs.FreqBins[top[1].idx]
	// Sort so freq0 < freq1
	if freq0 > freq1 {
		freq0, freq1 = freq1, freq0
	}
	if math.Abs(freq0-440) > 50 {
		t.Fatalf("First peak = %v Hz, want ~440", freq0)
	}
	if math.Abs(freq1-1000) > 50 {
		t.Fatalf("Second peak = %v Hz, want ~1000", freq1)
	}
}

func TestFFTObserverSilence(t *testing.T) {
	w := Silence(0.1, 44100)
	obs := NewFFTObserver(1024).Observe(w)
	// All bins should be very low (< -80dB)
	for i, v := range obs.Spectrum {
		if v > -80 {
			t.Fatalf("Spectrum[%d] = %v dB for silence, want < -80", i, v)
		}
	}
}

func TestFFTObserverDC(t *testing.T) {
	w := DC(0.5, 0.1, 44100)
	obs := NewFFTObserver(1024).Observe(w)
	// DC component should be in bin 0, all others low
	if obs.Spectrum[0] < -10 {
		t.Fatalf("Spectrum[0] for DC = %v dB, want > -10", obs.Spectrum[0])
	}
	for i := 2; i < len(obs.Spectrum); i++ {
		if obs.Spectrum[i] > -40 {
			t.Fatalf("Spectrum[%d] = %v dB for DC, want < -40", i, obs.Spectrum[i])
		}
	}
}

func TestFFTObserverParseval(t *testing.T) {
	// Parseval's theorem: sum of squared time samples ≈ sum of squared FFT magnitudes
	// This validates energy conservation through the FFT
	w := Noise(42, 0.5, 0.1, 44100)
	// Use exactly 1024 samples for clean FFT
	w.Samples = w.Samples[:1024]

	// Time-domain energy
	var timeEnergy float64
	for _, s := range w.Samples {
		timeEnergy += s * s
	}

	// Run FFT and get linear magnitudes for Parseval check
	obs := NewFFTObserver(1024).Observe(w)
	var freqEnergy float64
	for _, db := range obs.Spectrum {
		mag := math.Pow(10, db/20.0)
		freqEnergy += mag * mag
	}
	// Account for mirroring (multiply positive frequencies by 2, except DC)
	freqEnergy = freqEnergy*2 - math.Pow(10, obs.Spectrum[0]/10.0)

	// Energy should be roughly conserved (within 20% — windowing affects this)
	ratio := freqEnergy / timeEnergy
	if ratio < 0.3 || ratio > 3.0 {
		t.Fatalf("Parseval ratio = %v (time=%v, freq=%v), want ~1.0", ratio, timeEnergy, freqEnergy)
	}
}

func TestFFTObserverShortWave(t *testing.T) {
	// Wave shorter than FFT size — should still work (zero-padded)
	w := Sine(440, 1.0, 0.005, 44100) // 5ms = 220 samples < 1024
	obs := NewFFTObserver(1024).Observe(w)
	if len(obs.Spectrum) != 512 {
		t.Fatalf("Spectrum len = %d, want 512", len(obs.Spectrum))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run FFT`
Expected: Compilation failure — `NewFFTObserver` not defined.

- [ ] **Step 3: Implement Go FFT and FFT observer**

```go
// src/go/internal/wave/fft.go
package wave

import (
	"math"
	"math/cmplx"
)

// fftObserver computes a magnitude spectrum via Cooley-Tukey radix-2 FFT.
type fftObserver struct {
	size int // must be power of 2
}

// NewFFTObserver returns an observer that computes the FFT magnitude spectrum.
// Size must be a power of 2 (512, 1024, 2048). The observer applies a Hann window
// and returns size/2 magnitude bins in dB.
func NewFFTObserver(size int) Observer {
	// Round up to nearest power of 2
	p := 1
	for p < size {
		p <<= 1
	}
	return fftObserver{size: p}
}

func (f fftObserver) Observe(w Wave) Observation {
	n := f.size
	half := n / 2

	// Prepare input: take last n samples (or zero-pad if shorter)
	buf := make([]complex128, n)
	srcLen := len(w.Samples)
	offset := 0
	if srcLen > n {
		offset = srcLen - n
	}
	for i := 0; i < n; i++ {
		srcIdx := offset + i
		if srcIdx >= 0 && srcIdx < srcLen {
			// Apply Hann window
			win := 0.5 * (1.0 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
			buf[i] = complex(w.Samples[srcIdx]*win, 0)
		}
	}

	fftInPlace(buf)

	// Compute magnitude spectrum in dB
	spectrum := make([]float64, half)
	freqBins := make([]float64, half)
	binWidth := float64(w.SampleRate) / float64(n)
	for i := 0; i < half; i++ {
		mag := cmplx.Abs(buf[i]) / float64(n)
		spectrum[i] = ampToDB(mag)
		freqBins[i] = float64(i) * binWidth
	}

	return Observation{Kind: ObsFFT, Spectrum: spectrum, FreqBins: freqBins}
}

// fftInPlace performs an in-place iterative Cooley-Tukey radix-2 FFT.
func fftInPlace(buf []complex128) {
	n := len(buf)
	if n == 0 || (n&(n-1)) != 0 {
		return
	}
	// Bit-reversal permutation
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j &= ^bit
		}
		j |= bit
		if i < j {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}
	// Butterfly stages
	for length := 2; length <= n; length <<= 1 {
		theta := -2 * math.Pi / float64(length)
		wlen := complex(math.Cos(theta), math.Sin(theta))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			half := length >> 1
			for j := 0; j < half; j++ {
				u := buf[i+j]
				v := w * buf[i+j+half]
				buf[i+j] = u + v
				buf[i+j+half] = u - v
				w *= wlen
			}
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run FFT`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/wave/fft.go src/go/internal/wave/fft_test.go
git commit -m "feat(wave): add pure Go FFT and FFT observer"
```

---

## Task 5: Transforms (Normalize, Trim, Window, Resample)

**Files:**
- Create: `src/go/internal/wave/transforms.go`
- Create: `src/go/internal/wave/transforms_test.go`

- [ ] **Step 1: Write failing tests for all transforms**

```go
// src/go/internal/wave/transforms_test.go
package wave

import (
	"math"
	"testing"
)

func TestNormalizeTransform(t *testing.T) {
	// DC at 0.25 amplitude → normalize to 0dB (peak = 1.0)
	w := DC(0.25, 0.01, 44100)
	out := NormalizeTransform(0).Apply(w)
	if math.Abs(out.PeakSample()-1.0) > 1e-6 {
		t.Fatalf("Normalized peak = %v, want 1.0", out.PeakSample())
	}
	// Original should not be mutated
	if w.Samples[0] != 0.25 {
		t.Fatal("Normalize mutated input")
	}
}

func TestNormalizeTransformToTarget(t *testing.T) {
	// Normalize to -6dB (amplitude = 0.5012)
	w := DC(0.1, 0.01, 44100)
	out := NormalizeTransform(-6).Apply(w)
	target := math.Pow(10, -6.0/20.0) // ≈ 0.5012
	if math.Abs(out.PeakSample()-target) > 0.01 {
		t.Fatalf("Normalized peak = %v, want ~%v", out.PeakSample(), target)
	}
}

func TestNormalizeTransformSilence(t *testing.T) {
	w := Silence(0.01, 44100)
	out := NormalizeTransform(0).Apply(w)
	// Silence stays silence
	if out.PeakSample() != 0 {
		t.Fatalf("Normalized silence peak = %v, want 0", out.PeakSample())
	}
}

func TestTrimTransform(t *testing.T) {
	w := DC(1.0, 1.0, 44100) // 1 second
	out := TrimTransform(100, 200).Apply(w)
	expected := 4410 // 100ms of samples
	if len(out.Samples) != expected {
		t.Fatalf("Trim len = %d, want %d", len(out.Samples), expected)
	}
}

func TestTrimTransformClamp(t *testing.T) {
	w := DC(1.0, 0.1, 44100) // 100ms
	out := TrimTransform(0, 5000).Apply(w) // 5 seconds, clamped
	if len(out.Samples) != len(w.Samples) {
		t.Fatalf("Trim clamped len = %d, want %d", len(out.Samples), len(w.Samples))
	}
}

func TestWindowTransformHann(t *testing.T) {
	w := DC(1.0, 0.01, 44100)
	out := WindowTransform(WindowHann).Apply(w)
	// Hann window: first and last samples should be ~0
	if math.Abs(out.Samples[0]) > 1e-6 {
		t.Fatalf("Hann first sample = %v, want ~0", out.Samples[0])
	}
	if math.Abs(out.Samples[len(out.Samples)-1]) > 1e-6 {
		t.Fatalf("Hann last sample = %v, want ~0", out.Samples[len(out.Samples)-1])
	}
	// Middle should be close to 1.0
	mid := out.Samples[len(out.Samples)/2]
	if math.Abs(mid-1.0) > 0.01 {
		t.Fatalf("Hann middle = %v, want ~1.0", mid)
	}
}

func TestWindowTransformDoesNotMutate(t *testing.T) {
	w := DC(1.0, 0.01, 44100)
	_ = WindowTransform(WindowHann).Apply(w)
	if w.Samples[0] != 1.0 {
		t.Fatal("Window mutated input")
	}
}

func TestResampleTransformDouble(t *testing.T) {
	// 440Hz sine at 22050Hz → resample to 44100Hz. Length should double.
	w := Sine(440, 1.0, 0.1, 22050)
	out := ResampleTransform(44100).Apply(w)
	expectedLen := len(w.Samples) * 2
	if math.Abs(float64(len(out.Samples)-expectedLen)) > 2 {
		t.Fatalf("Resample len = %d, want ~%d", len(out.Samples), expectedLen)
	}
	if out.SampleRate != 44100 {
		t.Fatalf("Resample rate = %d, want 44100", out.SampleRate)
	}
}

func TestResampleTransformSameRate(t *testing.T) {
	w := Sine(440, 1.0, 0.01, 44100)
	out := ResampleTransform(44100).Apply(w)
	if len(out.Samples) != len(w.Samples) {
		t.Fatalf("Same-rate resample changed length: %d → %d", len(w.Samples), len(out.Samples))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run "Transform"`
Expected: Compilation failure — transform types not defined.

- [ ] **Step 3: Implement transforms**

```go
// src/go/internal/wave/transforms.go
package wave

import "math"

// WindowType identifies a window function for FFT preparation.
type WindowType int

const (
	WindowHann WindowType = iota
	WindowHamming
	WindowBlackman
)

// --- Normalize ---

type normalizeTransform struct{ targetDB float64 }

// NormalizeTransform returns a transform that peak-normalizes to the target dB level.
// 0 dB = peak amplitude 1.0; -6 dB ≈ 0.5.
func NormalizeTransform(targetDB float64) Transform {
	return normalizeTransform{targetDB: targetDB}
}

func (n normalizeTransform) Apply(w Wave) Wave {
	peak := w.PeakSample()
	if peak == 0 {
		return w.Clone()
	}
	targetAmp := math.Pow(10, n.targetDB/20.0)
	scale := targetAmp / peak
	out := make([]float64, len(w.Samples))
	for i, s := range w.Samples {
		out[i] = s * scale
	}
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label + "/norm"}
}

// --- Trim ---

type trimTransform struct{ startMs, endMs float64 }

// TrimTransform extracts a time range (in milliseconds).
func TrimTransform(startMs, endMs float64) Transform {
	return trimTransform{startMs: startMs, endMs: endMs}
}

func (tr trimTransform) Apply(w Wave) Wave {
	return w.Slice(tr.startMs, tr.endMs)
}

// --- Window ---

type windowTransform struct{ wt WindowType }

// WindowTransform applies a window function to the wave.
func WindowTransform(wt WindowType) Transform {
	return windowTransform{wt: wt}
}

func (wn windowTransform) Apply(w Wave) Wave {
	n := len(w.Samples)
	if n == 0 {
		return w.Clone()
	}
	out := make([]float64, n)
	for i, s := range w.Samples {
		out[i] = s * windowCoeff(wn.wt, i, n)
	}
	return Wave{Samples: out, SampleRate: w.SampleRate, Label: w.Label + "/win"}
}

func windowCoeff(wt WindowType, i, n int) float64 {
	x := 2 * math.Pi * float64(i) / float64(n-1)
	switch wt {
	case WindowHann:
		return 0.5 * (1.0 - math.Cos(x))
	case WindowHamming:
		return 0.54 - 0.46*math.Cos(x)
	case WindowBlackman:
		return 0.42 - 0.5*math.Cos(x) + 0.08*math.Cos(2*x)
	default:
		return 1.0
	}
}

// --- Resample ---

type resampleTransform struct{ targetRate int }

// ResampleTransform resamples to a target sample rate via linear interpolation.
func ResampleTransform(targetRate int) Transform {
	return resampleTransform{targetRate: targetRate}
}

func (r resampleTransform) Apply(w Wave) Wave {
	if w.SampleRate == r.targetRate || len(w.Samples) == 0 {
		return w.Clone()
	}
	ratio := float64(r.targetRate) / float64(w.SampleRate)
	outLen := int(float64(len(w.Samples)) * ratio)
	out := make([]float64, outLen)
	for i := range out {
		srcPos := float64(i) / ratio
		idx := int(srcPos)
		frac := srcPos - float64(idx)
		if idx+1 < len(w.Samples) {
			out[i] = w.Samples[idx]*(1-frac) + w.Samples[idx+1]*frac
		} else if idx < len(w.Samples) {
			out[i] = w.Samples[idx]
		}
	}
	return Wave{Samples: out, SampleRate: r.targetRate, Label: w.Label + "/resample"}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run "Transform"`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/wave/transforms.go src/go/internal/wave/transforms_test.go
git commit -m "feat(wave): add Normalize, Trim, Window, Resample transforms"
```

---

## Task 6: Chain Compositor Integration Tests

**Files:**
- Modify: `src/go/internal/wave/wave_test.go`

- [ ] **Step 1: Write integration tests for the Chain/Observe/Pipeline compositors**

Append to `wave_test.go`:

```go
func TestChainSourceThroughTransforms(t *testing.T) {
	src := WaveSource{W: Sine(440, 1.0, 0.1, 44100)}
	w := Chain(src, TrimTransform(0, 50), NormalizeTransform(-6))
	// Trimmed to 50ms = 2205 samples, then normalized to -6dB
	if len(w.Samples) != 2205 {
		t.Fatalf("Chain len = %d, want 2205", len(w.Samples))
	}
	targetAmp := math.Pow(10, -6.0/20.0)
	if math.Abs(w.PeakSample()-targetAmp) > 0.01 {
		t.Fatalf("Chain peak = %v, want ~%v", w.PeakSample(), targetAmp)
	}
}

func TestObserveMultiple(t *testing.T) {
	w := Sine(440, 1.0, 0.1, 44100)
	results := Observe(w, NewPeakRMSObserver(), NewZeroCrossingObserver())
	if len(results) != 2 {
		t.Fatalf("Observe returned %d results, want 2", len(results))
	}
	if results[0].Kind != ObsPeakRMS {
		t.Fatalf("First result kind = %v, want ObsPeakRMS", results[0].Kind)
	}
	if results[1].Kind != ObsZeroCrossing {
		t.Fatalf("Second result kind = %v, want ObsZeroCrossing", results[1].Kind)
	}
}

func TestPipelineEndToEnd(t *testing.T) {
	src := WaveSource{W: Sine(440, 0.5, 0.1, 44100)}
	transforms := []Transform{NormalizeTransform(0)}
	observers := []Observer{NewPeakRMSObserver(), NewFFTObserver(1024)}

	w, obs := Pipeline(src, transforms, observers)
	// After normalize to 0dB, peak should be 1.0
	if math.Abs(w.PeakSample()-1.0) > 1e-6 {
		t.Fatalf("Pipeline peak = %v, want 1.0", w.PeakSample())
	}
	if len(obs) != 2 {
		t.Fatalf("Pipeline obs count = %d, want 2", len(obs))
	}
	// PeakDB should be 0 after normalization
	if math.Abs(obs[0].PeakDB) > 0.1 {
		t.Fatalf("Pipeline PeakDB = %v, want ~0", obs[0].PeakDB)
	}
	// FFT should show 440Hz peak
	if len(obs[1].Spectrum) != 512 {
		t.Fatalf("Pipeline spectrum len = %d, want 512", len(obs[1].Spectrum))
	}
}

func TestChainPreservesLabel(t *testing.T) {
	src := WaveSource{W: Wave{Samples: []float64{0.5}, SampleRate: 44100, Label: "test"}}
	w := Chain(src, NormalizeTransform(0))
	if w.Label != "test/norm" {
		t.Fatalf("Chain label = %q, want %q", w.Label, "test/norm")
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/... -v -run "Chain|Observe|Pipeline"`
Expected: All PASS (implementations already exist from Tasks 1-5).

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/wave/wave_test.go
git commit -m "test(wave): add Chain, Observe, Pipeline integration tests"
```

---

## Task 7: SPSC Ring Buffer

**Files:**
- Create: `src/go/internal/analyzer/ring.go`
- Create: `src/go/internal/analyzer/ring_test.go`

- [ ] **Step 1: Write failing tests for the ring buffer**

```go
// src/go/internal/analyzer/ring_test.go
package analyzer

import (
	"sync"
	"testing"
)

func TestRingBufferWriteRead(t *testing.T) {
	r := NewRingBuffer(8)
	data := []float64{1, 2, 3, 4}
	r.Write(data)
	out := make([]float64, 8)
	n := r.Read(out)
	if n != 4 {
		t.Fatalf("Read n = %d, want 4", n)
	}
	for i := 0; i < 4; i++ {
		if out[i] != data[i] {
			t.Fatalf("out[%d] = %v, want %v", i, out[i], data[i])
		}
	}
}

func TestRingBufferWrap(t *testing.T) {
	r := NewRingBuffer(4)
	r.Write([]float64{1, 2, 3})
	r.Read(make([]float64, 4)) // drain
	r.Write([]float64{4, 5, 6}) // wraps around
	out := make([]float64, 4)
	n := r.Read(out)
	if n != 3 {
		t.Fatalf("Read after wrap n = %d, want 3", n)
	}
	if out[0] != 4 || out[1] != 5 || out[2] != 6 {
		t.Fatalf("Read after wrap = %v, want [4 5 6]", out[:3])
	}
}

func TestRingBufferOverflow(t *testing.T) {
	r := NewRingBuffer(4)
	// Write more than capacity — oldest data is overwritten
	r.Write([]float64{1, 2, 3, 4, 5, 6})
	out := make([]float64, 4)
	n := r.Read(out)
	// Should have the last 4 values
	if n != 4 {
		t.Fatalf("Read after overflow n = %d, want 4", n)
	}
	if out[0] != 3 || out[1] != 4 || out[2] != 5 || out[3] != 6 {
		t.Fatalf("Read after overflow = %v, want [3 4 5 6]", out[:4])
	}
}

func TestRingBufferEmpty(t *testing.T) {
	r := NewRingBuffer(8)
	out := make([]float64, 8)
	n := r.Read(out)
	if n != 0 {
		t.Fatalf("Read empty = %d, want 0", n)
	}
}

func TestRingBufferAvailable(t *testing.T) {
	r := NewRingBuffer(8)
	if r.Available() != 0 {
		t.Fatalf("Available empty = %d, want 0", r.Available())
	}
	r.Write([]float64{1, 2, 3})
	if r.Available() != 3 {
		t.Fatalf("Available after write = %d, want 3", r.Available())
	}
	r.Read(make([]float64, 2))
	if r.Available() != 1 {
		t.Fatalf("Available after partial read = %d, want 1", r.Available())
	}
}

func TestRingBufferConcurrentSPSC(t *testing.T) {
	r := NewRingBuffer(1024)
	const total = 10000
	var wg sync.WaitGroup

	// Producer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < total; i++ {
			r.Write([]float64{float64(i)})
		}
	}()

	// Consumer goroutine
	var received int
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]float64, 64)
		for received < total {
			n := r.Read(buf)
			received += n
			if n == 0 {
				// Spin briefly — SPSC, no blocking needed
				continue
			}
		}
	}()

	wg.Wait()
	if received < total/2 {
		t.Fatalf("Received only %d/%d samples (some lost to overflow is OK)", received, total)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v`
Expected: Compilation failure — package `analyzer` does not exist.

- [ ] **Step 3: Implement SPSC ring buffer**

```go
// src/go/internal/analyzer/ring.go
package analyzer

import "sync/atomic"

// RingBuffer is a single-producer, single-consumer circular buffer for float64 samples.
// Write overwrites oldest data on overflow (never blocks). Read drains available data.
type RingBuffer struct {
	buf   []float64
	cap   int
	write atomic.Int64 // total samples written (monotonic)
	read  atomic.Int64 // total samples read (monotonic)
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]float64, capacity),
		cap: capacity,
	}
}

// Write appends samples to the ring buffer. If data exceeds remaining capacity,
// oldest unread data is overwritten (the read pointer advances).
func (r *RingBuffer) Write(data []float64) {
	n := len(data)
	if n == 0 {
		return
	}
	w := r.write.Load()
	rd := r.read.Load()

	// If writing more than capacity, only keep the last cap samples
	if n > r.cap {
		data = data[n-r.cap:]
		n = r.cap
	}

	// If this write would overflow, advance read pointer
	avail := int(w - rd)
	if avail+n > r.cap {
		advance := int64(avail + n - r.cap)
		r.read.Add(advance)
	}

	// Copy data into ring buffer
	for i := 0; i < n; i++ {
		idx := int((w + int64(i)) % int64(r.cap))
		r.buf[idx] = data[i]
	}
	r.write.Add(int64(n))
}

// Read drains up to len(dst) available samples into dst. Returns the number of
// samples actually read.
func (r *RingBuffer) Read(dst []float64) int {
	rd := r.read.Load()
	w := r.write.Load()
	avail := int(w - rd)
	if avail <= 0 {
		return 0
	}
	n := avail
	if n > len(dst) {
		n = len(dst)
	}
	for i := 0; i < n; i++ {
		idx := int((rd + int64(i)) % int64(r.cap))
		dst[i] = r.buf[idx]
	}
	r.read.Add(int64(n))
	return n
}

// Available returns the number of unread samples.
func (r *RingBuffer) Available() int {
	return int(r.write.Load() - r.read.Load())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/analyzer/ring.go src/go/internal/analyzer/ring_test.go
git commit -m "feat(analyzer): add SPSC ring buffer"
```

---

## Task 8: Analyzer State Types

**Files:**
- Create: `src/go/internal/analyzer/state.go`

- [ ] **Step 1: Implement state types**

```go
// src/go/internal/analyzer/state.go
package analyzer

import "github.com/ingyamilmolinar/beatmo/internal/wave"

// State is the atomically-published snapshot that the UI reads.
type State struct {
	Instruments []InstrumentMetrics // all active instruments (for meter bridge)
	Master      ChannelMetrics      // master output
	Detail      *ChannelMetrics     // selected instrument (full FFT + envelope)
	Capture     *CaptureBuffer      // last-triggered or frozen waveform
	Timestamp   int64               // monotonic nanoseconds
}

// ChannelMetrics holds computed analysis for a single channel.
type ChannelMetrics struct {
	ID       string
	Name     string
	PeakDB   float64
	RMSDB    float64
	ClipCount int
	Waveform []float64 // rolling window for display
	FFTBins  []float64 // magnitude spectrum in dB (nil if not computed)
	FreqBins []float64 // Hz labels (nil if not computed)
	Envelope []float64 // amplitude envelope (nil if not computed)
	Active   bool      // had signal this window
}

// InstrumentMetrics holds lightweight per-instrument data for the meter bridge.
type InstrumentMetrics struct {
	ID        string
	Name      string
	PeakDB    float64
	RMSDB     float64
	ClipCount int
	Active    bool
}

// CaptureBuffer holds a captured waveform for static inspection.
type CaptureBuffer struct {
	InstID     string
	Wave       wave.Wave
	Frozen     bool  // user pinned this capture
	TriggerIdx int64 // monotonic trigger counter
}

// Config configures the analyzer service.
type Config struct {
	FFTSize        int // power of 2 (default 1024)
	WindowSize     int // analysis window in samples (default 2048)
	MaxInstruments int // max instrument slots (default 32)
	SampleRate     int // audio sample rate (default 44100)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		FFTSize:        1024,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v -run NONE`
Expected: `ok` (compiles, no tests matched).

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/analyzer/state.go
git commit -m "feat(analyzer): add State, ChannelMetrics, CaptureBuffer types"
```

---

## Task 9: Analyzer Service

**Files:**
- Create: `src/go/internal/analyzer/service.go`
- Create: `src/go/internal/analyzer/service_test.go`

- [ ] **Step 1: Write failing tests for the analyzer service**

```go
// src/go/internal/analyzer/service_test.go
package analyzer

import (
	"math"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestServicePublishesState(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        256,
		WindowSize:     256,
		MaxInstruments: 4,
		SampleRate:     44100,
	})
	go svc.Run()
	defer svc.Stop()

	// Push a block of sine wave data to the master channel
	sine := wave.Sine(440, 0.5, 0.01, 44100)
	svc.PushMasterBuf(sine.Samples)

	// Wait for analysis to complete
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.Master.Active {
			// Verify metrics are reasonable
			if st.Master.PeakDB < -20 || st.Master.PeakDB > 0 {
				t.Fatalf("Master PeakDB = %v, want between -20 and 0", st.Master.PeakDB)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Service did not publish state within 1s")
}

func TestServiceInstrumentMetrics(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        256,
		WindowSize:     256,
		MaxInstruments: 4,
		SampleRate:     44100,
	})
	svc.RegisterInstrument(0, "kick", "Kick")
	go svc.Run()
	defer svc.Stop()

	sine := wave.Sine(100, 0.8, 0.01, 44100)
	svc.PushInstBuf(0, sine.Samples)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && len(st.Instruments) > 0 && st.Instruments[0].Active {
			if math.Abs(st.Instruments[0].PeakDB-(-1.94)) > 1.0 {
				// 0.8 amplitude = 20*log10(0.8) ≈ -1.94 dB
				t.Fatalf("Instrument PeakDB = %v, want ~-1.94", st.Instruments[0].PeakDB)
			}
			if st.Instruments[0].ID != "kick" {
				t.Fatalf("Instrument ID = %q, want %q", st.Instruments[0].ID, "kick")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Service did not publish instrument metrics within 1s")
}

func TestServiceCaptureTrigger(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        256,
		WindowSize:     256,
		MaxInstruments: 4,
		SampleRate:     44100,
	})
	go svc.Run()
	defer svc.Stop()

	// Trigger with a raw buffer
	buf := make([]float32, 100)
	for i := range buf {
		buf[i] = float32(i) * 0.01
	}
	svc.NotifyTrigger("snare", buf)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.Capture != nil && st.Capture.InstID == "snare" {
			if len(st.Capture.Wave.Samples) != 100 {
				t.Fatalf("Capture len = %d, want 100", len(st.Capture.Wave.Samples))
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Service did not publish capture within 1s")
}

func TestServiceFreeze(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        256,
		WindowSize:     256,
		MaxInstruments: 4,
		SampleRate:     44100,
	})
	go svc.Run()
	defer svc.Stop()

	buf1 := []float32{0.5}
	svc.NotifyTrigger("snare", buf1)
	time.Sleep(50 * time.Millisecond)

	svc.Freeze()

	buf2 := []float32{0.9}
	svc.NotifyTrigger("kick", buf2)
	time.Sleep(50 * time.Millisecond)

	st := svc.State()
	if st == nil || st.Capture == nil {
		t.Fatal("No capture after freeze")
	}
	if st.Capture.InstID != "snare" {
		t.Fatalf("Frozen capture ID = %q, want %q (first trigger)", st.Capture.InstID, "snare")
	}
	if !st.Capture.Frozen {
		t.Fatal("Capture not marked as frozen")
	}
}

func TestServiceDetailChannel(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        256,
		WindowSize:     256,
		MaxInstruments: 4,
		SampleRate:     44100,
	})
	svc.RegisterInstrument(0, "snare", "Snare")
	svc.SetDetailChannel("snare")
	go svc.Run()
	defer svc.Stop()

	sine := wave.Sine(440, 0.5, 0.01, 44100)
	svc.PushInstBuf(0, sine.Samples)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		st := svc.State()
		if st != nil && st.Detail != nil && st.Detail.Active {
			if st.Detail.FFTBins == nil {
				t.Fatal("Detail FFTBins should be populated")
			}
			if st.Detail.ID != "snare" {
				t.Fatalf("Detail ID = %q, want %q", st.Detail.ID, "snare")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Service did not publish detail metrics within 1s")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v`
Expected: Compilation failure — `NewService` not defined.

- [ ] **Step 3: Implement the analyzer service**

```go
// src/go/internal/analyzer/service.go
package analyzer

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Service orchestrates live audio analysis on a dedicated goroutine.
// The audio thread pushes data via PushInstBuf/PushMasterBuf (memcopy only).
// The goroutine runs observers and publishes State atomically.
type Service struct {
	cfg Config

	// Ring buffers: one per instrument slot + one for master
	instRings  []*RingBuffer
	masterRing *RingBuffer

	// Instrument metadata (set once at registration, read from goroutine)
	instMeta []instMeta
	metaMu   sync.RWMutex

	// Capture
	pendingCapture atomic.Pointer[pendingCap]
	frozen         atomic.Bool

	// Detail channel selection
	detailID atomic.Pointer[string]

	// Published state (atomic pointer swap)
	state atomic.Pointer[State]

	// Lifecycle
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Observers (created once)
	peakRMS  wave.Observer
	fftObs   wave.Observer
	envObs   wave.Observer

	// Working buffers (reused across cycles, goroutine-local)
	workBuf []float64
}

type instMeta struct {
	id   string
	name string
}

type pendingCap struct {
	instID string
	buf    []float64
}

// NewService creates an analyzer service. Call Run() in a goroutine.
func NewService(cfg Config) *Service {
	if cfg.FFTSize == 0 {
		cfg = DefaultConfig()
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}
	rings := make([]*RingBuffer, cfg.MaxInstruments)
	for i := range rings {
		rings[i] = NewRingBuffer(cfg.WindowSize * 2)
	}
	s := &Service{
		cfg:        cfg,
		instRings:  rings,
		masterRing: NewRingBuffer(cfg.WindowSize * 2),
		instMeta:   make([]instMeta, cfg.MaxInstruments),
		stopCh:     make(chan struct{}),
		peakRMS:    wave.NewPeakRMSObserver(),
		fftObs:     wave.NewFFTObserver(cfg.FFTSize),
		envObs:     wave.NewEnvelopeObserver(1.0, 50.0),
		workBuf:    make([]float64, cfg.WindowSize*2),
	}
	return s
}

// RegisterInstrument sets metadata for an instrument slot.
func (s *Service) RegisterInstrument(slot int, id, name string) {
	if slot < 0 || slot >= s.cfg.MaxInstruments {
		return
	}
	s.metaMu.Lock()
	s.instMeta[slot] = instMeta{id: id, name: name}
	s.metaMu.Unlock()
}

// PushInstBuf is called from the audio thread. Copies buf into the instrument's ring buffer.
func (s *Service) PushInstBuf(slot int, buf []float64) {
	if slot >= 0 && slot < len(s.instRings) {
		s.instRings[slot].Write(buf)
	}
}

// PushMasterBuf is called from the audio thread. Copies buf into the master ring buffer.
func (s *Service) PushMasterBuf(buf []float64) {
	s.masterRing.Write(buf)
}

// NotifyTrigger is called when a voice is scheduled. Copies the raw rendered buffer.
func (s *Service) NotifyTrigger(instID string, rawBuf []float32) {
	if s.frozen.Load() {
		return
	}
	samples := make([]float64, len(rawBuf))
	for i, v := range rawBuf {
		samples[i] = float64(v)
	}
	cap := &pendingCap{instID: instID, buf: samples}
	s.pendingCapture.Store(cap)
}

// Freeze pins the current capture.
func (s *Service) Freeze() { s.frozen.Store(true) }

// Unfreeze resumes live capture updates.
func (s *Service) Unfreeze() { s.frozen.Store(false) }

// SetDetailChannel selects which instrument gets full FFT/envelope analysis.
func (s *Service) SetDetailChannel(id string) {
	s.detailID.Store(&id)
}

// State returns the latest published state. Lock-free atomic read.
func (s *Service) State() *State {
	return s.state.Load()
}

// Run starts the analyzer goroutine. Blocks until Stop() is called.
func (s *Service) Run() {
	s.wg.Add(1)
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(float64(time.Second) * float64(s.cfg.WindowSize) / float64(s.cfg.SampleRate)))
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.analyze()
		}
	}
}

// Stop signals the goroutine to exit and waits for it.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Service) analyze() {
	st := &State{Timestamp: time.Now().UnixNano()}

	// Read instrument metadata snapshot
	s.metaMu.RLock()
	metaSnap := make([]instMeta, len(s.instMeta))
	copy(metaSnap, s.instMeta)
	s.metaMu.RUnlock()

	detailPtr := s.detailID.Load()
	var detailID string
	if detailPtr != nil {
		detailID = *detailPtr
	}

	// Analyze each instrument
	st.Instruments = make([]InstrumentMetrics, 0, s.cfg.MaxInstruments)
	for slot := 0; slot < s.cfg.MaxInstruments; slot++ {
		meta := metaSnap[slot]
		if meta.id == "" {
			continue
		}
		ring := s.instRings[slot]
		n := ring.Read(s.workBuf)
		if n == 0 {
			st.Instruments = append(st.Instruments, InstrumentMetrics{
				ID: meta.id, Name: meta.name,
			})
			continue
		}
		w := wave.Wave{Samples: s.workBuf[:n], SampleRate: s.cfg.SampleRate}
		obs := s.peakRMS.Observe(w)
		im := InstrumentMetrics{
			ID:        meta.id,
			Name:      meta.name,
			PeakDB:    obs.PeakDB,
			RMSDB:     obs.RMSDB,
			ClipCount: obs.ClipCount,
			Active:    true,
		}
		st.Instruments = append(st.Instruments, im)

		// Detail channel: full FFT + envelope
		if meta.id == detailID {
			wClone := w.Clone()
			fftObs := s.fftObs.Observe(wClone)
			envObs := s.envObs.Observe(wClone)
			waveSnap := make([]float64, n)
			copy(waveSnap, s.workBuf[:n])
			st.Detail = &ChannelMetrics{
				ID:        meta.id,
				Name:      meta.name,
				PeakDB:    obs.PeakDB,
				RMSDB:     obs.RMSDB,
				ClipCount: obs.ClipCount,
				Waveform:  waveSnap,
				FFTBins:   fftObs.Spectrum,
				FreqBins:  fftObs.FreqBins,
				Envelope:  envObs.Envelope,
				Active:    true,
			}
		}
	}

	// Analyze master
	n := s.masterRing.Read(s.workBuf)
	if n > 0 {
		w := wave.Wave{Samples: s.workBuf[:n], SampleRate: s.cfg.SampleRate}
		obs := s.peakRMS.Observe(w)
		fftObs := s.fftObs.Observe(w)
		waveSnap := make([]float64, n)
		copy(waveSnap, s.workBuf[:n])
		st.Master = ChannelMetrics{
			ID:        "master",
			Name:      "Master",
			PeakDB:    obs.PeakDB,
			RMSDB:     obs.RMSDB,
			ClipCount: obs.ClipCount,
			Waveform:  waveSnap,
			FFTBins:   fftObs.Spectrum,
			FreqBins:  fftObs.FreqBins,
			Active:    true,
		}
	}

	// Process capture trigger
	if pc := s.pendingCapture.Swap(nil); pc != nil && !s.frozen.Load() {
		st.Capture = &CaptureBuffer{
			InstID: pc.instID,
			Wave:   wave.Wave{Samples: pc.buf, SampleRate: s.cfg.SampleRate, Label: pc.instID},
		}
	} else {
		// Carry forward existing capture
		prev := s.state.Load()
		if prev != nil && prev.Capture != nil {
			st.Capture = prev.Capture
		}
	}

	// Handle freeze
	if s.frozen.Load() && st.Capture != nil {
		st.Capture.Frozen = true
	}

	// Atomic publish
	s.state.Store(st)
}

// stateSize returns the unsafe.Sizeof for documentation — not used at runtime.
var _ = unsafe.Sizeof(State{})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/analyzer/... -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/analyzer/service.go src/go/internal/analyzer/state.go src/go/internal/analyzer/service_test.go
git commit -m "feat(analyzer): add Service with goroutine, ring buffers, and atomic state publishing"
```

---

## Task 10: Audio Engine Integration

**Files:**
- Modify: `src/go/internal/audio/engine.go`
- Modify: `src/go/internal/audio/engine_stop.go`
- Modify: `src/go/internal/audio/engine_mixer.go`

This task adds the minimal integration hooks — memcopy calls from the audio thread into the analyzer service.

- [ ] **Step 1: Read the current engine files to identify exact insertion points**

Read `engine.go` for the init/startup code. Read `engine_stop.go` for Phase 1 and Phase 3 completion points. Read `engine_mixer.go` for the Schedule() function.

- [ ] **Step 2: Add analyzer service initialization to engine.go**

Add after the existing global variables and init code:

```go
import "github.com/ingyamilmolinar/beatmo/internal/analyzer"

// Global analyzer service — nil until audio is initialized.
var analyzerSvc *analyzer.Service

// AnalyzerService returns the global analyzer for UI consumption.
func AnalyzerService() *analyzer.Service { return analyzerSvc }
```

In the audio initialization path (where `setupMasterCompressor` is called), add:

```go
analyzerSvc = analyzer.NewService(analyzer.Config{
    FFTSize:        1024,
    WindowSize:     2048,
    MaxInstruments: 32,
    SampleRate:     sampleRate,
})
go analyzerSvc.Run()
```

- [ ] **Step 3: Add push calls to engine_stop.go**

After Phase 1 voice rendering loop completes (after the `activeSlots` loop), add:

```go
if analyzerSvc != nil {
    for _, slot := range activeSlots {
        analyzerSvc.PushInstBuf(slot, instBufs[slot][:blockSize])
    }
}
```

After Phase 3 master processing completes (after workBuf is filled), add:

```go
if analyzerSvc != nil {
    analyzerSvc.PushMasterBuf(workBuf[:blockSize])
}
```

- [ ] **Step 4: Add trigger notification to engine_mixer.go**

In `Schedule()`, after the voice is wrapped in antiPopVoice but before adding to pending queue, add:

```go
if analyzerSvc != nil {
    if cv, ok := v.(*cVoice); ok {
        analyzerSvc.NotifyTrigger(id, cv.buf)
    }
}
```

Note: The exact type assertion depends on how the unwrapped voice is accessible. Check if the raw buffer is available on the voice before wrapping. If the voice is already wrapped at this point, capture from the original `cVoice` buffer passed to `Schedule`.

- [ ] **Step 5: Verify compilation**

Run: `cd src/go && ../../.tools/go/bin/go build ./...`
Expected: Builds successfully (the analyzer import resolves).

- [ ] **Step 6: Run full test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 60s ./...`
Expected: All existing tests still pass. The analyzer hooks are guarded by nil checks so they're no-ops in test builds.

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/audio/engine.go src/go/internal/audio/engine_stop.go src/go/internal/audio/engine_mixer.go
git commit -m "feat(analyzer): integrate analyzer service with audio engine (memcopy hooks)"
```

---

## Task 11: UI Tab System + Expand/Collapse

**Files:**
- Create: `src/go/internal/ui/panel_tab.go`
- Create: `src/go/internal/ui/panel_tab_test.go`
- Modify: `src/go/internal/ui/eq_panel_zone.go`

- [ ] **Step 1: Write failing tests for tab system**

```go
// src/go/internal/ui/panel_tab_test.go
package ui

import "testing"

func TestPanelTabDefault(t *testing.T) {
	assertDefaultParityState(t)
	pt := NewPanelTabState()
	if pt.ActiveTab() != TabEQ {
		t.Fatalf("Default tab = %v, want TabEQ", pt.ActiveTab())
	}
	if pt.Expanded() {
		t.Fatal("Default should be collapsed")
	}
}

func TestPanelTabSwitch(t *testing.T) {
	assertDefaultParityState(t)
	pt := NewPanelTabState()
	pt.SetActiveTab(TabWave)
	if pt.ActiveTab() != TabWave {
		t.Fatalf("Tab after switch = %v, want TabWave", pt.ActiveTab())
	}
}

func TestPanelTabExpand(t *testing.T) {
	assertDefaultParityState(t)
	pt := NewPanelTabState()
	pt.ToggleExpanded()
	if !pt.Expanded() {
		t.Fatal("Should be expanded after toggle")
	}
	pt.ToggleExpanded()
	if pt.Expanded() {
		t.Fatal("Should be collapsed after second toggle")
	}
}

func TestPanelTabCollapsedHeight(t *testing.T) {
	assertDefaultParityState(t)
	pt := NewPanelTabState()
	if pt.PanelHeight() != eqPanelHeight {
		t.Fatalf("Collapsed height = %d, want %d", pt.PanelHeight(), eqPanelHeight)
	}
}

func TestPanelTabExpandedHeight(t *testing.T) {
	assertDefaultParityState(t)
	pt := NewPanelTabState()
	pt.ToggleExpanded()
	expected := eqPanelHeight * 2
	if pt.PanelHeight() != expected {
		t.Fatalf("Expanded height = %d, want %d", pt.PanelHeight(), expected)
	}
}

func TestPanelTabLabels(t *testing.T) {
	assertDefaultParityState(t)
	tabs := AllPanelTabs()
	if len(tabs) != 4 {
		t.Fatalf("Tab count = %d, want 4", len(tabs))
	}
	expected := []string{"Wave", "Spectrum", "Meters", "EQ"}
	for i, tab := range tabs {
		if PanelTabLabel(tab) != expected[i] {
			t.Fatalf("Tab %d label = %q, want %q", i, PanelTabLabel(tab), expected[i])
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -v -run PanelTab`
Expected: Compilation failure — `NewPanelTabState` not defined.

- [ ] **Step 3: Implement PanelTab types and state**

```go
// src/go/internal/ui/panel_tab.go
package ui

// PanelTab identifies which view is active in the analyzer panel.
type PanelTab int

const (
	TabWave     PanelTab = iota
	TabSpectrum
	TabMeters
	TabEQ
)

// AllPanelTabs returns all tab values in display order.
func AllPanelTabs() []PanelTab {
	return []PanelTab{TabWave, TabSpectrum, TabMeters, TabEQ}
}

// PanelTabLabel returns the display label for a tab.
func PanelTabLabel(tab PanelTab) string {
	switch tab {
	case TabWave:
		return "Wave"
	case TabSpectrum:
		return "Spectrum"
	case TabMeters:
		return "Meters"
	case TabEQ:
		return "EQ"
	default:
		return "?"
	}
}

// PanelTabState manages the active tab and expanded state.
type PanelTabState struct {
	activeTab PanelTab
	expanded  bool
}

// NewPanelTabState returns a state with EQ tab active and collapsed (matching current behavior).
func NewPanelTabState() *PanelTabState {
	return &PanelTabState{activeTab: TabEQ}
}

// ActiveTab returns the currently active tab.
func (p *PanelTabState) ActiveTab() PanelTab { return p.activeTab }

// SetActiveTab switches the active tab.
func (p *PanelTabState) SetActiveTab(tab PanelTab) { p.activeTab = tab }

// Expanded returns whether the panel is expanded.
func (p *PanelTabState) Expanded() bool { return p.expanded }

// ToggleExpanded toggles the expanded state.
func (p *PanelTabState) ToggleExpanded() { p.expanded = !p.expanded }

// PanelHeight returns the current panel height in pixels.
func (p *PanelTabState) PanelHeight() int {
	if p.expanded {
		return eqPanelHeight * 2
	}
	return eqPanelHeight
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -v -run PanelTab`
Expected: All PASS.

- [ ] **Step 5: Integrate PanelTabState into EQPanelZone**

Modify `eq_panel_zone.go`:
- Replace `waveformMode bool` field with `tabState *PanelTabState`
- Replace the "Wave"/"EQ" toggle button with a tab bar that creates a button per tab
- Route `Draw()` based on `tabState.ActiveTab()` instead of `waveformMode`
- Add expand/collapse button
- Keep the `WaveformMode()` / `SetWaveformMode()` methods as compatibility shims that map `true → TabWave`, `false → TabEQ`

The exact diff depends on the current state of `eq_panel_zone.go`. Read the file, identify all references to `waveformMode`, and replace them with `tabState` equivalents.

- [ ] **Step 6: Run full UI test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -timeout 60s`
Expected: All existing tests pass (compatibility shims preserve behavior).

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/ui/panel_tab.go src/go/internal/ui/panel_tab_test.go src/go/internal/ui/eq_panel_zone.go
git commit -m "feat(ui): add tabbed panel system with expand/collapse, replace waveformMode toggle"
```

---

## Task 12: Waveform Renderer

**Files:**
- Create: `src/go/internal/ui/render_waveform.go`

- [ ] **Step 1: Implement the enhanced waveform renderer**

Read the existing `drumview_draw_waveform.go` for the current rendering pattern. The new renderer follows the same `drawRect`-per-pixel-column technique but reads from `analyzer.State()` instead of `audio.AnalyzerSnapshot`.

```go
// src/go/internal/ui/render_waveform.go
package ui

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// drawAnalyzerWaveform renders the waveform from the analyzer state.
// rect defines the drawing area. If capture is non-nil and frozen, draws
// the captured waveform instead of the live rolling window.
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	if state == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Choose waveform source: frozen capture or live master
	var wave []float64
	var label string
	var peakDB, rmsDB float64
	var clipCount int

	if state.Capture != nil && state.Capture.Frozen {
		wave = state.Capture.Wave.Samples
		label = state.Capture.InstID + " (frozen)"
		obs := state.Capture.Wave.PeakSample()
		peakDB = 20 * math.Log10(math.Max(obs, 1e-10))
	} else if state.Master.Active {
		wave = state.Master.Waveform
		label = "master"
		peakDB = state.Master.PeakDB
		rmsDB = state.Master.RMSDB
		clipCount = state.Master.ClipCount
	}

	if len(wave) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Draw midline
	midY := rect.Min.Y + rect.Dy()/2
	drawRect(dst, image.Rect(rect.Min.X, midY, rect.Max.X, midY+1), colWaveMid, true)

	// Draw waveform trace
	width := rect.Dx()
	if width <= 0 {
		return
	}
	drawWaveTrace(dst, wave, rect, midY, width, colWaveTrace)

	// Draw inline stats (top-right)
	statsStr := fmt.Sprintf("Peak: %.1fdB", peakDB)
	if rmsDB != 0 {
		statsStr += fmt.Sprintf(" | RMS: %.1fdB", rmsDB)
	}
	if clipCount > 0 {
		statsStr += fmt.Sprintf(" | CLIP: %d", clipCount)
	}
	_ = statsStr // TODO: draw text when font rendering is wired up
	_ = label
}

// drawWaveTrace draws a single waveform trace using the min/max-per-pixel-column technique.
func drawWaveTrace(dst *ebiten.Image, wave []float64, rect image.Rectangle, midY, width int, col interface{ RGBA() (uint32, uint32, uint32, uint32) }) {
	step := float64(len(wave)) / float64(width)
	for x := 0; x < width; x++ {
		start := int(float64(x) * step)
		end := int(float64(x+1) * step)
		if start >= len(wave) {
			start = len(wave) - 1
		}
		if end <= start {
			end = start + 1
		}
		if end > len(wave) {
			end = len(wave)
		}
		minV, maxV := 1.0, -1.0
		for i := start; i < end; i++ {
			v := wave[i]
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
		halfH := float64(rect.Dy()) * 0.48
		y0 := midY - int(maxV*halfH)
		y1 := midY - int(minV*halfH)
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		if y1 < rect.Min.Y {
			y1 = rect.Min.Y
		}
		if y0 > rect.Max.Y {
			y0 = rect.Max.Y
		}
		if y0 == y1 {
			y1 = y0 + 1
		}
		drawRect(dst, image.Rect(rect.Min.X+x, y0, rect.Min.X+x+1, y1), col, true)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add src/go/internal/ui/render_waveform.go
git commit -m "feat(ui): add enhanced waveform renderer reading from analyzer state"
```

---

## Task 13: Spectrum Renderer

**Files:**
- Create: `src/go/internal/ui/render_spectrum.go`

- [ ] **Step 1: Implement the spectrum bar renderer**

```go
// src/go/internal/ui/render_spectrum.go
package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Spectrum display constants
var spectrumBarColor = color.RGBA{124, 108, 240, 255}
var spectrumBgColor = color.RGBA{26, 26, 46, 255}

// drawAnalyzerSpectrum renders FFT magnitude bars within the given rect.
// Uses logarithmic frequency grouping into ISO bands.
func drawAnalyzerSpectrum(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	if state == nil || !state.Master.Active || len(state.Master.FFTBins) == 0 {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	bins := state.Master.FFTBins
	freqs := state.Master.FreqBins

	// ISO band edges (Hz)
	bands := [][2]float64{
		{20, 44}, {44, 88}, {88, 177}, {177, 355},
		{355, 710}, {710, 1420}, {1420, 2840},
		{2840, 5680}, {5680, 11360}, {11360, 22000},
	}

	barWidth := rect.Dx() / len(bands)
	if barWidth < 2 {
		barWidth = 2
	}
	gap := 1
	maxDB := 0.0
	minDB := -80.0

	for i, band := range bands {
		// Average magnitude in this band
		var sum float64
		var count int
		for j, f := range freqs {
			if f >= band[0] && f < band[1] && j < len(bins) {
				sum += bins[j]
				count++
			}
		}
		if count == 0 {
			continue
		}
		avgDB := sum / float64(count)

		// Clamp and normalize to 0-1
		if avgDB < minDB {
			avgDB = minDB
		}
		if avgDB > maxDB {
			avgDB = maxDB
		}
		normalized := (avgDB - minDB) / (maxDB - minDB)

		// Draw bar
		barH := int(normalized * float64(rect.Dy()-4))
		x := rect.Min.X + i*barWidth + gap
		y := rect.Max.Y - barH - 2
		w := barWidth - gap*2
		if w < 1 {
			w = 1
		}

		// Gradient: brighter at top
		alpha := uint8(math.Min(255, 128+normalized*127))
		barCol := color.RGBA{spectrumBarColor.R, spectrumBarColor.G, spectrumBarColor.B, alpha}
		drawRect(dst, image.Rect(x, y, x+w, rect.Max.Y-2), barCol, true)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add src/go/internal/ui/render_spectrum.go
git commit -m "feat(ui): add FFT spectrum bar renderer with ISO band grouping"
```

---

## Task 14: Meter Bridge Renderer

**Files:**
- Create: `src/go/internal/ui/render_meters.go`

- [ ] **Step 1: Implement the meter bridge renderer**

```go
// src/go/internal/ui/render_meters.go
package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// Meter color constants
var (
	meterGreen  = color.RGBA{90, 180, 90, 255}
	meterYellow = color.RGBA{200, 200, 60, 255}
	meterRed    = color.RGBA{220, 60, 60, 255}
	meterBg     = color.RGBA{34, 34, 50, 255}
	meterClip   = color.RGBA{255, 40, 40, 255}
)

// drawMeterBridge renders horizontal meter bars for all active instruments plus master.
// Each instrument gets one row. Master is at the bottom, visually separated.
func drawMeterBridge(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State) {
	if state == nil {
		drawRect(dst, rect, colButtonBorder, false)
		return
	}

	// Count active instruments + master
	active := make([]analyzer.InstrumentMetrics, 0, len(state.Instruments))
	for _, im := range state.Instruments {
		if im.ID != "" {
			active = append(active, im)
		}
	}

	totalRows := len(active) + 1 // +1 for master
	if totalRows == 0 {
		return
	}

	rowH := rect.Dy() / totalRows
	if rowH < 8 {
		rowH = 8
	}
	if rowH > 24 {
		rowH = 24
	}
	barH := rowH - 3 // leave gap between rows
	labelW := 50     // pixels reserved for instrument name

	for i, im := range active {
		y := rect.Min.Y + i*rowH
		drawMeterRow(dst, rect.Min.X, y, rect.Dx(), barH, labelW, im.Name, im.PeakDB, im.RMSDB, im.ClipCount)
	}

	// Master meter at bottom (with separator)
	masterY := rect.Min.Y + len(active)*rowH
	sepY := masterY - 1
	drawRect(dst, image.Rect(rect.Min.X, sepY, rect.Max.X, sepY+1), colWaveMid, true)
	drawMeterRow(dst, rect.Min.X, masterY, rect.Dx(), barH, labelW, "Master", state.Master.PeakDB, state.Master.RMSDB, state.Master.ClipCount)
}

func drawMeterRow(dst *ebiten.Image, x, y, width, height, labelW int, name string, peakDB, rmsDB float64, clips int) {
	barX := x + labelW
	barW := width - labelW - 20 // reserve space for clip indicator

	// Background
	drawRect(dst, image.Rect(barX, y, barX+barW, y+height), meterBg, true)

	// Peak bar (normalized: -60dB = 0%, 0dB = 100%)
	peakNorm := (peakDB + 60) / 60
	if peakNorm < 0 {
		peakNorm = 0
	}
	if peakNorm > 1 {
		peakNorm = 1
	}
	peakW := int(peakNorm * float64(barW))
	peakCol := meterGreen
	if peakDB > -6 {
		peakCol = meterYellow
	}
	if peakDB > -1 {
		peakCol = meterRed
	}
	drawRect(dst, image.Rect(barX, y, barX+peakW, y+height), peakCol, true)

	// RMS overlay (slightly dimmer, thinner)
	rmsNorm := (rmsDB + 60) / 60
	if rmsNorm < 0 {
		rmsNorm = 0
	}
	if rmsNorm > 1 {
		rmsNorm = 1
	}
	rmsW := int(rmsNorm * float64(barW))
	rmsCol := color.RGBA{peakCol.R, peakCol.G, peakCol.B, 128}
	innerH := height / 2
	innerY := y + (height-innerH)/2
	drawRect(dst, image.Rect(barX, innerY, barX+rmsW, innerY+innerH), rmsCol, true)

	// Clip indicator
	clipX := barX + barW + 4
	if clips > 0 {
		drawRect(dst, image.Rect(clipX, y, clipX+12, y+height), meterClip, true)
	}

	_ = name // TODO: draw text label when font rendering is wired up
	_ = math.Abs(0) // suppress unused import
}
```

- [ ] **Step 2: Commit**

```bash
git add src/go/internal/ui/render_meters.go
git commit -m "feat(ui): add meter bridge renderer for all instruments + master"
```

---

## Task 15: Wire Tab Renderers into EQPanelZone

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go`
- Modify: `src/go/internal/ui/drumview_ctor.go`

- [ ] **Step 1: Update EQPanelZone.Draw() to route to tab renderers**

In `eq_panel_zone.go`, modify the `Draw()` method. Replace the current `if z.waveformMode` block with a switch on `z.tabState.ActiveTab()`:

```go
switch z.tabState.ActiveTab() {
case TabWave:
    svc := audio.AnalyzerService()
    var state *analyzer.State
    if svc != nil {
        state = svc.State()
    }
    drawAnalyzerWaveform(screen, z.eqContentRect(), state)
case TabSpectrum:
    svc := audio.AnalyzerService()
    var state *analyzer.State
    if svc != nil {
        state = svc.State()
    }
    drawAnalyzerSpectrum(screen, z.eqContentRect(), state)
case TabMeters:
    svc := audio.AnalyzerService()
    var state *analyzer.State
    if svc != nil {
        state = svc.State()
    }
    drawMeterBridge(screen, z.eqContentRect(), state)
case TabEQ:
    // Existing EQ drawing code (unchanged)
    z.drawSpectrum(screen)
    z.drawEQCurve(screen)
    z.drawEQSliders(screen)
}
```

Add a helper `eqContentRect()` that returns the content area (rect minus tab bar height).

- [ ] **Step 2: Update tab bar rendering**

Replace the single toggle button with a row of tab buttons in `initButtons()`. Each tab button calls `z.tabState.SetActiveTab(tab)`. Add an expand/collapse button at the right edge.

- [ ] **Step 3: Update drumview_ctor.go**

The `DrawWaveform` callback in `EQCallbacks` can now be removed or simplified — the zone draws directly from the analyzer service. Update the callback wiring to pass the analyzer state if needed.

- [ ] **Step 4: Run full test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 60s ./...`
Expected: All tests pass. The tab system defaults to TabEQ which preserves existing behavior.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/eq_panel_zone.go src/go/internal/ui/drumview_ctor.go
git commit -m "feat(ui): wire tab renderers into EQPanelZone, route Wave/Spectrum/Meters/EQ"
```

---

## Task 16: Makefile + Final Integration Test

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Update Makefile test targets**

Ensure the `wave` and `analyzer` packages are included in test runs. The existing `./...` glob should already catch them, but verify. If the Makefile has explicit package lists, add:

```makefile
# In the test target, ensure these packages are included:
# ./internal/wave/... ./internal/analyzer/...
```

- [ ] **Step 2: Run the full test pipeline**

Run: `make test`
Expected: All tests pass, including the new `wave` and `analyzer` packages.

- [ ] **Step 3: Run the application**

Run: `make run` (desktop) or `make wasm` + browser to verify the UI renders without crashes. The panel should show the EQ tab by default. Switching to Wave/Spectrum/Meters tabs should show content when audio is playing.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "chore: ensure wave and analyzer packages included in test pipeline"
```
