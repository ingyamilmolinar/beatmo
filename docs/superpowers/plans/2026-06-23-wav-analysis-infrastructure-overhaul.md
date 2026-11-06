# WAV Analysis Infrastructure Overhaul (Phase A) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the WAV-analysis fingerprint infrastructure comprehensive and bug-free — fix the F0 octave bug, decode 24-bit references, and add five time-varying DSP metric families — all validated against the real violin D5 reference.

**Architecture:** Pure-Go DSP in `internal/audio/fingerprint` on top of `internal/wave`'s FFT. Each new metric family is a self-contained file with one struct + one `compute*` entry called from `FromWave`. Bug fixes land first (they unblock the golden test and correct every harmonic-relative metric). `cmd/synth-analyze` is the human/JSON reporting surface.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `-tags test -modfile=go.test.mod` fast path, numpy (host) only for cross-checking ground truth.

## Global Constraints

- **Never `git add`/`git commit`/`git stash`.** Leave all work uncommitted for the user. Each task ends at a green checkpoint, not a commit.
- Build/test with the bundled Go: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/...`
- gofmt-clean via `../../.tools/go/bin/gofmt -w` (system gofmt may be absent).
- New `Fingerprint` fields are additive and JSON-backward-compatible (pointer sub-structs use `,omitempty`).
- `Distance(fp, fp).Total == 0` (identity) and `Distance(a,b) == Distance(b,a)` (symmetry) must remain true after new terms are added.
- Reference file: `/home/ymolinar/Repos/beatmo/356181__mtg__violin-d5.wav` (24-bit / 48 kHz / mono / 2.675 s). Ground truth: F0≈591.3 Hz, vibrato≈4 Hz/±5 c, bridge hill≈2.3–2.4 kHz, corr(centroid,RMS)≈+0.73, spectral flatness≈0.094, attack 10–90%≈0.5 s.
- Match existing patterns in each file you modify — read the file first; do not restructure unrelated code.

---

## File Structure

**Bug fixes (existing files):**
- `src/go/internal/audio/sample_pcm_teststub.go` — add 24-bit + 32-bit-float decode.
- `src/go/internal/audio/fingerprint/pitch.go` — HPS octave guard.
- `src/go/internal/audio/fingerprint/spectral.go` — proportional partial window + silence guard.
- `src/go/internal/audio/fingerprint/distance.go` — zero/empty-fingerprint guard + new terms.
- `src/go/internal/audio/synthmatch/seedpatch.go`, `.../optimize.go`, `cmd/synth-match/main.go` — edge fixes.

**New metric files (`src/go/internal/audio/fingerprint/`):**
- `vibrato.go` + `vibrato_test.go` — `VibratoFingerprint`, `computeVibrato`.
- `harmonics_temporal.go` + `_test.go` — `HarmonicTrajectory`, `computeHarmonicTrajectory`.
- `formants.go` + `_test.go` — `SpectralEnvelope`, `computeSpectralEnvelope`.
- `coupling.go` + `_test.go` — `DynamicCoupling`, `computeCoupling`.
- `noise_residual.go` + `_test.go` — `NoiseProfile`, `computeNoiseProfile`.

**Integration:**
- `fingerprint.go` — extend `Fingerprint`, wire `compute*` calls.
- `cmd/synth-analyze/main.go` — print new metrics; preserve native SR in `ref`.
- `fingerprint/violin_golden_test.go` (new) — real-reference characterization.

**Test helper:**
- `fingerprint/synthgen_test.go` (new) — synthetic signal generators (vibrato tone, formant-shaped tone, amplitude-coupled tone, noisy tone) shared by metric tests.

---

## Task 1: 24-bit / 32-bit-float PCM decode (unblocks real-reference tests)

**Files:**
- Modify: `src/go/internal/audio/sample_pcm_teststub.go`
- Test: `src/go/internal/audio/sample_pcm_teststub_test.go` (create if absent)

**Interfaces:**
- Produces: `DecodeWAVToPCM(path string) ([]float32, int, error)` now also succeeds for 24-bit signed and 32-bit IEEE-float WAV (`fmt` audioFormat 1 with 24 bits, and audioFormat 3 with 32 bits). Mono/stereo handling unchanged (stereo averaged to mono).

- [ ] **Step 1: Write the failing test.** Read the file first to learn the existing decode/struct style, then add:

```go
func TestDecodeWAVToPCM_24bit(t *testing.T) {
	// 356181__mtg__violin-d5.wav is 24-bit/48k/mono.
	pcm, sr, err := DecodeWAVToPCM("../../../356181__mtg__violin-d5.wav")
	if err != nil {
		t.Fatalf("decode 24-bit: %v", err)
	}
	if sr != 48000 {
		t.Fatalf("sample rate = %d, want 48000", sr)
	}
	if len(pcm) < 100000 {
		t.Fatalf("len(pcm) = %d, want > 100000 (~2.6s @ 48k)", len(pcm))
	}
	var peak float32
	for _, s := range pcm {
		if s > peak {
			peak = s
		} else if -s > peak {
			peak = -s
		}
	}
	if peak < 0.05 || peak > 1.0 {
		t.Fatalf("peak = %.4f, want in (0.05, 1.0]", peak)
	}
}
```

Adjust the relative path so it resolves from the package dir (verify with `pwd`; repo root is three levels up from `internal/audio`).

- [ ] **Step 2: Run to verify it fails.** Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestDecodeWAVToPCM_24bit -v` → Expected: FAIL (current decoder rejects non-16-bit).

