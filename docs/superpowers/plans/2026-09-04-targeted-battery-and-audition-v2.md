# Targeted Battery, Calibration, Ablation and Audition v2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the analysis battery see the eight failing instrument families (f0-free modes, resolved decay, modulation, decorrelation, onset), calibrate it against every label the listener has given, add stage ablation, and ship an audition tool that plays reference + current + two candidates and asks targeted per-family questions.

**Architecture:** New measurements are registry rows in `internal/audio/fingerprint` fed by one new memoized `Bundle` artifact (the mode tracker). Calibration lives in `cmd/refs` and writes two embedded JSON goldens. Ablation is a new `internal/audio/synthdiag` package (CGo, `!test && !js`). The session tool is a new `cmd/refs session` subcommand with an in-Go blocking player.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), CGo modular synth (`build/libdrums.a`), `internal/wave` FFT, `paplay`/`aplay`/`ffplay` for playback.

**Spec:** `docs/superpowers/specs/2026-09-04-targeted-battery-and-audition-v2-design.md`

## Global Constraints

- **Never commit or stage.** Standing user rule: leave every file uncommitted for the user to review. There are no commit steps in this plan.
- **Never bare `./...`.** Name packages: `./internal/audio/fingerprint/`, `./cmd/refs/`, `./internal/audio/synthdiag/`, `./internal/audio/`.
- **Fast path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run <Name> <pkg>`. The C synth renders **silence** under `-tags test`; any test that needs real audio is `//go:build !test && !js` and runs as `cd src/go && ../../.tools/go/bin/go test -run <Name> <pkg>` (no tags) after `make build/libdrums.a`.
- **Corpus:** tools that need reference audio require `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs` and `AUDIO_SAMPLE_RATE=48000`. Tests needing it must `t.Skip` when the directory is absent.
- **`fingerprint` imports only `internal/wave` + stdlib.** Never `internal/audio`. Only untagged `go vet ./internal/audio/fingerprint/` catches a violation.
- **Build the CLI into the scratchpad**, not `tmp/` (that mount is noexec):
  `cd src/go && ../../.tools/go/bin/go build -o "$SCRATCH/refs" ./cmd/refs` where `SCRATCH=/tmp/claude-1000/-home-ymolinar-Repos-beatmo/7e9613ac-0035-4e7e-9d47-618d0f3127f1/scratchpad`.
- **Registry conventions** (from phase 1–3): every `Descriptor` has non-empty `ID`, `Group`, `Name`, `Doc`, `Applies`, `Extract`. Registration order == report order, so file names sort in the narrative order (`registry_core.go` … ). `KindRate` has no default thresholds — every `KindRate` row sets `Drift`/`Mismatch`/`Unit`.
- **`Unavailable(reason)` never a fabricated zero.**
- **Existing gate must stay green:** `TestDiagnose_ReproducesListenerVerdicts`, `TestDiagnose_ReproducesBonusVerdicts` (3/5 pre-existing), `TestDiagnoseClassifierReplay*` in `cmd/refs`.

---

## File structure

| File | Responsibility |
|---|---|
| `internal/audio/fingerprint/modes.go` | Mode tracker: `Mode`, `ModeSet`, `AnalyzeModes`, `(*Bundle).Modes()`, `RequiresModes` |
| `internal/audio/fingerprint/registry_modes.go` | Group `modal structure` rows (§1.2) |
| `internal/audio/fingerprint/decay_resolved.go` | `BandT60`, per-mode decay helpers |
| `internal/audio/fingerprint/registry_decay.go` | Group `resolved decay` rows (§1.3); demotes `env.ring_t60` |
| `internal/audio/fingerprint/modulation.go` | `ModulationSpectrum` |
| `internal/audio/fingerprint/registry_modulation.go` | `mod.am.*`, `mod.centroid.depth` rows (§1.4) |
| `internal/audio/fingerprint/decorrelation.go` | `PartialBandwidthCents`, `PartialIndependence` |
| `internal/audio/fingerprint/registry_aliveness.go` | Group `partial decorrelation` rows (§1.5) |
| `internal/audio/fingerprint/onset.go` | onset + noise-character helpers |
| `internal/audio/fingerprint/registry_onset.go` | Group `onset character` + two `GroupNoise` rows (§1.6) |
| `internal/audio/fingerprint/organic.go` | `OrganicIDs`, `OrganicRange`, embedded `testdata/organic_range.json` |
| `internal/audio/fingerprint/thresholds.go` | embedded `testdata/thresholds.json`, applied lazily by the registry; `Descriptor.Demoted` |
| `internal/audio/fingerprint/descriptor.go` | + `Demoted bool` field |
| `internal/audio/fingerprint/registry.go` | + `ensureCalibrated()` |
| `internal/audio/synthdiag/ablate.go` | `Stages`, `Ablate`, `Report` |
| `internal/audio/synth_recipe_hash_export.go` | `InstrumentRecipeHash` |
| `cmd/refs/diagnose.go` | `axisResult.ID`; overrides plumbing; `-ablate` |
| `cmd/refs/compare.go` | `renderFamilyCompareOverrides` |
| `cmd/refs/organic.go` | `refs organic`, `refs calibrate-organic` |
| `cmd/refs/calibrate_thresholds.go` | `refs calibrate-thresholds` |
| `cmd/refs/session_record.go` | `earSession` types, read/append |
| `cmd/refs/session_candidates.go` | candidates JSON loader |
| `cmd/refs/session_questions.go` | per-family question table + adjectives |
| `cmd/refs/player.go` | blocking player detection |
| `cmd/refs/session.go` | `refs session` prompt flow |
| `cmd/refs/validate.go` | PREF column, STALE, sessions ingestion |
| `cmd/refs/validate_terms.go` | rebound `termRules` |
| `cmd/refs/audition_verdicts.go` | `RecipeSHA` on `earVerdict` |
| `cmd/refs/main.go` | dispatch new subcommands |
| `internal/audio/testdata/audition_candidates.json` | the six candidates |
| `internal/audio/testdata/ear_sessions.jsonl` | session records (created by the tool) |
| `Makefile` | `ab-session` target |

---

### Task 1: Mode tracker

**Files:**
- Create: `src/go/internal/audio/fingerprint/modes.go`
- Create: `src/go/internal/audio/fingerprint/modes_test.go`
- Modify: `src/go/internal/audio/fingerprint/bundle.go` (add `modesOnce`, `modes` fields + accessor)

**Interfaces:**
- Consumes: `(*Bundle).STFT(size, hop) *STFTFrames` (`Frames [][]float64` linear magnitude, `BinHz`, `HopSec`), `peakPickPartials` (kick_roughness.go:101, returns `[]spectralPartial{Freq, Amp}` strongest-first), `ampToDBFloor` (tracks.go:227), `olsSlope(xs, ys)` (signal.go:27).
- Produces:
  ```go
  const modeFFTSize = 8192
  const modeHop = 2048
  type Mode struct {
      Hz            float64   // amplitude-weighted mean frequency over the track
      PeakDB        float64   // loudest frame level, dBFS
      PeakFrame     int
      DecayDBPerSec float64   // OLS slope of dB vs seconds from PeakFrame to the last tracked frame (or first frame ≤ PeakDB-40)
      DecayOK       bool      // false when fewer than 3 frames were available for the fit
      Track         []float64 // dB per frame; math.NaN() where the mode was not present
  }
  type ModeSet struct {
      Modes     []Mode  // ascending Hz
      Anchor    int     // index into Modes; -1 when none
      HopSec    float64
      NumFrames int
  }
  func AnalyzeModes(st *STFTFrames) ModeSet
  func (m ModeSet) Strong(withinDB float64) []Mode       // modes whose PeakDB >= max(PeakDB) - withinDB, ascending Hz
  func (m ModeSet) Ratios(n int) []float64               // first n Strong(20) mode Hz divided by anchor Hz
  func (m ModeSet) AnchorHz() float64                    // 0 when Anchor < 0
  func (b *Bundle) Modes() *ModeSet
  func RequiresModes(b *Bundle) bool                     // RequiresAudible && len(Strong(20)) >= 2
  const naNoModes = "fewer than two stable spectral modes were tracked"
  ```

- [ ] **Step 1: Write the failing tests**

```go
// modes_test.go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// twoDecayingModes: 440 Hz decaying at -20 dB/s and 1215 Hz (2.76x, a free
// bar's second mode) decaying at -60 dB/s. Both start at equal amplitude.
func twoDecayingModes(sr int, dur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		a1 := math.Pow(10, -20*t/20)
		a2 := math.Pow(10, -60*t/20)
		s[i] = 0.4*a1*math.Sin(2*math.Pi*440*t) + 0.4*a2*math.Sin(2*math.Pi*1215*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestAnalyzeModes_FindsTwoModesWithRatioAndDecay(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 2.0), "bar")
	ms := b.Modes()
	strong := ms.Strong(20)
	if len(strong) != 2 {
		t.Fatalf("want 2 strong modes, got %d: %+v", len(strong), strong)
	}
	if math.Abs(strong[0].Hz-440) > 3 || math.Abs(strong[1].Hz-1215) > 6 {
		t.Fatalf("mode frequencies off: %.1f %.1f", strong[0].Hz, strong[1].Hz)
	}
	if r := ms.Ratios(2); math.Abs(r[1]-2.76) > 0.03 {
		t.Fatalf("ratio want 2.76, got %.3f", r[1])
	}
	if !strong[0].DecayOK || math.Abs(strong[0].DecayDBPerSec-(-20)) > 4 {
		t.Fatalf("mode 1 decay want -20 dB/s, got %.1f ok=%v", strong[0].DecayDBPerSec, strong[0].DecayOK)
	}
	if !strong[1].DecayOK || math.Abs(strong[1].DecayDBPerSec-(-60)) > 8 {
		t.Fatalf("mode 2 decay want -60 dB/s, got %.1f", strong[1].DecayDBPerSec)
	}
	if ms.AnchorHz() < 437 || ms.AnchorHz() > 443 {
		t.Fatalf("anchor want 440, got %.1f", ms.AnchorHz())
	}
}

func TestAnalyzeModes_NoiseHasNoStableModes(t *testing.T) {
	b := NewBundle(wave.Noise(7, 0.3, 1.5, 44100), "noise")
	if RequiresModes(b) && len(b.Modes().Strong(20)) > 6 {
		t.Fatalf("white noise should not yield many stable strong modes, got %d", len(b.Modes().Strong(20)))
	}
}

func TestAnalyzeModes_SilenceHasNoModes(t *testing.T) {
	b := NewBundle(wave.Silence(1.0, 44100), "silent")
	if RequiresModes(b) {
		t.Fatal("silence must not satisfy RequiresModes")
	}
	if b.Modes().Anchor != -1 {
		t.Fatalf("anchor want -1, got %d", b.Modes().Anchor)
	}
}

func TestModes_MemoizedOnce(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 1.0), "bar")
	p1 := b.Modes()
	p2 := b.Modes()
	if p1 != p2 {
		t.Fatal("Modes() must return the memoized pointer")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestAnalyzeModes|TestModes_' ./internal/audio/fingerprint/`
Expected: FAIL — `b.Modes undefined`.

- [ ] **Step 3: Implement `modes.go`**

```go
package fingerprint

import (
	"math"
	"sort"
)

// modes.go — the f0-free partial tracker.
//
// Every partial-structure descriptor before this file anchored on DetectF0.
// That is the wrong model for bars, bells, drums and cymbals: 19 of 59
// corpus references have no reliable fundamental, and on the ones that do
// (kalimba, tubular-bells) the two sides of a comparison landed on different
// pitch grids. A mode is simply a spectral peak that persists across frames;
// ratios are taken against the LOWEST prominent mode. Both sides of any
// comparison use the same rule, so a bell's hum partial anchors both.

const (
	modeFFTSize       = 8192
	modeHop           = 2048
	modeMinFrames     = 4     // a peak must persist this many frames to be a mode
	modeMinRelDB      = -50.0 // per-frame floor relative to the frame's loudest bin
	modeProminenceDB  = 6.0   // over the neighbouring +/-3 bins
	modeLinkTolFrac   = 0.005 // +/-0.5% frequency continuity between frames
	modeStrongWithin  = 20.0  // Strong()'s default window, in dB below the loudest mode
	modeDecayFloorDB  = 40.0  // fit the decay from the peak down to this far below it
	modeMaxPerFrame   = 40
	modeMinHz         = 30.0
)

type Mode struct {
	Hz            float64
	PeakDB        float64
	PeakFrame     int
	DecayDBPerSec float64
	DecayOK       bool
	Track         []float64
}

type ModeSet struct {
	Modes     []Mode
	Anchor    int
	HopSec    float64
	NumFrames int
}

const naNoModes = "fewer than two stable spectral modes were tracked"

type modeTrack struct {
	freqs, amps []float64 // per hit
	frames      []int
	lastHz      float64
}

// framePeaks returns the prominent local maxima of one magnitude frame.
func framePeaks(mag []float64, binHz float64) []spectralPartial {
	peaks := peakPickPartials(mag, binHz, modeMinHz, modeMaxPerFrame*4)
	if len(peaks) == 0 {
		return nil
	}
	maxAmp := 0.0
	for _, p := range peaks {
		if p.Amp > maxAmp {
			maxAmp = p.Amp
		}
	}
	floor := maxAmp * math.Pow(10, modeMinRelDB/20)
	var out []spectralPartial
	for _, p := range peaks {
		if p.Amp < floor {
			continue
		}
		bin := int(p.Freq/binHz + 0.5)
		// prominence over the neighbourhood, excluding the peak's own +/-1 bins
		nb := 0.0
		for d := 2; d <= 3; d++ {
			for _, j := range []int{bin - d, bin + d} {
				if j >= 0 && j < len(mag) && mag[j] > nb {
					nb = mag[j]
				}
			}
		}
		if nb > 0 && 20*math.Log10(p.Amp/nb) < modeProminenceDB {
			continue
		}
		out = append(out, p)
		if len(out) >= modeMaxPerFrame {
			break
		}
	}
	return out
}

func AnalyzeModes(st *STFTFrames) ModeSet {
	ms := ModeSet{Anchor: -1}
	if st == nil || len(st.Frames) == 0 {
		return ms
	}
	ms.HopSec, ms.NumFrames = st.HopSec, len(st.Frames)

	var active []*modeTrack
	var done []*modeTrack
	for fi, mag := range st.Frames {
		peaks := framePeaks(mag, st.BinHz)
		used := make([]bool, len(peaks))
		var next []*modeTrack
		for _, tr := range active {
			best, bestGap := -1, math.Inf(1)
			tol := math.Max(tr.lastHz*modeLinkTolFrac, st.BinHz)
			for i, p := range peaks {
				if used[i] {
					continue
				}
				if gap := math.Abs(p.Freq - tr.lastHz); gap <= tol && gap < bestGap {
					best, bestGap = i, gap
				}
			}
			if best < 0 {
				done = append(done, tr)
				continue
			}
			used[best] = true
			tr.freqs = append(tr.freqs, peaks[best].Freq)
			tr.amps = append(tr.amps, peaks[best].Amp)
			tr.frames = append(tr.frames, fi)
			tr.lastHz = peaks[best].Freq
			next = append(next, tr)
		}
		for i, p := range peaks {
			if !used[i] {
				next = append(next, &modeTrack{freqs: []float64{p.Freq}, amps: []float64{p.Amp}, frames: []int{fi}, lastHz: p.Freq})
			}
		}
		active = next
	}
	done = append(done, active...)

	for _, tr := range done {
		if len(tr.frames) < modeMinFrames {
			continue
		}
		ms.Modes = append(ms.Modes, buildMode(tr, ms.NumFrames, st.HopSec))
	}
	sort.Slice(ms.Modes, func(i, j int) bool { return ms.Modes[i].Hz < ms.Modes[j].Hz })
	ms.Modes = mergeCloseModes(ms.Modes, st.BinHz)

	// Anchor: the lowest mode within modeStrongWithin dB of the loudest.
	maxDB := math.Inf(-1)
	for _, m := range ms.Modes {
		maxDB = math.Max(maxDB, m.PeakDB)
	}
	for i, m := range ms.Modes {
		if m.PeakDB >= maxDB-modeStrongWithin {
			ms.Anchor = i
			break
		}
	}
	return ms
}

func buildMode(tr *modeTrack, numFrames int, hopSec float64) Mode {
	m := Mode{Track: make([]float64, numFrames)}
	for i := range m.Track {
		m.Track[i] = math.NaN()
	}
	var wsum, fsum float64
	peakDB, peakFrame := math.Inf(-1), 0
	for i, f := range tr.frames {
		db := ampToDBFloor(tr.amps[i])
		m.Track[f] = db
		wsum += tr.amps[i]
		fsum += tr.amps[i] * tr.freqs[i]
		if db > peakDB {
			peakDB, peakFrame = db, f
		}
	}
	m.Hz = fsum / wsum
	m.PeakDB, m.PeakFrame = peakDB, peakFrame

	var xs, ys []float64
	for f := peakFrame; f < numFrames; f++ {
		db := m.Track[f]
		if math.IsNaN(db) {
			break
		}
		if db < peakDB-modeDecayFloorDB {
			break
		}
		xs = append(xs, float64(f-peakFrame)*hopSec)
		ys = append(ys, db)
	}
	if len(xs) >= 3 {
		m.DecayDBPerSec = olsSlope(xs, ys)
		m.DecayOK = true
	}
	return m
}

// mergeCloseModes folds tracks closer than one bin into the louder one — a
// track that briefly split across a bin boundary must not count twice.
func mergeCloseModes(modes []Mode, binHz float64) []Mode {
	var out []Mode
	for _, m := range modes {
		if n := len(out); n > 0 && m.Hz-out[n-1].Hz < binHz {
			if m.PeakDB > out[n-1].PeakDB {
				out[n-1] = m
			}
			continue
		}
		out = append(out, m)
	}
	return out
}

func (m ModeSet) Strong(withinDB float64) []Mode {
	maxDB := math.Inf(-1)
	for _, x := range m.Modes {
		maxDB = math.Max(maxDB, x.PeakDB)
	}
	var out []Mode
	for _, x := range m.Modes {
		if x.PeakDB >= maxDB-withinDB {
			out = append(out, x)
		}
	}
	return out
}

func (m ModeSet) AnchorHz() float64 {
	if m.Anchor < 0 || m.Anchor >= len(m.Modes) {
		return 0
	}
	return m.Modes[m.Anchor].Hz
}

func (m ModeSet) Ratios(n int) []float64 {
	a := m.AnchorHz()
	if a <= 0 {
		return nil
	}
	var out []float64
	for _, x := range m.Strong(modeStrongWithin) {
		if x.Hz < a*0.999 {
			continue
		}
		out = append(out, x.Hz/a)
		if len(out) >= n {
			break
		}
	}
	return out
}

func (b *Bundle) Modes() *ModeSet {
	b.modesOnce.Do(func() {
		ms := AnalyzeModes(b.STFT(modeFFTSize, modeHop))
		b.modes = &ms
	})
	return b.modes
}

func RequiresModes(b *Bundle) bool {
	return RequiresAudible(b) && len(b.Modes().Strong(modeStrongWithin)) >= 2
}
```

Add to `Bundle` in `bundle.go`:

```go
	modesOnce sync.Once
	modes     *ModeSet
```

- [ ] **Step 4: Run the tests until they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestAnalyzeModes|TestModes_' ./internal/audio/fingerprint/`
Expected: PASS. If the decay tolerance fails, check that the 8192-window smearing of the first frames is excluded (the fit starts at `PeakFrame`, which is what excludes the rise).

- [ ] **Step 5: Run the whole package fast path and `go vet`**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ && ../../.tools/go/bin/go vet ./internal/audio/fingerprint/`
Expected: PASS, no vet output (`unread_fields_test.go` may require every exported `Mode` field to be read; `Track` and `PeakFrame` are read by Tasks 3 and 5 — if the test fires before those exist, add the field to that test's allowlist with the reason "read by registry_decay.go / decorrelation.go").

---

### Task 2: Modal-structure descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/registry_modes.go`
- Create: `src/go/internal/audio/fingerprint/registry_modes_test.go`

**Interfaces:**
- Consumes: Task 1 (`Modes()`, `Strong`, `Ratios`, `AnchorHz`, `RequiresModes`, `naNoModes`), `Vector`, `Scalar`, `Unavailable`, `compareReduced(mag, label, drift, mismatch)` (compare.go:164).
- Produces:
  ```go
  const (
      IDModeAnchorHz       = "mode.anchor_hz"
      IDModeRatios         = "mode.ratios"
      IDModeCountPerOctave = "mode.count_per_octave"
      IDModeHarmonicity    = "mode.harmonicity"
      IDModeBeatingRate    = "mode.beating_rate_hz"
      IDModeBeatingDepth   = "mode.beating_depth"
  )
  const GroupModes = "modal structure"
  func ModeRatioDistanceCents(ref, synth []float64) float64  // symmetric mean nearest-gap
  func ModeHarmonicity(ms *ModeSet) (float64, bool)
  func ModeCountPerOctave(ms *ModeSet) (float64, bool)
  func ModeBeating(ms *ModeSet) (rateHz, depth float64, ok bool)
  ```

- [ ] **Step 1: Write the failing tests**

```go
// registry_modes_test.go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func ratioTone(sr int, dur, f0 float64, ratios []float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		for _, r := range ratios {
			s[i] += (0.3 / float64(len(ratios))) * math.Sin(2*math.Pi*f0*r*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestModeHarmonicity_BarVsString(t *testing.T) {
	bar := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2.76, 5.40}), "bar")
	str := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2, 3, 4, 5}), "string")
	hb, ok := ModeHarmonicity(bar.Modes())
	if !ok || hb > 0.4 {
		t.Fatalf("bar harmonicity want < 0.4, got %.2f ok=%v", hb, ok)
	}
	hs, ok := ModeHarmonicity(str.Modes())
	if !ok || hs < 0.9 {
		t.Fatalf("string harmonicity want > 0.9, got %.2f", hs)
	}
}

func TestModeRatioDistanceCents(t *testing.T) {
	if d := ModeRatioDistanceCents([]float64{1, 2, 3}, []float64{1, 2, 3}); d != 0 {
		t.Fatalf("identical want 0, got %.2f", d)
	}
	// 2.76 vs 3.00 is 144 cents; symmetric mean over 3 modes.
	d := ModeRatioDistanceCents([]float64{1, 2.76, 5.40}, []float64{1, 3, 5.40})
	if d < 40 || d > 60 {
		t.Fatalf("want ~48 cents (144/3), got %.1f", d)
	}
}

func TestModeBeating_TwoCloseSines(t *testing.T) {
	sr := 44100
	n := int(2.0 * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tt := float64(i) / float64(sr)
		s[i] = 0.3*math.Sin(2*math.Pi*880*tt) + 0.3*math.Sin(2*math.Pi*884*tt)
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "beat")
	rate, depth, ok := ModeBeating(b.Modes())
	if !ok {
		t.Fatal("beating pair not found")
	}
	if math.Abs(rate-4) > 1.0 {
		t.Fatalf("beating rate want 4 Hz, got %.2f", rate)
	}
	if depth < 0.3 {
		t.Fatalf("equal-amplitude pair should beat deeply, got depth %.2f", depth)
	}
}

func TestModeRows_AppearForUnpitchedSignal(t *testing.T) {
	// A bar tone whose DetectF0 confidence is irrelevant: the mode rows must
	// apply purely on RequiresModes.
	b := NewBundle(ratioTone(44100, 1.5, 220, []float64{1, 2.76, 5.40}), "bar")
	ids := map[string]bool{}
	for _, d := range DefaultRegistry().Applicable(b) {
		ids[d.ID] = true
	}
	for _, id := range []string{IDModeAnchorHz, IDModeRatios, IDModeCountPerOctave, IDModeHarmonicity} {
		if !ids[id] {
			t.Errorf("%s must apply to a two-mode bar tone", id)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMode' ./internal/audio/fingerprint/`
