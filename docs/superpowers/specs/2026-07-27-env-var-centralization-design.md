# Environment Variable Centralization — Design

**Date:** 2026-07-27
**Status:** Approved, pending implementation plan

---

## Problem

The Beatmo Go tree reads **95 distinct environment variables** across **99
`os.Getenv` / `os.LookupEnv` call sites in 46 files**. Two further names survive
only in a stale test list with no reader, bringing the inventory in §Complete
inventory to 97 rows. There is no single place that knows what they are, what
they do, or which ones still exist. Four concrete defects follow from that.

### 1. `os.Getenv` is a silent no-op in the browser — all 95 knobs are dead on WASM

`src/js/wasm_exec.js:106` constructs the Go bridge with `this.env = {}`, and
nothing anywhere assigns to it. `wasm_exec.js:528` copies `this.env` into linear
memory as `KEY=value` pairs for Go's `goenvs_unix`, so an empty object means
`os.Getenv` returns `""` for every key in the browser build.

The casualties include variables written *specifically* for WASM:

| Var | Site | Intended purpose | Actual behavior |
|---|---|---|---|
| `PARITY_WASM_FATAL` | `internal/ui/game.go:120` | Arm parity panics on WASM | Never fires |
| `PARITY_TIMING_FATAL` | `internal/ui/game_parity_timing.go:56` | Arm timing-gate panics on browser | Never fires |
| `SEQ_TICK_MS` | `internal/ui/game_sequencer_loop.go:19` | Override the 4 ms WASM tick | Unsettable on WASM |
| `PERF_BROWSER_UPDATE_MAX_MS` | `internal/ui/game.go:49` | Browser perf ceiling | Dead in browser |
| `PERF_FAST_PATH` | `internal/ui/game.go:49` | Enable browser fast path | Dead in browser |

This is why `internal/ui/runtime_profile_js.go` had to invent a parallel
mechanism (`window.__beatmoProfileOverride`) — but that covers only the 13
`RuntimeProfile` fields. The other ~82 variables have no browser path at all.

### 2. A hand-maintained registry already exists, and it has drifted

`internal/ui/game_test_registry_test.go:121-150` holds an `envKeys` slice of 29
names that `resetTestEnvAndGlobals` unsets between tests. It has rotted in both
directions:

- **Contains two names with no reader anywhere in the tree:** `DRAW_FRAMEBUFFER`
  and `PREVIEW_DEBUG`.
- **Missing live keys:** `PARITY_TIMING`, `PARITY_TIMING_FATAL`,
  `BEATMO_DEBUG_LAYOUT`, `BEATMO_HEAP_PROBE`, `BEATMO_HEAP_WATCH`, and all 13
  `BEATMO_*` RuntimeProfile override keys. A test that sets any of those leaks
  the value into every later test in the package.

### 3. `CLAUDE.md` is out of sync in both directions

- **37 variables exist in code but are undocumented**, including every
  RuntimeProfile override (`BEATMO_EDGE_CACHE_PAD`, `BEATMO_AUDIO_LOOKAHEAD_SEC`,
  …), every runtime-tuning knob (`BEATMO_GC_PERCENT`, `BEATMO_GOMAXPROCS`,
  `BEATMO_MEMORY_LIMIT_MB`), `PARITY_TIMING`, `PARITY_TIMING_FATAL`,
  `PARITY_SCAN_EVERY`, and `BEATMO_WAV_REFS`.
- **The documented tables interleave game variables with Node/Playwright/Makefile
  variables** with no marking. `BROWSER_JOBS` and `BYPASS_EQ` sit in adjacent
  tables with nothing indicating that one is read by a test runner and the other
  by the audio engine.

### 4. Five duplicated parse helpers, three naming conventions

Parsing is reimplemented five times with subtly different semantics:

| Helper | File | Notes |
|---|---|---|
| `bench.EnvInt` | `internal/bench/env.go` | `0` treated as unset |
| `async.envInt` | `internal/async/runtime.go:134` | identical body, separate copy |
| `ui.envIntDefault` | `internal/ui/parity_scan_config.go:17` | rejects `< 1` |
| `ui.envInt/envFloat/envBool` | `internal/ui/runtime_profile_notjs.go:66-100` | its own truthy set |
| `deploy.envBytes` | `internal/deploy/asset_size_test.go:78` | `*testing.T`-coupled |