- [ ] **Step 3: Implement.** In the WAV parser, after reading `bitsPerSample` and `audioFormat` from the `fmt ` chunk, branch the sample conversion:
  - 16-bit signed (existing): `int16 → /32768`.
  - 24-bit signed: read 3 little-endian bytes per sample, sign-extend the top bit, divide by `1<<23`.
  - 32-bit IEEE float (`audioFormat == 3`): `math.Float32frombits(binary.LittleEndian.Uint32(...))`, already in [-1,1].
  Keep the channel-averaging logic; generalize the per-sample stride to `bitsPerSample/8 * numChannels`. Reject anything still unsupported with a clear error.

- [ ] **Step 4: Run to verify it passes.** Same command → Expected: PASS. Also run the full package: `... ./internal/audio/ -run TestDecodeWAVToPCM` → all PASS (16-bit path unbroken).

- [ ] **Step 5: gofmt + checkpoint.** Run `../../.tools/go/bin/gofmt -w internal/audio/sample_pcm_teststub.go internal/audio/sample_pcm_teststub_test.go`. Leave uncommitted.

---

## Task 2: F0 octave robustness (HPS guard)

**Files:**
- Modify: `src/go/internal/audio/fingerprint/pitch.go` (the octave-guard block inside `DetectF0`)
- Test: `src/go/internal/audio/fingerprint/pitch_test.go`

**Interfaces:**
- Consumes: existing `DetectF0(w wave.Wave) float64`, helpers `harmonicCoverage`, `snapToSpectralPeak`, `wave.MagnitudeSpectrum`.
- Produces: `DetectF0` correctly returns ≈591 Hz on the D5 reference (no behavioral change to currently-correct cases).

- [ ] **Step 1: Write the failing test.**

```go
func TestDetectF0_NoSubharmonicOctaveError(t *testing.T) {
	pcm, sr, err := loadRefWaveForTest(t) // helper: decode the D5 ref into wave.Wave (see below)
	if err != nil {
		t.Skipf("reference unavailable: %v", err)
	}
	w := wave.Wave{Samples: pcm, SampleRate: sr}
	seg := AutoSegment(w, 1.5)
	f0 := DetectF0(seg)
	if f0 < 575 || f0 > 605 {
		t.Fatalf("DetectF0 = %.1f Hz, want ~591 (D5); sub-harmonic bug if ~197", f0)
	}
}
```

Add a small `loadRefWaveForTest` helper in the test file that calls `audio.DecodeWAVToPCM` is not importable from `fingerprint` (cycle); instead read the WAV with the existing pure-Go path or a minimal local 24-bit reader. Simplest: add an unexported test reader that mirrors Task 1's decode (24-bit LE → float64), since `fingerprint` must not import `audio`. Convert `[]float32`→`[]float64`.

