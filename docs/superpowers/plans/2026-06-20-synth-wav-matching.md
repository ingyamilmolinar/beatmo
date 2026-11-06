# Analysis-Driven Synth Matching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tune Beatmo synth instruments to measurably approach real instrument WAV recordings by searching the modular-synth parameter space against a shared, tested DSP fingerprint, then auto-patching the instrument seed.

**Architecture:** A single DSP core (`internal/wave`) is shared by the game, a new pure-DSP fingerprint library (`internal/audio/fingerprint`), the optimizer (`internal/audio/synthmatch`), and all tests. The optimizer renders a synth at the reference's detected pitch, fingerprints it, minimizes one canonical `Distance()` via Hooke–Jeeves coordinate descent, and rewrites the `var …Seed` block in `instrument_seeds.go`. Two thin CLIs (`cmd/synth-analyze`, `cmd/synth-match`) and make targets drive it. Real reference WAVs come from University of Iowa MIS (AIFF→WAV via python stdlib).

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `internal/wave` DSP, `internal/audio` recipe render, python3 stdlib (`aifc`/`wave`) for AIFF→WAV, bash fetch script.

## Global Constraints

- **Use the bundled Go:** `../../.tools/go/bin/go` from `src/go` (avoid version drift).
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
- **No duplication of DSP:** all FFT/window/normalize/envelope go through `internal/wave`. Never reintroduce a private FFT. `internal/audio/fingerprint` imports **only** `internal/wave` (+ stdlib). `internal/audio/synthmatch` imports the parent `internal/audio` + `fingerprint`; the parent `audio` package must NEVER import either subpackage (cycle guard).
- **CLIs only:** `cmd/*` files carry `//go:build !test && !js`. Library packages carry **no** build tags (must compile under `-tags test`).
- **Determinism:** optimizer uses a fixed `math/rand` seed; no `time.Now`/unseeded RNG in library code.
- **Do NOT `git add`/`git commit`** — standing repo rule: leave all work uncommitted for the user to review. Treat every "Commit" step below as **"Checkpoint: stop, summarize, let the user review the diff."** Do not stage.
- **Reference WAVs:** real recordings only, license documented per file in `refs/MANIFEST.md`. Tests must pass with `refs/` empty (skip-if-absent).

---

### Task 1: Shared linear-magnitude FFT primitive in `wave`

Add one exported function so the fingerprint reuses `wave`'s single FFT instead of duplicating it, and refactor the dB observer onto it.

**Files:**
- Modify: `src/go/internal/wave/fft.go`
- Test: `src/go/internal/wave/fft_test.go`

**Interfaces:**
- Produces: `func MagnitudeSpectrum(w Wave, fftSize int, window WindowType) (mag []float64, binHz float64)` — linear magnitude, `fftSize/2+1` bins (DC→Nyquist), zero-padded/truncated from the **start** of `w.Samples`, `window` applied (reuse `WindowType` from transforms.go).

- [ ] **Step 1: Write the failing test**

```go
func TestMagnitudeSpectrum_SinePeakBin(t *testing.T) {
	sr := 44100
	w := Sine(1000, 1.0, 0.2, sr) // exported testgen
	mag, binHz := MagnitudeSpectrum(w, 16384, WindowHann)
	if len(mag) != 16384/2+1 {
		t.Fatalf("got %d bins, want %d", len(mag), 16384/2+1)
	}
	// Peak bin should be nearest 1000 Hz.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/ -run TestMagnitudeSpectrum_SinePeakBin`
Expected: FAIL — `undefined: MagnitudeSpectrum` (confirm `WindowHann` is the real const name in transforms.go; if it differs, use the actual name).

- [ ] **Step 3: Implement `MagnitudeSpectrum` and refactor the observer**

In `fft.go` add:

```go
// MagnitudeSpectrum returns the linear magnitude spectrum (fftSize/2+1 bins,
// DC..Nyquist) of the first fftSize samples of w (zero-padded if shorter),
// after applying the given window. binHz is the per-bin frequency resolution.
// This is the single shared FFT entry point for analysis; NewFFTObserver
// builds its dB output on top of it.
func MagnitudeSpectrum(w Wave, fftSize int, window WindowType) (mag []float64, binHz float64) {
	n := nextPow2(fftSize)
	sr := w.SampleRate
	if sr == 0 {
		sr = sr44100Default
	}
	seg := make([]float64, n)
	copy(seg, w.Samples) // from the start; truncates if longer, zero-pads if shorter
	applyWindowInPlace(seg, window)
	buf := make([]complex128, n)
	for i, s := range seg {
		buf[i] = complex(s, 0)
	}
	fft(buf)
	numBins := n/2 + 1
	mag = make([]float64, numBins)
	for i := 0; i < numBins; i++ {
		mag[i] = cmplx.Abs(buf[i])
	}
	return mag, float64(sr) / float64(n)
}
```

