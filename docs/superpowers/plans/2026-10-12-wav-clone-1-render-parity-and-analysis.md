# WAV clone — Plan 1: render parity + the analysis half

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the two foundations every later clone stage stands on — a document render that is byte-identical to what playback plays (`audio.RenderAsShipped`), and a pure-Go analysis pipeline that turns a struck/modal recording into a measured model (`clone.Analyze`: modes by sub-band ESPRIT, strike, decay law, residual noise bands) with a CLI report.

**Architecture:** `internal/audio` gains `renderDocVoice` (the voice buffer, shared with the voice cache) and `RenderAsShipped` (the document twin of `RenderAsPlayed`). A new dev-only package `internal/audio/clone` (+ `clone/linalg`) holds pure, separately testable stages (`Prepare` → `EstimateModes` → `EstimateStrike` / `FitDecayLaw` / `MeasureResidual` → `Analyze`), all tested on generated audio. `cmd/clone` resolves a reference through `wavref`, runs the analysis and writes `analysis.json` + `report.md` under `$BEATMO_WAV_REFS/clones/<inst>/`.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), cgo C engine (real-build tests only), no new dependencies.

**Spec:** `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md` (§1, §2, §6a's analysis half, §7, §8). Plans 2 (tables + `modal`/`strike`/`noiseband` engine modules) and 3 (invert / loss / CMA-ES refine / adopt / Synth tab) follow.

## Global Constraints

- No audio file is ever written into the repo; tests generate audio in `t.TempDir()`; a test that touches `wavref` sets `t.Setenv(wavref.EnvVar, t.TempDir())` (`TestNoTestReadsReferenceAudio`).
- Repo rule: **never `git add` / `git commit`** — the user commits. Every task ends with "leave uncommitted".
- Use the bundled toolchain: `cd src/go && ../../.tools/go/bin/go …`. Fast path: `-tags test -modfile=go.test.mod`. Real-C tests run without `-tags test` (need `make build/libdrums.a` first).
- The tree is edited concurrently by other sessions: touch only the files named in a task, run focused `-run` tests, never delete or "fix" files outside the task.
- `internal/audio/clone` and `clone/linalg` are dev-only: nothing reachable from `cmd/beatmo.go` or the WASM main may import them (`TestCloneIsDevOnly`).
- Analysis runs at the file's OWN sample rate (no resampling); time is seconds from the detected onset; amplitudes are linear full-scale.
- Refusals are typed (`*clone.Refusal` with `Code` ∈ `no_onset`, `short_decay`, `clipped`, `not_modal`); partial results are still returned / written.
- JSON output must never contain NaN/Inf (encoding/json rejects them) — clamp dB values at −200.

## Review Focus

1. **A reference still ringing when the clip ends** (VCSL tubular bells clamp at 2.97 s) — Prepare must set the window end to the clip end and NOT refuse. Pinned in Task 6 (`TestPrepareStillRingingAtClipEnd`).
2. **Onset at sample 0** (no pre-roll) — onset must be detected at ≤ 1 ms. Pinned in Task 6 (`TestPrepareOnsetAtSampleZero`).
3. **Digital silence** (generated audio with no noise at all) — no division by zero, no −Inf in the JSON. Pinned in Task 12 (`TestAnalyzeNoiselessJSONIsFinite`).
4. **44.1 kHz input** — the analysis must not assume 48 kHz anywhere (band layout, filters). Pinned in Task 8 (`TestEstimateModesAt44100`).
5. **A mode sitting near a band edge** — counted exactly once despite the band overlap. Pinned in Task 8 (`TestEstimateModesBandEdgeCountedOnce`).

---

## File structure

| File | Responsibility |
|---|---|
| `src/go/internal/audio/doc_voice_native.go` (modify) | `renderDocVoice`, `RenderAsShipped`; `renderDocIntoCache` calls `renderDocBlock` |
| `src/go/internal/audio/export.go` (modify) | `normalizeAndScaleAmp(buf, amp)`; `normalizeAndScale` wraps it |
| `src/go/internal/audio/render_as_shipped_test.go` (create) | real-C guard vs `RenderAsPlayed` |
| `src/go/internal/audio/synthmatch/render_prod.go`, `render_stub.go` (modify) | `RenderShipped` |
| `src/go/cmd/refs/compare.go` (modify) | family compare renders through `RenderShipped` |
| `src/go/internal/audio/clone/linalg/{matrix,hermitian,eig,lstsq}.go` (+ tests) | complex dense linear algebra |
| `src/go/internal/audio/clone/{doc,refusal,synth,prepare,subband,esprit,modes,decaylaw,strike,residual,analyze,report}.go` (+ tests) | the analysis stages |
| `src/go/internal/audio/clone/devonly_test.go` | import guard |
| `src/go/cmd/clone/main.go` (+ `main_test.go`) | CLI |
| `Makefile`, `CLAUDE.md`, `AGENTS.synth.instruments.md` (modify) | `make clone`, docs |

---

### Task 1: `RenderAsShipped` — the document render that equals playback

**Files:**
- Modify: `src/go/internal/audio/export.go` (`normalizeAndScale`)
- Modify: `src/go/internal/audio/doc_voice_native.go` (`renderDocIntoCache`)
- Test: `src/go/internal/audio/render_as_shipped_test.go`

**Interfaces:**
- Produces: `func RenderAsShipped(d instrument.Doc, bpm, sampleRate int, pitch float64) []float64` (`//go:build !test && !js`), `func renderDocVoice(d instrument.Doc, bpm, sampleRate int, pitch float64, pitched bool) []float32`, `func normalizeAndScaleAmp(buf []float32, amp float32)`.

- [ ] **Step 1: Write the failing guard**

```go
//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestRenderAsShippedMatchesPlayback pins RenderAsShipped — a pure function
// of a DOCUMENT — to RenderAsPlayed, which goes through the real dispatch
// path (project store, voice cache, playedVoice). Every analysis tool renders
// candidates through RenderAsShipped, so a drift here means every comparison
// measures a sound nobody hears.
func TestRenderAsShippedMatchesPlayback(t *testing.T) {
	for _, id := range BuiltinInstrumentIDs {
		d, ok := EnsureInstrumentDoc(id)
		if !ok {
			t.Fatalf("%s: no document", id)
		}
		for _, pitch := range []float64{0, 7} {
			want, sr := RenderAsPlayed(id, pitch, 1, 60)
			got := RenderAsShipped(d, bpm, sr, pitch)
			if len(got) != len(want) {
				t.Errorf("%s pitch %+v: %d samples, playback %d", id, pitch, len(got), len(want))
				continue
			}
			for i := range got {
				if math.Float64bits(got[i]) != math.Float64bits(want[i]) {
					t.Errorf("%s pitch %+v: sample %d = %v, playback %v", id, pitch, i, got[i], want[i])
					break
				}
			}
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `make build/libdrums.a && cd src/go && ../../.tools/go/bin/go test ./internal/audio -run TestRenderAsShippedMatchesPlayback -count=1`
Expected: FAIL — `undefined: RenderAsShipped`.

- [ ] **Step 3: Implement**

In `export.go`, split the amplitude lookup out of `normalizeAndScale`:

```go
// normalizeAndScale applies peak normalization followed by amplitude scaling
// to match the WASM audio processing pipeline in audio.js.
// This ensures cross-platform audio parity.
func normalizeAndScale(buf []float32, instrumentID string) {
	normalizeAndScaleAmp(buf, amplitudeForInstrument(instrumentID))
}

// normalizeAndScaleAmp is normalizeAndScale with the amplitude given
// directly — a document render must use ITS amplitude, never whatever is
// registered under its id (a candidate document is never registered).
func normalizeAndScaleAmp(buf []float32, amp float32) {
	if env.BypassNormalize.Get() {
		// Just apply amplitude scaling, no normalization
		for i := range buf {
			buf[i] *= amp
		}
		applyFadeOut(buf)
		return
	}
	// ... the existing body from "// 1. Find peak" down to applyFadeOut,
	// with `amp := amplitudeForInstrument(instrumentID)` deleted (amp is the
	// parameter now).
}
```

In `doc_voice_native.go`, replace the render half of `renderDocIntoCache` and add the two new functions:

```go
// renderDocIntoCache renders d's voice for id and stores it under its key
// (no-op returning the cached buffer when already present).
func renderDocIntoCache(id string, d instrument.Doc, bpm, sampleRate int, pitch float64, pitched, ignoreEdit bool, origin renderOrigin) []float32 {
	key, block := docVoiceKey(id, d, bpm, sampleRate, pitch, pitched, ignoreEdit)
	if buf, ok := globalVoiceCache.Get(key); ok {
		return buf
	}
	start := time.Now()
	buf := renderDocBlock(d, block, bpm, sampleRate)
	recordVoiceRender(time.Since(start), origin)
	if !ignoreEdit {
		if edit, ok := SampleEditFor(id); ok {
			buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
		}
	}
	globalVoiceCache.Put(key, buf)
	return buf
}

// renderDocBlock is the voice buffer of d given its already-flattened block:
// C render → document FX → gate tail → peak-normalise × d's amplitude. The
// ONE implementation behind the voice cache and RenderAsShipped.
func renderDocBlock(d instrument.Doc, block []float64, bpm, sampleRate int) []float32 {
	n := beatsDurationSamples(d.Render.Beats, bpm, sampleRate)
	buf := make([]float32, n)
	renderModularBlock(buf, sampleRate, n, modularBlock(block))
	if len(d.FX) > 0 {
		applyDefaultFX(buf, sampleRate, effectSlotsOf(d.FX))
	}
	if d.Render.GateTail > 0 {
		gateTail(buf, d.Render.GateTail)
	}
	normalizeAndScaleAmp(buf, float32(d.Render.Amplitude))
	return buf
}

// renderDocVoice flattens d (writing pitch when pitched, exactly as
// docVoiceKey does) and renders its voice buffer. No cache, no globals.
func renderDocVoice(d instrument.Doc, bpm, sampleRate int, pitch float64, pitched bool) []float32 {
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	if pitched {
		block[pitchFieldIndex()] = pitch
	}
	return renderDocBlock(d, block, bpm, sampleRate)
}

// RenderAsShipped returns the samples a trigger of document d at (pitch,
// duration 1) puts into the mix — RenderAsPlayed for a document that need not
// be registered anywhere. Melodic documents render AT pitch and play at rate
// 1; everything else renders at its own pitch and is resampled by
// 2^(pitch/12) through the same resampleVoice playedVoice uses. Writes no
// global state, so analysis tools may call it from several goroutines.
// Guard: TestRenderAsShippedMatchesPlayback.
func RenderAsShipped(d instrument.Doc, bpm, sampleRate int, pitch float64) []float64 {
	var buf []float32
	rate := 1.0
	if d.Render.Melodic {
		buf = renderDocVoice(d, bpm, sampleRate, pitch, true)
	} else {
		buf = renderDocVoice(d, bpm, sampleRate, 0, false)
		rate = pow2(pitch / 12.0)
	}
	v := &resampleVoice{src: &cVoice{buf: buf}, step: rate, buf: make([]float64, 0, len(buf)+2)}
	out := make([]float64, 0, len(buf))
	for {
		s, done := v.Sample()
		if done {
			return out
		}
		out = append(out, s)
	}
}
```

- [ ] **Step 4: Run the guard and the existing voice-cache tests**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio -run 'TestRenderAsShippedMatchesPlayback|VoiceCache|Prewarm|SampleEdit' -count=1`
Expected: PASS. If the guard fails for a melodic id only at pitch 7, check that `RenderAsPlayed` wraps the voice (e.g. a gate voice) — compare against `playedVoice` and mirror the wrapper, never loosen the byte comparison.

- [ ] **Step 5: Fast path still builds**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/audio/`
Expected: no output. Leave uncommitted.

---

### Task 2: `synthmatch.RenderShipped` and `refs compare` on it

**Files:**
- Modify: `src/go/internal/audio/synthmatch/render_prod.go`, `src/go/internal/audio/synthmatch/render_stub.go`
- Modify: `src/go/cmd/refs/compare.go` (`renderFamilyCompareOverrides`)
- Test: `src/go/internal/audio/synthmatch/render_shipped_test.go` (`!test`)

**Interfaces:**
- Consumes: `audio.RenderAsShipped` (Task 1).
- Produces: `func RenderShipped(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error)` in both builds. `ShippedRenderBPM = 120`.

- [ ] **Step 1: Write the failing test**

```go
//go:build !test

package synthmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestRenderShippedIncludesDocumentFX: for an instrument whose document
// carries FX, RenderShipped is the PLAYED sound (FX applied, normalised),
// which the raw RenderWithParams is not — the bug this exists to fix.
func TestRenderShippedIncludesDocumentFX(t *testing.T) {
	var id string
	for _, cand := range audio.BuiltinInstrumentIDs {
		if d, ok := audio.ShippedDoc(cand); ok && len(d.FX) > 0 {
			id = cand
			break
		}
	}
	if id == "" {
		t.Fatal("no shipped instrument carries FX")
	}
	sr := 48000
	got, err := RenderShipped(id, 0, sr, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := audio.EnsureInstrumentDoc(id)
	want := audio.RenderAsShipped(d, ShippedRenderBPM, sr, 0)
	for i := 0; i < len(got.Samples) && i < len(want); i++ {
		if math.Float64bits(got.Samples[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s sample %d: %v want %v", id, i, got.Samples[i], want[i])
		}
	}
	if len(got.Samples) != sr {
		t.Fatalf("len %d, want %d (1 s, zero-padded past the voice)", len(got.Samples), sr)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/synthmatch -run TestRenderShippedIncludesDocumentFX -count=1`
Expected: FAIL — `undefined: RenderShipped`.

- [ ] **Step 3: Implement**

Append to `render_prod.go`:

```go
// ShippedRenderBPM is the tempo RenderShipped sizes a voice at — the
// engine's default, and the tempo clone documents size their voices for.
const ShippedRenderBPM = 120

// RenderShipped renders instID (with overrides set through its document) as
// a trigger PLAYS it — document FX, gate tail, normalisation, amplitude, and
// percussion pitched by resampling — via audio.RenderAsShipped, then fits it
// to durSec (0 = the instrument default): truncated, or zero-padded past the
// end of the voice (silence is what plays there). The comparison tools use
// this; RenderWithParams stays the RAW C render for the tools that need it
// (measure-loudness computes the amplitude, so it must not see it applied).
func RenderShipped(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	ov := make(map[string]float64, len(overrides))
	for k, v := range overrides {
		if k != "pitch" {
			ov[k] = v
		}
	}
	d, err := audio.InstrumentDocWithFields(instID, ov)
	if err != nil {
		return wave.Wave{}, fmt.Errorf("synthmatch: %w", err)
	}
	if durSec <= 0 {
		durSec = audio.ConfigForInstrument(instID).DurationSec
	}
	n := int(float64(sr) * durSec)
	if n < 1 {
		return wave.Wave{}, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	v := audio.RenderAsShipped(d, ShippedRenderBPM, sr, pitch)
	out := make([]float64, n)
	copy(out, v)
	return wave.Wave{Samples: out, SampleRate: sr, Label: instID}, nil
}
```

Append to `render_stub.go` (test build — the stub has no FX or normalisation, so it is the pure-Go synth):

```go
// ShippedRenderBPM mirrors the production constant.
const ShippedRenderBPM = 120

// RenderShipped is the test-build stand-in: the pure-Go synth of
// RenderWithParams (there is no C engine, FX or normalisation here).
func RenderShipped(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error) {
	return RenderWithParams(instID, pitch, sr, durSec, overrides)
}
```

In `cmd/refs/compare.go` `renderFamilyCompareOverrides`, replace the two-branch render with one call:

```go
		w, err := synthmatch.RenderShipped(inst, pitch, sr, dur, ov)
```

and update the function's doc comment: "renders on top of insts[i]'s shipped defaults via synthmatch.RenderShipped — the sound as PLAYED (FX, gate, normalisation; percussion pitched by resampling)".

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/synthmatch -run TestRenderShipped -count=1 && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ ./internal/audio/synthmatch/ -count=1`
Expected: PASS. Then the real-build refs tests: `../../.tools/go/bin/go test ./cmd/refs/ -count=1` — a red test here is a behaviour change of `refs compare` (FX now applied); read the failure, and if it is a golden of compare output, regenerate it with the test's documented update flag and say so in the summary. Leave uncommitted.

---

### Task 3: `linalg` — matrix type and Hermitian eigensolver

**Files:**
- Create: `src/go/internal/audio/clone/linalg/matrix.go`, `hermitian.go`
- Test: `src/go/internal/audio/clone/linalg/hermitian_test.go`

**Interfaces:**
- Produces: `type Matrix struct{ Rows, Cols int; Data []complex128 }`, `New(r, c int) *Matrix`, `(*Matrix).At/Set/Clone`, `Mul(a, b *Matrix) *Matrix`, `ConjTranspose(a *Matrix) *Matrix`, `EigHermitian(a *Matrix) (vals []float64, vecs *Matrix)` (descending, unit eigenvectors as columns).

- [ ] **Step 1: Write the failing test**

```go
package linalg

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

func randHermitian(n int, seed int64) *Matrix {
	r := rand.New(rand.NewSource(seed))
	a := New(n, n)
	for i := 0; i < n; i++ {
		a.Set(i, i, complex(r.NormFloat64(), 0))
		for j := i + 1; j < n; j++ {
			v := complex(r.NormFloat64(), r.NormFloat64())
			a.Set(i, j, v)
			a.Set(j, i, cmplx.Conj(v))
		}
	}
	return a
}

func TestEigHermitianReconstructs(t *testing.T) {
	for _, n := range []int{1, 2, 5, 40} {
		a := randHermitian(n, int64(n))
		vals, v := EigHermitian(a)
		for i := 1; i < n; i++ {
			if vals[i] > vals[i-1] {
				t.Fatalf("n=%d: eigenvalues not descending: %v", n, vals)
			}
		}
		// A·v_k = λ_k·v_k and vᴴv = I
		for k := 0; k < n; k++ {
			for i := 0; i < n; i++ {
				var av complex128
				for j := 0; j < n; j++ {
					av += a.At(i, j) * v.At(j, k)
				}
				if cmplx.Abs(av-complex(vals[k], 0)*v.At(i, k)) > 1e-9 {
					t.Fatalf("n=%d: A·v != λv at (%d,%d)", n, i, k)
				}
			}
		}
		vh := Mul(ConjTranspose(v), v)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				want := 0.0
				if i == j {
					want = 1
				}
				if cmplx.Abs(vh.At(i, j)-complex(want, 0)) > 1e-10 {
					t.Fatalf("n=%d: VᴴV not identity at (%d,%d): %v", n, i, j, vh.At(i, j))
				}
			}
		}
	}
}

func TestEigHermitianKnownSpectrum(t *testing.T) {
	// diag(3, 1, -2) rotated by a fixed unitary: eigenvalues must come back.
	q := randHermitian(3, 7) // any Hermitian has a unitary eigenbasis
	_, u := EigHermitian(q)
	d := New(3, 3)
	d.Set(0, 0, 3)
	d.Set(1, 1, 1)
	d.Set(2, 2, -2)
	a := Mul(Mul(u, d), ConjTranspose(u))
	vals, _ := EigHermitian(a)
	for i, want := range []float64{3, 1, -2} {
		if math.Abs(vals[i]-want) > 1e-10 {
			t.Fatalf("eig %d = %v, want %v", i, vals[i], want)
		}
	}
}

// TestEigHermitianMatchesJacobi cross-checks the tridiagonal-QL solver
// against the Jacobi oracle on random and on rank-deficient (ESPRIT-like)
// matrices.
func TestEigHermitianMatchesJacobi(t *testing.T) {
	for _, n := range []int{3, 17, 64} {
		a := randHermitian(n, int64(100+n))
		got, _ := EigHermitian(a)
		want, _ := eigHermitianJacobi(a)
		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-9*math.Max(1, math.Abs(want[i])) {
				t.Fatalf("n=%d eig %d: %v vs Jacobi %v", n, i, got[i], want[i])
			}
		}
	}
	// rank 2 + tiny noise, like a two-mode covariance
	n := 32
	a := New(n, n)
	r := rand.New(rand.NewSource(9))
	for _, z := range []complex128{cmplx.Rect(0.99, 0.5), cmplx.Rect(0.98, -1)} {
		u := make([]complex128, n)
		acc := complex(1, 0)
		for i := range u {
			u[i] = acc
			acc *= z
		}
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				a.Set(i, j, a.At(i, j)+u[i]*cmplx.Conj(u[j]))
			}
		}
	}
	for i := 0; i < n; i++ {
		a.Set(i, i, a.At(i, i)+complex(1e-6*(1+r.Float64()), 0))
	}
	got, _ := EigHermitian(a)
	want, _ := eigHermitianJacobi(a)
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9*math.Max(1, math.Abs(want[i])) {
			t.Fatalf("rank-2 eig %d: %v vs Jacobi %v", i, got[i], want[i])
		}
	}
}

func BenchmarkEigHermitian128(b *testing.B) {
	a := randHermitian(128, 5)
	for i := 0; i < b.N; i++ {
		EigHermitian(a)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -count=1`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement `matrix.go`**

```go
// Package linalg is the small dense complex linear algebra the WAV-clone
// analysis needs: a Hermitian eigensolver (ESPRIT signal subspaces), the
// eigenvalues of a general complex matrix (ESPRIT poles) and complex least
// squares (rotational invariance, Vandermonde amplitudes). Matrices are at
// most a few hundred wide and every routine is deterministic. gonum was
// considered and rejected: it has no decompositions for complex matrices.
//
// Dev-only: nothing reachable from a shipped binary imports this package
// (TestCloneIsDevOnly).
package linalg

import "math/cmplx"

// Matrix is a dense row-major complex matrix.
type Matrix struct {
	Rows, Cols int
	Data       []complex128
}

// New returns a zero rows×cols matrix.
func New(rows, cols int) *Matrix {
	return &Matrix{Rows: rows, Cols: cols, Data: make([]complex128, rows*cols)}
}

// At returns element (i, j).
func (m *Matrix) At(i, j int) complex128 { return m.Data[i*m.Cols+j] }

// Set writes element (i, j).
func (m *Matrix) Set(i, j int, v complex128) { m.Data[i*m.Cols+j] = v }

// Clone returns a deep copy.
func (m *Matrix) Clone() *Matrix {
	c := &Matrix{Rows: m.Rows, Cols: m.Cols, Data: make([]complex128, len(m.Data))}
	copy(c.Data, m.Data)
	return c
}

// Mul returns a·b.
func Mul(a, b *Matrix) *Matrix {
	out := New(a.Rows, b.Cols)
	for i := 0; i < a.Rows; i++ {
		for k := 0; k < a.Cols; k++ {
			aik := a.At(i, k)
			if aik == 0 {
				continue
			}
			for j := 0; j < b.Cols; j++ {
				out.Data[i*out.Cols+j] += aik * b.At(k, j)
			}
		}
	}
	return out
}

// ConjTranspose returns aᴴ.
func ConjTranspose(a *Matrix) *Matrix {
	out := New(a.Cols, a.Rows)
	for i := 0; i < a.Rows; i++ {
		for j := 0; j < a.Cols; j++ {
			out.Set(j, i, cmplx.Conj(a.At(i, j)))
		}
	}
	return out
}
```

- [ ] **Step 4: Implement `hermitian.go`**

```go
package linalg

import (
	"math"
	"math/cmplx"
	"sort"
)

// eigHermitianJacobi is the cyclic-Jacobi Hermitian eigensolver: accurate to
// working precision and simple, but O(n³) PER SWEEP (0.25 s at 128 × 128), so
// it is kept only as the test oracle for EigHermitian.
//
// Each rotation first turns the pivot a_pq real with a diagonal phase, then
// applies the classic real Jacobi rotation (Numerical Recipes' θ/t/c/s); the
// product J = diag(1, e^{−iφ})·P is unitary and zeroes a_pq.
func eigHermitianJacobi(a *Matrix) ([]float64, *Matrix) {
	n := a.Rows
	A := a.Clone()
	V := New(n, n)
	for i := 0; i < n; i++ {
		V.Set(i, i, 1)
	}
	var total float64
	for _, v := range A.Data {
		total += real(v)*real(v) + imag(v)*imag(v)
	}
	tol := 1e-30 * total
	for sweep := 0; sweep < 100; sweep++ {
		var off float64
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				v := A.At(p, q)
				off += real(v)*real(v) + imag(v)*imag(v)
			}
		}
		if off <= tol {
			break
		}
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				apq := A.At(p, q)
				mag := cmplx.Abs(apq)
				if mag*mag <= tol/float64(n*n) {
					continue
				}
				app, aqq := real(A.At(p, p)), real(A.At(q, q))
				theta := (aqq - app) / (2 * mag)
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				eNeg := cmplx.Conj(apq / complex(mag, 0)) // e^{−iφ}
				jpp, jpq := complex(c, 0), complex(s, 0)
				jqp, jqq := complex(-s, 0)*eNeg, complex(c, 0)*eNeg
				for k := 0; k < n; k++ { // A ← A·J
					akp, akq := A.At(k, p), A.At(k, q)
					A.Set(k, p, akp*jpp+akq*jqp)
					A.Set(k, q, akp*jpq+akq*jqq)
				}
				for k := 0; k < n; k++ { // A ← Jᴴ·A
					apk, aqk := A.At(p, k), A.At(q, k)
					A.Set(p, k, cmplx.Conj(jpp)*apk+cmplx.Conj(jqp)*aqk)
					A.Set(q, k, cmplx.Conj(jpq)*apk+cmplx.Conj(jqq)*aqk)
				}
				A.Set(p, q, 0)
				A.Set(q, p, 0)
				A.Set(p, p, complex(real(A.At(p, p)), 0))
				A.Set(q, q, complex(real(A.At(q, q)), 0))
				for k := 0; k < n; k++ { // V ← V·J
					vkp, vkq := V.At(k, p), V.At(k, q)
					V.Set(k, p, vkp*jpp+vkq*jqp)
					V.Set(k, q, vkp*jpq+vkq*jqq)
				}
			}
		}
	}
	diag := make([]float64, n)
	for i := range diag {
		diag[i] = real(A.At(i, i))
	}
	return sortDescending(diag, V)
}

// sortDescending orders eigenpairs by descending eigenvalue (stable).
func sortDescending(diag []float64, V *Matrix) ([]float64, *Matrix) {
	n := len(diag)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool { return diag[idx[x]] > diag[idx[y]] })
	vals := make([]float64, n)
	vecs := New(V.Rows, n)
	for c, i := range idx {
		vals[c] = diag[i]
		for r := 0; r < V.Rows; r++ {
			vecs.Set(r, c, V.At(r, i))
		}
	}
	return vals, vecs
}

// EigHermitian returns the eigenvalues of the Hermitian matrix a in
// DESCENDING order and the matching unit eigenvectors as the columns of vecs.
// a is not modified. Householder reduction to a Hermitian tridiagonal T
// (A = Q·T·Qᴴ), a diagonal phase D that makes T real (T = D·T'·Dᴴ), then the
// implicit QL iteration on T' with eigenvector accumulation: eigenvectors are
// Q·D·Z. O(n³) once — ~100× faster than Jacobi at the 128 × 128 ESPRIT
// covariances, same accuracy (TestEigHermitianMatchesJacobi).
func EigHermitian(a *Matrix) ([]float64, *Matrix) {
	n := a.Rows
	if n == 0 {
		return nil, New(0, 0)
	}
	A := a.Clone()
	Q := New(n, n)
	for i := 0; i < n; i++ {
		Q.Set(i, i, 1)
	}
	v := make([]complex128, n)
	w := make([]complex128, n)
	for k := 0; k < n-2; k++ {
		var norm float64
		for i := k + 1; i < n; i++ {
			x := A.At(i, k)
			norm += real(x)*real(x) + imag(x)*imag(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		x0 := A.At(k+1, k)
		ph := complex(1, 0)
		if x0 != 0 {
			ph = x0 / complex(cmplx.Abs(x0), 0)
		}
		alpha := -ph * complex(norm, 0)
		for i := range v {
			v[i] = 0
		}
		for i := k + 1; i < n; i++ {
			v[i] = A.At(i, k)
		}
		v[k+1] -= alpha
		var vn float64
		for i := k + 1; i < n; i++ {
			vn += real(v[i])*real(v[i]) + imag(v[i])*imag(v[i])
		}
		if vn == 0 {
			continue
		}
		vn = math.Sqrt(vn)
		for i := k + 1; i < n; i++ {
			v[i] /= complex(vn, 0)
		}
		// H = I − 2vvᴴ; HAH = A − v·wᴴ − w·vᴴ with p = A·v, κ = vᴴp, w = 2(p − κv).
		// v is zero above k+1, so only rows/columns ≥ k are touched (column k
		// becomes [.., alpha, 0, …]).
		var kappa complex128
		ad := A.Data
		for i := k; i < n; i++ {
			var s complex128
			row := ad[i*n : i*n+n]
			for j := k + 1; j < n; j++ {
				s += row[j] * v[j]
			}
			w[i] = s
		}
		for i := k + 1; i < n; i++ {
			kappa += cmplx.Conj(v[i]) * w[i]
		}
		for i := k; i < n; i++ {
			w[i] = 2 * (w[i] - kappa*v[i])
		}
		cv := make([]complex128, n)
		cw := make([]complex128, n)
		for j := k; j < n; j++ {
			cv[j], cw[j] = cmplx.Conj(v[j]), cmplx.Conj(w[j])
		}
		for i := k; i < n; i++ {
			row := ad[i*n : i*n+n]
			vi, wi := v[i], w[i]
			for j := k; j < n; j++ {
				row[j] -= vi*cw[j] + wi*cv[j]
			}
		}
		for i := k + 2; i < n; i++ {
			A.Set(i, k, 0)
			A.Set(k, i, 0)
		}
		// Q ← Q·H
		for i := 0; i < n; i++ {
			row := Q.Data[i*n : i*n+n]
			var s complex128
			for j := k + 1; j < n; j++ {
				s += row[j] * v[j]
			}
			s *= 2
			for j := k + 1; j < n; j++ {
				row[j] -= s * cv[j]
			}
		}
	}
	// T = D·T'·Dᴴ with T' real: δ_0 = 1, δ_{i+1} = δ_i·e_i/|e_i|.
	d := make([]float64, n)
	e := make([]float64, n)
	delta := make([]complex128, n)
	delta[0] = 1
	for i := 0; i < n; i++ {
		d[i] = real(A.At(i, i))
		if i+1 < n {
			sub := A.At(i+1, i)
			mag := cmplx.Abs(sub)
			e[i] = mag
			if mag > 0 {
				delta[i+1] = delta[i] * sub / complex(mag, 0)
			} else {
				delta[i+1] = delta[i]
			}
		}
	}
	z := make([]float64, n*n)
	for i := 0; i < n; i++ {
		z[i*n+i] = 1
	}
	tqli(d, e, z, n)
	// eigenvectors = Q·D·Z
	vecs := New(n, n)
	qd := make([]complex128, n)
	for r := 0; r < n; r++ {
		for k := 0; k < n; k++ {
			qd[k] = Q.Data[r*n+k] * delta[k]
		}
		out := vecs.Data[r*n : r*n+n]
		for k := 0; k < n; k++ {
			q := qd[k]
			zr := z[k*n : k*n+n]
			for c := 0; c < n; c++ {
				out[c] += q * complex(zr[c], 0)
			}
		}
	}
	return sortDescending(d, vecs)
}

// tqli is the implicit QL iteration with Wilkinson-type shifts on the real
// symmetric tridiagonal (d diagonal, e[i] = T[i+1][i], e[n−1] ignored),
// accumulating the rotations into z (row-major n×n). Numerical Recipes §11.3.
func tqli(d, e, z []float64, n int) {
	if n > 0 {
		e[n-1] = 0
	}
	for l := 0; l < n; l++ {
		for iter := 0; iter < 60; iter++ {
			m := l
			for ; m < n-1; m++ {
				dd := math.Abs(d[m]) + math.Abs(d[m+1])
				if math.Abs(e[m])+dd == dd {
					break
				}
			}
			if m == l {
				break
			}
			g := (d[l+1] - d[l]) / (2 * e[l])
			r := math.Hypot(g, 1)
			g = d[m] - d[l] + e[l]/(g+math.Copysign(r, g))
			s, c, p := 1.0, 1.0, 0.0
			underflow := false
			i := m - 1
			for ; i >= l; i-- {
				f := s * e[i]
				b := c * e[i]
				r = math.Hypot(f, g)
				e[i+1] = r
				if r == 0 {
					d[i+1] -= p
					e[m] = 0
					underflow = true
					break
				}
				s = f / r
				c = g / r
				g = d[i+1] - p
				r = (d[i]-g)*s + 2*c*b
				p = s * r
				d[i+1] = g + p
				g = c*r - b
				for k := 0; k < n; k++ {
					f = z[k*n+i+1]
					z[k*n+i+1] = s*z[k*n+i] + c*f
					z[k*n+i] = c*z[k*n+i] - s*f
				}
			}
			if underflow {
				continue
			}
			d[l] -= p
			e[l] = g
			e[m] = 0
		}
	}
}```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 4: `linalg` — general complex eigenvalues

**Files:**
- Create: `src/go/internal/audio/clone/linalg/eig.go`
- Test: `src/go/internal/audio/clone/linalg/eig_test.go`

**Interfaces:**
- Produces: `func Eigenvalues(a *Matrix) ([]complex128, error)` (order unspecified).

- [ ] **Step 1: Write the failing test**

```go
package linalg

import (
	"math/cmplx"
	"math/rand"
	"sort"
	"testing"
)

// matching sorts both lists by (real, imag) and compares pairwise.
func sameSpectrum(t *testing.T, got, want []complex128, tol float64) {
	t.Helper()
	key := func(v []complex128) func(i, j int) bool {
		return func(i, j int) bool {
			if real(v[i]) != real(v[j]) {
				return real(v[i]) < real(v[j])
			}
			return imag(v[i]) < imag(v[j])
		}
	}
	g := append([]complex128(nil), got...)
	w := append([]complex128(nil), want...)
	sort.Slice(g, key(g))
	sort.Slice(w, key(w))
	if len(g) != len(w) {
		t.Fatalf("got %d eigenvalues, want %d", len(g), len(w))
	}
	for i := range g {
		if cmplx.Abs(g[i]-w[i]) > tol {
			t.Fatalf("eigenvalue %d: got %v want %v (all got %v)", i, g[i], w[i], g)
		}
	}
}

func TestEigenvaluesKnownPoles(t *testing.T) {
	// Damped-pole-like spectrum, incl. a close pair (a doublet).
	want := []complex128{
		cmplx.Rect(0.999, 0.3), cmplx.Rect(0.998, 0.3005), cmplx.Rect(0.95, -1.2),
		cmplx.Rect(0.9, 2.5), cmplx.Rect(0.7, 0.01), 0.5,
	}
	n := len(want)
	r := rand.New(rand.NewSource(3))
	s := New(n, n) // a random (invertible with probability 1) similarity
	for i := range s.Data {
		s.Data[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	// A = S·D·S⁻¹, built as solving S·X = D·S⁻¹… simpler: A = S·D·S⁻¹ via
	// the QR solve of S against D columns.
	d := New(n, n)
	for i, v := range want {
		d.Set(i, i, v)
	}
	sd := Mul(s, d)
	f, err := FactorQR(ConjTranspose(s)) // (S⁻¹)ᴴ = (Sᴴ)⁻¹; solve Sᴴ·Y = I → Y = S⁻ᴴ
	if err != nil {
		t.Fatal(err)
	}
	sinvH := New(n, n)
	for j := 0; j < n; j++ {
		e := make([]complex128, n)
		e[j] = 1
		y := f.Solve(e)
		for i := 0; i < n; i++ {
			sinvH.Set(i, j, y[i])
		}
	}
	a := Mul(sd, ConjTranspose(sinvH))
	got, err := Eigenvalues(a)
	if err != nil {
		t.Fatal(err)
	}
	sameSpectrum(t, got, want, 1e-8)
}

func TestEigenvaluesTriangularAndScalar(t *testing.T) {
	a := New(3, 3)
	a.Set(0, 0, 2)
	a.Set(0, 1, 5)
	a.Set(1, 1, complex(0, 1))
	a.Set(1, 2, 4)
	a.Set(2, 2, -1)
	got, err := Eigenvalues(a)
	if err != nil {
		t.Fatal(err)
	}
	sameSpectrum(t, got, []complex128{2, complex(0, 1), -1}, 1e-12)
	one := New(1, 1)
	one.Set(0, 0, 0.3+0.4i)
	got, _ = Eigenvalues(one)
	sameSpectrum(t, got, []complex128{0.3 + 0.4i}, 0)
}
```

(`FactorQR` / `Solve` come from Task 5; write Task 5's test+code first if executing strictly in order fails to compile — the two tasks are reviewed together.)

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -run Eigenvalues -count=1`
Expected: FAIL — `undefined: Eigenvalues`.

- [ ] **Step 3: Implement `eig.go`**

```go
package linalg

import (
	"errors"
	"math"
	"math/cmplx"
)

// ErrNoConvergence is returned when the QR iteration exceeds its budget.
var ErrNoConvergence = errors.New("linalg: QR iteration did not converge")

// Eigenvalues returns the eigenvalues of the square complex matrix a:
// Householder reduction to upper Hessenberg form, then single-shift QR with
// Wilkinson shifts (an exceptional shift every 10th iteration) and deflation
// at negligible subdiagonals. Only eigenvalues are needed (ESPRIT poles), so
// each QR step touches only the active diagonal block. a is not modified.
func Eigenvalues(a *Matrix) ([]complex128, error) {
	n := a.Rows
	if n == 0 {
		return nil, nil
	}
	h := hessenberg(a)
	var hnorm float64
	for _, v := range h.Data {
		if m := cmplx.Abs(v); m > hnorm {
			hnorm = m
		}
	}
	const eps = 2.220446049250313e-16
	eig := make([]complex128, n)
	hi := n - 1
	iter, total := 0, 0
	for hi >= 0 {
		if hi == 0 {
			eig[0] = h.At(0, 0)
			break
		}
		l := hi
		for ; l > 0; l-- {
			s := cmplx.Abs(h.At(l-1, l-1)) + cmplx.Abs(h.At(l, l))
			if s == 0 {
				s = hnorm
			}
			if cmplx.Abs(h.At(l, l-1)) <= eps*s {
				h.Set(l, l-1, 0)
				break
			}
		}
		if l == hi {
			eig[hi] = h.At(hi, hi)
			hi--
			iter = 0
			continue
		}
		if total > 100*n {
			return nil, ErrNoConvergence
		}
		iter++
		total++
		mu := wilkinson(h.At(hi-1, hi-1), h.At(hi-1, hi), h.At(hi, hi-1), h.At(hi, hi))
		if iter%10 == 0 {
			mu = h.At(hi, hi) + complex(cmplx.Abs(h.At(hi, hi-1)), 0)
		}
		qrStep(h, l, hi, mu)
	}
	return eig, nil
}

// wilkinson returns the eigenvalue of [[a b] [c d]] closer to d.
func wilkinson(a, b, c, d complex128) complex128 {
	tr := (a + d) / 2
	disc := cmplx.Sqrt((a-d)*(a-d)/4 + b*c)
	m1, m2 := tr+disc, tr-disc
	if cmplx.Abs(m1-d) < cmplx.Abs(m2-d) {
		return m1
	}
	return m2
}

// givens returns (c, s), c real, with [c s; −s̄ c]·[a; b] = [r; 0].
func givens(a, b complex128) (float64, complex128) {
	if b == 0 {
		return 1, 0
	}
	if a == 0 {
		return 0, cmplx.Conj(b) / complex(cmplx.Abs(b), 0)
	}
	aa, bb := cmplx.Abs(a), cmplx.Abs(b)
	r := math.Hypot(aa, bb)
	return aa / r, (a / complex(aa, 0)) * cmplx.Conj(b) / complex(r, 0)
}

// qrStep performs one shifted QR step H ← RQ + μI on the active block
// [l, hi] of the Hessenberg matrix h.
func qrStep(h *Matrix, l, hi int, mu complex128) {
	for k := l; k <= hi; k++ {
		h.Set(k, k, h.At(k, k)-mu)
	}
	cs := make([]float64, hi-l)
	sn := make([]complex128, hi-l)
	for k := l; k < hi; k++ {
		c, s := givens(h.At(k, k), h.At(k+1, k))
		cs[k-l], sn[k-l] = c, s
		cc := complex(c, 0)
		for j := k; j <= hi; j++ {
			x, y := h.At(k, j), h.At(k+1, j)
			h.Set(k, j, cc*x+s*y)
			h.Set(k+1, j, -cmplx.Conj(s)*x+cc*y)
		}
	}
	for k := l; k < hi; k++ {
		cc, s := complex(cs[k-l], 0), sn[k-l]
		for i := l; i <= k+1; i++ {
			x, y := h.At(i, k), h.At(i, k+1)
			h.Set(i, k, x*cc+y*cmplx.Conj(s))
			h.Set(i, k+1, -x*s+y*cc)
		}
	}
	for k := l; k <= hi; k++ {
		h.Set(k, k, h.At(k, k)+mu)
	}
}

// hessenberg reduces a to upper Hessenberg form by Householder similarity
// transforms P = I − 2vvᴴ.
func hessenberg(a *Matrix) *Matrix {
	n := a.Rows
	h := a.Clone()
	v := make([]complex128, n)
	for k := 0; k < n-2; k++ {
		var norm float64
		for i := k + 1; i < n; i++ {
			x := h.At(i, k)
			norm += real(x)*real(x) + imag(x)*imag(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		x0 := h.At(k+1, k)
		ph := complex(1, 0)
		if x0 != 0 {
			ph = x0 / complex(cmplx.Abs(x0), 0)
		}
		alpha := -ph * complex(norm, 0)
		var vn float64
		for i := k + 1; i < n; i++ {
			v[i] = h.At(i, k)
		}
		v[k+1] -= alpha
		for i := k + 1; i < n; i++ {
			vn += real(v[i])*real(v[i]) + imag(v[i])*imag(v[i])
		}
		if vn == 0 {
			continue
		}
		vn = math.Sqrt(vn)
		for i := k + 1; i < n; i++ {
			v[i] /= complex(vn, 0)
		}
		for j := 0; j < n; j++ { // H ← P·H
			var dot complex128
			for i := k + 1; i < n; i++ {
				dot += cmplx.Conj(v[i]) * h.At(i, j)
			}
			for i := k + 1; i < n; i++ {
				h.Set(i, j, h.At(i, j)-2*v[i]*dot)
			}
		}
		for i := 0; i < n; i++ { // H ← H·P
			var dot complex128
			for j := k + 1; j < n; j++ {
				dot += h.At(i, j) * v[j]
			}
			for j := k + 1; j < n; j++ {
				h.Set(i, j, h.At(i, j)-2*dot*cmplx.Conj(v[j]))
			}
		}
		for i := k + 2; i < n; i++ {
			h.Set(i, k, 0)
		}
	}
	return h
}
```

- [ ] **Step 4: Run the tests** (after Task 5's `FactorQR` exists)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 5: `linalg` — complex least squares

**Files:**
- Create: `src/go/internal/audio/clone/linalg/lstsq.go`
- Test: `src/go/internal/audio/clone/linalg/lstsq_test.go`

**Interfaces:**
- Produces: `type QR struct{ Q, R *Matrix }`, `func FactorQR(a *Matrix) (*QR, error)`, `func (f *QR) Solve(b []complex128) []complex128`, `var ErrRankDeficient`.

- [ ] **Step 1: Write the failing test**

```go
package linalg

import (
	"errors"
	"math/cmplx"
	"math/rand"
	"testing"
)

func TestLeastSquaresRecoversKnownSolution(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	m, n := 50, 6
	a := New(m, n)
	for i := range a.Data {
		a.Data[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	x := []complex128{1, -2i, 0.5 + 0.5i, 3, -1, 0.25i}
	b := make([]complex128, m)
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			b[i] += a.At(i, j) * x[j]
		}
	}
	f, err := FactorQR(a)
	if err != nil {
		t.Fatal(err)
	}
	got := f.Solve(b)
	for j := range x {
		if cmplx.Abs(got[j]-x[j]) > 1e-10 {
			t.Fatalf("x[%d] = %v, want %v", j, got[j], x[j])
		}
	}
}

func TestLeastSquaresRejectsDependentColumns(t *testing.T) {
	a := New(4, 2)
	for i := 0; i < 4; i++ {
		a.Set(i, 0, complex(float64(i+1), 0))
		a.Set(i, 1, complex(2*float64(i+1), 0))
	}
	if _, err := FactorQR(a); !errors.Is(err, ErrRankDeficient) {
		t.Fatalf("err = %v, want ErrRankDeficient", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -run LeastSquares -count=1`
Expected: FAIL — `undefined: FactorQR`.

- [ ] **Step 3: Implement `lstsq.go`**

```go
package linalg

import (
	"errors"
	"math"
	"math/cmplx"
)

// ErrRankDeficient is returned when a column is numerically dependent on the
// ones before it.
var ErrRankDeficient = errors.New("linalg: matrix is rank deficient")

// QR is a thin QR factorisation (Q m×n with orthonormal columns, R n×n upper
// triangular).
type QR struct{ Q, R *Matrix }

// FactorQR factors a (m ≥ n) by modified Gram–Schmidt with one full
// re-orthogonalisation pass ("twice is enough"), which keeps Q orthonormal to
// working precision for the near-dependent columns a Vandermonde matrix of a
// doublet produces.
func FactorQR(a *Matrix) (*QR, error) {
	m, n := a.Rows, a.Cols
	if m < n {
		return nil, errors.New("linalg: FactorQR needs rows >= cols")
	}
	q := New(m, n)
	r := New(n, n)
	v := make([]complex128, m)
	var scale float64
	for j := 0; j < n; j++ {
		for i := 0; i < m; i++ {
			v[i] = a.At(i, j)
		}
		if cn := norm2(v); cn > scale {
			scale = cn
		}
		for pass := 0; pass < 2; pass++ {
			for k := 0; k < j; k++ {
				var dot complex128
				for i := 0; i < m; i++ {
					dot += cmplx.Conj(q.At(i, k)) * v[i]
				}
				r.Set(k, j, r.At(k, j)+dot)
				for i := 0; i < m; i++ {
					v[i] -= dot * q.At(i, k)
				}
			}
		}
		nv := norm2(v)
		if nv == 0 || nv <= 1e-13*scale {
			return nil, ErrRankDeficient
		}
		r.Set(j, j, complex(nv, 0))
		for i := 0; i < m; i++ {
			q.Set(i, j, v[i]/complex(nv, 0))
		}
	}
	return &QR{Q: q, R: r}, nil
}

// Solve returns x minimising ‖a·x − b‖₂ for the factored a.
func (f *QR) Solve(b []complex128) []complex128 {
	m, n := f.Q.Rows, f.Q.Cols
	y := make([]complex128, n)
	for k := 0; k < n; k++ {
		var s complex128
		for i := 0; i < m; i++ {
			s += cmplx.Conj(f.Q.At(i, k)) * b[i]
		}
		y[k] = s
	}
	x := make([]complex128, n)
	for k := n - 1; k >= 0; k-- {
		s := y[k]
		for j := k + 1; j < n; j++ {
			s -= f.R.At(k, j) * x[j]
		}
		x[k] = s / f.R.At(k, k)
	}
	return x
}

func norm2(v []complex128) float64 {
	var s float64
	for _, x := range v {
		s += real(x)*real(x) + imag(x)*imag(x)
	}
	return math.Sqrt(s)
}
```

- [ ] **Step 4: Run the whole linalg package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/linalg/ -count=1`
Expected: PASS (Tasks 3–5). Leave uncommitted.

---

### Task 6: `clone` — package doc, refusals, resynthesis, `Prepare`

**Files:**
- Create: `src/go/internal/audio/clone/doc.go`, `refusal.go`, `synth.go`, `prepare.go`
- Create: `src/go/internal/audio/clone/testsignal_test.go` (shared test generator)
- Test: `src/go/internal/audio/clone/prepare_test.go`

**Interfaces:**
- Produces:
  - `type Mode struct{ Freq, Alpha, Amp, Phase float64; Band int; SNRdB float64 }` with `Energy()`, `T60()`.
  - `func Synthesize(modes []Mode, rate, n int) []float64`.
  - `type Refusal struct{ Code, Reason string }`, codes `RefuseNoOnset`, `RefuseShortDecay`, `RefuseClipped`, `RefuseNotModal`; `func IsRefusal(err error, code string) bool`.
  - `type PitchInfo`, `type PrepareOptions`, `type Prepared`, `func Prepare(samples []float64, rate int, opt PrepareOptions) (Prepared, error)`, `func PrepareFile(path string, opt PrepareOptions) (Prepared, error)`, `func dB(x float64) float64` (clamped at −200).
  - test helper `modalSignal(modes []Mode, rate int, preroll, dur, noiseRMS float64, seed int64) []float64`.

- [ ] **Step 1: Write the shared generator and the failing tests**

`testsignal_test.go`:

```go
package clone

import "math/rand"

// modalSignal is the known-answer generator: preroll seconds of noise, then
// the modes (onset at exactly preroll), plus white noise of noiseRMS
// throughout, seeded.
func modalSignal(modes []Mode, rate int, preroll, dur, noiseRMS float64, seed int64) []float64 {
	pre := int(preroll * float64(rate))
	n := int(dur * float64(rate))
	out := make([]float64, pre+n)
	copy(out[pre:], Synthesize(modes, rate, n))
	if noiseRMS > 0 {
		r := rand.New(rand.NewSource(seed))
		for i := range out {
			out[i] += noiseRMS * r.NormFloat64()
		}
	}
	return out
}

// barModes is a struck-bar-like set: inharmonic, higher modes decay faster.
func barModes() []Mode {
	return []Mode{
		{Freq: 440, Alpha: 2.0, Amp: 0.40, Phase: 0.3},
		{Freq: 1210, Alpha: 4.5, Amp: 0.20, Phase: -1.1},
		{Freq: 2380, Alpha: 9.0, Amp: 0.12, Phase: 2.0},
		{Freq: 3950, Alpha: 16, Amp: 0.06, Phase: 0.7},
	}
}
```

`prepare_test.go`:

```go
package clone

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

func TestPrepareFindsOnsetFloorAndWindow(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2.5, 0.001, 1)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d := math.Abs(float64(p.Onset - 2400)); d > 48 { // ≤ 1 ms
		t.Fatalf("onset %d, want 2400 ± 48", p.Onset)
	}
	if fl := dB(p.FloorRMS); math.Abs(fl-(-60)) > 3 {
		t.Fatalf("floor %.1f dB, want ≈ −60", fl)
	}
	if p.WindowStart <= p.Onset || p.WindowStart-p.Onset > 48*60 {
		t.Fatalf("window start %d not in (onset, onset+60 ms]", p.WindowStart)
	}
	if p.WindowEnd-p.WindowStart < 48000 {
		t.Fatalf("window %d..%d shorter than 1 s for a 2 Np/s fundamental", p.WindowStart, p.WindowEnd)
	}
	if p.Rate != 48000 || len(p.Samples) != len(x) {
		t.Fatalf("rate/len changed: %d %d", p.Rate, len(p.Samples))
	}
}

func TestPrepareOnsetAtSampleZero(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0, 2, 0.001, 2)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Onset > 48 {
		t.Fatalf("onset %d, want ≤ 48 (1 ms) for a signal starting at sample 0", p.Onset)
	}
}

func TestPrepareStillRingingAtClipEnd(t *testing.T) {
	m := []Mode{{Freq: 300, Alpha: 0.5, Amp: 0.5}} // −6.5 dB after 1.5 s
	x := modalSignal(m, 48000, 0.05, 1.5, 0.0005, 3)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatalf("refused a reference still ringing at its end: %v", err)
	}
	if p.WindowEnd != len(x) {
		t.Fatalf("window end %d, want the clip end %d", p.WindowEnd, len(x))
	}
}

func TestPrepareRefusals(t *testing.T) {
	silent := make([]float64, 48000)
	if _, err := Prepare(silent, 48000, PrepareOptions{}); !IsRefusal(err, RefuseNoOnset) {
		t.Fatalf("silence: err = %v, want no_onset", err)
	}
	short := modalSignal([]Mode{{Freq: 800, Alpha: 60, Amp: 0.5}}, 48000, 0.05, 1, 0.001, 4)
	if _, err := Prepare(short, 48000, PrepareOptions{}); !IsRefusal(err, RefuseShortDecay) {
		t.Fatalf("60 Np/s decay: err = %v, want short_decay", err)
	}
	clipped := modalSignal(barModes(), 48000, 0.05, 1, 0.001, 5)
	for i := 2400; i < 2410; i++ {
		clipped[i] = 1
	}
	if _, err := Prepare(clipped, 48000, PrepareOptions{}); !IsRefusal(err, RefuseClipped) {
		t.Fatalf("clipped: err = %v, want clipped", err)
	}
}

func TestPreparePitch(t *testing.T) {
	harm := []Mode{{Freq: 220, Alpha: 1.5, Amp: 0.4}, {Freq: 440, Alpha: 2, Amp: 0.2}, {Freq: 660, Alpha: 3, Amp: 0.1}}
	x := modalSignal(harm, 48000, 0.05, 2, 0.0005, 6)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Pitch.Pitched || p.Pitch.Source != "detected" || math.Abs(p.Pitch.F0Hz-220) > 2 {
		t.Fatalf("pitch %+v, want detected 220 Hz", p.Pitch)
	}
	p, err = Prepare(x, 48000, PrepareOptions{F0Hz: 110})
	if err != nil {
		t.Fatal(err)
	}
	if p.Pitch.F0Hz != 110 || p.Pitch.Source != "flag" {
		t.Fatalf("flag pitch %+v, want 110 from flag", p.Pitch)
	}
}

func TestPrepareFileKeepsRate(t *testing.T) {
	x := modalSignal(barModes(), 44100, 0.05, 1.5, 0.001, 7)
	f32 := make([]float32, len(x))
	for i, v := range x {
		f32[i] = float32(v)
	}
	b, err := wavgen.EncodeWAV(f32, 44100)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bar.wav")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := PrepareFile(path, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Rate != 44100 {
		t.Fatalf("rate %d, want the file's own 44100", p.Rate)
	}
}

func TestSynthesizeMatchesClosedForm(t *testing.T) {
	m := Mode{Freq: 1000, Alpha: 3, Amp: 0.5, Phase: 0.4}
	got := Synthesize([]Mode{m}, 48000, 48000)
	for _, i := range []int{0, 1, 4800, 47999} {
		tt := float64(i) / 48000
		want := m.Amp * math.Exp(-m.Alpha*tt) * math.Cos(2*math.Pi*m.Freq*tt+m.Phase)
		if math.Abs(got[i]-want) > 1e-9 {
			t.Fatalf("sample %d = %v, want %v", i, got[i], want)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -count=1`
Expected: FAIL — package has no non-test files / undefined symbols.

- [ ] **Step 3: Implement `doc.go`, `refusal.go`, `synth.go`**

`doc.go`:

```go
// Package clone turns a recording of a struck / modal instrument into a
// MEASURED model — damped modes, a strike, a decay law, residual noise bands
// — that later stages invert into an instrument document of synth
// parameters (spec docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md).
// Nothing here copies audio into a document: the output is model numbers.
//
// Stages are pure functions, each testable on generated audio:
// Prepare → EstimateModes → EstimateStrike / FitDecayLaw / MeasureResidual →
// Analyze. Analysis runs at the file's own sample rate; time is seconds from
// the detected onset.
//
// Dev-only: nothing reachable from a shipped binary imports this package
// (TestCloneIsDevOnly).
package clone

import "math"

// Mode is one damped sinusoid of the modal model, t in seconds from the onset:
//
//	x(t) = Amp · e^(−Alpha·t) · cos(2π·Freq·t + Phase)
type Mode struct {
	Freq  float64 `json:"freq_hz"`
	Alpha float64 `json:"alpha_np_per_s"`
	Amp   float64 `json:"amp"`
	Phase float64 `json:"phase_rad"`
	Band  int     `json:"band"`
	SNRdB float64 `json:"snr_db"`
}

// Energy is the mode's total energy ∫x² ≈ Amp²/(4·Alpha) — the ranking key.
func (m Mode) Energy() float64 { return m.Amp * m.Amp / (4 * m.Alpha) }

// T60 is the time for the mode to fall 60 dB.
func (m Mode) T60() float64 { return 3 * math.Ln10 / m.Alpha }

// dB converts a linear amplitude to dB, clamped at −200 so JSON never sees
// −Inf.
func dB(x float64) float64 {
	if x <= 1e-10 {
		return -200
	}
	return 20 * math.Log10(x)
}
```

`refusal.go`:

```go
package clone

import (
	"errors"
	"fmt"
)

// Refusal codes: the analysis declines rather than guess (spec §7).
const (
	RefuseNoOnset    = "no_onset"
	RefuseShortDecay = "short_decay"
	RefuseClipped    = "clipped"
	RefuseNotModal   = "not_modal"
)

// Refusal is a typed "this input cannot be cloned" error.
type Refusal struct {
	Code   string
	Reason string
}

func (r *Refusal) Error() string { return fmt.Sprintf("clone refused (%s): %s", r.Code, r.Reason) }

func refuse(code, format string, args ...any) error {
	return &Refusal{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// IsRefusal reports whether err is a Refusal with the given code.
func IsRefusal(err error, code string) bool {
	var r *Refusal
	return errors.As(err, &r) && r.Code == code
}
```

`synth.go`:

```go
package clone

import (
	"math"
	"math/cmplx"
)

// Synthesize renders modes from t = 0 for n samples at rate: the pure-Go
// reference resynthesis the residual and the tests use (the engine's C modal
// bank is plan 2). One phasor recursion per mode, stopped once the mode is
// 160 dB below its start.
func Synthesize(modes []Mode, rate, n int) []float64 {
	out := make([]float64, n)
	for _, m := range modes {
		if m.Amp == 0 {
			continue
		}
		z := cmplx.Exp(complex(-m.Alpha/float64(rate), 2*math.Pi*m.Freq/float64(rate)))
		s := cmplx.Rect(m.Amp, m.Phase)
		stop := m.Amp * 1e-8
		for i := 0; i < n; i++ {
			out[i] += real(s)
			s *= z
			if i&1023 == 1023 && cmplx.Abs(s) < stop {
				break
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Implement `prepare.go`**

```go
package clone

import (
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// PitchInfo is the nominal pitch the mode ratios are expressed against.
type PitchInfo struct {
	Pitched     bool    `json:"pitched"`
	F0Hz        float64 `json:"f0_hz"`
	Source      string  `json:"source"` // "flag" | "detected" | "none"
	Confidence  float64 `json:"confidence"`
	SemisFromA3 int     `json:"semis_from_a3"`
	HasSemis    bool    `json:"has_semis"`
}

// PrepareOptions carries what the caller knows about the reference.
type PrepareOptions struct {
	F0Hz        float64 // explicit nominal pitch (-f0); wins over detection
	SemisFromA3 int     // the catalogue's measured note, recorded for rendering
	HasSemis    bool
	Label       string
}

// Prepared is a reference ready for analysis.
type Prepared struct {
	Label       string
	Samples     []float64 // mono, the whole file, its own rate
	Rate        int
	Onset       int     // sample index
	FloorRMS    float64 // linear
	Peak        float64
	RMS400DB    float64 // RMS of the first 400 ms after the onset, dBFS
	WindowStart int     // decay-analysis window [WindowStart, WindowEnd), sample indices
	WindowEnd   int
	Pitch       PitchInfo
}

const (
	minDecayWindowSec = 0.150
	clipLevel         = 0.999
	clipMinSamples    = 3
)

// PrepareFile decodes a WAV (any channel count, folded to mono) and prepares it.
func PrepareFile(path string, opt PrepareOptions) (Prepared, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Prepared{}, err
	}
	pcm, rate, err := wavgen.DecodeWAV(b)
	if err != nil {
		return Prepared{}, fmt.Errorf("clone: decode %s: %w", path, err)
	}
	x := make([]float64, len(pcm))
	for i, v := range pcm {
		x[i] = float64(v)
	}
	if opt.Label == "" {
		opt.Label = path
	}
	return Prepare(x, rate, opt)
}

// Prepare measures the onset, noise floor, level and decay window of a mono
// recording and decides its nominal pitch (spec §2a).
func Prepare(samples []float64, rate int, opt PrepareOptions) (Prepared, error) {
	p := Prepared{Label: opt.Label, Samples: samples, Rate: rate}
	clipped := 0
	for _, v := range samples {
		a := math.Abs(v)
		if a >= clipLevel {
			clipped++
		}
		if a > p.Peak {
			p.Peak = a
		}
	}
	if clipped >= clipMinSamples {
		return p, refuse(RefuseClipped, "%d samples at or above %.3f full scale", clipped, clipLevel)
	}
	fluxFrame := fluxPeakFrame(samples, rate)
	p.FloorRMS = noiseFloor(samples, rate, fluxFrame*onsetHop-onsetFrame)
	if p.Peak == 0 || p.Peak <= 10*p.FloorRMS {
		return p, refuse(RefuseNoOnset, "nothing rises 20 dB above the noise floor")
	}
	// floor + 20 dB and at least −40 dB re the peak (a struck attack crosses
	// that within ~1 ms; a lower bar lets pre-roll noise fire it — VCSL's
	// cowbell has a digitally silent tail, floor 0, and the onset landed on
	// the first noise sample 10 ms early, inflating every fast mode's
	// back-extrapolated amplitude), but never above −20 dB re the peak.
	thr := math.Min(math.Max(10*p.FloorRMS, 0.01*p.Peak), 0.1*p.Peak)
	onset, ok := refineOnset(samples, fluxFrame, thr)
	if !ok {
		return p, refuse(RefuseNoOnset, "no sample above floor+20 dB near the spectral-flux peak")
	}
	p.Onset = onset
	p.RMS400DB = dB(rms(samples[onset:min(len(samples), onset+int(0.4*float64(rate)))]))

	// Window start: past the strike transient — 5 ms after the 2 ms-RMS
	// envelope first comes within 3 dB of its maximum over the first 50 ms.
	// (Not after the maximum itself: a vibraphone's beating keeps creeping
	// up for 70 ms and pushed the window to 49 ms, where its fast high modes
	// had already fallen 50 dB.)
	f2 := int(0.002 * float64(rate))
	var env []float64
	for k := 0; k < 25; k++ {
		a, b := onset+k*f2, onset+(k+1)*f2
		if b > len(samples) {
			break
		}
		env = append(env, rms(samples[a:b]))
	}
	best := 0.0
	for _, v := range env {
		best = math.Max(best, v)
	}
	settle := 0
	for k, v := range env {
		if v >= best/math.Sqrt2 {
			settle = k
			break
		}
	}
	p.WindowStart = onset + (settle+1)*f2 + int(0.005*float64(rate))
	// Window end: the last 10 ms frame still 6 dB above the floor (or the
	// clip end when it still rings — VCSL clips clamp mid-ring).
	f10 := int(0.010 * float64(rate))
	p.WindowEnd = p.WindowStart
	for a := p.WindowStart; a+f10 <= len(samples); a += f10 {
		if rms(samples[a:a+f10]) > 2*p.FloorRMS {
			p.WindowEnd = a + f10
		}
	}
	if len(samples)-p.WindowEnd < f10 && p.WindowEnd > p.WindowStart {
		p.WindowEnd = len(samples)
	}
	if float64(p.WindowEnd-p.WindowStart)/float64(rate) < minDecayWindowSec {
		return p, refuse(RefuseShortDecay, "decay window %.0f ms < %.0f ms",
			1000*float64(p.WindowEnd-p.WindowStart)/float64(rate), 1000*minDecayWindowSec)
	}
	p.Pitch = decidePitch(samples[onset:p.WindowEnd], rate, opt)
	return p, nil
}

func decidePitch(seg []float64, rate int, opt PrepareOptions) PitchInfo {
	pi := PitchInfo{SemisFromA3: opt.SemisFromA3, HasSemis: opt.HasSemis, Source: "none"}
	if opt.F0Hz > 0 {
		pi.Pitched, pi.F0Hz, pi.Source, pi.Confidence = true, opt.F0Hz, "flag", 1
		return pi
	}
	w := wave.Wave{Samples: seg, SampleRate: rate}
	est := fingerprint.DetectF0Detailed(w)
	pi.Confidence = est.Confidence
	if ok, _ := fingerprint.UsableAsPitch(w, est); ok {
		pi.Pitched, pi.F0Hz, pi.Source = true, est.Hz, "detected"
	}
	return pi
}

// noiseFloor is the recording's noise: the median 10 ms-frame RMS of the
// pre-roll before preEnd when there are ≥ 20 ms of it (the only stretch that
// is certainly noise), else the quietest NON-ZERO 2 ms frame. The fallback is
// biased low on purpose: a floor read too high (the quietest 10 % of a clip
// that rings to its end is the ring — VCSL's vibraphone decays 6 dB in 1.5 s
// behind an 8 ms pre-roll) refuses the clip and cuts its window short, while
// one read too low only lengthens the window. Digital zeros (trimmed or
// padded audio) are never the noise.
func noiseFloor(x []float64, rate, preEnd int) float64 {
	if f := int(0.010 * float64(rate)); preEnd >= 2*f {
		var pre []float64
		for a := 0; a+f <= preEnd; a += f {
			if r := rms(x[a : a+f]); r > 0 {
				pre = append(pre, r)
			}
		}
		if len(pre) >= 2 {
			sort.Float64s(pre)
			return pre[len(pre)/2]
		}
	}
	f := max(1, int(0.002*float64(rate)))
	floor := 0.0
	for a := 0; a+f <= len(x); a += f {
		if r := rms(x[a : a+f]); r > 0 && (floor == 0 || r < floor) {
			floor = r
		}
	}
	if sf := spectralFloor(x, rate); sf > 0 && (floor == 0 || sf < floor) {
		floor = sf
	}
	return floor
}

// spectralFloor estimates white-noise RMS from the last 4096 samples without
// needing a quiet stretch: noise is broadband and a ring is a handful of
// tones, so the MEDIAN Hann-windowed bin power is noise even mid-ring. For
// white noise of variance σ², |X_k|² is exponential with mean σ²·Σw², whose
// median is ln 2 times the mean. VCSL's tubular bells ring to the clip end
// with no pre-roll; the quietest 2 ms frame there is a trough of the ring
// (−34 dB against a −3.5 dB peak), which cut the window and skipped bands.
func spectralFloor(x []float64, rate int) float64 {
	const n = 4096
	if len(x) < n {
		return 0
	}
	mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: x[len(x)-n:], SampleRate: rate}, n, wave.WindowHann)
	pw := make([]float64, 0, len(mag))
	for _, m := range mag[1 : len(mag)-1] {
		pw = append(pw, m*m)
	}
	sort.Float64s(pw)
	med := pw[len(pw)/2]
	const sumW2 = 0.375 * n // Σ Hann² ≈ 3N/8
	return math.Sqrt(med / (math.Ln2 * sumW2))
}

const onsetFrame, onsetHop = 512, 128

// fluxPeakFrame is the frame (512-sample Hann, hop 128) of maximum spectral
// flux — independent of any level threshold.
func fluxPeakFrame(x []float64, rate int) int {
	var prev []float64
	bestFlux, bestFrame := -1.0, 0
	for i := 0; i*onsetHop+onsetFrame <= len(x); i++ {
		seg := x[i*onsetHop : i*onsetHop+onsetFrame]
		mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: rate}, onsetFrame, wave.WindowHann)
		var flux float64
		for k, m := range mag {
			d := m
			if prev != nil {
				d = m - prev[k]
			}
			if d > 0 {
				flux += d
			}
		}
		if flux > bestFlux {
			bestFlux, bestFrame = flux, i
		}
		prev = mag
	}
	return bestFrame
}

// refineOnset is the first sample above thr from four hops before the flux
// peak frame to its end.
func refineOnset(x []float64, frame int, thr float64) (int, bool) {
	lo := max(0, (frame-4)*onsetHop)
	hi := min(len(x), frame*onsetHop+onsetFrame)
	for i := lo; i < hi; i++ {
		if math.Abs(x[i]) > thr {
			return i, true
		}
	}
	return 0, false
}

func rms(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -count=1`
Expected: PASS. If `TestPreparePitch` reports `UsableAsPitch` false for the clean 220 Hz harmonic tone, print `fingerprint.AnalyzeF0Support` for it and fix the TEST signal (a longer clip), never the gate. Leave uncommitted.

---

### Task 7: `clone` — sub-band heterodyne and single-band ESPRIT

**Files:**
- Create: `src/go/internal/audio/clone/subband.go`, `esprit.go`
- Test: `src/go/internal/audio/clone/esprit_test.go`

**Interfaces:**
- Consumes: `linalg.EigHermitian`, `linalg.Eigenvalues`, `linalg.FactorQR`.
- Produces: `type band struct{ Index int; Lo, Hi, Center float64; Decim int; Taps []float64 }`, `bandLayout(rate int) []band`, `baseband(x []float64, rate, onset, from, to int, b band) (y []complex128, first int)`, `(b band) gain(s complex128, rate int) complex128`, `type espritResult struct{ Poles, Amps []complex128; NoiseVar float64; Order int }`, `esprit(y []complex128, maxOrder int) (espritResult, error)`.

- [ ] **Step 1: Write the failing test**

```go
package clone

import (
	"math"
	"math/cmplx"
	"math/rand"
	"sort"
	"testing"
)

func TestESPRITRecoversDampedExponentials(t *testing.T) {
	poles := []complex128{cmplx.Rect(0.995, 0.4), cmplx.Rect(0.99, -1.3), cmplx.Rect(0.985, 2.1)}
	amps := []complex128{1, 0.5i, -0.3}
	n := 400
	r := rand.New(rand.NewSource(1))
	y := make([]complex128, n)
	for m := 0; m < n; m++ {
		for k := range poles {
			y[m] += amps[k] * cmplx.Pow(poles[k], complex(float64(m), 0))
		}
		y[m] += complex(1e-3*r.NormFloat64(), 1e-3*r.NormFloat64())
	}
	res, err := esprit(y, 24)
	if err != nil {
		t.Fatal(err)
	}
	if res.Order != 3 {
		t.Fatalf("order %d, want 3", res.Order)
	}
	sort.Slice(res.Poles, func(i, j int) bool { return cmplx.Phase(res.Poles[i]) < cmplx.Phase(res.Poles[j]) })
	want := []complex128{poles[1], poles[0], poles[2]}
	for i := range want {
		if cmplx.Abs(res.Poles[i]-want[i]) > 1e-4 {
			t.Fatalf("pole %d = %v, want %v", i, res.Poles[i], want[i])
		}
	}
}

func TestBandLayoutCoversTheSpectrumOnce(t *testing.T) {
	for _, rate := range []int{44100, 48000} {
		bs := bandLayout(rate)
		if len(bs) != 24 || bs[0].Lo != 30 {
			t.Fatalf("rate %d: %d bands from %.1f", rate, len(bs), bs[0].Lo)
		}
		for i := 1; i < len(bs); i++ {
			if math.Abs(bs[i].Lo-bs[i-1].Hi) > 1e-9 {
				t.Fatalf("rate %d: gap between band %d and %d", rate, i-1, i)
			}
		}
		if top := bs[len(bs)-1].Hi; math.Abs(top-math.Min(20000, 0.45*float64(rate))) > 1e-6 {
			t.Fatalf("rate %d: top edge %.1f", rate, top)
		}
		for _, b := range bs {
			if float64(rate)/float64(b.Decim) < 2*(b.Hi-b.Lo) {
				t.Fatalf("rate %d band %d: decimated rate below twice the band width", rate, b.Index)
			}
		}
	}
}

func TestBasebandPreservesAModeAfterGainCorrection(t *testing.T) {
	rate := 48000
	m := Mode{Freq: 1000, Alpha: 3, Amp: 0.5, Phase: 0.9}
	x := Synthesize([]Mode{m}, rate, rate*2)
	var b band
	for _, c := range bandLayout(rate) {
		if m.Freq >= c.Lo && m.Freq < c.Hi {
			b = c
		}
	}
	from, to := 2400, len(x)
	y, first := baseband(x, rate, 0, from, to, b)
	if len(y) < 50 {
		t.Fatalf("only %d decimated samples", len(y))
	}
	s := complex(-m.Alpha, 2*math.Pi*(m.Freq-b.Center))
	tt := float64(first) / float64(rate)
	want := cmplx.Rect(m.Amp/2, m.Phase) * cmplx.Exp(s*complex(tt, 0)) * b.gain(s, rate)
	if cmplx.Abs(y[0]-want) > 1e-6 {
		t.Fatalf("y[0] = %v, want %v", y[0], want)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run 'ESPRIT|BandLayout|Baseband' -count=1`
Expected: FAIL — undefined `esprit`, `bandLayout`, `baseband`.

- [ ] **Step 3: Implement `subband.go`**

```go
package clone

import (
	"math"
	"math/cmplx"
)

// Sub-band layout (spec §2b): 24 log-spaced bands from 30 Hz to
// min(20 kHz, 0.45·rate). A mode belongs to the band whose CORE [Lo, Hi)
// holds its frequency; each band heterodynes its core centre to DC,
// low-passes and decimates so ESPRIT sees few poles.
//
// Decimated rate fs_d ≥ 2·w (w = core width). The low-pass passes ±0.5w flat
// and stops from ±1.5w (Blackman, cutoff w): a component at |f| ≥ 1.5w is the
// only thing that could fold into the core ±0.5w, and it is ≥ 74 dB down.
type band struct {
	Index  int
	Lo, Hi float64 // core edges, Hz
	Center float64 // heterodyne frequency, Hz
	Decim  int
	Taps   []float64 // zero-phase low-pass, odd length, centred, unit DC gain
}

const (
	bandCount = 24
	bandLoHz  = 30.0
	bandHiHz  = 20000.0
)

func bandLayout(rate int) []band {
	top := math.Min(bandHiHz, 0.45*float64(rate))
	ratio := math.Pow(top/bandLoHz, 1/float64(bandCount))
	out := make([]band, 0, bandCount)
	lo := bandLoHz
	for k := 0; k < bandCount; k++ {
		hi := bandLoHz * math.Pow(ratio, float64(k+1))
		if k == bandCount-1 {
			hi = top
		}
		w := hi - lo
		d := int(float64(rate) / (2 * w))
		if d < 1 {
			d = 1
		}
		out = append(out, band{Index: k, Lo: lo, Hi: hi, Center: (lo + hi) / 2, Decim: d, Taps: lowpassTaps(rate, w)})
		lo = hi
	}
	return out
}

// lowpassTaps is a Blackman-windowed sinc with cutoff w Hz and transition
// width w (0.5w → 1.5w), normalised to unit DC gain.
func lowpassTaps(rate int, w float64) []float64 {
	n := int(math.Ceil(5.5 * float64(rate) / w))
	if n%2 == 0 {
		n++
	}
	if n < 3 {
		n = 3
	}
	fc := w / float64(rate)
	half := n / 2
	h := make([]float64, n)
	var sum float64
	for j := 0; j < n; j++ {
		k := float64(j - half)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		ph := 2 * math.Pi * float64(j) / float64(n-1)
		h[j] = s * (0.42 - 0.5*math.Cos(ph) + 0.08*math.Cos(2*ph))
		sum += h[j]
	}
	for j := range h {
		h[j] /= sum
	}
	return h
}

// baseband heterodynes x by e^{−i2π·Center·(n−onset)/rate}, low-passes and
// decimates it, using only samples in [from, to) (each output's whole filter
// support lies inside). first is the absolute sample index of y[0]; y[m] is
// centred on first + m·Decim.
func baseband(x []float64, rate, onset, from, to int, b band) ([]complex128, int) {
	half := len(b.Taps) / 2
	first := from + half
	if first+half >= to {
		return nil, 0
	}
	re := make([]float64, to-from)
	im := make([]float64, to-from)
	// e^{−iw(n−onset)} by phasor recursion, re-anchored exactly every 1024
	// samples (drift stays ~1e-13; math.Sincos per sample was a third of the
	// whole analysis).
	w := 2 * math.Pi * b.Center / float64(rate)
	sw, cw := math.Sincos(-w)
	var pr, pi float64
	for n := from; n < to; n++ {
		if (n-from)&1023 == 0 {
			s, c := math.Sincos(-w * float64(n-onset))
			pr, pi = c, s
		}
		re[n-from] = x[n] * pr
		im[n-from] = x[n] * pi
		pr, pi = pr*cw-pi*sw, pr*sw+pi*cw
	}
	var y []complex128
	for c := first; c+half < to; c += b.Decim {
		base := c - half - from
		var ar, ai float64
		for j, h := range b.Taps {
			ar += h * re[base+j]
			ai += h * im[base+j]
		}
		y = append(y, complex(ar, ai))
	}
	return y, first
}

// gain is the band filter's exact response to a baseband component
// e^{s·t} (s = −α + i·2π·Δf, continuous time): Σ h_j e^{s·τ_j}. Dividing a
// measured amplitude by it removes both the pass-band shape and the bias a
// long symmetric filter puts on a DAMPED exponential.
func (b band) gain(s complex128, rate int) complex128 {
	half := len(b.Taps) / 2
	var g complex128
	for j, h := range b.Taps {
		tau := float64(j-half) / float64(rate)
		g += complex(h, 0) * cmplx.Exp(s*complex(tau, 0))
	}
	return g
}```

- [ ] **Step 4: Implement `esprit.go`**

```go
package clone

import (
	"errors"
	"math"
	"math/cmplx"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone/linalg"
)

var errTooShort = errors.New("clone: too few samples for ESPRIT")

// espritResult is one band's pole estimate. Poles are per decimated sample;
// Amps are the complex amplitudes at y[0].
type espritResult struct {
	Poles    []complex128
	Amps     []complex128
	NoiseVar float64 // noise eigenvalue median (per-sample power of y)
	Order    int
	Signal   int // eigenvalues ≥ 6 dB above the noise median, before ESTER
}

const (
	espritMaxRows      = 128 // Hankel rows L
	espritMaxSnapshots = 384 // covariance snapshots (strided)
	espritMaxLSRows    = 4096
	ompMinGain         = 20 // a kept pole must explain ≥ 20σ² (13 dB over chance)
)

// esprit estimates the damped exponentials in y (spec §2b): L × L sample
// covariance → Hermitian eigendecomposition → candidate order = eigenvalues
// ≥ 6 dB above the noise median (≤ maxOrder) → candidate poles = eig(Φ) →
// orthogonal matching pursuit keeps the ones that explain y → least squares
// for their amplitudes.
func esprit(y []complex128, maxOrder int) (espritResult, error) {
	n := len(y)
	L := min(n/3, espritMaxRows)
	if L < 4 {
		return espritResult{}, errTooShort
	}
	K := n - L + 1
	stride := (K + espritMaxSnapshots - 1) / espritMaxSnapshots
	R := linalg.New(L, L)
	count := 0
	for k := 0; k < K; k += stride {
		count++
		for i := 0; i < L; i++ {
			yi := y[k+i]
			for j := i; j < L; j++ {
				R.Data[i*L+j] += yi * cmplx.Conj(y[k+j])
			}
		}
	}
	inv := complex(1/float64(count), 0)
	for i := 0; i < L; i++ {
		for j := i; j < L; j++ {
			v := R.Data[i*L+j] * inv
			R.Data[i*L+j] = v
			R.Data[j*L+i] = cmplx.Conj(v)
		}
	}
	vals, vecs := linalg.EigHermitian(R)
	lower := append([]float64(nil), vals[L/2:]...)
	sort.Float64s(lower)
	noise := math.Max(lower[len(lower)/2], 0)
	res := espritResult{NoiseVar: noise}
	pmax := min(maxOrder, L/2)
	p := 0
	for p < pmax && vals[p] > 4*noise {
		p++
	}
	res.Signal = p
	if p == 0 {
		return res, nil
	}
	phi, err := rotation(vecs, L, p)
	if err != nil {
		return res, err
	}
	poles, err := linalg.Eigenvalues(phi)
	if err != nil {
		return res, err
	}
	poles, amps, err := selectPoles(y, dedupePoles(poles), noise)
	if err != nil {
		return res, err
	}
	res.Poles, res.Amps, res.Order = poles, amps, len(poles)
	return res, nil
}

// selectPoles keeps the candidate poles that actually explain y: orthogonal
// matching pursuit over strided rows — repeatedly add the candidate whose
// component orthogonal to those already chosen removes the most residual
// energy, while that removal is ≥ ompMinGain·σ² (a column fits ~σ² of white
// noise by chance). Then one least-squares fit over the chosen poles.
//
// Order selection on recorded audio, measured on the VCSL corpus: the
// noise-threshold count saturates (a real recording has no clean eigenvalue
// floor), ESTER at a fixed bar (‖W↑ − W↓Φ‖_F ≤ 0.3) trimmed real tubular-bell
// partials away (E(1) is already 0.3–0.6 in long high bands full of strike
// noise), and the untrimmed order handed the joint refit clusters of
// cancelling poles that blew up outside the window. A pursuit never keeps a
// cancelling phantom: on its own it explains nothing.
func selectPoles(y, cand []complex128, noiseVar float64) ([]complex128, []complex128, error) {
	n := len(y)
	stride := (n + espritMaxLSRows - 1) / espritMaxLSRows
	rows := (n + stride - 1) / stride
	b := make([]complex128, rows)
	for r := range b {
		b[r] = y[r*stride]
	}
	cols := make([][]complex128, len(cand))
	for k, z := range cand {
		col := make([]complex128, rows)
		zs := cmplx.Pow(z, complex(float64(stride), 0))
		acc := complex(1, 0)
		for r := range col {
			col[r] = acc
			acc *= zs
		}
		cols[k] = col
	}
	res := append([]complex128(nil), b...)
	// Each candidate's component orthogonal to the chosen basis, updated
	// incrementally as the basis grows (O(candidates·rows) per step).
	perp := make([][]complex128, len(cols))
	norm0 := make([]float64, len(cols))
	for k, col := range cols {
		perp[k] = append([]complex128(nil), col...)
		for _, v := range col {
			norm0[k] += cmplxAbs2(v)
		}
	}
	used := make([]bool, len(cand))
	var chosen []complex128
	thr := ompMinGain * math.Max(noiseVar, 1e-300)
	for len(chosen) < len(cand) {
		best, bestGain, bestNorm := -1, 0.0, 0.0
		for k, v := range perp {
			if used[k] {
				continue
			}
			var nv float64
			var dot complex128
			for r := range v {
				nv += cmplxAbs2(v[r])
				dot += cmplx.Conj(v[r]) * res[r]
			}
			if nv <= 1e-10*norm0[k] {
				continue
			}
			if g := cmplxAbs2(dot) / nv; g > bestGain {
				best, bestGain, bestNorm = k, g, nv
			}
		}
		if best < 0 || bestGain < thr {
			break
		}
		inv := complex(1/math.Sqrt(bestNorm), 0)
		q := make([]complex128, rows)
		for r := range q {
			q[r] = perp[best][r] * inv
		}
		project := func(v []complex128) {
			var dot complex128
			for r := range q {
				dot += cmplx.Conj(q[r]) * v[r]
			}
			for r := range q {
				v[r] -= dot * q[r]
			}
		}
		project(res)
		used[best] = true
		chosen = append(chosen, cand[best])
		for k := range perp {
			if !used[k] {
				project(perp[k])
			}
		}
	}
	if len(chosen) == 0 {
		return nil, nil, nil
	}
	amps, err := vandermonde(y, chosen)
	if err != nil {
		return nil, nil, err
	}
	return chosen, amps, nil
}

// rotation solves W↓·Φ = W↑ in the least-squares sense for the first p
// eigenvectors (W↓ = rows 0..L−2, W↑ = rows 1..L−1).
func rotation(vecs *linalg.Matrix, L, p int) (*linalg.Matrix, error) {
	down := linalg.New(L-1, p)
	for i := 0; i < L-1; i++ {
		for j := 0; j < p; j++ {
			down.Set(i, j, vecs.At(i, j))
		}
	}
	f, err := linalg.FactorQR(down)
	if err != nil {
		return nil, err
	}
	phi := linalg.New(p, p)
	col := make([]complex128, L-1)
	for j := 0; j < p; j++ {
		for i := 0; i < L-1; i++ {
			col[i] = vecs.At(i+1, j)
		}
		x := f.Solve(col)
		for i := 0; i < p; i++ {
			phi.Set(i, j, x[i])
		}
	}
	return phi, nil
}

// dedupePoles drops a pole numerically identical to an earlier one (it would
// make the Vandermonde matrix singular).
func dedupePoles(p []complex128) []complex128 {
	var out []complex128
	for _, z := range p {
		dup := false
		for _, o := range out {
			if cmplx.Abs(z-o) < 1e-12 {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, z)
		}
	}
	return out
}

// vandermonde fits y[m] ≈ Σ_k c_k z_k^m by least squares over at most
// espritMaxLSRows uniformly strided samples.
func vandermonde(y []complex128, poles []complex128) ([]complex128, error) {
	n := len(y)
	stride := (n + espritMaxLSRows - 1) / espritMaxLSRows
	rows := (n + stride - 1) / stride
	v := linalg.New(rows, len(poles))
	b := make([]complex128, rows)
	for k, z := range poles {
		zs := cmplx.Pow(z, complex(float64(stride), 0))
		acc := complex(1, 0)
		for r := 0; r < rows; r++ {
			v.Set(r, k, acc)
			acc *= zs
		}
	}
	for r := 0; r < rows; r++ {
		b[r] = y[r*stride]
	}
	f, err := linalg.FactorQR(v)
	if err != nil {
		return nil, err
	}
	return f.Solve(b), nil
}```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -count=1`
Expected: PASS. If the order test gives 4–5 instead of 3, inspect `vals` vs `noise` before changing thresholds; spurious poles are allowed to survive ESPRIT (hygiene removes them in Task 8) — the test may then assert "the three true poles are among the result" instead, which is the contract Task 8 relies on. Leave uncommitted.

---

### Task 8: `clone.EstimateModes` — all bands, pole hygiene, known answers

**Files:**
- Create: `src/go/internal/audio/clone/modes.go`
- Test: `src/go/internal/audio/clone/modes_test.go`

**Interfaces:**
- Consumes: `bandLayout`, `baseband`, `band.gain`, `esprit`, `Prepared`.
- Produces: `type BandDiag struct{ Index int; LoHz, HiHz float64; Decim, Samples, Order int; NoiseDB float64; Skipped string }`, `type ModesResult struct{ Modes []Mode; Bands []BandDiag }`, `func EstimateModes(p Prepared) (ModesResult, error)`. Modes sorted by descending `Energy()`.

- [ ] **Step 1: Write the failing tests**

```go
package clone

import (
	"math"
	"sort"
	"testing"
)

func nearest(modes []Mode, f float64) (Mode, bool) {
	best, ok := Mode{}, false
	for _, m := range modes {
		if !ok || math.Abs(m.Freq-f) < math.Abs(best.Freq-f) {
			best, ok = m, true
		}
	}
	return best, ok
}

func cents(a, b float64) float64 { return 1200 * math.Log2(a/b) }

func checkRecovered(t *testing.T, got []Mode, want []Mode) {
	t.Helper()
	for _, w := range want {
		m, ok := nearest(got, w.Freq)
		if !ok {
			t.Fatalf("no modes at all")
		}
		if c := math.Abs(cents(m.Freq, w.Freq)); c > 0.5 {
			t.Errorf("%.1f Hz: got %.4f Hz (%.2f cents)", w.Freq, m.Freq, c)
		}
		if r := math.Abs(m.Alpha/w.Alpha - 1); r > 0.05 {
			t.Errorf("%.1f Hz: alpha %.3f, want %.3f (%.1f%%)", w.Freq, m.Alpha, w.Alpha, 100*r)
		}
		if d := math.Abs(dB(m.Amp) - dB(w.Amp)); d > 0.5 {
			t.Errorf("%.1f Hz: amp %.2f dB, want %.2f dB", w.Freq, dB(m.Amp), dB(w.Amp))
		}
	}
}

// 40 dB SNR: noise RMS 1% of the strongest mode's amplitude.
func TestEstimateModesKnownAnswer(t *testing.T) {
	want := barModes()
	x := modalSignal(want, 48000, 0.05, 2.5, 0.004, 11)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Modes {
		t.Logf("mode %9.4f Hz  α %7.4f  amp %.5f  φ %+.3f  band %2d  SNR %.0f dB", m.Freq, m.Alpha, m.Amp, m.Phase, m.Band, m.SNRdB)
	}
	checkRecovered(t, res.Modes, want)
	if len(res.Modes) > len(want)+2 {
		t.Errorf("%d modes for %d true ones: %+v", len(res.Modes), len(want), res.Modes)
	}
	if !sort.SliceIsSorted(res.Modes, func(i, j int) bool { return res.Modes[i].Energy() > res.Modes[j].Energy() }) {
		t.Errorf("modes not sorted by energy")
	}
}

func TestEstimateModesResolvesADoublet(t *testing.T) {
	want := []Mode{{Freq: 440, Alpha: 1.0, Amp: 0.3}, {Freq: 441, Alpha: 1.3, Amp: 0.25, Phase: 1}}
	x := modalSignal(want, 48000, 0.05, 2, 0.003, 12)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	var near []Mode
	for _, m := range res.Modes {
		if m.Freq > 438 && m.Freq < 443 {
			near = append(near, m)
		}
	}
	if len(near) != 2 {
		t.Fatalf("doublet 440/441 Hz resolved into %d modes: %+v", len(near), near)
	}
	sort.Slice(near, func(i, j int) bool { return near[i].Freq < near[j].Freq })
	for i, w := range want {
		if math.Abs(near[i].Freq-w.Freq) > 0.1 {
			t.Errorf("doublet member %d at %.3f Hz, want %.1f", i, near[i].Freq, w.Freq)
		}
	}
}

func TestEstimateModesAt44100(t *testing.T) {
	want := barModes()
	x := modalSignal(want, 44100, 0.05, 2.5, 0.004, 13)
	p, err := Prepare(x, 44100, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	checkRecovered(t, res.Modes, want)
}

func TestEstimateModesBandEdgeCountedOnce(t *testing.T) {
	edge := bandLayout(48000)[12].Lo
	want := []Mode{{Freq: edge * 1.0005, Alpha: 3, Amp: 0.3}, {Freq: 300, Alpha: 2, Amp: 0.3}}
	x := modalSignal(want, 48000, 0.05, 2, 0.003, 14)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range res.Modes {
		if math.Abs(cents(m.Freq, want[0].Freq)) < 20 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("edge mode at %.2f Hz counted %d times: %+v", want[0].Freq, n, res.Modes)
	}
	checkRecovered(t, res.Modes, want)
}```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run EstimateModes -count=1`
Expected: FAIL — `undefined: EstimateModes`.

- [ ] **Step 3: Implement `modes.go`**

```go
package clone

import (
	"math"
	"math/cmplx"
	"math/rand"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone/linalg"
)

// BandDiag reports what one analysis band did (for the report).
type BandDiag struct {
	Index   int     `json:"index"`
	LoHz    float64 `json:"lo_hz"`
	HiHz    float64 `json:"hi_hz"`
	Decim   int     `json:"decim"`
	Samples int     `json:"samples"`
	Order   int     `json:"order"`
	Signal  int     `json:"signal_eigs"` // order before the ESTER trim
	NoiseDB float64 `json:"noise_db"`
	Skipped string  `json:"skipped,omitempty"`
	// Poles ESPRIT returned but hygiene rejected, by reason.
	RejGrowing int `json:"rej_growing,omitempty"`
	RejOutside int `json:"rej_outside_core,omitempty"`
	RejLowSNR  int `json:"rej_low_snr,omitempty"`
}

// ModesResult is EstimateModes' output.
type ModesResult struct {
	Modes []Mode     `json:"modes"`
	Bands []BandDiag `json:"bands"`
}

const (
	maxOrderPerBand = 24
	minBandSamples  = 12
	modeRelFloorDB  = -60 // drop modes this far below the strongest
	// maxExtrapolationNp: 40 dB = ln(100) Np of decay between the onset and
	// the window start.
	maxExtrapolationNp = 4.605170185988092
)

// EstimateModes runs sub-band ESPRIT over p's decay window (spec §2b) and
// returns the modes with their amplitudes back-extrapolated to the onset,
// sorted by descending energy. Pole hygiene: α > 0, frequency inside the
// band's CORE (so overlapping bands never double-count), SNR at the window
// start ≥ 0 dB, amplitude within 60 dB of the strongest mode.
func EstimateModes(p Prepared) (ModesResult, error) {
	var res ModesResult
	for _, b := range bandLayout(p.Rate) {
		d := BandDiag{Index: b.Index, LoHz: b.Lo, HiHz: b.Hi, Decim: b.Decim, NoiseDB: -200}
		y, first := baseband(p.Samples, p.Rate, p.Onset, p.WindowStart, p.WindowEnd, b)
		y = trimToBandSignal(y, p.FloorRMS, b, p.Rate)
		d.Samples = len(y)
		if len(y) < minBandSamples {
			d.Skipped = "no signal above the band's noise in the window"
			res.Bands = append(res.Bands, d)
			continue
		}
		er, err := esprit(y, maxOrderPerBand)
		if err != nil {
			d.Skipped = err.Error()
			res.Bands = append(res.Bands, d)
			continue
		}
		d.Order, d.Signal, d.NoiseDB = er.Order, er.Signal, 10*math.Log10(math.Max(er.NoiseVar, 1e-20))
		fsd := float64(p.Rate) / float64(b.Decim)
		t0 := float64(first-p.Onset) / float64(p.Rate)
		for k, z := range er.Poles {
			if z == 0 {
				continue
			}
			s := cmplx.Log(z) * complex(fsd, 0) // continuous-time baseband pole
			alpha := -real(s)
			f := b.Center + imag(s)/(2*math.Pi)
			if f < b.Lo || f >= b.Hi {
				d.RejOutside++
				continue
			}
			if alpha <= 0 {
				d.RejGrowing++
				continue
			}
			snr := 10 * math.Log10(cmplxAbs2(er.Amps[k])/math.Max(er.NoiseVar, 1e-30))
			if snr < 0 {
				d.RejLowSNR++
				continue
			}
			a := 2 * er.Amps[k] / (b.gain(s, p.Rate) * cmplx.Exp(s*complex(t0, 0)))
			res.Modes = append(res.Modes, Mode{
				Freq: f, Alpha: alpha, Amp: cmplx.Abs(a), Phase: cmplx.Phase(a),
				Band: b.Index, SNRdB: snr,
			})
		}
		res.Bands = append(res.Bands, d)
	}
	// A mode that had to fall more than 40 dB before the window opened was
	// essentially over before analysis began: it belongs to the strike
	// transient (the residual), and its onset amplitude would be a 40 dB+
	// extrapolation of noise.
	tws := float64(p.WindowStart-p.Onset) / float64(p.Rate)
	observable := res.Modes[:0]
	for _, m := range res.Modes {
		if m.Alpha*tws <= maxExtrapolationNp {
			observable = append(observable, m)
		}
	}
	res.Modes = refitAmplitudes(p, observable)
	var strongest float64
	for _, m := range res.Modes {
		strongest = math.Max(strongest, m.Amp)
	}
	floor := strongest * math.Pow(10, modeRelFloorDB/20.0)
	kept := res.Modes[:0]
	for _, m := range res.Modes {
		if m.Amp >= floor {
			kept = append(kept, m)
		}
	}
	res.Modes = kept
	sort.SliceStable(res.Modes, func(i, j int) bool { return res.Modes[i].Energy() > res.Modes[j].Energy() })
	return res, nil
}

// trimToBandSignal cuts y after the last point where its smoothed power is
// still 6 dB above the band's share of the recording's noise floor (white
// noise of RMS σ heterodyned and low-passed to two-sided width 2w carries
// σ²·2w/rate). A band that never rises above it returns nil.
func trimToBandSignal(y []complex128, floorRMS float64, b band, rate int) []complex128 {
	if len(y) == 0 {
		return nil
	}
	const win = 8
	smooth := make([]float64, len(y))
	var acc float64
	quietest := math.Inf(1)
	for m := range y {
		acc += cmplxAbs2(y[m])
		if m >= win {
			acc -= cmplxAbs2(y[m-win])
		}
		smooth[m] = acc / float64(min(m+1, win))
		if m >= win-1 && smooth[m] < quietest {
			quietest = smooth[m]
		}
	}
	// The band's noise is the lower of its share of the recording floor and
	// its own quietest moment: either can be read too high (a floor taken
	// from a ring; a band that rings throughout), and too high a noise skips
	// weak bands outright.
	noise := floorRMS * floorRMS * 2 * (b.Hi - b.Lo) / float64(rate)
	if quietest < noise {
		noise = quietest
	}
	thr := 4 * noise
	last := -1
	for m, v := range smooth {
		if v > thr {
			last = m
		}
	}
	if last < 0 {
		return nil
	}
	return y[:last+1]
}

func cmplxAbs2(z complex128) float64 { return real(z)*real(z) + imag(z)*imag(z) }

const (
	refitRows     = 4096
	refitMaxModes = 128 // the modal layer's table capacity (spec §3b)
	refitDenseSec = 0.2
	// refitRelTol drops normal-matrix eigen-directions below this fraction of
	// the largest (singular values below 1e-4 of the largest).
	refitRelTol = 1e-8
)

// refitAmplitudes replaces every mode's amplitude and phase with ONE joint
// least-squares fit of the full-band recording from the onset, keeping
// ESPRIT's frequencies and decay rates:
//
//	x(t) ≈ Σ_k e^{−α_k·t}·(A_k·cos ω_k t + B_k·sin ω_k t),  t from the onset.
//
// The per-band Vandermonde amplitudes are only valid INSIDE each band's
// window, which starts a long filter half-length after the onset in the low
// bands; extrapolated back to the onset they blew up — a spurious 32 Hz
// "mode" in VCSL's marimba came back +48 dB, and clustered poles (cowbell's
// 464–491 Hz group, a tom's pitch glide) cancel each other inside the window
// and stop cancelling outside it. A joint fit sees the early signal.
//
// The solve is the TRUNCATED minimum-norm one: fast modes at nearby
// frequencies are nearly collinear over the window, and a plain least-squares
// gives them huge cancelling amplitudes (measured: residuals of 10⁶ % on the
// corpus). The normal matrix is eigendecomposed and directions below
// refitRelTol of the largest — exactly those cancelling combinations — are
// dropped. Rows are a seeded jittered sample of times (deterministic; no two
// modes alias onto one column the way a uniform stride could).
func refitAmplitudes(p Prepared, modes []Mode) []Mode {
	if len(modes) == 0 {
		return modes
	}
	sort.SliceStable(modes, func(i, j int) bool { return modes[i].Energy() > modes[j].Energy() })
	if len(modes) > refitMaxModes {
		modes = modes[:refitMaxModes]
	}
	// Rows span [onset, window end): frequencies and decays come from the
	// settled window, but the amplitudes must hold FROM THE ONSET, where the
	// resynthesis starts — fitted on the window alone, fast modes were free
	// before it and blew up there.
	// Rows: EVERY sample of the first refitDenseSec after the onset (where
	// nearly all the energy is — a sparse sample there leaves the high modes
	// unconstrained between rows: one row per 16 samples cannot pin a mode
	// above ~1.4 kHz, and the fit blew up between them), then a seeded
	// jittered sample of the rest of the window. The amplitudes must hold FROM
	// THE ONSET, where the resynthesis starts; frequencies and decays still
	// come from the settled window.
	k2 := 2 * len(modes)
	gr := make([]float64, k2*k2) // the real normal matrix (upper triangle)
	h := make([]float64, k2)
	row := make([]float64, k2)
	addRow := func(si int) {
		t := float64(si-p.Onset) / float64(p.Rate)
		for k, m := range modes {
			env := math.Exp(-m.Alpha * t)
			sn, cs := math.Sincos(2 * math.Pi * m.Freq * t)
			row[2*k], row[2*k+1] = env*cs, env*sn
		}
		b := p.Samples[si]
		for a := 0; a < k2; a++ {
			ra := row[a]
			if ra == 0 {
				continue
			}
			h[a] += ra * b
			grow := gr[a*k2 : a*k2+k2]
			for c := a; c < k2; c++ {
				grow[c] += ra * row[c]
			}
		}
	}
	dense := min(p.WindowEnd, p.Onset+int(refitDenseSec*float64(p.Rate)))
	for si := p.Onset; si < dense; si++ {
		addRow(si)
	}
	if n := p.WindowEnd - dense; n > 0 {
		rows := min(refitRows, n)
		r := rand.New(rand.NewSource(1))
		for i := 0; i < rows; i++ {
			lo := dense + i*n/rows
			hi := dense + (i+1)*n/rows
			addRow(lo + r.Intn(max(1, hi-lo)))
		}
	}
	g := linalg.New(k2, k2)
	for a := 0; a < k2; a++ {
		for c := a; c < k2; c++ {
			v := complex(gr[a*k2+c], 0)
			g.Data[a*k2+c] = v
			g.Data[c*k2+a] = v
		}
	}
	vals, vecs := linalg.EigHermitian(g)
	x := make([]float64, k2)
	if len(vals) > 0 && vals[0] > 0 {
		tol := vals[0] * refitRelTol
		for e, lam := range vals {
			if lam <= tol {
				break
			}
			var dot complex128
			for a := 0; a < k2; a++ {
				dot += cmplx.Conj(vecs.At(a, e)) * complex(h[a], 0)
			}
			coef := dot / complex(lam, 0)
			for a := 0; a < k2; a++ {
				x[a] += real(vecs.At(a, e) * coef)
			}
		}
	}
	out := modes[:0]
	for k, m := range modes {
		A, B := x[2*k], x[2*k+1]
		m.Amp = math.Hypot(A, B)
		m.Phase = math.Atan2(-B, A)
		out = append(out, m)
	}
	return out
}```

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -count=1`
Expected: PASS. Tolerances are the spec's (0.5 cent, 5 % α, 0.5 dB); if one misses, print the band's `BandDiag` and the raw poles, and fix the estimator (window, trim, order) — never widen the spec tolerance silently; a justified change goes into the spec's §8 in the same task. Leave uncommitted.

---

### Task 9: `clone.FitDecayLaw`

**Files:**
- Create: `src/go/internal/audio/clone/decaylaw.go`
- Test: `src/go/internal/audio/clone/decaylaw_test.go`

**Interfaces:**
- Produces: `type DecayLaw struct{ A, P, B, RelRMS float64 }`, `func (d DecayLaw) Eval(f float64) float64`, `func FitDecayLaw(modes []Mode) DecayLaw`.

- [ ] **Step 1: Write the failing test**

```go
package clone

import (
	"math"
	"testing"
)

func TestFitDecayLawRecoversKnownLaw(t *testing.T) {
	law := DecayLaw{A: 2, P: 1.3, B: 30}
	var modes []Mode
	for i := 0; i < 20; i++ {
		f := 200 * math.Pow(60, float64(i)/19) // 200 Hz .. 12 kHz
		modes = append(modes, Mode{Freq: f, Alpha: law.Eval(f), Amp: 0.1})
	}
	got := FitDecayLaw(modes)
	if math.Abs(got.A/law.A-1) > 0.02 || math.Abs(got.P-law.P) > 0.02 || math.Abs(got.B/law.B-1) > 0.05 {
		t.Fatalf("fit %+v, want %+v", got, law)
	}
	if got.RelRMS > 0.01 {
		t.Fatalf("relative RMS error %.4f on an exact law", got.RelRMS)
	}
}

func TestFitDecayLawFewModes(t *testing.T) {
	got := FitDecayLaw([]Mode{{Freq: 440, Alpha: 2, Amp: 1}, {Freq: 880, Alpha: 4, Amp: 1}})
	// energies amp²/(4α) = 1/8 and 1/16 → mean α = (2/8 + 4/16) / (3/16) = 8/3
	if got.P != 0 || got.B != 0 || math.Abs(got.A-8.0/3) > 1e-9 {
		t.Fatalf("two modes: %+v, want a flat law at the energy-weighted mean 8/3", got)
	}
	if (FitDecayLaw(nil) != DecayLaw{}) {
		t.Fatal("no modes must give the zero law")
	}
}```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run DecayLaw -count=1`
Expected: FAIL — `undefined: DecayLaw`.

- [ ] **Step 3: Implement `decaylaw.go`**

```go
package clone

import "math"

// DecayLaw is α(f) = A·(f/1 kHz)^P + B·(f/10 kHz)^6 (spec §2e). Both
// reference frequencies are fixed: a free HF corner is not identifiable (B and
// the corner only ever appear as B/f_c⁶).
type DecayLaw struct {
	A      float64 `json:"a"`
	P      float64 `json:"p"`
	B      float64 `json:"b"`
	RelRMS float64 `json:"rel_rms"` // energy-weighted RMS of (fit/measured − 1)
}

// Eval returns α at frequency f (Np/s).
func (d DecayLaw) Eval(f float64) float64 {
	return d.A*math.Pow(f/1000, d.P) + d.B*math.Pow(f/10000, 6)
}

// FitDecayLaw fits the law across the modes, weighted by energy, minimising
// the RELATIVE error (fast high modes and slow low ones count alike): a grid
// over P ∈ [−1, 3] (step 0.01) with a non-negative linear solve for (A, B) at
// each P. Fewer than three modes cannot pin a shape: a flat law at the
// energy-weighted mean α.
func FitDecayLaw(modes []Mode) DecayLaw {
	if len(modes) == 0 {
		return DecayLaw{}
	}
	w := make([]float64, len(modes))
	var wsum float64
	for i, m := range modes {
		w[i] = m.Energy()
		wsum += w[i]
	}
	if len(modes) < 3 {
		var a float64
		for i, m := range modes {
			a += w[i] * m.Alpha
		}
		return DecayLaw{A: a / wsum}
	}
	best := DecayLaw{RelRMS: math.MaxFloat64}
	for step := 0; step <= 400; step++ {
		p := -1 + 0.01*float64(step)
		var suu, suv, svv, su, sv float64
		for i, m := range modes {
			u := math.Pow(m.Freq/1000, p) / m.Alpha
			v := math.Pow(m.Freq/10000, 6) / m.Alpha
			suu += w[i] * u * u
			suv += w[i] * u * v
			svv += w[i] * v * v
			su += w[i] * u
			sv += w[i] * v
		}
		var cands [][2]float64
		if det := suu*svv - suv*suv; det > 1e-300 {
			a := (su*svv - sv*suv) / det
			b := (sv*suu - su*suv) / det
			if a >= 0 && b >= 0 {
				cands = append(cands, [2]float64{a, b})
			}
		}
		if suu > 0 {
			cands = append(cands, [2]float64{su / suu, 0})
		}
		if svv > 0 {
			cands = append(cands, [2]float64{0, sv / svv})
		}
		for _, c := range cands {
			law := DecayLaw{A: c[0], P: p, B: c[1]}
			var e float64
			for i, m := range modes {
				r := law.Eval(m.Freq)/m.Alpha - 1
				e += w[i] * r * r
			}
			law.RelRMS = math.Sqrt(e / wsum)
			if law.RelRMS < best.RelRMS-1e-12 {
				best = law
			}
		}
	}
	return best
}
```

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run DecayLaw -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 10: `clone.EstimateStrike`

**Files:**
- Create: `src/go/internal/audio/clone/strike.go`
- Test: `src/go/internal/audio/clone/strike_test.go`

**Interfaces:**
- Produces: `type Strike struct{ TcSec, HardnessDBPerOct, GainDB, ClickLevelDB, FitRMSdB float64 }`, `func StrikeSpectrum(f, tc float64) float64`, `func HardnessGain(f, h float64) float64`, `func EstimateStrike(modes []Mode) Strike` (ClickLevelDB is filled by Analyze from the residual).

- [ ] **Step 1: Write the failing test**

```go
package clone

import (
	"math"
	"testing"
)

func TestStrikeSpectrumShape(t *testing.T) {
	tc := 0.001
	if StrikeSpectrum(0, tc) != 1 {
		t.Fatal("DC gain must be 1")
	}
	if v := StrikeSpectrum(1/tc, tc); math.Abs(v-0.5) > 1e-9 {
		t.Fatalf("S(1/Tc) = %v, want 0.5", v)
	}
	if v := StrikeSpectrum(2/tc, tc); v > 1e-9 {
		t.Fatalf("S(2/Tc) = %v, want the first null", v)
	}
}

func TestEstimateStrikeRecoversContactTime(t *testing.T) {
	for _, c := range []struct{ tc, h float64 }{{0.001, 0}, {0.0004, -3}, {0.003, 2}} {
		var modes []Mode
		for i := 0; i < 40; i++ {
			f := 100 * math.Pow(150, float64(i)/39) // 100 Hz .. 15 kHz
			amp := 0.2 * StrikeSpectrum(f, c.tc) * HardnessGain(f, c.h)
			if amp < 1e-7 {
				continue
			}
			modes = append(modes, Mode{Freq: f, Alpha: 5, Amp: amp})
		}
		got := EstimateStrike(modes)
		if math.Abs(got.TcSec/c.tc-1) > 0.10 {
			t.Errorf("tc %.4f ms: got %.4f ms", 1000*c.tc, 1000*got.TcSec)
		}
		if math.Abs(got.HardnessDBPerOct-c.h) > 0.5 {
			t.Errorf("tc %.4f ms: hardness %.2f, want %.2f", 1000*c.tc, got.HardnessDBPerOct, c.h)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run Strike -count=1`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `strike.go`**

```go
package clone

import "math"

// Strike is the fitted excitation (spec §2c / §3c): a raised-cosine contact
// force of duration TcSec plus a hardness tilt (dB/oct re 1 kHz).
type Strike struct {
	TcSec            float64 `json:"tc_s"`
	HardnessDBPerOct float64 `json:"hardness_db_per_oct"`
	GainDB           float64 `json:"gain_db"`        // fitted mode level at 1 kHz before the force spectrum
	ClickLevelDB     float64 `json:"click_level_db"` // residual vs reference over the contact (from MeasureResidual)
	FitRMSdB         float64 `json:"fit_rms_db"`
}

// StrikeSpectrum is |F(f)| of the force 0.5·(1 − cos(2πt/tc)) on [0, tc],
// normalised to 1 at DC: |sinc(f·tc) / (1 − (f·tc)²)|, first null at 2/tc.
func StrikeSpectrum(f, tc float64) float64 {
	x := f * tc
	if x == 0 {
		return 1
	}
	d := 1 - x*x
	if math.Abs(d) < 1e-9 {
		return 0.5
	}
	return math.Abs(math.Sin(math.Pi*x) / (math.Pi * x) / d)
}

// HardnessGain is the hardness tilt as a linear gain: h dB per octave re 1 kHz.
func HardnessGain(f, h float64) float64 {
	return math.Pow(10, h*math.Log2(f/1000)/20)
}

// EstimateStrike fits dB(amp_i) ≈ G + dB(S(f_i; Tc)) + H·log2(f_i/1 kHz)
// across the modes, energy-weighted: a log grid over Tc ∈ [0.05 ms, 50 ms]
// (241 points, 2.9 % apart) with a weighted line fit for (G, H) at each Tc.
// S is floored at −60 dB so a mode near a null cannot dominate.
func EstimateStrike(modes []Mode) Strike {
	if len(modes) == 0 {
		return Strike{}
	}
	n := len(modes)
	u := make([]float64, n)
	a := make([]float64, n)
	w := make([]float64, n)
	for i, m := range modes {
		u[i] = math.Log2(m.Freq / 1000)
		a[i] = dB(m.Amp)
		w[i] = m.Energy()
	}
	best := Strike{FitRMSdB: math.MaxFloat64}
	for i := 0; i <= 240; i++ {
		tc := 0.05e-3 * math.Pow(10, float64(i)/80)
		r := make([]float64, n)
		for k, m := range modes {
			r[k] = a[k] - dB(math.Max(StrikeSpectrum(m.Freq, tc), 1e-3))
		}
		g, h, cost := weightedLine(u, r, w)
		if cost < best.FitRMSdB-1e-9 {
			best = Strike{TcSec: tc, HardnessDBPerOct: h, GainDB: g, FitRMSdB: cost}
		}
	}
	return best
}

// weightedLine fits r ≈ g + h·u by weighted least squares and returns the
// weighted RMS residual. One distinct u (or one point) fits a constant.
func weightedLine(u, r, w []float64) (g, h, rmsErr float64) {
	var sw, su, sr, suu, sur float64
	for i := range u {
		sw += w[i]
		su += w[i] * u[i]
		sr += w[i] * r[i]
		suu += w[i] * u[i] * u[i]
		sur += w[i] * u[i] * r[i]
	}
	if sw == 0 {
		return 0, 0, 0
	}
	den := sw*suu - su*su
	if math.Abs(den) < 1e-12*sw*sw {
		g = sr / sw
	} else {
		h = (sw*sur - su*sr) / den
		g = (sr - h*su) / sw
	}
	var e float64
	for i := range u {
		d := r[i] - g - h*u[i]
		e += w[i] * d * d
	}
	return g, h, math.Sqrt(e / sw)
}
```

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run Strike -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 11: `clone.MeasureResidual`

**Files:**
- Create: `src/go/internal/audio/clone/residual.go`
- Test: `src/go/internal/audio/clone/residual_test.go`

**Interfaces:**
- Consumes: `Synthesize`, `Prepared`.
- Produces: `type NoiseBand struct{ CenterHz, Q, LevelDB, AttackSec, FastRate, FastMix, TailRate float64; Active bool }`, `type Residual struct{ EnergyFraction, ClickLevelDB float64; Bands []NoiseBand }`, `func MeasureResidual(p Prepared, modes []Mode, clickSec float64) Residual`, `func bandpass(x []float64, rate int, fc, q float64) []float64`.

- [ ] **Step 1: Write the failing test**

```go
package clone

import (
	"math"
	"math/rand"
	"testing"
)

// TestMeasureResidualRecoversABandEnvelope: a band-limited noise envelope can
// only be measured to ~1/sqrt(B·τ). Measured spread of the fast rate over 20
// seeds: 26 % in the 2 kHz third-octave (B ≈ 470 Hz) at τ = 1/30 s, 12 % in
// the 8 kHz one (B ≈ 1.9 kHz) — so the estimator is judged statistically:
// unbiased on average over 10 seeds, every seed within a 2.5σ bound.
func TestMeasureResidualRecoversABandEnvelope(t *testing.T) {
	const fc, m, r1, r2 = 8063.0, 0.7, 30.0, 6.0
	var sumFast, sumTail, sumMix float64
	const seeds = 10
	for seed := int64(1); seed <= seeds; seed++ {
		got := measureOneBandEnvelope(t, seed, fc, m, r1, r2)
		if math.Abs(got.FastRate/r1-1) > 0.35 || math.Abs(got.TailRate/r2-1) > 0.2 || math.Abs(got.FastMix-m) > 0.15 {
			t.Errorf("seed %d: band %+v, want fast %.0f mix %.2f tail %.0f", seed, got, r1, m, r2)
		}
		if got.AttackSec > 0.01 {
			t.Errorf("seed %d: attack %.4f s for an instant onset", seed, got.AttackSec)
		}
		sumFast += got.FastRate
		sumTail += got.TailRate
		sumMix += got.FastMix
	}
	if b := sumFast/seeds/r1 - 1; math.Abs(b) > 0.10 {
		t.Errorf("fast rate biased %+.0f%% over %d seeds", 100*b, seeds)
	}
	if b := sumTail/seeds/r2 - 1; math.Abs(b) > 0.05 {
		t.Errorf("tail rate biased %+.0f%% over %d seeds", 100*b, seeds)
	}
	if b := sumMix/seeds - m; math.Abs(b) > 0.05 {
		t.Errorf("fast mix biased %+.2f over %d seeds", b, seeds)
	}
}

func measureOneBandEnvelope(t *testing.T, seed int64, fc, m, r1, r2 float64) NoiseBand {
	t.Helper()
	rate := 48000
	pre := 2400
	n := pre + int(1.5*float64(rate))
	r := rand.New(rand.NewSource(seed))
	white := make([]float64, n)
	for i := range white {
		white[i] = r.NormFloat64()
	}
	nb := bandpass(white, rate, fc, thirdOctaveQ)
	x := make([]float64, n)
	for i := pre; i < n; i++ {
		tt := float64(i-pre) / float64(rate)
		x[i] = 0.05 * nb[i] * (m*math.Exp(-r1*tt) + (1-m)*math.Exp(-r2*tt))
	}
	p := Prepared{Samples: x, Rate: rate, Onset: pre, WindowStart: pre, WindowEnd: n, FloorRMS: 1e-9}
	res := MeasureResidual(p, nil, 0.002)
	if res.EnergyFraction < 0.999 {
		t.Fatalf("no modes: residual fraction %.3f, want 1", res.EnergyFraction)
	}
	for _, b := range res.Bands {
		if math.Abs(b.CenterHz-fc) < 5 {
			if !b.Active {
				t.Fatalf("%.0f Hz band inactive: %+v", fc, b)
			}
			return b
		}
	}
	t.Fatalf("no band at %.0f Hz", fc)
	return NoiseBand{}
}

func TestMeasureResidualOfAPureModalSignalIsSmall(t *testing.T) {
	modes := barModes()
	x := modalSignal(modes, 48000, 0.05, 2, 0, 0)
	p := Prepared{Samples: x, Rate: 48000, Onset: 2400, WindowStart: 2400, WindowEnd: len(x), FloorRMS: 1e-9}
	if f := MeasureResidual(p, modes, 0.002).EnergyFraction; f > 1e-6 {
		t.Fatalf("exact modes leave residual fraction %.2e", f)
	}
}```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run Residual -count=1`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `residual.go`**

```go
package clone

import (
	"math"
	"sort"
)

// NoiseBand is the residual's envelope in one third-octave band (spec §2d):
// level, attack, and a two-rate decay A·(mix·e^{−fast·t} + (1−mix)·e^{−tail·t})
// after the peak. A MEASUREMENT of the residual, never the residual itself.
type NoiseBand struct {
	CenterHz  float64 `json:"center_hz"`
	Q         float64 `json:"q"`
	LevelDB   float64 `json:"level_db"`
	AttackSec float64 `json:"attack_s"`
	FastRate  float64 `json:"fast_rate"`
	FastMix   float64 `json:"fast_mix"`
	TailRate  float64 `json:"tail_rate"`
	Active    bool    `json:"active"`
}

// Residual summarises reference − modal resynthesis.
type Residual struct {
	EnergyFraction float64     `json:"energy_fraction"` // residual / reference energy over the window
	ClickLevelDB   float64     `json:"click_level_db"`  // the same ratio over the contact, dB
	Bands          []NoiseBand `json:"bands"`
}

const (
	thirdOctaveQ   = 4.318 // 1 / (2^(1/6) − 2^(−1/6))
	residualHopSec = 0.004
)

// MeasureResidual subtracts the modal resynthesis from the reference over
// [onset, window end) and measures the remainder per third-octave band
// (centres 31.5·2^(k/3) Hz below 0.45·rate). clickSec is the contact length
// the click level is measured over.
func MeasureResidual(p Prepared, modes []Mode, clickSec float64) Residual {
	ref := p.Samples[p.Onset:p.WindowEnd]
	syn := Synthesize(modes, p.Rate, len(ref))
	res := make([]float64, len(ref))
	var eRef, eRes float64
	for i, v := range ref {
		res[i] = v - syn[i]
		eRef += v * v
		eRes += res[i] * res[i]
	}
	out := Residual{EnergyFraction: eRes / math.Max(eRef, 1e-30)}
	nc := min(len(ref), max(1, int(clickSec*float64(p.Rate))))
	out.ClickLevelDB = dB(rms(res[:nc])) - dB(rms(ref[:nc]))
	for k := 0; ; k++ {
		fc := 31.5 * math.Pow(2, float64(k)/3)
		if fc*math.Pow(2, 1.0/6) >= 0.45*float64(p.Rate) {
			break
		}
		out.Bands = append(out.Bands, measureBand(bandpass(res, p.Rate, fc, thirdOctaveQ), p.Rate, fc))
	}
	return out
}

func measureBand(x []float64, rate int, fc float64) NoiseBand {
	nb := NoiseBand{CenterHz: fc, Q: thirdOctaveQ, LevelDB: -200}
	hop := max(1, int(residualHopSec*float64(rate)))
	var env []float64
	for a := 0; a+hop <= len(x); a += hop {
		env = append(env, rms(x[a:a+hop]))
	}
	if len(env) < 4 {
		return nb
	}
	tail := append([]float64(nil), env[len(env)*4/5:]...)
	sort.Float64s(tail)
	floor := tail[len(tail)/2]
	// Peak on a 3-frame smoothed envelope: band-limited noise fluctuates
	// several dB frame to frame, and the raw maximum lands on a fluctuation.
	ip := 0
	sm := func(i int) float64 {
		lo, hi := max(0, i-1), min(len(env)-1, i+1)
		var s float64
		for k := lo; k <= hi; k++ {
			s += env[k]
		}
		return s / float64(hi-lo+1)
	}
	for i := range env {
		if sm(i) > sm(ip) {
			ip = i
		}
	}
	peak := sm(ip)
	if peak <= 2*floor || peak == 0 {
		return nb
	}
	// The attack ends at the first frame within 3 dB of the peak — a later
	// jitter maximum must not swallow the fast decay that starts at once.
	for i := 0; i < ip; i++ {
		if sm(i) >= peak/math.Sqrt2 {
			ip = i
			break
		}
	}
	last := ip
	for i := ip; i < len(env); i++ {
		if env[i] > 2*floor {
			last = i
		}
	}
	dt := residualHopSec
	nb.Active = true
	nb.AttackSec = float64(ip) * dt
	if last-ip < 6 {
		nb.LevelDB = dB(peak)
		return nb
	}
	// Tail rate: log-linear fit over the second half of [peak, last].
	mid := ip + (last-ip)/2
	s2, _ := logLine(env, ip, mid, last, dt)
	nb.TailRate = math.Max(-s2, 0)
	a1, a2, r1 := fitTwoRate(env[ip:last+1], dt, nb.TailRate)
	nb.FastRate = r1
	if a1+a2 > 0 {
		nb.FastMix = a1 / (a1 + a2)
	}
	if nb.FastMix == 0 {
		nb.FastRate = nb.TailRate
	}
	nb.LevelDB = dB(a1 + a2)
	return nb
}

// fitTwoRate fits env[k] ≈ a1·e^{−r1·t} + a2·e^{−r2·t} (t = k·dt, r2 given)
// minimising the RELATIVE error — the envelope jitter of band-limited noise is
// multiplicative. The weights come from the MODEL (iteratively reweighted
// least squares), not from the data: weighting by 1/env over-trusts the frames
// that happened to dip low and biased the fast rate upward. A log grid over
// r1 ∈ (r2, 2000] with a non-negative linear solve for (a1, a2) at each r1; a
// pure tail (a1 = 0) competes too.
func fitTwoRate(env []float64, dt, r2 float64) (a1, a2, r1 float64) {
	n := len(env)
	e2s := make([]float64, n)
	for k := range e2s {
		e2s[k] = math.Exp(-r2 * float64(k) * dt)
	}
	e1s := make([]float64, n)
	w := make([]float64, n)
	solve := func(oneTerm bool) (float64, float64) {
		var s11, s12, s22, s1, s2 float64
		for k, v := range env {
			ww := w[k] * w[k]
			s11 += ww * e1s[k] * e1s[k]
			s12 += ww * e1s[k] * e2s[k]
			s22 += ww * e2s[k] * e2s[k]
			s1 += ww * e1s[k] * v
			s2 += ww * e2s[k] * v
		}
		if oneTerm || s11*s22-s12*s12 <= 1e-300*s11*s22 {
			if s22 == 0 {
				return 0, 0
			}
			return 0, math.Max(s2/s22, 0)
		}
		det := s11*s22 - s12*s12
		x1 := (s1*s22 - s2*s12) / det
		x2 := (s2*s11 - s1*s12) / det
		if x1 < 0 {
			return 0, math.Max(s2/s22, 0)
		}
		if x2 < 0 {
			return math.Max(s1/s11, 0), 0
		}
		return x1, x2
	}
	fit := func(r float64, oneTerm bool) (c, x1, x2 float64) {
		for k := range e1s {
			e1s[k] = math.Exp(-r * float64(k) * dt)
		}
		for k, v := range env {
			w[k] = 0
			if v > 0 {
				w[k] = 1 / v
			}
		}
		for it := 0; it < 4; it++ {
			x1, x2 = solve(oneTerm)
			if it == 3 {
				break
			}
			for k := range w {
				if m := x1*e1s[k] + x2*e2s[k]; m > 0 {
					w[k] = 1 / m
				}
			}
		}
		for k, v := range env {
			m := x1*e1s[k] + x2*e2s[k]
			if m <= 0 {
				continue
			}
			d := (m - v) / m
			c += d * d
		}
		return c, x1, x2
	}
	best, a1, a2 := fit(r2, true)
	r1 = r2
	lo := math.Max(r2*1.05, 1)
	for i := 0; i <= 200; i++ {
		r := lo * math.Pow(2000/lo, float64(i)/200)
		if c, x1, x2 := fit(r, false); c < best-1e-12 {
			best, a1, a2, r1 = c, x1, x2, r
		}
	}
	return a1, a2, r1
}

// logLine fits ln(v[i]) ≈ c + s·(i−origin)·dt over i ∈ [lo, hi] where v > 0.
func logLine(v []float64, origin, lo, hi int, dt float64) (s, c float64) {
	var u, r, w []float64
	for i := lo; i <= hi; i++ {
		if v[i] > 0 {
			u = append(u, float64(i-origin)*dt)
			r = append(r, math.Log(v[i]))
			w = append(w, 1)
		}
	}
	c, s, _ = weightedLine(u, r, w)
	return s, c
}

// bandpass is an RBJ constant-0-dB-peak band-pass, run forward.
func bandpass(x []float64, rate int, fc, q float64) []float64 {
	w0 := 2 * math.Pi * fc / float64(rate)
	alpha := math.Sin(w0) / (2 * q)
	a0 := 1 + alpha
	b0, b2 := alpha/a0, -alpha/a0
	a1, a2 := -2*math.Cos(w0)/a0, (1-alpha)/a0
	out := make([]float64, len(x))
	var x1, x2, y1, y2 float64
	for i, v := range x {
		y := b0*v + b2*x2 - a1*y1 - a2*y2
		x2, x1 = x1, v
		y2, y1 = y1, y
		out[i] = y
	}
	return out
}```

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run Residual -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 12: `clone.Analyze` — composition, refusal, glide, cross-check, JSON, import guard

**Files:**
- Create: `src/go/internal/audio/clone/analyze.go`
- Test: `src/go/internal/audio/clone/analyze_test.go`, `src/go/internal/audio/clone/devonly_test.go`

**Interfaces:**
- Consumes: everything above; `fingerprint.NewBundle`, `fingerprint.AnalyzeModes`, `wave.MagnitudeSpectrum`.
- Produces: `type Analysis struct{…}` (fields below), `type MissedMode struct{ Hz, PeakDB float64 }`, `type Timings struct{…}`, `func Analyze(p Prepared) (Analysis, error)` — returns a filled Analysis together with a `not_modal` refusal.

- [ ] **Step 1: Write the failing tests**

`analyze_test.go`:

```go
package clone

import (
	"encoding/json"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestAnalyzeEndToEnd(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2.5, 0.004, 31)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "bar"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("timings %+v", a.Timings)
	if a.Format != AnalysisFormat || len(a.Modes) < 4 || a.Residual.EnergyFraction > 0.1 {
		t.Fatalf("analysis %+v", a)
	}
	if math.Abs(a.GlideCents) > 2 {
		t.Fatalf("glide %.2f cents on fixed-frequency modes", a.GlideCents)
	}
	b, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Modes, b.Modes) || !reflect.DeepEqual(a.Strike, b.Strike) || !reflect.DeepEqual(a.Residual, b.Residual) {
		t.Fatal("Analyze is not deterministic")
	}
}

func TestAnalyzeNoiselessJSONIsFinite(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 1.5, 0, 0)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatalf("noiseless analysis is not valid JSON: %v", err)
	}
}

func TestAnalyzeRefusesASustainedNoise(t *testing.T) {
	r := rand.New(rand.NewSource(41))
	x := make([]float64, 48000*2)
	for i := 2400; i < len(x); i++ {
		x[i] = 0.2 * r.NormFloat64() * math.Exp(-1.5*float64(i-2400)/48000)
	}
	for i := 0; i < 2400; i++ {
		x[i] = 0.0005 * r.NormFloat64()
	}
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if !IsRefusal(err, RefuseNotModal) {
		t.Fatalf("decaying noise: err = %v, want not_modal", err)
	}
	if a.Residual.EnergyFraction <= 0.5 || a.Format != AnalysisFormat {
		t.Fatalf("refusal must still return the partial analysis: %+v", a)
	}
}

func TestAnalyzeReportsAGlide(t *testing.T) {
	rate := 48000
	n := 2 * rate
	x := make([]float64, 2400+n)
	var ph float64
	for i := 0; i < n; i++ {
		tt := float64(i) / float64(rate)
		f := 200 * (1 + 0.03*math.Exp(-2*tt)) // a membrane's tension glide: 3 % sharp at the strike
		ph += 2 * math.Pi * f / float64(rate)
		x[2400+i] = 0.4 * math.Exp(-1.5*tt) * math.Cos(ph)
	}
	r := rand.New(rand.NewSource(42))
	for i := range x {
		x[i] += 0.0005 * r.NormFloat64()
	}
	p, err := Prepare(x, rate, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Analyze(p)
	if a.GlideCents > -10 {
		t.Fatalf("glide %.1f cents, want a downward drift > 10 cents", a.GlideCents)
	}
	if len(a.Notes) == 0 {
		t.Fatal("a glide must be reported in Notes")
	}
}```

`devonly_test.go`:

```go
package clone

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloneIsDevOnly: the clone analysis is developer tooling. Only the clone
// packages themselves and cmd/clone may import it, so it never reaches the
// desktop binary or the WASM bundle.
func TestCloneIsDevOnly(t *testing.T) {
	const pkg = "github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	allowed := map[string]bool{
		"internal/audio/clone":        true,
		"internal/audio/clone/linalg": true,
		"cmd/clone":                   true,
	}
	srcRoot, err := filepath.Abs("../../..") // src/go
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	err = filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(srcRoot, path)
		if allowed[filepath.ToSlash(filepath.Dir(rel))] {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		for _, imp := range f.Imports {
			if p := strings.Trim(imp.Path.Value, `"`); p == pkg || strings.HasPrefix(p, pkg+"/") {
				t.Errorf("%s imports %s — the clone analysis is dev-only tooling", rel, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ -run 'Analyze|DevOnly' -count=1`
Expected: FAIL — `undefined: Analyze` (DevOnly passes already; it is a guard).

- [ ] **Step 3: Implement `analyze.go`**

```go
package clone

import (
	"fmt"
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// AnalysisFormat identifies analysis.json (a local dev artifact, never shipped).
const AnalysisFormat = "beatmo.clone.analysis/1"

// MissedMode is a strong STFT-tracked partial with no ESPRIT mode nearby.
type MissedMode struct {
	Hz     float64 `json:"hz"`
	PeakDB float64 `json:"peak_db"`
}

// Timings are wall-clock milliseconds per stage (measured, never assumed).
type Timings struct {
	ModesMs      float64 `json:"modes_ms"`
	StrikeMs     float64 `json:"strike_ms"`
	DecayLawMs   float64 `json:"decay_law_ms"`
	ResidualMs   float64 `json:"residual_ms"`
	CrossCheckMs float64 `json:"cross_check_ms"`
	TotalMs      float64 `json:"total_ms"`
}

// Analysis is everything measured about one reference (spec §2f).
type Analysis struct {
	Format         string       `json:"format"`
	Label          string       `json:"label"`
	Rate           int          `json:"rate"`
	OnsetSec       float64      `json:"onset_s"`
	FloorDB        float64      `json:"floor_db"`
	PeakDB         float64      `json:"peak_db"`
	RMS400DB       float64      `json:"rms400_db"`
	WindowStartSec float64      `json:"window_start_s"` // from the onset
	WindowEndSec   float64      `json:"window_end_s"`
	Pitch          PitchInfo    `json:"pitch"`
	Modes          []Mode       `json:"modes"`
	Strike         Strike       `json:"strike"`
	DecayLaw       DecayLaw     `json:"decay_law"`
	Residual       Residual     `json:"residual"`
	Bands          []BandDiag   `json:"bands"`
	Missed         []MissedMode `json:"missed"`
	GlideCents     float64      `json:"glide_cents"`
	Notes          []string     `json:"notes"`
	Timings        Timings      `json:"timings"`
}

const (
	maxResidualFraction = 0.5
	glideWarnCents      = 10
	missedRangeDB       = 40
)

// Analyze composes the stages. On a not_modal refusal it still returns the
// filled Analysis (partial results are always written, spec §7).
func Analyze(p Prepared) (Analysis, error) {
	t0 := time.Now()
	rate := float64(p.Rate)
	a := Analysis{
		Format: AnalysisFormat, Label: p.Label, Rate: p.Rate,
		OnsetSec: float64(p.Onset) / rate, FloorDB: dB(p.FloorRMS), PeakDB: dB(p.Peak), RMS400DB: p.RMS400DB,
		WindowStartSec: float64(p.WindowStart-p.Onset) / rate, WindowEndSec: float64(p.WindowEnd-p.Onset) / rate,
		Pitch: p.Pitch, Notes: []string{}, Missed: []MissedMode{},
	}
	ms := func(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

	t := time.Now()
	mr, err := EstimateModes(p)
	if err != nil {
		return a, err
	}
	a.Modes, a.Bands = mr.Modes, mr.Bands
	a.Timings.ModesMs = ms(t)

	t = time.Now()
	a.Strike = EstimateStrike(a.Modes)
	a.Timings.StrikeMs = ms(t)

	t = time.Now()
	a.DecayLaw = FitDecayLaw(a.Modes)
	a.Timings.DecayLawMs = ms(t)

	t = time.Now()
	a.Residual = MeasureResidual(p, a.Modes, math.Max(a.Strike.TcSec, 0.002))
	a.Strike.ClickLevelDB = a.Residual.ClickLevelDB
	a.Timings.ResidualMs = ms(t)

	t = time.Now()
	a.Missed = missedModes(p, a.Modes)
	if len(a.Modes) > 0 {
		a.GlideCents = glideCents(p, a.Modes[0])
		if math.Abs(a.GlideCents) > glideWarnCents {
			a.Notes = append(a.Notes, fmt.Sprintf(
				"pitch glide: the strongest mode (%.1f Hz) drifts %+.1f cents over the decay — a membrane? Fixed-frequency modes cannot express it (out of slice-1 scope)",
				a.Modes[0].Freq, a.GlideCents))
		}
	}
	a.Timings.CrossCheckMs = ms(t)
	a.Timings.TotalMs = ms(t0)

	if a.Residual.EnergyFraction > maxResidualFraction {
		return a, refuse(RefuseNotModal, "residual is %.0f%% of the energy after the modal fit — not a struck/modal sound",
			100*a.Residual.EnergyFraction)
	}
	return a, nil
}

// missedModes cross-checks against the STFT mode tracker: a tracked partial
// within 40 dB of the loudest with no ESPRIT mode within max(3 cents, one STFT
// bin) is listed. (A 4096-point tracker places a partial to about a bin —
// half a bin flagged modes ESPRIT had found 3 cents away.)
func missedModes(p Prepared, modes []Mode) []MissedMode {
	out := []MissedMode{}
	w := wave.Wave{Samples: p.Samples[p.Onset:p.WindowEnd], SampleRate: p.Rate}
	st := fingerprint.NewBundle(w, p.Label).STFT(4096, 1024)
	set := fingerprint.AnalyzeModes(st)
	if len(set.Modes) == 0 {
		return out
	}
	loud := math.Inf(-1)
	for _, m := range set.Modes {
		loud = math.Max(loud, m.PeakDB)
	}
	bin := float64(p.Rate) / 4096
	for _, m := range set.Modes {
		if m.PeakDB < loud-missedRangeDB || m.Hz < bandLoHz {
			continue
		}
		tol := math.Max(3, 1200*math.Log2(1+bin/m.Hz))
		found := false
		for _, e := range modes {
			if math.Abs(1200*math.Log2(e.Freq/m.Hz)) <= tol {
				found = true
				break
			}
		}
		if !found {
			out = append(out, MissedMode{Hz: m.Hz, PeakDB: m.PeakDB})
		}
	}
	return out
}

// glideCents measures the strongest mode's frequency over the first and the
// last third of the decay window (Hann FFT, parabolic peak within ±3 %) and
// returns the drift last − first in cents.
func glideCents(p Prepared, m Mode) float64 {
	n := p.WindowEnd - p.WindowStart
	third := n / 3
	if third < 64 {
		return 0
	}
	f1, ok1 := peakNear(p.Samples[p.WindowStart:p.WindowStart+third], p.Rate, m.Freq)
	f2, ok2 := peakNear(p.Samples[p.WindowEnd-third:p.WindowEnd], p.Rate, m.Freq)
	if !ok1 || !ok2 {
		return 0
	}
	return 1200 * math.Log2(f2/f1)
}

// peakNear uses the largest power-of-two prefix of seg: MagnitudeSpectrum
// windows the WHOLE (zero-padded) buffer, so padding would leave the data
// under only the rising edge of the Hann window.
func peakNear(seg []float64, rate int, f float64) (float64, bool) {
	size := 1
	for size*2 <= len(seg) && size < 1<<18 {
		size <<= 1
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: seg[:size], SampleRate: rate}, size, wave.WindowHann)
	lo := max(1, int(f*0.97/binHz))
	hi := min(len(mag)-2, int(f*1.03/binHz)+1)
	if lo >= hi {
		return 0, false
	}
	k := lo
	for i := lo; i <= hi; i++ {
		if mag[i] > mag[k] {
			k = i
		}
	}
	a, b, c := dB(mag[k-1]), dB(mag[k]), dB(mag[k+1])
	off := 0.0
	if d := a - 2*b + c; d != 0 {
		off = 0.5 * (a - c) / d
	}
	return (float64(k) + off) * binHz, true
}```

- [ ] **Step 4: Run the whole package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/... -count=1`
Expected: PASS. Check `wave.MagnitudeSpectrum` zero-pads a shorter input to `size` (read `internal/wave/fft.go:75`); if it truncates instead, pass a padded copy. Leave uncommitted.

---

### Task 13: `cmd/clone`, the report, `make clone`, docs

**Files:**
- Create: `src/go/internal/audio/clone/report.go` (+ `report_test.go`)
- Create: `src/go/cmd/clone/main.go`, `src/go/cmd/clone/main_test.go`
- Modify: `Makefile` (new `clone` target next to `ab-session`), `CLAUDE.md` (Quick Start rows + one gotcha), `AGENTS.synth.instruments.md` (new "Cloning a struck instrument — analysis" section)

**Interfaces:**
- Consumes: `clone.PrepareFile`, `clone.Analyze`, `wavref.Catalogue`, `wavref.Resolve`, `wavref.Dir`.
- Produces: `func (a Analysis) Markdown() string`; CLI `clone -inst <id> [-ref <ref id>] [-wav <path>] [-f0 <Hz>] [-out <dir>] [-json]` writing `<out>/analysis.json` and `<out>/report.md` (default out `$BEATMO_WAV_REFS/clones/<inst>/`), exit 0 ok / 2 refusal (files still written) / 1 error.

- [ ] **Step 1: Write the failing tests**

`report_test.go`:

```go
package clone

import (
	"strings"
	"testing"
)

func TestMarkdownCarriesTheModeTable(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2, 0.002, 51)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "bar", F0Hz: 440})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	md := a.Markdown()
	for _, want := range []string{"# Clone analysis — bar", "| Hz |", "440.0", "Decay law", "Strike", "Residual", "Timings"} {
		if !strings.Contains(md, want) {
			t.Errorf("report lacks %q", want)
		}
	}
}
```

`cmd/clone/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestCloneWritesAnalysisAndReport(t *testing.T) {
	refs := t.TempDir()
	t.Setenv(wavref.EnvVar, refs)
	rate := 48000
	n := 2 * rate
	pcm := make([]float32, 2400+n)
	for i := 0; i < n; i++ {
		tt := float64(i) / float64(rate)
		pcm[2400+i] = float32(0.4 * math.Exp(-2*tt) * math.Cos(2*math.Pi*440*tt))
	}
	b, err := wavgen.EncodeWAV(pcm, rate)
	if err != nil {
		t.Fatal(err)
	}
	wav := filepath.Join(t.TempDir(), "bar.wav")
	if err := os.WriteFile(wav, b, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "testbar", "-wav", wav}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	dir := filepath.Join(refs, "clones", "testbar")
	raw, err := os.ReadFile(filepath.Join(dir, "analysis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var a clone.Analysis
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	if a.Format != clone.AnalysisFormat || len(a.Modes) == 0 {
		t.Fatalf("analysis.json: %+v", a)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCloneRefusalStillWritesAndExits2(t *testing.T) {
	refs := t.TempDir()
	t.Setenv(wavref.EnvVar, refs)
	b, _ := wavgen.EncodeWAV(make([]float32, 48000), 48000)
	wav := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(wav, b, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "silent", "-wav", wav}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2 (refusal): %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(refs, "clones", "silent", "report.md")); err != nil {
		t.Fatalf("a refusal must still write the report: %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/ ./cmd/clone/ -run 'Markdown|Clone' -count=1`
Expected: FAIL — `undefined: Markdown` / no `cmd/clone` package.

- [ ] **Step 3: Implement `report.go`**

```go
package clone

import (
	"fmt"
	"math"
	"strings"
)

// Markdown renders the human report (spec §6a).
func (a Analysis) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Clone analysis — %s\n\n", a.Label)
	fmt.Fprintf(&b, "- rate %d Hz · onset %.3f s · floor %.1f dB · peak %.1f dB · RMS(400 ms) %.1f dB\n",
		a.Rate, a.OnsetSec, a.FloorDB, a.PeakDB, a.RMS400DB)
	fmt.Fprintf(&b, "- decay window %.3f → %.3f s after the onset\n", a.WindowStartSec, a.WindowEndSec)
	if a.Pitch.Pitched {
		fmt.Fprintf(&b, "- pitch %.2f Hz (%s, confidence %.2f)", a.Pitch.F0Hz, a.Pitch.Source, a.Pitch.Confidence)
	} else {
		fmt.Fprintf(&b, "- unpitched (modes in absolute Hz; confidence %.2f)", a.Pitch.Confidence)
	}
	if a.Pitch.HasSemis {
		fmt.Fprintf(&b, " · catalogue note %+d st from A3", a.Pitch.SemisFromA3)
	}
	b.WriteString("\n")
	for _, n := range a.Notes {
		fmt.Fprintf(&b, "- **note:** %s\n", n)
	}

	fmt.Fprintf(&b, "\n## Modes (%d, by energy)\n\n| # | Hz | ratio | cents from harmonic | T60 s | dB re strongest | SNR dB |\n|---|---|---|---|---|---|---|\n", len(a.Modes))
	var top float64
	for _, m := range a.Modes {
		top = math.Max(top, m.Amp)
	}
	for i, m := range a.Modes {
		ratio, cents := "—", "—"
		if a.Pitch.Pitched && a.Pitch.F0Hz > 0 {
			r := m.Freq / a.Pitch.F0Hz
			h := math.Max(1, math.Round(r))
			ratio = fmt.Sprintf("%.4f", r)
			cents = fmt.Sprintf("%+.1f", 1200*math.Log2(r/h))
		}
		fmt.Fprintf(&b, "| %d | %.1f | %s | %s | %.2f | %.1f | %.0f |\n",
			i+1, m.Freq, ratio, cents, m.T60(), dB(m.Amp)-dB(top), m.SNRdB)
	}
	if len(a.Missed) > 0 {
		b.WriteString("\n**Missed** (STFT partials with no ESPRIT mode nearby):")
		for _, m := range a.Missed {
			fmt.Fprintf(&b, " %.1f Hz (%.1f dB);", m.Hz, m.PeakDB)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "\n## Decay law\n\nα(f) = %.4g·(f/1 kHz)^%.2f + %.4g·(f/10 kHz)^6 · relative RMS error %.1f%%\n",
		a.DecayLaw.A, a.DecayLaw.P, a.DecayLaw.B, 100*a.DecayLaw.RelRMS)
	fmt.Fprintf(&b, "\n## Strike\n\nTc %.3f ms · hardness %+.2f dB/oct · click %.1f dB re reference · fit RMS %.2f dB\n",
		1000*a.Strike.TcSec, a.Strike.HardnessDBPerOct, a.Strike.ClickLevelDB, a.Strike.FitRMSdB)
	fmt.Fprintf(&b, "\n## Residual\n\n%.1f%% of the energy is not modal.\n\n| Hz | level dB | attack ms | fast /s | fast mix | tail /s |\n|---|---|---|---|---|---|\n",
		100*a.Residual.EnergyFraction)
	for _, nb := range a.Residual.Bands {
		if !nb.Active {
			continue
		}
		fmt.Fprintf(&b, "| %.0f | %.1f | %.1f | %.1f | %.2f | %.1f |\n",
			nb.CenterHz, nb.LevelDB, 1000*nb.AttackSec, nb.FastRate, nb.FastMix, nb.TailRate)
	}
	t := a.Timings
	fmt.Fprintf(&b, "\n## Timings (ms)\n\nmodes %.1f · strike %.1f · decay law %.1f · residual %.1f · cross-check %.1f · total %.1f\n",
		t.ModesMs, t.StrikeMs, t.DecayLawMs, t.ResidualMs, t.CrossCheckMs, t.TotalMs)
	return b.String()
}
```

- [ ] **Step 4: Implement `cmd/clone/main.go`**

```go
// Command clone analyses a reference recording of a struck / modal instrument
// into a measured model (spec docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md).
// Plan 1 ships the analysis half: analysis.json + report.md under
// $BEATMO_WAV_REFS/clones/<inst>/. Inversion into a document, refinement and
// adopt follow in plans 2–3.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inst := fs.String("inst", "", "instrument id the clone is for (names the output directory)")
	refID := fs.String("ref", "", "catalogue reference id (default: the instrument's first reference)")
	wavPath := fs.String("wav", "", "analyse this WAV instead of a catalogue reference")
	f0 := fs.Float64("f0", 0, "nominal pitch in Hz (overrides detection)")
	outDir := fs.String("out", "", "output directory (default $BEATMO_WAV_REFS/clones/<inst>)")
	asJSON := fs.Bool("json", false, "print analysis.json to stdout")
	cpuProfile := fs.String("cpuprofile", "", "write a CPU profile of the analysis to this file")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		defer pprof.StopCPUProfile()
	}
	if *inst == "" {
		fmt.Fprintln(stderr, "clone: -inst is required")
		return 1
	}
	opt := clone.PrepareOptions{F0Hz: *f0, Label: *inst}
	path := *wavPath
	if path == "" {
		p, semis, has, err := resolveReference(*inst, *refID)
		if err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		path, opt.SemisFromA3, opt.HasSemis = p, semis, has
	}
	dir := *outDir
	if dir == "" {
		// Beside sources/ and candidates/ in the reference root — local dev
		// output, never in the repo.
		dir = filepath.Join(wavref.Root(), "clones", *inst)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}

	start := time.Now()
	p, perr := clone.PrepareFile(path, opt)
	prepMs := float64(time.Since(start).Microseconds()) / 1000
	var a clone.Analysis
	aerr := perr
	if perr == nil {
		a, aerr = clone.Analyze(p)
	} else {
		a = clone.Analysis{Format: clone.AnalysisFormat, Label: *inst, Notes: []string{perr.Error()}}
	}
	if aerr != nil && perr == nil {
		a.Notes = append(a.Notes, aerr.Error())
	}
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "clone: encode analysis:", err)
		return 1
	}
	md := a.Markdown() + fmt.Sprintf("\nPrepare: %.1f ms · source `%s`\n", prepMs, path)
	if err := os.WriteFile(filepath.Join(dir, "analysis.json"), raw, 0o644); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(md), 0o644); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}
	if *asJSON {
		stdout.Write(raw)
		fmt.Fprintln(stdout)
	} else {
		fmt.Fprintf(stdout, "clone %s: %d modes, residual %.0f%%, %.0f ms → %s\n",
			*inst, len(a.Modes), 100*a.Residual.EnergyFraction, a.Timings.TotalMs+prepMs, dir)
	}
	var ref *clone.Refusal
	if errors.As(aerr, &ref) {
		fmt.Fprintln(stderr, aerr)
		return 2
	}
	if aerr != nil {
		fmt.Fprintln(stderr, "clone:", aerr)
		return 1
	}
	return 0
}

// resolveReference picks the catalogue reference for inst (or refID) and
// returns its local path and measured note.
func resolveReference(inst, refID string) (string, int, bool, error) {
	cat, err := wavref.Catalogue()
	if err != nil {
		return "", 0, false, err
	}
	var ref wavref.Ref
	if refID != "" {
		r, ok := cat.Ref(refID)
		if !ok {
			return "", 0, false, fmt.Errorf("unknown reference %q", refID)
		}
		ref = r
	} else {
		found := false
		for _, r := range cat.ForInstrument(inst) {
			if r.Status == "" {
				ref, found = r, true
				break
			}
		}
		if !found {
			return "", 0, false, fmt.Errorf("no reference recording for %q (use -wav, or `make refs-adopt`)", inst)
		}
	}
	path, ok := wavref.Resolve(ref.ID)
	if !ok {
		return "", 0, false, fmt.Errorf("audio for %s is not available locally; run `refs fetch -id %s`", ref.ID, ref.ID)
	}
	if ref.Measured != nil {
		return path, ref.Measured.SemisFromA3, true, nil
	}
	return path, 0, false, nil
}```

(Check `wavref.Ref` has `ID` and `Status` fields — `cmd/refs/candidates.go` uses both — and that `wavref.Dir()` honours `$BEATMO_WAV_REFS`; if `Dir` reports `ok=false` for an existing default, use `wavref.Root()` instead, as `cmd/refs` does.)

- [ ] **Step 5: Makefile target and docs**

`Makefile` (after `ab-session`):

```make
clone: ## Analyse a struck/modal reference into a measured model: modes, strike, decay law, residual (INST=<instrument>, [REF=<ref id>], [WAV=<path>], [F0=<Hz>]; writes $BEATMO_WAV_REFS/clones/<inst>/analysis.json + report.md)
	@cd src/go && ../../.tools/go/bin/go run ./cmd/clone -inst $(INST) $(if $(REF),-ref $(REF),) $(if $(WAV),-wav $(WAV),) $(if $(F0),-f0 $(F0),)
```

`CLAUDE.md` Quick Start table, after the `make ab-session`-adjacent rows:

```
| Analyse a struck/modal reference for cloning | `make clone INST=<instrument> [REF=] [WAV=] [F0=]` (sub-band ESPRIT modes, strike, decay law, residual noise bands → `$BEATMO_WAV_REFS/clones/<inst>/analysis.json` + `report.md`; refuses clipped / onset-less / short / non-modal input; spec `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md`) |
```

`CLAUDE.md` Critical Gotchas, one bullet:

```
- **Analysis tools render through `audio.RenderAsShipped`, never the raw C render**: `RenderAsShipped(doc, bpm, sr, pitch)` is what a trigger plays (document FX, gate tail, peak-normalise × the DOCUMENT's amplitude, percussion pitched by resampling) and is pinned byte-identical to `RenderAsPlayed` for every shipped instrument (`TestRenderAsShippedMatchesPlayback`). `refs compare`/`diagnose` use it via `synthmatch.RenderShipped`. `RenderInstrumentWithFields` stays the RAW C render on purpose for `cmd/export_audio` (the cross-platform reference compares raw C on both sides), `make measure-loudness` (it computes `render.amplitude`, so it must not see it applied), `gen-fundamentals` and the organic calibration. Known divergence, not yet fixed: the browser sizes a voice by `render.seconds`, desktop by `render.beats × 60/bpm`.
```

`AGENTS.synth.instruments.md`, a new section:

```markdown
## Cloning a struck instrument — the analysis half (`make clone`)

`make clone INST=<id>` measures the instrument's reference recording into a
MODEL — never audio: damped modes (sub-band ESPRIT: 24 log bands, heterodyne +
Blackman low-pass + decimation, covariance eigenspace, order by noise
threshold + ESTER, Vandermonde amplitudes back-extrapolated to the onset), a
raised-cosine strike (Tc, hardness), the decay law α(f) = a·(f/1k)^p +
b·(f/10k)^6, and third-octave residual noise bands (level, attack, two-rate
decay). Output: `$BEATMO_WAV_REFS/clones/<id>/analysis.json` + `report.md`
(mode table, STFT cross-check "missed" partials, glide diagnostic, timings).
It refuses clipped, onset-less, short (< 150 ms) and non-modal (> 50 %
residual) input. Code: `internal/audio/clone` (dev-only, `TestCloneIsDevOnly`),
`cmd/clone`. Turning an analysis into a document (the `modal` / `strike` /
`noiseband` modules) and the refine/adopt loop are plans 2 and 3 of the spec
`docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md`.
```

- [ ] **Step 6: Run everything in this plan**

Run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/clone/... ./cmd/clone/ ./internal/audio/synthmatch/ ./cmd/refs/ -count=1
cd src/go && ../../.tools/go/bin/go test ./internal/audio -run TestRenderAsShippedMatchesPlayback -count=1
cd src/go && ../../.tools/go/bin/go test ./internal/audio/synthmatch -run TestRenderShipped -count=1
```
Expected: PASS. Then a real run when a local corpus exists: `make clone INST=marimba` and open `report.md` — the mode table, decay law and timings must be plausible (marimba: a strong fundamental, ~4× and ~10× partials; total well under a second). Leave uncommitted.

---

## Self-review notes

- Spec coverage (plan 1 scope): §1 → Tasks 1–2; §2a → 6; §2b → 7–8; §2c → 10 (ClickLevel via 11/12); §2d → 11; §2e → 9; §2f + glide + cross-check → 12; §6a analysis half → 13; §7 refusals: clipped / no onset / short decay → 6, not modal → 12, pitch fallback → 6; §8 known-answer / refusals / determinism (12) / imports (12) / linalg (3–5) / render parity (1). §3, §4, §5 and the adopt half of §6 are plans 2–3.
- Placeholder scan: none; the two "if X, do Y" notes (Task 7 order, Task 12 zero-padding) name the exact check and the exact fallback.
- Type consistency: `Mode`, `Prepared`, `band`, `espritResult`, `ModesResult`, `Strike`, `DecayLaw`, `Residual`, `NoiseBand`, `Analysis` are defined once and used with the same field names throughout; `weightedLine` (Task 10) is reused by `logLine` (Task 11).

## Execution notes (2026-10-12, plan 1 executed inline)

The code blocks above are synced to what shipped. Deviations from the first
draft, each forced by a failing test or by the local VCSL corpus:

- Task 3: `EigHermitian` is Householder tridiagonalisation + implicit QL; cyclic
  Jacobi (0.25 s at 128 × 128, ~6 s per instrument) became the test oracle
  `eigHermitianJacobi` (`TestEigHermitianMatchesJacobi`).
- Task 6: floor = pre-roll median when ≥ 20 ms of pre-roll, else the lower of
  the quietest non-zero 2 ms frame and a spectral floor; onset threshold also
  ≥ −40 dB re the peak; window start = 5 ms after the envelope first comes
  within 3 dB of its early maximum.
- Task 7/8: ESTER trim replaced by orthogonal matching pursuit (`selectPoles`);
  amplitudes come from one joint truncated-eigen least-squares fit from the
  onset (`refitAmplitudes`); modes needing > 40 dB of back-extrapolation are
  dropped; per-band rejection counters and pre-pursuit order in `BandDiag`.
- Task 9: the two-mode expectation is the ENERGY-weighted mean (8/3), not 3.
- Task 11: the two-rate fit is a model-weighted (IRLS) grid fit; the attack is
  the first frame within 3 dB of the smoothed peak; the test is statistical
  (10 seeds, 8 kHz band) because a single draw cannot beat ~1/sqrt(B·τ).
- Task 12: `peakNear` uses the largest power-of-two prefix (MagnitudeSpectrum
  windows the whole zero-padded buffer); the cross-check tolerance is one
  STFT bin.
- Task 13: `wavref.Root()` (default `~/.beatmo/wav-refs`) instead of
  `wavref.Dir()`; `-cpuprofile` flag.