- [ ] **Step 2: Run to verify it fails.** Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run TestDetectF0_NoSubharmonicOctaveError -v` → Expected: FAIL with f0 ≈ 197.

- [ ] **Step 3: Implement HPS guard.** Replace the candidate set `{f/2, f, 2f}` with `{f/3, f/2, f, 2f, 3f}` (skip candidates outside a sane vocal/instrument range, e.g. 40–4000 Hz). Score each candidate with a Harmonic Product Spectrum measure: downsample-multiply the magnitude spectrum by integer factors (1..5) and read the product at the candidate bin, OR reuse `harmonicCoverage` summing magnitude at the first ~8 harmonics. Pick the max-score candidate; on near-ties (<10% score diff) prefer the one closest to the YIN estimate. Keep the existing snap-to-peak afterward.

- [ ] **Step 4: Run to verify it passes.** Same `-run` command → PASS. Then run the whole pitch + spectral suite: `... ./internal/audio/fingerprint/ -run 'Pitch|F0|Spectral'` → all PASS (no regression on existing pitch tests).

- [ ] **Step 5: gofmt + checkpoint.** `gofmt -w` both files. Uncommitted.

---

## Task 3: Robust partial picking + silence guard

**Files:**
- Modify: `src/go/internal/audio/fingerprint/spectral.go` (partial loop in `computeSpectral`)
- Modify: `src/go/internal/audio/fingerprint/fingerprint.go` (`FromWave` early-out for silence)
- Test: `src/go/internal/audio/fingerprint/spectral_test.go`

**Interfaces:**
- Produces: `Partials[k]` for high k no longer collapses to ~0 under vibrato; `FromWave` on a zero/near-zero wave returns a zero-valued `Fingerprint` without NaN.

- [ ] **Step 1: Write failing tests.**

```go
func TestPartials_HighHarmonicsSurviveVibrato(t *testing.T) {
	sr := 48000
	w := synthVibratoTone(sr, 2.0, 591.0, []float64{1, .75, .22, .58, .37, .1, .08, .06}, 5.0, 20.0) // see synthgen_test.go
	fp := FromWave(w, "vib")
	if fp.Partials[3] < 0.2 { // H4 is strong in the model
		t.Fatalf("Partials[3]=%.3f, want >0.2 (window too tight under vibrato)", fp.Partials[3])
	}
	if fp.Partials[7] < 0.02 {
		t.Fatalf("Partials[7]=%.3f, want >0.02", fp.Partials[7])
	}
}