Boolean parsing is inconsistent at the call sites too: `PARITY_FATAL` accepts one
set of spellings, `BEATMO_ROW_SNAPSHOTS` accepts `1|true`, `NO_GRID_DRAW` accepts
only `1`, and `BEATMO_DEBUG_LAYOUT` accepts `1|true|TRUE|yes|on`.

Naming splits three ways: `BEATMO_*`-prefixed (48), bare
`DEBUG_*`/`NO_*`/`BYPASS_*`/`TEST_*` (34), domain-prefixed
`PARITY_*`/`SCOPE_*`/`PERF_*`/`PYROSCOPE_*` (13). Several bare names — `SWEEP`,
`WRITE_WAV`, `TEST_TONE`, `TEST_VOICE`, `SINGLE_VOICE`, `UPDATE_GOLDEN` — are
generic enough to collide with unrelated tooling in a developer's shell.

---

## Goals

1. One file lists every environment variable the game reads, each with a purpose
   comment.
2. No `os.Getenv` outside that package.
3. Environment variables work identically on desktop and WASM.
4. Drift is caught by tests, not by review.
5. Collision-prone names are prefixed; dead and duplicated names are removed.

## Non-goals

- **JS/Node harness variables stay in JS.** `BROWSER_JOBS`, `PERF_E2E_*`,
  `WASM_LOAD_*`, `BROWSERSTACK_*`, `STRESS_COMPLEX_*` and ~55 others are read by
  Playwright runners and `Makefile` targets, never by the game. They are out of
  scope for `internal/env`. They gain only a documentation marker (see §5).
- **No general config-file or CLI-flag system.** Existing `flag.*` definitions in
  `cmd/beatmo.go` are unchanged. Where a flag and an env var overlap
  (`-event-log` / `BEATMO_EVENT_LOG`, `-scope-export` / `SCOPE_EXPORT`), the
  current flag-wins precedence is preserved.

---

## Design

### 1. `internal/env` — the table

A leaf package importing only the standard library, so `internal/log`,
`internal/async`, `internal/audio`, and `internal/ui` can all depend on it
without a cycle.

```
internal/env/
  env.go     — Var type, typed constructors, resolution, Reload, All
  vars.go    — THE table: every variable, one declaration each
  env_test.go
```

Each variable is one declaration carrying name, type, default, scope, and
purpose:

```go
var (
    // ---- Runtime configuration ----------------------------------------

    AssetsRoot = Str("BEATMO_ASSETS", "", ScopeRuntime,
        "Override the default assets root directory.")

    DemoConfig = Str("BEATMO_DEMO_CONFIG", "", ScopeRuntime,
        "Path to a JSON circuit to load as the initial demo.")

    // ---- Audio debug: bypasses ----------------------------------------

    BypassEQ = Bool("BEATMO_BYPASS_EQ", false, ScopeDebug,
        "Skip all EQ/processor processing. Isolates mixer vs channel bugs.")
)
```

**Constructors** — `Str`, `Bool`, `Int`, `Float`, `Dur` — replace the five
duplicated helpers. `Bool` has one uniform truthy set (`1`, `true`, `yes`, `on`,
case-insensitive) and one falsy set (`0`, `false`, `no`, `off`); anything else
falls back to the declared default. `Int`/`Float` fall back to the default on a
parse error rather than panicking, matching current behavior everywhere.

**Scope** is one of `ScopeRuntime` (user-facing configuration), `ScopeDebug`
(developer diagnostics, shipped but off), or `ScopeTest` (read only from
`_test.go`). It drives the generated documentation tables and the naming guard.

**Caching.** `Var.Get()` returns a value held in an atomic, resolved on first
access. This preserves today's zero-cost behavior for per-frame readers —
`NO_PIXEL_SNAP`, `RENDER_SAFE`, and the `engine_stop.go` bypasses are read inside
draw and mixing loops and are currently cached in package-level `var`s for
exactly that reason — while removing the fragile init-order coupling of the
`var x = os.Getenv(...) == "1"` idiom.

`env.Reload()` re-resolves every variable from the environment. Called once at
startup and from the test reset hook.

### 2. WASM: populate `go.env`, don't fork the Go code

`wasm_exec.js` already implements the entire mechanism — it serializes
`this.env` into linear memory at `run()`, which is precisely what Go's
`goenvs_unix` reads. The only missing piece is that nothing assigns to it.

