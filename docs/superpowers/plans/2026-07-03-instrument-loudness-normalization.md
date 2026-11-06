# Instrument Loudness Normalization + Headroom Defaults — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every instrument sit at a similar perceived loudness (measured, in dB) and start fresh projects at 50% instrument + 50% master volume so users have room to grow.

**Architecture:** A one-time offline measurement command renders every instrument through the real C DSP engine, measures each instrument's crest factor (RMS-to-peak, scale-invariant), and computes a per-instrument playback amplitude that lands each instrument at a common RMS target. Those amplitudes are emitted into **two generated files from one source of truth** — `instrument_loudness_gen.go` (consumed by the native Go apply path) and `instrument_loudness.gen.js` (consumed by the browser apply path) — overlaid at the single amplitude-lookup chokepoint on each platform. Fresh row and master volumes default to 0.5.

**Tech Stack:** Go 1.23 (`.tools/go/bin/go`), C DSP (`libdrums.a`), JavaScript/WebAudio (`audio.js`), Playwright browser tests.

## Global Constraints

- **Bundled Go only:** all Go commands use `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` (aliased below as `$GO`). Set `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go` for browser tests.
- **Fast Go test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
- **DO NOT COMMIT OR STAGE.** The user's standing rule ([[feedback_never_stage_or_commit]]) forbids `git add`/`git commit`. Every task ends at a **review checkpoint**, not a commit. Where a normal plan says "commit", here: stop, summarize the diff, and let the user review. Leave all work uncommitted.
- **Uniform strategy — no per-instrument or per-category special-casing.** The same formula and the same `L`/`peakCap` apply to every instrument. Kicks are NOT special-cased; they were only the user's example of a quiet instrument.
- **Loudness metric = flat RMS in dBFS over the loudest window.** NOT K-weighted LUFS (it de-emphasizes low frequencies and would penalise kicks).
- **The measurement tool must be a production build** (`!test`, no `-tags test`): under `-tags test` the synth is a pure-Go approximation, not the real C engine.
- **Do not change instrument timbre, durations, seed `"gain"`, the DSP, or the real-time audio path.** Amplitude and default volumes only.
- **Saved projects are sacred:** import restores stored volumes; the 50% defaults apply to fresh sessions only.

## File Structure

| File | New/Modify | Responsibility |
|---|---|---|
| `src/go/cmd/measure-loudness/main.go` | Create | Offline measurement + generator. Renders every instrument (real DSP), measures dB, computes amplitudes, emits both generated files + a before/after dB report. |
| `src/go/internal/audio/instrument_loudness_gen.go` | Create (generated) | `var instrumentLoudnessAmp map[string]float32` — the Go source of truth, no build tags. |
| `src/js/instrument_loudness.gen.js` | Create (generated) | `export const INSTRUMENT_LOUDNESS_AMP` — the JS source of truth. |
| `src/go/internal/audio/loudness_amp.go` | Create | `loudnessAmp(id)` helper (build-tag-free) + overlay logic shared documentation. |
| `src/go/internal/audio/config.go` | Modify (:100 `ConfigForInstrument`) | Overlay `loudnessAmp` onto returned `Amplitude`. |
| `src/go/internal/audio/config_stub.go` | Modify (:50 `ConfigForInstrument`) | Same overlay. |
| `src/go/internal/audio/config_wasm.go` | Modify (:50 `ConfigForInstrument`) | Same overlay. |
| `src/js/audio.js` | Modify (after :1429) | Apply `INSTRUMENT_LOUDNESS_AMP` over `RENDER_INFO` (replacement form for frozen modular entries). |
| `src/go/internal/audio/loudness_amp_parity_test.go` | Create | Go↔JS amp value-parity guard + "map covers all instruments" guard. |
| `src/go/internal/ui/drumview_ctor.go` | Modify (:182) | Fresh initial row `Volume: 1 → 0.5`. |
| `src/go/internal/ui/drumview_rows.go` | Modify (:119) | Runtime-added row `Volume: 1 → 0.5`. |
| `src/go/internal/audio/channels.go` | Modify (:541 `reset`) | Master channel default volume `1 → 0.5`. |
| `src/go/internal/ui/volume_defaults_test.go` | Create | Fresh row + master default 0.5; import still restores stored values. |
| `Makefile` | Modify (near :105 `gen-modular-instruments`) | Add `measure-loudness` target. |