func TestFromWave_SilenceNoNaN(t *testing.T) {
	w := wave.Wave{Samples: make([]float64, 48000), SampleRate: 48000}
	fp := FromWave(w, "silent")
	if math.IsNaN(fp.SpectralCentroid) || math.IsNaN(fp.F0Hz) {
		t.Fatalf("silence produced NaN: centroid=%v f0=%v", fp.SpectralCentroid, fp.F0Hz)
	}
}
```

- [ ] **Step 2: Run to verify they fail.** Run the two `-run` tests → Expected: FAIL (Task 3 depends on `synthVibratoTone` from Task 4's helper; if executing strictly in order, create `synthgen_test.go` minimal generator now — see Task 4 Step 1 — or reorder so Task 4's helper lands first).

- [ ] **Step 3: Implement.** In `computeSpectral`, change the per-harmonic search from a fixed `±1.5 Hz` window to `±max(1.5, k*tolHz)` where `tolHz ≈ 0.004*F0` (≈±7 cents) so the window scales with harmonic index; take the local max magnitude in that window. In `FromWave`, after peak detection, if peak < 1e-6 return the zero `Fingerprint{Label:label, SampleRate:sr}` early. Guard divisions by total energy (`if total<=0 return`).

- [ ] **Step 4: Run to verify pass.** Both tests PASS; full `./internal/audio/fingerprint/` suite PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 4: Synthetic signal generators (shared test helper) + `vibrato.go`

**Files:**
- Create: `src/go/internal/audio/fingerprint/synthgen_test.go`
- Create: `src/go/internal/audio/fingerprint/vibrato.go`
- Create: `src/go/internal/audio/fingerprint/vibrato_test.go`

**Interfaces:**
- Produces (helpers, test-only): `synthTone(sr, dur, f0 float64, partials []float64) wave.Wave`; `synthVibratoTone(sr, dur, f0 float64, partials []float64, vibRateHz, vibCents float64) wave.Wave`; `synthAMTone(...)`, `synthFormantTone(...)`, `synthNoisyTone(...)` (add the latter three as their consuming tasks need them).
- Produces: `type VibratoFingerprint struct { RateHz, ExtentCents, Jitter, AMDepth, OnsetSec float64; Detected bool }` and `func computeVibrato(w wave.Wave, f0 float64, cfg AnalysisConfig) VibratoFingerprint`.

- [ ] **Step 1: Write the generators + failing vibrato test.**

```go
// synthgen_test.go
func synthVibratoTone(sr int, dur, f0 float64, partials []float64, vibRate, vibCents float64) wave.Wave {
	n := int(float64(sr) * dur)
	s := make([]float64, n)
	phase := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sr)
		cents := vibCents * math.Sin(2*math.Pi*vibRate*t)
		f := f0 * math.Pow(2, cents/1200)
		phase += 2 * math.Pi * f / float64(sr)
		var v float64
		for k, a := range partials {
			v += a * math.Sin(float64(k+1)*phase)
		}
		s[i] = 0.5 * v
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}
```

```go
// vibrato_test.go
func TestComputeVibrato_RecoversInjectedRate(t *testing.T) {
	sr := 48000
	w := synthVibratoTone(sr, 2.0, 440, []float64{1, .5, .25}, 5.5, 20)
	seg := sustainWindow(w)
	vf := computeVibrato(seg, 440, DefaultAnalysisConfig())
	if !vf.Detected {
		t.Fatal("vibrato not detected")
	}
	if math.Abs(vf.RateHz-5.5) > 0.7 {
		t.Fatalf("RateHz=%.2f, want ~5.5", vf.RateHz)
	}
	if math.Abs(vf.ExtentCents-20) > 6 {
		t.Fatalf("ExtentCents=%.1f, want ~20", vf.ExtentCents)
	}
}
```

- [ ] **Step 2: Run to verify it fails.** `-run TestComputeVibrato_RecoversInjectedRate` → FAIL (undefined `computeVibrato`).

- [ ] **Step 3: Implement `computeVibrato`.** Build an F0 trajectory across STFT frames via parabolic interpolation of the fundamental peak (reuse the approach in `temporal.go`'s vibrato block — move it here). Convert to cents about the median; detrend; estimate rate by autocorrelation or FFT of the cents track over a 3–9 Hz band; `ExtentCents` = (p95−p5)/2; `Jitter` = std of cycle-to-cycle period; `AMDepth` = relative std of the RMS envelope at the detected rate; `OnsetSec` = first time the local cents oscillation amplitude exceeds half its sustained value; `Detected` = band power > 3× noise floor AND ExtentCents > 2.

- [ ] **Step 4: Run to verify pass.** Test PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 5: `harmonics_temporal.go` — per-harmonic trajectories

**Files:**
- Create: `src/go/internal/audio/fingerprint/harmonics_temporal.go`
- Create: `src/go/internal/audio/fingerprint/harmonics_temporal_test.go`

**Interfaces:**
- Produces: `type HarmonicTrajectory struct { N int; AttackRate, PeakTimeSec, SustainLevel, DecayRate [16]float64; EvenOddOverTime []float64 }` and `func computeHarmonicTrajectory(w wave.Wave, f0 float64, cfg AnalysisConfig) HarmonicTrajectory`.

- [ ] **Step 1: Write the failing test.** Generate a tone whose H2 swells then fades (amplitude ramp on the 2nd partial) and assert `PeakTimeSec[1]` lands mid-note and `DecayRate[1] < 0` after the peak:

```go
func TestHarmonicTrajectory_DetectsMidNotePeak(t *testing.T) {
	sr := 48000
	w := synthSwellH2(sr, 2.0, 440) // helper in synthgen_test.go: H1 steady, H2 amplitude = triangular(0->1->0)
	traj := computeHarmonicTrajectory(sustainWindow(w), 440, DefaultAnalysisConfig())
	if traj.PeakTimeSec[1] < 0.2 || traj.PeakTimeSec[1] > 1.1 {
		t.Fatalf("H2 peak time=%.2fs, want mid-window", traj.PeakTimeSec[1])
	}
}
```

Add `synthSwellH2` to `synthgen_test.go`.

- [ ] **Step 2: Run to verify it fails.** → FAIL (undefined).

- [ ] **Step 3: Implement.** STFT the window (2048/512 like `temporal.go`); for each harmonic k=1..N (N=min(16, sr/2/f0)) read the magnitude at the nearest bin per frame → amplitude track. Per track compute: `AttackRate` (OLS dB/s pre-peak), `PeakTimeSec` (argmax·hop/sr), `SustainLevel` (median of middle third), `DecayRate` (OLS dB/s post-peak). `EvenOddOverTime[f]` = Σeven/Σodd magnitudes per frame.

- [ ] **Step 4: Run to verify pass.** PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 6: `formants.go` — spectral envelope + bridge hill

**Files:**
- Create: `src/go/internal/audio/fingerprint/formants.go`
- Create: `src/go/internal/audio/fingerprint/formants_test.go`

**Interfaces:**
- Produces: `type Formant struct { Hz, BandwidthHz, GainDB float64 }`; `type SpectralEnvelope struct { Formants []Formant; BridgeHillHz, BridgeHillGainDB float64 }`; `func computeSpectralEnvelope(w wave.Wave, cfg AnalysisConfig) SpectralEnvelope`.

- [ ] **Step 1: Write the failing test.** Generate a tone passed through a single strong resonance at 2400 Hz and assert a formant near there and a bridge hill in 1.5–4 kHz:

```go
func TestSpectralEnvelope_FindsResonance(t *testing.T) {
	sr := 48000
	w := synthFormantTone(sr, 2.0, 300, 2400, 8.0) // helper: rich tone -> biquad BP at 2400 Hz, Q=8
	env := computeSpectralEnvelope(sustainWindow(w), DefaultAnalysisConfig())
	if env.BridgeHillHz < 1800 || env.BridgeHillHz > 3000 {
		t.Fatalf("BridgeHillHz=%.0f, want ~2400", env.BridgeHillHz)
	}
}
```

Add `synthFormantTone` (a rich saw-ish tone through an RBJ band-pass) to `synthgen_test.go`.

- [ ] **Step 2: Run to verify it fails.** → FAIL.

- [ ] **Step 3: Implement.** Compute the log-magnitude spectrum (reuse `mfccFFTSize` or `cfg.FFTSize`); smooth it via real-cepstrum liftering (keep low quefrencies, inverse) to get a spectral envelope; peak-pick local maxima as `Formant{Hz, BandwidthHz (−3 dB width), GainDB (rel. envelope max)}`; set `BridgeHillHz/GainDB` to the strongest broad peak within 1.5–4 kHz. (Optional LPC cross-check may be added but is not required to pass.)

- [ ] **Step 4: Run to verify pass.** PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 7: `coupling.go` — brightness↔loudness coupling

**Files:**
- Create: `src/go/internal/audio/fingerprint/coupling.go`
- Create: `src/go/internal/audio/fingerprint/coupling_test.go`

**Interfaces:**
- Produces: `type DynamicCoupling struct { CentroidRMSCorr, BrightnessModDepth, BrightnessModRateHz float64 }`; `func computeCoupling(w wave.Wave, cfg AnalysisConfig) DynamicCoupling`.

- [ ] **Step 1: Write the failing test.** Generate a tone whose brightness rises with amplitude and assert positive correlation; and a static tone asserting ~0:

```go
func TestCoupling_PositiveWhenBrightnessTracksLoudness(t *testing.T) {
	sr := 48000
	coupled := synthBrightLoudTone(sr, 2.0, 440)   // helper: louder => more upper harmonics
	flat := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	c1 := computeCoupling(sustainWindow(coupled), DefaultAnalysisConfig())
	c0 := computeCoupling(sustainWindow(flat), DefaultAnalysisConfig())
	if c1.CentroidRMSCorr < 0.4 {
		t.Fatalf("coupled corr=%.2f, want >0.4", c1.CentroidRMSCorr)
	}
	if math.Abs(c0.CentroidRMSCorr) > 0.3 {
		t.Fatalf("flat corr=%.2f, want ~0", c0.CentroidRMSCorr)
	}
}
```

Add `synthBrightLoudTone` to `synthgen_test.go` (amplitude-modulate the upper partials together with overall gain).

- [ ] **Step 2: Run to verify it fails.** → FAIL.

- [ ] **Step 3: Implement.** Per STFT frame compute spectral centroid and frame RMS → two series. `CentroidRMSCorr` = Pearson correlation over the sustain frames. `BrightnessModDepth` = std(centroid)/mean(centroid). `BrightnessModRateHz` = dominant frequency of the detrended centroid series (FFT, 0–15 Hz). Guard zero-variance series → return 0.

- [ ] **Step 4: Run to verify pass.** PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 8: `noise_residual.go` — bow-noise profile

**Files:**
- Create: `src/go/internal/audio/fingerprint/noise_residual.go`
- Create: `src/go/internal/audio/fingerprint/noise_residual_test.go`

**Interfaces:**
- Produces: `type NoiseProfile struct { ResidualRatio, ResidualCentroidHz float64; ResidualBandLevels []float64 }`; `func computeNoiseProfile(w wave.Wave, f0 float64, cfg AnalysisConfig) NoiseProfile`.

- [ ] **Step 1: Write the failing test.** Compare a pure harmonic tone (low residual) vs the same tone + broadband noise (higher residual):

```go
func TestNoiseProfile_SeparatesResidual(t *testing.T) {
	sr := 48000
	pure := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	noisy := synthNoisyTone(sr, 2.0, 440, []float64{1, .5, .25}, 0.15) // helper: + white noise at 0.15 rms
	p := computeNoiseProfile(sustainWindow(pure), 440, DefaultAnalysisConfig())
	n := computeNoiseProfile(sustainWindow(noisy), 440, DefaultAnalysisConfig())
	if n.ResidualRatio <= p.ResidualRatio {
		t.Fatalf("noisy residual %.3f not > pure %.3f", n.ResidualRatio, p.ResidualRatio)
	}
}
```

Add `synthNoisyTone` to `synthgen_test.go` (deterministic PRNG noise — seed a `rand.New(rand.NewSource(1))`).

- [ ] **Step 2: Run to verify it fails.** → FAIL.

- [ ] **Step 3: Implement.** Estimate the harmonic spectrum by summing magnitude in narrow windows around k·f0; the residual = total energy minus harmonic energy. `ResidualRatio` = residual/total. Build the residual magnitude (zero out harmonic bins) → `ResidualCentroidHz` and per-band `ResidualBandLevels` over `cfg.Bands`.

- [ ] **Step 4: Run to verify pass.** PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 9: Wire new metrics into `Fingerprint` + `FromWave`

**Files:**
- Modify: `src/go/internal/audio/fingerprint/fingerprint.go`
- Test: `src/go/internal/audio/fingerprint/fingerprint_test.go`

**Interfaces:**
- Consumes: `computeVibrato`, `computeHarmonicTrajectory`, `computeSpectralEnvelope`, `computeCoupling`, `computeNoiseProfile`.
- Produces: `Fingerprint` gains pointer fields `Vibrato *VibratoFingerprint`, `Harmonics *HarmonicTrajectory`, `Envelope2 *SpectralEnvelope`, `Coupling *DynamicCoupling`, `Noise2 *NoiseProfile` (all `json:",omitempty"`).

- [ ] **Step 1: Write the failing test.**

```go
func TestFromWave_PopulatesNewMetrics(t *testing.T) {
	w := synthVibratoTone(48000, 2.0, 440, []float64{1, .6, .3, .15}, 6.0, 25)
	fp := FromWave(w, "x")
	if fp.Vibrato == nil || !fp.Vibrato.Detected {
		t.Fatal("Vibrato not populated/detected")
	}
	if fp.Coupling == nil || fp.Envelope2 == nil || fp.Harmonics == nil || fp.Noise2 == nil {
		t.Fatal("new metric sub-structs not populated")
	}
}
```

- [ ] **Step 2: Run to verify it fails.** → FAIL.

- [ ] **Step 3: Implement.** Add the five pointer fields to the `Fingerprint` struct (with doc comments) and call each `compute*` at the end of `FromWave`, assigning `&result`. Reuse the already-detected `F0Hz` and the sustain window.

- [ ] **Step 4: Run to verify pass.** PASS; full fingerprint suite PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 10: Extend `Distance`/`Score` with new terms

**Files:**
- Modify: `src/go/internal/audio/fingerprint/distance.go`
- Test: `src/go/internal/audio/fingerprint/distance_test.go`

**Interfaces:**
- Produces: `Score` gains `Vibrato, Coupling, Formant, HTrajectory float64`; `Total` includes them with documented weights (`Vibrato 0.8, Coupling 1.0, Formant 0.7, HTrajectory 0.6`). Nil sub-structs on either side → that term is 0 (not NaN).

- [ ] **Step 1: Write the failing tests.**

```go
func TestDistance_IdentityStillZero(t *testing.T) {
	w := synthVibratoTone(48000, 2.0, 440, []float64{1, .6, .3}, 6, 25)
	fp := FromWave(w, "a")
	if d := Distance(fp, fp).Total; d > 1e-9 {
		t.Fatalf("identity distance=%.6g, want 0", d)
	}
}