Expected: FAIL — undefined `ModeHarmonicity`.

- [ ] **Step 3: Implement `registry_modes.go`**

```go
package fingerprint

import (
	"fmt"
	"math"
)

const (
	IDModeAnchorHz       = "mode.anchor_hz"
	IDModeRatios         = "mode.ratios"
	IDModeCountPerOctave = "mode.count_per_octave"
	IDModeHarmonicity    = "mode.harmonicity"
	IDModeBeatingRate    = "mode.beating_rate_hz"
	IDModeBeatingDepth   = "mode.beating_depth"
)

const GroupModes = "modal structure"

const (
	modeRatioCount        = 12
	modeHarmonicTolFrac   = 0.03
	modeBeatingMaxGapHz   = 25.0
	modeCountLoHz         = 100.0
	modeCountHiHz         = 8000.0
)

// ModeRatioDistanceCents is the symmetric mean nearest-neighbour gap between
// two ratio lists, in cents: for each ratio on one side, the closest ratio on
// the other; averaged; the larger of the two directions is returned so a
// synth that adds spurious modes is penalised as much as one that misses.
func ModeRatioDistanceCents(ref, synth []float64) float64 {
	one := func(a, b []float64) float64 {
		if len(a) == 0 || len(b) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, x := range a {
			best := math.Inf(1)
			for _, y := range b {
				if c := math.Abs(1200 * math.Log2(y/x)); c < best {
					best = c
				}
			}
			sum += best
		}
		return sum / float64(len(a))
	}
	return math.Max(one(ref, synth), one(synth, ref))
}

func ModeHarmonicity(ms *ModeSet) (float64, bool) {
	a := ms.AnchorHz()
	strong := ms.Strong(modeStrongWithin)
	if a <= 0 || len(strong) < 2 {
		return 0, false
	}
	var total, harmonic float64
	for _, m := range strong {
		e := math.Pow(10, m.PeakDB/10)
		total += e
		r := m.Hz / a
		if math.Abs(r-math.Round(r)) <= modeHarmonicTolFrac*math.Round(r) {
			harmonic += e
		}
	}
	if total <= 0 {
		return 0, false
	}
	return harmonic / total, true
}

func ModeCountPerOctave(ms *ModeSet) (float64, bool) {
	strong := ms.Strong(modeStrongWithin)
	if len(strong) < 2 {
		return 0, false
	}
	n := 0
	for _, m := range strong {
		if m.Hz >= modeCountLoHz && m.Hz <= modeCountHiHz {
			n++
		}
	}
	octaves := math.Log2(modeCountHiHz / modeCountLoHz)
	return float64(n) / octaves, true
}

// ModeBeating finds the closest pair of strong modes within
// modeBeatingMaxGapHz and returns their difference frequency plus the
// modulation depth of their summed level track (max-min over max+min of
// the linear sum of the two mode tracks).
func ModeBeating(ms *ModeSet) (rateHz, depth float64, ok bool) {
	strong := ms.Strong(modeStrongWithin)
	bi, bj, best := -1, -1, math.Inf(1)
	for i := 0; i < len(strong); i++ {
		for j := i + 1; j < len(strong); j++ {
			if gap := strong[j].Hz - strong[i].Hz; gap > 0 && gap <= modeBeatingMaxGapHz && gap < best {
				bi, bj, best = i, j, gap
			}
		}
	}
	if bi < 0 {
		return 0, 0, false
	}
	// Depth from the combined track. A beating pair is resolved as two modes
	// only when the window separates them; the envelope of their sum is what
	// the ear hears, so reconstruct it from the two dB tracks.
	lo, hi := math.Inf(1), math.Inf(-1)
	for f := 0; f < ms.NumFrames; f++ {
		a, b := strong[bi].Track[f], strong[bj].Track[f]
		if math.IsNaN(a) || math.IsNaN(b) {
			continue
		}
		v := math.Pow(10, a/20) + math.Pow(10, b/20)
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi <= 0 || math.IsInf(lo, 1) {
		return best, 0, true
	}
	// Two equal sines Δf apart sum to an envelope swinging between 0 and 2A;
	// sampled at the mode hop this reads as a deep swing. Report as (hi-lo)/(hi+lo).
	return best, (hi - lo) / (hi + lo), true
}

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDModeAnchorHz, Group: GroupModes, Name: "mode anchor",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Drift: 5, Mismatch: 15,
		Doc: "Frequency of the LOWEST spectral mode within 20 dB of the loudest — the pitch grid " +
			"every mode ratio is measured against. Independent of DetectF0, so it applies to " +
			"bells, bars, drums and cymbals. A large delta here means the two sides are on " +
			"different grids (the tubular-bells synth was measured at 4x the reference's " +
			"anchor before this row existed) and every ratio comparison below is suspect.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			if a := b.Modes().AnchorHz(); a > 0 {
				return Scalar(a)
			}
			return Unavailable(naNoModes)
		},
	})

	r.Register(Descriptor{
		ID: IDModeRatios, Group: GroupModes, Name: "mode ratios",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "cents",
		Drift: 30, Mismatch: 80,
		Doc: "The first twelve strong modes divided by the anchor, compared as the symmetric " +
			"mean nearest-neighbour gap in cents. This is 'where do the overtones sit' " +
			"without assuming a harmonic series — a plucked string reads 1,2,3…, a marimba " +
			"bar 1,4,10, a tube bell 0.62,1,1.47,1.71,2.01… The kalimba complaint ('sounds " +
			"like a plucked string, needs the wooden marimba feel') is this row.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rs := b.Modes().Ratios(modeRatioCount)
			if len(rs) < 2 {
				return Unavailable(naNoModes)
			}
			labels := make([]string, len(rs))
			for i := range rs {
				labels[i] = fmt.Sprintf("m%d", i+1)
			}
			return Vector(rs, labels)
		},
		Compare: func(ref, synth Measurement) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "mode ratios unavailable on one side"}
			}
			d := ModeRatioDistanceCents(ref.Values, synth.Values)
			return compareReduced(d, fmt.Sprintf("mean gap %.0f cents", d), 30, 80)
		},
	})

	r.Register(Descriptor{
		ID: IDModeCountPerOctave, Group: GroupModes, Name: "mode density",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRatio,
		Drift: 1.4, Mismatch: 2.0,
		Doc: "Strong modes per octave over 100 Hz–8 kHz. A bell or cymbal is dense, a string or " +
			"pipe sparse. 'Metallic complexity' in the listener's words.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := ModeCountPerOctave(b.Modes())
			if !ok {
				return Unavailable(naNoModes)
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDModeHarmonicity, Group: GroupModes, Name: "mode harmonicity",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Share of strong-mode ENERGY sitting within 3% of an integer multiple of the " +
			"anchor. ~1.0 for strings and pipes, low for bars, bells and membranes. This is the " +
			"wooden-vs-string axis the kalimba needed: its inharmonicity already matched the " +
			"reference (0.010 vs 0.008) while its mode SEQUENCE did not.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := ModeHarmonicity(b.Modes())
			if !ok {
				return Unavailable(naNoModes)
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDModeBeatingRate, Group: GroupModes, Name: "beating rate",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "Hz",
		Drift: 1, Mismatch: 4,
		Doc: "Difference frequency of the closest pair of strong modes under 25 Hz apart — the " +
			"slow 'wave' a bell's near-coincident partials produce. N/A when no such pair " +
			"exists, which for a bell reference is itself the finding.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rate, _, ok := ModeBeating(b.Modes())
			if !ok {
				return Unavailable("no pair of strong modes closer than 25 Hz")
			}
			return Scalar(rate)
		},
	})

	r.Register(Descriptor{
		ID: IDModeBeatingDepth, Group: GroupModes, Name: "beating depth",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Modulation depth of the summed envelope of that closest pair. The tubular-bells " +
			"verdict — 'a resonant wavy metallic ring that prolongs and waves in time' — is a " +
			"deep, slow beat; a clean additive tone has none.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			_, depth, ok := ModeBeating(b.Modes())
			if !ok {
				return Unavailable("no pair of strong modes closer than 25 Hz")
			}
			return Scalar(depth)
		},
	})
}
```

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestMode|TestRegistry' ./internal/audio/fingerprint/`
Expected: PASS. `TestRegistryAllPreservesNarrativeGroupOrder` may need the new group appended to its expected order list — add `GroupModes` after `GroupPartials`.

- [ ] **Step 5: Check the live report on the two worst bar/bell cases**

Run (needs corpus): `cd src/go && ../../.tools/go/bin/go build -o "$SCRATCH/refs" ./cmd/refs && BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" diagnose -instrument kalimba | sed -n '/modal structure/,/^$/p'`
Expected: a `modal structure` block with `mode anchor`, `mode ratios`, `mode density`, `mode harmonicity` rows and numbers on both sides. Repeat for `tubular-bells` and `snare`; the snare must show the block too (no f0 needed).

---

### Task 3: Resolved-decay descriptors and the `Demoted` flag

**Files:**
- Modify: `src/go/internal/audio/fingerprint/descriptor.go` (add `Demoted bool`)
- Create: `src/go/internal/audio/fingerprint/decay_resolved.go`
- Create: `src/go/internal/audio/fingerprint/registry_decay.go`
- Create: `src/go/internal/audio/fingerprint/decay_resolved_test.go`
- Modify: `src/go/internal/audio/fingerprint/registry_core.go` (the `IDEnvRingT60` row gets `Demoted: true` and a Doc pointer)

**Interfaces:**
- Consumes: Task 1 `Modes`; `(*Bundle).STFT(trackFFTSize, trackHop)` (2048/512); `olsSlope`.
- Produces:
  ```go
  // descriptor.go
  Demoted bool // excluded from "worst offending axes" rankings and from validate's trusted set; still measured and displayed
  // decay_resolved.go
  var octaveBandCentersHz = []float64{125, 250, 500, 1000, 2000, 4000, 8000}
  var octaveBandLabels    = []string{"125", "250", "500", "1k", "2k", "4k", "8k"}
  type BandDecay struct { T60Sec []float64; OK []bool; RateDBPerSec []float64 }
  func BandT60(st *STFTFrames) BandDecay        // per band: energy track → peak → OLS slope down to -40 dB or clip end → T60 = 60/|rate|; OK=false when <3 frames after peak or rate >= 0
  func PerModeDecay(ms *ModeSet, n int) (rates []float64, labels []string, ok bool)   // top-n Strong(20) modes with DecayOK, ascending Hz, labels "m1".."mn"
  func DecaySlopeVsFreq(ms *ModeSet) (dbPerSecPerKHz float64, ok bool)              // OLS of DecayDBPerSec vs Hz/1000 over Strong(20) with DecayOK; needs >= 3
  // registry_decay.go
  const ( IDDecayPerMode = "decay.per_mode"; IDDecaySlopeVsFreq = "decay.slope_vs_freq"; IDBandT60 = "band.t60" )
  const GroupDecayResolved = "resolved decay"
  ```

- [ ] **Step 1: Write the failing tests**

```go
// decay_resolved_test.go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// decayingTone: a 1 kHz sine decaying at rateDBPerSec, so its 1k band T60 is 60/rate.
func decayingTone(sr int, dur, hz, rateDBPerSec float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5 * math.Pow(10, -rateDBPerSec*t/20) * math.Sin(2*math.Pi*hz*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestBandT60_FitsConstructedDecay(t *testing.T) {
	b := NewBundle(decayingTone(44100, 3.0, 1000, 30), "d")
	bd := BandT60(b.STFT(trackFFTSize, trackHop))
	i := 3 // 1k band
	if !bd.OK[i] {
		t.Fatal("1k band should be measurable")
	}
	if math.Abs(bd.T60Sec[i]-2.0) > 0.2 {
		t.Fatalf("T60 want 2.0 s (60/30), got %.2f", bd.T60Sec[i])
	}
}

func TestBandT60_FiniteWhenClipEndsAboveMinus20dB(t *testing.T) {
	// 1.0 s clip at 15 dB/s only reaches -15 dB: the old ring T60 clamped;
	// the fitted rate still yields 4.0 s.
	b := NewBundle(decayingTone(44100, 1.0, 1000, 15), "short")
	bd := BandT60(b.STFT(trackFFTSize, trackHop))
	if !bd.OK[3] || math.IsInf(bd.T60Sec[3], 0) || math.Abs(bd.T60Sec[3]-4.0) > 0.6 {
		t.Fatalf("want ~4.0 s extrapolated, got %.2f ok=%v", bd.T60Sec[3], bd.OK[3])
	}
}

func TestDecaySlopeVsFreq_HighsDieFaster(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 2.0), "bar") // 440@-20, 1215@-60 dB/s
	s, ok := DecaySlopeVsFreq(b.Modes())
	// needs >=3 modes; add a third by using a richer tone
	if ok && s >= 0 {
		t.Fatalf("slope must be negative (highs decay faster), got %.2f", s)
	}
	rates, labels, ok := PerModeDecay(b.Modes(), 8)
	if !ok || len(rates) != 2 || labels[0] != "m1" {
		t.Fatalf("per-mode decay want 2 rates, got %v %v ok=%v", rates, labels, ok)
	}
	if rates[0] > rates[1] {
		t.Fatalf("mode 1 should decay slower (less negative): %v", rates)
	}
}

func TestRingT60IsDemoted(t *testing.T) {
	d, ok := DefaultRegistry().ByID(IDEnvRingT60)
	if !ok || !d.Demoted {
		t.Fatal("env.ring_t60 must be registered with Demoted: true")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestBandT60|TestDecaySlope|TestRingT60' ./internal/audio/fingerprint/`
Expected: FAIL — undefined `BandT60`, `Demoted`.

- [ ] **Step 3: Add `Demoted` to `Descriptor` and mark `ring T60`**

In `descriptor.go`, after `Unit string`:

```go
	// Demoted marks a descriptor that is measured and displayed but excluded
	// from every "worst offending axes" ranking and from validate's trusted
	// set. Set when calibration showed the axis misleads on this corpus.
	Demoted bool
```

In `registry_core.go`, in the `IDEnvRingT60` registration add `Demoted: true,` and append to its Doc: `" DEMOTED 2026-09-04: every corpus clip is truncated by convert.max_sec, so anything that rings clamps at the clip end on BOTH sides and compares as fine (AUC 0.19, anti-correlated). Use band.t60, which fits the visible tail's slope instead of clamping."`

- [ ] **Step 4: Implement `decay_resolved.go`**

```go
package fingerprint

import (
	"fmt"
	"math"
)

var octaveBandCentersHz = []float64{125, 250, 500, 1000, 2000, 4000, 8000}
var octaveBandLabels = []string{"125", "250", "500", "1k", "2k", "4k", "8k"}

const bandDecayFitFloorDB = 40.0

type BandDecay struct {
	T60Sec       []float64
	OK           []bool
	RateDBPerSec []float64
}

// BandT60 fits each octave band's decay RATE from the visible tail and
// converts it to a T60, so a clip that ends above -60 dB still yields a
// number. This is the replacement for the clamped env.ring_t60.
func BandT60(st *STFTFrames) BandDecay {
	n := len(octaveBandCentersHz)
	out := BandDecay{T60Sec: make([]float64, n), OK: make([]bool, n), RateDBPerSec: make([]float64, n)}
	if st == nil || len(st.Frames) < 3 {
		return out
	}
	for bi, c := range octaveBandCentersHz {
		lo, hi := c/math.Sqrt2, c*math.Sqrt2
		track := make([]float64, len(st.Frames))
		for fi, mag := range st.Frames {
			e := 0.0
			for k := int(lo / st.BinHz); k <= int(hi/st.BinHz) && k < len(mag); k++ {
				if k >= 1 {
					e += mag[k] * mag[k]
				}
			}
			track[fi] = 10 * math.Log10(e+1e-20)
		}
		peak, pi := math.Inf(-1), 0
		for i, v := range track {
			if v > peak {
				peak, pi = v, i
			}
		}
		if peak < -100 {
			continue
		}
		var xs, ys []float64
		for i := pi; i < len(track); i++ {
			if track[i] < peak-bandDecayFitFloorDB {
				break
			}
			xs = append(xs, float64(i-pi)*st.HopSec)
			ys = append(ys, track[i])
		}
		if len(xs) < 3 {
			continue
		}
		rate := olsSlope(xs, ys)
		if rate >= -1e-6 {
			continue
		}
		out.RateDBPerSec[bi] = rate
		out.T60Sec[bi] = 60 / -rate
		out.OK[bi] = true
	}
	return out
}

func PerModeDecay(ms *ModeSet, n int) (rates []float64, labels []string, ok bool) {
	for _, m := range ms.Strong(modeStrongWithin) {
		if !m.DecayOK {
			continue
		}
		rates = append(rates, m.DecayDBPerSec)
		labels = append(labels, fmt.Sprintf("m%d", len(rates)))
		if len(rates) >= n {
			break
		}
	}
	return rates, labels, len(rates) >= 1
}

func DecaySlopeVsFreq(ms *ModeSet) (float64, bool) {
	var xs, ys []float64
	for _, m := range ms.Strong(modeStrongWithin) {
		if m.DecayOK {
			xs = append(xs, m.Hz/1000)
			ys = append(ys, m.DecayDBPerSec)
		}
	}
	if len(xs) < 3 {
		return 0, false
	}
	return olsSlope(xs, ys), true
}
```

- [ ] **Step 5: Implement `registry_decay.go`**

```go
package fingerprint

import (
	"fmt"
	"math"
)

const (
	IDDecayPerMode     = "decay.per_mode"
	IDDecaySlopeVsFreq = "decay.slope_vs_freq"
	IDBandT60          = "band.t60"
)

const GroupDecayResolved = "resolved decay"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDDecayPerMode, Group: GroupDecayResolved, Name: "decay per mode",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "dB/s",
		Drift: 6, Mismatch: 15,
		Doc: "Decay rate of each strong mode, fitted from its own level track after its peak. " +
			"Compared as mean absolute difference over the modes both sides share by rank. " +
			"A harp's 'let it ring' and a nylon-vs-steel 'metallic' verdict both live here: " +
			"the question is not how fast the note dies but how fast EACH partial dies.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			rates, labels, ok := PerModeDecay(b.Modes(), 8)
			if !ok {
				return Unavailable("no strong mode had three frames of decay to fit")
			}
			return Vector(rates, labels)
		},
		Compare: func(ref, synth Measurement) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "per-mode decay unavailable on one side"}
			}
			n := len(ref.Values)
			if len(synth.Values) < n {
				n = len(synth.Values)
			}
			if n == 0 {
				return Delta{OK: false, NA: "no shared modes"}
			}
			sum := 0.0
			for i := 0; i < n; i++ {
				sum += math.Abs(ref.Values[i] - synth.Values[i])
			}
			d := sum / float64(n)
			return compareReduced(d, fmt.Sprintf("mean|Δ| %.1f dB/s", d), 6, 15)
		},
	})

	r.Register(Descriptor{
		ID: IDDecaySlopeVsFreq, Group: GroupDecayResolved, Name: "decay slope vs frequency",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "dB/s per kHz",
		Drift: 3, Mismatch: 8,
		Doc: "How much faster the high modes die than the low ones. Nylon and wood: strongly " +
			"negative (bright attack, warm sustain). Steel and metal: near zero (the highs " +
			"ring as long as the fundamental). The centroid can match while this differs — " +
			"which is exactly the 'metallic acoustic, not nylon' verdict recorded in the guide.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := DecaySlopeVsFreq(b.Modes())
			if !ok {
				return Unavailable("fewer than three strong modes with a fitted decay")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDBandT60, Group: GroupDecayResolved, Name: "band T60",
		Scope: ScopeBand, Repr: ReprSpectrum, Kind: KindSeconds,
		Drift: 1.4, Mismatch: 2.0,
		Doc: "T60 per octave band, derived from the fitted decay RATE of the visible tail " +
			"rather than the time a threshold was crossed, so a 3 s clip of a 10 s ring " +
			"still reports ~10 s instead of a clamped 3 s on both sides. Compared as the " +
			"mean |log2(ref/synth)| over bands measurable on both sides. This replaces " +
			"ring T60 in every ranking.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			bd := BandT60(b.STFT(trackFFTSize, trackHop))
			vals := make([]float64, len(bd.T60Sec))
			any := false
			for i := range vals {
				if bd.OK[i] {
					vals[i] = bd.T60Sec[i]
					any = true
				} else {
					vals[i] = math.NaN()
				}
			}
			if !any {
				return Unavailable("no octave band had a fittable decay")
			}
			return Vector(vals, octaveBandLabels)
		},
		Compare: func(ref, synth Measurement) Delta {
			if !ref.OK || !synth.OK {
				return Delta{OK: false, NA: "band T60 unavailable on one side"}
			}
			sum, n := 0.0, 0
			signed := 0.0
			for i := range ref.Values {
				if i >= len(synth.Values) || math.IsNaN(ref.Values[i]) || math.IsNaN(synth.Values[i]) {
					continue
				}
				l := math.Log2(synth.Values[i] / ref.Values[i])
				sum += math.Abs(l)
				signed += l
				n++
			}
			if n == 0 {
				return Delta{OK: false, NA: "no octave band measurable on both sides"}
			}
			d := compareReduced(sum/float64(n), fmt.Sprintf("mean %.2fx", math.Pow(2, sum/float64(n))), 0.5, 1.0)
			if signed/float64(n) > 0 {
				d.Direction = "high"
			} else {
				d.Direction = "low"
			}
			if d.Verdict == VerdictOK {
				d.Direction = ""
			}
			return d
		},
	})
}
```

Check `compareReduced`'s signature in `compare.go:164` (`mag float64, label string, drift, mismatch float64`) and that it sets `Delta.OK=true`; if it does not set `Direction`, the explicit assignment above is what makes the report say "rings too long/short".

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestBandT60|TestDecaySlope|TestRingT60|TestRegistry' ./internal/audio/fingerprint/`
Expected: PASS (append `GroupDecayResolved` to the narrative order test if it fails on order).

- [ ] **Step 7: Live check**

