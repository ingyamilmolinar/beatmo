# Parity Timing-Sync Enforcement + Logical-Checker Hardening — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce that the whole audio chain — real clock → sequencer scheduling → trigger → dispatch → actual sample emission — stays in sync within strict, statistically-gated tolerances and panics with a detailed dump on drift, while never false-triggering during live circuit/rule edits; and close the highest-value coverage gaps in the existing logical parity checker.

**Architecture:** A 4th parity surface (`parityTiming`) reuses the existing `parityGen`/`parityGraceUntilNS` grace spine and the `emitParityFatal` dump path. Stages A/B/C reuse the existing `schedMetrics` reservoirs; a new Stage-D emission-drift reservoir plus a bounded `timingTrace` ring add the final "actually producing audio" hop (desktop mixer device-clock anchor + lock-free emission report; browser WebAudio `getTimingTraces`). A multi-statistic gate (mean, P99 tail, stddev, absolute-max) fires per stage.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten stub test harness (`-tags test -modfile=go.test.mod`), miniaudio/Oto desktop audio, WASM + WebAudio (`src/js/audio.js`), Playwright browser tests.

## Global Constraints

- Always use the bundled Go: `.tools/go/bin/go` (from `/home/ymolinar/Repos/beatmo`). Fast test invocation: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...`.
- **Never `git add`/`commit`/`push`** in this repo (user convention). The "Commit" steps below are written as the plan's logical checkpoints; in this repo, **do not run them** — instead run the test command and leave changes uncommitted. (Kept in the plan only as review boundaries.)
- **Do not alter audio DSP output.** Stage-D mixer instrumentation is additive and must not change any sample. A discipline test asserts no new per-block allocation in the hot path.
- **No lock/alloc inside `mixer.processBlock` / `mixer.Read`** hot path (see `recording_pipeline.go:203`). Stage-D emission reporting uses a lock-free SPSC ring + atomics only.
- Activation policy: timing panic **default ON under `go test` + desktop dev**, **OFF on WASM/production** (log-only), armable via `PARITY_TIMING_FATAL`. Mirrors the existing `PARITY_FATAL`/`PARITY_WASM_FATAL` model.
- A "timing mismatch" is **statistical** (window of samples), never a single sample — except a dropout-class absolute-max reading, which fires alone. Warmup (0.5 s) and the parity grace window fully suppress.
- Timing tolerances live on `RuntimeProfile` (desktop tight, browser looser) with env overrides; each divergent value carries a one-line rationale comment.
- Follow existing patterns: parity state on the embedded `parityTracker`; build-tag splits (`!test && !js` desktop, `test` stub, `js` WASM) for any `internal/audio` change.
- Race safety: any new cross-thread state is covered by `make test-race`.

---

## File Structure

**New files (`src/go/internal/ui/`)**
- `game_parity_timing_types.go` — `timingTrace`, the bounded `timingRing`, timing-gate config accessors, `timingBreach` result type.
- `game_parity_timing.go` — `parityTimingCheck()` (the gate) + `emitTimingFatal()` + trace open/close helpers + env parsing/activation.
- Tests: `parity_timing_gate_test.go`, `parity_timing_grace_test.go`, `parity_timing_stress_test.go`, `parity_timing_stage_d_desktop_test.go`, and additions to existing logical tests (Phase 0).

**New file (`src/go/internal/audio/`)**
- `engine_clock_probe.go` (`//go:build !test && !js`) — the mixer device-clock anchor (seqlock pair) + the lock-free emission-report SPSC ring + public accessors `DeviceClockSnapshot()` / `DrainEmitReports()`.

**Modified files**
- `src/go/internal/ui/schedule_metrics.go` — Stage-D emission-drift reservoir + `ObserveEmitDrift` + snapshot fields.
- `src/go/internal/ui/game_ui_types.go` — add `emittedDevPos`/emission plumbing fields only if needed (trace lives in the ring, not `soundReq`; `soundReq` unchanged unless noted).
- `src/go/internal/ui/game_parity_tracker.go` — add `timingRing`, timing watch mode field.
- `src/go/internal/ui/game_audio_schedule.go` — open a timing trace when a sequencer sound is scheduled.
- `src/go/internal/ui/game_audio_loop.go` — stamp dispatch hop; drain Stage-D emission reports (desktop) and close traces.
- `src/go/internal/ui/game_refresh_drum_row.go` — call `parityTimingCheck()` next to `parityScan`.
- `src/go/internal/ui/game_parity_state.go` — env parsing + activation defaults + `emitParityFatal` timing section reuse.
- `src/go/internal/ui/runtime_profile.go` — `Timing timingBounds` field on `RuntimeProfile` + values in both builders.
- `src/go/internal/audio/engine_mixer.go` — additive `row,abs` fields on `voiceState`; thread through `Schedule`.
- `src/go/internal/audio/engine_play.go` — thread `row,abs` into `Schedule`; `PlayParamsAt`/`PlayBatch` carry identity.
- `src/go/internal/audio/types.go` — add `Row,Abs int` to `BatchParam`.
- `src/go/internal/audio/engine_stop.go` — publish an emission report the first block a voice renders.
- `src/js/audio.js` — timing ring + `getTimingTraces()`/`resetTimingTraces()`.
- `src/go/internal/ui/parity_runtime_mutation_matrix_test.go` — add `SetLength`/subdiv mid-play case (Phase 0).

---

# PHASE 0 — Logical-checker hardening (Track 1)

Independent of the timing work; do first to make the shared grace machinery trustworthy before Track 2 builds on it.

### Task 0.1: Enable the real-time grace path under test

Today `parityInGrace()` returns `false` under `go test` (`game_parity_state.go:88`), so the grace-demotion branch in `parityReport`/`emitParityFatal` has **zero** coverage. Add a test-only override so tests can exercise real grace.

**Files:**
- Modify: `src/go/internal/ui/game_parity_state.go` (the `parityInGrace` body near line 84-96)
- Test: `src/go/internal/ui/parity_grace_realtime_test.go` (new)

**Interfaces:**
- Produces: `func SetParityGraceForceForTest(on bool) (restore func())` — when `on`, `parityInGrace()` honors the real `parityGraceUntilNS` deadline even under `go test`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/parity_grace_realtime_test.go
package ui

import (
	"testing"
	"time"
)

func TestParityGraceDemotesMismatchThenRefiresAfterExpiry(t *testing.T) {
	restore := SetParityGraceForceForTest(true)
	defer restore()

	g := newTestGameForParity(t) // existing helper used by parity tests
	g.parityWatch = parityWatchPanic
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(true) })

	// Arm a short grace window (structural mutation just happened).
	g.parityGraceUntilNS.Store(time.Now().Add(40 * time.Millisecond).UnixNano())

	// Inject a genuine on-screen mismatch. Within grace it must NOT panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("mismatch panicked inside grace window: %v", r)
			}
		}()
		injectParityMismatchAndScan(t, g) // helper below drives a forced mismatch
	}()

	// After grace expires, the same mismatch must panic.
	time.Sleep(60 * time.Millisecond)
	paniced := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				paniced = true
			}
		}()
		injectParityMismatchAndScan(t, g)
	}()
	if !paniced {
		t.Fatal("expected panic after grace expiry, got none")
	}
}
```

> Note for implementer: `newTestGameForParity` and a small `injectParityMismatchAndScan` helper follow the existing pattern in `parity_scan_throttle_test.go` (`buildParityMismatchGame`) and `parity_fatal_without_watch_test.go`. If a shared helper does not already exist, add `injectParityMismatchAndScan` to a `parity_test_helpers_test.go` that sets a stale slate at the current beat and calls `g.parityScan("test")`. Reuse `buildParityMismatchGame` if it already produces a forced mismatch.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityGraceDemotesMismatchThenRefiresAfterExpiry -v`
Expected: FAIL — `SetParityGraceForceForTest` undefined.

- [ ] **Step 3: Implement the override**

In `game_parity_state.go`, replace the `go test` bypass with a gated one:

```go
// parityGraceForceForTest, when true, makes parityInGrace honor the real
// parityGraceUntilNS deadline even under `go test`. Default false preserves
// the historical test bypass (most tests want grace OFF for determinism).
var parityGraceForceForTest atomic.Bool

// SetParityGraceForceForTest opts a test into real grace-window behavior.
func SetParityGraceForceForTest(on bool) (restore func()) {
	prev := parityGraceForceForTest.Load()
	parityGraceForceForTest.Store(on)
	return func() { parityGraceForceForTest.Store(prev) }
}

func (g *Game) parityInGrace() bool {
	if runningUnderGoTest() && !parityGraceForceForTest.Load() {
		return false
	}
	now := time.Now().UnixNano()
	return now < g.parityGraceUntilNS.Load()
}
```