func TestDistance_VibratoTermDiscriminates(t *testing.T) {
	vib := FromWave(synthVibratoTone(48000, 2, 440, []float64{1, .6, .3}, 6, 30), "vib")
	flat := FromWave(synthTone(48000, 2, 440, []float64{1, .6, .3}), "flat")
	if Distance(vib, flat).Vibrato <= 0 {
		t.Fatal("vibrato term did not discriminate vibrato vs flat")
	}
}
```

- [ ] **Step 2: Run to verify they fail.** → FAIL (no `Vibrato` field on `Score`).

- [ ] **Step 3: Implement.** Add the four `Score` fields and compute each as a normalized, nil-safe delta (e.g. vibrato: mean of |ΔRate|/8, |ΔExtent|/50, |ΔAMDepth|/0.3; coupling: |ΔCentroidRMSCorr|/1 + |ΔModDepth|/0.3; formant: bridge-hill Hz delta/2000 + gain delta/20; htrajectory: mean |ΔDecayRate|/100 over harmonics). Add weighted contributions to `Total`. Document each weight inline.

- [ ] **Step 4: Run to verify pass.** Both tests PASS; existing `distance_test.go` PASS (identity/symmetry intact).

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 11: `synth-analyze` reporting + native-SR ref path

**Files:**
- Modify: `src/go/cmd/synth-analyze/main.go`
- Test: `src/go/cmd/synth-analyze/main_test.go`

**Interfaces:**
- Produces: `ref`/`synth`/`diff` print all new metrics (human-readable); JSON includes the new sub-structs; the `ref` path analyzes at the file's native sample rate (no silent resample to engine rate).

- [ ] **Step 1: Write the failing test.** Add a test that runs the `ref` analysis on the D5 file (via the same in-package path the CLI uses) and asserts the resulting fingerprint reports F0≈591 and a non-nil Vibrato. If the CLI funcs aren't unit-testable, refactor the analysis core into an exported `analyzeFile(path string, opts) (fingerprint.Fingerprint, error)` and test that.

```go
func TestAnalyzeFile_RefReportsCorrectF0AndVibrato(t *testing.T) {
	fp, err := analyzeFile("../../../356181__mtg__violin-d5.wav", analyzeOpts{})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}
	if fp.F0Hz < 575 || fp.F0Hz > 605 {
		t.Fatalf("F0=%.1f, want ~591", fp.F0Hz)
	}
	if fp.Vibrato == nil {
		t.Fatal("Vibrato nil")
	}
}
```

- [ ] **Step 2: Run to verify it fails.** → FAIL.

- [ ] **Step 3: Implement.** Refactor the `ref` command body into `analyzeFile`; ensure it decodes and analyzes at native SR (don't resample before `FromWave`). Add print sections for Vibrato / SpectralEnvelope / Coupling / HarmonicTrajectory / NoiseProfile mirroring the existing summary style. Ensure JSON marshals the new fields.

- [ ] **Step 4: Run to verify pass.** PASS. Manual smoke: `cd src/go && ../../.tools/go/bin/go run -tags '' ./cmd/synth-analyze ref -file ../../356181__mtg__violin-d5.wav` prints F0≈591 and a vibrato line (prod build; needs CGo). Note the test runs under `-tags test`.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Task 12: Real-reference characterization (golden) test

**Files:**
- Create: `src/go/internal/audio/fingerprint/violin_golden_test.go`

**Interfaces:**
- Consumes: everything above. Pure guardrail; tolerances not exact values.

- [ ] **Step 1: Write the test.**

```go
func TestViolinD5_Characterization(t *testing.T) {
	pcm, sr, err := loadRefWaveForTest(t)
	if err != nil {
		t.Skipf("reference unavailable: %v", err)
	}
	w := AutoSegment(wave.Wave{Samples: pcm, SampleRate: sr}, 1.5)
	fp := FromWave(w, "violin-d5")

	approx := func(name string, got, want, tol float64) {
		if math.Abs(got-want) > tol {
			t.Errorf("%s=%.3f, want %.3f ±%.3f", name, got, want, tol)
		}
	}
	approx("F0Hz", fp.F0Hz, 591, 8)
	if fp.Envelope2 == nil || fp.Envelope2.BridgeHillHz < 2000 || fp.Envelope2.BridgeHillHz > 2800 {
		t.Errorf("bridge hill = %v, want ~2.3–2.4 kHz", fp.Envelope2)
	}
	if fp.Coupling == nil {
		t.Fatal("coupling nil")
	}
	approx("CentroidRMSCorr", fp.Coupling.CentroidRMSCorr, 0.73, 0.30)
	if fp.Vibrato == nil || fp.Vibrato.RateHz < 3 || fp.Vibrato.RateHz > 7 {
		t.Errorf("vibrato rate = %v, want ~4 Hz", fp.Vibrato)
	}
}
```

- [ ] **Step 2: Run.** Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ -run TestViolinD5_Characterization -v` → Expected: PASS (all metrics within tolerance). If a metric is off, fix the responsible task's implementation — do not loosen tolerances to pass.