---

## Task 1: Loudness measurement + generator command

Builds `cmd/measure-loudness`: renders every instrument through the real DSP engine, measures per-instrument dB, computes loudness-normalized amplitudes (auto-calibrated so the *median* instrument keeps ~its current level), prints a before/after dB report, and writes the two generated files.

**Files:**
- Create: `src/go/cmd/measure-loudness/main.go`
- Create (generated output): `src/go/internal/audio/instrument_loudness_gen.go`, `src/js/instrument_loudness.gen.js`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `audio.InstrumentConfigs` (map of all playable ids + current `.Amplitude`), `audio.RecipeForInstrument`, `synthmatch.RenderWithParams(id string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error)`, `wave.NewPeakRMSObserver().Observe(wave.Wave) wave.Observation` (fields `Peak, RMS, PeakDB, RMSDB`), `fingerprint.AutoSegment(w wave.Wave, durSec float64) wave.Wave`.
- Produces: `var instrumentLoudnessAmp = map[string]float32{...}` in package `audio`; `export const INSTRUMENT_LOUDNESS_AMP = Object.freeze({...})` in JS.

**Key math (scale-invariant — works whether the render is raw or normalized):**
```
crest_i    = peak_i / rms_i          (rms over the loudest window)
rmsNorm_i  = rms_i / peak_i = 1/crest_i
L          = median(rmsNorm) * 0.8   (auto-calibrate: median instrument → amp 0.8)
amp_i      = clamp(L / rmsNorm_i, 0, peakCap)   (peakCap default 1.0)
```
`amp_i` is the post-peak-normalization scale, so applied-peak == `amp_i` and applied-RMS == `amp_i * rmsNorm_i`. Report current-applied dB = `20*log10(currentAmp_i * rmsNorm_i)` and projected dB = `20*log10(amp_i * rmsNorm_i)`.

- [ ] **Step 1: Scaffold the command with the render+measure core**

Create `src/go/cmd/measure-loudness/main.go`:

```go
// Command measure-loudness renders every instrument through the real C DSP
// engine, measures its crest factor (scale-invariant RMS-to-peak), and computes
// a per-instrument playback amplitude that lands each instrument at a common
// perceived-loudness (flat-RMS) target. It prints a before/after dB report and
// (with -write) emits the two generated amplitude files.
//
// MUST be built as a production binary (NOT -tags test): the test build swaps
// in a pure-Go approximation synth instead of the real engine.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const sampleRate = 48000

type row struct {
	id         string
	rmsNorm    float64 // rms/peak over loudest window (== 1/crest)
	currentAmp float64
	newAmp     float64
}

// measureRMSNorm renders one instrument and returns rms/peak over its loudest
// window. Returns ok=false when the instrument cannot be rendered or is silent.
func measureRMSNorm(id string, durSec float64) (float64, bool) {
	w, err := synthmatch.RenderWithParams(id, 0, sampleRate, durSec, nil)
	if err != nil || len(w.Samples) == 0 {
		return 0, false
	}
	seg := fingerprint.AutoSegment(w, 0.3) // loudest 300ms window
	obs := wave.NewPeakRMSObserver().Observe(seg)
	if obs.Peak <= 1e-9 || obs.RMS <= 1e-12 {
		return 0, false
	}
	return obs.RMS / obs.Peak, true
}

func main() {
	peakCap := flag.Float64("peakcap", 1.0, "maximum post-normalization amplitude")
	write := flag.Bool("write", false, "write the generated Go + JS files")
	flag.Parse()

	ids := make([]string, 0, len(audio.InstrumentConfigs))
	for id := range audio.InstrumentConfigs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var rows []row
	for _, id := range ids {
		cfg := audio.ConfigForInstrument(id)
		rn, ok := measureRMSNorm(id, cfg.DurationSec)
		if !ok {
			fmt.Fprintf(os.Stderr, "skip %s: unrenderable/silent\n", id)
			continue
		}
		rows = append(rows, row{id: id, rmsNorm: rn, currentAmp: float64(cfg.Amplitude)})
	}

	// Auto-calibrate L so the MEDIAN instrument keeps amplitude ~0.8.
	norms := make([]float64, len(rows))
	for i, r := range rows {
		norms[i] = r.rmsNorm
	}
	sort.Float64s(norms)
	medianNorm := norms[len(norms)/2]
	L := medianNorm * 0.8

	for i := range rows {
		amp := L / rows[i].rmsNorm
		if amp > *peakCap {
			amp = *peakCap
		}
		if amp < 0 {
			amp = 0
		}
		rows[i].newAmp = amp
	}

	printReport(rows, L, *peakCap)
	if *write {
		writeGo(rows)
		writeJS(rows)
	}
}

func db(x float64) float64 {
	if x <= 1e-9 {
		return -120
	}
	return 20 * math.Log10(x)
}
```

