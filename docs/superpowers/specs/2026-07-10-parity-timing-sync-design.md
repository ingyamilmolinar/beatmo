# Parity Timing-Sync Enforcement + Logical-Checker Hardening — Design

**Date:** 2026-07-10
**Status:** Approved (design), pending implementation plan
**Author:** brainstorming session

---

## 1. Problem

Beatmo already has a **logical** parity watchdog (`parityCheck` / `parityScan` / `recordParityAudio`) that
compares predictor-truth ↔ DrumView slate ↔ recorded audio ↔ recorded highlight ↔ immutable timeline and
**panics** on a logical mismatch, with a generation/grace suppression spine (`parityGen` +
`parityGraceUntilNS`) so live circuit edits do not false-trigger.

Separately, `schedule_metrics.go` measures a three-stage timing chain (Stage A real-clock→scheduler-fire-late,
Stage B scheduler→dispatch-bridge, Stage C dispatch→lead/overdue) plus a browser `renderLatencyMetrics`. These
are **pure observability counters** surfaced to the HUD / JS — nothing enforces them.

**Gap:** the requirement is that *real clock → scheduling → triggering → dispatching → actually producing audio*
stay in sync with extremely low tolerances, and that **any logical OR timing mismatch triggers a panic with
detailed debugging information**, while **never** false-triggering during live circuit/rule edits. Today:

- Logical mismatches panic. ✓ (but with concrete test-coverage gaps)
- Timing drift is measured but **never enforced**. ✗
- The final hop — *actually producing audio samples* — is not observed at all on desktop (`audio.Now()` is a Go
  wall-clock, not the Oto device clock).

## 2. Goals / Non-goals

**Goals**
1. Enforce timing sync across the whole chain down to **sample emission**, with a statistical (not single-sample)
   gate that is strict on both average and tail, and panics with a detailed dump.
2. Reuse the existing `parityGen`/grace spine so live edits (node add/delete, rule/group/path change, insert-FX,
   BPM/length change) never false-trigger the timing checker.
3. Activation policy: **strict, test + desktop-dev by default**; **browser/production log-only**, armable via env.
4. Harden the existing logical checker: close the highest-value test-coverage gaps, including asserting the panic
   *payload* contents.

**Non-goals**
- No rewrite of the logical checker's comparison logic — only additive tests + payload assertions.
- No change to audio DSP behavior. Stage-D instrumentation is additive and lock-free; it must not alter samples.
- Not attempting to correct/compensate drift — this is detection + panic only.

## 3. Architecture — Approach A (extend the existing parity machinery)

A new **4th parity surface** (`parityTiming`) lives alongside `parityCheck`/`parityScan`, embedded in the same
`parityTracker`, and **shares the `parityGen` / `parityGraceUntilNS` grace spine and the `emitParityFatal` dump
path**. This makes live-edit suppression free and keeps one unified crash report.

### 3.1 The timing trace (per-event record)

Each scheduled beat gets one bounded-ring record capturing wall-clock-comparable timestamps for its four hops:

```
timingTrace {
  row, abs            // identity
  parityGen, gen      // SAME suppression keys the audio checks use (stale-gen filter)
  idealWhen           // scheduled ideal audio-time (beatIdealAudio)
  seqFiredAt          // Stage A: seqScheduleTime emitted it
  enqAt               // Stage B start: queued to audioCh (already on soundReq)
  dispatchedAt        // Stage B end / Stage C: audioLoop → audio.PlayBatch, with audioNow snapshot
  audioNowAtDispatch  // Stage C: audio.Now() at dispatch (for lead/overdue)
  emittedDevPos       // Stage D: device sample-position where the voice actually began (NEW)
  emittedAt           // Stage D: realized wall-time of emission (via clock-skew anchor)
  closed              // Stage D received?
}
```

Lifecycle: **opened** at schedule (`scheduleSound`/`queueSoundAtParams`), **stamped** at dispatch (`audioLoop`),
**closed** when the mixer/WebAudio reports emission. Stored in a bounded ring (~256) on `parityTracker`, keyed by
`(row, abs, gen)` so out-of-order closure finds its open trace.

### 3.2 Stage D — desktop (mixer sample emission), lock-free in production

Chosen: **lock-free in production** (no build tag). The mixer already knows everything:

- **Device-clock anchor**: an atomic `(devPos int64, wallAtUnixNano int64)` updated once per `Read()` — samples
  emitted so far paired with `time.Now()`. Any thread can convert device-sample-pos ↔ wall-time. Written with a
  seqlock-style pair (two atomics + version) so the reader never sees a torn (pos, wall) pair.
- **Voice→beat identity**: thread the `(row, abs)` already known at `scheduleSound` down through `PlayParamsAt`
  into `voiceState` as additive fields. No behavior change.
- **Emission report**: when `processBlock` first renders a voice whose `start` sample-pos is reached (voice truly
  producing audio), publish `(row, abs, gen, startPos)` via a **lock-free single-producer ring / atomic slot**
  drained by the audio loop — **no mutex, no alloc inside `processBlock`** (honors the mixer's hot-path
  constraints in `recording_pipeline.go:203`). The audio loop closes the matching trace.
