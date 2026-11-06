# Off-thread voice pre-warming — design

**Date:** 2026-07-13
**Branch:** add-core-node-types
**Status:** approved, pending implementation plan

## Problem

`make run-debug` panics after ~38 s of playback of the startup demo:

```
panic: audio timing parity breach: overdue/absmax
```

### Root cause (verified from the crash dump + code trace)

1. A melody note (the demo's `organ-church`, pitch **−2 st**) is dispatched
   **28 ms overdue** (`overdue.maxLag=0.027761`), coinciding with a **64 ms
   Stage-B bridge spike** (`bridge.max=0.064098`) — i.e. the single `audioLoop`
   goroutine was blocked for 64 ms.
2. The block is a **synchronous cold voice render** on the dispatch goroutine.
   `PlayBatch` → `playParamsIdentity` → `newRecipeAwareVoicePitched` →
   `tryRecipeVoicePitched`. On a cache **miss** this calls `recipe.Render(...)`
   inline (13–64 ms for a heavy additive/formant organ). While it renders, the
   next scheduled beat waits in `audioCh` → dispatches late → Stage C overdue.
3. The miss happens because pitch **−2** is **not** in the fixed warm spread
   `DefaultWarmPitches = {-12,-7,-5,0,4,7,12}` (`voice_warm.go`). Warming today
   only fires on **instrument switch**, at a guessed pitch spread — it does not
   cover the actual pitches the circuit plays.
4. Stage C `Overdue.AbsMax = 0.020` is a **single-sample dropout check that
   fires regardless of `MinSamples`** (`evalTimingStage`). One 28 ms-late beat
   latches `maxLag` → fatal panic. `PERF_LOG=1` (set by `run-debug`) resets
   `schedMetrics` every 2 s (`game_update.go:66`), so the gate runs over a
   rolling ~1.5 s window that never reaches `MinSamples=64` — only the
   single-sample dropout checks are ever live, making it a hair-trigger:
   any single overdue beat over the session panics.

This is the "I1 accepted-by-design" case previously assumed not to occur on real
desktop (`overdueCount=0`). New information: it **does** occur on real
interactive desktop when the circuit plays a pitch outside the guessed warm
spread.

### Chosen fix direction (user-selected)

Attack the physical cause: **the voice render must never run on the `audioLoop`
dispatch goroutine.** Since rendering async *at dispatch time* would delay/drop
the note, the voice must be **warm in `globalVoiceCache` by dispatch time**.
Therefore: pre-warm the actual pitches the circuit will play, ahead of dispatch,
on a background pool. (Approach 2 below.)

The timing gate itself is **not** changed — it correctly flagged a genuine late
beat. We remove the lateness at its source.

## Non-goals

- No change to `parityTimingCheck`, Stage C bounds, or the fatal policy.
- No change to the browser render-worker warm path (`voice_warm_js.go`), which
  already off-threads warming.
- No general voice-render optimization (voice-pool rework, SIMD, etc.).

## Foundations (existing, reused)

- `globalVoiceCache` (`voice_cache.go`) — `RWMutex`-guarded; concurrent
  warm + dispatch is already safe.
- `tryRecipeVoicePitched(id, bpm, sr, pitch)` / `newRecipeAwareVoice(id, bpm,
  sr)` (`synth_recipe_dispatch.go`) — render-into-cache; cache **hit** returns
  instantly. These are exactly what dispatch calls.
- `evalNodeParamsOnly(row, idx, info)` (`game_trigger_logic.go`) — pure param
  math returning the beat's `(vol, pitch, dur)` including node pitch and (for
  group-ruled nodes) the `idx`-dependent `model.GroupEffect` transpose. No
  render.
- `async.DefaultRegistry().Get(name, opts)` with private-pool fallback —
  pattern at `game_new.go:216-220` (`audio.timers`).

## Architecture

Two producers feed one background render pool that populates `globalVoiceCache`.
The `audioLoop` dispatch goroutine is unchanged; it simply finds warm cache
entries.

### New audio seam — `audio.PrewarmVoice(id string, pitch float64)`

Native build (`!js && !test`): renders `(id, pitch)` into `globalVoiceCache`
through the **same** functions dispatch uses, so the produced cache key is
byte-identical to the one dispatch reads:

```
recipeID := RecipeForInstrument(id)
if pitchAwareRecipe(recipeID) {
    newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch) // -> tryRecipeVoicePitched
} else {
    newRecipeAwareVoice(id, bpm, SampleRate())              // pitch ignored; key has no pitch
}
// returned voice discarded; the render+Put is the point
```

Stub: `-tags test` → no-op (C renderer stubbed; UI tests observe the seam via a
package-var indirection, mirroring `WarmInstrument`). `//go:build js` → **no-op**
(the browser already renders warms off-thread on its render worker; the two
producers below still call `PrewarmVoice`, they just no-op on js — extending
browser coverage to per-note pitches is explicitly out of scope).