The fix is two lines in the single production bootstrap
(`src/js/index.html:842`):

```js
const go = new Go(); // from wasm_exec.js
go.env = { ...(globalThis.__beatmoEnv || {}) };
```

A host page, a Playwright `addInitScript`, or a deploy-time inline script sets
`window.__beatmoEnv = { PARITY_WASM_FATAL: "1" }` before the module loads.

**Why this over a build-tagged `env` source:** `os.Getenv` then behaves
identically on both platforms, so `internal/env` needs no `//go:build js` file
and no parallel lookup path. Every variable in the table works everywhere by
construction, rather than requiring per-variable WASM plumbing.

**Consequence:** `runtime_profile_js.go`'s bespoke `__beatmoProfileOverride`
reader is deleted; the browser profile calls the same `applyEnvProfileOverride`
the desktop profile already uses. `runtime_profile_js.go` and
`runtime_profile_notjs.go` collapse to near-identical builder selectors, which is
what the RuntimeProfile design in CLAUDE.md describes them as being.

`__beatmoProfileOverride` is retired in favor of `__beatmoEnv`. The three browser
tests that set it (`webaudio_bench_startup`, `webaudio_perf_e2e`,
`webaudio_fx_chain_stress`) migrate to `__beatmoEnv` with the corresponding
`BEATMO_*` keys.

### 3. Call-site migration

All 99 sites convert mechanically:

```go
// before
var bypassEQ = os.Getenv("BYPASS_EQ") == "1"
...
if bypassEQ { return }

// after
if env.BypassEQ.Get() { return }
```

The package-level cached `var`s in `internal/audio/engine_stop.go` (9),
`internal/ui/game_draw_grid_pane.go` (7), `internal/audio/channels.go` (2),
`internal/ui/perf.go`, and `internal/ui/widgets.go` are deleted — the `Var` *is*
the cache. The corresponding manual resets in `resetTestEnvAndGlobals`
(`envRenderSafe = false`, `timelineTrace = false`, `debugGeom = false`, …) are
deleted with them, replaced by a single `env.Reload()`.

The five parse helpers are deleted. `internal/bench/env.go` loses `EnvInt`; if
nothing else remains in the package it is removed entirely.

### 4. Guards

All in the fast `-tags test` path, alongside the existing `design_md_drift_test.go`
and `token_discipline_test.go` precedent:

| Guard | Catches |
|---|---|
| `TestNoDirectGetenv` | Any `os.Getenv` / `os.LookupEnv` / `os.Environ` outside `internal/env`. |
| `TestEveryVarDocumented` | A `Var` declared with an empty purpose string. |
| `TestEnvNamingConvention` | A **new** name that is not `BEATMO_`-prefixed. Enforced as a ratchet against a frozen 37-entry legacy allowlist (below) that may only shrink — the same shape as `token_discipline_test.go`'s per-file budget. |
| `TestNoDuplicateEnvNames` | Two `Var`s claiming the same name. |
| `TestCLAUDEMDEnvTablesMatch` | Regenerates the CLAUDE.md environment tables from `env.All()` and diffs; fails on drift in either direction. |

`resetTestEnvAndGlobals` becomes:

```go
for _, v := range env.All() {
    _ = os.Unsetenv(v.Name())
}
env.Reload()
```

which cannot drift, and immediately fixes the missing-key leak described in
Problem 2.

`./beatmo -list-env` prints `name<TAB>type<TAB>default<TAB>scope<TAB>purpose`
for every variable, mirroring the existing `-list-scenes` /
`-list-scene-subjects` convention.

### 5. Documentation

The CLAUDE.md environment tables are **generated** from `env.All()` and pinned by
`TestCLAUDEMDEnvTablesMatch`. They gain a **Read by** column distinguishing
`game` (this registry) from `harness` (Node/Playwright/Makefile). Harness
variables remain hand-maintained in a clearly separated section, since they are
outside `internal/env`.

---

## Removals

Six variables are removed. Clean break — no deprecation aliases, since all are
developer/debug knobs rather than user-facing configuration, and a permanent
alias shadow list would become its own drift source.