(Import `sync/atomic` if not already imported in this file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityGraceDemotesMismatchThenRefiresAfterExpiry -v`
Expected: PASS.

- [ ] **Step 5: Regression — run the whole parity suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Parity' -count=1`
Expected: PASS (the default `false` keeps every existing test's grace-off assumption intact).

- [ ] **Step 6: Commit** (checkpoint only — do not run git in this repo)

```bash
git add src/go/internal/ui/game_parity_state.go src/go/internal/ui/parity_grace_realtime_test.go
git commit -m "test(parity): exercise real-time grace demotion path"
```

---

### Task 0.2: True-positive test for `audio_vs_seq`

`audio_vs_seq` is only tested negatively today. Force a genuine audio-event-without-audible-decision and assert it fires.

**Files:**
- Test: `src/go/internal/ui/parity_audio_vs_seq_positive_test.go` (new)

**Interfaces:**
- Consumes: `recordParityAudio`, `parityScan`, `paritySeqDecisions` (existing).

- [ ] **Step 1: Write the failing/failing-then-passing test**

```go
// src/go/internal/ui/parity_audio_vs_seq_positive_test.go
package ui

import "testing"

func TestParityDetectsAudioWithoutSeqDecision(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityWatch = parityWatchPanic
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(true) })
	g.SetPlaying(true)

	// Advance the playhead so abs=3 is the current beat, but record NO seq
	// decision for it — then record an audio event there. That is an audio
	// event with no audible decision → audio_vs_seq.
	setPlayheadForTest(g, 0, 4) // helper: nextBeatIdxs[row]=4 → current beat abs=3
	g.parityGen.Store(1)
	curGen := g.audioGen.Load()
	g.recordParityAudio(0, 3, g.nowAudioForTest(), "kick", 1, 0, 1, curGen, g.parityGen.Load())

	assertPanicKind(t, func() { g.parityScan("test") }, "audio_vs_seq")
}
```

> Implementer: `setPlayheadForTest`, `assertPanicKind`, and `nowAudioForTest` mirror helpers already used in `parity_audio_current_beat_test.go` / `parity_audio_scan_playhead_test.go`. If `assertPanicKind` does not exist, add it to `parity_test_helpers_test.go`: recover a panic and assert `strings.Contains(recoveredMsg, kind)` — but since `emitParityFatal` panics with a fixed string, instead assert by capturing the logged `mismatchEntry.Kind` via a test hook. Prefer the existing pattern: these tests recover the panic and additionally assert on `g.parityRing.snapshot()` containing an entry whose `Kind == "audio_vs_seq"`. Follow whichever assertion style the current `parity_audio_current_beat_test.go` uses.

- [ ] **Step 2: Run it**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityDetectsAudioWithoutSeqDecision -v`
Expected: PASS (proves detection). If it does NOT fire, that is a real checker gap — capture the actual behavior and stop for review before proceeding.

- [ ] **Step 3: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/parity_audio_vs_seq_positive_test.go
git commit -m "test(parity): positive audio_vs_seq detection"
```

---

### Task 0.3: True-positive test for `view_vs_truth` on a mute node

Detection tests use regular nodes only. Add a mute-node case (predictor truth via `TriggeredAt`).

**Files:**
- Test: `src/go/internal/ui/parity_view_vs_truth_mute_test.go` (new)

- [ ] **Step 1: Write the test**

```go
// src/go/internal/ui/parity_view_vs_truth_mute_test.go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/core/model"
)

func TestParityDetectsViewVsTruthForMuteNode(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityWatch = parityWatchPanic
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(true) })
	g.SetPlaying(true)

	// Build a row whose predictor TriggeredAt(row, abs) is true for a mute
	// node at abs in-window, but corrupt the slate so slate != viewExpected.
	row, abs := 0, 2
	setMuteNodeTruthForTest(g, row, abs, true) // predictor.TriggeredAt → true
	corruptSlateForTest(g, row, abs, false)    // rendered slate says off

	assertParityKindFires(t, g, "view_vs_truth")
}
```

> Implementer: `setMuteNodeTruthForTest`/`corruptSlateForTest`/`assertParityKindFires` follow `parity_cached_view_test.go` (which already corrupts slate to force `view_vs_truth`). Extend that file's helpers to accept a mute node type rather than duplicating. The predictor-truth branch under test is `parityScan` line ~381 (`TriggeredAt` for `NodeTypeMute`).

- [ ] **Step 2: Run it**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityDetectsViewVsTruthForMuteNode -v`
Expected: PASS.

- [ ] **Step 3: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/parity_view_vs_truth_mute_test.go
git commit -m "test(parity): positive view_vs_truth for mute node"
```

---

### Task 0.4: Assert the `emitParityFatal` payload contents

Regression-guard the crash dump: today only "a panic occurred" is asserted, so payload regressions are invisible.

**Files:**
- Modify: `src/go/internal/ui/game_parity_state.go` — extract the dump string builder into a testable helper.
- Test: `src/go/internal/ui/parity_fatal_payload_test.go` (new)

**Interfaces:**
- Produces: `func (g *Game) buildParityFatalDump(e mismatchEntry) string` — the exact string `emitParityFatal` logs and (for fatal) panics after.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/parity_fatal_payload_test.go
package ui

import (
	"strings"
	"testing"
)

func TestParityFatalDumpContainsDiagnostics(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlaying(true)
	e := mismatchEntry{Row: 0, Abs: 5, Source: "test", Kind: "view_vs_truth", Scheduled: true, Slate: false, InWindow: true}
	dump := g.buildParityFatalDump(e)
	for _, want := range []string{
		"[PARITY]", "row=0 abs=5", "kind=view_vs_truth",
		"transport:", "seqNextIdxs=", "nextBeatIdxs=",
		"predictor visible=", "timeline offset=", "graph start=",
		"recent mismatches:",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump missing %q\n---\n%s", want, dump)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityFatalDumpContainsDiagnostics -v`
Expected: FAIL — `buildParityFatalDump` undefined.

- [ ] **Step 3: Extract the builder**

In `game_parity_state.go`, refactor `emitParityFatal` so the string construction (currently lines ~345-390) lives in a new method, and `emitParityFatal` calls it:

```go
func (g *Game) buildParityFatalDump(e mismatchEntry) string {
	builder := &strings.Builder{}
	builder.WriteString("[PARITY] scheduler vs DrumView mismatch\n")
	// ... move the exact existing Fprintf/WriteString sequence here verbatim ...
	// (transport snapshot, scheduler indices, row window, predictor values,
	//  timeline snapshot, graph summary, recent-mismatch ring, highlight state)
	return builder.String()
}

func (g *Game) emitParityFatal(e mismatchEntry) {
	if !parityFatalEnabled.Load() && !e.Force {
		if g.logger != nil {
			g.logger.Warnf("[parity][nonfatal] row=%d abs=%d src=%s sched=%v slate=%v inWindow=%v", e.Row, e.Abs, e.Source, e.Scheduled, e.Slate, e.InWindow)
		}
		return
	}
	msg := g.buildParityFatalDump(e)
	if g.logger != nil {
		g.logger.Errorf(msg)
	}
	if os.Getenv("PARITY_DUMP_STDERR") == "1" {
		fmt.Fprint(os.Stderr, msg)
	}
	panic("scheduler/drumview parity mismatch")
}
```

- [ ] **Step 4: Run to verify it passes + full parity suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParityFatalDumpContainsDiagnostics|Parity' -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/game_parity_state.go src/go/internal/ui/parity_fatal_payload_test.go
git commit -m "refactor(parity): extract testable fatal-dump builder + assert contents"
```

---

### Task 0.5: `SetLength`/subdivision mid-play in the runtime-mutation matrix + stride phase-rotation positive

**Files:**
- Modify: `src/go/internal/ui/parity_runtime_mutation_matrix_test.go` — add a `length-change` case.
- Test: `src/go/internal/ui/parity_stride_positive_test.go` (new) — a mismatch at a strided abs still fires within a few scans.

- [ ] **Step 1: Add the length-change matrix case**

Locate the table in `parity_runtime_mutation_matrix_test.go` (`TestParityRuntimeMutationMatrix`, runner `runRuntimeMutationCase`) and add:

```go
{
	name: "length-change-midplay",
	mutate: func(t *testing.T, g *Game) {
		// Change the sequence length / subdivision mid-play; must not trip parity.
		g.drum.SetLength(g.drum.Length + 4)
		g.updateBeatInfos() // recompute paths (bumps parityGen internally)
	},
	// expect zero mismatches (panic mode) — same as the other matrix cases.
},
```

> Implementer: match the exact field names of the existing table struct (`name`, and whatever the mutate/assert closures are called in the current file). If `SetLength` is not the correct method, use the same call the transport length control uses (grep `SetLength`/`setLength` in `internal/ui` and `internal/drum`).

- [ ] **Step 2: Write the stride positive test**

```go
// src/go/internal/ui/parity_stride_positive_test.go
package ui

import "testing"

func TestParityStrideEventuallyFiresMismatch(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityWatch = parityWatchLog // log mode: collect into ring, don't panic
	g.parityScanStride = 3         // force phase rotation
	g.SetPlaying(true)

	// Plant a persistent mismatch at a single abs that stride may skip on
	// some scans; assert it appears in the ring within stride*2 scans.
	planted := planPersistentMismatchForTest(t, g, 0, 7)
	found := false
	for i := 0; i < g.parityScanStride*2+2; i++ {
		g.parityScanPhase = i % g.parityScanStride
		g.parityScan("test")
		if ringContainsAbs(g, planted.row, planted.abs) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stride %d never scanned the mismatching abs", g.parityScanStride)
	}
}
```

> Implementer: `planPersistentMismatchForTest`/`ringContainsAbs` follow the slate-corruption helpers from `parity_cached_view_test.go` + `parityRing.snapshot()`. Keep the mismatch persistent (re-corrupt each loop if the scan clears it).

- [ ] **Step 3: Run both**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParityRuntimeMutationMatrix|TestParityStrideEventuallyFiresMismatch' -v -count=1`
Expected: PASS.

- [ ] **Step 4: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/parity_runtime_mutation_matrix_test.go src/go/internal/ui/parity_stride_positive_test.go
git commit -m "test(parity): length-change matrix case + stride positive"
```

---

# PHASE 1 — Timing foundation (config, gate types, Stage-D reservoir)

### Task 1.1: Stage-D emission-drift reservoir on `schedMetrics`

Add a 4th stage to the existing metrics: emission drift (`emittedAt − idealWhen`), signed sum for systematic-bias mean, absolute samples for tail/max/stddev.

**Files:**
- Modify: `src/go/internal/ui/schedule_metrics.go`
- Test: `src/go/internal/ui/schedule_metrics_emit_test.go` (new)

**Interfaces:**
- Produces: `func (m *scheduleMetrics) ObserveEmitDrift(driftSec float64)`; snapshot fields `EmitCount int64`, `EmitAvg float64` (signed), `EmitAbsMax float64`, `EmitP99 float64` (abs), `EmitStdDev float64` (abs). `Reset()` clears them.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/schedule_metrics_emit_test.go
package ui

import (
	"math"
	"testing"
)

func TestScheduleMetricsEmitDrift(t *testing.T) {
	m := &scheduleMetrics{}
	m.startAt = -1 // disable warmup gating for this unit test if startAt is used; see note
	for _, d := range []float64{0.001, -0.002, 0.003, -0.001} {
		m.ObserveEmitDrift(d)
	}
	s := m.Snapshot()
	if s.EmitCount != 4 {
		t.Fatalf("EmitCount=%d want 4", s.EmitCount)
	}
	if math.Abs(s.EmitAbsMax-0.003) > 1e-9 {
		t.Errorf("EmitAbsMax=%.6f want 0.003", s.EmitAbsMax)
	}
	// signed mean = (0.001-0.002+0.003-0.001)/4 = 0.00025
	if math.Abs(s.EmitAvg-0.00025) > 1e-9 {
		t.Errorf("EmitAvg=%.6f want 0.00025", s.EmitAvg)
	}
	m.Reset()
	if m.Snapshot().EmitCount != 0 {
		t.Error("Reset did not clear emit stage")
	}
}
```

> Note: if `ObserveEmitDrift` honors `schedWarmupSec` like the other Observe* methods (comparing against `m.startAt`), the test must advance past warmup. Mirror how `schedule_metrics_test.go` already handles warmup for `ObserveBridge`/`ObserveSeqFireLate`; copy that setup rather than the `startAt = -1` shortcut if those methods gate on warmup.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestScheduleMetricsEmitDrift -v`
Expected: FAIL — `ObserveEmitDrift`/`EmitCount` undefined.

- [ ] **Step 3: Implement**

Add to the `scheduleMetrics` struct (after the Stage-A block):

```go
	// Stage D — emission drift (idealWhen → actual sample emission).
	emitCount    int64
	emitSumSig   float64 // signed sum, for systematic-bias mean
	emitSumSqAbs float64 // sum of squares of |drift|, for stddev
	emitAbsMax   float64
	emitSamples  []float64 // |drift| reservoir for percentile
```

Add the observer (mirror `ObserveBridge`, honoring warmup the same way it does):

```go
// ObserveEmitDrift records a Stage-D sample: drift = emittedAt - idealWhen
// (seconds). Signed drift feeds the systematic-bias mean; |drift| feeds the
// tail/max/stddev so a symmetric jitter cannot cancel out.
func (m *scheduleMetrics) ObserveEmitDrift(driftSec float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Warmup gate: match ObserveBridge/ObserveSeqFireLate behavior in this file.
	a := math.Abs(driftSec)
	m.emitCount++
	m.emitSumSig += driftSec
	m.emitSumSqAbs += a * a
	if a > m.emitAbsMax {
		m.emitAbsMax = a
	}
	pushSample(&m.emitSamples, a)
}
```

Extend `ScheduleMetricsSnapshot`:

```go
	// Stage D — emission drift (idealWhen → sample emission), seconds.
	EmitCount  int64   `json:"emitCount"`
	EmitAvg    float64 `json:"emitAvg"`    // signed mean (systematic bias)
	EmitAbsMax float64 `json:"emitAbsMax"` // max |drift|
	EmitP99    float64 `json:"emitP99"`    // P99 of |drift|
	EmitStdDev float64 `json:"emitStdDev"` // stddev of |drift|
```

In `Snapshot()` populate them:

```go
	if m.emitCount > 0 {
		s.EmitCount = m.emitCount
		s.EmitAvg = m.emitSumSig / float64(m.emitCount)
		s.EmitAbsMax = m.emitAbsMax
		s.EmitP99 = percentileSample(m.emitSamples, 0.99)
		s.EmitStdDev = stddev(sumAbs(m.emitSamples), m.emitSumSqAbs, m.emitCount)
	}
```

> `stddev` takes (sum, sumSq, n). For |drift| we need the plain sum of |drift|; add a tiny local helper `sumAbs(xs []float64) float64` (sum of the reservoir) OR track a running `emitSumAbs` field alongside the others and pass it directly (preferred — avoids re-summing the reservoir every Snapshot). Implement the running-field version: add `emitSumAbs float64`, accumulate `m.emitSumAbs += a` in the observer, and pass `m.emitSumAbs` to `stddev`.

In `Reset()` zero all five `emit*` fields and `m.emitSamples = m.emitSamples[:0]`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestScheduleMetricsEmitDrift|TestScheduleMetrics' -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/schedule_metrics.go src/go/internal/ui/schedule_metrics_emit_test.go
git commit -m "feat(metrics): Stage-D emission-drift reservoir"
```

---

### Task 1.2: Timing tolerance config on `RuntimeProfile`

**Files:**
- Modify: `src/go/internal/ui/runtime_profile.go`
- Test: `src/go/internal/ui/runtime_profile_timing_test.go` (new)

**Interfaces:**
- Produces:
```go
type timingStageBounds struct{ MeanMax, P99Max, StdDevMax, AbsMax float64 } // seconds; 0 disables that statistic
type timingBounds struct {
	Enabled      bool
	FatalDefault bool
	WatchDefault parityWatchMode
	MinSamples   int // don't gate a stage until its window has this many samples
	SeqFire      timingStageBounds
	Bridge       timingStageBounds
	Overdue      timingStageBounds // Stage C; MeanMax=AvgLag, P99Max=LagP99, AbsMax=MaxLag; StdDevMax unused (0)
	EmitDrift    timingStageBounds
}
```
Field `Timing timingBounds` on `RuntimeProfile`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/runtime_profile_timing_test.go
package ui

import "testing"

func TestRuntimeProfileTimingBounds(t *testing.T) {
	d := desktopRuntimeProfile()
	if !d.Timing.Enabled {
		t.Error("desktop timing gate should be enabled")
	}
	if !d.Timing.FatalDefault {
		t.Error("desktop timing fatal default should be true")
	}
	if d.Timing.EmitDrift.AbsMax <= 0 {
		t.Error("desktop emit-drift AbsMax must be set")
	}
	b := browserRuntimeProfile()
	if b.Timing.FatalDefault {
		t.Error("browser timing fatal default must be false (log-only)")
	}
	// Browser bounds must be >= desktop (looser).
	if b.Timing.EmitDrift.AbsMax < d.Timing.EmitDrift.AbsMax {
		t.Error("browser emit-drift AbsMax should be looser than desktop")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestRuntimeProfileTimingBounds -v`
Expected: FAIL — `Timing` field undefined.

- [ ] **Step 3: Implement**

Add the two types (near the top-level type declarations in `runtime_profile.go`) and the field on `RuntimeProfile` (after the `ParityWatchDefault` field):

```go
	// Timing-parity gate tolerances. See parityTimingCheck. All values in
	// seconds; a bound of 0 disables that statistic. Desktop values are tight
	// (dev/test enforcement); browser values looser + log-only by default.
	Timing timingBounds
```

Add to `desktopRuntimeProfile()`'s literal (starting values — tune against `bench-results/`):

```go
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: true,
			WatchDefault: parityWatchPanic,
			MinSamples:   64,
			// Stage A: scheduler tick is 1ms desktop; fire-late should be tiny.
			SeqFire:   timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.030},
			// Stage B: enqAt→dispatch goroutine handoff, sub-ms typical.
			Bridge:    timingStageBounds{MeanMax: 0.002, P99Max: 0.008, StdDevMax: 0.003, AbsMax: 0.030},
			// Stage C: overdue (lag). Any sustained overdue is bad; dropout at AbsMax.
			Overdue:   timingStageBounds{MeanMax: 0.0005, P99Max: 0.006, StdDevMax: 0, AbsMax: 0.020},
			// Stage D: realized emission vs ideal. Desktop wall-vs-device skew.
			EmitDrift: timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.040},
		},
```

Add to `browserRuntimeProfile()`'s literal (looser, log-only):

```go
		Timing: timingBounds{
			Enabled:      true,
			FatalDefault: false, // log-only on browser unless PARITY_TIMING_FATAL arms it
			WatchDefault: parityWatchLog,
			MinSamples:   128,
			SeqFire:   timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060},
			Bridge:    timingStageBounds{MeanMax: 0.006, P99Max: 0.020, StdDevMax: 0.008, AbsMax: 0.060},
			Overdue:   timingStageBounds{MeanMax: 0.002, P99Max: 0.012, StdDevMax: 0, AbsMax: 0.040},
			EmitDrift: timingStageBounds{MeanMax: 0.008, P99Max: 0.025, StdDevMax: 0.010, AbsMax: 0.080},
		},
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestRuntimeProfileTimingBounds -v`
Expected: PASS.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/runtime_profile.go src/go/internal/ui/runtime_profile_timing_test.go
git commit -m "feat(timing): runtime-profile tolerance bounds"
```

---

### Task 1.3: Timing gate types + the pure `evalTimingGate` function

Pure, table-testable gate logic separated from Game plumbing.

**Files:**
- Create: `src/go/internal/ui/game_parity_timing_types.go`
- Test: `src/go/internal/ui/parity_timing_gate_test.go`

**Interfaces:**
- Produces:
```go
type timingBreach struct {
	Stage string  // "seqfire" | "bridge" | "overdue" | "emitdrift"
	Stat  string  // "mean" | "p99" | "stddev" | "absmax"
	Value float64
	Limit float64
	Count int64
}
// evalTimingStage returns the first breached statistic for one stage, or nil.
func evalTimingStage(stage string, b timingStageBounds, count int64, mean, p99, stddev, absMax float64, minSamples int) *timingBreach
```

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/parity_timing_gate_test.go
package ui

import "testing"

func TestEvalTimingStage(t *testing.T) {
	b := timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.040}

	// Under all bounds with enough samples → no breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.001, 0.005, 0.002, 0.02, 64); br != nil {
		t.Fatalf("unexpected breach: %+v", br)
	}
	// Not enough samples → statistical stats never breach even if huge.
	// (absMax kept < b.AbsMax=0.040 so the MinSamples-INDEPENDENT absmax check
	// does not fire here — that path is covered by the case below.)
	if br := evalTimingStage("emitdrift", b, 10, 1.0, 1.0, 1.0, 0.02, 64); br != nil {
		t.Fatalf("breach below MinSamples: %+v", br)
	}
	// Absmax dropout fires even at low sample count? No — MinSamples still
	// governs statistical stages; absmax uses its own single-sample rule:
	if br := evalTimingStage("emitdrift", b, 1, 0, 0, 0, 0.5, 64); br == nil || br.Stat != "absmax" {
		t.Fatalf("absmax dropout should fire alone, got %+v", br)
	}
	// Mean breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.004, 0.005, 0.002, 0.02, 64); br == nil || br.Stat != "mean" {
		t.Fatalf("expected mean breach, got %+v", br)
	}
	// P99 tail breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.001, 0.011, 0.002, 0.02, 64); br == nil || br.Stat != "p99" {
		t.Fatalf("expected p99 breach, got %+v", br)
	}
	// A bound of 0 disables that statistic.
	b2 := timingStageBounds{MeanMax: 0, P99Max: 0, StdDevMax: 0, AbsMax: 0}
	if br := evalTimingStage("bridge", b2, 1000, 5, 5, 5, 5, 1); br != nil {
		t.Fatalf("zero bounds must disable gating, got %+v", br)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEvalTimingStage -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

```go
// src/go/internal/ui/game_parity_timing_types.go
package ui

import "math"

// timingBreach describes the first statistic of a stage that exceeded its bound.
type timingBreach struct {
	Stage string
	Stat  string
	Value float64
	Limit float64
	Count int64
}

// evalTimingStage applies the multi-statistic gate to one stage's window.
// Order of checks: absmax (single-sample dropout, fires regardless of
// MinSamples), then mean/p99/stddev (statistical, require MinSamples). A bound
// of 0 disables that statistic. mean is compared by absolute value so a signed
// systematic bias in either direction is caught.
func evalTimingStage(stage string, b timingStageBounds, count int64, mean, p99, stddev, absMax float64, minSamples int) *timingBreach {
	// Dropout-class: one reading so large it is an audible glitch, not noise.
	if b.AbsMax > 0 && absMax > b.AbsMax {
		return &timingBreach{Stage: stage, Stat: "absmax", Value: absMax, Limit: b.AbsMax, Count: count}
	}
	if count < int64(minSamples) {
		return nil
	}
	if b.MeanMax > 0 && math.Abs(mean) > b.MeanMax {
		return &timingBreach{Stage: stage, Stat: "mean", Value: mean, Limit: b.MeanMax, Count: count}
	}
	if b.P99Max > 0 && p99 > b.P99Max {
		return &timingBreach{Stage: stage, Stat: "p99", Value: p99, Limit: b.P99Max, Count: count}
	}
	if b.StdDevMax > 0 && !math.IsNaN(stddev) && stddev > b.StdDevMax {
		return &timingBreach{Stage: stage, Stat: "stddev", Value: stddev, Limit: b.StdDevMax, Count: count}
	}
	return nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEvalTimingStage -v`
Expected: PASS.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/game_parity_timing_types.go src/go/internal/ui/parity_timing_gate_test.go
git commit -m "feat(timing): pure multi-statistic gate evaluator"
```

---

# PHASE 2 — Checker, activation, panic payload (Stages A/B/C live)

### Task 2.1: Timing activation state + env parsing

**Files:**
- Modify: `src/go/internal/ui/game_parity_tracker.go` (add `parityTimingWatch parityWatchMode` field)
- Create part of: `src/go/internal/ui/game_parity_timing.go` (env init + activation var)
- Test: `src/go/internal/ui/parity_timing_activation_test.go`

**Interfaces:**
- Produces: `var parityTimingFatalEnabled atomic.Bool`; `func SetParityTimingFatal(bool)`; package `init()` parsing `PARITY_TIMING` (`off|log|panic`) and `PARITY_TIMING_FATAL`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/parity_timing_activation_test.go
package ui

import "testing"

func TestParityTimingActivationDefaults(t *testing.T) {
	// Desktop default: fatal on. Under go test the process is desktop-profile.
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	SetParityTimingFatal(desktopRuntimeProfile().Timing.FatalDefault)
	if !parityTimingFatalEnabled.Load() {
		t.Error("expected desktop timing fatal default ON")
	}
	SetParityTimingFatal(false)
	if parityTimingFatalEnabled.Load() {
		t.Error("SetParityTimingFatal(false) did not stick")
	}
	SetParityTimingFatal(true) // restore for other tests
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityTimingActivationDefaults -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Create `game_parity_timing.go` header + activation (mirror `game.go` parity env pattern):

```go
// src/go/internal/ui/game_parity_timing.go
package ui

import (
	"os"
	"strings"
	"sync/atomic"
)

// parityTimingFatalEnabled gates whether a timing-gate breach panics.
// Default ON (desktop/test), forced OFF on browser unless PARITY_TIMING_FATAL
// arms it — mirrors parityFatalEnabled.
var parityTimingFatalEnabled atomic.Bool

// parityTimingWatchDefault is the initial watch mode for the timing gate.
var parityTimingWatchDefault parityWatchMode

func SetParityTimingFatal(enabled bool) { parityTimingFatalEnabled.Store(enabled) }

// resolveParityTimingActivation is a PURE helper (testable) that mirrors
// game.go's PARITY_WASM_FATAL safety net: on browser, fatal is forced OFF
// (watch=log) UNLESS PARITY_TIMING_FATAL explicitly arms it — so a leaked
// PARITY_TIMING=panic cannot crash WASM/Playwright runs. This enforces the
// project requirement "strict test/desktop-only; browser log-only, armable".
func resolveParityTimingActivation(seedWatch parityWatchMode, seedFatal bool, isBrowser bool, timingEnv, timingFatalEnv string) (watch parityWatchMode, fatal bool) {
	watch, fatal = seedWatch, seedFatal
	switch strings.ToLower(strings.TrimSpace(timingEnv)) {
	case "off":
		watch, fatal = parityWatchOff, false
	case "log", "1", "true":
		watch, fatal = parityWatchLog, false
	case "panic", "fatal":
		watch, fatal = parityWatchPanic, true
	}
	if isBrowser {
		armed := false
		switch strings.ToLower(strings.TrimSpace(timingFatalEnv)) {
		case "1", "true", "panic":
			armed = true
		}
		if armed {
			watch, fatal = parityWatchPanic, true
		} else {
			watch, fatal = parityWatchLog, false // force log-only unless armed
		}
	}
	return watch, fatal
}

func init() {
	tb := RuntimeProf().Timing
	w, f := resolveParityTimingActivation(tb.WatchDefault, tb.FatalDefault, RuntimeProf().IsBrowser, os.Getenv("PARITY_TIMING"), os.Getenv("PARITY_TIMING_FATAL"))
	parityTimingWatchDefault = w
	parityTimingFatalEnabled.Store(f)
}
```

Add to `parityTracker` struct in `game_parity_tracker.go`:

```go
	// parityTimingWatch is the per-Game timing-gate watch mode, seeded from
	// parityTimingWatchDefault at construction (mirrors parityWatch).
	parityTimingWatch parityWatchMode
```

Seed it in `game_new.go` next to `parityWatch = parityWatchDefault`:

```go
	g.parityTimingWatch = parityTimingWatchDefault
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityTimingActivationDefaults -v`
Expected: PASS.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/game_parity_timing.go src/go/internal/ui/game_parity_tracker.go src/go/internal/ui/game_new.go src/go/internal/ui/parity_timing_activation_test.go
git commit -m "feat(timing): activation state + PARITY_TIMING env"
```

---

### Task 2.2: `parityTimingCheck()` + `emitTimingFatal()` (Stages A/B/C)

Wire the gate to the live `schedMetrics` snapshot, with grace/warmup/stopped suppression, and a detailed panic reusing the parity dump plus a timing section.

**Files:**
- Modify: `src/go/internal/ui/game_parity_timing.go`
- Modify: `src/go/internal/ui/game_refresh_drum_row.go` (call site next to `parityScan`)
- Test: `src/go/internal/ui/parity_timing_check_test.go`, `src/go/internal/ui/parity_timing_grace_test.go`

**Interfaces:**
- Consumes: `evalTimingStage` (2.3→1.3), `schedMetrics.Snapshot()`, `parityInGrace`, `RuntimeProf().Timing`.
- Produces: `func (g *Game) parityTimingCheck()`, `func (g *Game) emitTimingFatal(br timingBreach, snap ScheduleMetricsSnapshot)`, `func (g *Game) buildTimingFatalDump(br timingBreach, snap ScheduleMetricsSnapshot) string`.

- [ ] **Step 1: Write the failing tests**

```go
// src/go/internal/ui/parity_timing_check_test.go
package ui

import "testing"

func TestParityTimingCheckPanicsOnEmitDropout(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })
	g.SetPlaying(true)

	// One dropout-class emission-drift reading (> desktop AbsMax 0.040).
	g.schedMetrics.ObserveEmitDrift(0.060)

	assertPanics(t, "emitdrift", func() { g.parityTimingCheck() })
}

func TestParityTimingCheckNoPanicUnderBounds(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })
	g.SetPlaying(true)

	// 200 healthy samples across stages, all well under bounds.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveEmitDrift(0.001)
		g.schedMetrics.ObserveBridge(0.0005)
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.Observe(1.0, 1.02) // healthy lead, no lag
	}
	// Must not panic.
	g.parityTimingCheck()
}

func TestParityTimingCheckSuppressedWhenStopped(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })
	g.SetPlaying(false) // stopped

	g.schedMetrics.ObserveEmitDrift(0.060) // would breach if playing
	g.parityTimingCheck()                  // no panic
}
```

```go
// src/go/internal/ui/parity_timing_grace_test.go
package ui

import (
	"testing"
	"time"
)

func TestParityTimingSuppressedDuringGrace(t *testing.T) {
	restore := SetParityGraceForceForTest(true)
	defer restore()
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })
	g.SetPlaying(true)

	g.parityGraceUntilNS.Store(time.Now().Add(50 * time.Millisecond).UnixNano())
	g.schedMetrics.ObserveEmitDrift(0.060) // dropout-class

	// Inside grace: no panic.
	g.parityTimingCheck()

	// After grace: panics on the same (still-present) breach.
	time.Sleep(70 * time.Millisecond)
	assertPanics(t, "emitdrift", func() { g.parityTimingCheck() })
}
```

> Implementer: `assertPanics(t, wantSubstr, fn)` recovers a panic and asserts its message contains `wantSubstr`. `emitTimingFatal` must panic with a message that includes the stage name. Add `assertPanics` to `parity_test_helpers_test.go` if absent.

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParityTiming(Check|Suppressed)' -v`
Expected: FAIL — `parityTimingCheck` undefined.

- [ ] **Step 3: Implement the checker**

Append to `game_parity_timing.go`:

```go
// parityTimingCheck evaluates the four-stage timing gate against the live
// schedule-metrics window. It runs on the UI thread at parity-scan cadence.
// Suppressed while stopped/paused, during the parity grace window (so live
// circuit/rule edits never false-trigger), and until each stage's window has
// RuntimeProf().Timing.MinSamples samples. Warmup (0.5s) is already applied
// inside schedMetrics. On breach it routes through the same watch/fatal policy
// as the logical checker.
func (g *Game) parityTimingCheck() {
	tb := RuntimeProf().Timing
	if !tb.Enabled {
		return
	}
	if g.parityTimingWatch == parityWatchOff && !parityTimingFatalEnabled.Load() {
		return
	}
	if !g.Playing() || g.Paused() {
		return
	}
	if g.parityInGrace() {
		return
	}
	snap := g.schedMetrics.Snapshot()

	if br := evalTimingStage("seqfire", tb.SeqFire, snap.SeqFireCount, snap.SeqFireAvg, snap.SeqFireP99, snap.SeqFireStdDev, snap.SeqFireMax, tb.MinSamples); br != nil {
		g.emitTimingFatal(*br, snap)
		return
	}
	if br := evalTimingStage("bridge", tb.Bridge, snap.BridgeCount, snap.BridgeAvg, snap.BridgeP99, snap.BridgeStdDev, snap.BridgeMax, tb.MinSamples); br != nil {
		g.emitTimingFatal(*br, snap)
		return
	}
	// Stage C — overdue/lag. mean=AvgLag, p99=LagP99, absmax=MaxLag; no stddev.
	if br := evalTimingStage("overdue", tb.Overdue, snap.Count, snap.AvgLag, snap.LagP99, 0, snap.MaxLag, tb.MinSamples); br != nil {
		g.emitTimingFatal(*br, snap)
		return
	}
	if br := evalTimingStage("emitdrift", tb.EmitDrift, snap.EmitCount, snap.EmitAvg, snap.EmitP99, snap.EmitStdDev, snap.EmitAbsMax, tb.MinSamples); br != nil {
		g.emitTimingFatal(*br, snap)
		return
	}
}

// buildTimingFatalDump reuses the logical parity dump and appends a timing
// section: the offending stage/statistic, the full three-stage snapshot, and
// (desktop) the device-clock anchor.
func (g *Game) buildTimingFatalDump(br timingBreach, snap ScheduleMetricsSnapshot) string {
	base := g.buildParityFatalDump(mismatchEntry{
		Row: -1, Abs: -1, Source: "timing", Kind: "timing_" + br.Stage,
		Detail: br.Stat, When: br.Value,
	})
	b := &strings.Builder{}
	b.WriteString(base)
	b.WriteString("[PARITY][timing] gate breach\n")
	fmtTimingLine(b, "stage", br.Stage)
	fmtTimingLine(b, "stat", br.Stat)
	fmtTimingF(b, "value", br.Value)
	fmtTimingF(b, "limit", br.Limit)
	fmtTimingI(b, "windowSamples", br.Count)
	fmtTimingF(b, "seqFire.avg", snap.SeqFireAvg)
	fmtTimingF(b, "seqFire.p99", snap.SeqFireP99)
	fmtTimingF(b, "seqFire.max", snap.SeqFireMax)
	fmtTimingF(b, "bridge.avg", snap.BridgeAvg)
	fmtTimingF(b, "bridge.p99", snap.BridgeP99)
	fmtTimingF(b, "bridge.max", snap.BridgeMax)
	fmtTimingF(b, "overdue.avgLag", snap.AvgLag)
	fmtTimingF(b, "overdue.p99Lag", snap.LagP99)
	fmtTimingF(b, "overdue.maxLag", snap.MaxLag)
	fmtTimingI(b, "overdueCount", snap.Overdue)
	fmtTimingF(b, "emit.avg", snap.EmitAvg)
	fmtTimingF(b, "emit.p99", snap.EmitP99)
	fmtTimingF(b, "emit.absMax", snap.EmitAbsMax)
	fmtTimingF(b, "emit.stdDev", snap.EmitStdDev)
	fmtTimingF(b, "e2eP99", snap.E2EP99)
	// Desktop device-clock anchor (see engine_clock_probe.go). No-op elsewhere.
	// deviceClockSnapshot is a package var (below) reassigned to
	// audio.DeviceClockSnapshot by the desktop build in Task 3.1.
	if dp, dw, ok := deviceClockSnapshot(); ok {
		fmtTimingI(b, "device.pos", dp)
		fmtTimingF(b, "device.wallSec", dw)
	}
	return b.String()
}

func (g *Game) emitTimingFatal(br timingBreach, snap ScheduleMetricsSnapshot) {
	fatal := parityTimingFatalEnabled.Load()
	if g.parityTimingWatch == parityWatchOff && !fatal {
		return
	}
	msg := g.buildTimingFatalDump(br, snap)
	if g.logger != nil {
		if fatal {
			g.logger.Errorf(msg)
		} else {
			g.logger.Warnf(msg)
		}
	}
	if os.Getenv("PARITY_DUMP_STDERR") == "1" {
		fmt.Fprint(os.Stderr, msg)
	}
	if fatal {
		panic("audio timing parity breach: " + br.Stage + "/" + br.Stat)
	}
}
```

Add the small format helpers + the device-clock placeholder (imports: `fmt`, `strings` — do NOT import `audio` yet) at the top of `game_parity_timing.go`:

```go
func fmtTimingLine(b *strings.Builder, k, v string) { fmt.Fprintf(b, " %s=%s\n", k, v) }
func fmtTimingF(b *strings.Builder, k string, v float64) { fmt.Fprintf(b, " %s=%.6f\n", k, v) }
func fmtTimingI(b *strings.Builder, k string, v int64) { fmt.Fprintf(b, " %s=%d\n", k, v) }

// deviceClockSnapshot reports (devPos, wallSec, ok). Phase 2 stub returns
// unavailable; Task 3.1 reassigns it to audio.DeviceClockSnapshot on desktop.
var deviceClockSnapshot = func() (int64, float64, bool) { return 0, 0, false }
```

> This avoids a forward dependency on the `audio` package for Phase 2 and keeps each task independently compilable/testable. Task 3.1 replaces the placeholder body (see that task).

- [ ] **Step 4: Wire the call site**

In `game_refresh_drum_row.go`, immediately after the existing `g.parityScan("refresh")` call (~line 430), add:

```go
	g.parityTimingCheck()
```

- [ ] **Step 5: Run to verify tests pass + full parity suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParityTiming|Parity' -v -count=1`
Expected: PASS.

- [ ] **Step 6: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/game_parity_timing.go src/go/internal/ui/game_refresh_drum_row.go src/go/internal/ui/parity_timing_check_test.go src/go/internal/ui/parity_timing_grace_test.go
git commit -m "feat(timing): parityTimingCheck gate + detailed panic (stages A/B/C)"
```

---

### Task 2.3: Stress test — gate holds under load, no false positives

Drive a real playback loop at high BPM with many rows + rapid live edits and assert the gate does not fire on healthy timing, and DOES fire when synthetic drift is injected.

**Files:**
- Test: `src/go/internal/ui/parity_timing_stress_test.go`

- [ ] **Step 1: Write the test**

```go
// src/go/internal/ui/parity_timing_stress_test.go
package ui

import "testing"

func TestParityTimingStressNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })

	// Many rows, high BPM, deterministic Update()-driven scheduling.
	loadStressCircuitForTest(t, g, 8 /*rows*/, 240 /*bpm*/)
	g.SetPlaying(true)

	// Run 3000 abs of real advancement with periodic live edits.
	for i := 0; i < 3000; i++ {
		advancePlaybackByAbs(t, g, 1) // existing helper (parity_decisions_bound_test.go)
		if i%250 == 0 {
			cycleInstrumentForTest(t, g, i%8) // live edit → bumps parityGen/grace
		}
		g.refreshDrumRow(0) // drives parityScan + parityTimingCheck
	}
	// Reaching here without panic proves no false positive under stress.
}

