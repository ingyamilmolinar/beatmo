# Predictive neighborhood voice cache — design

**Date:** 2026-07-13
**Branch:** add-core-node-types
**Status:** A/B/D + clear-removal + hot-path alloc opt IMPLEMENTED & verified; C (synth-control neighborhood) scoped as next increment.

## Implementation status (2026-07-13)

- **Hot-path alloc opt** — memoized `sharedModularParamSchemaIdentity`: 178KB→23KB,
  3027→997 allocs/render, byte-identical (golden suite green).
- **A. Byte-budget + LRU** — DONE. `voiceCache` now bounds by `budgetBytes`
  (desktop 48MB / browser 24MB) with LRU-by-access eviction + `maxEntries`
  guard. Verified: `TestGlobalVoiceCacheByteBudgetSoak` (500×176KB warms →
  bounded to 11 entries / 1.94MB under a 2MB budget), `TestVoiceCacheByteBudget
  EvictsLRU`, `TestVoiceCacheMaxEntriesEvictsLRU`. `Get` stays alloc-free
  (`TestSynthTabHotPathAllocBudget` green).
- **Clear-removal (the storm fix)** — DONE. `SetInstrumentParam` no longer
  blanket-invalidates (paramsHash is complete; LRU bounds memory). Storm A/B
  (knob-drag, 18s): cyclic drag misses **828→96**, heap **1.0GB→423MB**;
  monotonic sweep 125 misses (leading-edge, needs C). `TestSetInstrumentParam_
  DoesNotBlanketInvalidate`. Full audio suite green (only the behavior-pinning
  test changed).
- **B. Melodic-pitch neighborhood** — DONE. `warmFromBeatInfos` emits
  `nodePitch ± ticks×0.5st` for melodic instruments only. `TestPitchNeighborhood
  WarmedForMelodic`, `...SkippedForNonMelodic`.
- **D. Config** — DONE. `RuntimeProfile.VoiceCacheBudgetBytes` +
  `PredictiveWarmTicks` (default 1); wired via `audio.SetVoiceCacheBudgetBytes`
  at prewarmer construction.
- **C. Synth-control neighborhood** — NEXT INCREMENT. Needs the
  `audio.PrewarmVoiceParams(id, pitch, params)` explicit-params render seam
  (extract a params-taking core from `tryRecipeVoicePitched`) + a focus/edit
  trigger in `synth_panel_zone.go` calling `warmControlNeighborhood(instID,
  param, cur, step, n)` at each current node pitch. Deferred to ship the
  verified A/B/D+clear-removal increment first; the clear-removal already makes
  synth-control cyclic drags hit (96 misses), so C is the leading-edge polish.

## Goal

Raise the voice-cache hit rate for *immediate* real-time edits (nudge a melodic
pitch, drag a synth control) so the edit does not force a cold ~24ms C render on
the audio dispatch goroutine — while keeping total cache memory hard-bounded so
the game never OOMs or hoards RAM.

## Evidence that scopes the work

Measured on the startup demo (see `synth_render_cost_bench_test.go`,
`synth_render_scaling_bench_test.go`, and the knob-drag storm runs):

- **organ-church cold render ≈ 24ms isolated, 41–60ms under load, up to 122ms
  during a knob-drag storm** (GC-amplified). 96% of it is `render_modular_p`
  (C DSP). Linear in the render window (1.0s buffer = `Beats=2.0`).
- **A knob drag produces a 93% cache MISS rate** — every `SetInstrumentParam`
  calls `ClearInstrument`, so the whole instrument re-renders each beat.
- **Volume, duration, and non-melodic pitch are already cache hits** — they are
  post-render transforms (`scaledVoice.gain`, `resampleVoice.step`), NOT in the
  cache key. Only **melodic pitch** (baked into the render; key carries
  `pitch@0.5st`) and **synth controls** (change `paramsHash`) re-render.
- The per-render allocation hot path was fixed separately (memoized
  `sharedModularParamSchemaIdentity`: 178KB→23KB, 3027→997 allocs/render,
  byte-identical). That reduces GC amplification but does NOT change the miss
  rate — this design does.

## Scope

In: neighborhood pre-warming for (1) melodic pitch and (2) the synth control
currently being edited; a byte-budgeted + LRU voice cache to bound memory.

Out: volume/duration/non-melodic-pitch (already hits); browser render-worker
warm; C DSP optimization; render-window reduction (ear-check territory).