- **Realized emission wall-time**: `emittedAt = anchorWall + (startPos − anchorPos)/sampleRate`.
  `Stage-D drift = emittedAt − idealWhen` — the wall-vs-device clock skew, i.e. the desktop "are we actually in
  sync" signal that is invisible today.

### 3.3 Stage D — browser (WebAudio emission)

Browser runs on the true `AudioContext` clock and already has `renderLatencyMetrics`.

- At each real `BufferSource.start(when)`, record `(row, abs, actualStartWhen, ctx.currentTime)` into a JS-side
  timing ring keyed by `(row, abs)`.
- Expose `getTimingTraces()` / `resetTimingTraces()` (mirroring `getAudioScheduleMetrics`). The Go-WASM checker
  pulls realized emission times across the bridge and closes the same trace records.
- Browser is **log-only by default** → warnings + JSONL event stream, not a crash (unless armed).

### 3.4 The checker + statistical firing model

`parityTiming()` runs on the UI thread right after `parityScan` (same cadence, same `refreshDrumRow` site). For
each **closed** trace in the playhead window and **not filtered by stale `parityGen`/`gen`** (identical filter to
the audio checks — this is what makes live edits safe), it derives four stage deltas:

- Stage A: `seqFiredAt − idealWhen`
- Stage B: `dispatchedAt − enqAt`
- Stage C: overdue = `audioNowAtDispatch − when` (positive ⇒ overdue)
- Stage D: `emittedAt − idealWhen`

Each stage feeds a **rolling per-stage reservoir** (reuse the existing `schedule_metrics` reservoir + percentile +
stddev machinery). The gate fires when **any** of these breaches — strict on center *and* tail, immune to a lone
blip:

| Statistic | Catches | Desktop starting bound |
|---|---|---|
| Mean / P50 | systematic drift (whole stream slightly late) | e.g. Stage-D mean > 3 ms |
| P99 (tail, over window) | fat tail — worst ~1% bad *repeatedly*, not once | e.g. > 10 ms |
| StdDev (jitter) | instability / unsteady clock even if mean is fine | e.g. > 4 ms |
| Absolute max (single) | true dropout — one reading so large it is an audible glitch | e.g. > 40 ms ⇒ panic immediately |

Rationale: a single 1-in-2048 OS/GC spike barely moves a 128-sample-window P99, so no flake; but a systematically
late stream (mean), a fat tail (P99), or an unsteady clock (stddev) all fire; and a genuine dropout-class single
reading hard-panics because *that reading is the fault*, not scheduling noise.

**Universal suppression** (all reuse existing gates): warmup (0.5 s, `schedWarmupSec`), grace window
(`parityInGrace`), stale `parityGen`/`gen`, stopped/paused/importing, `renderReady`, per-row mute/solo gating.

**Suppression must be applied at the OBSERVATION sites, not only the check** (the C1 fix): Stage D is
gen-filtered when a trace closes, but Stages A/B/C are recorded on the sequencer/audio threads and their
`absMax` statistic is a session-monotonic running max. Grace suppressing only the *check* let an edit-induced
catch-up spike latch into the monotonic max and fatal-panic after grace expired — the exact live-edit false
panic the feature forbids. Fix: (1) the `ObserveSeqFireLate`/`ObserveBridge`/`Observe` call sites skip recording
while `parityInGrace()`; (2) `bumpParityGen` (every structural edit) and the BPM re-anchor path call
`schedMetrics.ResetPeaks()`, which clears only the four peak-max fields (counts/sums/reservoirs persist, so
mean/P99/stddev keep firing on genuine sustained drift). This makes the grace/gen spine genuinely universal.

All bounds are `RuntimeProfile` fields (desktop tight, browser looser) with env overrides, consistent with the
codebase's runtime-profile discipline. Each surviving divergent bound carries a one-line bench citation.

### 3.5 Detailed panic payload

Extend `emitParityFatal` with a **timing section**: offending stage + measured-vs-tolerance for each breached
statistic, the full 4-hop timestamp breakdown for the worst beat, the clock-skew anchor (device vs wall), the last
N timing traces from the ring, and the current three-stage `ScheduleMetricsSnapshot` (P50/P90/P99/jitter/max). It
reuses the existing transport/scheduler-indices/predictor/timeline/graph dump. One unified crash report for both
logical and timing faults.

### 3.6 Activation policy

- New `parityTimingFatalEnabled atomic.Bool` — **on for the production desktop binary** (real wall clock), **off on WASM/production** (log-only), and **dormant (off) by default under `go test`**. Dedicated timing tests opt in via `enableParityTimingForTest(t, g)`, which saves/restores the prior global state.
- `PARITY_TIMING=off|log|panic` and `PARITY_TIMING_FATAL=1` to arm browser — via the pure `resolveParityTimingActivation` helper (mirrors `PARITY_WASM_FATAL`: browser forced log-only unless armed).
- **Dormant-under-test is required, not optional:** the general Go suite fast-forwards time via backdated `PlayStart`, synthesizing large Stage-A "lateness" that would false-panic an active gate. The wall-clock timing gate is meaningless under synthetic time, so it is off by default under `go test`; only the dedicated timing tests (which inject metrics deterministically) arm it. This still satisfies "test/desktop-only enforcement" — the dedicated timing tests plus the real desktop binary.
- A **settle guard** (`time.Since(PlayStart) < schedWarmupSec`) suppresses the gate during the first ~0.5s of playback, because schedMetrics warmup is applied to Stage C only — Stages A/B/D are not warmup-filtered.
- Because it reuses the same grace spine (`parityInGrace`), arming can never fire during a live edit.