- [ ] **Step 3: gofmt + checkpoint.**

---

## Task 13: `synthmatch` edge fixes (seedpatch / seedVarName / duration)

**Files:**
- Modify: `src/go/internal/audio/synthmatch/seedpatch.go`, `cmd/synth-match/main.go`, `src/go/internal/audio/synthmatch/optimize.go`
- Test: `src/go/internal/audio/synthmatch/seedpatch_test.go`, `optimize_test.go`

**Interfaces:**
- Produces: `PatchSeedBlock` preserves values containing `/` in trailing comments; `seedVarName` never emits an empty prefix; optimizer renders the candidate at the reference's segmented duration.

- [ ] **Step 1: Write failing tests.**

```go
func TestPatchSeedBlock_PreservesSlashComment(t *testing.T) {
	src := []byte("var xSeed = RecipeParams{\n\t\"filter_cutoff\": 600, // bright / open\n}\n")
	out, err := PatchSeedBlock(src, "xSeed", map[string]float64{"filter_cutoff": 1234})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "1234") || !strings.Contains(s, "bright / open") {
		t.Fatalf("bad patch: %s", s)
	}
}
```

- [ ] **Step 2: Run to verify it fails.** → FAIL (regex eats the comment).

- [ ] **Step 3: Implement.** Make the value-replace match only up to the `//` (replace `[^,\n/]+` with a pattern that stops at comma/newline or `//`, preserving the comment). Guard `seedVarName` against empty parts. In `optimize.go`, thread the segmented reference duration into `RenderWithParams` (pass `len(ref.Samples)/ref.SampleRate` as `durSec`).