func TestParityTimingStressCatchesInjectedDrift(t *testing.T) {
	g := newTestGameForParity(t)
	g.parityTimingWatch = parityWatchPanic
	SetParityTimingFatal(true)
	t.Cleanup(func() { SetParityTimingFatal(true) })
	g.SetPlaying(true)

	// Fill the emit window with a systematically-late stream (mean breach).
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveEmitDrift(0.005) // > desktop EmitDrift.MeanMax 0.003
	}
	assertPanics(t, "emitdrift", func() { g.parityTimingCheck() })
}
```

> Implementer: `loadStressCircuitForTest`, `advancePlaybackByAbs`, `cycleInstrumentForTest` exist or are trivial wrappers over helpers in `parity_decisions_bound_test.go` / `parity_runtime_mutation_matrix_test.go`. Reuse; do not duplicate the circuit builder.

- [ ] **Step 2: Run**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestParityTimingStress -v -count=1`
Expected: PASS. If `NoFalsePositive` panics, the tolerances are too tight for the harness — record the offending stage/values, loosen the corresponding `desktopRuntimeProfile().Timing` bound with a comment, and re-run. Do NOT silence by disabling the gate.

- [ ] **Step 3: Commit** (checkpoint only)

```bash
git add src/go/internal/ui/parity_timing_stress_test.go
git commit -m "test(timing): stress no-false-positive + injected-drift positive"
```