| Var | Sites | Rationale | Replacement |
|---|---|---|---|
| `DRAW_FRAMEBUFFER` | `game_test_registry_test.go:126` only | No reader anywhere in the tree. | — |
| `PREVIEW_DEBUG` | `game_test_registry_test.go:141` only | No reader anywhere in the tree. | — |
| `BEATMO_CONFIG` | `demo.go:66`, `demo_stub.go:43` | Pure fallback alias; `BEATMO_DEMO_CONFIG` already takes precedence at both sites. Two names, one job. | `BEATMO_DEMO_CONFIG` |
| `SEQ_TICK_MS` | `game_sequencer_loop.go:19` | Duplicates `BEATMO_SEQUENCER_TICK_MS` in `RuntimeProfile`, which is the documented mechanism for runtime divergence. | `BEATMO_SEQUENCER_TICK_MS` |
| `PERF_FAST_PATH` | `game.go:49` | `os.Getenv("PERF_BROWSER_UPDATE_MAX_MS") != ""` in the same condition already enables the fast path; the second gate adds no reachable behavior. | `PERF_BROWSER_UPDATE_MAX_MS` |
| `BEATMO_HEAP_PROBE` + `BEATMO_HEAP_WATCH` | `heap_growth_probe.go:26,33` | PROBE already implies WATCH (`heapWatchEnabled = heapProbeEnabled \|\| ...`); two booleans encode one three-state level. | `BEATMO_HEAP_DIAG=watch\|probe` |

**`SEQ_TICK_MS` removal note.** The two are not currently wired together — the
sequencer loop reads `SEQ_TICK_MS` directly while `RuntimeProfile.SequencerTickMS`
is set from `BEATMO_SEQUENCER_TICK_MS`. The migration routes
`game_sequencer_loop.go` through `RuntimeProf().SequencerTickMS`, which is the
architecture CLAUDE.md already prescribes for browser/desktop divergence.

---

## Renames

Twelve collision-prone bare names gain the `BEATMO_` prefix. Clean break, no
aliases.

| Old | New |
|---|---|
| `SWEEP` | `BEATMO_KICK_SWEEP` |
| `WRITE_WAV` | `BEATMO_WRITE_WAV` |
| `TEST_TONE` | `BEATMO_TEST_TONE` |
| `TEST_VOICE` | `BEATMO_TEST_VOICE` |
| `TEST_RAW_VOICE` | `BEATMO_TEST_RAW_VOICE` |
| `TEST_RAW_VOICE_FULL` | `BEATMO_TEST_RAW_VOICE_FULL` |
| `TEST_RAW_VOICE_NOSCALE` | `BEATMO_TEST_RAW_VOICE_NOSCALE` |
| `SINGLE_VOICE` | `BEATMO_SINGLE_VOICE` |
| `AUDIO_SAMPLE_RATE` | `BEATMO_AUDIO_SAMPLE_RATE` |
| `PPROF` | `BEATMO_PPROF` |
| `UPDATE_GOLDEN` | `BEATMO_UPDATE_GOLDEN` |
| `RUN_SOAK_REAL` | `BEATMO_RUN_SOAK_REAL` |

**Explicitly not renamed — the frozen legacy allowlist (37).** These are
domain-scoped enough in practice; renaming is churn without a safety gain.
`TestEnvNamingConvention` hard-codes this exact list: a `Var` whose name is not
`BEATMO_`-prefixed and not on the list fails the test, and the list may only
shrink. That encodes the decision so it does not get relitigated per-PR, while
leaving the door open to prefix any of these later.

```
AUDIO_CLIP_DEBUG        DEBUG_MIXER            PARITY_TIMING_FATAL
BPM_TIMING_TEST         FONTCACHE_SUBPROCESS   PARITY_WASM_FATAL
BYPASS_CHANNEL_PROC     IFX_GOLDEN_UPDATE      PARITY_WATCH
BYPASS_EQ               NO_EDGE_CACHE          PERF_BROWSER_UPDATE_MAX_MS
BYPASS_HEADROOM         NO_GRID_DRAW           PERF_LOG
BYPASS_NORMALIZE        NO_GRID_TILE_CACHE     PYROSCOPE_APP
DEBUG_CHANNEL           NO_PIXEL_SNAP          PYROSCOPE_URL
DEBUG_DRAW_NODES        NO_SPRITE_NODES        RENDER_SAFE
DEBUG_GEOM              PARITY_DUMP_STDERR     SCOPE_EXPORT
DEBUG_HISTORY_SEED      PARITY_FATAL           SCOPE_EXPORT_INTERVAL
                        PARITY_SCAN_EVERY      SCOPE_EXPORT_PATH
                        PARITY_SCAN_STRIDE     SCREEN_EDGES
                        PARITY_TIMING          TIMELINE_TRACE
                                               TIMELINE_TRACE_ROW
```

