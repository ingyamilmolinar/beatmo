# Import warm-up + playback readiness gate

**Date:** 2026-09-13
**Status:** Design approved, not yet implemented
**Branch:** `add-core-node-types`

## Problem

Loading a circuit with instruments the engine has not yet rendered leaves the
voice cache cold. Playback is available immediately, so the first ~30 seconds
of a freshly imported circuit are rendered *while playing*: the UI stutters,
notes land off the grid, and notes play at the wrong pitch. Stopping and
pressing play again "fixes" it, because by then the renders have happened.

## Measured baseline (2026-09-13)

### Working set

Computed by walking each row's reachable nodes across all 24 built-in
templates. The unit is a distinct `(instrument, rounded-pitch)` pair.

| Template | rows | nodes | distinct pairs | ×3 with ±1 neighborhood |
|---|---|---|---|---|
| beach-gaelic | 4 | 293 | **91** | 273 |
| liliuokalani-aloha-oe | 6 | 372 | 87 | 261 |
| mozart-quartet-k387 | 5 | 149 | 68 | 204 |
| beethoven-ode-to-joy | 3 | 117 | 16 | 48 |

Range across all 24: 16–91 exact pairs.

### Desktop native render cost (44.1 kHz, direct probe)

| Instrument | cold render | cache hit |
|---|---|---|
| violin-ensemble | **23.3 ms** | 0.36 ms |
| bassoon | 17.6 ms | 0.40 ms |
| oboe | 13.5 ms | 0.37 ms |
| cello / viola / violin | 10.8–11.8 ms | 0.37 ms |
| piano-grand / guitar-nylon / celesta | 1.8–2.9 ms | 0.35 ms |
| kick / snare / hihat | 4.3–7.4 ms | **0.008 ms** |

A 96-pair warm set costs **1.42 s of serial CGo** (14.8 ms/pair). At 180 BPM /
subdiv 16 one step is **20.8 ms** — a single cold `violin-ensemble` render
exceeds a whole step.

### Desktop, `make bench` with a heavy circuit loaded (beach-gaelic, 180 BPM, 15 s)

```
RenderLatency: cacheHits=221 cacheMisses=175 dispatchRenders=0
               prewarmRenders=175 renderAvg=16.55ms renderMax=38.83ms
```

**175 cold renders occur during the first 15 seconds of playback** (~2.9 s of
CGo work). The bench host has ~866 fps of headroom so `overdue=0` masks it; a
60 fps machine does not.

### Browser, import-then-play (real WASM + WebAudio, same circuit, 27 s)

```
renderMsAvg=270ms  renderMsMax=532ms  workerQueuePeak=12
neighborPlays=66   deferredPlays=4    deferMsAvg=288ms
updateAvg 1.4ms → 3.9ms,  drawAvg 0ms → 8.0ms   (still climbing at t=27s)
```

**66 notes played at the wrong pitch** (nearest-cached-pitch resample); 4 notes
landed **288 ms off the grid**; the circuit was still warming after 27 seconds.

### Browser, full eager warm of the same 91-pair set

```
72 renders in 7.79 s  (108 ms/render throughput, ONE worker)
main-thread event-loop gap: p90 586 ms, max 716 ms
```

## Root causes

1. ~~**`warmFromBeatInfos` enumerates the predictor *window*, not the
   circuit.**~~ **RETRACTED 2026-09-13 during implementation — this was wrong.**
   `beatInfosByRow[i]` is `graph.CalculateBeatRowFrom(origin)` truncated by
   `rawBeatLen` (`game_graph_update_beat_infos.go:94-97`) — the COMPLETE per-row
   graph path including the loop — and it is the predictor's *input* via
   `SetPaths` (`:514`), not its windowed output. The existing walk already covers
   the whole circuit. `Predictor.Ensure(horizon)` grows a different structure
   (prediction buffers) and is irrelevant here. Causes 2 and 3 below are the real
   ones; no horizon extension is needed.
2. **The prewarm pool drops work silently.** `async.Pool.Submit` is
   non-blocking by contract; 273 submits into a 64-deep queue means most of the
   set is never warmed.
3. **The browser has no Go-side warming at all.** `voice_prewarm_js.go` stubs
   `PrewarmVoice` / `PrewarmVoiceParams` to no-ops, `IsMelodicInstrument` to
   `false`, and `VoiceCached` to a hardcoded `false` — so the prewarmer cannot
   even tell what is already cached.
4. **`audio.WarmInstrument`, the one real cross-platform warm seam, is called
   from exactly one site** (`drumview_transport.go:398`, `SetInstrument`) and
   **never from `Import`**.