---

# PHASE 3 — Desktop Stage-D (mixer emission + device clock)

### Task 3.1: Device-clock anchor + emission-report ring in the mixer

**Files:**
- Create: `src/go/internal/audio/engine_clock_probe.go` (`//go:build !test && !js`)
- Test: `src/go/internal/audio/engine_clock_probe_test.go` (`//go:build !test && !js`)

**Interfaces:**
- Produces:
```go
// DeviceClockSnapshot returns (samplePos, wallSeconds, ok) — the mixer's
// device sample position paired with the wall time it was observed. ok=false
// before the first Read().
func DeviceClockSnapshot() (int64, float64, bool)

// EmitReport identifies a voice's first-emitted block. Exported so the UI
// package can drain into a []audio.EmitReport.
type EmitReport struct{ Row, Abs int; DevPos int64 }

// DrainEmitReports copies pending emission reports into dst[:n] and returns n
// (lock-free SPSC drain; caller is the audio loop).
func DrainEmitReports(dst []EmitReport) int

// publishEmitReport is called from processBlock (producer). Non-blocking; drops
// on ring-full (bounded).
func publishEmitReport(row, abs int, devPos int64)

// updateDeviceClock is called once per Read() with the post-block pos.
func updateDeviceClock(pos int64)
```

- [ ] **Step 1: Write the failing test**