- [ ] **Step 2: Add the dB report printer**

Append to `main.go`:

```go
func printReport(rows []row, L, peakCap float64) {
	fmt.Printf("# loudness measurement  L=%.4f  peakCap=%.2f  n=%d\n", L, peakCap, len(rows))
	fmt.Printf("%-22s %10s %10s %10s %10s\n", "instrument", "curRMSdB", "newRMSdB", "curAmp", "newAmp")
	var before, after []float64
	for _, r := range rows {
		curDB := db(r.currentAmp * r.rmsNorm)
		newDB := db(r.newAmp * r.rmsNorm)
		before = append(before, curDB)
		after = append(after, newDB)
		fmt.Printf("%-22s %10.2f %10.2f %10.3f %10.3f\n", r.id, curDB, newDB, r.currentAmp, r.newAmp)
	}
	fmt.Printf("# spread (max-min RMS dB): before=%.2f  after=%.2f\n", spread(before), spread(after))
}

func spread(v []float64) float64 {
	lo, hi := v[0], v[0]
	for _, x := range v {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	return hi - lo
}
```

- [ ] **Step 3: Add the two file emitters**

Append to `main.go` (paths are relative to `src/go/`, i.e. run from that dir):

```go
func writeGo(rows []row) {
	var b []byte
	b = append(b, []byte("// Code generated by cmd/measure-loudness. DO NOT EDIT.\n"+
		"// Regenerate: cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write\n"+
		"// Per-instrument playback amplitude, loudness-normalized (flat-RMS) so every\n"+
		"// instrument sits at a common perceived loudness. See\n"+
		"// docs/superpowers/specs/2026-07-03-instrument-loudness-normalization-design.md\n\n"+
		"package audio\n\n"+
		"var instrumentLoudnessAmp = map[string]float32{\n")...)
	for _, r := range rows {
		b = append(b, []byte(fmt.Sprintf("\t%q: %s,\n", r.id, formatAmp(r.newAmp)))...)
	}
	b = append(b, []byte("}\n")...)
	must(os.WriteFile("internal/audio/instrument_loudness_gen.go", b, 0o644))
}

func writeJS(rows []row) {
	var b []byte
	b = append(b, []byte("// Code generated by cmd/measure-loudness. DO NOT EDIT.\n"+
		"// Regenerate: cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write\n"+
		"export const INSTRUMENT_LOUDNESS_AMP = Object.freeze({\n")...)
	for _, r := range rows {
		b = append(b, []byte(fmt.Sprintf("  %q: %s,\n", r.id, formatAmp(r.newAmp)))...)
	}
	b = append(b, []byte("});\n")...)
	must(os.WriteFile("../js/instrument_loudness.gen.js", b, 0o644))
}

// formatAmp emits the float32 value both files share, identically formatted, so
// the Go↔JS parity guard is a clean numeric match.
func formatAmp(v float64) string {
	return fmt.Sprintf("%.4f", float64(float32(v)))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
```

- [ ] **Step 4: Add the Makefile target**

In `Makefile`, next to the `gen-modular-instruments` target (~:105), add:

```makefile
.PHONY: measure-loudness
measure-loudness: ## Measure + regenerate per-instrument loudness amplitudes
	cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write
	@echo "regenerated instrument_loudness_gen.go + instrument_loudness.gen.js"
```

- [ ] **Step 5: Build and run the report (dry, no -write) to verify it renders the real engine**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness
```
Expected: a table of ~52 instruments with `curRMSdB`/`newRMSdB`/`curAmp`/`newAmp`, ending with a `# spread` line. The **before** spread should be wide (many dB); the **after** spread should be small.

If some instruments print `skip … unrenderable/silent` on stderr, note which — legacy bespoke drums (snare/kick/hihat/tom/clap/cowbell/rimshot/sidestick/kick-deep/shaker/ride/crash/fm-*) may not resolve through `synthmatch` (which uses `RecipeForInstrument`). If any are skipped, extend `measureRMSNorm` with an `audio.ExportVoice(id, 120, sampleRate)` fallback that fills a `[]float64` via the voice `.Sample()` loop for `cfg.DurationSec*sampleRate` samples, wrap it in a `wave.Wave`, and measure the same way (RMS/peak is scale-invariant, so the already-normalized voice output yields the same `rmsNorm`). Re-run until every instrument is covered (count == `len(InstrumentConfigs)`).

- [ ] **Step 6: Tune `peakCap` and confirm a tight after-spread**