5. ~~**One render worker in the browser** serializes the warm set at ~108 ms/pair.~~
   **CORRECTED 2026-09-13 during implementation — misattributed.** `audio.js` at
   HEAD already had a render worker POOL (`SYNTH_WORKER_POOL_SIZE =
   max(1, min(3, hardwareConcurrency-1))`, least-loaded dispatch, added
   2026-07-09). The measured ~108 ms/pair throughput was real, but its cause was
   not a single worker. The verified cause is a **main-thread microtask burst**:
   `warmInstrumentBatch`'s dispatch loop fires every item without yielding, and
   because they all await the same already-settled module promise, their
   continuations (param-block build + `postMessage`) drain back-to-back in one
   microtask turn. Pool size is nearly irrelevant to it — measured max
   event-loop gap was 465 ms at pool=1 vs 448 ms at pool=4. Fixing it requires
   chunked/yielding dispatch, not more workers.
6. **Nothing gates playback.** Play is live the instant `Import` returns.
7. **Browser channel/insert-FX graph construction is deferred** into
   `pendingChannelOps` when no `AudioContext` exists at import time, so it all
   fires as a burst at first play.

The **cache identity is already correct**:
`voiceCacheKey{instrumentID, bpm, sampleRate, paramsHash, pitch}` is exactly the
"same instrument + same params + same note ⇒ hit" contract. The warm-up only
has to *ask* before it renders.

## Design

### 1. The warm set

```go
type WarmItem struct {
    InstrumentID string
    Pitch        float64 // rounded to 0.5 st
    Params       RecipeParams
}
type WarmSet struct {
    Items      []WarmItem
    BPM        int
    SampleRate int
}
```

Enumeration **reuses the existing walk**: `warmFromBeatInfos` already iterates
`g.beatInfosByRow` and calls `evalNodeParamsOnly(row, idx, info)`, which is the
only correct pitch computation (node pitch + `model.GroupEffect` windowed group
transpose + the ±24 clamp). The change is the **horizon**: ensure the predictor
covers one full loop per row before enumerating. No new pitch derivation, so no
drift risk against dispatch.

Bounded by `maxWarmSetItems` (512) with a stable priority order — rows in
order, pitches in first-appearance order. Overflow warms in the background
after the gate opens; hitting the cap is logged and counted.

**The ±1 pitch neighborhood is excluded from the gating set.** It exists so a
one-tick pitch *edit* lands warm; it is not needed for playback correctness and
including it triples the wait (91 → 273). It keeps warming in the background
after the gate opens, exactly as today.

### 2. The readiness gate

`Game.warmupState ∈ {Ready, Warming}` with `WarmupProgress() (done, total int)`.

**Trips on** (wholesale-invalidation events): project/template/file-picker/JS
`importJSON` import; BPM change; sample-rate change.

**Does not trip on**: single instrument switch, one synth-param Save, one node
pitch edit. Those keep the existing non-blocking background warm plus the
`serveBestAvailable` fallback.

**Blocks**: the user-facing transport entry points only — the Play button
handler, the `startPlay` / `togglePlay` JS exports, and the spacebar binding.
`Game.SetPlaying` stays unguarded so tests, scenes, parity harnesses and the
bench keep working; the bench instead *waits* for `Ready` and reports the wait
as a measured number.

**Failure and timeout policy (the gate must never deadlock).** The gate opens
on any of: warm set complete; a per-set deadline elapsed
(`warmupDeadline`, 15 s desktop / 30 s browser); or the warm backend reporting
it cannot make progress (no render worker, WASM module unavailable). In the
non-complete cases the gate opens anyway, pushes `notifyErrorKey` (surfacing
the existing error toast + a durable Logs line), and playback proceeds with
today's `serveBestAvailable` / neighbor-substitution behaviour — degraded, not
blocked. A user Stop/abort during warm-up also opens the gate and cancels the
remaining work; the cancelled items stay queued as background warms. Pinned by
a test that stubs the backend to never complete and asserts Play becomes
enabled within the deadline.

### 3. Progress UI — generalize `logsToast`

There is no in-progress notification mechanism today. Two surfaces exist and
both are terminal-event only:

| Surface | File | Shape |
|---|---|---|
| `notificationStore` | `notification_store.go` | Bounded ring (100) of **completed** events as i18n *key + args*, persisted via `userprefs.NotificationHistoryStore`, shown in the Logs panel + unread badge. |
| `logsToast` | `logs_toast.go` | Transient alert, top-centre of grid pane. **Error-only**, plain string, fixed 240-frame TTL, no state, no progress. Ticked in `game_update.go:76`, drawn from *inside* `drawGridHelpButton` (`grid_pane_draw.go:1416`). |

