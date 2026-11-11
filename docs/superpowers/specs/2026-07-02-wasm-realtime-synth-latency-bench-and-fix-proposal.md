# WASM real-time synth pipeline: measured bottlenecks + fix proposal

Status: **IMPLEMENTED** (P0–P4 all landed in audio.js + synth_render_worker.js,
same session, uncommitted). `RENDER_BENCH_STRICT=1 make bench-render` is green:
every storm scenario now plays **deferAbsMsP90 = 0.0ms** (on grid), including
the 4× CPU-throttled mobile simulation (absP90 8ms, all notes stale-on-time)
and the dense-cadence scenario under full worker-queue saturation (qPeak 7,
renders backed up to 411ms — notes still on grid via stale buffers).
Post-fix measured table at the bottom (§6). Design deltas vs the original
proposal, discovered during implementation:
- P0+P1 unified into a **fresh-by-deadline race**: a stale-cache note waits for
  the fresh render up to (intendedWhen − 12ms margin) and plays FRESH on grid
  if it lands, else STALE on grid. Best of both — a single edit followed by a
  sequenced note usually sounds fresh AND on time.
- **One race per note**: the race's replay always carries `allowStale:true`
  ("play the best available buffer, don't re-race") — otherwise continuous
  60Hz edits re-stale the record mid-race and the note drifts later each cycle.
- **Un-scheduled plays (preview taps, no caller `when`) are not grid-locked**:
  they wait a freshness budget (80ms) instead, so a knob edit is heard on the
  very next tap rather than one tap late.
- P2 needed no timer: generation-capture at render start + in-flight dedup
  (pendingRenderEnsures) already coalesces to ≤1 render in flight per
  (id,pitch); renders/note stayed 1.0 during storms and dropped off the
  critical path entirely.
Follow-up to Part 3a / Phase 4 of
`2026-07-02-kick-metrics-and-synth-pipeline-proposal.md`. The off-thread render
worker shipped (commit `b7664860a`, `synth_render_worker.js`) and
`webaudio_synth_param_stress.browser.test.js` passes — yet live synth edits
during playback are still audibly laggy in WASM. This doc explains why with
numbers, and ranks the fixes.

---

## 1. The flow (scheduler trigger → sound), WASM

```
sequencer goroutine (Go)     game_sequencer_schedule.go
  beat decision → when = beatIdealAudio + lookahead(0.06s)
      ↓ audioCh (non-blocking, drop-oldest)
audioLoop goroutine (Go)     game_audio_loop.go
  batch ≤32, when clamped ≥ now+MinDispatchLeadSec(12ms) → audio.PlayBatch
      ↓ 1 Go→JS crossing         batch_wasm.go → playSoundsBatchFlat
JS main thread               audio.js
  enqueueAudioEvents → microtask flush (1.2ms budget) → processAudioEvent
      ├─ renderCache HIT  → createBufferSource + antipop gain → src.start(when)   [sub-ms, ON TIME]
      └─ renderCache MISS → **drop `when`**, await async render, re-enqueue
             ↓ postMessage                      ↓ replay at now+4ms
         synth_render_worker.js (ONE worker, serialized ccall render_modular_p,
         ~25–40ms per 2s voice on desktop CPU; ~4× that throttled)
  main-thread residue per render: normalize + amp scale + optional sample-edit
  + copyToChannel (O(frames), ~0.5–1.5ms)
      ↓
WebAudio: source → per-voice antipop gain → instrument bus → insert-FX
AudioWorklet (audio thread) → EQ → sends → master
```

A live param edit (`setInstrumentParam` → Go `SetInstrumentParam` →
`updateInstrumentParams` → `evictRenderCache(id)` audio.js:2329) deletes the
bare render AND **every per-pitch variant**, so during a knob drag every
subsequent trigger takes the MISS path.

**Two structural defects on the MISS path:**