## 4. Track 1 — logical-checker hardening (fill gaps)

Highest-value gaps from the coverage sweep:

1. **Real-time grace path has zero coverage** (grace is bypassed under `go test`: `parityInGrace` returns false).
   Add a test hook to *enable* `parityInGrace` under test, drive a real background sequencer + concurrent
   mutation, assert grace demotes to log (no panic) then a genuine mismatch re-fires after grace expiry.
2. **True-positive tests for Kinds only tested negatively:** `audio_vs_seq`, `timeline_vs_pred_immutable`, and
   `view_vs_truth` for **mute/invisible** node types.
3. **Assert the panic payload contents** (not merely that a panic occurs) — regression guard covering both the
   logical dump and the new timing dump.
4. **`SetLength` / subdivision change mid-play** added to the runtime-mutation matrix
   (`parity_runtime_mutation_matrix_test.go`).
5. **Stride phase-rotation** true-positive: a mismatch at a strided abs must still eventually fire (guards against
   a stride bug that skips a mismatching abs indefinitely).

## 5. Testing strategy (timing layer)

- **True-positive:** inject a trace with an absolute-max breach → assert panic + payload. Inject a window whose
  mean/P99/stddev breach → assert panic. Inject a window that stays under all bounds → assert no panic.
- **True-negative (critical):** breaches *inside grace* → no panic; breaches at *stale parityGen* (mid-edit) →
  filtered; breaches during *warmup/stopped/paused* → suppressed; a legitimate BPM re-anchor → no panic.
- **Desktop Stage-D unit:** feed the mixer a scheduled voice, advance `Read()`, assert reported emission
  sample-pos + derived drift match expectation; simulate a device-clock stall → assert drift detected.
- **Browser:** a `.browser.test.js` asserting `getTimingTraces()` reports realized `.start()` times and that
  armed mode surfaces a warning on induced lateness.
- **Stress, not idle:** the negative/positive suites push high BPM, many rows, rapid live edits, and channel
  backpressure while asserting the statistics hold — the checker is proven under load, not on a quiet loop.

## 6. File change map (additive-first)

**New**
- `game_parity_timing.go` — the checker + statistical gate.
- `game_parity_timing_types.go` — `timingTrace`, its bounded ring, per-stage reservoirs.
- `parity_timing_*_test.go` — positive/negative/grace/stress suites; desktop Stage-D unit.
- one `*.browser.test.js` — WebAudio Stage-D + armed-mode warning.

**Touched (additive)**
- `game_parity_tracker.go` — timing ring + per-stage reservoirs on `parityTracker`.
- `game_audio_schedule.go` / `game_audio_loop.go` — open trace at schedule, stamp Stage-B/C hops, drain Stage-D
  reports and close traces.
- `internal/audio/engine_mixer.go` / `engine_stop.go` / `engine_play.go` — device-clock anchor, lock-free
  emission report, `(row, abs)` thread-through into `voiceState`.
- `schedule_metrics.go` — expose snapshot into the payload; reuse reservoir for per-stage stats.
- `runtime_profile.go` — timing tolerance fields (desktop/browser) with bench citations.
- `src/js/audio.js` — timing ring + `getTimingTraces`/`resetTimingTraces`.
- `game_parity_state.go` — `emitParityFatal` timing section; `PARITY_TIMING*` env parsing + activation defaults.
- `parity_runtime_mutation_matrix_test.go` — add `SetLength`/subdiv mid-play case.

## 7. Risks

- **Hot-path intrusion (desktop mixer).** Mitigated: additive fields + lock-free single-slot/ring report, no
  mutex/alloc in `processBlock`; a discipline test asserts no new alloc in the Stage-D report path.
- **Cross-thread trace closure races.** The trace ring is written from the audio loop (drain side) and read from
  the UI checker; guarded by the existing `parityMu` for the buffer plus atomics for the anchor. Race-detector
  test (`make test-race`) covers it.
- **Flaky panics under CI load.** The statistical gate (window + P99/stddev, not single-sample except
  dropout-class) plus warmup is specifically designed against this; stress tests validate stability. Bounds are
  runtime-profile + env-tunable so CI can loosen without code changes.
- **Browser bridge cost of pulling traces.** Traces are pulled at the parity-scan cadence (already throttled),
  batched, and browser default is log-only.

---

## 8. Open items to confirm during planning

- Exact starting numeric bounds per stage/platform (table above are starting points; tune against
  `bench-results/runtime_profile_sweep.md`).
- Whether Stage-D browser closure should reuse `renderLatencyMetrics` internals or a parallel ring.