Run: `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" diagnose -instrument harp | sed -n '/resolved decay/,/^$/p'` (rebuild `refs` first).
Expected: `band T60` shows the synth far shorter than the reference (the harp verdict), with finite numbers on both sides.

---

### Task 4: Modulation-spectrum descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/modulation.go`
- Create: `src/go/internal/audio/fingerprint/registry_modulation.go`
- Create: `src/go/internal/audio/fingerprint/modulation_test.go`

**Interfaces:**
- Consumes: `(*Bundle).Tracks()` (`RMS`, `Centroid`, `HopSec`, `NumFrames`), `wave.MagnitudeSpectrum(w, fftSize, wave.WindowHann) (mag, binHz)`, `olsSlope`, `spectralFlatnessOf(mag)`.
- Produces:
  ```go
  type ModSpectrum struct {
      RateHz   float64    // dominant rate in 0.5–30 Hz
      Depth    float64    // power at the dominant bin / total 0.5–30 Hz power, times the track's coefficient of variation
      Bands    [3]float64 // energy shares: gesture 0.5–2, vibrato 2–8, warble 8–30
      Flatness float64    // spectral flatness of the 0.5–30 Hz modulation spectrum
      OK       bool
  }
  const modMinFrames = 32
  func ModulationSpectrum(track []float64, hopSec float64) ModSpectrum
  const ( IDModAMRate = "mod.am.rate_hz"; IDModAMDepth = "mod.am.depth"; IDModAMBands = "mod.am.bands"; IDModAMFlatness = "mod.am.flatness"; IDModCentroidDepth = "mod.centroid.depth" )
  ```

- [ ] **Step 1: Write the failing tests**

```go
// modulation_test.go
package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func tremoloTone(sr int, dur, hz, rate, depth float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.4 * (1 + depth*math.Sin(2*math.Pi*rate*t)) * math.Sin(2*math.Pi*hz*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestModulationSpectrum_FiveHzTremoloPeaksAtFive(t *testing.T) {
	b := NewBundle(tremoloTone(44100, 3.0, 440, 5, 0.5), "trem")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || math.Abs(ms.RateHz-5) > 0.6 {
		t.Fatalf("want 5 Hz, got %.2f ok=%v", ms.RateHz, ms.OK)
	}
	if ms.Bands[1] < 0.6 {
		t.Fatalf("energy should sit in the vibrato band, got %v", ms.Bands)
	}
	if ms.Flatness > 0.5 {
		t.Fatalf("a pure tremolo is peaky, flatness %.2f", ms.Flatness)
	}
}

func TestModulationSpectrum_FlatToneHasNoDepth(t *testing.T) {
	b := NewBundle(wave.Sine(440, 0.4, 3.0, 44100), "flat")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || ms.Depth > 0.02 {
		t.Fatalf("steady sine depth want ~0, got %.3f", ms.Depth)
	}
}

func TestModulationSpectrum_NoiseIsFlat(t *testing.T) {
	b := NewBundle(wave.Noise(3, 0.3, 3.0, 44100), "noise")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || ms.Flatness < 0.4 {
		t.Fatalf("noise envelope should be a flat modulation spectrum, got %.2f", ms.Flatness)
	}
}

func TestModulationSpectrum_TooShort(t *testing.T) {
	if ms := ModulationSpectrum(make([]float64, 10), 0.0116); ms.OK {
		t.Fatal("fewer than modMinFrames must be !OK")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestModulationSpectrum' ./internal/audio/fingerprint/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `modulation.go`**

```go
package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// modulation.go — the modulation spectrum of a frame-wise track.
//
// The program spec's failure mode 2: the violin's amplitude variation was
// tuned to std/mean 0.40 against a real 0.64 and still sounded wrong, because
// the real signal is a slow bow-stroke SHAPE and the synth's was fast chaos.
// A scalar cannot see the difference; the spectrum of the envelope can.

const (
	modMinFrames = 32
	modLoHz      = 0.5
	modHiHz      = 30.0
)

var modBandEdgesHz = [4]float64{0.5, 2, 8, 30}

type ModSpectrum struct {
	RateHz   float64
	Depth    float64
	Bands    [3]float64
	Flatness float64
	OK       bool
}

func ModulationSpectrum(track []float64, hopSec float64) ModSpectrum {
	var out ModSpectrum
	if len(track) < modMinFrames || hopSec <= 0 {
		return out
	}
	// Detrend: remove mean and OLS line so a swell or decay does not read as
	// a sub-hertz modulation; then normalise by the mean so depth is relative.
	xs := make([]float64, len(track))
	mean := 0.0
	for i, v := range track {
		xs[i] = float64(i) * hopSec
		mean += v
	}
	mean /= float64(len(track))
	if mean <= 1e-9 {
		return out
	}
	slope := olsSlope(xs, track)
	det := make([]float64, len(track))
	var ss float64
	for i, v := range track {
		det[i] = (v - mean - slope*(xs[i]-xs[len(xs)/2])) / mean
		ss += det[i] * det[i]
	}
	cv := math.Sqrt(ss / float64(len(det)))

	frameRate := 1 / hopSec
	n := 1
	for n < len(det) {
		n <<= 1
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: det, SampleRate: int(math.Round(frameRate))}, n, wave.WindowHann)
	lo, hi := int(modLoHz/binHz), int(modHiHz/binHz)
	if hi >= len(mag) {
		hi = len(mag) - 1
	}
	if lo < 1 {
		lo = 1
	}
	if hi <= lo {
		return out
	}
	var total float64
	peak, pk := 0.0, lo
	for k := lo; k <= hi; k++ {
		p := mag[k] * mag[k]
		total += p
		if p > peak {
			peak, pk = p, k
		}
		f := float64(k) * binHz
		for b := 0; b < 3; b++ {
			if f >= modBandEdgesHz[b] && f < modBandEdgesHz[b+1] {
				out.Bands[b] += p
			}
		}
	}
	if total <= 0 {
		return out
	}
	for b := range out.Bands {
		out.Bands[b] /= total
	}
	out.RateHz = float64(pk) * binHz
	out.Depth = (peak / total) * cv
	out.Flatness = spectralFlatnessOf(mag[lo : hi+1])
	out.OK = true
	return out
}
```

- [ ] **Step 4: Implement `registry_modulation.go`**

```go
package fingerprint

const (
	IDModAMRate        = "mod.am.rate_hz"
	IDModAMDepth       = "mod.am.depth"
	IDModAMBands       = "mod.am.bands"
	IDModAMFlatness    = "mod.am.flatness"
	IDModCentroidDepth = "mod.centroid.depth"
)

const naModShort = "clip too short for a modulation spectrum (needs 32 frames)"

func modApplies(b *Bundle) bool { return RequiresAudible(b) && b.Tracks().NumFrames >= modMinFrames }

func amSpectrum(b *Bundle) (ModSpectrum, bool) {
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	return ms, ms.OK
}

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDModAMRate, Group: GroupMod, Name: "loudness modulation rate",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindRate, Unit: "Hz",
		Drift: 1, Mismatch: 3,
		Doc: "Dominant rate of the detrended loudness track's spectrum over 0.5–30 Hz. A bow " +
			"stroke sits near 1 Hz, vibrato/tremolo 4–7 Hz, a bell's beat 1–4 Hz, chaos nowhere.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(naModShort)
			}
			return Scalar(ms.RateHz)
		},
	})
	r.Register(Descriptor{
		ID: IDModAMDepth, Group: GroupMod, Name: "loudness modulation depth",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15,
		Doc: "Share of modulation power at the dominant rate, scaled by the loudness track's " +
			"coefficient of variation: how much the note moves, and how much of that movement " +
			"is ONE shape. The tubular-bells 'waves in time' verdict is a high value here.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(naModShort)
			}
			return Scalar(ms.Depth)
		},
	})
	r.Register(Descriptor{
		ID: IDModAMBands, Group: GroupMod, Name: "modulation bands",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Energy shares of the loudness modulation spectrum in gesture (0.5–2 Hz), " +
			"vibrato (2–8 Hz) and warble (8–30 Hz). This is the discriminator the program " +
			"doc names as missing: a slow bow stroke and fast chaos can have identical " +
			"variance and opposite band profiles.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(naModShort)
			}
			return Vector(ms.Bands[:], []string{"gesture", "vibrato", "warble"})
		},
	})
	r.Register(Descriptor{
		ID: IDModAMFlatness, Group: GroupMod, Name: "modulation flatness",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Spectral flatness of the loudness modulation spectrum. A shaped gesture is " +
			"peaky (low); random amplitude noise is flat (high). 'Wobbles like a machine' " +
			"versus 'breathes'.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			ms, ok := amSpectrum(b)
			if !ok {
				return Unavailable(naModShort)
			}
			return Scalar(ms.Flatness)
		},
	})
	r.Register(Descriptor{
		ID: IDModCentroidDepth, Group: GroupMod, Name: "brightness modulation depth",
		Scope: ScopeTrack, Repr: ReprModulation, Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15,
		Doc: "Same measurement on the spectral-centroid track: does the brightness move with " +
			"the note? A real flute's vibrato swings the centroid ~40%; a synth LFO on " +
			"amplitude alone leaves it still.",
		Applies: modApplies,
		Extract: func(b *Bundle) Measurement {
			tr := b.Tracks()
			ms := ModulationSpectrum(tr.Centroid, tr.HopSec)
			if !ms.OK {
				return Unavailable(naModShort)
			}
			return Scalar(ms.Depth)
		},
	})
}
```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestModulationSpectrum|TestRegistry|TestNoUnreadMetricFields' ./internal/audio/fingerprint/`
Expected: PASS.

---

### Task 5: Partial-decorrelation descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/decorrelation.go`
- Create: `src/go/internal/audio/fingerprint/registry_aliveness.go`
- Create: `src/go/internal/audio/fingerprint/decorrelation_test.go`

**Interfaces:**
- Consumes: Task 1 `Modes`, `sustainWindow(w)` (signal.go:15), `wave.MagnitudeSpectrum`, `peakPickPartials`.
- Produces:
  ```go
  const decorrFFTSize = 32768
  func PartialBandwidthCents(w wave.Wave, modes []Mode) (meanCents float64, ok bool)  // per mode: -3 dB width around the nearest peak of a 32768-pt Hann spectrum of the sustain window, minus the Hann main lobe (4 bins), converted to cents at that mode's Hz; mean over modes with a resolvable width; ok when >= 2
  func PartialIndependence(modes []Mode) (float64, bool)  // 1 - mean pairwise Pearson r of detrended dB tracks over frames where both are present; needs >= 2 modes with >= 8 shared frames
  const ( IDPartialBandwidth = "partial.bandwidth_cents"; IDPartialIndependence = "partial.independence" )
  const GroupAliveness = "partial decorrelation"
  ```

- [ ] **Step 1: Write the failing tests**

```go
// decorrelation_test.go
package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// lockedAdditive: 6 phase-locked harmonics under ONE slow amplitude envelope.
func lockedAdditive(sr int, dur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		env := 1 + 0.3*math.Sin(2*math.Pi*0.7*t)
		for k := 1; k <= 6; k++ {
			s[i] += env * (0.3 / float64(k)) * math.Sin(2*math.Pi*220*float64(k)*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// independentAdditive: each harmonic has its OWN random slow envelope and a
// small random frequency wobble — a poor man's living tone.
func independentAdditive(sr int, dur float64) wave.Wave {
	rng := rand.New(rand.NewSource(11))
	n := int(dur * float64(sr))
	s := make([]float64, n)
	type h struct{ rate, phase, wob float64 }
	hs := make([]h, 6)
	for k := range hs {
		hs[k] = h{0.4 + 1.5*rng.Float64(), 2 * math.Pi * rng.Float64(), 0.002 * rng.Float64()}
	}
	for i := range s {
		t := float64(i) / float64(sr)
		for k := 1; k <= 6; k++ {
			p := hs[k-1]
			env := 1 + 0.4*math.Sin(2*math.Pi*p.rate*t+p.phase)
			f := 220 * float64(k) * (1 + p.wob*math.Sin(2*math.Pi*1.3*t+p.phase))
			s[i] += env * (0.3 / float64(k)) * math.Sin(2*math.Pi*f*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestPartialIndependence_LockedVsIndependent(t *testing.T) {
	locked := NewBundle(lockedAdditive(44100, 3.0), "locked")
	free := NewBundle(independentAdditive(44100, 3.0), "free")
	li, ok := PartialIndependence(locked.Modes().Strong(20))
	if !ok || li > 0.15 {
		t.Fatalf("locked additive independence want < 0.15, got %.2f ok=%v", li, ok)
	}
	fi, ok := PartialIndependence(free.Modes().Strong(20))
	if !ok || fi < 0.3 {
		t.Fatalf("independent partials want > 0.3, got %.2f", fi)
	}
}

func TestPartialBandwidth_PureSineIsNarrow(t *testing.T) {
	b := NewBundle(wave.Sine(440, 0.4, 3.0, 44100), "sine")
	bw, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(20))
	if ok && bw > 6 {
		t.Fatalf("a pure sine should have near-zero excess bandwidth, got %.1f cents", bw)
	}
}

func TestPartialBandwidth_WobblyIsWider(t *testing.T) {
	b := NewBundle(independentAdditive(44100, 3.0), "free")
	bw, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(20))
	if !ok || bw < 5 {
		t.Fatalf("frequency-wobbling partials should widen, got %.1f ok=%v", bw, ok)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPartialIndependence|TestPartialBandwidth' ./internal/audio/fingerprint/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `decorrelation.go`**

```go
package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// decorrelation.go — is each partial a single locked sine, or a living band?
//
// AGENTS.synth.instruments.md records the PADsynth argument verbatim:
// phase-locked partials read as artificial; a real harmonic is a narrow band
// of slightly detuned, randomly phased components. Two measurements make that
// checkable: how wide each partial is, and how independently the partials
// move.

const (
	decorrFFTSize        = 32768
	decorrMainLobeBins   = 4.0 // Hann main lobe, -3 dB full width ≈ 1.44 bins; use 4 as the "resolution floor" to subtract
	decorrMinSharedFrames = 8
	decorrMaxModes       = 8
)

func PartialBandwidthCents(w wave.Wave, modes []Mode) (float64, bool) {
	if len(modes) < 2 || w.Len() < decorrFFTSize {
		return 0, false
	}
	seg := sustainWindow(w)
	if seg.Len() < decorrFFTSize {
		seg = w
	}
	mag, binHz := wave.MagnitudeSpectrum(seg, decorrFFTSize, wave.WindowHann)
	var sum float64
	n := 0
	for i, m := range modes {
		if i >= decorrMaxModes {
			break
		}
		c := int(m.Hz/binHz + 0.5)
		if c < 2 || c >= len(mag)-2 {
			continue
		}
		// climb to the local max near the mode
		for c > 1 && mag[c-1] > mag[c] {
			c--
		}
		for c < len(mag)-2 && mag[c+1] > mag[c] {
			c++
		}
		half := mag[c] / math.Sqrt2
		lo, hi := c, c
		for lo > 0 && mag[lo] > half {
			lo--
		}
		for hi < len(mag)-1 && mag[hi] > half {
			hi++
		}
		widthBins := float64(hi-lo) - decorrMainLobeBins
		if widthBins < 0 {
			widthBins = 0
		}
		widthHz := widthBins * binHz
		sum += 1200 * math.Log2((m.Hz+widthHz/2)/(m.Hz-widthHz/2))
		n++
	}
	if n < 2 {
		return 0, false
	}
	return sum / float64(n), true
}

func PartialIndependence(modes []Mode) (float64, bool) {
	if len(modes) < 2 {
		return 0, false
	}
	if len(modes) > decorrMaxModes {
		modes = modes[:decorrMaxModes]
	}
	var sum float64
	pairs := 0
	for i := 0; i < len(modes); i++ {
		for j := i + 1; j < len(modes); j++ {
			r, ok := pearsonShared(modes[i].Track, modes[j].Track)
			if ok {
				sum += r
				pairs++
			}
		}
	}
	if pairs == 0 {
		return 0, false
	}
	return 1 - sum/float64(pairs), true
}

// pearsonShared correlates two dB tracks over the frames where both are
// present, after removing each one's own OLS trend (a shared decay must not
// read as correlation — every partial of a plucked note decays).
func pearsonShared(a, b []float64) (float64, bool) {
	var xs, ya, yb []float64
	for f := range a {
		if f < len(b) && !math.IsNaN(a[f]) && !math.IsNaN(b[f]) {
			xs = append(xs, float64(f))
			ya = append(ya, a[f])
			yb = append(yb, b[f])
		}
	}
	if len(xs) < decorrMinSharedFrames {
		return 0, false
	}
	da, db := detrend(xs, ya), detrend(xs, yb)
	var sab, saa, sbb float64
	for i := range da {
		sab += da[i] * db[i]
		saa += da[i] * da[i]
		sbb += db[i] * db[i]
	}
	if saa <= 1e-12 || sbb <= 1e-12 {
		return 1, true // both flat: perfectly "locked"
	}
	return sab / math.Sqrt(saa*sbb), true
}

func detrend(xs, ys []float64) []float64 {
	slope := olsSlope(xs, ys)
	var mx, my float64
	for i := range xs {
		mx += xs[i]
		my += ys[i]
	}
	mx /= float64(len(xs))
	my /= float64(len(ys))
	out := make([]float64, len(ys))
	for i := range ys {
		out[i] = ys[i] - my - slope*(xs[i]-mx)
	}
	return out
}
```

- [ ] **Step 4: Implement `registry_aliveness.go`**

```go
package fingerprint

const (
	IDPartialBandwidth    = "partial.bandwidth_cents"
	IDPartialIndependence = "partial.independence"
)

const GroupAliveness = "partial decorrelation"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDPartialBandwidth, Group: GroupAliveness, Name: "partial bandwidth",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindRate, Unit: "cents",
		Drift: 5, Mismatch: 15,
		Doc: "Mean excess -3 dB width of the strong partials on a 32768-point spectrum of the " +
			"sustain, in cents, after subtracting the window's own main lobe. A locked sine is " +
			"0; a real harmonic is a band of slightly detuned components. The PADsynth " +
			"argument, measured.",
		Applies: func(b *Bundle) bool { return RequiresModes(b) && RequiresSustain(b) },
		Extract: func(b *Bundle) Measurement {
			v, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(modeStrongWithin))
			if !ok {
				return Unavailable("fewer than two partials with a resolvable width (clip shorter than 32768 samples, or too few modes)")
			}
			return Scalar(v)
		},
	})

	r.Register(Descriptor{
		ID: IDPartialIndependence, Group: GroupAliveness, Name: "partial independence",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "1 minus the mean pairwise correlation of the strong partials' detrended level " +
			"tracks. Phase-locked additive synthesis under one envelope reads ~0; partials " +
			"that breathe on their own read higher. This is 'sterile' vs 'alive' as a number.",
		Applies: RequiresModes,
		Extract: func(b *Bundle) Measurement {
			v, ok := PartialIndependence(b.Modes().Strong(modeStrongWithin))
			if !ok {
				return Unavailable("no pair of partials shared eight frames")
			}
			return Scalar(v)
		},
	})
}
```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPartialIndependence|TestPartialBandwidth|TestRegistry' ./internal/audio/fingerprint/`
Expected: PASS. If `TestPartialIndependence_LockedVsIndependent` reads the locked tone above 0.15, check `detrend` is applied (a shared 0.7 Hz envelope is common-mode and must correlate to ~1, giving independence ~0).

---

### Task 6: Onset and noise-character descriptors

**Files:**
- Create: `src/go/internal/audio/fingerprint/onset.go`
- Create: `src/go/internal/audio/fingerprint/registry_onset.go`
- Create: `src/go/internal/audio/fingerprint/onset_test.go`

**Interfaces:**
- Consumes: `Tracks()` (`RMS`, `Flatness`, `NoiseFraction`, `HopSec`, `F0Hz`), `Segments()` (`AttackSec`, `Silent`), `wave.MagnitudeSpectrum`, `spectralFlatnessOf`, `spectralCentroidHz`, `DetectF0`, `decayThreshold(env, peakIdx, peakVal, dbDown, hopSec)` (envelope_segments.go:456), `harmonicPeak`.
- Produces:
  ```go
  func OnsetBiteRatio(tr *Tracks) (float64, bool)              // mean(RMS²·Flatness) over first 30 ms after onset ÷ same over 300–600 ms after onset; onset = first frame with RMS >= 0.1·max
  func OnsetFlatness(w wave.Wave) (float64, bool)               // flatness of a 2048-pt spectrum starting at the onset sample (first sample >= 0.1·peak), 20 ms of signal zero-padded
  func OnsetPitchGlideCents(w wave.Wave, f0 float64) (float64, bool)  // DetectF0 over [20,60) ms minus over [200,320) ms after onset, in cents; ok when both detections are within an octave of f0
  func ResidualCentroidHz(mag []float64, binHz, f0 float64) float64   // centroid of mag with ±1 bin around each k·f0 zeroed
  func NoiseDecayVsTone(tr *Tracks) (float64, bool)             // decayThreshold(20 dB) of residual energy track ÷ of harmonic energy track
  const ( IDOnsetBite = "onset.bite_ratio"; IDOnsetFlatness = "onset.flatness"; IDOnsetPitchGlide = "onset.pitch_glide_cents"; IDNoiseResidualCentroid = "noise.residual_centroid_hz"; IDNoiseDecayVsTone = "noise.decay_vs_tone" )
  const GroupOnset = "onset character"
  ```

- [ ] **Step 1: Write the failing tests**