If `transforms.go` does not already expose an in-place window helper, add `applyWindowInPlace(seg []float64, wt WindowType)` there mirroring the existing `WindowTransform` coefficients (Hann/Hamming/etc.), and have `WindowTransform` call it. Refactor `(*fftObserver).Observe` to call `MagnitudeSpectrum(w, o.size, WindowHann)` then convert to dB (`20*log10(max(mag,floor))`), preserving existing `Bins/FreqBins/BinHz/PeakFreq/PeakMag` semantics.

- [ ] **Step 4: Run tests to verify pass (incl. existing wave tests)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wave/`
Expected: PASS (new test + all existing fft/observer tests unchanged).

- [ ] **Step 5: Checkpoint** (review; do not commit)

---

### Task 2: `fingerprint` package skeleton + signal helpers

**Files:**
- Create: `src/go/internal/audio/fingerprint/fingerprint.go`
- Create: `src/go/internal/audio/fingerprint/signal.go`
- Test: `src/go/internal/audio/fingerprint/signal_test.go`

**Interfaces:**
- Produces:
  - `type Fingerprint struct { … }` (fields filled across Tasks 3–8; start with `Label string; SampleRate int`).
  - `func PeakNormalize(w wave.Wave) wave.Wave` (reuse `wave.NormalizeTransform` to 0 dBFS).
  - `func RMSEnvelope(w wave.Wave, hop, nPoints int) []float64`.
  - `func AutoSegment(w wave.Wave, durSec float64) wave.Wave` (loudest sustained window; ported from `cmd/synth-analyze`).
  - `const SustainSkipSec = 0.3; const SustainLenSec = 1.3` (shared segmentation policy).

- [ ] **Step 1: Write the failing test**

```go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestRMSEnvelope_ConstantAmplitude(t *testing.T) {
	w := wave.Sine(220, 0.5, 1.0, 44100)
	env := RMSEnvelope(w, 256, 50)
	if len(env) == 0 {
		t.Fatal("empty envelope")
	}
	// Sine RMS = amp/sqrt(2) ≈ 0.3536, roughly constant in the middle.
	mid := env[len(env)/2]
	if math.Abs(mid-0.3536) > 0.05 {
		t.Errorf("mid RMS=%.4f want ~0.3536", mid)
	}
}