Inspect the report. Target: after-spread ≤ ~4 dB (≈ ±2 dB band). High-crest instruments pinned at `peakCap` are expected and uniform (they physically can't reach `L` without clipping). If the after-spread is dominated by a cluster hitting `peakCap`, that is acceptable; confirm the *typical* (non-capped) instruments cluster within ~±2 dB. Note the chosen `-peakcap` value (default 1.0; try 0.95 if you want more safety margin) in the checkpoint summary.

- [ ] **Step 7: Write the generated files**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write
```
Expected: creates `src/go/internal/audio/instrument_loudness_gen.go` and `src/js/instrument_loudness.gen.js`. Verify both exist and contain one entry per measured instrument:
```bash
head -20 /home/ymolinar/Repos/beatmo/src/go/internal/audio/instrument_loudness_gen.go
head -20 /home/ymolinar/Repos/beatmo/src/js/instrument_loudness.gen.js
```

- [ ] **Step 8: Checkpoint (do NOT commit)**

Summarize for review: the before/after dB spread numbers, the chosen `peakCap`, any instruments pinned at the cap, and confirmation both files were written. Leave uncommitted.

---

## Task 2: Wire the generated amplitude into the Go apply path

Overlay `instrumentLoudnessAmp` onto every `ConfigForInstrument` result so the native Go `normalizeAndScale` path (and any Go reader) uses the measured amplitude. One helper + three identical small edits.

**Files:**
- Create: `src/go/internal/audio/loudness_amp.go`
- Modify: `src/go/internal/audio/config.go:100-109`, `config_stub.go:50-59`, `config_wasm.go:50-59`
- Test: `src/go/internal/audio/loudness_amp_test.go` (create)

**Interfaces:**
- Consumes: `instrumentLoudnessAmp` (Task 1).
- Produces: `loudnessAmp(id string) (float32, bool)`; `ConfigForInstrument(id)` now returns loudness-normalized `.Amplitude`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/audio/loudness_amp_test.go`:

```go
package audio

import "testing"

func TestConfigForInstrumentUsesLoudnessAmp(t *testing.T) {
	// Pick any instrument that has a generated loudness amplitude.
	var id string
	var want float32
	for k, v := range instrumentLoudnessAmp {
		id, want = k, v
		break
	}
	if id == "" {
		t.Fatal("instrumentLoudnessAmp is empty — run `make measure-loudness`")
	}
	got := ConfigForInstrument(id).Amplitude
	if got != want {
		t.Errorf("ConfigForInstrument(%q).Amplitude = %v, want loudness amp %v", id, got, want)
	}
}

func TestLoudnessAmpFallsBackWhenAbsent(t *testing.T) {
	if _, ok := loudnessAmp("definitely-not-an-instrument"); ok {
		t.Error("loudnessAmp returned ok for an unknown id")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestConfigForInstrumentUsesLoudnessAmp|TestLoudnessAmpFallsBackWhenAbsent' ./internal/audio/
```
Expected: FAIL — `loudnessAmp` undefined (compile error).

- [ ] **Step 3: Add the helper**

Create `src/go/internal/audio/loudness_amp.go`:

```go
package audio

// loudnessAmp returns the loudness-normalized playback amplitude for an
// instrument id and whether a measured value exists. The values are generated
// offline by cmd/measure-loudness (instrument_loudness_gen.go) so that every
// instrument sits at a common perceived loudness. Build-tag-free: it is the
// single amplitude source of truth across desktop, test, and wasm Go builds.
func loudnessAmp(id string) (float32, bool) {
	v, ok := instrumentLoudnessAmp[id]
	return v, ok
}
```

- [ ] **Step 4: Overlay in all three `ConfigForInstrument` variants**

In `src/go/internal/audio/config.go`, replace the body of `ConfigForInstrument` (currently :100-109):

```go
func ConfigForInstrument(id string) InstrumentConfig {
	cfg, ok := InstrumentConfigs[id]
	if !ok {
		cfg = InstrumentConfig{
			ID:          id,
			DurationSec: DefaultDurationSec,
			Amplitude:   DefaultAmplitude,
		}
	}
	if amp, ok := loudnessAmp(id); ok {
		cfg.Amplitude = amp
	}
	return cfg
}
```

Apply the identical replacement in `src/go/internal/audio/config_stub.go:50-59` and `src/go/internal/audio/config_wasm.go:50-59` (same function body verbatim).

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestConfigForInstrumentUsesLoudnessAmp|TestLoudnessAmpFallsBackWhenAbsent' ./internal/audio/
```
Expected: PASS.

- [ ] **Step 6: Confirm the package still builds under all three tag sets**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go build ./internal/audio/ && \
  ../../.tools/go/bin/go build -tags test ./internal/audio/ && \
  GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags "js wasm" ./internal/audio/
```
Expected: all three build with no output.

- [ ] **Step 7: Checkpoint (do NOT commit)** — summarize the overlay wiring for review.

---

## Task 3: Wire the generated amplitude into the browser + add the Go↔JS parity guard

Apply `INSTRUMENT_LOUDNESS_AMP` over `RENDER_INFO` in `audio.js`, and add a Go test that parses the generated JS file and asserts its values equal the Go map (a real amp-parity guard that does not exist today).

**Files:**
- Modify: `src/js/audio.js` (after :1429, the `Object.assign(RENDER_INFO, MODULAR_RENDER_INFO)` line)
- Create: `src/go/internal/audio/loudness_amp_parity_test.go`

**Interfaces:**
- Consumes: `INSTRUMENT_LOUDNESS_AMP` (Task 1), `instrumentLoudnessAmp` (Task 1), `builtinInstrumentRecipeBindings` / `InstrumentConfigs` (for coverage).
- Produces: browser `RENDER_INFO[id].amp` == the measured amp for every instrument.

- [ ] **Step 1: Import the generated map in audio.js**

Near the existing generated import (`src/js/audio.js:29`):
```js
import { MODULAR_RENDER, MODULAR_RENDER_INFO } from './modular_instruments.gen.js';
```
add:
```js
import { INSTRUMENT_LOUDNESS_AMP } from './instrument_loudness.gen.js';
```

- [ ] **Step 2: Apply the override after RENDER_INFO is assembled**

Immediately after `Object.assign(RENDER_INFO, MODULAR_RENDER_INFO);` (`audio.js:1429`), add:

```js
// Loudness normalization: override each instrument's amp with the measured,
// loudness-normalized value so browser output matches native Go (which applies
// the same value via ConfigForInstrument). Modular entries come from a frozen
// generated object, so REPLACE the entry (spread) rather than mutate .amp.
for (const [id, amp] of Object.entries(INSTRUMENT_LOUDNESS_AMP)) {
  if (RENDER_INFO[id]) {
    RENDER_INFO[id] = { ...RENDER_INFO[id], amp };
  }
}
```

- [ ] **Step 3: Write the failing parity guard**

Create `src/go/internal/audio/loudness_amp_parity_test.go`:

```go
package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// parseJSAmpMap parses `src/js/instrument_loudness.gen.js` entries of the form
//   "id": 0.1234,
func parseJSAmpMap(t *testing.T) map[string]float32 {
	t.Helper()
	// test cwd is the package dir: internal/audio → repo/src/js.
	path := filepath.Join("..", "..", "..", "src", "js", "instrument_loudness.gen.js")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated JS: %v (run `make measure-loudness`)", err)
	}
	re := regexp.MustCompile(`"([^"]+)":\s*([0-9.]+)`)
	out := map[string]float32{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		f, err := strconv.ParseFloat(m[2], 32)
		if err != nil {
			t.Fatalf("parse %q: %v", m[2], err)
		}
		out[m[1]] = float32(f)
	}
	return out
}