Note that a pure prefix-set guard would *not* work here: after the renames,
`AUDIO_CLIP_DEBUG`, `BPM_TIMING_TEST`, `FONTCACHE_SUBPROCESS`,
`IFX_GOLDEN_UPDATE`, `RENDER_SAFE`, `SCREEN_EDGES`, `TIMELINE_TRACE`, and
`TIMELINE_TRACE_ROW` match no shared prefix. An explicit allowlist is the only
formulation that is both correct today and a ratchet going forward.

---

## Complete inventory — 97 rows (95 read by code + 2 dead names)

Scope key: **R** = runtime configuration, **D** = debug/diagnostic (shipped, off
by default), **T** = test-only (never read by the shipping binary).

### Runtime configuration (6)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `BEATMO_ASSETS` | R | `audio/catalog_desktop.go:39`, `audio/catalog_stub.go:129` | Override the default assets root directory. | keep |
| `BEATMO_DEMO_CONFIG` | R | `ui/demo.go:64`, `ui/demo_stub.go:41` | Path to a JSON circuit loaded as the initial demo. | keep |
| `BEATMO_CONFIG` | R | `ui/demo.go:66`, `ui/demo_stub.go:43` | Fallback alias of the above. | **remove** |
| `BEATMO_RECORDINGS_DIR` | R | `audio/recording_paths.go:14` | Base directory for recording output; defaults to `$CWD/recordings`. | keep |
| `AUDIO_SAMPLE_RATE` | R | `audio/engine.go:22` | Output sample rate in Hz (default 44100). | **rename** |
| `SEQ_TICK_MS` | R | `ui/game_sequencer_loop.go:19` | Sequencer tick duration. | **remove** |

### Logging and event stream (5)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `BEATMO_INFO_LOG` | R | `cmd/beatmo.go:171` | Set to `off` to disable the hooks.Bus → INFO narrative consumer. | keep |
| `BEATMO_EVENT_LOG` | R | `cmd/beatmo.go:143` | Write the full hooks.Bus event stream to this JSONL path. | keep |
| `BEATMO_EVENT_LOG_VERBOSE` | D | `cmd/beatmo.go:145` | Include verbose kinds (drag progress) in the narrative and JSONL stream. | keep |
| `BEATMO_TEST_LOG` | T | `log/log.go:116` | Enable in-process test logger output. | keep |
| `BEATMO_TEST_LOG_LEVEL` | T | `log/log.go:118` | Override log level when test logging is enabled. | keep |

### Profiling and runtime tuning (15)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `PPROF` | D | `cmd/profiling.go:16` | Start the pprof HTTP server on `localhost:6060`. | **rename** |
| `PYROSCOPE_URL` | D | `cmd/pyroscope.go:14` | Pyroscope profiling backend URL. | keep |
| `PYROSCOPE_APP` | D | `cmd/pyroscope.go:18` | Pyroscope app identifier (default `beatmo`). | keep |
| `BEATMO_BLOCK_PROFILE` | D | `async/runtime.go:78`, `cmd/beatmo_record_bench.go:62` | `runtime.SetBlockProfileRate` value. | keep |
| `BEATMO_MUTEX_PROFILE` | D | `async/runtime.go:81`, `cmd/beatmo_record_bench.go:63` | `runtime.SetMutexProfileFraction` value. | keep |
| `BEATMO_MEMORY_LIMIT_MB` | R | `async/runtime.go:69` | Soft memory limit via `debug.SetMemoryLimit`. | keep |
| `BEATMO_GC_PERCENT` | R | `async/runtime.go:72` | `debug.SetGCPercent` value. | keep |
| `BEATMO_GOMAXPROCS` | R | `async/runtime.go:75` | `runtime.GOMAXPROCS` value. | keep |
| `PERF_LOG` | D | `ui/perf.go:14` | Emit perf logs roughly every 2 s. | keep |
| `PERF_FAST_PATH` | D | `ui/game.go:49` | Enable the fast perf path. | **remove** |
| `PERF_BROWSER_UPDATE_MAX_MS` | D | `ui/game.go:49` | Max `Update()` duration for browser perf optimization. | keep |
| `BEATMO_HEAP_PROBE` | D | `ui/heap_growth_probe.go:26` | Per-second allocation-rate probe with top contributors by tag. | **remove** |
| `BEATMO_HEAP_WATCH` | D | `ui/heap_growth_probe.go:33` | 30 s heap-growth rollup WARN. | **remove** |
| `BEATMO_RECORD_BENCH_HEAP_SNAPSHOTS` | D | `cmd/beatmo_record_bench.go:90` | Write heap snapshots during `-record-bench`. | keep |
| `BEATMO_RECORD_BENCH_SYNTH_CHURN_HZ` | D | `cmd/beatmo_record_bench.go:94` | Synth-param mutation rate during `-record-bench`. | keep |