## Design

### A. Byte-budget + LRU eviction in `voiceCache` (memory bound — the crux)

Today `globalVoiceCache` caps at **256 entries, no eviction** — a poor memory
proxy (organ entry = 3×176KB = 528KB; shaker = 3×53KB = 159KB). Neighborhood
warming multiplies keys, so replace the entry cap with:

- **`budgetBytes`** hard ceiling (configurable; default **48MB** — tighter than
  today's 135MB worst case). `curBytes` tracks the sum of all variant lengths×4.
- **LRU by access:** `roundRobinEntry` gains `lastNS atomic.Int64`, stamped on
  every `Get` (atomic store, still under RLock — no Get serialization) and on
  `Put`. When a `Put` pushes `curBytes` over budget, evict the entries with the
  smallest `lastNS` until under budget.
- **Secondary `maxEntries`** guard (e.g. 1024) bounds the eviction scan cost and
  map size regardless of buffer sizes.
- **Safety:** eviction only deletes the map entry; any in-flight `cVoice`
  holding the `[]float32` keeps it alive via GC until the voice finishes. A
  currently-playing voice has a recent `lastNS`, so it is never the eviction
  victim. `lastSamples` (Synth-tab preview pointer) is unchanged.
- Net: warming raises hit rate, never peak memory — the budget is a hard ceiling.

### B. Melodic-pitch neighborhood (`warmFromBeatInfos`)

For each enumerated melodic `(inst, nodePitch)`, also warm `nodePitch ± k×0.5`
for `k in 1..N`. Bounded by `nodePitches × (2N+1)`. Deduped within the pass.
Non-melodic instruments are unaffected (their pitch is post-render).

### C. Synth-control neighborhood (`warmControlNeighborhood`)

`(*voicePrewarmer).warmControlNeighborhood(instID string, param string, cur
float64, step float64, n int)`: for `k in -n..n`, build the full merged params
with `param = cur + k*step` (all other params current), and warm the resulting
key at each current node pitch of any row using `instID`. **Only the focused
control is warmed** — never all controls (explosion trap). Fired from the
synth-tab focus/edit site (`synth_panel_zone.go` knob handler) so the neighbor
of the next nudge is already rendering.

The audio seam: `audio.PrewarmVoiceParams(instID string, pitch float64, params
RecipeParams)` renders a voice for an explicit param override set (so the UI can
warm a hypothetical neighbor value without mutating live instrument params).
No-op stub under test/js.

### D. Configuration (`RuntimeProfile`)

- `VoiceCacheBudgetBytes` (default 48MB desktop; smaller on browser).
- `PredictiveWarmTicks` N (default 1).
- Both overridable for tests/bench.

## Verification (required)

1. **Memory ceiling test** (`voice_cache` native): warm thousands of distinct
   neighbor keys; assert `VoiceCacheStatsForTest().totalBytes ≤ budget` and
   entries were evicted (LRU), and that a recently-`Get` key survives while a
   stale one is evicted.
2. **Benchmark:** neighborhood-warm throughput + the existing render-cost bench;
   confirm the memoization + budget don't regress render ns/op.
3. **Profiler / storm A-B:** re-run the knob-drag storm (alloc + heap profile);
   target: miss rate collapses from 93% (neighbor of the dragged value is warm),
   `heap` stays bounded (no 1GB spike; ≤ budget + transient render garbage),
   overdue drops.
4. **Correctness:** golden/determinism suite stays green (byte-identical
   renders); `TestVoiceCached*` still pass.

## Files (anticipated)

- `internal/audio/voice_cache.go` — byte budget + LRU eviction + `lastNS`.
- `internal/audio/voice_prewarm_desktop.go` (+ stub/js) — `PrewarmVoiceParams`.
- `internal/audio/synth_recipe_dispatch.go` — key helper reused for params warm.
- `internal/ui/voice_prewarmer.go` — pitch neighborhood + `warmControlNeighborhood`.
- `internal/ui/synth_panel_zone.go` — fire neighborhood warm on control focus/edit.
- `internal/ui/runtime_profile.go` — budget + N config.
- Tests + benchmarks per Verification.

## Risks

- Touching the central `voiceCache` eviction — mitigated by keeping the public
  API (`Get`/`Put`/`Has`/`Clear`/`ClearInstrument`) identical, and by the memory
  + golden tests. Existing soak tests that assumed the 256-entry cap must be
  re-pointed at the byte budget.