1. **The intended `when` is discarded** (audio.js `processAudioEvent`): the
   deferred note replays at `now + AUDIO_MIN_LEAD_SEC(4ms)` whenever the render
   happens to finish. On a fast machine the render (25–40ms) undercuts the 60ms
   lead → the note plays **early**; on a slow/throttled machine it plays
   **late**. Either way it is off the musical grid by `|lead − renderTime|` —
   the error never converges to 0, even for fast renders.
2. **Eviction leaves nothing playable**: the previous (stale-timbre) buffer is
   deleted rather than kept as a fallback, so timing is sacrificed for timbre
   freshness on *every* note of a drag.

**Why every existing metric was blind to it:** `observeScheduleTiming` (and
therefore `getAudioScheduleMetrics` overdue/smallLead/lag, and the perf-e2e
gates) observes the *rewritten* `when`, which always has a healthy fresh lead.
`webaudio_synth_param_stress` gates on `mainThreadRenders == 0` (off-thread-ness),
not on-time-ness. On WASM the Go-side `render_latency.go` counters never fire
(the render happens in JS; `synth_recipe_dispatch.go` is `!js` desktop-only).

## 2. New instrumentation + bench (implemented)

- **`renderLatencyMetrics` in audio.js** (`getRenderLatencyMetrics()` /
  `resetRenderLatencyMetrics()` window exports), companion to
  `scheduleMetrics`:
  - `deferMs avg/min/max` (signed) + `deferAbsMsP90` — actual source start
    minus the *originally intended* `when`, carried across defers via a
    metrics-only `origWhen` event field. This is the number the listener hears.
  - `renderMs avg/p90/max` — per-voice render cost as experienced by the
    trigger path (worker round-trip incl. queue wait + main-thread post).
  - `workerQueuePeak` — pileup on the single render worker.
- **`webaudio_render_latency_bench.browser.test.js`** (`make bench-render`,
  writes `bench-results/render_latency.json`): raw-WAV baseline
  (`registerSamplePCM`) vs drum-storm vs warm melodic synth vs melodic
  param-storm (80ms + 40ms dense cadences) vs sampler-edit storm vs 4×
  CPU-throttled storm (mobile-class device via CDP). Structural gates (raw-WAV
  renders==0, warm defers==0); `RENDER_BENCH_STRICT=1` gates storm
  `deferAbsMsP90 ≤ 15ms` — the post-fix acceptance bar.

## 3. Measured results (2026-07-02, dev machine, Chromium headless)

| scenario | notes | renders | deferred | deferMs avg/min/max (absP90) | renderMs avg/p90/max | qPeak | maxGap |
|---|---|---|---|---|---|---|---|
| raw-wav | 31 | 0 | 0 | on grid (0) | – | 0 | 8.6ms |
| drum-storm (snare) | 31 | 31 | **31** | −41 / −45 / −13 (**45.3**) | 14 / 16 / 39 | 0 | 49ms |
| synth-warm (cello) | 31 | 0 | 0 | on grid (0) | – | 0 | 12.9ms |
| synth-storm (cello) | 31 | 31 | **31** | −29 / −35 / −13 (**34.7**) | 24 / 25 / 40 | 0 | 9.5ms |
| synth-storm-dense (40ms) | 62 | 62 | **62** | −25 / −35 / −13 (**34.7**) | 29 / 33 / 35 | 1 | 9.3ms |
| sampler-storm (cello) | 31 | 31 | **31** | −28 / −35 / −13 (**34.7**) | 25 / 32 / 40 | 0 | 11.8ms |
| synth-storm-4x (throttled) | 30 | 30 | **30** | −16 / −24 / **+104** (24.0) | 36 / 38 / **158** | 0 | **154ms** |

Reading:

- **Raw WAV and warm synth are perfect** — the trigger path itself is real-time
  (sub-ms schedule at intended `when`). The entire problem is the cache-miss
  defer path under param edits.
