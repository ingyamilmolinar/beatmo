# Real-time param-edit audio model (desktop) — design

**Date:** 2026-07-15
**Branch:** add-core-node-types
**Status:** approved design, not yet planned
**Scope:** DESKTOP / native Go + CGo DSP only. The browser/WASM path already shipped this fix
(see `docs/superpowers/specs/2026-07-02-wasm-realtime-synth-latency-bench-and-fix-proposal.md`).

---

## 1. Problem

Changing a synth knob or a sampler setting while playback is running can make audio crack.

The requirement, stated by the user:

> The audio should not crack or thrash or have any noticeable effect on the audio from all previously
> triggered nodes. The effects should be chained and the audio buffer computation must happen
> predictably and applied to the next iteration of that instrument's trigger nodes, leaving the rest
> of the audio unchanged.

### 1.1 What is already correct (do not rebuild)

The intended model largely exists. The cache key is:

```go
// internal/audio/voice_cache.go:34-40
type voiceCacheKey struct {
    instrumentID string
    bpm          int     // 0 for instruments whose render is BPM-independent
    sampleRate   int
    paramsHash   uint64  // FNV-1a over the FULL merged param set; sample-edit hash XORed in
    pitch        float64 // rounded to 0.5 semitone (roundPitchForCache)
}
```

Consequences that already hold:

- **Edits compose latest-wins.** `hashRecipeParams` (`synth_recipe.go:692`) hashes the whole merged
  set with sorted keys, so N successive edits produce one key describing the composed state. There is
  no per-param race.
- **An edit rotates the key rather than mutating anything.** `SetInstrumentParam`
  (`synth_recipe.go:221`) contains no `voiceCacheInvalidate` — deliberately (`synth_recipe.go:256-267`).
- **Already-triggered audio is immutable**, by two independent mechanisms:
  1. No back-pointer from cache to live voices. A new binding renders into a **new** buffer under a
     **new** key; it cannot reach an in-flight `voiceState.v`.
  2. Cached buffers are never written. `cVoice` only reads `buf` (`drums_c.go:80-110`); `Put`
     transfers ownership (`voice_cache.go:122-127`); eviction only `delete`s the map entry and Go's
     GC keeps the slice alive for a voice still reading it (`voice_cache.go:186-187`).

### 1.2 The actual defect

**A cache miss renders synchronously on the `audioLoop` dispatch goroutine.**

```
go g.audioLoop()                         internal/ui/game_new.go:229
  → audio.PlayBatch(batch)               internal/ui/game_audio_loop.go:204
    → for … PlayParamsAt                 internal/audio/batch_desktop.go:7-16   (plain serial loop)
      → playParamsIdentity               internal/audio/engine_play.go:86
        → newRecipeAwareVoicePitched     internal/audio/synth_recipe_dispatch.go:74
          → recipe.Render                internal/audio/synth_recipe_dispatch.go:163  ← CGo, ~24ms organ
```

An edit rotates the key → the next trigger is a guaranteed miss → a ~24ms `render_modular_p` runs on
the dispatch goroutine. A batch of N cold voices costs N renders **serially**.

**Important nuance:** the oto callback (`mixer.Read`, `engine_stop.go:269`) is a *separate*
goroutine fed by a pending queue, so this does **not** cause buffer underruns. It causes **late
beats**: `req.when` is fixed at enqueue, so a stalled dispatch shrinks `lead = when - audioNow` until
it goes negative → `overdue` (`game_audio_loop.go:158`). That is the audible crack.

### 1.3 Two aggravating defects found during design

1. **The sampler path still has the storm the synth path was cured of.** `SetSampleEdit`
   (`sample_edit_descriptor.go:65`) and `ClearSampleEdit` (`:111`) call `voiceCacheInvalidate(instID)`,
   which drops **every pitch's** entry for that instrument. But `hashSampleEdit(edit)` is already XORed
   into `paramsHash` (`synth_recipe_dispatch.go:138`, `:192`, `:264`), so the edit rotates the key by
   itself. The invalidate is redundant — and it re-creates the 93%-miss blanket-clear storm that
   `SetInstrumentParam` was explicitly fixed to avoid.
2. **LRU eviction fires silently.** `evictToBudgetLocked`/`evictLRULocked` (`voice_cache.go:162-199`)
   drop entries with **no** `notifyVoiceCacheInvalidated` call, so an evicted voice is never re-warmed
   and cold-renders on the next trigger.