```go
//go:build !test && !js

package audio

import "testing"

func TestDeviceClockAndEmitRing(t *testing.T) {
	resetClockProbeForTest()

	if _, _, ok := DeviceClockSnapshot(); ok {
		t.Fatal("snapshot ok before any Read()")
	}
	updateDeviceClock(48000)
	pos, wall, ok := DeviceClockSnapshot()
	if !ok || pos != 48000 || wall <= 0 {
		t.Fatalf("snapshot=(%d,%f,%v)", pos, wall, ok)
	}

	publishEmitReport(1, 42, 48000)
	publishEmitReport(2, 43, 48064)
	dst := make([]EmitReport, 8)
	n := DrainEmitReports(dst)
	if n != 2 || dst[0].Row != 1 || dst[0].Abs != 42 || dst[1].DevPos != 48064 {
		t.Fatalf("drain n=%d dst=%+v", n, dst[:max(n, 0)])
	}
	if DrainEmitReports(dst) != 0 {
		t.Fatal("second drain should be empty")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run TestDeviceClockAndEmitRing -v`
(This file is desktop-tagged, so it needs the non-test build — run without `-tags test`. `xvfb-run` is used because the desktop audio package links Oto; if it builds headless without X, plain `go test ./internal/audio/ -run TestDeviceClockAndEmitRing` also works.)
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