- **During a param storm, 100% of notes are off-grid.** On a fast machine they
  play 13–56ms EARLY (deviation ≈ lead − renderTime, so the *cheap* snare
  render deviates MOST, 45ms); under 4× CPU throttle they play up to **104ms
  LATE** with 158ms worst render — the user-reported "laggy / not real time".
  Perceptible rhythmic jitter starts at ~5–10ms.
- The single worker keeps up at realistic cadences (qPeak ≤ 1) — **queue
  pileup is not the bottleneck; the per-note defer/replay policy is.**
- Existing `getAudioScheduleMetrics` reported `overdue=0, smallLead=0` in the
  same runs — confirming the blindness.

## 4. Proposed fixes, ranked by measured leverage

### P0 — Play stale, refresh async ("stale-while-revalidate" voice cache)
`evictRenderCache(id)` → `markRenderCacheStale(id)` (generation counter, keep
buffers). MISS-with-stale-entry: **play the stale buffer at the intended
`when`** (on time, one-edit-old timbre) and kick a worker re-render that swaps
the record for subsequent notes. Timing is never sacrificed; timbre lags at
most one note (~80ms at seq cadence — inaudible during a drag, and the final
settle re-renders anyway).
Expected: `deferAbsMsP90 → 0` for every storm scenario (the STRICT gate);
render/note ratio unchanged (renders still happen, just off the critical path).
Applies identically to the sampler-edit path (same evict site) and should be
mirrored on desktop (`voiceCacheInvalidate` in Go) where the same policy exists
with a 20ms lookahead.

### P1 — Never drop the intended `when` on defer
For the residual true-miss case (first play of a pitch, no stale buffer):
re-enqueue with `when = max(origWhen, now+minLead)` instead of unconditionally
`now+minLead`. On the dev machine this alone puts fast-render notes back ON
GRID (render 25ms < 60ms lead) — the measured −13…−56ms early-jitter is 100%
this bug. The plumbing (origWhen passthrough) already exists from the metrics
work. Cheap, zero risk: a note can only start later than it would today when
today's start would have been *early*.

### P2 — Debounce the storm's re-renders (trailing settle)
With P0, a 60Hz drag still queues a worker render per trigger per pitch
(ratio 1.0). Debounce the *background refresh* per (id, pitch) with a ~50ms
trailing timer so a drag coalesces to ~1 render per settle instead of 12/s.
Keeps the worker cool on mobile (renderMs 36→158ms under throttle is
CPU-contention: fewer renders = less contention with the UI/audio threads).

### P3 — Base-pitch render + playbackRate fallback for first plays
When even a stale buffer is absent for `id:<pitch>` but ANY pitch of `id` is
cached, play the nearest cached pitch resampled via `playbackRate`
(2^(Δsemis/12)) at the intended `when`, then background-render the exact
pitch. Removes the last defer source (first-play). This is the "base-pitch
render + resample" item from the original proposal, scoped down to a fallback
(fidelity restored one note later) instead of replacing per-pitch rendering.

### P4 — Move post-processing into the worker
Normalize + amp scale + sample-edit currently run on the main thread per render
(O(frames), ~0.5–1.5ms each, inside the measured renderMs). Ship them in the
worker request so the main thread only receives a play-ready buffer.
Small, but it is pure win and shrinks the main-thread work under storms.

### Explicitly NOT proposed (measured as non-bottlenecks)
- Worker pool >1: qPeak ≤1 at realistic cadences; a second DSP module instance
  costs ~+8MB WASM heap. Revisit only if chords of >3 melodic rows measure
  pileup.
- Stage-boundary incremental render: highest-fidelity payoff but a real C
  refactor; P0–P3 reach the real-time bar without it. Keep as follow-up.
- Param smoothing: an edit mid-note doesn't click today because voices are
  per-trigger buffers with antipop ramps; irrelevant to the latency problem.