### 1.4 The measurement gap

There is **no desktop test or benchmark** for "param edit during playback".

| Existing | Why it misses this |
|---|---|
| `make bench-render` (`Makefile:358`) | **Browser-only** — runs `webaudio_render_latency_bench.browser.test.js`. Never touches the CGo render path. |
| `TestSoakHeapBounded_PlayingSynthTabParamChurn` | Churns knobs during playback, but runs under **`-tags test`** where the audio layer is stubbed — **no real render ever happens**. Asserts heap, not latency. |
| `TestRenderLatencyMetrics` | Native, calls `SetInstrumentParam`, but is a 2-trigger counter test — no playback, no timing assertion. |
| `BenchmarkOrganChurchColdRender` | Isolated render cost, no playback. |

`RenderLatencyMetrics` exists and is wired into the native dispatch path, but is surfaced **only** in a
human-readable log line (`game_update.go:116`) and is **absent from the `[bench_json]` line**
(`game_update.go:124`), so `scripts/bench-desktop.sh` never captures it.

---

## 2. The model

### 2.1 Voice Binding

Name the tuple that fully determines a node's audio. This is `voiceCacheKey` today; the design
promotes it from "a cache key" to **the model's contract**:

```
VoiceBinding = { instrumentID, recipeID, bpm, sampleRate,
                 pitch (rounded 0.5st),
                 paramsHash = FNV1a(full merged param set) ^ hashSampleEdit(edit) }
```

**Bind at trigger; never mutate after.**

### 2.2 Invariants

| # | Invariant | Status | Action |
|---|---|---|---|
| **I1** | Once a voice is scheduled, nothing can alter its samples. | Already structurally true (§1.1) | **Pin with tests** |
| **I2** | An edit at T is visible to the first trigger of that instrument after T, and composes latest-wins. | Already true via key rotation | **Pin with tests** |
| **I3** | The `audioLoop` goroutine **never** calls `recipe.Render`. | **False today** | **The change** |
| **I4** | An edit to instrument A cannot change B's audio, nor any already-scheduled voice. | Already true | **Pin with tests** |

Three of four are documentation + tests. Only **I3** needs machinery.

### 2.3 The resolved tension (user decision)

"Edit applies on the next trigger" and "audio never cracks" conflict when a knob moves faster than the
render. Chosen resolution:

> **Never block the dispatch goroutine.** If the exact new-binding voice is not rendered yet, play the
> closest already-rendered variant **on time** and converge on the following trigger.

Timing is always correct; at most one trigger sounds pre-edit during a fast sweep. Prewarm (§4.2) makes
this fallback rare, and the benchmark **reports how rare** (`stalePlays`). This matches the shipped
WASM behavior (`stalePlays`/`neighborPlays` + fresh-by-deadline race), so desktop converges with browser.

### 2.4 Scope of the isolation guarantee (user decision)

The codebase has a clean dividing line, and the guarantee is scoped to the left column:

| Surface | Timing | Retroactive on a sounding note? |
|---|---|---|
| Synth param (`SetInstrumentParam`) | **Baked at trigger** | **No** |
| Sampler sample-edit (`SetSampleEdit`) | **Baked at trigger** | **No** |
| Insert FX param (`SetInsertEffectParam`) | Live per-block | Yes |
| Per-row EQ (`SetChannelEQ`) | Live per-block | Yes |
| Sends / volume / pan | Live per-block | Yes |

Anything feeding *voice synthesis* is baked at trigger. Everything from the per-instrument buffer
onward (`engine_stop.go` Phase 2a/2b) is live per-block and therefore retroactive on in-flight notes.
**That is correct mixer behavior** (it matches a hardware insert), and per-voice FX instances would
multiply FX CPU by the live-voice count.

Therefore: **trigger-isolation applies to the voice; for FX the requirement is "no clicks" (smoothing),
not isolation.** Consistent with `feedback_next_trigger_param_deferral`.

---

## 3. Phase 1 — measurement (ships first, alone)

### 3.1 Instrumentation: render origin

`RenderLatencyMetrics` (`internal/audio/render_latency.go:20`) gains:

- `DispatchRenders int64` — renders that ran on the audioLoop dispatch goroutine (**the bug**)
- `PrewarmRenders int64` — renders that ran off-thread (**healthy**)