```go
//go:build !test && !js

package audio

import (
	"sync/atomic"
	"time"
)

// --- Device-clock anchor (seqlock pair) -----------------------------------
// A single writer (the Oto Read goroutine) publishes (pos, wallNano); readers
// (UI parity thread) get a torn-free pair via an even/odd version counter.
var (
	dcVersion atomic.Uint64
	dcPos     atomic.Int64
	dcWall    atomic.Int64 // UnixNano
)

func updateDeviceClock(pos int64) {
	dcVersion.Add(1) // odd → write in progress
	dcPos.Store(pos)
	dcWall.Store(time.Now().UnixNano())
	dcVersion.Add(1) // even → stable
}

// DeviceClockSnapshot returns (pos, wallSeconds, ok). wallSeconds is relative
// to program start (same base as Now()).
func DeviceClockSnapshot() (int64, float64, bool) {
	for {
		v1 := dcVersion.Load()
		if v1 == 0 {
			return 0, 0, false
		}
		if v1&1 != 0 {
			continue // write in progress
		}
		pos := dcPos.Load()
		wall := dcWall.Load()
		if dcVersion.Load() != v1 {
			continue // torn; retry
		}
		return pos, time.Duration(wall - start.UnixNano()).Seconds(), true
	}
}

// --- Emission-report SPSC ring --------------------------------------------
type EmitReport struct {
	Row, Abs int
	DevPos   int64
}

const emitRingSize = 1024 // power of two

var (
	emitRing  [emitRingSize]EmitReport
	emitHead  atomic.Uint64 // producer (processBlock)
	emitTail  atomic.Uint64 // consumer (audio loop)
	emitDrops atomic.Uint64
)

// publishEmitReport is lock-free, non-blocking, single-producer. Drops on full.
func publishEmitReport(row, abs int, devPos int64) {
	h := emitHead.Load()
	t := emitTail.Load()
	if h-t >= emitRingSize {
		emitDrops.Add(1)
		return
	}
	emitRing[h&(emitRingSize-1)] = EmitReport{Row: row, Abs: abs, DevPos: devPos}
	emitHead.Store(h + 1) // publish after the slot write
}

// DrainEmitReports copies up to len(dst) reports; single-consumer.
func DrainEmitReports(dst []EmitReport) int {
	h := emitHead.Load()
	t := emitTail.Load()
	n := 0
	for t < h && n < len(dst) {
		dst[n] = emitRing[t&(emitRingSize-1)]
		t++
		n++
	}
	emitTail.Store(t)
	return n
}

func resetClockProbeForTest() {
	dcVersion.Store(0)
	dcPos.Store(0)
	dcWall.Store(0)
	emitHead.Store(0)
	emitTail.Store(0)
	emitDrops.Store(0)
}
```

> `start` is the package var set in `engine.go:38` (`start = time.Now()`); `DeviceClockSnapshot` reuses it so device-wall seconds share `Now()`'s base. Confirm `start` is `time.Time` (it is — `time.Since(start)` in `Now()`).

**Add stubs so all builds compile.** The exported `EmitReport` type + `DeviceClockSnapshot` + `DrainEmitReports` must exist under every build tag:
- `internal/audio/stub.go` (`test` build): declare `type EmitReport struct{ Row, Abs int; DevPos int64 }`, `func DeviceClockSnapshot() (int64, float64, bool) { return 0, 0, false }`, `func DrainEmitReports(dst []EmitReport) int { return 0 }`, and `func SampleRate() int { return sampleRate }` (if not already present).
- `internal/audio/engine_wasm.go` (`js` build): same three functions + the `EmitReport` type (WASM has no mixer; all stubs return zero/unavailable — browser Stage-D comes from JS in Phase 4, not this ring).

Then in `game_parity_timing.go` reassign the Phase-2 placeholder so the desktop dump uses the real anchor:
```go
func init() { deviceClockSnapshot = audio.DeviceClockSnapshot }
```
(add the `audio` import to `game_parity_timing.go` now — it is safe because the stubs above make `audio.DeviceClockSnapshot` resolve under `test`/`js` too).

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestDeviceClockAndEmitRing -v`
Expected: PASS.
Also verify all three build tags compile:
Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: all succeed.

- [ ] **Step 5: Commit** (checkpoint only)

```bash
git add src/go/internal/audio/engine_clock_probe.go src/go/internal/audio/engine_clock_probe_test.go src/go/internal/audio/stub.go src/go/internal/audio/engine_wasm.go src/go/internal/ui/game_parity_timing.go
git commit -m "feat(audio): desktop device-clock anchor + lock-free emission ring"
```

---

### Task 3.2: Thread `(row, abs)` through to `voiceState` and publish emission on first render

**Files:**
- Modify: `src/go/internal/audio/types.go` (`BatchParam` +`Row,Abs`)
- Modify: `src/go/internal/audio/engine_mixer.go` (`voiceState` +`row,abs`; `Schedule` signature)
- Modify: `src/go/internal/audio/engine_play.go` (`PlayParams`/`PlayParamsAt` thread identity)
- Modify: `src/go/internal/audio/batch_desktop.go` (pass identity)
- Modify: `src/go/internal/audio/engine_stop.go` (`processBlock` publish on first render)
- Modify: `src/go/internal/ui/game_audio_loop.go` (`BatchParam` carries `req.row/req.abs`)
- Test: `src/go/internal/audio/engine_emit_report_test.go` (`//go:build !test && !js`)

**Interfaces:**
- Produces: `Schedule(id string, v Voice, delaySamples, row, abs int)`; `BatchParam{...; Row, Abs int}`; emission published when a voice's first block renders.

- [ ] **Step 1: Write the failing test**

```go
//go:build !test && !js

package audio

import "testing"

func TestVoiceEmissionPublishesReport(t *testing.T) {
	resetClockProbeForTest()
	m := newTestMixer(t) // helper: constructs a mixer without a live Oto player if one exists; else use newMixer with a fake context

	// Schedule a voice to start after 128 samples, tagged (row=3, abs=9).
	m.pos = 0
	m.scheduleForTest("kick", 128, 3, 9) // helper wrapping Schedule with a silent test voice

	// Drive Read for enough samples to cross the start boundary.
	buf := make([]byte, 2*512)
	_, _ = m.Read(buf)

	dst := make([]emitReport, 8)
	n := DrainEmitReports(dst)
	found := false
	for _, r := range dst[:n] {
		if r.Row == 3 && r.Abs == 9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no emit report for (3,9); got %+v", dst[:n])
	}
}
```

> Implementer: if constructing a real `*oto.Player` in a unit test is impractical, add a minimal `newTestMixer`/`scheduleForTest` that builds a `mixer{}` with pre-allocated buffers and a silent voice and calls `processBlock` directly, bypassing Oto. Model it on any existing `engine_*_test.go` that exercises `processBlock` without a device (grep for `processBlock(` in `internal/audio/*_test.go`). Only publish-on-first-render behavior is under test.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestVoiceEmissionPublishesReport -v`
Expected: FAIL — `Schedule` arity / `scheduleForTest` undefined.

- [ ] **Step 3: Implement the thread-through + publish**

`types.go` — add fields:
```go
type BatchParam struct {
	ID      string
	Vol     float64
	Pitch   float64
	Dur     float64
	When    float64
	HasWhen bool
	Row     int // parity identity (-1 for fire-and-forget)
	Abs     int
}
```

`engine_mixer.go` — extend `voiceState` and `Schedule`:
```go
type voiceState struct {
	start    int
	id       string
	slot     int
	v        Voice
	ch       *Channel
	row, abs int  // parity identity; (-1,-1) for non-sequencer sounds
	emitted  bool // guard so we publish exactly one report per voice
}

func (m *mixer) Schedule(id string, v Voice, delaySamples, row, abs int) {
	// ... unchanged body up to the voiceState construction ...
	vs := &voiceState{start: m.pos + delaySamples, id: id, slot: slot, v: v, ch: ch, row: row, abs: abs}
	// ... unchanged pending append ...
}
```

`engine_play.go` — thread identity. Change `PlayParams` to accept identity (add trailing params with a non-breaking default via a new internal function), and `PlayParamsAt`:
```go
func PlayParamsAt(id string, vol, pitch, dur, when float64, rowAbs ...int) {
	row, abs := -1, -1
	if len(rowAbs) == 2 {
		row, abs = rowAbs[0], rowAbs[1]
	}
	if when > 0 {
		playParamsIdentity(id, vol, pitch, dur, row, abs, when)
	} else {
		playParamsIdentity(id, vol, pitch, dur, row, abs)
	}
}
```
Rename the existing `PlayParams` body to `playParamsIdentity(id string, vol, pitch, dur float64, row, abs int, when ...float64)`, and have the old exported `PlayParams(id, vol, pitch, dur, when...)` delegate with `row=-1, abs=-1` to preserve every existing caller. At the `mix.Schedule(...)` call inside, pass `row, abs`.

`batch_desktop.go` — pass identity:
```go
func PlayBatch(reqs []BatchParam) {
	for i := range reqs {
		r := reqs[i]
		when := 0.0
		if r.HasWhen {
			when = r.When
		}
		PlayParamsAt(r.ID, r.Vol, r.Pitch, r.Dur, when, r.Row, r.Abs)
	}
}
```

`engine_stop.go` `processBlock` — publish on first render. Inside the Phase-1 voice loop, right after the `if startPos < vs.start { i++; continue }` guard (the point where we know the voice is rendering this block), add:
```go
		if !vs.emitted && vs.row >= 0 {
			publishEmitReport(vs.row, vs.abs, int64(startPos))
			vs.emitted = true
		}