New: `BEATMO_HEAP_DIAG` (`watch`|`probe`) replaces the two heap vars.

### RuntimeProfile overrides (13)

All read in `ui/runtime_profile_notjs.go:22-64`; all currently desktop-only and
all undocumented. Purpose in every case: override the corresponding
`RuntimeProfile` field for bench sweeps without recompiling. All keep their names.

`BEATMO_EDGE_CACHE_PAD` · `BEATMO_GRID_CACHE_PAD` · `BEATMO_DISABLE_NODE_GLOW` ·
`BEATMO_DISABLE_EDGE_ARROWS` · `BEATMO_SIMPLE_DRAW_DEFAULT` ·
`BEATMO_TIMELINE_INFO_THROTTLE_MS` · `BEATMO_AUDIO_LOOKAHEAD_SEC` ·
`BEATMO_AUDIO_BATCH_MAX` · `BEATMO_SEQUENCER_TICK_MS` ·
`BEATMO_ADAPTIVE_PAN_PAD` · `BEATMO_FAST_PAN_DETECT` ·
`BEATMO_PREDICTOR_BACKOFF` · `BEATMO_FORCE_GC_INTERVAL_MS`

### Parity (8)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `PARITY_FATAL` | D | `ui/game.go:80` | `0`/`false` disables parity panics. | keep |
| `PARITY_WATCH` | D | `ui/game.go:101` | Parity mode on desktop: `log` or `panic`. | keep |
| `PARITY_WASM_FATAL` | D | `ui/game.go:120` | Arm parity panics on WASM (`1`/`true`/`panic`). | keep |
| `PARITY_TIMING` | D | `ui/game_parity_timing.go:56` | Timing-gate watch mode. | keep |
| `PARITY_TIMING_FATAL` | D | `ui/game_parity_timing.go:56` | Arm timing-gate panics on browser. | keep |
| `PARITY_DUMP_STDERR` | D | `ui/game_parity_state.go:387`, `ui/game_parity_timing.go:274` | Dump parity error messages to stderr. | keep |
| `PARITY_SCAN_EVERY` | D | `ui/parity_scan_config.go:14` | Frames between parity scan passes (default 4). | keep |
| `PARITY_SCAN_STRIDE` | D | `ui/parity_scan_config.go:15` | Cells per parity scan pass (default 1). | keep |

### Audio debug (13)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `TEST_TONE` | D | `audio/engine_stop.go:81` | Output a pure 440 Hz sine, bypassing all synth. Tests the Oto/driver path. | **rename** |
| `TEST_VOICE` | D | `audio/engine_stop.go:86` | Replace the C synth with simple 220 Hz sine voices. Tests the voice/mixer path. | **rename** |
| `TEST_RAW_VOICE` | D | `audio/engine_stop.go:65` | Output the first voice's raw samples, bypassing all processing. | **rename** |
| `TEST_RAW_VOICE_FULL` | D | `audio/engine_stop.go:73` | With the above, use full amplitude instead of 0.2×. | **rename** |
| `TEST_RAW_VOICE_NOSCALE` | D | `audio/engine_stop.go:77` | With the above, skip all multiplication; write direct to int16. | **rename** |
| `SINGLE_VOICE` | D | `audio/engine_stop.go:69` | Limit the mixer to one voice at a time. Tests multi-voice accumulation. | **rename** |
| `BYPASS_CHANNEL_PROC` | D | `audio/engine_stop.go:54` | Skip all channel processing (volume, EQ). Isolates mixer vs channel. | keep |
| `BYPASS_HEADROOM` | D | `audio/engine_stop.go:61` | Skip per-voice headroom attenuation (0.25×). | keep |
| `BYPASS_EQ` | D | `audio/channels.go:15` | Skip all EQ/processor processing. | keep |
| `BYPASS_NORMALIZE` | D | `audio/export.go:12` | Skip peak normalization during export. | keep |
| `AUDIO_CLIP_DEBUG` | D | `audio/engine_stop.go:50` | Log clipping events (samples exceeding ±1.0 before hard clamp). | keep |
| `DEBUG_MIXER` | D | `audio/engine_stop.go:57` | Log mixer workBuf min/max and voice stats periodically. | keep |
| `DEBUG_CHANNEL` | D | `audio/channels.go:18` | Log channel processing values. | keep |