```go
// onset_test.go
package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// toneWithBite: a steady 330 Hz tone preceded by 25 ms of broadband noise
// at the same level.
func toneWithBite(sr int, dur float64, bite bool) wave.Wave {
	rng := rand.New(rand.NewSource(5))
	n := int(dur * float64(sr))
	s := make([]float64, n)
	biteN := int(0.025 * float64(sr))
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.4 * math.Sin(2*math.Pi*330*t)
		if bite && i < biteN {
			s[i] += 0.4 * (2*rng.Float64() - 1)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestOnsetBiteRatio_BiteVsClean(t *testing.T) {
	withB, _ := OnsetBiteRatio(NewBundle(toneWithBite(44100, 1.5, true), "b").Tracks())
	clean, _ := OnsetBiteRatio(NewBundle(toneWithBite(44100, 1.5, false), "c").Tracks())
	if withB < 3*clean {
		t.Fatalf("bite ratio with noise burst (%.2f) should dwarf the clean tone's (%.2f)", withB, clean)
	}
}

func TestOnsetPitchGlide_DownwardGlide(t *testing.T) {
	sr := 44100
	n := int(1.0 * float64(sr))
	s := make([]float64, n)
	phase := 0.0
	for i := range s {
		t := float64(i) / float64(sr)
		// 40 cents sharp at t=0 decaying to 0 by 150 ms
		cents := 40 * math.Exp(-t/0.05)
		f := 440 * math.Pow(2, cents/1200)
		phase += 2 * math.Pi * f / float64(sr)
		s[i] = 0.4 * math.Sin(phase)
	}
	g, ok := OnsetPitchGlideCents(wave.Wave{Samples: s, SampleRate: sr}, 440)
	if !ok || g < 10 || g > 45 {
		t.Fatalf("want a positive glide of ~20-40 cents, got %.1f ok=%v", g, ok)
	}
}

func TestResidualCentroid_HissAboveWire(t *testing.T) {
	// The centroid of a harmonic-notched spectrum must rise when the noise is
	// high-passed.
	rng := rand.New(rand.NewSource(9))
	mk := func(hp bool) wave.Wave {
		sr, n := 44100, 44100
		s := make([]float64, n)
		prev := 0.0
		for i := range s {
			t := float64(i) / float64(sr)
			v := 2*rng.Float64() - 1
			if hp {
				v, prev = v-prev, v // crude first-difference high-pass
			}
			s[i] = 0.3*math.Sin(2*math.Pi*200*t) + 0.2*v
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	c := func(w wave.Wave) float64 {
		mag, binHz := wave.MagnitudeSpectrum(w, 16384, wave.WindowHann)
		return ResidualCentroidHz(mag, binHz, 200)
	}
	if c(mk(true)) <= c(mk(false)) {
		t.Fatal("high-passed residual must have the higher residual centroid")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestOnset|TestResidualCentroid' ./internal/audio/fingerprint/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `onset.go`**

```go
package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// onset.go — what happens in the first tens of milliseconds, and how the
// noise part of a note behaves against its tonal part.
//
// The guide records that every noise source in the modular pipeline injects
// PRE-filter, so a bowed string cannot produce the broadband bite the
// references show (0.20 at onset dropping to ~0.01). onset.bite_ratio is the
// axis that shows whether a bite survived the voice filter.

const (
	onsetFrac        = 0.1  // onset = first frame/sample at this fraction of the max
	onsetBiteWinSec  = 0.030
	onsetRefStartSec = 0.300
	onsetRefEndSec   = 0.600
	onsetFlatWinSec  = 0.020
)

func onsetFrame(tr *Tracks) int {
	mx := 0.0
	for _, v := range tr.RMS {
		mx = math.Max(mx, v)
	}
	for i, v := range tr.RMS {
		if v >= onsetFrac*mx {
			return i
		}
	}
	return 0
}

func OnsetBiteRatio(tr *Tracks) (float64, bool) {
	if tr.NumFrames < 4 || tr.HopSec <= 0 {
		return 0, false
	}
	o := onsetFrame(tr)
	win := func(fromSec, toSec float64) (float64, bool) {
		a := o + int(fromSec/tr.HopSec)
		b := o + int(toSec/tr.HopSec)
		if b <= a {
			b = a + 1
		}
		if a >= tr.NumFrames {
			return 0, false
		}
		if b > tr.NumFrames {
			b = tr.NumFrames
		}
		sum := 0.0
		for i := a; i < b; i++ {
			sum += tr.RMS[i] * tr.RMS[i] * tr.Flatness[i]
		}
		return sum / float64(b-a), true
	}
	bite, ok1 := win(0, onsetBiteWinSec)
	ref, ok2 := win(onsetRefStartSec, onsetRefEndSec)
	if !ok1 || !ok2 || ref <= 1e-12 {
		return 0, false
	}
	return bite / ref, true
}

func onsetSample(w wave.Wave) int {
	pk := w.PeakSample()
	for i, v := range w.Samples {
		if math.Abs(v) >= onsetFrac*pk {
			return i
		}
	}
	return 0
}

func OnsetFlatness(w wave.Wave) (float64, bool) {
	if w.Len() < 2048 {
		return 0, false
	}
	o := onsetSample(w)
	n := int(onsetFlatWinSec * float64(w.SampleRate))
	if o+n > w.Len() {
		return 0, false
	}
	seg := make([]float64, 2048)
	copy(seg, w.Samples[o:o+n])
	mag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: seg, SampleRate: w.SampleRate}, 2048, wave.WindowHann)
	return spectralFlatnessOf(mag), true
}

func OnsetPitchGlideCents(w wave.Wave, f0 float64) (float64, bool) {
	if f0 <= 0 {
		return 0, false
	}
	o := onsetSample(w)
	slice := func(fromSec, toSec float64) (float64, bool) {
		a := o + int(fromSec*float64(w.SampleRate))
		b := o + int(toSec*float64(w.SampleRate))
		if b > w.Len() {
			return 0, false
		}
		f := DetectF0(wave.Wave{Samples: w.Samples[a:b], SampleRate: w.SampleRate})
		if f <= 0 || f < f0/1.9 || f > f0*1.9 {
			return 0, false
		}
		return f, true
	}
	early, ok1 := slice(0.020, 0.060)
	late, ok2 := slice(0.200, 0.320)
	if !ok1 || !ok2 {
		return 0, false
	}
	return 1200 * math.Log2(early/late), true
}

func ResidualCentroidHz(mag []float64, binHz, f0 float64) float64 {
	if f0 <= 0 || binHz <= 0 {
		return spectralCentroidHz(mag, binHz)
	}
	res := make([]float64, len(mag))
	copy(res, mag)
	for k := 1; ; k++ {
		c := int(f0*float64(k)/binHz + 0.5)
		if c >= len(res) {
			break
		}
		for d := -1; d <= 1; d++ {
			if j := c + d; j >= 0 && j < len(res) {
				res[j] = 0
			}
		}
	}
	return spectralCentroidHz(res, binHz)
}

func NoiseDecayVsTone(tr *Tracks) (float64, bool) {
	if tr.NumFrames < 4 || tr.F0Hz <= 0 {
		return 0, false
	}
	resid := make([]float64, tr.NumFrames)
	tone := make([]float64, tr.NumFrames)
	pr, pt := 0, 0
	for i := range resid {
		e := tr.RMS[i] * tr.RMS[i]
		resid[i] = e * tr.NoiseFraction[i]
		tone[i] = e * (1 - tr.NoiseFraction[i])
		if resid[i] > resid[pr] {
			pr = i
		}
		if tone[i] > tone[pt] {
			pt = i
		}
	}
	// decayThreshold works on amplitude-like envelopes; pass sqrt(energy).
	sq := func(v []float64) []float64 {
		out := make([]float64, len(v))
		for i := range v {
			out[i] = math.Sqrt(v[i])
		}
		return out
	}
	rs, tsv := sq(resid), sq(tone)
	tr20, _ := decayThreshold(rs, pr, rs[pr], 20, tr.HopSec)
	tt20, _ := decayThreshold(tsv, pt, tsv[pt], 20, tr.HopSec)
	if tt20 <= 0 {
		return 0, false
	}
	return tr20 / tt20, true
}
```

- [ ] **Step 4: Implement `registry_onset.go`**

```go
package fingerprint

const (
	IDOnsetBite             = "onset.bite_ratio"
	IDOnsetFlatness         = "onset.flatness"
	IDOnsetPitchGlide       = "onset.pitch_glide_cents"
	IDNoiseResidualCentroid = "noise.residual_centroid_hz"
	IDNoiseDecayVsTone      = "noise.decay_vs_tone"
)

const GroupOnset = "onset character"

func init() {
	r := DefaultRegistry()

	r.Register(Descriptor{
		ID: IDOnsetBite, Group: GroupOnset, Name: "onset bite",
		Scope: ScopeSegment, Repr: ReprTime, Kind: KindRatio,
		Drift: 2.0, Mismatch: 4.0,
		Doc: "Broadband energy (frame energy x flatness) in the first 30 ms after onset, relative " +
			"to the same quantity 300-600 ms in. The bow bite, the tonguing 'ttah', the " +
			"stick crack. Needs no fundamental. The shipped bowed strings read near 1 " +
			"(flat) against references that spike — the 'slightly fluty' verdict.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetBiteRatio(b.Tracks())
			if !ok {
				return Unavailable("clip shorter than 600 ms after onset")
			}
			return Scalar(v)
		},
	})
	r.Register(Descriptor{
		ID: IDOnsetFlatness, Group: GroupOnset, Name: "onset flatness",
		Scope: ScopeSegment, Repr: ReprSpectrum, Kind: KindFraction,
		Drift: 0.10, Mismatch: 0.25,
		Doc: "Spectral flatness of the first 20 ms. Noise-led (clap, snare crack) is high; a " +
			"clean pluck or a soft bow start is low.",
		Applies: RequiresAudible,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetFlatness(b.Wave())
			if !ok {
				return Unavailable("clip too short for a 20 ms onset window")
			}
			return Scalar(v)
		},
	})
	r.Register(Descriptor{
		ID: IDOnsetPitchGlide, Group: GroupOnset, Name: "onset pitch glide",
		Scope: ScopeSegment, Repr: ReprTime, Kind: KindRate, Unit: "cents",
		Drift: 10, Mismatch: 30,
		Doc: "Pitch 20-60 ms after onset minus pitch at 200-320 ms, in cents. A plucked string " +
			"starts sharp and settles; a brass lip slurs in; a synth sits dead on pitch.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			v, ok := OnsetPitchGlideCents(b.Wave(), b.Fingerprint().F0Hz)
			if !ok {
				return Unavailable("pitch not detectable in both onset windows")
			}
			return Scalar(v)
		},
	})
	r.Register(Descriptor{
		ID: IDNoiseResidualCentroid, Group: GroupNoise, Name: "residual centroid",
		Scope: ScopeWhole, Repr: ReprSpectrum, Kind: KindHz,
		Drift: 15, Mismatch: 40,
		Doc: "Centroid of the spectrum with +/-1 bin around every harmonic removed: WHERE the " +
			"noise lives. Hiss is high, snare wire and rattle are mid. The snare verdict " +
			"('breathy, fuzzy, hissy') was the wrong KIND of noise, not too much of it.",
		Applies: RequiresPitched,
		Extract: func(b *Bundle) Measurement {
			fp := b.Fingerprint()
			mag, binHz := wave.MagnitudeSpectrum(sustainWindow(b.Wave()), 16384, wave.WindowHann)
			return Scalar(ResidualCentroidHz(mag, binHz, fp.F0Hz))
		},
	})
	r.Register(Descriptor{
		ID: IDNoiseDecayVsTone, Group: GroupNoise, Name: "noise decay vs tone",
		Scope: ScopeTrack, Repr: ReprSpectrum, Kind: KindRatio,
		Drift: 1.5, Mismatch: 3.0,
		Doc: "T20 of the residual (noise) energy track divided by T20 of the harmonic energy " +
			"track. Above 1 the noise outlives the tone ('noisy throughout' — the clap " +
			"verdict); below 1 the noise belongs to the onset.",
		Applies: func(b *Bundle) bool { return RequiresPitched(b) && RequiresAudible(b) },
		Extract: func(b *Bundle) Measurement {
			v, ok := NoiseDecayVsTone(b.Tracks())
			if !ok {
				return Unavailable("no fundamental, or the tonal track never decayed 20 dB")
			}
			return Scalar(v)
		},
	})
}
```

`registry_onset.go` needs `import "github.com/ingyamilmolinar/beatmo/internal/wave"` for `MagnitudeSpectrum`.

- [ ] **Step 5: Run the tests and the full package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/fingerprint/ && ../../.tools/go/bin/go vet ./internal/audio/fingerprint/`
Expected: PASS, no vet output.

- [ ] **Step 6: Live check on violin and clap**

Run: rebuild `refs`; `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" diagnose -instrument violin | grep -E "onset bite|partial independence|modulation bands"`.
Expected: `onset bite` synth far below reference; `partial independence` synth near 0. Then `-instrument clap` shows `noise decay vs tone` and `onset flatness` rows.

- [ ] **Step 7: Confirm the existing gates**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDiagnose|TestDiagnoseClassifierReplay' ./cmd/refs/`
Expected: same pass/fail set as before this plan (`TestDiagnose_ReproducesBonusVerdicts` stays 3/5).

---

### Task 7: Reference-free organic range and `refs organic`

**Files:**
- Create: `src/go/internal/audio/fingerprint/organic.go`
- Create: `src/go/internal/audio/fingerprint/testdata/organic_range.json` (generated; commit-able output of `refs calibrate-organic`)
- Create: `src/go/internal/audio/fingerprint/organic_test.go`
- Create: `src/go/cmd/refs/organic.go`
- Create: `src/go/cmd/refs/organic_test.go`
- Modify: `src/go/cmd/refs/main.go` (dispatch `organic`, `calibrate-organic`; usage lines)

**Interfaces:**
- Consumes: descriptor IDs from Tasks 4–6, `IDNoisePersistence`, `IDVibratoJitter` (check the const name in `registry_timbre.go`; the ID string is `vibrato.jitter`), `IDTrackCrestMedian`; `wavref.Cat`, `wavref.Resolve`, `audio.DecodeWAVToPCM`, `toWave` (diagnose.go:281), `synthmatch.RenderInstrument`, `audio.SampleRate()`.
- Produces:
  ```go
  // fingerprint/organic.go
  func OrganicIDs() []string   // fixed order: mod.am.rate_hz, mod.am.depth, mod.am.flatness, mod.centroid.depth, partial.bandwidth_cents, partial.independence, onset.bite_ratio, onset.flatness, onset.pitch_glide_cents, noise.residual_centroid_hz, noise.decay_vs_tone, noise.persistence, vibrato.jitter, track.crest.median
  type RangeEntry struct { P10, P50, P90 float64; N int }
  type OrganicRange map[string]RangeEntry
  func LoadOrganicRange() OrganicRange           // parses the embedded JSON; empty map when the file is empty
  func (r OrganicRange) Place(id string, v float64) (pos float64, where string, ok bool) // pos=(v-P50)/(P90-P10); where = "below"|"inside"|"above"
  func ComputeOrganicRange(values map[string][]float64) OrganicRange   // percentiles per id; N = len
  // cmd/refs/organic.go
  func runOrganic(w, diag io.Writer, inst string, pitch, dur float64) error      // renders inst, prints the table
  func runCalibrateOrganic(w, diag io.Writer, outPath string) error              // over every locally resolvable Tier-II reference; writes JSON
  ```

- [ ] **Step 1: Write the failing fingerprint tests**

```go
// organic_test.go
package fingerprint

import "testing"

func TestOrganicIDsAreRegistered(t *testing.T) {
	for _, id := range OrganicIDs() {
		if _, ok := DefaultRegistry().ByID(id); !ok {
			t.Errorf("organic id %q is not a registered descriptor", id)
		}
	}
}

func TestOrganicRangePlace(t *testing.T) {
	r := OrganicRange{"x": {P10: 1, P50: 2, P90: 3, N: 10}}
	if pos, where, ok := r.Place("x", 2.5); !ok || where != "inside" || pos != 0.25 {
		t.Fatalf("got %v %s %v", pos, where, ok)
	}
	if _, where, _ := r.Place("x", 0.5); where != "below" {
		t.Fatal("0.5 is below p10")
	}
	if _, where, _ := r.Place("x", 9); where != "above" {
		t.Fatal("9 is above p90")
	}
	if _, _, ok := r.Place("missing", 1); ok {
		t.Fatal("unknown id must not be ok")
	}
}

func TestComputeOrganicRangePercentiles(t *testing.T) {
	vals := map[string][]float64{"x": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
	r := ComputeOrganicRange(vals)
	e := r["x"]
	if e.N != 10 || e.P50 < 5 || e.P50 > 6 || e.P10 > 2 || e.P90 < 9 {
		t.Fatalf("unexpected entry %+v", e)
	}
}

func TestEmbeddedOrganicRangeParses(t *testing.T) {
	_ = LoadOrganicRange() // must not panic; may be empty before calibrate-organic has run
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestOrganic|TestComputeOrganic|TestEmbeddedOrganic' ./internal/audio/fingerprint/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `organic.go` and an empty golden**

Create `testdata/organic_range.json` containing `{}`.

```go
package fingerprint

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// organic.go — the aliveness axes and their real-world range.
//
// Identity axes (mode ratios, formants, envelope) say WHICH instrument this
// is. Organic axes say whether it sounds like a physical thing at all. An
// invented instrument has no reference for the first kind and can still be
// held to the second: its organic values should sit where real recordings
// sit. The range is computed over every real reference in the corpus and
// committed, so `refs organic` needs no reference clip.

//go:embed testdata/organic_range.json
var organicRangeJSON []byte

func OrganicIDs() []string {
	return []string{
		IDModAMRate, IDModAMDepth, IDModAMFlatness, IDModCentroidDepth,
		IDPartialBandwidth, IDPartialIndependence,
		IDOnsetBite, IDOnsetFlatness, IDOnsetPitchGlide,
		IDNoiseResidualCentroid, IDNoiseDecayVsTone,
		IDNoisePersistence, IDVibratoJitter, IDTrackCrestMedian,
	}
}