```
And at the end of `processBlock` where `m.pos += blockLen`, also refresh the device clock:
```go
	m.pos += blockLen
	updateDeviceClock(int64(m.pos))
```
(There are three `m.pos += blockLen` sites — processTestTone:341, processRawVoice:400, processBlock:665. Add `updateDeviceClock(int64(m.pos))` after each so the anchor tracks the real device clock in every mode.)

`game_audio_loop.go` — set identity on the `BatchParam`. At the `b := audio.BatchParam{...}` construction (~line 172), add:
```go
			b := audio.BatchParam{ID: req.id, Vol: req.vol, Pitch: req.pitch, Dur: req.dur, Row: req.row, Abs: req.abs}
```

- [ ] **Step 4: Run to verify it passes + all builds compile**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run TestVoiceEmissionPublishesReport -v`
Expected: PASS.
Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: all succeed (fix any caller of `Schedule`/`PlayParamsAt` in `test`/`js` stubs to the new signatures — the `test` stub `PlayBatch`/`PlayParamsAt` must accept the new params too).

- [ ] **Step 5: Alloc discipline check**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Alloc|ProcessBlock' -bench . -benchmem 2>/dev/null | head` (if a processBlock benchmark exists) — confirm the per-block alloc count is unchanged. If an alloc discipline test exists (grep `AllocsPerRun` in `internal/audio`), run it; `publishEmitReport` must be zero-alloc (it writes into a fixed array).
Expected: no new allocations in `processBlock`.

- [ ] **Step 6: Commit** (checkpoint only)

```bash
git add -A src/go/internal/audio src/go/internal/ui/game_audio_loop.go
git commit -m "feat(audio): thread parity identity to voiceState + publish emission report"
```

---

### Task 3.3: Close traces from emission reports and feed Stage-D drift

The audio loop drains emission reports each iteration, matches them to their scheduled ideal-when, and feeds `ObserveEmitDrift`. Because the mixer only knows the device sample-pos, the UI maps it to wall-seconds via `DeviceClockSnapshot` and compares to the trace's `idealWhen`.

**Files:**
- Modify: `src/go/internal/ui/game_parity_timing_types.go` — add the compact `timingTrace` type.
- Modify: `src/go/internal/ui/game_parity_tracker.go` — add `timingTraces map[int64]timingTrace` + `emissionReportBuf []audio.EmitReport`.
- Modify: `src/go/internal/ui/game_parity_timing.go` — `openTimingTrace` / `closeTimingTrace` / `timingTraceKey`.
- Create: `src/go/internal/ui/parity_timing_drain_notjs.go` (`//go:build !js`) — `drainEmissionReports`.
- Modify: `src/go/internal/ui/game_new.go` — init `timingTraces` + `emissionReportBuf`.
- Modify: `src/go/internal/ui/game_audio_schedule.go` — open a trace in `scheduleSound` (record `idealWhen`).
- Modify: `src/go/internal/ui/game_audio_loop.go` — call `drainEmissionReports()` after `PlayBatch`.
- Test: `src/go/internal/ui/parity_timing_stage_d_desktop_test.go`

**Interfaces:**
- Produces: `func (g *Game) openTimingTrace(row, abs int, idealWhen float64, gen, parityGen uint64)`; `func (g *Game) drainEmissionReports()`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/ui/parity_timing_stage_d_desktop_test.go
package ui

import (
	"math"
	"testing"
)

func TestStageDDriftFedFromEmission(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlaying(true)

	// Open a trace: ideal emission at t=1.000s.
	g.openTimingTrace(0, 5, 1.000, g.audioGen.Load(), g.parityGen.Load())

	// Simulate an emission report whose mapped wall-time is 1.008s → +8ms drift.
	feedEmissionReportForTest(t, g, 0, 5, /*mappedWallSec=*/1.008)

	s := g.schedMetrics.Snapshot()
	if s.EmitCount != 1 {
		t.Fatalf("EmitCount=%d want 1", s.EmitCount)
	}
	if math.Abs(s.EmitAbsMax-0.008) > 1e-6 {
		t.Errorf("EmitAbsMax=%.6f want 0.008", s.EmitAbsMax)
	}
}
```

> Implementer: `feedEmissionReportForTest` bridges the desktop-only `audio.DrainEmitReports` (which needs a device pos, not a wall time). To keep this a portable `-tags test` UI test, have `drainEmissionReports` accept an injectable source: define `var emissionReportSource = audio.DrainEmitReports` (desktop) with a `test`/`js` stub returning 0, and in the test replace `g`'s mapping so a report `(row,abs,devPos)` maps to the given wall-second. Simplest: expose `g.closeTimingTrace(row, abs, emittedWallSec float64)` as the seam that `drainEmissionReports` calls after mapping devPos→wall, and have the test call `closeTimingTrace` directly. Then `feedEmissionReportForTest` == `g.closeTimingTrace(0,5,1.008)`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStageDDriftFedFromEmission -v`
Expected: FAIL — `openTimingTrace`/`closeTimingTrace` undefined.

- [ ] **Step 3: Implement the trace + close seam**

Add to `game_ui_types.go` (or a new small file `game_parity_timing_types.go` already created — put it there to keep timing code together):

```go
type timingTrace struct {
	row, abs  int
	gen       uint64
	parityGen uint64
	idealWhen float64
	openAt    time.Time
}
```

Add to `parityTracker` (in `game_parity_tracker.go`), guarded by `parityMu`:
```go
	// timingTraces maps (row<<32|abs) → open timing trace, closed when the
	// mixer/WebAudio reports the voice's actual emission. Bounded by pruning
	// alongside the other parity buffers.
	timingTraces map[int64]timingTrace
```
Initialize it in `game_new.go` where the other parity maps are made: `g.timingTraces = make(map[int64]timingTrace)`.

Add the seams in `game_parity_timing.go`:
```go
func timingTraceKey(row, abs int) int64 { return int64(row)<<32 | int64(uint32(abs)) }

func (g *Game) openTimingTrace(row, abs int, idealWhen float64, gen, parityGen uint64) {
	if row < 0 {
		return
	}
	if !RuntimeProf().Timing.Enabled {
		return
	}
	g.parityMu.Lock()
	g.timingTraces[timingTraceKey(row, abs)] = timingTrace{
		row: row, abs: abs, gen: gen, parityGen: parityGen, idealWhen: idealWhen, openAt: time.Now(),
	}
	// Bound: drop far-behind traces (never closed, e.g. dropped audio).
	if len(g.timingTraces) > 2048 {
		for k, tr := range g.timingTraces {
			if time.Since(tr.openAt) > 2*time.Second {
				delete(g.timingTraces, k)
			}
		}
	}
	g.parityMu.Unlock()
}

// closeTimingTrace matches an emission to its open trace, feeds Stage-D drift,
// and removes the trace. Stale-generation reports are dropped (mirrors the
// audio-parity gen filter) so a report that lands after a live edit does not
// pollute the drift window.
func (g *Game) closeTimingTrace(row, abs int, emittedWallSec float64) {
	g.parityMu.Lock()
	key := timingTraceKey(row, abs)
	tr, ok := g.timingTraces[key]
	if ok {
		delete(g.timingTraces, key)
	}
	curGen := g.audioGen.Load()
	curParityGen := g.parityGen.Load()
	g.parityMu.Unlock()
	if !ok {
		return
	}
	if tr.gen != curGen || tr.parityGen != curParityGen {
		return // stale — dropped by generation filter
	}
	g.schedMetrics.ObserveEmitDrift(emittedWallSec - tr.idealWhen)
}
```

Add a reusable per-Game drain buffer on the `parityTracker` (or Game) and allocate it in `game_new.go`:
```go
	emissionReportBuf []audio.EmitReport // scratch for draining mixer emission reports
```
```go
	g.emissionReportBuf = make([]audio.EmitReport, 128) // in game_new.go, near timingTraces init
```