Extend `logsToast` into a one-slot status host — same type, same tick/draw
chokepoints, one extra mode:

```go
func (t *logsToast) push(text string)                  // unchanged: error, TTL
func (t *logsToast) beginTask(key i18n.Key, total int) // sticky, determinate
func (t *logsToast) updateTask(done int)
func (t *logsToast) endTask()
```

- **Sticky** (no TTL) while a task is live; `tick()` skips the countdown rather
  than introducing a second lifetime concept.
- **i18n-keyed, re-rendered every frame** (same mechanism as
  `notification.display()`), so "Loading circuit… 42/120" follows a mid-load
  locale switch. The error path keeps its plain-string behaviour.
- **Progress bar drawn with `drawRect`**, not `drawRoundedRect` — deliberate:
  `collectFilledRects` sees `drawRect`, but text drawn to a SubImage does not
  propagate under ebitenstub (established by the notification-band pixel test).
  Track `colSurface3`, fill `colPrimary`. No new color literals.
- It gets **its own GridTree layer** (`grid-toast`) above `grid-help-button`,
  with a row added to the pinned table in `grid_layer_zorder_test.go`. Today it
  is painted from inside the help-button layer's draw func, which is why a long
  toast can overlap the top-right button cluster.

**Placement.** Desktop: the existing top-centre grid-pane rect, unchanged.
Mobile, when the grid pane is not the active view mode: the same toast rendered
as a strip pinned above the bottom nav, so it is visible in every view mode.
One toast object, one draw method, two host rects.

**Play button**: `Button.Disabled = true`. The field already exists
(`design_types.go`, `uigrid.go`) with disabled styling and input suppression.
No bespoke progress rendering on the keycap — the toast owns progress display.

### 4. Reuse of existing notification/logs infra

- **Completion pushes exactly one keyed notification** —
  `notifyInfoKey("notif.circuit.ready", elapsed, count)` — into
  `notificationStore`. That buys the durable Logs-panel line, the unread badge,
  persistence across restarts, and translation, with no new code. **Start
  pushes nothing** (no ring spam).
- Failure/timeout → `notifyErrorKey`, which already raises the error toast.
- New hooks kinds `EventWarmupStarted` / `EventWarmupComplete`, scope
  `ScopeSession`, tag `warmup`, feed `internal/eventlogger` so the Logs panel's
  merged *event* stream carries the narrative too. Full ritual required:
  `KindAll` entry, `ActionRegistry` entry with a lowercase tag, an
  `internal/eventlogger` formatter, and `hooks.NumNonVerbose` 76 → 78.

The `beginTask`/`updateTask`/`endTask` API is generic, so WAV import, recording
finalize and template fetch can adopt it later. It is built for warm-up only;
no unused hooks are added for hypothetical callers.

### 5. Platform warm backends

One interface, two implementations, **neither may silently drop work**.

**Desktop.** Keep the `audio.prewarm` pool but stop using non-blocking `Submit`
for gated warm-up: the controller owns the queue and feeds workers so
backpressure blocks the *feeder*, never discards an item. Worker count becomes
a `RuntimeProfile` field (`PrewarmWorkers`: desktop 4, browser 1).

> Per the project's own standing-pool rule this is **three edits, not one**:
> the standing-pool table in `CLAUDE.md`, `standingFootprint` in
> `internal/async/registry_footprint_test.go`, and `minDefaultBudget` bumped to
> the new total. The documented precedent for skipping it is an app panic on
> Record.

**Browser.** Replace the no-op stubs in `voice_prewarm_js.go` with real
implementations:

- `warmInstrumentBatch(items) → Promise` in `audio.js`, driving
  `ensureRenderReady` per `(id, pitch)`, reporting progress + completion to Go.
- `VoiceCached(id, pitch)` becomes a real query into `renderCache`, keyed by
  `cacheKeyFor(id, roundPitchForCache(pitch))` plus the param generation,
  instead of the hardcoded `false` it returns today. Without this the browser
  cannot skip already-warm voices.
- Render worker pool 1 → `min(4, hardwareConcurrency - 1)`. This is what takes
  the measured 7.79 s full warm down to roughly 2 s.
- The measured **716 ms main-thread stall** during eager warm is *attributed
  before it is fixed*: profile whether it is the result transfer,
  `ensureAudioBuffer`, or headless CPU contention, then fix or document. No fix
  is specified here because the cause is not yet known.

### 6. Cache-hit identity