## 5. Acceptance
`RENDER_BENCH_STRICT=1 make bench-render` green (storm `deferAbsMsP90 ≤ 15ms`,
`mainThreadRenders == 0`), including the 4× throttle scenario staying under
~30ms absP90; `webaudio_synth_param_stress` and
`synth_revoice_render_resilience` stay green; `xplat_audio_compare` unchanged
(none of P0–P4 alters rendered samples, only *when* buffers swap).

## 6. Post-fix measured results (2026-07-02, same machine/protocol as §3)

| scenario | renders | deferred | stale | deferMs avg/min/max (absP90) | renderMs avg/p90/max | qPeak |
|---|---|---|---|---|---|---|
| raw-wav | 0 | 0 | 0 | on grid (0) | – | 0 |
| drum-storm | 31 | 31 | 29 | 0.0 / 0.0 / 0.0 (**0.0**) | 11 / 13 / 15 | 0 |
| synth-warm | 0 | 0 | 0 | on grid (0) | – | 0 |
| synth-storm | 31 | 31 | 31 | 0.0 / 0.0 / 0.0 (**0.0**) | 34 / 42 / 45 | 0 |
| synth-storm-dense | 54 | 62 | 62 | 1.2 / 0.0 / 18.7 (**0.0**) | 150 / 331 / 412 | 7 |
| sampler-storm | 31 | 31 | 31 | 0.0 / 0.0 / 0.0 (**0.0**) | 44 / 85 / 115 | 1 |
| synth-storm-4x | 31 | 31 | 31 | 4.0 / 0.0 / 40.0 (**8.0**) | 67 / 83 / 116 | 1 |

vs pre-fix absP90 34.7–45.3ms (fast machine, early) and +104ms late (throttled):
every scenario is on the musical grid; under saturation (dense: renders queue to
412ms) and 4× throttle the stale-buffer path holds the grid while timbre lags
one render behind the knob. Verification beyond the bench (all green):
webaudio_synth_param_stress, synth_revoice_render_resilience,
instrument_params_playback_stability, instrument_params_audible,
synth_sample_param_change_keeps_audio, synth_edits_override_stale_sample,
sample_edit_descriptor, webaudio_drums_consistency, webaudio_mixer_eq_parity
(solo; flakes under parallel-suite CPU load), webaudio_smoke,
webaudio_perf_e2e(startup), webaudio_bench_startup (after fixing its readiness
gate: js_bootstrap_wasm.go registers a STUB startPlay early, so the test's
`typeof startPlay` gate raced the real export batch and bare `forceDraw?.()`
threw — now gates on startPlay && forceDraw), xplat_parity.

Failures adjudicated PRE-EXISTING by running the identical suites against a
baseline tree (my audio.js/synth_render_worker.js edits reverted, external
branch churn kept — byte-for-byte identical failures):
- `xplat_audio_compare`: 20 "param kick-*" WASM-silence cases + modular
  0.027@23949 diff (documented in [[project_synth_param_rerender_storm]]
  before this work; the test renders in its own standalone DSP module).
- `webaudio_three_stage_latency`: Stage A seqFire p99 2.3–3.3 s at BPM 240–320
  on baseline too — a Go-side sequencer stall in the branch, worth its own
  investigation.
- `webaudio_fx_chain_stress`: (a) organ-church "modulators 2/3" — the branch's
  organChurchSeed ships `lfo_enabled:1`, so the test's forced lfo_enabled=1 is
  elided from export as a no-op delta (test assumption "pitchenv/lfo/burst
  start at 0" is stale vs the new seeds); (b) Stage A p99 149–182ms vs 50ms —
  same Go-side lateness family as three_stage.

Remaining follow-up (not done here): mirror stale-while-revalidate on desktop
(`voiceCacheInvalidate` in Go renders synchronously on the audioLoop goroutine
with a 20ms lookahead — same defect class, unmeasured); stage-boundary
incremental render for timbre-lag-free storms (C refactor, optional).