func TestAutoSegment_PicksLoudRegion(t *testing.T) {
	quiet := wave.Sine(220, 0.01, 0.5, 44100)
	loud := wave.Sine(220, 0.9, 0.5, 44100)
	cat := wave.Wave{Samples: append(append([]float64{}, quiet.Samples...), loud.Samples...), SampleRate: 44100}
	seg := AutoSegment(cat, 0.3)
	if seg.PeakSample() < 0.5 {
		t.Errorf("autosegment peak %.3f — picked quiet region", seg.PeakSample())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement skeleton + helpers**

Create `fingerprint.go` with the `Fingerprint` struct (Label, SampleRate; more fields added later) and package doc. Create `signal.go` porting `RMSEnvelope`, `AutoSegment`, and `PeakNormalize` (the latter wrapping `wave.Chain(&wave.WaveSource{W: w}, wave.NormalizeTransform(0))`). Use `float64` samples throughout (wave is float64).

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/`
Expected: PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 3: Spectral fingerprint metrics

**Files:**
- Create: `src/go/internal/audio/fingerprint/spectral.go`
- Test: `src/go/internal/audio/fingerprint/spectral_test.go`
- Modify: `src/go/internal/audio/fingerprint/fingerprint.go` (add fields)

**Interfaces:**
- Produces (fields on `Fingerprint`): `F0Hz float64; Partials [16]float64; SpectralCentroid, SpectralRolloff, EvenOddRatio, Inharmonicity, NoiseRatio float64`.
- Produces: `func computeSpectral(w wave.Wave, fp *Fingerprint)` — uses `wave.MagnitudeSpectrum(seg, 16384, wave.WindowHann)` on the sustain window `[SustainSkipSec, SustainLenSec]` via `w.Slice`. F0 is filled by Task 4; here, accept `fp.F0Hz` as input for partial tracking and compute centroid/rolloff/even-odd/inharmonicity/noise.
- Produces: `func SpectralCentroid(mag []float64, binHz float64) float64` (exported — shared scalar helper the game analyzer can adopt).

- [ ] **Step 1: Write the failing test**

```go
func TestSpectralCentroid_BrightVsDull(t *testing.T) {
	sr := 44100
	mLow, hz := wave.MagnitudeSpectrum(wave.Sine(200, 1, 0.3, sr), 16384, wave.WindowHann)
	mHi, _ := wave.MagnitudeSpectrum(wave.Sine(4000, 1, 0.3, sr), 16384, wave.WindowHann)
	cLow := SpectralCentroid(mLow, hz)
	cHi := SpectralCentroid(mHi, hz)
	if !(cHi > cLow*5) {
		t.Errorf("centroid low=%.0f hi=%.0f — expected hi >> low", cLow, cHi)
	}
}

func TestNoiseRatio_SineLowNoiseHigh(t *testing.T) {
	var sfp, nfp Fingerprint
	sfp.F0Hz = 220
	computeSpectral(wave.Sine(220, 0.9, 1.5, 44100), &sfp)
	nfp.F0Hz = 220
	computeSpectral(wave.Noise(7, 0.9, 1.5, 44100), &nfp)
	if sfp.NoiseRatio > 0.3 {
		t.Errorf("sine NoiseRatio=%.3f want <0.3", sfp.NoiseRatio)
	}
	if nfp.NoiseRatio < 0.6 {
		t.Errorf("noise NoiseRatio=%.3f want >0.6", nfp.NoiseRatio)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `… ./internal/audio/fingerprint/ -run 'Spectral|Noise'`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `spectral.go`**

Port the spectral math from `cmd/synth-analyze/main.go` (`computeSpectralFingerprint`: partials near `k*F0`, centroid = Σ(hz·mag)/Σmag, 85% rolloff, even/odd over partials, inharmonicity, noise ratio = 1 − harmonicEnergy/totalEnergy) but: (a) source magnitudes from `wave.MagnitudeSpectrum`, (b) factor out the exported `SpectralCentroid(mag, binHz)`, (c) read `fp.F0Hz` rather than detecting F0 here.

- [ ] **Step 4: Run to verify pass**

Run: `… ./internal/audio/fingerprint/ -run 'Spectral|Noise'`
Expected: PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 4: Robust F0 (no octave errors)

**Files:**
- Create: `src/go/internal/audio/fingerprint/pitch.go`
- Test: `src/go/internal/audio/fingerprint/pitch_test.go`

**Interfaces:**
- Produces: `func DetectF0(w wave.Wave) float64` — autocorrelation (time-domain) cross-checked against the strongest spectral peak; resolves octave ambiguity by choosing the candidate whose integer-multiple series has the most spectral energy.

- [ ] **Step 1: Write the failing test**

```go
func TestDetectF0_PureTone(t *testing.T) {
	got := DetectF0(wave.Sine(220, 0.9, 0.5, 44100))
	if math.Abs(got-220) > 220*0.02 {
		t.Errorf("F0=%.1f want ~220", got)
	}
}

func TestDetectF0_MissingFundamental(t *testing.T) {
	// 440+660+880 (harmonics 2,3,4 of 220) — naive lowest-peak picks 440.
	sr := 44100
	mk := func(f float64) []float64 { return wave.Sine(f, 0.3, 0.5, sr).Samples }
	a, b, c := mk(440), mk(660), mk(880)
	sum := make([]float64, len(a))
	for i := range sum {
		sum[i] = a[i] + b[i] + c[i]
	}
	got := DetectF0(wave.Wave{Samples: sum, SampleRate: sr})
	if math.Abs(got-220) > 220*0.05 {
		t.Errorf("F0=%.1f want ~220 (missing fundamental)", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `… ./internal/audio/fingerprint/ -run TestDetectF0`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `DetectF0`**

Normalized autocorrelation over the sustain window for lags in `[sr/2000, sr/50]` (50–2000 Hz); pick the lag of the first strong local maximum (cumulative-mean-normalized, à la YIN's difference function) to avoid octave-up errors; then snap to the nearest strong spectral peak from `wave.MagnitudeSpectrum` to refine. Cross-check: among `{f, f/2, 2f}`, choose the candidate maximizing summed magnitude at its first 6 integer multiples (octave guard).

- [ ] **Step 4: Run to verify pass; wire into the builder**

Add `FromWave` to `fingerprint.go`:

```go
func FromWave(w wave.Wave, label string) Fingerprint {
	w = PeakNormalize(w)
	fp := Fingerprint{Label: label, SampleRate: w.SampleRate}
	fp.F0Hz = DetectF0(w)
	computeSpectral(w, &fp)
	// Tasks 5–8 append: temporal, mfcc, shape, envelope metrics.
	return fp
}
```

Run: `… ./internal/audio/fingerprint/`
Expected: PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 5: Temporal (STFT) metrics

**Files:**
- Create: `src/go/internal/audio/fingerprint/temporal.go`
- Test: `src/go/internal/audio/fingerprint/temporal_test.go`
- Modify: `fingerprint.go` (add `Temporal *TemporalFingerprint`; call in `FromWave`)

**Interfaces:**
- Produces: `type TemporalFingerprint struct { CentroidMean, CentroidSlope, CentroidRange, HarmonicFluxMean, SpectralVariance float64; PartialDecaySlopes [6]float64; VibratoDetected bool; VibratoRateHz, VibratoDepthCents float64; NumFrames int }`.
- Produces: `func computeTemporal(w wave.Wave, f0 float64) *TemporalFingerprint` (frames via repeated `wave.MagnitudeSpectrum` over `windowSize=2048, hop=512`).

- [ ] **Step 1: Write the failing test**

```go
func TestTemporal_VibratoDetected(t *testing.T) {
	// 5 Hz FM around 440 Hz, ±20 cents.
	sr := 44100
	dur := 1.5
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		depth := math.Pow(2, 20.0/1200) - 1 // ~20 cents
		f := 440 * (1 + depth*math.Sin(2*math.Pi*5*tsec))
		s[i] = 0.8 * math.Sin(2*math.Pi*f*tsec)
	}
	tf := computeTemporal(wave.Wave{Samples: s, SampleRate: sr}, 440)
	if !tf.VibratoDetected || math.Abs(tf.VibratoRateHz-5) > 1.0 {
		t.Errorf("vibrato=%v rate=%.2f want detected ~5Hz", tf.VibratoDetected, tf.VibratoRateHz)
	}
}

func TestTemporal_StaticToneLowFlux(t *testing.T) {
	tf := computeTemporal(wave.Sine(440, 0.8, 1.5, 44100), 440)
	if tf.HarmonicFluxMean > 0.05 {
		t.Errorf("static sine flux=%.4f want ~0", tf.HarmonicFluxMean)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run TestTemporal` → FAIL.

- [ ] **Step 3: Implement** — port `computeTemporalFingerprint` from `cmd/synth-analyze`, sourcing each frame from `wave.MagnitudeSpectrum(frameWave, 2048, wave.WindowHann)`; keep centroid trajectory/slope/range, flux, variance, partial decay slopes, and the DFT-based vibrato detector. Call it in `FromWave`.

- [ ] **Step 4: Run to verify pass** — `… -run TestTemporal` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 6: MFCC + delta

**Files:**
- Create: `src/go/internal/audio/fingerprint/mfcc.go`
- Test: `src/go/internal/audio/fingerprint/mfcc_test.go`
- Modify: `fingerprint.go` (add `MFCC [13]float64`; call in `FromWave`)

**Interfaces:**
- Produces: `func computeMFCC(w wave.Wave, n int) [13]float64` — mel filterbank (26 filters, 0–sr/2) over `wave.MagnitudeSpectrum`, log, DCT-II, keep first 13.

- [ ] **Step 1: Write the failing test**

```go
func TestMFCC_DiffersSineVsNoise(t *testing.T) {
	s := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	n := computeMFCC(wave.Noise(3, 0.9, 0.5, 44100), 13)
	d := 0.0
	for i := 1; i < 13; i++ { // skip c0 (energy)
		d += math.Abs(s[i] - n[i])
	}
	if d < 1.0 {
		t.Errorf("MFCC sine vs noise distance=%.3f too small", d)
	}
}

func TestMFCC_Stable(t *testing.T) {
	a := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	b := computeMFCC(wave.Sine(440, 0.9, 0.5, 44100), 13)
	if a != b {
		t.Error("MFCC not deterministic for identical input")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** standard MFCC (mel scale `2595*log10(1+f/700)`, triangular filterbank, log energy, DCT-II). Call in `FromWave`.
- [ ] **Step 4: Run to verify pass** — PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 7: Spectral-shape metrics (flatness, crest, skew, kurtosis, tristimulus)

**Files:**
- Create: `src/go/internal/audio/fingerprint/shape.go`
- Test: `src/go/internal/audio/fingerprint/shape_test.go`
- Modify: `fingerprint.go` (add `SpectralFlatness, SpectralCrest, SpectralSkewness, SpectralKurtosis float64; Tristimulus [3]float64`)

**Interfaces:**
- Produces: `func computeShape(w wave.Wave, fp *Fingerprint)` — operates on the sustain-window magnitude spectrum + `fp.Partials`.

- [ ] **Step 1: Write the failing test**

```go
func TestSpectralFlatness_SineLowNoiseHigh(t *testing.T) {
	var s, n Fingerprint
	computeShape(wave.Sine(440, 0.9, 1.0, 44100), &s)
	computeShape(wave.Noise(11, 0.9, 1.0, 44100), &n)
	if s.SpectralFlatness > 0.2 {
		t.Errorf("sine flatness=%.3f want low", s.SpectralFlatness)
	}
	if n.SpectralFlatness < 0.4 {
		t.Errorf("noise flatness=%.3f want high", n.SpectralFlatness)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** flatness = geomean/arithmean of magnitude; crest = max/mean; skewness/kurtosis = 3rd/4th standardized moments of the magnitude distribution over frequency; tristimulus T1=P1², T2=(P2²+P3²+P4²)/Σ, T3=rest/Σ over `fp.Partials²`. Call in `FromWave`.
- [ ] **Step 4: Run to verify pass** — PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 8: Envelope metrics (LAT, ADSR, HNR) + spectral-convergence helper

**Files:**
- Create: `src/go/internal/audio/fingerprint/envelope.go`
- Test: `src/go/internal/audio/fingerprint/envelope_test.go`
- Modify: `fingerprint.go` (add `AttackTimeSec, LogAttackTime, DecaySlope, SustainLevel, ReleaseTimeSec, HNR float64`)

**Interfaces:**
- Produces: `func computeEnvelope(w wave.Wave, fp *Fingerprint)` (RMS env → attack 10–90%, LAT = log10(attack time), decay slope dB/s, sustain level, release; HNR via cepstrum).
- Produces: `func LogSpectralDistance(a, b []float64) float64` (mean |20log10(a/b)| over bins) and `func SpectralConvergence(a, b []float64) float64` (‖a−b‖₂/‖a‖₂) — used by `Distance` in Task 9.

- [ ] **Step 1: Write the failing test**

```go
func TestLogAttackTime_LongerAttackLargerLAT(t *testing.T) {
	mk := func(attackSec float64) wave.Wave {
		sr := 44100
		n := int(1.0 * float64(sr))
		s := make([]float64, n)
		aN := int(attackSec * float64(sr))
		for i := range s {
			env := 1.0
			if i < aN {
				env = float64(i) / float64(aN)
			}
			s[i] = env * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	var fast, slow Fingerprint
	computeEnvelope(mk(0.005), &fast)
	computeEnvelope(mk(0.2), &slow)
	if !(slow.AttackTimeSec > fast.AttackTimeSec && slow.LogAttackTime > fast.LogAttackTime) {
		t.Errorf("attack fast=%.4f slow=%.4f; LAT fast=%.3f slow=%.3f", fast.AttackTimeSec, slow.AttackTimeSec, fast.LogAttackTime, slow.LogAttackTime)
	}
}

func TestSpectralConvergence_IdentityZero(t *testing.T) {
	a := []float64{1, 2, 3, 4}
	if d := SpectralConvergence(a, a); d > 1e-9 {
		t.Errorf("identity convergence=%.6f want 0", d)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** envelope segmentation from `RMSEnvelope` (attack 10→90% of peak, LAT=log10(attackSec) clamped, decay slope via least-squares dB fit, sustain = median of the middle third, release = 90→10% trailing); HNR via real cepstrum (peak quefrency energy vs residual). Add the two distance helpers. Call `computeEnvelope` in `FromWave`.
- [ ] **Step 4: Run to verify pass** — PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 9: Canonical `Distance()`

**Files:**
- Create: `src/go/internal/audio/fingerprint/distance.go`
- Test: `src/go/internal/audio/fingerprint/distance_test.go`

**Interfaces:**
- Produces: `type Score struct { Spectral, MFCC, Partials, Temporal, Envelope, Noise, Total float64 }`.
- Produces: `func Distance(ref, cand Fingerprint) Score` — each term normalized to ~[0,1] for a typical mismatch, weighted via a documented `var distanceWeights`, summed into `Total`. No double-counting (centroid/rolloff folded into one Spectral term alongside log-spectral metrics; partials separate; MFCC perceptual).

- [ ] **Step 1: Write the failing test**

```go
func TestDistance_IdentityZero(t *testing.T) {
	fp := FromWave(wave.Sine(220, 0.9, 1.5, 44100), "a")
	s := Distance(fp, fp)
	if s.Total > 1e-6 {
		t.Errorf("identity Total=%.6f want 0", s.Total)
	}
}

func TestDistance_CloserIsSmaller(t *testing.T) {
	ref := FromWave(wave.Sine(220, 0.9, 1.5, 44100), "ref")
	near := FromWave(wave.Sine(225, 0.9, 1.5, 44100), "near")
	far := FromWave(wave.Noise(5, 0.9, 1.5, 44100), "far")
	if !(Distance(ref, near).Total < Distance(ref, far).Total) {
		t.Errorf("near=%.3f far=%.3f — expected near < far", Distance(ref, near).Total, Distance(ref, far).Total)
	}
}

func TestDistance_Symmetric(t *testing.T) {
	a := FromWave(wave.Sine(220, 0.9, 1.0, 44100), "a")
	b := FromWave(wave.Sine(300, 0.9, 1.0, 44100), "b")
	if math.Abs(Distance(a, b).Total-Distance(b, a).Total) > 1e-9 {
		t.Error("Distance not symmetric")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** the weighted, normalized distance with a documented weight table; Spectral term combines normalized centroid/rolloff deltas + `LogSpectralDistance`/`SpectralConvergence` on the sustain magnitude (store the sustain spectrum on the Fingerprint or recompute deterministically); Partials = L1 over `Partials`; MFCC = L2 over coeffs 1..12; Temporal = normalized flux/centroid-range/vibrato deltas; Envelope = normalized attack/LAT/decay/sustain deltas; Noise = |ΔNoiseRatio|+|ΔFlatness|.
- [ ] **Step 4: Run to verify pass** — PASS, plus full package: `… ./internal/audio/fingerprint/`.
- [ ] **Step 5: Checkpoint**

---

### Task 10: Refactor `cmd/synth-analyze` onto the library

**Files:**
- Modify: `src/go/cmd/synth-analyze/main.go` (delete private DSP; call the library)
- Modify/Delete: `src/go/cmd/synth-analyze/main_test.go` (point at library or remove now-dead tests)

**Interfaces:**
- Consumes: `fingerprint.FromWave`, `fingerprint.Distance`, `fingerprint.AutoSegment`; `audio.DecodeWAVToPCM`, `synthmatch.RenderInstrument` (Task 11) for the `synth` subcommand.

- [ ] **Step 1:** Rewrite `main.go` `ref`/`synth`/`diff` to: load WAV via `audio.DecodeWAVToPCM` → `wave.Wave`, crop/auto-segment via the library, `fingerprint.FromWave`, print/JSON, and `diff` via `fingerprint.Distance`. Delete the private `fft/nextPow2/peakNormalize/rmsEnvelope/attackDecay/autoSegment/computeSpectralFingerprint/computeSTFT/computeTemporalFingerprint`.
- [ ] **Step 2:** Build smoke: `cd src/go && ../../.tools/go/bin/go build ./cmd/synth-analyze/` → exit 0.
- [ ] **Step 3:** Remove or rewrite `main_test.go` so the package compiles under `-tags test` (it currently can't). Prefer deleting DSP tests now covered in `internal/audio/fingerprint`; keep only CLI-arg tests if any.
- [ ] **Step 4:** Run `cd src/go && ../../.tools/go/bin/go vet ./cmd/synth-analyze/` → exit 0.
- [ ] **Step 5: Checkpoint**

---

### Task 11: `synthmatch` package — render + tunable spec

**Files:**
- Create: `src/go/internal/audio/synthmatch/render.go`
- Create: `src/go/internal/audio/synthmatch/tunable.go`
- Test: `src/go/internal/audio/synthmatch/tunable_test.go`

**Interfaces:**
- Produces: `func RenderInstrument(instID string, pitch float64, sr int, durSec float64) (wave.Wave, error)` (ports `renderSynth` from the old CLI: `audio.RecipeForInstrument`→`audio.NewRecipe`→`audio.MergeRecipeDefaults(GetInstrumentParams)`→`Render`; returns a `wave.Wave`).
- Produces: `func RenderWithParams(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error)`.
- Produces: `type Param struct { Key string; Min, Max, Step float64; Discrete []float64; Int bool }`, `type TunableSpec struct { Family string; Params []Param }`, `func SpecForInstrument(instID string) (TunableSpec, bool)`.
- Produces: `func (s TunableSpec) ValidateKeys(instID string) error` (every key must exist in the merged recipe defaults).

- [ ] **Step 1: Write the failing test**

```go
func TestSpecKeysExistInRecipe(t *testing.T) {
	audio.Reset(); audio.ResetInstruments()
	for _, inst := range []string{"sax", "organ", "clavinet", "violin", "trumpet"} {
		spec, ok := SpecForInstrument(inst)
		if !ok {
			t.Fatalf("no TunableSpec for %q", inst)
		}
		if err := spec.ValidateKeys(inst); err != nil {
			t.Errorf("%s: %v", inst, err)
		}
	}
}

func TestRenderWithParams_ChangesOutput(t *testing.T) {
	audio.Reset(); audio.ResetInstruments()
	base, err := RenderInstrument("sax", 0, 44100, 0.5)
	if err != nil { t.Fatal(err) }
	mod, err := RenderWithParams("sax", 0, 44100, 0.5, map[string]float64{"filter_cutoff": 400})
	if err != nil { t.Fatal(err) }
	if base.PeakSample() == mod.PeakSample() && base.Samples[1000] == mod.Samples[1000] {
		t.Error("filter_cutoff override did not change output")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** `render.go` (render→`wave.Wave`) and `tunable.go` with three family specs (subtractive-reed, additive-organ, ks-pluck) mapping the curated tunable keys + bounds, and `SpecForInstrument` keyed by instrument→family. `ValidateKeys` checks each key against `audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))`.
- [ ] **Step 4: Run to verify pass** — PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 12: Hooke–Jeeves optimizer

**Files:**
- Create: `src/go/internal/audio/synthmatch/optimize.go`
- Test: `src/go/internal/audio/synthmatch/optimize_test.go`

**Interfaces:**
- Produces: `type Result struct { Params map[string]float64; StartLoss, BestLoss float64; Evals int; RefFP, BestFP fingerprint.Fingerprint }`.
- Produces: `func Match(instID string, ref wave.Wave, budget, restarts int) (Result, error)` — renders at `fingerprint.DetectF0(ref)` (semitones from A3=220 derived from refF0), pattern-search over `SpecForInstrument`, seeded restarts (`rand.New(rand.NewSource(1))`), loss = `fingerprint.Distance(refFP, candFP).Total`.

- [ ] **Step 1: Write the failing test**

```go
func TestMatch_RecoversPerturbedParams(t *testing.T) {
	audio.Reset(); audio.ResetInstruments()
	sr := 44100
	// Ground truth = sax with a known cutoff; treat its render as the "reference".
	ref, err := RenderWithParams("sax", 0, sr, 1.2, map[string]float64{"filter_cutoff": 1500})
	if err != nil { t.Fatal(err) }
	res, err := Match("sax", ref, 250, 2)
	if err != nil { t.Fatal(err) }
	if res.BestLoss >= res.StartLoss {
		t.Errorf("optimizer did not improve: start=%.4f best=%.4f", res.StartLoss, res.BestLoss)
	}
	// Tuned cutoff should move toward 1500 from the seed default.
	if got := res.Params["filter_cutoff"]; math.Abs(got-1500) > 600 {
		t.Errorf("filter_cutoff=%.0f did not approach 1500", got)
	}
}

func TestMatch_Deterministic(t *testing.T) {
	audio.Reset(); audio.ResetInstruments()
	ref, _ := RenderInstrument("violin", 0, 44100, 1.0)
	a, _ := Match("violin", ref, 120, 1)
	b, _ := Match("violin", ref, 120, 1)
	if a.BestLoss != b.BestLoss {
		t.Errorf("non-deterministic: %.6f vs %.6f", a.BestLoss, b.BestLoss)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** Hooke–Jeeves: start at spec midpoints (or current seed values), per-param exploratory moves of ±Step (respecting Min/Max/Discrete/Int), pattern moves on success, halve steps on stall, stop at `budget` evals; `restarts` seeded random restarts keep the best. Pitch: `semis = 12*log2(refF0/220)`.
- [ ] **Step 4: Run to verify pass** (may take a few seconds) — `… ./internal/audio/synthmatch/ -run TestMatch`.
- [ ] **Step 5: Checkpoint**

---

### Task 13: Seed auto-patcher

**Files:**
- Create: `src/go/internal/audio/synthmatch/seedpatch.go`
- Test: `src/go/internal/audio/synthmatch/seedpatch_test.go`
- Create (fixture): `src/go/internal/audio/synthmatch/testdata/seed_fixture.txt`

**Interfaces:**
- Produces: `func PatchSeedBlock(src []byte, varName string, tuned map[string]float64) ([]byte, error)` — pure function: update existing keys, insert new keys before the closing `}`, preserve comments/order/other keys, gofmt the result.
- Produces: `func PatchSeedFile(path, varName string, tuned map[string]float64) error` — read/patch/write wrapper for `instrument_seeds.go`.

- [ ] **Step 1: Write the failing test**

```go
func TestPatchSeedBlock_UpdatesAndInserts(t *testing.T) {
	src := []byte("package audio\n\nvar saxSeed = RecipeParams{\n\t\"filter_cutoff\": 1000,\n\t\"gain\": 0.9, // keep comment\n}\n")
	out, err := PatchSeedBlock(src, "saxSeed", map[string]float64{"filter_cutoff": 1450, "amp_attack": 0.02})
	if err != nil { t.Fatal(err) }
	s := string(out)
	if !strings.Contains(s, `"filter_cutoff": 1450`) {
		t.Errorf("cutoff not updated:\n%s", s)
	}
	if !strings.Contains(s, `"amp_attack": 0.02`) {
		t.Errorf("new key not inserted:\n%s", s)
	}
	if !strings.Contains(s, "// keep comment") {
		t.Errorf("comment not preserved:\n%s", s)
	}
	// Must still be valid, gofmt-stable Go.
	if _, err := format.Source(out); err != nil {
		t.Errorf("output not valid Go: %v", err)
	}
}

func TestPatchSeedBlock_UnknownVarErrors(t *testing.T) {
	_, err := PatchSeedBlock([]byte("package audio\n"), "nopeSeed", map[string]float64{"x": 1})
	if err == nil {
		t.Error("expected error for missing var block")
	}
}
```

(Imports: `go/format`, `strings`.)

- [ ] **Step 2: Run to verify it fails** — FAIL.
- [ ] **Step 3: Implement** `PatchSeedBlock`: locate `var <varName> = RecipeParams{` … matching `}`; within the block, for each tuned key, regex-replace `"key": <value>,` preserving trailing comment; for keys not found, insert `\t"key": <formatted>,\n` before the closing brace; run `format.Source`. Format floats without trailing-zero noise (e.g. `strconv.FormatFloat(v, 'g', -1, 64)`). `PatchSeedFile` reads `internal/audio/instrument_seeds.go`, calls `PatchSeedBlock`, writes back.
- [ ] **Step 4: Run to verify pass** — PASS.
- [ ] **Step 5: Checkpoint**

---

### Task 14: `cmd/synth-match` CLI + make targets

**Files:**
- Create: `src/go/cmd/synth-match/main.go` (`//go:build !test && !js`)
- Modify: `Makefile`

**Interfaces:**
- Consumes: `synthmatch.Match`, `synthmatch.PatchSeedFile`, `fingerprint.Distance`, `audio.DecodeWAVToPCM`.

- [ ] **Step 1:** Implement `main.go`: flags `-inst`, `-ref`, `-budget` (400), `-restarts` (3), `-dry-run`, `-report`. Load ref WAV → `wave.Wave`; `synthmatch.Match`; print ref vs baseline vs tuned `Distance` + a per-param before→after table; unless `-dry-run`, derive the seed var name (`<inst>Seed` via a small instID→seedVar map or convention) and `PatchSeedFile`.
- [ ] **Step 2:** Build smoke: `cd src/go && ../../.tools/go/bin/go build ./cmd/synth-match/` → exit 0.
- [ ] **Step 3:** Add to `Makefile`:

```make
synth-match: ## Tune a synth toward a reference WAV (INST=, REF=, [BUDGET=], [DRY_RUN=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-match -inst $(INST) -ref $(REF) \
		$(if $(BUDGET),-budget $(BUDGET),) $(if $(DRY_RUN),-dry-run,)

synth-fingerprint: ## Print the analysis fingerprint of a WAV (FILE=)
	cd src/go && ../../.tools/go/bin/go run ./cmd/synth-analyze ref -file $(FILE)

fetch-synth-refs: ## Download + convert the reference instrument WAVs
	bash scripts/fetch_synth_refs.sh
```

- [ ] **Step 4:** Verify: `make synth-match INST=sax REF=<any wav> DRY_RUN=1` runs end-to-end and prints a distance improvement without writing.
- [ ] **Step 5: Checkpoint**

---

### Task 15: Real reference WAVs — fetch script, manifest, smoke test

**Files:**
- Create: `scripts/fetch_synth_refs.sh`
- Create: `scripts/aiff_to_wav.py`
- Create: `src/go/internal/audio/testdata/refs/MANIFEST.md`
- Create: `src/go/internal/audio/synthmatch/refs_smoke_test.go`

**Interfaces:** none (tooling + data).

- [ ] **Step 1: Write the failing/skip test**

```go
func TestRefs_DecodeAndFingerprint(t *testing.T) {
	dir := "../../audio/testdata/refs" // adjust to package-relative path
	entries, err := os.ReadDir(dir)
	if err != nil || len(wavs(entries)) == 0 {
		t.Skip("no reference WAVs present; run scripts/fetch_synth_refs.sh")
	}
	for _, name := range wavs(entries) {
		pcm, sr, err := audio.DecodeWAVToPCM(filepath.Join(dir, name))
		if err != nil { t.Fatalf("%s: %v", name, err) }
		fp := fingerprint.FromWave(wave.Wave{Samples: toF64(pcm), SampleRate: sr}, name)
		if fp.F0Hz <= 0 || fp.F0Hz > 8000 {
			t.Errorf("%s: implausible F0=%.1f", name, fp.F0Hz)
		}
	}
}
```

(Add `wavs`/`toF64` helpers in the test.)

- [ ] **Step 2: Run to verify it skips cleanly** — `… ./internal/audio/synthmatch/ -run TestRefs` → PASS (skipped, dir empty).
- [ ] **Step 3: Write `aiff_to_wav.py`** (python stdlib `aifc`+`wave`, downmix to mono, 16-bit) and `fetch_synth_refs.sh` (curl each Iowa MIS AIFF URL → convert → write to `testdata/refs/<inst>_<note>.wav` → append a `MANIFEST.md` row with source URL + license + sha256). Seed `MANIFEST.md` with the license header (Iowa MIS: "free … without restrictions") and the per-instrument source list.
- [ ] **Step 4: Run the fetch, then the smoke test for real**

Run: `bash scripts/fetch_synth_refs.sh` then `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/synthmatch/ -run TestRefs`
Expected: real WAVs present, decode + plausible F0. (If a host URL 404s, fix the URL in the script; verify at least violin/sax/trumpet/cello/flute/piano land.)

- [ ] **Step 5: Checkpoint** — then run an end-to-end real match: `make synth-match INST=sax REF=src/go/internal/audio/testdata/refs/<sax file> DRY_RUN=1` and confirm the distance drops; review the proposed seed diff.

---

## Self-Review

**Spec coverage:** §2 reuse → Tasks 1,2,10,11 (wave FFT shared, DecodeWAV, render reuse). §4 fingerprint hardening+metrics → Tasks 2–8. §5 Distance → Task 9. §6 matcher (TunableSpec, optimizer, seed patch) → Tasks 11,12,13. §7 CLI+make → Tasks 10,14. §8 refs → Task 15. §9 testing → every task is TDD; shared `wave` fixtures; skip-if-absent in Task 15. All spec sections map to tasks.

**Placeholder scan:** no "TBD/TODO/handle edge cases" — each step has concrete code or an exact port reference + commands.

**Type consistency:** `Fingerprint` fields are introduced once and reused; `fingerprint.FromWave`/`Distance`/`Score` names consistent across Tasks 4,9,10,12,15; `synthmatch.RenderInstrument`/`RenderWithParams`/`Match`/`SpecForInstrument`/`PatchSeedBlock`/`PatchSeedFile` consistent across Tasks 11–15; `wave.MagnitudeSpectrum` signature consistent across Tasks 1,3,5,6,7. `WindowHann` flagged in Task 1 to confirm against the real const name in `transforms.go`.

**Open verification during execution:** confirm `WindowType` const names and whether `transforms.go` already has an in-place window helper (Task 1); confirm exact Iowa MIS URLs (Task 15).