Create the desktop drain in its final build-tagged file **now** (Task 4.2 only adds the `js` sibling — it does not redefine this):
```go
// src/go/internal/ui/parity_timing_drain_notjs.go
//go:build !js

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// drainEmissionReports pulls the mixer's lock-free emission reports, maps each
// device sample-pos to wall-seconds via the device-clock anchor, and closes the
// matching timing trace (feeding Stage-D drift). Under -tags test the audio
// stub returns 0, so UI tests drive Stage-D via closeTimingTrace directly.
func (g *Game) drainEmissionReports() {
	n := audio.DrainEmitReports(g.emissionReportBuf)
	if n == 0 {
		return
	}
	pos, wall, ok := audio.DeviceClockSnapshot()
	if !ok {
		return
	}
	sr := float64(audio.SampleRate())
	for i := 0; i < n; i++ {
		r := g.emissionReportBuf[i]
		g.closeTimingTrace(r.Row, r.Abs, wall+float64(r.DevPos-pos)/sr)
	}
}
```
(`audio.SampleRate()` — add the trivial exported getter in Task 3.1's stub step if it does not already exist; grep `sampleRate` in `internal/audio` first.)

Wire the drain into `audioLoop` (`game_audio_loop.go`): in the default dispatch path, right after `audio.PlayBatch(batch)`, add:
```go
		g.drainEmissionReports()
```
so reports from the just-dispatched and previously-playing voices are collected each cycle.

- [ ] **Step 4: Open the trace at schedule time**

In `game_audio_schedule.go` `queueSoundAtParams`, after `g.markSeqAudioEnqueued(row, abs)`, open the trace with the ideal-when (the pre-lookahead grid time). The cleanest `idealWhen` is the `beatIdealAudio` passed into `scheduleSound`; thread it through. Minimal approach: open the trace in `scheduleSound` right before `queueSoundAtParams`, using `baseNow` (the ideal audio time before groove/lookahead):
```go
	if sequencer {
		g.openTimingTrace(row, idx, baseNow, g.audioGen.Load(), g.parityGen.Load())
	}
```
Insert this just before the `g.queueSoundAtParams(...)` call in `scheduleSound`. (`baseNow` is the ideal grid time; groove + lookahead are intentionally excluded so Stage-D drift measures scheduler-intended vs actually-produced, not the deliberate lookahead cushion.)

- [ ] **Step 5: Run to verify it passes + builds**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestStageDDriftFedFromEmission|TestParityTiming' -v -count=1`
Expected: PASS.
Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: all succeed.

- [ ] **Step 6: Race check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/ui/ -run 'TestParityTimingStress' -count=1`
Expected: PASS, no race reports (trace map under `parityMu`; device clock via seqlock; emit ring SPSC).

- [ ] **Step 7: Commit** (checkpoint only)

```bash
git add -A src/go/internal/ui src/go/internal/audio
git commit -m "feat(timing): close traces from mixer emission → Stage-D drift"
```

---

# PHASE 4 — Browser Stage-D (WebAudio emission)

### Task 4.1: JS timing ring + `getTimingTraces`/`resetTimingTraces`

**Files:**
- Modify: `src/js/audio.js`
- Test: `src/js/timing_traces.browser.test.js` (new)

**Interfaces:**
- Produces: `window.getTimingTraces()` → `[{row, abs, idealWhen, startWhen, ctxAtStart}]`; `window.resetTimingTraces()`. Recorded at each real `BufferSource.start(when)`.

- [ ] **Step 1: Write the failing browser test**

```js
// src/js/timing_traces.browser.test.js
// COVERED-BY-GO: none (WebAudio emission timing is a JS-only boundary).
import { startHarness } from './real_input_test_helpers.js';

const t = await startHarness();
await t.evaluate(() => window.resetTimingTraces && window.resetTimingTraces());

// Play a few scheduled beats.
await t.evaluate(() => window.startPlay());
await t.wait(400);
await t.evaluate(() => window.stop());

const traces = await t.evaluate(() => window.getTimingTraces());
if (!Array.isArray(traces)) throw new Error('getTimingTraces did not return an array');
if (traces.length === 0) throw new Error('no timing traces recorded during playback');
for (const tr of traces) {
  if (typeof tr.startWhen !== 'number' || typeof tr.ctxAtStart !== 'number') {
    throw new Error('trace missing startWhen/ctxAtStart: ' + JSON.stringify(tr));
  }
}
console.log('PASS timing_traces:', traces.length, 'traces');
await t.close();
```

> Implementer: match the harness import/lifecycle used by neighboring browser tests (`webaudio_smoke.browser.test.js`). Use whatever start/stop export names those tests use.

- [ ] **Step 2: Run to verify it fails**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/timing_traces.browser.test.js`
Expected: FAIL — `getTimingTraces` undefined / returns nothing.

- [ ] **Step 3: Implement the JS ring**

In `src/js/audio.js`, near the `scheduleMetrics` / `renderLatencyMetrics` objects (~lines 256-451), add:

```js
const TIMING_TRACES_MAX = 512;
const timingTraces = {
  buf: [],
  push(row, abs, idealWhen, startWhen, ctxAtStart) {
    if (this.buf.length >= TIMING_TRACES_MAX) this.buf.shift();
    this.buf.push({ row, abs, idealWhen, startWhen, ctxAtStart });
  },
  reset() { this.buf.length = 0; },
  snapshot() { return this.buf.slice(); },
};
window.getTimingTraces = () => timingTraces.snapshot();
window.resetTimingTraces = () => timingTraces.reset();
```

At every real `BufferSource.start(when)` dispatch that carries sequencer identity (the sites near lines 754, 784, and the enqueue path 640-760), record the trace. The events already carry the intended `when`; thread `row`/`abs` from the batch event (Go sends them in `BatchParam` — ensure the WASM `PlayBatch` bridge includes `Row`/`Abs` in the flat batch; if the JS batch decode does not currently read them, add the two fields to the batch marshaling in `batch_wasm.go` and the JS decode). Then at `.start(when)`:

```js
        source.start(when);
        if (ev.row !== undefined && ev.row >= 0) {
          timingTraces.push(ev.row, ev.abs, ev.idealWhen ?? when, when, ctx.currentTime);
        }
```

`ev.idealWhen` is the pre-clamp intended time; if only `when` is available at that site, pass `when` for both (browser Stage-D then measures clamp/render slippage, which is the browser-relevant drift). Keep it minimal: `startWhen` (scheduled) vs `ctxAtStart` (the true clock at scheduling call) already exposes lateness.

- [ ] **Step 4: Run to verify it passes**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/timing_traces.browser.test.js`
Expected: PASS.

- [ ] **Step 5: Add to the bridge smoke catalogue**

Per the JS Test Charter, add `getTimingTraces`/`resetTimingTraces` existence to `src/js/wasm_bridge_smoke.browser.test.js`'s catalogue (not a new dedicated file — this test file is justified because it verifies WebAudio emission recording, a JS-only boundary, but the export existence still belongs in the catalogue).

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS.

- [ ] **Step 6: Commit** (checkpoint only)

```bash
git add src/js/audio.js src/js/timing_traces.browser.test.js src/js/wasm_bridge_smoke.browser.test.js
git commit -m "feat(timing): WebAudio emission timing ring + getTimingTraces"
```

---

### Task 4.2: Go-WASM pull of browser emission traces → Stage-D drift

On WASM, the UI checker pulls `getTimingTraces()` across the bridge at scan cadence and closes traces (mirrors desktop `drainEmissionReports`).

**Files:**
- Create: `src/go/internal/ui/parity_timing_drain_js.go` (`//go:build js`) — the browser sibling of the `!js` drain created in Task 3.3.
- Test: covered by `timing_traces.browser.test.js` (armed-mode warning) — extend it.

**Interfaces:**
- Produces: `func (g *Game) drainEmissionReports()` split by build tag: desktop = mixer ring; browser = JS `getTimingTraces` via `syscall/js`; also a `resetTimingTraces` call on stop.

- [ ] **Step 1: Extend the browser test to assert armed-mode warning**

Append to `timing_traces.browser.test.js`:

```js
// Arm timing fatal in log mode and induce lateness; assert a warning surfaces
// via the perf/parity log channel (browser never panics by default).
await t.evaluate(() => window.resetTimingTraces());
// (Induce lateness by heavy main-thread work if the harness supports it; else
// just assert the drain path runs without throwing and traces keep flowing.)
await t.evaluate(() => window.startPlay());
await t.wait(300);
const more = await t.evaluate(() => window.getTimingTraces().length);
await t.evaluate(() => window.stop());
if (more === 0) throw new Error('no traces after arm');
console.log('PASS timing_traces armed drain');
```

- [ ] **Step 2: Add the `js` drain sibling**

The desktop drain `parity_timing_drain_notjs.go` (`//go:build !js`) already exists from Task 3.3. Because it is `!js`-tagged, the `-tags test` build (which is still `!js`) uses it with the audio `test`-stub `DrainEmitReports` returning 0 — so no `js` conflict. Now add only the browser sibling:

`parity_timing_drain_js.go`:
```go
//go:build js

package ui

import "syscall/js"

func (g *Game) drainEmissionReports() {
	arr := js.Global().Call("getTimingTraces")
	if !arr.Truthy() {
		return
	}
	n := arr.Length()
	curGen := g.audioGen.Load()
	_ = curGen
	for i := 0; i < n; i++ {
		tr := arr.Index(i)
		row := tr.Get("row").Int()
		abs := tr.Get("abs").Int()
		startWhen := tr.Get("startWhen").Float()
		ctxAtStart := tr.Get("ctxAtStart").Float()
		// Browser drift = when it actually started on the audio clock minus
		// the clock value observed at scheduling. Positive = late.
		g.closeTimingTrace(row, abs, ctxAtStart+(ctxAtStart-startWhen))
	}
	js.Global().Call("resetTimingTraces")
}
```
> Note: the exact browser drift formula depends on what `startWhen`/`ctxAtStart` mean at the JS record site (Task 4.1). Reconcile so that `closeTimingTrace`'s `emittedWallSec - idealWhen` yields "how late did the sample actually start vs the scheduled grid time". If the JS side already computes lateness, pass it directly and change `closeTimingTrace` to accept a precomputed drift on the js path. Keep the semantics identical to desktop: positive = emitted later than ideal.

`openTimingTrace`/`closeTimingTrace` stay shared in `game_parity_timing.go` (no build tag); only the `drainEmissionReports` method is split across the `!js`/`js` files.

- [ ] **Step 3: Run browser + all builds**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/timing_traces.browser.test.js`
Expected: PASS.
Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: all succeed.

- [ ] **Step 4: Commit** (checkpoint only)

```bash
git add -A src/go/internal/ui src/js/timing_traces.browser.test.js
git commit -m "feat(timing): WASM pull of browser emission traces → Stage-D"
```

---

# FINAL VERIFICATION

- [ ] **V1: Full fast Go suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ -count=1`
Expected: PASS.

- [ ] **V2: Race suite on the timing + parity paths**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/ui/ -run 'Timing|Parity' -count=1`
Expected: PASS, no data races.

- [ ] **V3: Desktop audio package (real build)**

Run: `cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -run 'Timing|Emit|DeviceClock|ProcessBlock|Mixer' -count=1`
Expected: PASS.

- [ ] **V4: Cross-platform build matrix**

Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go build -tags test ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...`
Expected: all succeed.

- [ ] **V5: Browser bridge + timing**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js && GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/timing_traces.browser.test.js`
Expected: PASS.

- [ ] **V6: Live-edit no-false-positive sweep (the core requirement)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParityRuntimeMutationMatrix|TestParityTimingStressNoFalsePositive|TestDeleteRowDuringPlayback|TestParityDemo|TestParityImport' -v -count=3`
Expected: PASS on all repetitions — proves the timing gate + logical checker survive live circuit/rule edits during playback without false panics.

---

## Notes for the implementer

- **The single most important property** is V6: the gate must never fire during live edits. Every timing suppression rides the same `parityGen`/grace spine as the logical checker (`parityInGrace`, gen-stamped traces dropped in `closeTimingTrace`). If you find yourself adding a *second* suppression mechanism, stop — reuse the existing one.
- Tolerance numbers in Task 1.2 are **starting points**. If CI/harness jitter trips `TestParityTimingStressNoFalsePositive`, loosen the specific breached stage bound with a one-line rationale; never disable the gate to make a test pass.
- Keep `internal/audio` free of any `internal/ui` import (the ui→audio direction only). The emission ring lives in audio; the trace map lives in ui.