Threaded **explicitly** through `tryRecipeVoiceOpts` as an origin argument. Not goroutine-ID sniffing.
This is the desktop analogue of the WASM gate `mainThreadRenders == 0`.

### 3.2 Benchmark

**`internal/ui/param_edit_playback_bench_test.go`**, `//go:build !test && !js`.

Untagged is **mandatory**: under `-tags test` the synth is stubbed and no real render happens — the
exact reason the existing knob-churn soak test never caught this.

Setup: startup demo (`assets_pkg.StartupDemoJSON`, contains `organ-church` = worst case), real `Game`,
deterministic playback via `advancePlaybackByAbs`. Must **not** call `SetPlayFunc` (that stubs the
trigger and the render never happens).

Scenarios: `baseline` → `synth-knob storm @60Hz` → `sampler sample-edit storm` → `FX-param storm` →
`combined`.

### 3.3 Gates

Define `exactMissRatio = (triggers served without an exact-binding cache hit) / triggers`.
A blanket-invalidate storm drives this to ≈1.0 (every note re-renders); a healthy cache drives it toward 0.

| Tier | Metric | Rule |
|---|---|---|
| **Primary** (deterministic) | `DispatchRenders` | **== 0** — the one unambiguous statement of I3 |
| ~~Primary~~ **Reported only** | `exactMissRatio` | **NOT gateable** — measured in Phase 1, see below (amended 2026-07-15) |
| Secondary (reported) | `overdue`, `smallLead` | reported; env-tunable, never a hard flake |
| Secondary (reported) | `renderMax`, `renderAvg` | reported only |
| Secondary (reported) | `stalePlays`, `neighborPlays` | reported only |
| Secondary (reported) | FX discontinuity magnitude | reported only (feeds the follow-up spec) |

**AMENDED 2026-07-15 (user-approved): `exactMissRatio` is NOT gateable — demoted to reported evidence.**
Phase 1 measured it and this section's premise was false. The SAME `baseline` scenario (no edits at all)
produced missRatio 0.667 / 0.179 / 0.090 and triggers 9 / 67 / 78 across three runs. A cold-cache reset
plus a realistic 60-step warmup did NOT stabilise it: the cause is structural, not cache warmth —
`audioLoop` is a real goroutine racing the test's tight `Update()` loop, so `triggers` is
timing-dependent by construction and any ratio over it is a coin flip. `DispatchRenders == 0` is the
sole gate, and is the better one: post-Task-8 it is a *structural* property (non-blocking dispatch never
renders), not a statistical one. The original reasoning below is retained for history.

**How the `exactMissRatio` threshold is chosen — deliberately not guessed now.** Phase 1 ships the
benchmark with this metric **recorded and printed but not gated**, producing the pre-fix baseline
(expected ≈1.0 for the sampler storm, given §1.3(1)). Phase 2 lands the model change, and the gate is
then pinned just above the measured post-fix value with margin. Inventing a number before measuring is
how you get a gate that either never fires or flakes forever.

`DispatchRenders == 0` is gated from Phase 1 onward and is expected to **fail on the pre-fix tree** —
that failing gate is what justifies Phase 2.

**Wall-clock is reported, never gated.** Precedent: the first cut of the solo-analyzer cost table
asserted on wall-clock and flaked immediately under parallel suite load. Counts are deterministic;
times are not.

### 3.4 Bench JSON

Add RenderLatency (incl. the new origin split) to the `[bench_json]` line at `game_update.go:124` so
`scripts/bench-desktop.sh` captures it.

---

## 4. Phase 2 — the model change

### 4.1 Non-blocking dispatch + best-available fallback

`PlayBatch` renders nothing. `tryRecipeVoice*` gains a must-not-block mode used by the dispatch path.

On miss:
1. Select the best available variant — same pitch with an older `paramsHash`, else nearest cached pitch
   (the existing `playbackRate`/`resampleVoice` path).
2. Schedule it **at the intended `when`**.
3. Record `stalePlay` / `neighborPlay`.
4. Submit the exact binding's render to the off-thread pool so the next trigger hits.

Dispatch never renders → **I3**.

### 4.2 Bind-ahead prewarm on param edit (the deferred "Part C")

`SetInstrumentParam` / `SetSampleEdit` publish a binding-changed event (a dedicated observer — **not**
`platformInstrumentParamsChanged`, which is the JS bridge and semantically wrong).