The diff uses the **same key computation dispatch uses** —
`pitchedVoiceKeyForParams` on desktop (already guarded by
`TestPrewarmVoiceParamsKeyMatchesDispatch`), and the JS `cacheKeyFor` +
`paramsGenOf` pair in the browser. The controller computes the warm set, asks
the cache which items it already holds, and renders only the remainder.

Guarantees, each becoming a test:

- Re-importing a **byte-identical document** → 0 renders, gate opens next frame.
- Importing a document sharing instruments+params+pitches with the current one
  → only genuinely new items render.
- Changing **only** effects / pan / sends / volume / EQ → **0 renders**. Insert
  effects are post-voice processing and are correctly absent from
  `voiceCacheKey`; pin this so nobody "fixes" it later.
- Changing BPM → variant instruments re-key and re-warm; fixed-duration core
  instruments (`bpm: 0` in their key) stay hit.

### 7. Non-voice warm-up

- **Browser channel + insert-FX graph**: `updateInsertEffects` pushes onto
  `pendingChannelOps` when no `AudioContext` exists, so a pre-unlock import
  defers every channel/FX node into a burst at first play. The gate forces the
  context and flushes those ops before opening.
- Send delay/reverb buffer allocation; per-instrument analyser wiring
  (`reapplyAnalyzerTaps` already runs at import).
- **Desktop** insert-FX processors are already built eagerly in
  `SetInsertEffects` — verified, no change needed.

## Measurement & verification

### Phase 0 instrumentation (lands before any optimization)

The desktop bench already prints `RenderLatency` but is **missing
`StalePlays`** — the counter that says "we played the wrong buffer".

- Add `stalePlays`, `neighborPlays`, `warmSetSize`, `warmSkippedCached`,
  `warmMS` to the desktop `[bench_json]` line and the browser metrics snapshot.
- Add an **`import-cold` scenario** to both harnesses: load circuit →
  immediately play → report. Permanent versions of the throwaway probes used
  for this baseline, so the regression is trackable in `bench-results/`.
- Reset render-latency metrics at bench start (they currently carry startup
  renders in).

### Acceptance criteria

Browser (beach-gaelic, import → immediate play):

| Metric | Baseline | Target |
|---|---|---|
| `neighborPlays` in first 30 s | 66 | **0** |
| `deferredPlays` / deferMs | 4 @ 288 ms | **0** |
| renders during playback | 75+, still climbing at 27 s | **0** |
| gate wait | n/a | ≤ ~2 s |
| re-import identical doc | — | **0 renders, ≤ 1 frame** |

Desktop (same circuit, 15 s):

| Metric | Baseline | Target |
|---|---|---|
| `prewarmRenders` during playback | 175 | **0** |
| `dispatchRenders` | 0 | **0** (must stay) |
| gate wait | n/a | ≤ ~600 ms |

### UI verification

Mandatory per project rules: scenes `warmup_progress` and
`crop_warmup_progress` in `scene_catalog.go`, captured and **visually
inspected** on desktop and with `MOBILE=1` (mobile capture must exercise a
non-grid view mode to prove the bottom-nav fallback strip). i18n strings via
`internal/i18n` (no hardcoded English), design tokens only, no forbidden
glyphs, `RadiusControl`/`RadiusContainer` by role.

### Test discipline

`t.Cleanup(g.CloseForTest)` on every test Game; `goleak` clean; any transient
named pool released in cleanup.

## Phasing

0. Metrics + `import-cold` bench scenarios → re-measure → record baseline.
1. Warm-set enumeration (full-loop horizon) + the cache diff.
2. Non-dropping warm backends; de-stub `voice_prewarm_js.go`; real browser
   `VoiceCached`.
3. Transport gate + progress toast + notification/hooks wiring
   (screenshot-verified).
4. Browser render-worker pool widening + main-thread-stall attribution.
5. Re-measure against the acceptance table; record in `bench-results/`.

## Rejected alternatives

- **Pre-render the circuit into a serialized voice bundle at import.** The
  expensive part is the C render itself, which still has to happen on whichever
  platform loads the file. Would bloat project files or add a cache-file format
  for no throughput win.
- **Minimal fix: enumerate the whole circuit + raise queue/worker counts, no
  gate.** Cheap, and would help desktop materially, but leaves the browser's
  measured 7.8 s serial warm racing against playback with no guarantee. Its
  cheap wins are folded into this design instead.
- **Progress rendered on the Play keycap.** Superseded by the notification-based
  progress toast, which reuses existing infrastructure and keeps the button on
  its existing `Disabled` styling.