type RangeEntry struct {
	P10 float64 `json:"p10"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	N   int     `json:"n"`
}

type OrganicRange map[string]RangeEntry

func LoadOrganicRange() OrganicRange {
	out := OrganicRange{}
	if len(organicRangeJSON) == 0 {
		return out
	}
	_ = json.Unmarshal(organicRangeJSON, &out)
	return out
}

func (r OrganicRange) Place(id string, v float64) (float64, string, bool) {
	e, ok := r[id]
	if !ok || e.N == 0 {
		return 0, "", false
	}
	span := e.P90 - e.P10
	pos := 0.0
	if span > 0 {
		pos = (v - e.P50) / span
	}
	switch {
	case v < e.P10:
		return pos, "below", true
	case v > e.P90:
		return pos, "above", true
	}
	return pos, "inside", true
}

func ComputeOrganicRange(values map[string][]float64) OrganicRange {
	out := OrganicRange{}
	for id, vs := range values {
		if len(vs) == 0 {
			continue
		}
		s := append([]float64(nil), vs...)
		sort.Float64s(s)
		out[id] = RangeEntry{P10: percentile(s, 10), P50: percentile(s, 50), P90: percentile(s, 90), N: len(s)}
	}
	return out
}
```

`percentile(sorted, p)` is the existing helper in `vibrato.go:278` and takes **p in 0–100**.

If `IDVibratoJitter` does not exist under that name, look up the const in `registry_timbre.go` whose value is `"vibrato.jitter"` and use it.

- [ ] **Step 4: Run the fingerprint tests**

Run: same command as Step 2. Expected: PASS.

- [ ] **Step 5: Write the failing `cmd/refs` test**

```go
// cmd/refs/organic_test.go
//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

func TestFormatOrganicTable_PlacesEveryOrganicAxis(t *testing.T) {
	r := fingerprint.OrganicRange{}
	for _, id := range fingerprint.OrganicIDs() {
		r[id] = fingerprint.RangeEntry{P10: 0, P50: 1, P90: 2, N: 5}
	}
	vals := map[string]organicValue{}
	for _, id := range fingerprint.OrganicIDs() {
		vals[id] = organicValue{V: 3, OK: true}
	}
	var buf bytes.Buffer
	formatOrganicTable(&buf, "test-inst", vals, r)
	out := buf.String()
	if strings.Count(out, "above") != len(fingerprint.OrganicIDs()) {
		t.Fatalf("every axis should read 'above':\n%s", out)
	}
}
```

- [ ] **Step 6: Implement `cmd/refs/organic.go`**

```go
//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

type organicValue struct {
	V  float64
	OK bool
	NA string
}

func organicValues(b *fingerprint.Bundle) map[string]organicValue {
	out := map[string]organicValue{}
	reg := fingerprint.DefaultRegistry()
	for _, id := range fingerprint.OrganicIDs() {
		d, ok := reg.ByID(id)
		if !ok {
			continue
		}
		if !d.Applies(b) {
			out[id] = organicValue{NA: "does not apply to this signal"}
			continue
		}
		m := d.Extract(b)
		if !m.OK {
			out[id] = organicValue{NA: m.NA}
			continue
		}
		out[id] = organicValue{V: m.Primary(), OK: true}
	}
	return out
}

func formatOrganicTable(w io.Writer, label string, vals map[string]organicValue, r fingerprint.OrganicRange) {
	fmt.Fprintf(w, "=== %s — organic axes against the real-recording range (p10–p90 over %d references) ===\n\n", label, organicN(r))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tVALUE\tREAL p10\tp50\tp90\tWHERE\tPOSITION")
	for _, id := range fingerprint.OrganicIDs() {
		d, _ := fingerprint.DefaultRegistry().ByID(id)
		v := vals[id]
		if !v.OK {
			fmt.Fprintf(tw, "%s\tn/a\t\t\t\t\t%s\n", d.Name, v.NA)
			continue
		}
		e, has := r[id]
		if !has {
			fmt.Fprintf(tw, "%s\t%.3g\t-\t-\t-\tuncalibrated\trun `refs calibrate-organic`\n", d.Name, v.V)
			continue
		}
		pos, where, _ := r.Place(id, v.V)
		fmt.Fprintf(tw, "%s\t%.3g\t%.3g\t%.3g\t%.3g\t%s\t%+.2f\n", d.Name, v.V, e.P10, e.P50, e.P90, where, pos)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nPOSITION = (value - p50) / (p90 - p10). 'below'/'above' = outside the band real recordings occupy.")
}

func organicN(r fingerprint.OrganicRange) int {
	n := 0
	for _, e := range r {
		if e.N > n {
			n = e.N
		}
	}
	return n
}

func runOrganic(w, diag io.Writer, inst string, pitch, dur float64) error {
	if inst == "" {
		return fmt.Errorf("refs organic: -instrument is required")
	}
	sr := audio.SampleRate()
	wv, err := synthmatch.RenderInstrument(inst, pitch, sr, dur)
	if err != nil {
		return fmt.Errorf("refs organic: render %s: %w", inst, err)
	}
	b := fingerprint.NewBundle(wv, inst)
	formatOrganicTable(w, inst, organicValues(b), fingerprint.LoadOrganicRange())
	return nil
}

func defaultOrganicRangePath() (string, error) {
	root, err := wavref.ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "internal", "audio", "fingerprint", "testdata", "organic_range.json"), nil
}

func runCalibrateOrganic(w, diag io.Writer, outPath string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	values := map[string][]float64{}
	n := 0
	for _, ref := range c.Refs {
		if ref.Tier != "instrument" {
			continue
		}
		path, ok := wavref.Resolve(ref.ID)
		if !ok {
			fmt.Fprintf(diag, "skipping %s: not available locally\n", ref.ID)
			continue
		}
		samples, rate, err := audio.DecodeWAVToPCM(path)
		if err != nil {
			fmt.Fprintf(diag, "skipping %s: %v\n", ref.ID, err)
			continue
		}
		b := fingerprint.NewBundle(toWave(samples, rate, ref.ID), ref.ID)
		for id, v := range organicValues(b) {
			if v.OK {
				values[id] = append(values[id], v.V)
			}
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("refs calibrate-organic: no reference could be analyzed (set %s)", wavref.EnvVar)
	}
	r := fingerprint.ComputeOrganicRange(values)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s from %d references\n", outPath, n)
	return nil
}

func cmdOrganic(args []string) error {
	fs := flag.NewFlagSet("organic", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to render and place against the real-recording range")
	pitch := fs.Float64("pitch", 0, "semitones from A3")
	dur := fs.Float64("dur", 2.0, "render duration in seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runOrganic(os.Stdout, os.Stderr, *inst, *pitch, *dur)
}

func cmdCalibrateOrganic(args []string) error {
	fs := flag.NewFlagSet("calibrate-organic", flag.ExitOnError)
	out := fs.String("out", "", "output JSON (default: internal/audio/fingerprint/testdata/organic_range.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := *out
	if p == "" {
		var err error
		if p, err = defaultOrganicRangePath(); err != nil {
			return err
		}
	}
	return runCalibrateOrganic(os.Stdout, os.Stderr, p)
}
```

Check `ref.Tier` value for instrument-tier entries in `internal/wavref/catalogue.json` (grep `"tier"`); use the exact string. Confirm `wavref.ModuleRoot()` exists (memory says it does); otherwise use the same helper `defaultVerdictsPath` uses.

In `main.go` add cases `"organic": cmdOrganic` and `"calibrate-organic": cmdCalibrateOrganic`, and two usage lines:
```
  refs organic    -instrument <inst> [-pitch <semis>] [-dur <sec>]
  refs calibrate-organic [-out <organic_range.json>]
```

- [ ] **Step 7: Run tests, then generate the golden**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestFormatOrganicTable' ./cmd/refs/`
Expected: PASS.

Run: `cd src/go && ../../.tools/go/bin/go build -o "$SCRATCH/refs" ./cmd/refs && BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" calibrate-organic`
Expected: `wrote .../organic_range.json from ~55 references`. Inspect the file: every organic id present with `n >= 30`; `partial.independence` p50 should be clearly above 0.

Run: `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" organic -instrument violin -pitch 12`
Expected: a table; `partial independence` and `onset bite` read `below`.

---

### Task 8: Thresholds from the good group

**Files:**
- Create: `src/go/internal/audio/fingerprint/thresholds.go`
- Create: `src/go/internal/audio/fingerprint/testdata/thresholds.json` (start as `{}`)
- Create: `src/go/internal/audio/fingerprint/thresholds_test.go`
- Modify: `src/go/internal/audio/fingerprint/registry.go` (lazy `ensureCalibrated`)
- Create: `src/go/cmd/refs/calibrate_thresholds.go`
- Modify: `src/go/cmd/refs/validate.go` (`DEMOTED` label), `src/go/cmd/refs/main.go`

**Interfaces:**
- Consumes: `collectValidationSamples`, `axisResult` (`Name`, `Norm`, `Verdict`, `RefNA`, `SynthNA`, and the `ID` field added in Task 9 — do Task 9's `axisResult.ID` addition FIRST if executing out of order; this task keys the JSON by descriptor ID).
- Produces:
  ```go
  type ThresholdEntry struct { Drift, Mismatch float64; N int; Source string }  // Source: "corpus" | "probe"
  type ThresholdFile map[string]ThresholdEntry
  func LoadThresholdFile() ThresholdFile
  func (r *Registry) ensureCalibrated()   // applies the file once, before any All/ByID/Applicable/Filter/SortedByGroup; Register after calibration applies the entry to the new row
  func (r *Registry) CalibratedFrom(id string) string  // "" | "corpus" | "probe" — for the report
  ```
  A descriptor with a file entry gets `Drift`/`Mismatch` replaced. `KindRate` and `KindHz` entries are in the descriptor's own units (the same units its `Signed` carries).

- [ ] **Step 1: Write the failing tests**

```go
// thresholds_test.go
package fingerprint

import "testing"

func TestThresholdFileOverridesRegistryRow(t *testing.T) {
	r := NewRegistry()
	r.Register(Descriptor{ID: "t.x", Group: "g", Name: "x", Doc: "d", Kind: KindFraction,
		Drift: 0.05, Mismatch: 0.15, Applies: AlwaysApplies,
		Extract: func(*Bundle) Measurement { return Scalar(0) }})
	r.thresholdFile = ThresholdFile{"t.x": {Drift: 0.2, Mismatch: 0.4, N: 8, Source: "corpus"}}
	d, _ := r.ByID("t.x")
	if d.Drift != 0.2 || d.Mismatch != 0.4 {
		t.Fatalf("file must override: %+v", d)
	}
	if r.CalibratedFrom("t.x") != "corpus" {
		t.Fatal("CalibratedFrom must report the source")
	}
	// A row registered AFTER calibration is still overridden.
	r.thresholdFile["t.y"] = ThresholdEntry{Drift: 1, Mismatch: 2, N: 3, Source: "probe"}
	r.Register(Descriptor{ID: "t.y", Group: "g", Name: "y", Doc: "d", Kind: KindRatio,
		Applies: AlwaysApplies, Extract: func(*Bundle) Measurement { return Scalar(0) }})
	d, _ = r.ByID("t.y")
	if d.Drift != 1 || d.Mismatch != 2 {
		t.Fatalf("late registration must be calibrated too: %+v", d)
	}
}

func TestEmbeddedThresholdFileParses(t *testing.T) {
	_ = LoadThresholdFile()
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestThresholdFile|TestEmbeddedThreshold' ./internal/audio/fingerprint/`
Expected: FAIL.

- [ ] **Step 3: Implement**

`thresholds.go`:
```go
package fingerprint

import (
	_ "embed"
	"encoding/json"
)

// thresholds.go — Drift/Mismatch values earned from the corpus.
//
// Hand-written thresholds saturated: MFCC profile fired on 98% of
// instruments, spectral irregularity on 100%. `refs calibrate-thresholds`
// takes the ✅-graded families as the definition of "close enough" and sets
// drift = p50 and mismatch = p90 of their |Signed| per descriptor. A probe
// entry (a listener-measured audibility step from `refs probe`) wins over a
// corpus entry.

//go:embed testdata/thresholds.json
var thresholdJSON []byte

type ThresholdEntry struct {
	Drift    float64 `json:"drift"`
	Mismatch float64 `json:"mismatch"`
	N        int     `json:"n"`
	Source   string  `json:"source"`
}

type ThresholdFile map[string]ThresholdEntry

func LoadThresholdFile() ThresholdFile {
	out := ThresholdFile{}
	if len(thresholdJSON) > 0 {
		_ = json.Unmarshal(thresholdJSON, &out)
	}
	return out
}
```

In `registry.go`: add fields `thresholdFile ThresholdFile`, `calibrated bool`, `sources map[string]string` to `Registry`; in `NewRegistry` leave `thresholdFile` nil; in `DefaultRegistry`'s construction set `thresholdFile: LoadThresholdFile()`. Add:

```go
func (r *Registry) applyThreshold(d *Descriptor) {
	if r.thresholdFile == nil {
		return
	}
	if e, ok := r.thresholdFile[d.ID]; ok && e.Mismatch > 0 {
		d.Drift, d.Mismatch = e.Drift, e.Mismatch
		if r.sources == nil {
			r.sources = map[string]string{}
		}
		r.sources[d.ID] = e.Source
	}
}

func (r *Registry) ensureCalibrated() {
	if r.calibrated {
		return
	}
	r.calibrated = true
	for i := range r.order {
		d := r.byID[r.order[i]]
		r.applyThreshold(&d)
		r.byID[r.order[i]] = d
	}
}

func (r *Registry) CalibratedFrom(id string) string {
	r.ensureCalibrated()
	return r.sources[id]
}
```

Read `registry.go:17-60` for the real field names (`order`, `byID` or whatever it uses) and adapt. Call `r.ensureCalibrated()` at the top of `ByID`, `All`, `SortedByGroup`, `Filter`, `Applicable`, and in `Register` call `r.applyThreshold(&d)` when `r.calibrated` is already true. The test sets `r.thresholdFile` directly, so the field must be unexported but on the struct.

`cmd/refs/calibrate_thresholds.go`:
```go
//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// goodGroupThresholds: per descriptor ID, p50/p90 of |Signed| over the
// families the listener graded "y".
func goodGroupThresholds(samples []validationSample) fingerprint.ThresholdFile {
	byID := map[string][]float64{}
	for _, s := range samples {
		if s.Class != classGood {
			continue
		}
		for _, a := range s.Diagnosis.Axes {
			if a.ID == "" || a.RefNA != "" || a.SynthNA != "" {
				continue
			}
			byID[a.ID] = append(byID[a.ID], math.Abs(a.Signed))
		}
	}
	out := fingerprint.ThresholdFile{}
	for id, vs := range byID {
		if len(vs) < 3 {
			continue
		}
		sort.Float64s(vs)
		p := func(q float64) float64 {
			i := int(math.Round(q / 100 * float64(len(vs)-1)))
			return vs[i]
		}
		drift, mismatch := p(50), p(90)
		if mismatch <= drift {
			mismatch = drift * 1.5
		}
		if mismatch <= 0 {
			continue
		}
		out[id] = fingerprint.ThresholdEntry{Drift: drift, Mismatch: mismatch, N: len(vs), Source: "corpus"}
	}
	return out
}

func mergeProbeEntries(fresh, existing fingerprint.ThresholdFile) fingerprint.ThresholdFile {
	for id, e := range existing {
		if e.Source == "probe" {
			fresh[id] = e
		}
	}
	return fresh
}

func defaultThresholdPath() (string, error) {
	root, err := wavref.ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "internal", "audio", "fingerprint", "testdata", "thresholds.json"), nil
}

func runCalibrateThresholds(w, diag io.Writer, verdictsPath, outPath string) error {
	verdicts, err := readVerdicts(verdictsPath)
	if err != nil {
		return err
	}
	samples := collectValidationSamples(diag, verdicts)
	fresh := goodGroupThresholds(samples)
	fresh = mergeProbeEntries(fresh, fingerprint.LoadThresholdFile())
	data, err := json.MarshalIndent(fresh, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s: %d descriptors calibrated from the good group\n", outPath, len(fresh))
	return nil
}

func cmdCalibrateThresholds(args []string) error {
	fs := flag.NewFlagSet("calibrate-thresholds", flag.ExitOnError)
	verdicts := fs.String("verdicts", "", "verdicts jsonl (default: ear_verdicts.jsonl)")
	out := fs.String("out", "", "output JSON (default: internal/audio/fingerprint/testdata/thresholds.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	vp := *verdicts
	if vp == "" {
		var err error
		if vp, err = defaultVerdictsPath(); err != nil {
			return err
		}
	}
	op := *out
	if op == "" {
		var err error
		if op, err = defaultThresholdPath(); err != nil {
			return err
		}
	}
	return runCalibrateThresholds(os.Stdout, os.Stderr, vp, op)
}
```

`axisResult` needs `Signed float64 \`json:"signed"\`` (from `delta.Signed`) and `ID string \`json:"id"\`` — both added in Task 9; if this task is executed first, add them here in `diagnose.go`'s `axisResult` and in the registry loop (`ID: d.ID, Signed: delta.Signed`).

Add a unit test in `calibrate_thresholds_test.go`:

```go
//go:build !js

package main

import "testing"

func TestGoodGroupThresholds_P50P90(t *testing.T) {
	mk := func(class verdictClass, signed float64) validationSample {
		return validationSample{Class: class, Diagnosis: instrumentDiagnosis{Axes: []axisResult{{ID: "a.x", Name: "x", Signed: signed}}}}
	}
	samples := []validationSample{mk(classGood, 1), mk(classGood, 2), mk(classGood, 3), mk(classGood, 10), mk(classBad, 100)}
	f := goodGroupThresholds(samples)
	e, ok := f["a.x"]
	if !ok || e.N != 4 || e.Drift < 2 || e.Drift > 3 || e.Mismatch < 3 {
		t.Fatalf("unexpected %+v ok=%v", e, ok)
	}
}
```

In `validate.go`'s row loop, append `" DEMOTED"` to `flag` when the descriptor (looked up by `a.ID`) has `Demoted` true; and add a `CAL` column printing `fingerprint.DefaultRegistry().CalibratedFrom(id)` or `hand`. `axisScore` gets an `ID string` field filled from `a.ID`.

Dispatch `calibrate-thresholds` in `main.go` and add the usage line.

- [ ] **Step 4: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestThreshold|TestEmbeddedThreshold|TestRegistry' ./internal/audio/fingerprint/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestGoodGroupThresholds' ./cmd/refs/`
Expected: PASS.

- [ ] **Step 5: Generate the file and confirm saturation drops**

Run: rebuild `refs`; `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" calibrate-thresholds` then rebuild (the JSON is embedded at build time) and `"$SCRATCH/refs" validate | grep -c SATURATED`.
Expected: ≤ 2 SATURATED rows (was 6). Also confirm the classifier replay gate: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDiagnoseClassifierReplay' ./cmd/refs/` — this test pushes stored pairs through comparators with the descriptor's thresholds, so it WILL change for calibrated axes. That is expected and intended: regenerate its baseline per its own file's instructions and note in the file's header comment that thresholds are now data-derived (date 2026-09-04). `TestDiagnose_ReproducesListenerVerdicts` (frozen JSON) must remain untouched.

---

### Task 9: Verdict provenance

**Files:**
- Create: `src/go/internal/audio/synth_recipe_hash_export.go`
- Create: `src/go/internal/audio/synth_recipe_hash_export_test.go`
- Modify: `src/go/cmd/refs/audition_verdicts.go` (`RecipeSHA` field), `src/go/cmd/refs/audition.go` (fill it on record), `src/go/cmd/refs/diagnose.go` (`axisResult.ID`, `axisResult.Signed`), `src/go/cmd/refs/validate.go` (STALE)

**Interfaces:**
- Produces:
  ```go
  // internal/audio
  func InstrumentRecipeHash(instID string, overrides map[string]float64) string  // 16-hex of hashRecipeParams(MergeRecipeDefaults(RecipeForInstrument(id), GetInstrumentParams(id)) + overrides); "" when no recipe
  // cmd/refs
  earVerdict.RecipeSHA string `json:"recipe_sha,omitempty"`
  axisResult.ID string `json:"id,omitempty"`; axisResult.Signed float64 `json:"signed"`
  func staleness(v earVerdict) string   // "" | "unhashed" | "STALE"
  ```

- [ ] **Step 1: Failing tests**

```go
// internal/audio/synth_recipe_hash_export_test.go
package audio

import "testing"

func TestInstrumentRecipeHash_StableAndSensitive(t *testing.T) {
	a := InstrumentRecipeHash("oboe", nil)
	b := InstrumentRecipeHash("oboe", nil)
	if a == "" || a != b {
		t.Fatalf("hash must be stable and non-empty: %q %q", a, b)
	}
	c := InstrumentRecipeHash("oboe", map[string]float64{"filter_cutoff": 1234})
	if c == a {
		t.Fatal("an override must change the hash")
	}
	if InstrumentRecipeHash("no-such-instrument", nil) != "" {
		t.Fatal("unknown instrument must hash to empty")
	}
}
```

```go
// cmd/refs/validate_stale_test.go
//go:build !js

package main

import "testing"

func TestStaleness(t *testing.T) {
	if s := staleness(earVerdict{Family: "oboe", Options: []string{"oboe"}}); s != "unhashed" {
		t.Fatalf("no hash → unhashed, got %q", s)
	}
	if s := staleness(earVerdict{Family: "oboe", Options: []string{"oboe"}, Chosen: "oboe", RecipeSHA: "deadbeefdeadbeef"}); s != "STALE" {
		t.Fatalf("mismatching hash → STALE, got %q", s)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestInstrumentRecipeHash' ./internal/audio/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestStaleness ./cmd/refs/`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/audio/synth_recipe_hash_export.go
package audio

import "fmt"

// InstrumentRecipeHash identifies the exact parameter set an instrument
// renders with, so a listening verdict can record what it was heard
// against and `refs validate` can flag it STALE once the seed changes.
func InstrumentRecipeHash(instID string, overrides map[string]float64) string {
	recipeID := RecipeForInstrument(instID)
	if recipeID == "" {
		return ""
	}
	merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(instID))
	if merged == nil {
		merged = RecipeParams{}
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return fmt.Sprintf("%016x", hashRecipeParams(merged))
}
```

`cmd/refs`: add `RecipeSHA string \`json:"recipe_sha,omitempty"\`` to `earVerdict`; in `runAuditionPrompt` where the verdict is built before `recordVerdict`, set `v.RecipeSHA = audio.InstrumentRecipeHash(chosenMember(v), nil)`.

```go
// in validate.go
func staleness(v earVerdict) string {
	if v.RecipeSHA == "" {
		return "unhashed"
	}
	if audio.InstrumentRecipeHash(chosenMember(v), nil) != v.RecipeSHA {
		return "STALE"
	}
	return ""
}
```

In `runValidate`, count `STALE` across verdicts and print after the header: `"%d families STALE (recipe changed since the verdict), %d unhashed (pre-provenance)\n"`, and list the STALE family names.

In `diagnose.go`'s registry loop set `ID: d.ID, Signed: delta.Signed` on `axisResult`; add the two fields to the struct.

- [ ] **Step 4: Run tests and the frozen gates**

Run: the Step 2 commands, then `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDiagnose' ./cmd/refs/`
Expected: PASS (added JSON fields do not affect the frozen baseline's replay).

---

### Task 10: Session record type and the preference test

**Files:**
- Create: `src/go/cmd/refs/session_record.go`
- Create: `src/go/cmd/refs/session_record_test.go`
- Modify: `src/go/cmd/refs/compare.go` (`renderFamilyCompareOverrides`), `src/go/cmd/refs/diagnose.go` (`diagnoseInstrumentOpts`), `src/go/cmd/refs/validate.go` (PREF column, `-sessions` table)

**Interfaces:**
- Produces:
  ```go
  type sessionClip struct { ID string `json:"id"`; RecipeSHA string `json:"recipe_sha"`; Overrides map[string]float64 `json:"overrides,omitempty"` }
  type sessionScale struct { Q string `json:"q"`; Axis string `json:"axis"`; Answer int `json:"answer"` } // -2..+2 after centring; Inverted questions already flipped so +2 always means "synth reads HIGH on axis"
  type sessionCandidate struct {
      sessionClip
      Label string `json:"label"`; Rating int `json:"rating"`; VsCurrent string `json:"vs_current"` // "better"|"same"|"worse"
      Scales []sessionScale `json:"scales,omitempty"`; Adjectives []string `json:"adjectives,omitempty"`
  }
  type earSession struct {
      Session string `json:"session"`; Family string `json:"family"`; Reference string `json:"reference"`
      SemisFromA3 int `json:"semis_from_a3"`; DurSec float64 `json:"dur_sec"`
      Current sessionClip `json:"current"`; Candidates []sessionCandidate `json:"candidates"`; Note string `json:"note,omitempty"`
  }
  const sessionsRel = "internal/audio/testdata/ear_sessions.jsonl"
  func defaultSessionsPath() (string, error)
  func readSessions(path string) ([]earSession, error)   // missing file → nil, nil
  func appendSession(path string, s earSession) error
  // preference pairs
  type prefPair struct { Family, RefID string; Chosen, Rejected sessionClip }
  func prefPairsFromVerdicts(vs []earVerdict) []prefPair   // numbered picks: chosen vs every other option
  func prefPairsFromSessions(ss []earSession) []prefPair   // candidate better-than-current → (cand, current); two candidates with different ratings → (higher, lower)
  // compare.go
  func renderFamilyCompareOverrides(diag io.Writer, insts []string, overrides []map[string]float64, ref wavref.Ref, durFlag float64, match string) ([]compareResult, error)
  // diagnose.go
  func diagnoseInstrumentOpts(diag io.Writer, inst, refID string, durFlag float64, overrides map[string]float64) (instrumentDiagnosis, error)
  ```

- [ ] **Step 1: Failing tests**

```go
// session_record_test.go
//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	s := earSession{Session: "2026-09-04T18:00:00Z", Family: "oboe", Reference: "oboe-A4", SemisFromA3: 12, DurSec: 2,
		Current: sessionClip{ID: "oboe", RecipeSHA: "aa"},
		Candidates: []sessionCandidate{
			{sessionClip: sessionClip{ID: "A", RecipeSHA: "bb", Overrides: map[string]float64{"filter_cutoff": 5000}}, Label: "darker", Rating: 4, VsCurrent: "better",
				Scales: []sessionScale{{Q: "breathiness", Axis: "noise.persistence", Answer: -1}}, Adjectives: []string{"nasal"}},
			{sessionClip: sessionClip{ID: "B", RecipeSHA: "cc"}, Label: "formant", Rating: 2, VsCurrent: "worse"},
		}, Note: "A is close"}
	if err := appendSession(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := readSessions(p)
	if err != nil || len(got) != 1 || got[0].Candidates[0].Overrides["filter_cutoff"] != 5000 || got[0].Candidates[0].Scales[0].Answer != -1 {
		t.Fatalf("round trip failed: %+v err=%v", got, err)
	}
	if _, err := readSessions(filepath.Join(t.TempDir(), "missing.jsonl")); err != nil {
		t.Fatalf("missing file must be nil,nil: %v", err)
	}
	_ = os.Remove(p)
}

func TestPrefPairsFromSessions(t *testing.T) {
	ss := []earSession{{Family: "oboe", Reference: "oboe-A4", Current: sessionClip{ID: "oboe"},
		Candidates: []sessionCandidate{
			{sessionClip: sessionClip{ID: "A"}, Rating: 4, VsCurrent: "better"},
			{sessionClip: sessionClip{ID: "B"}, Rating: 2, VsCurrent: "worse"},
		}}}
	pairs := prefPairsFromSessions(ss)
	// A>current, current>B (worse), A>B (ratings differ) = 3 pairs
	if len(pairs) != 3 {
		t.Fatalf("want 3 pairs, got %d: %+v", len(pairs), pairs)
	}
}

func TestPrefPairsFromVerdicts(t *testing.T) {
	vs := []earVerdict{{Family: "flute", Reference: "flute-A4", Options: []string{"flute", "flute-breathy"}, Verdict: "2", Chosen: "flute-breathy"}}
	pairs := prefPairsFromVerdicts(vs)
	if len(pairs) != 1 || pairs[0].Chosen.ID != "flute-breathy" || pairs[0].Rejected.ID != "flute" {
		t.Fatalf("got %+v", pairs)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSessionRoundTrip|TestPrefPairs' ./cmd/refs/`
Expected: FAIL.

- [ ] **Step 3: Implement `session_record.go`**

```go
//go:build !js

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

const sessionsRel = "internal/audio/testdata/ear_sessions.jsonl"

type sessionClip struct {
	ID        string             `json:"id"`
	RecipeSHA string             `json:"recipe_sha"`
	Overrides map[string]float64 `json:"overrides,omitempty"`
}

type sessionScale struct {
	Q      string `json:"q"`
	Axis   string `json:"axis"`
	Answer int    `json:"answer"`
}

type sessionCandidate struct {
	sessionClip
	Label      string         `json:"label"`
	Rating     int            `json:"rating"`
	VsCurrent  string         `json:"vs_current"`
	Scales     []sessionScale `json:"scales,omitempty"`
	Adjectives []string       `json:"adjectives,omitempty"`
}

type earSession struct {
	Session     string             `json:"session"`
	Family      string             `json:"family"`
	Reference   string             `json:"reference"`
	SemisFromA3 int                `json:"semis_from_a3"`
	DurSec      float64            `json:"dur_sec"`
	Current     sessionClip        `json:"current"`
	Candidates  []sessionCandidate `json:"candidates"`
	Note        string             `json:"note,omitempty"`
}

func defaultSessionsPath() (string, error) {
	root, err := wavref.ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, sessionsRel), nil
}

func readSessions(path string) ([]earSession, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []earSession
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var s earSession
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("refs session: %s: %w", path, err)
		}
		out = append(out, s)
	}
	return out, sc.Err()
}

func appendSession(path string, s earSession) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

type prefPair struct {
	Family, RefID    string
	Chosen, Rejected sessionClip
}

func prefPairsFromVerdicts(vs []earVerdict) []prefPair {
	var out []prefPair
	for _, v := range vs {
		if v.Chosen == "" || len(v.Options) < 2 {
			continue
		}
		if v.Verdict != "1" && v.Verdict != "2" && v.Verdict != "3" {
			continue
		}
		for _, o := range v.Options {
			if o != v.Chosen {
				out = append(out, prefPair{Family: v.Family, RefID: v.Reference, Chosen: sessionClip{ID: v.Chosen}, Rejected: sessionClip{ID: o}})
			}
		}
	}
	return out
}

func prefPairsFromSessions(ss []earSession) []prefPair {
	var out []prefPair
	for _, s := range ss {
		for i, c := range s.Candidates {
			switch c.VsCurrent {
			case "better":
				out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: c.sessionClip, Rejected: s.Current})
			case "worse":
				out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: s.Current, Rejected: c.sessionClip})
			}
			for j := i + 1; j < len(s.Candidates); j++ {
				d := s.Candidates[j]
				switch {
				case c.Rating > d.Rating:
					out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: c.sessionClip, Rejected: d.sessionClip})
				case d.Rating > c.Rating:
					out = append(out, prefPair{Family: s.Family, RefID: s.Reference, Chosen: d.sessionClip, Rejected: c.sessionClip})
				}
			}
		}
	}
	return out
}
```

Candidate clips carry a base instrument: for a session candidate the render target is `s.Current.ID` with `c.Overrides` applied. Add `Base string \`json:"base,omitempty"\`` to `sessionClip`, set to the family's base instrument for candidates, and have the render helper use `Base` when non-empty (else `ID`).

- [ ] **Step 4: Overrides plumbing in `compare.go` and `diagnose.go`**

In `compare.go`: rename the body of `renderFamilyCompare` into `renderFamilyCompareOverrides(diag, insts, overrides, ref, durFlag, match)`; inside the render loop use `synthmatch.RenderWithParams(inst, pitch, sr, dur, ov)` where `ov = overrides[i]` if `overrides != nil` else `nil`; the WAV filename for an override render becomes `SYNTH__<inst>__<8-hex of audio.InstrumentRecipeHash(inst, ov)>.wav` so candidates do not overwrite the current render. `renderFamilyCompare` becomes `return renderFamilyCompareOverrides(diag, insts, nil, ref, durFlag, match)`. `renderCompare` gains an overrides variant `renderCompareOverrides(diag, inst string, ov map[string]float64, ref, durFlag, match)` delegating to the family version with one element.

In `diagnose.go`: `diagnoseInstrumentDur(diag, inst, refID, durFlag)` becomes a one-line delegate to `diagnoseInstrumentOpts(diag, inst, refID, durFlag, nil)`; the new function uses `renderCompareOverrides` when `refID != ""`, and when `refID == ""` with overrides, resolves the first candidate via `compareCandidates(c, inst)[0]` and calls `renderCompareOverrides`.

- [ ] **Step 5: PREF column and `-sessions` table in `validate.go`**

Add to `axisScore`: `PrefAgree, PrefTotal int`. In `runValidate`, after `samples`:

```go
	sessions, _ := readSessions(sessionsPath)
	pairs := append(prefPairsFromVerdicts(verdicts), prefPairsFromSessions(sessions)...)
	prefByID := scorePreferences(diag, pairs)   // map[descriptorID]*[2]int{agree,total}
```

```go
// scorePreferences diagnoses both clips of every pair against the pair's
// reference and, per descriptor, counts the pairs where the chosen clip's
// Norm is strictly smaller than the rejected clip's.
func scorePreferences(diag io.Writer, pairs []prefPair) map[string][2]int {
	out := map[string][2]int{}
	normsOf := func(p prefPair, c sessionClip) map[string]float64 {
		inst := c.ID
		if c.Base != "" {
			inst = c.Base
		}
		d, err := diagnoseInstrumentOpts(io.Discard, inst, p.RefID, 0, c.Overrides)
		if err != nil {
			fmt.Fprintf(diag, "pref: skipping %s/%s: %v\n", p.Family, c.ID, err)
			return nil
		}
		m := map[string]float64{}
		for _, a := range d.Axes {
			if a.ID != "" && a.RefNA == "" && a.SynthNA == "" {
				m[a.ID] = a.Norm
			}
		}
		return m
	}
	for _, p := range pairs {
		ch, rj := normsOf(p, p.Chosen), normsOf(p, p.Rejected)
		if ch == nil || rj == nil {
			continue
		}
		for id, cn := range ch {
			rn, ok := rj[id]
			if !ok {
				continue
			}
			e := out[id]
			e[1]++
			if cn < rn {
				e[0]++
			}
			out[id] = e
		}
	}
	return out
}
```

In the table add a `PREF` column formatted `agree/total` (or `-` when total is 0); when `total >= 6 && agree/total >= 0.75` append ` PREDICTIVE(pref)` to the flag. Add a `-sessions <path>` flag (default `defaultSessionsPath()`).

Add `runValidateSessions(w, diag, sessionsPath)` behind a `-sessions-table` flag: for every session candidate and every `sessionScale` with `Answer != 0`, diagnose the candidate (`diagnoseInstrumentOpts(..., c.Overrides)`), find the axis by `Axis` ID, and count agreement: `Answer > 0` agrees when `Direction == "high"`, `Answer < 0` when `Direction == "low"`; print per axis `AGREE/TOTAL` plus per-row detail like `validate -terms` does.

- [ ] **Step 6: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/`
Expected: PASS (existing tests plus the new ones).

- [ ] **Step 7: Live: the seven existing preference families**

Run: rebuild `refs`; `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" validate | head -30`
Expected: a `PREF` column with totals of 7–9 for most axes (conga has two rejected siblings). Note which axes agree ≥ 6/7; those are the first trusted set for Task 15.

---

### Task 11: Word rules rebound to the new axes

**Files:**
- Modify: `src/go/cmd/refs/validate_terms.go` (`termRules`)
- Modify: `src/go/cmd/refs/validate_terms_test.go` (if it pins the rule count; otherwise create a small test)

**Interfaces:** `termRule{Words, Axis, WantDir, Why}` where `Axis` is the descriptor **Name** (see `registryHasAxis(name)` at validate_terms.go:225). The new axes' Names are: `mode harmonicity`, `beating depth`, `band T60`, `decay T20`, `spectral flatness`, `onset bite`, `first formant`, `noise persistence`, `brightness ratio`.

- [ ] **Step 1: Failing test**

```go
// validate_terms_rules_test.go
//go:build !js

package main

import "testing"

func TestTermRulesBindToRegisteredAxes(t *testing.T) {
	if len(termRules) != 11 {
		t.Fatalf("spec §2.4 defines 11 rules, got %d", len(termRules))
	}
	for _, r := range termRules {
		if !registryHasAxis(r.Axis) {
			t.Errorf("rule %v names unknown axis %q", r.Words, r.Axis)
		}
		if r.WantDir != "high" && r.WantDir != "low" {
			t.Errorf("rule %v: WantDir must be high|low", r.Words)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `-run TestTermRulesBindToRegisteredAxes ./cmd/refs/` FAILS on the count.

- [ ] **Step 3: Replace `termRules`**

```go
var termRules = []termRule{
	{Words: []string{"airy", "breathy", "wind instrument", "like a flute"}, Axis: "noise persistence", WantDir: "high",
		Why: "continuous noise through the note is what makes a bowed string read as blown"},
	{Words: []string{"warmer", "fuller", "needs warmness", "too bright", "pitched up or bright"}, Axis: "brightness ratio", WantDir: "high",
		Why: "asking for warmth means the synth currently sits too far above its own pitch"},
	{Words: []string{"too dark", "dark and mellow", "duller"}, Axis: "brightness ratio", WantDir: "low",
		Why: "the opposite complaint on the same axis"},
	{Words: []string{"wooden", "marimba"}, Axis: "mode harmonicity", WantDir: "high",
		Why: "a struck bar's modes are NOT integer multiples; a synth that reads as a plucked string is too harmonic"},
	{Words: []string{"metallic", "bell", "resonant", "wavy", "waves in time"}, Axis: "beating depth", WantDir: "low",
		Why: "a bell's near-coincident partials beat; a clean additive tone does not"},
	{Words: []string{"longer decay", "let it ring", "prolongs"}, Axis: "band T60", WantDir: "low",
		Why: "a ring the listener wants extended is currently too short, band by band"},
	{Words: []string{"too sustained", "ghost", "lingers"}, Axis: "decay T20", WantDir: "high",
		Why: "the synth outlives the reference"},
	{Words: []string{"hissy", "fuzzy"}, Axis: "spectral flatness", WantDir: "low",
		Why: "hiss is coloured, band-limited noise; wire is broadband"},
	{Words: []string{"rattly", "crack", "snap"}, Axis: "onset bite", WantDir: "low",
		Why: "the rattle and the crack are broadband onset energy the synth lacks"},
	{Words: []string{"fluty"}, Axis: "onset bite", WantDir: "low",
		Why: "a bowed string without its bow bite reads as blown"},
	{Words: []string{"nasal"}, Axis: "first formant", WantDir: "low",
		Why: "nasality is a raised low formant the synth lacks"},
}
```

- [ ] **Step 4: Run and check agreement**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTermRules|TestScoreTerms|TestMatchTerm' ./cmd/refs/`, then rebuild and `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" validate -terms | head -20`.
Expected: the summary table shows AGREE == FIRED on ≥ 8 of the rules that have at least one family (spec success criterion 2). If "wooden" still MISSES kalimba, print `diagnose -instrument kalimba | grep "mode harmonicity"` — the synth must read HIGHER than the reference; if the tracker found too few synth modes, lower `modeStrongWithin` for that check is NOT the fix — inspect the render's spectrum for the KS pluck dominating (the known dead `gen_gain` case in the guide).

---

### Task 12: Stage ablation

**Files:**
- Create: `src/go/internal/audio/synthdiag/ablate.go` (`//go:build !test && !js`)
- Create: `src/go/internal/audio/synthdiag/stages.go` (no build tag; pure table)
- Create: `src/go/internal/audio/synthdiag/stages_test.go` (fast path)
- Create: `src/go/internal/audio/synthdiag/ablate_test.go` (`//go:build !test && !js`)
- Modify: `src/go/cmd/refs/diagnose.go` (`-ablate`)

**Interfaces:**
- Consumes: `audio.RecipeForInstrument`, `audio.MergeRecipeDefaults`, `audio.GetInstrumentParams`, `synthmatch.RenderWithParams`, `fingerprint.NewBundle`, `DefaultRegistry().Applicable`, `Descriptor.CompareWith`.
- Produces:
  ```go
  type Stage struct { Name string; Overrides map[string]float64; Active func(merged map[string]float64) bool }
  func Stages() []Stage
  func ActiveStages(merged map[string]float64) []Stage
  type Report struct {
      Instrument string
      Stages     []string   // ablated stage names, in Stages() order
      Axes       []string   // descriptor IDs applicable to the full render
      Names      []string   // descriptor Names, same order
      Sens       [][]float64 // [stage][axis] Delta.Norm of (full vs ablated); NaN when not comparable
  }
  func Ablate(instID string, pitch float64, sr int, durSec float64, base map[string]float64) (Report, error)
  type StageSens struct { Stage string; Norm float64 }
  func (r Report) TopStages(axisID string, n int) []StageSens
  ```

- [ ] **Step 1: Failing fast-path test for the table**

```go
// stages_test.go
package synthdiag

import "testing"

func TestStages_GenSlotsAblateBySourceNotGain(t *testing.T) {
	for _, s := range Stages() {
		if len(s.Name) >= 3 && s.Name[:3] == "gen" {
			if _, ok := s.Overrides[s.Name+"_source"]; !ok {
				t.Errorf("%s must ablate via %s_source=0 (gain 0 means 'unconfigured' for transplanted sources)", s.Name, s.Name)
			}
			if _, bad := s.Overrides[s.Name+"_gain"]; bad {
				t.Errorf("%s must not touch _gain", s.Name)
			}
		}
	}
}

func TestActiveStages_OnlyActiveOnes(t *testing.T) {
	merged := map[string]float64{"osc_enabled": 1, "gen1_source": 1, "gen2_source": 0, "unison_voices": 1, "body_mix": 0, "lfo_enabled": 1}
	names := map[string]bool{}
	for _, s := range ActiveStages(merged) {
		names[s.Name] = true
	}
	for _, want := range []string{"osc", "gen1", "lfo"} {
		if !names[want] {
			t.Errorf("%s should be active", want)
		}
	}
	for _, no := range []string{"gen2", "ensemble", "resonator"} {
		if names[no] {
			t.Errorf("%s should be inactive", no)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/synthdiag/` FAILS (no package).

- [ ] **Step 3: Implement `stages.go`**

```go
package synthdiag

import "fmt"

// stages.go — what "this stage at identity" means, per synthesis stage.
//
// Several Synth-tab stages are identity BY VALUE, not by a flag: ENSEMBLE is
// unison_voices=1 with zero detune/drift, RESONATOR is body_mix=0, a gen
// slot is genN_source=0. A gen slot is deliberately NOT ablated through
// genN_gain: for the transplanted sources (KS, analytic, kick, tom, snare,
// clap, cymbal, FM) gain 0 means "unconfigured", so it would not silence
// them; source 0 does.

type Stage struct {
	Name      string
	Overrides map[string]float64
	Active    func(merged map[string]float64) bool
}

func flagOn(key string) func(map[string]float64) bool {
	return func(m map[string]float64) bool { return m[key] > 0.5 }
}

func Stages() []Stage {
	s := []Stage{
		{"osc", map[string]float64{"osc_enabled": 0}, flagOn("osc_enabled")},
	}
	for i := 1; i <= 8; i++ {
		src := fmt.Sprintf("gen%d_source", i)
		s = append(s, Stage{fmt.Sprintf("gen%d", i), map[string]float64{src: 0}, flagOn(src)})
	}
	s = append(s,
		Stage{"ensemble", map[string]float64{"unison_voices": 1, "unison_detune": 0, "unison_drift_rate": 0, "unison_drift_depth": 0},
			func(m map[string]float64) bool { return m["unison_voices"] > 1.5 }},
		Stage{"fm", map[string]float64{"fm_enabled": 0}, flagOn("fm_enabled")},
		Stage{"pitchenv", map[string]float64{"pitchenv_enabled": 0}, flagOn("pitchenv_enabled")},
		Stage{"lfo", map[string]float64{"lfo_enabled": 0}, flagOn("lfo_enabled")},
		Stage{"burst", map[string]float64{"burst_enabled": 0}, flagOn("burst_enabled")},
		Stage{"filter", map[string]float64{"filter_enabled": 0}, flagOn("filter_enabled")},
		Stage{"filtenv", map[string]float64{"filtenv_enabled": 0}, flagOn("filtenv_enabled")},
		Stage{"formant", map[string]float64{"formant_enabled": 0}, flagOn("formant_enabled")},
		Stage{"resonator", map[string]float64{"body_mix": 0}, func(m map[string]float64) bool { return m["body_mix"] > 0.01 }},
		Stage{"bow", map[string]float64{"bow_dynamics": 0}, func(m map[string]float64) bool { return m["bow_dynamics"] > 0.01 }},
		Stage{"drive", map[string]float64{"drive_enabled": 0}, flagOn("drive_enabled")},
		Stage{"post", map[string]float64{"post_enabled": 0}, flagOn("post_enabled")},
		Stage{"kick", map[string]float64{"kick_enabled": 0}, flagOn("kick_enabled")},
	)
	return s
}

func ActiveStages(merged map[string]float64) []Stage {
	var out []Stage
	for _, s := range Stages() {
		if s.Active(merged) {
			out = append(out, s)
		}
	}
	return out
}
```

Verify every override key is a real `ParamDef` name: `grep -o 'Name: "[a-z0-9_]*"' internal/audio/modular_recipe.go | grep -E 'osc_enabled|fm_enabled|pitchenv_enabled|lfo_enabled|burst_enabled|filter_enabled|filtenv_enabled|formant_enabled|drive_enabled|post_enabled|kick_enabled|body_mix|bow_dynamics|unison_'`. `post_enabled` appears in the range table, not as a `ParamDef` row — if `ParamDef` lookup is used anywhere, keep only keys that exist; otherwise the override is harmless (merge accepts unknown keys) but add a test asserting each key is in `audio.ModularSynthParamDefs()` or the range table.

- [ ] **Step 4: Implement `ablate.go`**

```go
//go:build !test && !js

package synthdiag

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

type Report struct {
	Instrument string
	Stages     []string
	Axes       []string
	Names      []string
	Sens       [][]float64
}

type StageSens struct {
	Stage string
	Norm  float64
}

func rms(w wave.Wave) float64 {
	var s float64
	for _, v := range w.Samples {
		s += v * v
	}
	if len(w.Samples) == 0 {
		return 0
	}
	return math.Sqrt(s / float64(len(w.Samples)))
}

func matchRMS(target, w wave.Wave) wave.Wave {
	tr, wr := rms(target), rms(w)
	if wr <= 1e-9 {
		return w
	}
	g := tr / wr
	out := wave.Wave{Samples: make([]float64, len(w.Samples)), SampleRate: w.SampleRate, Label: w.Label}
	for i, v := range w.Samples {
		out.Samples[i] = v * g
	}
	return out
}

// Ablate renders instID in full, then once per active stage with that stage
// at identity, RMS-matches each ablation to the full render and reports
// every applicable descriptor's Delta.Norm. Sensitivity, not contribution:
// stages interact, so the numbers do not sum.
func Ablate(instID string, pitch float64, sr int, durSec float64, base map[string]float64) (Report, error) {
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return Report{}, fmt.Errorf("synthdiag: no recipe for %q", instID)
	}
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	if merged == nil {
		merged = audio.RecipeParams{}
	}
	for k, v := range base {
		merged[k] = v
	}
	full, err := synthmatch.RenderWithParams(instID, pitch, sr, durSec, base)
	if err != nil {
		return Report{}, err
	}
	fullB := fingerprint.NewBundle(full, instID)
	descs := fingerprint.DefaultRegistry().Applicable(fullB)
	rep := Report{Instrument: instID}
	fullM := make([]fingerprint.Measurement, len(descs))
	for i, d := range descs {
		rep.Axes = append(rep.Axes, d.ID)
		rep.Names = append(rep.Names, d.Name)
		fullM[i] = d.Extract(fullB)
	}
	for _, st := range ActiveStages(map[string]float64(merged)) {
		ov := map[string]float64{}
		for k, v := range base {
			ov[k] = v
		}
		for k, v := range st.Overrides {
			ov[k] = v
		}
		w, err := synthmatch.RenderWithParams(instID, pitch, sr, durSec, ov)
		if err != nil {
			return Report{}, fmt.Errorf("synthdiag: ablate %s: %w", st.Name, err)
		}
		b := fingerprint.NewBundle(matchRMS(full, w), instID+"-"+st.Name)
		row := make([]float64, len(descs))
		for i, d := range descs {
			delta := d.CompareWith(fullM[i], d.Extract(b))
			if !delta.OK {
				row[i] = math.NaN()
				continue
			}
			row[i] = delta.Norm
		}
		rep.Stages = append(rep.Stages, st.Name)
		rep.Sens = append(rep.Sens, row)
	}
	return rep, nil
}

func (r Report) TopStages(axisID string, n int) []StageSens {
	col := -1
	for i, id := range r.Axes {
		if id == axisID {
			col = i
			break
		}
	}
	if col < 0 {
		return nil
	}
	var out []StageSens
	for si, name := range r.Stages {
		v := r.Sens[si][col]
		if !math.IsNaN(v) {
			out = append(out, StageSens{name, v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Norm > out[j].Norm })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
```

Check whether `audio.RecipeParams` is `map[string]float64` (synth_recipe.go:53 says yes) so the `map[string]float64(merged)` conversion compiles.

- [ ] **Step 5: CGo reproduction test**

```go
// ablate_test.go
//go:build !test && !js

package synthdiag

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The violin "too much bass" bug was found by hand by toggling stages and
// measuring energy below 1 kHz; the body resonator's low modes were the
// culprit. The ablation matrix must rediscover that the resonator is the
// top-leverage stage for brightness ratio.
func TestAblate_ViolinResonatorDrivesBrightness(t *testing.T) {
	rep, err := Ablate("violin", 12, 44100, 2.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	top := rep.TopStages(fingerprint.IDBrightnessRatio, 3)
	if len(top) == 0 {
		t.Fatal("no stage sensitivity for brightness ratio")
	}
	found := false
	for _, s := range top {
		if s.Stage == "resonator" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resonator must be in the top 3 for brightness ratio, got %+v", top)
	}
}
```

Run: `make build/libdrums.a && cd src/go && ../../.tools/go/bin/go test -run TestAblate ./internal/audio/synthdiag/`
Expected: PASS. If `resonator` is not active on violin (check `body_mix` in `violinSeed`), the test premise is wrong — report that rather than loosening the assertion.

- [ ] **Step 6: `refs diagnose -ablate`**

In `cmd/refs/diagnose.go`, add flag `ablate := fs.Bool("ablate", false, "after the report, render each synthesis stage at identity and print which stages move every MISMATCH axis")`. After `printDiagnosis`, when set:

```go
func printAblation(w io.Writer, d instrumentDiagnosis, rep synthdiag.Report) {
	fmt.Fprintln(w, "\n-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tTOP STAGES (Δnorm when the stage is at identity)")
	for _, a := range d.Axes {
		if a.Verdict != verdictMismatch || a.ID == "" {
			continue
		}
		top := rep.TopStages(a.ID, 3)
		if len(top) == 0 {
			fmt.Fprintf(tw, "%s\t(no stage moves it — engine-level)\n", a.Name)
			continue
		}
		parts := make([]string, len(top))
		for i, s := range top {
			parts[i] = fmt.Sprintf("%s %.2f", s.Stage, s.Norm)
		}
		fmt.Fprintf(tw, "%s\t%s\n", a.Name, strings.Join(parts, "  "))
	}
	tw.Flush()
}
```

The render pitch and duration must match the diagnosis: expose them on `instrumentDiagnosis` (`Pitch`, `DurSec`, `SR` fields filled from `compareResult`) so `synthdiag.Ablate(inst, d.Pitch, d.SR, d.DurSec, overrides)` reproduces the same render. With `-all -ablate` print the full matrix per instrument (`Stages` × `Names`, `%.2f`, `-` for NaN).

- [ ] **Step 7: Live check**

Run: rebuild; `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" diagnose -instrument tubular-bells -ablate | tail -25`
Expected: a stage-sensitivity block naming a gen slot or `lfo` for `beating depth` / `loudness modulation depth`. Keep this output; Task 15 uses it.

---

### Task 13: Candidates file, loader and liveness test

**Files:**
- Create: `src/go/internal/audio/testdata/audition_candidates.json` (skeleton now; Task 15 fills the overrides)
- Create: `src/go/cmd/refs/session_candidates.go`
- Create: `src/go/cmd/refs/session_candidates_test.go` (fast path: schema)
- Create: `src/go/cmd/refs/session_candidates_live_test.go` (`//go:build !test && !js`: liveness)

**Interfaces:**
- Produces:
  ```go
  type candidateSpec struct { ID, Label, Hypothesis string; Overrides map[string]float64 }
  type familyCandidates struct { Reference, Base string; Candidates []candidateSpec }
  const candidatesRel = "internal/audio/testdata/audition_candidates.json"
  func defaultCandidatesPath() (string, error)
  func loadCandidates(path string) (map[string]familyCandidates, error)  // validates: >=1 candidate, unique IDs, non-empty Label+Hypothesis, Base and Reference non-empty
  ```

- [ ] **Step 1: Skeleton JSON**

```json
{
  "tubular-bells": {"reference": "tubular-bells-vcsl", "base": "tubular-bells", "candidates": [
    {"id": "A", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"gain": 0.79}},
    {"id": "B", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"gain": 0.81}}
  ]},
  "snare": {"reference": "snare-vcsl", "base": "snare", "candidates": [
    {"id": "A", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"snare_noise_mix": 0.57}},
    {"id": "B", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"snare_noise_mix": 0.55}}
  ]},
  "oboe": {"reference": "oboe-A4", "base": "oboe", "candidates": [
    {"id": "A", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"filter_cutoff": 6900}},
    {"id": "B", "label": "TBD-by-task-15", "hypothesis": "TBD-by-task-15", "overrides": {"filter_cutoff": 7100}}
  ]}
}
```

The placeholder labels are deliberate and are replaced in Task 15; the schema test below rejects the literal string `TBD-by-task-15` so the plan cannot end with them in place.

- [ ] **Step 2: Failing tests**

```go
// session_candidates_test.go
//go:build !js

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCandidates_ValidatesShape(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"x":{"reference":"r","base":"b","candidates":[{"id":"A","label":"l","hypothesis":"h","overrides":{"k":1}},{"id":"A","label":"l","hypothesis":"h","overrides":{}}]}}`), 0o644)
	if _, err := loadCandidates(p); err == nil {
		t.Fatal("duplicate candidate id must be rejected")
	}
	os.WriteFile(p, []byte(`{"x":{"reference":"r","base":"b","candidates":[{"id":"A","label":"l","hypothesis":"h","overrides":{"k":1}}]}}`), 0o644)
	got, err := loadCandidates(p)
	if err != nil || got["x"].Candidates[0].Overrides["k"] != 1 {
		t.Fatalf("valid file failed: %v %+v", err, got)
	}
}