**Correctness linchpin:** warm key **must equal** dispatch key for both
pitch-aware and non-pitch-aware recipes. This is asserted by a dedicated
cache-hit test (see Testing #1). Non-pitch-aware recipes must route through the
identical no-pitch path so `merged["pitch"]` presence/absence matches.

### New pool — `audio.prewarm`

`async.Options{MaxConcurrent: 2, QueueSize: 64, Name: "audio.prewarm"}` via
`DefaultRegistry().Get`, private-pool fallback on budget exhaustion. Owned by
`Game`, drained on `Close`. Two workers so a burst of distinct pitches warms in
parallel without starving the single audio timer worker.

### Producer 1 — graph-change warm (`voicePrewarmer`, internal/ui)

New focused file (`voice_prewarmer.go`). Hooked into `updateBeatInfos()` (runs
on every circuit load / graph edit) and the BPM-change path (BPM is part of the
cache key for BPM-derived instruments).

`warmFromBeatInfos()`:
- For each row `r` with instrument `inst_r`, iterate `beatInfosByRow[r]` and call
  `evalNodeParamsOnly(r, idx, info)` to get each beat's pitch (captures node
  pitch **and** windowed group transpose — no render).
- Collect the distinct `(inst_r, roundPitchForCache(pitch))` set.
- For each pair not already in `seen`, `pool.Submit(func(){ PrewarmVoice(inst,
  pitch) })`; add to `seen`.

`seen` is a bounded `map[...]struct{}` cleared on graph change, instrument
change, and BPM change so stale pitches don't pin memory and re-warm happens
after invalidation.

### Producer 2 — schedule-time backstop (`scheduleSound`)

In the sequencer path (`game_audio_schedule.go` `scheduleSound`, `sequencer ==
true`), after the final post-effect `pitch` is known: non-blocking
`pool.Submit(func(){ PrewarmVoice(inst, pitch) })`, deduped against the same
`seen` set. Covers group/logic pitches beyond the enumerated `beatInfosByRow`
window.

## Data flow

```
graph load/edit ─► updateBeatInfos ─► voicePrewarmer.warmFromBeatInfos ─┐
BPM change ────────────────────────► warmFromBeatInfos ─────────────────┤
                                                                        ├─► pool.Submit ─► PrewarmVoice ─► recipe.Render ─► globalVoiceCache.Put
scheduleSound (post-effect pitch) ─► pool.Submit ──────────────────────┘
                                                                             audioLoop ─► newRecipeAwareVoice* ─► cache HIT (no inline render, no block)
```

## Error handling / degradation

- `Submit` is non-blocking. On `ErrBackpressure` the warm job is **dropped** —
  playback degrades to today's inline render for that one beat; the producer
  never blocks.
- If a warm job has not finished when dispatch needs the voice, `audioLoop`
  renders inline exactly as today: a single transient, not a repeated block.
- Param edits already `ClearInstrument`; the prewarmer clears `seen` on
  instrument change and re-warms on the next graph-change.

## Concurrency

The warm pool reads the `bpm` global and instrument params and renders —
identical access to what `audioLoop` already performs today, so no new race
class is introduced. `sampleRate` is init-once. `globalVoiceCache` is
`RWMutex`-guarded. `PrewarmVoice` allocates its own render buffer (as
`tryRecipeVoicePitched` already does) and hands ownership to the cache at `Put`
per the existing cache contract.

## Testing (TDD)

Fast `-tags test` path where possible; the render-key tests need the native
renderer and run under the non-test build.

1. **Key-match (linchpin).** Warm `(id, pitch)`, then dispatch the same `(id,
   pitch)`; assert a **cache hit** via `recordVoiceCacheHit` counter /
   `VoiceCacheStatsForTest`. Cover both a pitch-aware recipe (e.g. an organ at
   −2) and a non-pitch-aware recipe (a drum) so the no-pitch key path is proven
   to match. Red-first: without `PrewarmVoice`, dispatch is a miss.
2. **Enumeration.** A circuit with a node at a non-default pitch (−2) →
   `voicePrewarmer` submits exactly `(inst, −2)` (observe via the seam
   package-var in the test build).
3. **Backstop.** A group-transposed pitch beyond the enumerated window →
   `scheduleSound` submits it.
4. **Dedup.** Repeated frames / beats do not resubmit an already-warmed pair.
5. **Regression (functional, end-to-end).** Drive `Game.Update` through the
   crashing-demo scenario (organ at −2) with the timing gate armed via
   `enableParityTimingForTest`; assert **no overdue panic** because the voice is
   warm before dispatch. This is the real proof and follows the
   functional-tests-over-isolation preference.

## Files touched (anticipated)

- `internal/audio/voice_prewarm_desktop.go` (new) — `PrewarmVoice` native.
- `internal/audio/voice_prewarm_stub.go` (new) — `-tags test` no-op + seam var.
- `internal/audio/voice_prewarm_js.go` (new) — js no-op / forward.
- `internal/ui/voice_prewarmer.go` (new) — `voicePrewarmer`, pool, `seen`,
  `warmFromBeatInfos`.
- `internal/ui/game_new.go` — construct pool + prewarmer.
- `internal/ui/game_graph_update_beat_infos.go` — call `warmFromBeatInfos`.
- `internal/ui/game_audio_schedule.go` — schedule-time backstop Submit.
- BPM-change path — trigger re-warm + `seen` clear.
- `internal/ui/game_*close*.go` — pool release on Close.
- Tests per the Testing section.

## Open follow-ups (not in scope)

- The `PERF_LOG`-driven 2 s `schedMetrics.Reset()` couples the timing-gate
  window to the perf-log cadence, defeating the statistical (`MinSamples=64`)
  stages under `run-debug`. Worth decoupling later, but it does not cause this
  crash and is out of scope here.