### Render and UI debug (11)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `RENDER_SAFE` | D | `ui/game_draw_grid_pane.go:16` | Force `drawRect`-based rendering instead of the sprite cache, for pixel testing. | keep |
| `SCREEN_EDGES` | D | `ui/game_draw_grid_pane.go:17` | Draw screen-space edge arrows at viewport edges. | keep |
| `NO_GRID_DRAW` | D | `ui/game_draw_grid_pane.go:18` | Skip grid pane drawing entirely. | keep |
| `NO_GRID_TILE_CACHE` | D | `ui/game_draw_grid_pane.go:19` | Disable grid tile caching; force redraw every frame. | keep |
| `NO_PIXEL_SNAP` | D | `ui/game_draw_grid_pane.go:20` | Disable pixel snapping, for geometry diagnostics. | keep |
| `NO_EDGE_CACHE` | D | `ui/game_draw_grid_pane.go:21` | Disable the edge cache; force redraw every frame. | keep |
| `NO_SPRITE_NODES` | D | `ui/game_draw_grid_pane.go:22` | Disable node sprite rendering. | keep |
| `DEBUG_GEOM` | D | `ui/camera.go:80`, `ui/widgets.go:57`, `ui/game_new.go:265` | Verbose geometry logs. | keep |
| `DEBUG_DRAW_NODES` | D | `ui/game_new.go:265` | Detailed node draw logs. | keep |
| `BEATMO_DEBUG_LAYOUT` | D | `ui/layout_profile.go:14` | Draw developer-only layout guide ticks between widget surfaces. | keep |
| `BEATMO_DEBUG_INST` | D | `ui/drumview_instruments.go:163` | Debug logging for instrument menu operations. | keep |

### Timeline, sequencer, and synth debug (7)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `TIMELINE_TRACE` | D | `ui/game.go:54` | Trace timeline masking logic. | keep |
| `TIMELINE_TRACE_ROW` | D | `ui/game.go:59` | Which row to trace (default 0). | keep |
| `DEBUG_HISTORY_SEED` | D | `ui/game_graph_update_beat_infos.go:286,403` | Debug timeline history seeding logic. | keep |
| `BEATMO_ROW_SNAPSHOTS` | D | `ui/game_new.go:125` | Enable row snapshot mode. | keep |
| `BPM_TIMING_TEST` | D | `ui/game_new.go:258` | Enable BPM timing test mode. | keep |
| `BEATMO_SYNTH_DISPATCH_DEBUG` | D | `audio/synth_recipe_dispatch.go:19` | Log which dispatch path each voice took (recipe vs legacy), the merged paramsHash, and the instrument id. Diagnoses "the slider does nothing" reports. | keep |
| `BEATMO_PREWARM_DEBUG` | D | `audio/synth_recipe_dispatch.go:25` | Log voice-cache HIT/RENDER for the pitched path with the full cache key, exposing warm→dispatch key mismatches. | keep |

### Scope export (3)

| Var | Scope | Site | Purpose | Action |
|---|---|---|---|---|
| `SCOPE_EXPORT` | D | `cmd/beatmo.go:182`, `audio/engine.go:170` | Enable the scope export flight recorder (JSONL). | keep |
| `SCOPE_EXPORT_PATH` | D | `audio/engine.go:178` | Output file (default `scope_export.jsonl`). | keep |
| `SCOPE_EXPORT_INTERVAL` | D | `audio/engine.go:172` | Snapshot interval in seconds (default 2). | keep |

### Test-only (14)