`voicePrewarmer` enumerates the keys the next triggers will read (instrument × each enumerated node
pitch × current bpm × new hash) and renders them on the existing `audio.prewarm` pool.

Requires `audio.PrewarmVoiceParams(id, pitch, params RecipeParams)`, extracted as a params-taking core
from `tryRecipeVoicePitched`, **so the warm key is byte-identical to the read key**. Warming a key
nobody reads is the primary failure mode here; pin it with a linchpin test following the existing
`TestPrewarmVoiceKeyMatchesDispatch` precedent.

### 4.3 Eviction pinning

Maintain a **pin-set** of the bindings the next triggers will read; `evictToBudgetLocked` /
`evictLRULocked` skip pinned keys and evict the next-best instead. Bounded by node count, so the 48MB
byte budget stays honest. Closes the silent cold-miss source in §1.3(2).

### 4.4 Sampler blanket-invalidate removal

Remove `voiceCacheInvalidate` from `SetSampleEdit` / `ClearSampleEdit`, mirroring the synth fix.

**Must verify before removing:** the user-sample/WAV path goes through `reapplyUserSampleEdit` →
`RegisterSamplePCM` (`sample_edit_descriptor.go:88-100`), which is **not** voice-cache-keyed. The
invalidate may still be load-bearing for that path. Confirm with a test on a WAV-backed instrument
before deleting; if load-bearing, narrow the invalidate to the user-sample case rather than dropping it.

---

## 5. Testing

### 5.1 Invariant tests (fast path, `-tags test` where possible)

- **I1**: capture a scheduled voice's samples, apply a param edit mid-flight, assert the in-flight
  buffer is **bit-identical**.
- **I2**: edit → assert the next trigger reads the new binding; N successive edits → assert one
  composed latest-wins binding.
- **I4**: edit instrument A → assert B's binding and cached entries are untouched.

### 5.2 Gates (untagged, real DSP)

As §3.3. Run via explicit package path (`./internal/ui/`), never `./...`
(`reference_phantom_dotdotdot_build`).

### 5.3 Sampler storm regression

Miss-ratio across a sample-edit storm, before/after §4.4.

---

## 6. Out of scope — follow-up spec

**FX / EQ live-param click-free.** Measured here, remediated separately:

- **14 of 24 C effects are unsmoothed** (bitcrusher, waveshaper, ringmod, tremolo, gate, limiter,
  flanger, phaser, autowah, compressor, transient, tape, pitchshift). Only distortion/delay/reverb/
  chorus carry the 5ms one-pole (`IFX_SMOOTH_TIME_MS`, `insert_fx.h:15`).
- **`SetChannelEQ` is the most click-prone path**: it replaces the processor and **zeroes the biquad
  state** (`eq.go:268` → `RebuildChannelWithEQ`), so the new filter does not continue the old
  trajectory.
- **Go/C smoothing parity gap**: `param_smooth.go` is `//go:build test || js` — smoothing is **not
  compiled into the native desktop binary**. The Go filter documents a ~5ms crossfade
  (`fx_filter.go:10-12`) that the C filter never implements.
- **Data race**: `SetInsertEffectParam` writes the C struct under `insertChainMgr.mu`
  (`effect_chain.go:105`) while the audio thread reads it under `c.mu` (`channels.go:317`) — different
  mutexes. `-race` would flag it.
- `antiPopVoice` cannot mask any of this: it is **upstream** of the insert chain (Phase 1 vs Phase 2a).

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| Warm key ≠ read key → prewarm renders keys nobody reads, no measurable gain | Linchpin byte-identity test (`TestPrewarmVoiceKeyMatchesDispatch` precedent); the origin gate would show `DispatchRenders > 0` regardless |
| Best-available fallback masks a real regression (always stale, "no crack" but wrong sound) | Gate the **miss ratio** too, and report `stalePlays`; a healthy run has both low |
| §4.4 breaks WAV-backed sampler instruments | Verify the `RegisterSamplePCM` path first (§4.4); narrow rather than delete if load-bearing |
| Timing-based assertions flake under CI/parallel load | Primary gates are **counts**, not times (§3.3) |
| Pin-set grows unbounded → byte budget dishonest | Pin-set bounded by enumerated node count; assert its size in a test |