func TestShippedCandidatesFile_NoPlaceholders(t *testing.T) {
	p, err := defaultCandidatesPath()
	if err != nil {
		t.Fatal(err)
	}
	fams, err := loadCandidates(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range fams {
		if len(f.Candidates) != 2 {
			t.Errorf("%s: spec requires exactly two candidates, got %d", name, len(f.Candidates))
		}
		for _, c := range f.Candidates {
			if c.Label == "TBD-by-task-15" || c.Hypothesis == "TBD-by-task-15" {
				t.Errorf("%s/%s still carries the Task 13 placeholder; Task 15 must replace it", name, c.ID)
			}
		}
	}
}
```

```go
// session_candidates_live_test.go
//go:build !test && !js

package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
)

// Every candidate must (a) use only keys the base's recipe knows and (b)
// render differently from the base — a candidate cannot silently be the
// current version, and a dead override key cannot ship (118 such keys are
// already on the dead-seed ratchet).
func TestShippedCandidates_AreLive(t *testing.T) {
	p, _ := defaultCandidatesPath()
	fams, err := loadCandidates(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range fams {
		recipeID := audio.RecipeForInstrument(f.Base)
		known := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(f.Base))
		base, err := synthmatch.RenderInstrument(f.Base, 0, 44100, 1.0)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range f.Candidates {
			for k := range c.Overrides {
				if _, ok := known[k]; !ok {
					t.Errorf("%s/%s: override %q is not a parameter of recipe %s", name, c.ID, k, recipeID)
				}
			}
			w, err := synthmatch.RenderWithParams(f.Base, 0, 44100, 1.0, c.Overrides)
			if err != nil {
				t.Fatal(err)
			}
			diff := 0.0
			for i := range w.Samples {
				diff += math.Abs(w.Samples[i] - base.Samples[i])
			}
			if diff < 1e-6 {
				t.Errorf("%s/%s renders byte-identical to the base — overrides are dead", name, c.ID)
			}
		}
	}
}
```

- [ ] **Step 3: Run to verify failure** — the fast test fails on `loadCandidates` undefined.

- [ ] **Step 4: Implement `session_candidates.go`**

```go
//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