| Var | Site | Purpose | Action |
|---|---|---|---|
| `BEATMO_WAV_REFS` | `wavref/wavref.go:24` | Directory of local, uncommitted reference WAVs; tests skip cleanly when unset. | keep |
| `BEATMO_DIAG_WAV` | `audio/template_audio_quality_test.go:378` | Dump diagnostic WAVs from the template audio report. | keep |
| `BEATMO_RTF_MIN` | `audio/render_rtf_test.go:24` | Minimum acceptable render real-time factor (default 20). | keep |
| `BEATMO_UPDATE_ORACLE` | `audio/legacy_oracle_golden_test.go:197` | Regenerate the legacy oracle golden. | keep |
| `UPDATE_GOLDEN` | `eventlogger/golden_session_test.go:98` | Regenerate the eventlogger session golden. | **rename** |
| `IFX_GOLDEN_UPDATE` | `audio/ifx_xplat_golden_test.go:179` | Regenerate the insert-FX cross-platform golden. | keep |
| `RUN_SOAK_REAL` | `ui/soak_heap_bound_real_test.go:18` | Opt into the long real-Ebiten heap soak test. | **rename** |
| `FONTCACHE_SUBPROCESS` | `ui/fontcache_visibility_test.go:47` | Internal marker identifying the font-cache test subprocess. | keep |
| `SWEEP` | `audio/kick_refs_tune_test.go:121,160`, `audio/zgump_kick_tune_test.go:283` | Run the kick parameter sweep. | **rename** |
| `WRITE_WAV` | `audio/kick_refs_tune_test.go:143,184`, `audio/zgump_kick_tune_test.go:231,333` | Write tuning output WAVs to disk. | **rename** |
| `BEATMO_VERIFY_DEPLOY_URL` | `deploy/headers_verify_test.go:232` | Live deploy target for the post-deploy header probe. | keep |
| `BEATMO_WASM_RAW_MAX_BYTES` | `deploy/asset_size_test.go:168` | Raw `main.wasm` size budget. | keep |
| `BEATMO_WASM_GZIP_MAX_BYTES` | `deploy/asset_size_test.go:169` | Gzipped `main.wasm` size budget. | keep |
| `BEATMO_ASSETS_GZIP_MAX_BYTES` | `deploy/asset_size_test.go:170` | Total gzipped asset budget. | keep |

### Dead (2)

| Var | Site | Action |
|---|---|---|
| `DRAW_FRAMEBUFFER` | `ui/game_test_registry_test.go:126` | **remove** |
| `PREVIEW_DEBUG` | `ui/game_test_registry_test.go:141` | **remove** |

---

## Behavior changes

This is not a pure refactor. Three intended behavior changes:

1. **95 variables go from inert to live on WASM.** Defaults are unchanged, so
   nothing fires unless deliberately set — but a browser session that sets
   `PARITY_WASM_FATAL=1` or `PARITY_TIMING_FATAL=1` via `__beatmoEnv` will now
   actually panic. That is the point of the change; it is called out here so it
   is not a surprise.
2. **Boolean parsing is unified.** Variables that previously accepted only `1`
   (e.g. `NO_GRID_DRAW`) now also accept `true`/`yes`/`on`. This is strictly
   permissive; no previously-working value stops working.
3. **Renamed and removed variables stop working under their old names**, with no
   alias period.

## Risks

| Risk | Mitigation |
|---|---|
| A rename or removal breaks an uncommitted local script. | The full old→new mapping is in this document; `-list-env` prints the current set. Clean break was an explicit decision. |
| `go.env` population breaks WASM boot if `__beatmoEnv` is malformed. | The spread defaults to `{}`; `wasm_exec.js` stringifies values. A non-object `__beatmoEnv` is guarded by the `|| {}` fallback. |
| Init-order change: `Var.Get()` resolves lazily rather than at package init. | `env.Reload()` at startup makes resolution eager in production; tests call it after `Setenv`. Behavior for any single reader is unchanged. |
| The `TestNoDirectGetenv` guard is too strict for `internal/env` itself or for `cmd` bootstrapping. | The guard exempts `internal/env` by path, exactly as `token_discipline_test.go` exempts `theme.go`. |

## Verification

- `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
- `make test-real` — real-Ebiten paths that read render-debug vars.
- `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` —
  confirms the `go.env` change does not perturb boot.
- `GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_perf_e2e.browser.test.js` —
  the migrated `__beatmoProfileOverride` → `__beatmoEnv` path.
- `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_load_startup.browser.test.js` —
  boot waterfall unaffected.
- A new browser test asserting `__beatmoEnv` actually reaches Go: set a known
  var, read it back through an existing JS export.
- No UI surface changes, so no screenshot pass is required.