func TestLoudnessAmpGoJSParity(t *testing.T) {
	js := parseJSAmpMap(t)
	if len(js) != len(instrumentLoudnessAmp) {
		t.Fatalf("entry count mismatch: go=%d js=%d — regenerate with `make measure-loudness`",
			len(instrumentLoudnessAmp), len(js))
	}
	for id, goAmp := range instrumentLoudnessAmp {
		if jsAmp, ok := js[id]; !ok || jsAmp != goAmp {
			t.Errorf("amp mismatch for %q: go=%v js=%v (ok=%v)", id, goAmp, jsAmp, ok)
		}
	}
}

func TestLoudnessAmpCoversAllInstruments(t *testing.T) {
	for id := range InstrumentConfigs {
		if _, ok := instrumentLoudnessAmp[id]; !ok {
			t.Errorf("instrument %q has no loudness amp — run `make measure-loudness`", id)
		}
	}
}
```

- [ ] **Step 4: Run the parity guard**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLoudnessAmp' ./internal/audio/
```
Expected: PASS (Go and JS files were generated from the same run in Task 1, so values match; `TestLoudnessAmpCoversAllInstruments` confirms none were skipped).

If `TestLoudnessAmpCoversAllInstruments` fails, an instrument was skipped during measurement (Task 1 Step 5) — fix the `ExportVoice` fallback so every id is covered, re-run `make measure-loudness`, and re-run this test.

- [ ] **Step 5: Checkpoint (do NOT commit)** — summarize the browser override + parity guard.

---

## Task 4: Lower default volumes to 50%

Fresh initial row, runtime-added rows, and the master channel all start at 0.5. Import continues to restore stored values.

**Files:**
- Modify: `src/go/internal/ui/drumview_ctor.go:182`, `src/go/internal/ui/drumview_rows.go:119`, `src/go/internal/audio/channels.go:541`
- Test: `src/go/internal/ui/volume_defaults_test.go` (create)

**Interfaces:**
- Consumes: `audio.MainVolume() float64`, `audio.SetMainVolume(float64)`, `channelManager.reset()`.
- Produces: fresh `DrumRow.Volume == 0.5`; fresh master channel volume == 0.5.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/volume_defaults_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestFreshInitialRowVolumeIs50(t *testing.T) {
	dv := newTestDrumView(t) // existing helper used across the ui suite
	if got := dv.Rows[0].Volume; got != 0.5 {
		t.Errorf("fresh initial row Volume = %v, want 0.5", got)
	}
}

func TestAddedRowVolumeIs50(t *testing.T) {
	dv := newTestDrumView(t)
	before := len(dv.Rows)
	dv.AddRow()
	if len(dv.Rows) != before+1 {
		t.Fatalf("AddRow did not append a row")
	}
	if got := dv.Rows[len(dv.Rows)-1].Volume; got != 0.5 {
		t.Errorf("added row Volume = %v, want 0.5", got)
	}
}