const candidatesRel = "internal/audio/testdata/audition_candidates.json"

type candidateSpec struct {
	ID         string             `json:"id"`
	Label      string             `json:"label"`
	Hypothesis string             `json:"hypothesis"`
	Overrides  map[string]float64 `json:"overrides"`
}

type familyCandidates struct {
	Reference  string          `json:"reference"`
	Base       string          `json:"base"`
	Candidates []candidateSpec `json:"candidates"`
}

func defaultCandidatesPath() (string, error) {
	root, err := wavref.ModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, candidatesRel), nil
}

func loadCandidates(path string) (map[string]familyCandidates, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]familyCandidates
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("refs session: %s: %w", path, err)
	}
	for name, f := range out {
		if f.Reference == "" || f.Base == "" {
			return nil, fmt.Errorf("refs session: %s: reference and base are required", name)
		}
		if len(f.Candidates) == 0 {
			return nil, fmt.Errorf("refs session: %s: at least one candidate is required", name)
		}
		seen := map[string]bool{}
		for _, c := range f.Candidates {
			if c.ID == "" || c.Label == "" || c.Hypothesis == "" {
				return nil, fmt.Errorf("refs session: %s: every candidate needs id, label and hypothesis", name)
			}
			if seen[c.ID] {
				return nil, fmt.Errorf("refs session: %s: duplicate candidate id %q", name, c.ID)
			}
			seen[c.ID] = true
			if len(c.Overrides) == 0 {
				return nil, fmt.Errorf("refs session: %s/%s: overrides are empty (would equal the base)", name, c.ID)
			}
		}
	}
	return out, nil
}
```

- [ ] **Step 5: Run**

Fast: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLoadCandidates' ./cmd/refs/` → PASS. `TestShippedCandidatesFile_NoPlaceholders` FAILS until Task 15 — expected, leave it red.
Live: `cd src/go && ../../.tools/go/bin/go test -run TestShippedCandidates_AreLive ./cmd/refs/` → must PASS with the skeleton (it proves `snare_noise_mix`, `filter_cutoff`, `gain` are live keys). If `snare_noise_mix` is unknown to `drum-snare`'s merged params, list `known` keys in the failure and pick the wire-mix key from `snareVariantSpecs["drum-snare"].lit` (snare_modular_push.go:175) instead.

---

### Task 14: Player, question table and the session flow

**Files:**
- Create: `src/go/cmd/refs/player.go`, `player_test.go`
- Create: `src/go/cmd/refs/session_questions.go`, `session_questions_test.go`
- Create: `src/go/cmd/refs/session.go`, `session_test.go`
- Modify: `src/go/cmd/refs/main.go` (dispatch `session`), `Makefile` (`ab-session`)

**Interfaces:**
- Consumes: Task 10 (`earSession`, `appendSession`, `renderFamilyCompareOverrides`), Task 13 (`loadCandidates`), `audio.InstrumentRecipeHash`, `loadCatalogue`, `c.Ref(id)`, `readLine(in)` (audition_prompt.go), `splitChoice`.
- Produces:
  ```go
  type player interface{ Play(path string) error }   // blocks until the clip ends
  func detectPlayer(env func(string) string, look func(string) (string, error), sleepFallback func(path string) error) player
  type sessionQuestion struct { Key, Text, Axis, HighMeans string; Inverted bool }
  var familyQuestions map[string][]sessionQuestion
  var globalAdjectives = []string{"airy","dull","harsh","thin","full","metallic","wooden","plasticky","buzzy","hollow","nasal","muddy","static","alive","synthetic","real"}
  func runSession(w io.Writer, in io.Reader, p player, families []string, candPath, outPath string, planOnly bool) error
  // prompt helpers (pure, tested with scripted stdin):
  func promptRating(in io.Reader, out io.Writer, id string) (rating int, kind verdictKind)
  func promptVsCurrent(in io.Reader, out io.Writer, id string) (vs string, kind verdictKind)
  func promptScale(in io.Reader, out io.Writer, q sessionQuestion, ids []string) (answers map[string]int, kind verdictKind)  // "A=4 B=2" → centred and un-inverted: {"A":+1,"B":-1}; "-" → empty map
  func promptAdjectives(in io.Reader, out io.Writer, ids []string) (map[string][]string, verdictKind)  // "A=3 7  B=12"
  func promptNote(in io.Reader, out io.Writer) (string, verdictKind)
  ```

- [ ] **Step 1: Failing tests**

```go
// player_test.go
//go:build !js

package main

import (
	"errors"
	"testing"
)

func TestDetectPlayer_PrefersEnvThenPaplay(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "paplay" || name == "ffplay" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	p := detectPlayer(func(string) string { return "" }, look, nil)
	if p.(execPlayer).bin != "/usr/bin/paplay" {
		t.Fatalf("want paplay, got %+v", p)
	}
	p = detectPlayer(func(k string) string { if k == "BEATMO_PLAYER" { return "mpv" }; return "" }, look, nil)
	if p.(execPlayer).bin != "mpv" {
		t.Fatalf("env must win, got %+v", p)
	}
	fallbackHit := false
	p = detectPlayer(func(string) string { return "" }, func(string) (string, error) { return "", errors.New("no") },
		func(string) error { fallbackHit = true; return nil })
	_ = p.Play("x.wav")
	if !fallbackHit {
		t.Fatal("with no binary the sleep fallback must be used")
	}
}
```

```go
// session_questions_test.go
//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

func TestFamilyQuestions_BindToRegisteredDescriptorIDs(t *testing.T) {
	for fam, qs := range familyQuestions {
		if len(qs) < 3 {
			t.Errorf("%s: fewer than three questions", fam)
		}
		for _, q := range qs {
			if _, ok := fingerprint.DefaultRegistry().ByID(q.Axis); !ok {
				t.Errorf("%s/%s binds unknown descriptor id %q", fam, q.Key, q.Axis)
			}
			if q.Text == "" || q.HighMeans == "" {
				t.Errorf("%s/%s needs Text and HighMeans", fam, q.Key)
			}
		}
	}
	for _, fam := range []string{"tubular-bells", "snare", "oboe"} {
		if _, ok := familyQuestions[fam]; !ok {
			t.Errorf("%s must have questions this round", fam)
		}
	}
}
```

```go
// session_test.go
//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptScale_ParsesCentresAndInverts(t *testing.T) {
	q := sessionQuestion{Key: "warmth", Text: "warmth", Axis: "timbre.brightness_ratio", HighMeans: "too bright", Inverted: true}
	ans, kind := promptScale(strings.NewReader("A=1 B=5\n"), &bytes.Buffer{}, q, []string{"A", "B"})
	if kind != verdictRecorded {
		t.Fatalf("kind %v", kind)
	}
	// Inverted: 1 (far too warm) means the synth reads HIGH on brightness? No —
	// "1 = too warm" means brightness is too LOW; inversion flips so +2 always
	// means synth reads HIGH on the axis. A=1 → raw -2 → inverted +2? Careful:
	// spec: 5 means "too bright" for an Inverted question whose scale label
	// says 1 = too warm. HighMeans is what 5 means; Inverted=true only flips
	// the LABEL text order, never the sign. So A=1 → -2, B=5 → +2.
	if ans["A"] != -2 || ans["B"] != 2 {
		t.Fatalf("got %+v", ans)
	}
	ans, kind = promptScale(strings.NewReader("-\n"), &bytes.Buffer{}, q, []string{"A", "B"})
	if kind != verdictRecorded || len(ans) != 0 {
		t.Fatalf("dash must skip: %+v %v", ans, kind)
	}
	_, kind = promptScale(strings.NewReader("q\n"), &bytes.Buffer{}, q, []string{"A"})
	if kind != verdictQuit {
		t.Fatal("q must quit")
	}
}

func TestPromptRatingAndVs(t *testing.T) {
	r, kind := promptRating(strings.NewReader("7\n4\n"), &bytes.Buffer{}, "A")
	if kind != verdictRecorded || r != 4 {
		t.Fatalf("rating: %d %v (7 must be rejected then 4 accepted)", r, kind)
	}
	vs, kind := promptVsCurrent(strings.NewReader("b\n"), &bytes.Buffer{}, "A")
	if kind != verdictRecorded || vs != "better" {
		t.Fatalf("vs: %q %v", vs, kind)
	}
}

func TestPromptAdjectives(t *testing.T) {
	got, kind := promptAdjectives(strings.NewReader("A=3 7  B=-\n"), &bytes.Buffer{}, []string{"A", "B"})
	if kind != verdictRecorded || len(got["A"]) != 2 || got["A"][0] != globalAdjectives[2] || len(got["B"]) != 0 {
		t.Fatalf("got %+v", got)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `-run 'TestDetectPlayer|TestFamilyQuestions|TestPrompt' ./cmd/refs/` FAILS.

- [ ] **Step 3: Implement `player.go`**

```go
//go:build !js

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type player interface{ Play(path string) error }

type execPlayer struct {
	bin  string
	args []string
}

func (p execPlayer) Play(path string) error {
	cmd := exec.Command(p.bin, append(append([]string{}, p.args...), path)...)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run()
}

type funcPlayer func(string) error

func (f funcPlayer) Play(path string) error { return f(path) }

// detectPlayer picks the first blocking player available: $BEATMO_PLAYER,
// then paplay, aplay, ffplay; else the caller's open-and-sleep fallback.
func detectPlayer(env func(string) string, look func(string) (string, error), sleepFallback func(string) error) player {
	if v := env("BEATMO_PLAYER"); v != "" {
		parts := strings.Fields(v)
		return execPlayer{bin: parts[0], args: parts[1:]}
	}
	for _, c := range []execPlayer{
		{bin: "paplay"}, {bin: "aplay", args: []string{"-q"}},
		{bin: "ffplay", args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}},
	} {
		if p, err := look(c.bin); err == nil {
			c.bin = p
			return c
		}
	}
	if sleepFallback == nil {
		return funcPlayer(func(path string) error { return fmt.Errorf("no audio player found; set BEATMO_PLAYER") })
	}
	return funcPlayer(sleepFallback)
}

func defaultPlayer(durSec float64) player {
	return detectPlayer(os.Getenv, exec.LookPath, func(path string) error {
		open := "xdg-open"
		if _, err := exec.LookPath("open"); err == nil {
			open = "open"
		}
		_ = exec.Command(open, path).Start()
		time.Sleep(time.Duration((durSec + 0.6) * float64(time.Second)))
		return nil
	})
}
```
(add `"time"` to the imports.)

- [ ] **Step 4: Implement `session_questions.go`**

```go
//go:build !js

package main

import "github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"

// session_questions.go — the targeted questions per family.
//
// Each question is a 1–5 scale bound to ONE descriptor. HighMeans is what a
// 5 means in the listener's words; the stored answer is centred (-2..+2) and
// +2 ALWAYS means "the synth reads HIGH on Axis". Inverted only changes how
// the scale is labelled on screen (1 = <opposite>, 5 = <HighMeans>) so the
// wording reads naturally; it never flips the sign.
type sessionQuestion struct {
	Key       string
	Text      string
	Axis      string
	HighMeans string
	LowMeans  string
	Inverted  bool
}

var familyQuestions = map[string][]sessionQuestion{
	"tubular-bells": {
		{Key: "ring", Text: "ring length", Axis: fingerprint.IDBandT60, HighMeans: "rings too long", LowMeans: "dies too soon"},
		{Key: "wave", Text: "waviness / beating", Axis: fingerprint.IDModeBeatingDepth, HighMeans: "too wavy", LowMeans: "too steady"},
		{Key: "metal", Text: "metallic complexity", Axis: fingerprint.IDModeCountPerOctave, HighMeans: "too many overtones", LowMeans: "too plain"},
		{Key: "bright", Text: "brightness", Axis: fingerprint.IDBrightnessRatio, HighMeans: "too bright", LowMeans: "too dull"},
		{Key: "strike", Text: "strike noise", Axis: fingerprint.IDOnsetBite, HighMeans: "too much clank", LowMeans: "too soft a strike"},
	},
	"snare": {
		{Key: "wire", Text: "wire buzz vs hiss", Axis: fingerprint.IDNoiseFlatness, HighMeans: "too much broadband wire", LowMeans: "too hissy"},
		{Key: "tail", Text: "tail length", Axis: fingerprint.IDDecayT20, HighMeans: "rings too long", LowMeans: "too short"},
		{Key: "body", Text: "body / low thump", Axis: "snare.low_body_frac", HighMeans: "too much body", LowMeans: "too thin"},
		{Key: "crack", Text: "crack", Axis: fingerprint.IDOnsetBite, HighMeans: "too much crack", LowMeans: "not enough crack"},
		{Key: "tonal", Text: "tonal ring", Axis: fingerprint.IDNoiseFraction, HighMeans: "too noisy", LowMeans: "too tonal / pure"},
	},
	"oboe": {
		{Key: "breath", Text: "breathiness", Axis: fingerprint.IDNoisePersistence, HighMeans: "too breathy", LowMeans: "too dry"},
		{Key: "warmth", Text: "warmth", Axis: fingerprint.IDBrightnessRatio, HighMeans: "too bright", LowMeans: "too warm / dark", Inverted: true},
		{Key: "nasal", Text: "nasal / reedy", Axis: "formant.f1_hz", HighMeans: "too nasal", LowMeans: "not reedy enough"},
		{Key: "steady", Text: "steadiness", Axis: fingerprint.IDModAMFlatness, HighMeans: "wobbles like a machine", LowMeans: "dead flat"},
		{Key: "attack", Text: "attack speed", Axis: fingerprint.IDEnvAttack, HighMeans: "too slow", LowMeans: "too abrupt"},
	},
}

var globalAdjectives = []string{"airy", "dull", "harsh", "thin", "full", "metallic", "wooden", "plasticky", "buzzy", "hollow", "nasal", "muddy", "static", "alive", "synthetic", "real"}
```

Note on the snare "tonal ring" row: the spec's table said "5 = too tonal (inverted)"; binding it to `noise.fraction` with HighMeans "too noisy" keeps the sign rule (+2 = synth reads HIGH on noise fraction) without an exception. The on-screen scale reads `1 too tonal / pure … 5 too noisy`.

Check `"snare.low_body_frac"` and `"formant.f1_hz"` exist as IDs (they were listed by the registry dump); use their exported consts if present.

- [ ] **Step 5: Implement `session.go`**

```go
//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// ---- prompt helpers (pure) --------------------------------------------------

func readChoice(in io.Reader, out io.Writer, prompt string) (string, bool) {
	fmt.Fprint(out, prompt)
	line, ok := readLine(in)
	return strings.TrimSpace(line), ok
}

func controlKind(tok string) (verdictKind, bool) {
	switch tok {
	case "q":
		return verdictQuit, true
	case "s":
		return verdictSkipped, true
	case "r":
		return verdictReplay, true
	}
	return verdictRecorded, false
}

func promptRating(in io.Reader, out io.Writer, id string) (int, verdictKind) {
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("How close is %s to the reference? 1 nothing like it … 5 could be the real thing  (r replay, s skip, q quit)\n> ", id))
		if !ok {
			return 0, verdictQuit
		}
		if k, ctl := controlKind(line); ctl {
			return 0, k
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= 5 {
			return n, verdictRecorded
		}
		fmt.Fprintln(out, "invalid: want 1-5")
	}
}

func promptVsCurrent(in io.Reader, out io.Writer, id string) (string, verdictKind) {
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("%s against the CURRENT version: b better  s same  w worse\n> ", id))
		if !ok {
			return "", verdictQuit
		}
		switch line {
		case "b":
			return "better", verdictRecorded
		case "s":
			return "same", verdictRecorded
		case "w":
			return "worse", verdictRecorded
		case "q":
			return "", verdictQuit
		case "r":
			return "", verdictReplay
		}
		fmt.Fprintln(out, "invalid: want b, s or w")
	}
}