- [ ] **Step 4: Run to verify pass.** PASS; `./internal/audio/synthmatch/` suite PASS.

- [ ] **Step 5: gofmt + checkpoint.**

---

## Final verification

- [ ] Run the whole fast suite for touched packages:
  `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/... ./cmd/synth-analyze/... ./cmd/synth-match/...` → all PASS.
- [ ] gofmt-clean: `../../.tools/go/bin/gofmt -l internal/audio/fingerprint internal/audio/synthmatch cmd/synth-analyze` prints nothing.
- [ ] Leave everything uncommitted; summarize what changed and the D5 characterization numbers for the user.

## Self-review notes

- Spec §1 (bugs) → Tasks 1,2,3,13. §2 (5 metrics) → Tasks 4–8. §3 (integration) → Tasks 9,10,11. §4 (tests) → every task is TDD + Task 12 golden + Task 4 generators.
- Task ordering: Task 1 (decoder) and Task 4's `synthgen_test.go` are prerequisites for several later tests — if executed out of order, create `synthgen_test.go` first. Recommended execution order is the task number order, except move `synthgen_test.go` creation (Task 4 Step 1) ahead of Task 3 if Task 3 runs first.
- `fingerprint` must not import `audio` (cycle) — the reference WAV is loaded in tests via a local 24-bit reader (`loadRefWaveForTest`), not `audio.DecodeWAVToPCM`.