func TestFreshMasterVolumeIs50(t *testing.T) {
	audio.ResetInstruments() // rebuilds channels via channelManager.reset()
	if got := audio.MainVolume(); got != 0.5 {
		t.Errorf("fresh master volume = %v, want 0.5", got)
	}
}
```

Note: confirm the exact fresh-DrumView test constructor name used elsewhere in the `ui` package (search `func newTestDrumView` / `newDrumViewForTest` in `internal/ui/*_test.go`) and the exact channel-reset entry point (`audio.ResetInstruments` or equivalent) before running; substitute the real names if they differ. Do not invent a helper — reuse the one the suite already uses.

- [ ] **Step 2: Run the test to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestFreshInitialRowVolumeIs50|TestAddedRowVolumeIs50|TestFreshMasterVolumeIs50' ./internal/ui/
```
Expected: FAIL — volumes are 1, want 0.5.

- [ ] **Step 3: Change the initial row default**

In `src/go/internal/ui/drumview_ctor.go:182`, change `Volume: 1` to `Volume: 0.5` in the initial `DrumRow` literal (the `dv.Rows = []*DrumRow{{...}}` construction).

- [ ] **Step 4: Change the runtime-added row default**

In `src/go/internal/ui/drumview_rows.go:119`, change `Volume: 1` to `Volume: 0.5` in the `append(dv.Rows, &DrumRow{...})` construction inside `AddRow`.

- [ ] **Step 5: Change the master channel default**

In `src/go/internal/audio/channels.go`, in `channelManager.reset()` right after the main channel is created at :541:
```go
	cm.main = newChannel(mainChannelID, nil)
```
add, on the next line:
```go
	cm.main.SetVolume(0.5) // fresh-session master default (see loudness-normalization plan)
```
Do NOT change `newChannel` itself (:83-87) — it is shared with instrument channels, which must stay 1.0.

- [ ] **Step 6: Run the tests to verify they pass**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestFreshInitialRowVolumeIs50|TestAddedRowVolumeIs50|TestFreshMasterVolumeIs50' ./internal/ui/
```
Expected: PASS.

- [ ] **Step 7: Verify import still restores stored volumes (regression)**

The import path (`import.go:446-452` for rows, `:623-635` for master) explicitly restores stored values, overriding the fresh default. Leave the legacy branch at `import.go:632` (`audio.SetMainVolume(1)` for files that omit `MasterVolume`) as-is — legacy files should load at their historical level, not the new fresh default. Run the existing import suite to confirm nothing regressed:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Import' ./internal/ui/
```
Expected: PASS.

- [ ] **Step 8: Checkpoint (do NOT commit)** — summarize the four default changes + import-safety confirmation.

---

## Task 5: Fix fallout in existing tests + full fast-suite green

Changing amplitudes and default volumes will break tests that assert the old flat 0.8 amplitude, absolute peak/RMS levels, or default volume 1.0. Find and update them so the fast suite is green.

**Files:**
- Modify (as surfaced by test runs): `src/go/internal/audio/all_instruments_pitch_table_test.go`, `instrument_seeds_test.go`, and any `internal/ui` test asserting default volume `1.0`.
- Cross-platform goldens: `xplat_audio_compare` / `ifx_xplat_golden` regeneration as needed.

**Interfaces:** none new — this task only updates assertions/goldens to match the new, intended behavior.

- [ ] **Step 1: Run the full fast Go suite and collect failures**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... 2>&1 | tee /tmp/claude-1000/-home-ymolinar-Repos-beatmo/a777727d-a77b-4010-9741-bddde2fe1cdb/scratchpad/loudness_fastsuite.log | tail -40
```
Expected: a list of failures concentrated in `internal/audio` (amplitude/level assertions) and `internal/ui` (default volume assertions).

- [ ] **Step 2: Triage each failure into one of two buckets**

For each failing test, decide:
- **Assertion-on-old-behavior** (asserts amp==0.8, a specific peak/RMS number, or default volume==1.0): update the expected value to the new intended value. For amplitude, read the value from `instrumentLoudnessAmp`; for default volume, change `1.0` → `0.5`.
- **Genuine invariant** (finite, non-silent, non-clipping, monotonic decay): should STILL pass. If it fails, that is a real regression — investigate before changing the test. Do NOT weaken an invariant to force green.

Document the bucket for each touched test in the checkpoint summary.

- [ ] **Step 3: Update the known level-sensitive audio tests**

`all_instruments_pitch_table_test.go` (peak inline at ~:59) and `instrument_seeds_test.go` (peak ~:23, tail/first-half RMS ~:130/:146/:314) assert bounded/non-silent levels. Loudness normalization keeps signals finite and non-silent but changes absolute peaks. Where these assert a hard numeric peak/RMS ceiling or floor, widen/retune to the new levels (keep them as real bounds, not tautologies). Re-run each after editing:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'AllMelodicInstruments|InstrumentDefaults|Seeds' ./internal/audio/
```

- [ ] **Step 4: Regenerate cross-platform audio goldens if the xplat compare fails**

Native and browser now apply the *same* measured amp, so xplat parity should hold. If `render_table` / insert-fx golden tests fail on absolute values, regenerate per the documented mechanism (e.g. `IFX_GOLDEN_UPDATE=1` for `ifx_xplat_golden.json`, per CLAUDE.md). Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/... 2>&1 | tail -20
```
Expected: PASS after regeneration.

- [ ] **Step 5: Confirm the whole fast suite is green**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```
Expected: `ok` for all packages (no `FAIL`).

- [ ] **Step 6: Checkpoint (do NOT commit)** — list every test file touched with its bucket, and confirm the full fast suite is green.

---

## Task 6: WASM rebuild + cross-platform + real-app verification

Rebuild the browser bundle (regenerates modular gen + compiles wasm), verify cross-platform audio parity holds with the new amplitudes, and do an end-to-end ear/level check.

**Files:** none (verification only).

- [ ] **Step 1: Rebuild the WASM bundle**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && make wasm
```
Expected: builds `src/js/main.wasm`; also regenerates `modular_instruments.gen.js` (unchanged — modular amplitudes still 0.8, overridden at runtime by `INSTRUMENT_LOUDNESS_AMP`). If `TestModularInstrumentsGenFileIsCurrent`-style drift appears, run `make gen-modular-instruments`.

- [ ] **Step 2: Cross-platform audio parity (native Go vs WASM Go)**

Run:
```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
```
Expected: PASS — native and browser produce matching output (both apply the same measured amp).

- [ ] **Step 3: Browser mixer/EQ parity smoke**

Run:
```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/webaudio_mixer_eq_parity.browser.test.js
```
Expected: PASS.

- [ ] **Step 4: Ear/level check via the verify skill**

Invoke the `verify` skill (or `make run` desktop + a browser run) to drive playback of a circuit that mixes a previously-quiet instrument (e.g. a kick) with a previously-loud one (e.g. organ/sax) at the 50% defaults. Confirm by ear + level meter (Levels tab): instruments are balanced, the previously-quiet instrument is now present, and pushing master/instrument to 100% engages the limiter without harsh digital clipping (no sustained hard-clamp).

- [ ] **Step 5: Final checkpoint (do NOT commit)**

Summarize: fast suite green, xplat parity green, WASM rebuilt, and the ear/level result. Present the full uncommitted diff summary for the user's review.

---

## Self-Review Notes (coverage against the spec)

- Spec §1 measurement tool → Task 1 (real DSP, dBFS, both generated files). ✅
- Spec §2 gain formula (`clamp(L/RMS_norm, 0, peakCap)`) → Task 1 Step 1-3 (auto-calibrated `L` = median·0.8). ✅
- Spec §3 uniform strategy, no kick special-casing → Global Constraints + Task 1 (single `L`/`peakCap`). ✅
- Spec §4 headroom defaults 50% (row + master, slider stays 0–100%) → Task 4. ✅ (Slider range untouched — no code change needed; `Slider` is a 0..1 control, 50% = mid.)
- Spec §5 not-changed (seed gain, DSP, saved projects) → no tasks touch them; Task 4 Step 7 confirms import safety. ✅
- Spec §6 verification + guard test + known fallout → Task 3 (parity guard), Task 5 (fallout), Task 6 (xplat + ear). ✅
- Decision vs spec: the spec floated a byte-diff regen guard; this plan uses a cheaper **Go↔JS value-parity guard** (Task 3) instead of re-measuring on every test run (real DSP render is too slow/heavy for a unit test). Regeneration stays a manual `make measure-loudness` step. This is an intentional, documented divergence.