// parsePerID parses "A=4 B=2" / "A=3 7  B=12" into id → tokens.
func parsePerID(line string, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	cur := ""
	for _, tok := range strings.Fields(line) {
		if i := strings.Index(tok, "="); i > 0 {
			cur = strings.ToUpper(tok[:i])
			found := false
			for _, id := range ids {
				if id == cur {
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown clip %q", cur)
			}
			tok = tok[i+1:]
			if tok == "" {
				continue
			}
		}
		if cur == "" {
			return nil, fmt.Errorf("answers must be prefixed with a clip id, e.g. A=4 B=2")
		}
		out[cur] = append(out[cur], tok)
	}
	return out, nil
}

func promptScale(in io.Reader, out io.Writer, q sessionQuestion, ids []string) (map[string]int, verdictKind) {
	lo, hi := q.LowMeans, q.HighMeans
	for {
		line, ok := readChoice(in, out, fmt.Sprintf("%s — 1 %s · 3 about right · 5 %s   (per clip, e.g. %s=4; '-' to skip)\n> ", q.Text, lo, hi, ids[0]))
		if !ok {
			return nil, verdictQuit
		}
		if line == "-" {
			return map[string]int{}, verdictRecorded
		}
		if k, ctl := controlKind(line); ctl {
			return nil, k
		}
		per, err := parsePerID(line, ids)
		if err != nil {
			fmt.Fprintln(out, "invalid:", err)
			continue
		}
		ans := map[string]int{}
		bad := false
		for id, toks := range per {
			if len(toks) != 1 {
				bad = true
				break
			}
			if toks[0] == "-" {
				continue
			}
			n, err := strconv.Atoi(toks[0])
			if err != nil || n < 1 || n > 5 {
				bad = true
				break
			}
			ans[id] = n - 3
		}
		if bad {
			fmt.Fprintln(out, "invalid: each clip needs one value 1-5")
			continue
		}
		return ans, verdictRecorded
	}
}

func promptAdjectives(in io.Reader, out io.Writer, ids []string) (map[string][]string, verdictKind) {
	var menu []string
	for i, a := range globalAdjectives {
		menu = append(menu, fmt.Sprintf("%d %s", i+1, a))
	}
	for {
		line, ok := readChoice(in, out, "Adjectives: "+strings.Join(menu, "  ")+fmt.Sprintf("\n(per clip numbers, e.g. %s=3 7; '-' for none)\n> ", ids[0]))
		if !ok {
			return nil, verdictQuit
		}
		if line == "-" {
			return map[string][]string{}, verdictRecorded
		}
		if k, ctl := controlKind(line); ctl {
			return nil, k
		}
		per, err := parsePerID(line, ids)
		if err != nil {
			fmt.Fprintln(out, "invalid:", err)
			continue
		}
		res := map[string][]string{}
		bad := false
		for id, toks := range per {
			for _, t := range toks {
				if t == "-" {
					continue
				}
				n, err := strconv.Atoi(t)
				if err != nil || n < 1 || n > len(globalAdjectives) {
					bad = true
					break
				}
				res[id] = append(res[id], globalAdjectives[n-1])
			}
		}
		if bad {
			fmt.Fprintln(out, "invalid: numbers from the list only")
			continue
		}
		return res, verdictRecorded
	}
}

func promptNote(in io.Reader, out io.Writer) (string, verdictKind) {
	line, ok := readChoice(in, out, "Anything else? (Enter to skip)\n> ")
	if !ok {
		return "", verdictQuit
	}
	if line == "q" {
		return "", verdictQuit
	}
	return line, verdictRecorded
}

// ---- the session ------------------------------------------------------------

type sessionClips struct {
	refPath, curPath string
	candPaths        map[string]string
	order            []string // "R","C", then candidate ids
}

func playAll(p player, out io.Writer, c sessionClips, which string) {
	seq := c.order
	if which != "all" {
		seq = []string{strings.ToUpper(which)}
	}
	for _, k := range seq {
		path := ""
		switch k {
		case "R":
			path = c.refPath
		case "C":
			path = c.curPath
		default:
			path = c.candPaths[k]
		}
		if path == "" {
			continue
		}
		fmt.Fprintf(out, "▶ %s\n", k)
		_ = p.Play(path)
	}
}

func promptReplayOrRate(in io.Reader, out io.Writer, p player, c sessionClips) verdictKind {
	for {
		line, ok := readChoice(in, out, "replay: r c a b all | Enter to rate | s skip | q quit\n> ")
		if !ok {
			return verdictQuit
		}
		switch strings.ToLower(line) {
		case "":
			return verdictRecorded
		case "q":
			return verdictQuit
		case "s":
			return verdictSkipped
		case "all", "r", "c":
			playAll(p, out, c, line)
		default:
			if _, ok := c.candPaths[strings.ToUpper(line)]; ok {
				playAll(p, out, c, line)
			} else {
				fmt.Fprintln(out, "invalid")
			}
		}
	}
}

func runSessionFamily(w io.Writer, in io.Reader, p player, name string, fc familyCandidates, outPath string, planOnly bool) (verdictKind, error) {
	c, err := loadCatalogue()
	if err != nil {
		return verdictQuit, err
	}
	ref, ok := c.Ref(fc.Reference)
	if !ok {
		return verdictQuit, fmt.Errorf("refs session: %s: unknown reference %q", name, fc.Reference)
	}
	insts := []string{fc.Base}
	overrides := []map[string]float64{nil}
	var ids []string
	for _, cand := range fc.Candidates {
		insts = append(insts, fc.Base)
		overrides = append(overrides, cand.Overrides)
		ids = append(ids, strings.ToUpper(cand.ID))
	}
	results, err := renderFamilyCompareOverrides(os.Stderr, insts, overrides, ref, 0, matchRMS)
	if err != nil {
		return verdictQuit, err
	}
	clips := sessionClips{refPath: results[0].RealPath, curPath: results[0].SynthPath, candPaths: map[string]string{}, order: []string{"R", "C"}}
	for i, id := range ids {
		clips.candPaths[id] = results[i+1].SynthPath
		clips.order = append(clips.order, id)
	}
	fmt.Fprintf(w, "\n=== %s === ref=%s pitch=%+.2f dur=%.2fs\n", name, ref.ID, results[0].Pitch, results[0].Dur)
	fmt.Fprintf(w, "    R reference  C current (%s)", fc.Base)
	for _, cand := range fc.Candidates {
		fmt.Fprintf(w, "  %s %s", strings.ToUpper(cand.ID), cand.Label)
	}
	fmt.Fprintln(w)
	if planOnly {
		fmt.Fprintf(w, "    R=%s\n    C=%s\n", clips.refPath, clips.curPath)
		for _, id := range ids {
			fmt.Fprintf(w, "    %s=%s\n", id, clips.candPaths[id])
		}
		return verdictRecorded, nil
	}

	for {
		playAll(p, w, clips, "all")
		k := promptReplayOrRate(in, w, p, clips)
		if k != verdictRecorded {
			return k, nil
		}
		s := earSession{
			Session: time.Now().UTC().Format(time.RFC3339), Family: name, Reference: ref.ID,
			SemisFromA3: ref.Measured.SemisFromA3, DurSec: results[0].Dur,
			Current: sessionClip{ID: fc.Base, RecipeSHA: audio.InstrumentRecipeHash(fc.Base, nil)},
		}
		replay := false
		for i, cand := range fc.Candidates {
			id := ids[i]
			rating, k := promptRating(in, w, id)
			if k == verdictReplay {
				replay = true
				break
			}
			if k != verdictRecorded {
				return k, nil
			}
			vs, k := promptVsCurrent(in, w, id)
			if k == verdictReplay {
				replay = true
				break
			}
			if k != verdictRecorded {
				return k, nil
			}
			s.Candidates = append(s.Candidates, sessionCandidate{
				sessionClip: sessionClip{ID: id, Base: fc.Base, RecipeSHA: audio.InstrumentRecipeHash(fc.Base, cand.Overrides), Overrides: cand.Overrides},
				Label: cand.Label, Rating: rating, VsCurrent: vs,
			})
		}
		if replay {
			continue
		}
		for _, q := range familyQuestions[name] {
			ans, k := promptScale(in, w, q, ids)
			if k == verdictReplay {
				replay = true
				break
			}
			if k != verdictRecorded {
				return k, nil
			}
			for i := range s.Candidates {
				if v, ok := ans[s.Candidates[i].ID]; ok {
					s.Candidates[i].Scales = append(s.Candidates[i].Scales, sessionScale{Q: q.Key, Axis: q.Axis, Answer: v})
				}
			}
		}
		if replay {
			continue
		}
		adj, k := promptAdjectives(in, w, ids)
		if k == verdictReplay {
			continue
		}
		if k != verdictRecorded {
			return k, nil
		}
		for i := range s.Candidates {
			s.Candidates[i].Adjectives = adj[s.Candidates[i].ID]
		}
		note, k := promptNote(in, w)
		if k != verdictRecorded {
			return k, nil
		}
		s.Note = note
		if err := appendSession(outPath, s); err != nil {
			return verdictQuit, err
		}
		fmt.Fprintf(w, "recorded %s → %s\n", name, outPath)
		return verdictRecorded, nil
	}
}

func runSession(w io.Writer, in io.Reader, p player, families []string, candPath, outPath string, planOnly bool) error {
	all, err := loadCandidates(candPath)
	if err != nil {
		return err
	}
	if len(families) == 0 {
		for k := range all {
			families = append(families, k)
		}
		sort.Strings(families)
	}
	for _, name := range families {
		fc, ok := all[name]
		if !ok {
			return fmt.Errorf("refs session: no candidates for %q in %s", name, candPath)
		}
		k, err := runSessionFamily(w, in, p, name, fc, outPath, planOnly)
		if err != nil {
			return err
		}
		if k == verdictQuit {
			fmt.Fprintln(w, "quitting")
			return nil
		}
	}
	return nil
}

func cmdSession(args []string) error {
	fs := flag.NewFlagSet("session", flag.ExitOnError)
	fams := fs.String("families", "", "comma-separated family names (default: every family in the candidates file)")
	cand := fs.String("candidates", "", "candidates JSON (default: internal/audio/testdata/audition_candidates.json)")
	out := fs.String("out", "", "sessions jsonl (default: internal/audio/testdata/ear_sessions.jsonl)")
	plan := fs.Bool("plan", false, "render and print clip paths without prompting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cp := *cand
	if cp == "" {
		var err error
		if cp, err = defaultCandidatesPath(); err != nil {
			return err
		}
	}
	op := *out
	if op == "" {
		var err error
		if op, err = defaultSessionsPath(); err != nil {
			return err
		}
	}
	var families []string
	if *fams != "" {
		for _, f := range strings.Split(*fams, ",") {
			if f = strings.TrimSpace(f); f != "" {
				families = append(families, f)
			}
		}
	}
	return runSession(os.Stdout, os.Stdin, defaultPlayer(3.0), families, cp, op, *plan)
}
```

`sessionClip` needs the `Base` field from Task 10 Step 3's note. Dispatch `"session": cmdSession` in `main.go` with usage `refs session [-families a,b] [-candidates <json>] [-out <sessions.jsonl>] [-plan]`.

Makefile, after the `ab-notes` target:
```make
ab-session: ## Audition round: reference + current + two candidates per family, rated and qualified with targeted questions (FAMILIES=a,b; answers -> src/go/internal/audio/testdata/ear_sessions.jsonl)
	@cd src/go && ../../.tools/go/bin/go build -o /tmp/beatmo-refs-audition ./cmd/refs
	@cd src/go && /tmp/beatmo-refs-audition session $(if $(FAMILIES),-families $(FAMILIES),)
```
`/tmp/beatmo-refs-audition` is the path the existing loop already builds to; if `/tmp` is noexec on this machine, change BOTH to `$(CURDIR)/build/beatmo-refs-audition`.

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/`
Expected: PASS except `TestShippedCandidatesFile_NoPlaceholders` (until Task 15).

- [ ] **Step 7: Dry run**

Run: rebuild; `BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" session -families oboe -plan`
Expected: four clip paths printed, all existing WAVs, `C` and `A`/`B` in different files. Then `printf 'q\n' | "$SCRATCH/refs" session -families oboe` with `BEATMO_PLAYER="true"` (the `true` binary "plays" instantly) exits cleanly after playing.

---

### Task 15: Author the six candidates, evidence-led

**Files:**
- Modify: `src/go/internal/audio/testdata/audition_candidates.json` (replace the Task 13 skeleton)
- Create: `src/go/cmd/refs/candidate_gate_test.go` (`//go:build !test && !js`)
- Create: `docs/superpowers/plans/2026-09-04-candidate-evidence.md` (the diagnose + ablate excerpts each hypothesis cites)

**Interfaces:**
- Consumes: `refs diagnose -instrument X -ablate` (Task 12), `refs validate` PREF column (Task 10), `familyQuestions` (Task 14), `diagnoseInstrumentOpts` (Task 10), `Descriptor.Demoted` (Task 3).

This task is a measurement loop, not a code task. Each candidate's `hypothesis` must quote the axis it targets and the stage the ablation matrix named. The seeds live in `instrument_seeds_showcase2.go:182` (`tubularBellsSeed`), `instrument_seeds.go:696` (`oboeSeed`), and the snare in `snareVariantSpecs["drum-snare"]` (`snare_modular_push.go:175`), whose legacy knobs (`snare_tone2_freq`, `snare_noise_tune`, `snare_tone_decay`, `snare_noise_decay`, `snare_tail_decay`, `snare_tone_mix`, `snare_noise_mix`, `snare_wire_mix`, `snare_attack`, `snare_wave`) are the override keys.

- [ ] **Step 1: Write the pre-audition gate test first**

```go
// candidate_gate_test.go
//go:build !test && !js

package main

import (
	"io"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// The pre-audition gate (spec §5): a candidate must have FEWER MISMATCH rows
// than the current version among the axes bound by its family's questions,
// and must not turn any of those axes from ok/drift into MISMATCH. It is a
// filter, not the verdict — the listener is the verdict.
func TestShippedCandidates_PassPreAuditionGate(t *testing.T) {
	if os.Getenv(wavref.EnvVar) == "" {
		t.Skipf("set %s to run the candidate gate", wavref.EnvVar)
	}
	p, _ := defaultCandidatesPath()
	fams, err := loadCandidates(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range fams {
		bound := map[string]bool{}
		for _, q := range familyQuestions[name] {
			bound[q.Axis] = true
		}
		cur, err := diagnoseInstrumentOpts(io.Discard, f.Base, f.Reference, 0, nil)
		if err != nil {
			t.Fatalf("%s current: %v", name, err)
		}
		curV := map[string]fingerprint.Verdict{}
		curMis := 0
		for _, a := range cur.Axes {
			if bound[a.ID] {
				curV[a.ID] = a.Verdict
				if a.Verdict == verdictMismatch {
					curMis++
				}
			}
		}
		for _, c := range f.Candidates {
			d, err := diagnoseInstrumentOpts(io.Discard, f.Base, f.Reference, 0, c.Overrides)
			if err != nil {
				t.Fatalf("%s/%s: %v", name, c.ID, err)
			}
			mis := 0
			for _, a := range d.Axes {
				if !bound[a.ID] {
					continue
				}
				if a.Verdict == verdictMismatch {
					mis++
					if curV[a.ID] != verdictMismatch {
						t.Errorf("%s/%s: %s regressed to MISMATCH (was %s)", name, c.ID, a.Name, curV[a.ID])
					}
				}
			}
			if mis >= curMis && curMis > 0 {
				t.Errorf("%s/%s: %d MISMATCH bound axes, current has %d — candidate does not improve the targeted axes", name, c.ID, mis, curMis)
			}
			t.Logf("%s/%s: bound MISMATCH %d → %d", name, c.ID, curMis, mis)
		}
	}
}
```

Run: `cd src/go && BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 ../../.tools/go/bin/go test -run TestShippedCandidates_PassPreAuditionGate ./cmd/refs/`
Expected with the skeleton: FAIL (the ±0.01 overrides do not improve anything).

- [ ] **Step 2: Gather the evidence for tubular-bells**

Run and paste the output into `candidate-evidence.md` under a `## tubular-bells` heading:
```
BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" diagnose -instrument tubular-bells -ablate
```
Read three things: `mode anchor` (is the synth on the reference's grid? the seed places its hum at `gen1_freq 0.62`, so both anchors should match — if the synth anchor reads ~4× the reference, the hum slot is too quiet to be "within 20 dB" and the fix is `gen1_gain` up), `mode ratios` (the seed's ratios are copied from the reference, so the gap should be small; if not, a slot is dead — check the ratchet file), `decay per mode` and `band T60` (the reference's hum outlives everything; the seed's `gen1_env_fast_rate 0.03` vs the measured `DecayDBPerSec` says by how much), `beating depth` and `loudness modulation depth`.

Author:
- **A** — "match per-mode decay and beating": set each `genN_env_fast_rate` from the reference's per-mode `DecayDBPerSec` (rate = |dB/s| / 8.686 for the exponential's e-fold, then check `refs diagnose` reads the same dB/s), move `gen3_freq` so `mode.beating_rate_hz` matches the reference's, drop the `lfo` (ablation will show it is what inflates `loudness modulation depth` to 0.229 vs 0.058).
- **B** — "reference mode set with the hum at its measured level": rebuild the gen table from the reference's `mode ratios` and their `PeakDB` differences (`gen1..gen8_gain` from dB relative to the strongest), no LFO, `amp_decay`/`amp_release` from `band T60` at 1 k.

Each hypothesis string quotes the numbers, e.g. `"decay.per_mode mean|Δ| 22 dB/s and mode.beating_depth 0.02 vs 0.31 were MISMATCH; ablation: lfo 1.9 on loudness modulation depth, gen3 1.4 on beating depth"`.

- [ ] **Step 3: Gather the evidence for snare**

Same command with `-instrument snare`. The known rows: `spectral flatness` 0.599 → 0.204 (synth too coloured), `decay T20` 0.07 → 0.19 s, `percussive body` rows N/A on the synth because it establishes a plateau (`RequiresPercussive` fails) — that plateau is the first thing to remove (`snare_tail_decay` up, `snare_noise_decay` up until `Segments().NoSustainPlateau` is true on the render), then `snare_wire_mix` / `snare_noise_tune` for flatness, `snare_tone_mix` down for `noise fraction`.

Author:
- **A** — "broadband wire, no plateau": raise `snare_noise_decay`/`snare_tail_decay` to hit `decay T20` ≈ 0.07 s, raise `snare_noise_tune` toward 1.0 and `snare_wire_mix` up so `spectral flatness` climbs toward 0.6.
- **B** — "body vs wire rebalance from the snare.* rows": leave decay as A, set `snare_tone_mix`/`snare_tone2_freq` from `body fundamental` (194 Hz) and `low body fraction` (0.446) / `high buzz fraction` (0.344) on the reference.

- [ ] **Step 4: Gather the evidence for oboe**

`-instrument oboe -ablate`. Known: `noise persistence` high (the breath bed gen is constant — the seed's reed-buzz slot), `brightness ratio` 14.4 vs 6.9. The ablation matrix names the breath slot for persistence and `filter` for brightness ratio.

Author:
- **A** — "cut the persistent bed, darken": the breath slot's `genN_env_tail_mix` down to ~0.2 with a `genN_env_fast_mix` attack chiff, `filter_cutoff` down until `brightness ratio` is within 1.4× of 6.9.
- **B** — "formant stage for the nasal peak": A's noise fix plus `formant_enabled 1`, `formant_vowel`/`formant_shift` chosen so `first formant` moves toward the reference's, `formant_mix` moderate (≤ 0.5 — the guide warns a fixed formant bank vocodes the source when mixed high).

- [ ] **Step 5: Iterate each candidate against the gate**

For each family: edit `audition_candidates.json`, then
```
cd src/go && BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 ../../.tools/go/bin/go test -run 'TestShippedCandidates' -v ./cmd/refs/
```
until both `TestShippedCandidates_AreLive` and `TestShippedCandidates_PassPreAuditionGate` PASS and the `-v` log shows the bound-MISMATCH count dropping for every candidate. Then run the fast path so `TestShippedCandidatesFile_NoPlaceholders` passes.

- [ ] **Step 6: Record the evidence file**

`candidate-evidence.md` holds, per family: the `diagnose -ablate` excerpt for the current version, the final overrides, and the `diagnose` excerpt for each candidate (bound axes only). This is what the listener reads next to the session results.

---

### Task 16: The listening session and promotion

**Files:**
- Read: `src/go/internal/audio/testdata/ear_sessions.jsonl` (written by the user's session)
- Modify (after the verdict): `src/go/internal/audio/instrument_seeds*.go` / `snare_modular_push.go` (promote the winner), `audition_candidates.json` (clear the promoted family or leave for a second round)

- [ ] **Step 1: Hand over to the user**

Tell the user to run, from the repo root:
```
BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 make ab-session FAMILIES=tubular-bells,snare,oboe
```
and explain the keys once: `r c a b all` replay, Enter to rate, 1–5 per candidate, `b/s/w` against current, one line per question like `A=4 B=2`, adjectives by number, `-` to skip any question, `q` to quit.

- [ ] **Step 2: Read the results back**

Run: `cd src/go && BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 "$SCRATCH/refs" validate` and `"$SCRATCH/refs" validate -sessions-table`.
Report: per family, which candidate won and by how much; per axis, whether the listener's sign agreed with the measured direction; any candidate the gate passed but the ear rejected (that is a lying axis — record it in `AGENTS.synth.instruments.md` § "Tuning by measurement" with the numbers).

- [ ] **Step 3: Promote**

For a candidate rated `better` with rating ≥ 4: copy its overrides into the base seed (or, for the snare, into `snareVariantSpecs["drum-snare"].lit` — but note that table's byte-parity contract comment; a changed literal changes the golden, so prefer adding a `drum-snare-v2` recipe if the goldens must stay) and run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ ./cmd/refs/
make build/libdrums.a && cd src/go && ../../.tools/go/bin/go test -run 'TestSeedParamsAreLive|Golden' ./internal/audio/
```
The seed-liveness ratchet must not grow; any audio golden that changes is expected for the promoted instrument only and must be regenerated per that test's own instructions.

- [ ] **Step 4: Update the doc that drifted**

Replace the trust table in `AGENTS.synth.instruments.md` § "Only three axes have earned trust" with the live `validate` output (AUC, PREF, CAL columns) and date it; add `band.t60`, `mode.harmonicity`, `partial.independence`, `onset.bite_ratio`, `mod.am.*` to the "What the listener's words mean" table with the session evidence.

---

## Self-review

**Spec coverage.** §1.1 mode tracker → Task 1. §1.2 → Task 2. §1.3 + `ring T60` demotion → Task 3. §1.4 → Task 4. §1.5 → Task 5. §1.6 → Task 6. §1.7 organic range + `refs organic` → Task 7. §2.1 preference test → Task 10. §2.2 thresholds + DEMOTED → Task 8. §2.3 provenance → Task 9. §2.4 word rules → Task 11. §3 ablation + `-ablate` → Task 12. §4.1 candidates file → Task 13. §4.2 player, §4.3 prompt flow, §4.4 record, §4.5 questions, §4.6 Make target → Task 14 (record type in Task 10). §5 retunes + gate → Task 15. §6 testing → per task. §7 order → task order. Success criterion 1 is checked in Task 2 Step 5 and Task 3 Step 7; criterion 2 in Task 8 Step 5, Task 10 Step 7, Task 11 Step 4; criterion 3 in Task 12 Steps 5–7; criterion 4 in Task 7 Step 7; criterion 5 in Tasks 15–16.

**Known deviations from the spec, deliberate:** the snare "tonal ring" question binds to `noise.fraction` with "5 = too noisy" instead of an inverted "5 = too tonal", so the +2 = high rule has no exception (Task 14 Step 4 note). `Inverted` on a question only affects the on-screen label order.

**Placeholder scan.** The only intentional placeholder is the string `TBD-by-task-15` in Task 13's skeleton JSON; `TestShippedCandidatesFile_NoPlaceholders` fails until Task 15 replaces it, so it cannot ship.

**Type consistency.** `ModeSet.Strong(20)` is used with the literal 20 in tests and `modeStrongWithin` in code — same value. `sessionClip.Base` is introduced in Task 10 Step 3's note and used in Tasks 10 Step 5 and 14 Step 5. `axisResult.ID`/`Signed` are added in Task 9 and read in Tasks 8, 10, 12, 15 — Task 8 says to add them early if executed first. `compareReduced(mag, label, drift, mismatch)` matches compare.go:164. `verdictMismatch` is the `cmd/refs` alias of `fingerprint.VerdictMismatch` (diagnose.go:49). `diagnoseInstrumentOpts` replaces `diagnoseInstrumentDur`'s body in Task 10 and is what Tasks 8, 12, 15 call.
