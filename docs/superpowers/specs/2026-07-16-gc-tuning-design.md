# GC Instrumentation & Tuning — Design

**Date:** 2026-07-16
**Status:** Approved (design), execution in progress
**Convention:** All changes left **uncommitted** (project rule: never `git add`/`commit`).

## Goal

Make garbage collection contribute to *smooth audio and consistent FPS* rather than
occasional stop-the-world (STW) hitches. Instrument the GC on both desktop and WASM,
run long benchmarks (many GC cycles per config), sweep the parameters we control, and
apply the winning values as new defaults.

## Background / key facts

- Toolchain: **go1.26.5**, Ebitengine **v2.9.9**. Green Tea GC is default-on (no opt-out set).
- WASM (`GOOS=js GOARCH=wasm`) runs **single-threaded with no concurrent/background GC**:
  every collection is effectively a STW pause on the one thread that also services audio.
  → "consistent, not STW" on WASM is a *scheduling* problem: prefer many small,
  well-timed collections over rare large ones.
- Desktop has a real background GC thread → pauses are typically sub-ms; the concern is
  pause *duration* and heap high-water, not scheduling.
- Go does **not** use WebAssembly 3.0's WasmGC (golang/go#63904 Unplanned); it ships its
  own GC in linear memory. Nothing to leverage there — this work targets the Go runtime GC.

## Knobs we control

| Knob | API | Sweep via | Current WASM | Current desktop |
|---|---|---|---|---|
| `GOGC` | `debug.SetGCPercent` | `BEATMO_GC_PERCENT` | 50 | Go default (100) |
| Soft mem limit | `debug.SetMemoryLimit` | `BEATMO_MEMORY_LIMIT_MB` | 1500 MB | off |
| `ForceGCInterval` | cooperative `runtime.GC()` in `heapProbeTick` | `BEATMO_PROFILE_OVERRIDE` (browser) / `BEATMO_*` (desktop) | 30 s | 0 (off) |

## Tuning strategy (approach A — chosen)

Tune the *values* of the existing fixed-interval mechanism. Low `GOGC` shrinks the heap so
each GC is tiny; a short `ForceGCInterval` guarantees those tiny GCs land in quiet windows.
Go 1.26's smaller heap-increments + Green Tea make small-heap operation cheaper, so this
lever is stronger than when 30 s / GOGC=50 were chosen.

- Approach B (event-aligned GC after the audio callback) held as a follow-up only if A can't hit target.
- Approach C (pacer-only, no forced GC) used as the **control arm** to re-confirm on go1.26 that
  the single-threaded pacer still fails to self-trigger under sustained audio load.

## Instrumentation

- GC snapshot helper reading `runtime/metrics` `/gc/pauses:seconds` (→ p50/p99/max) plus
  `ReadMemStats` (`NumGC`, `NumForcedGC`, `HeapAlloc`, `HeapSys`, `NextGC`).
- ~1 s periodic sampler → time series of HeapAlloc + interval GC count (growth-under-load,
  pause clustering).
- Surfaced in: desktop `[BENCH_JSON]`, `perfStats()` WASM export, and `PERF_LOG`.
- **WASM caveat:** browser `performance.now()` is clamped → sub-ms pause durations are coarse.
  On WASM the ground truth for "smooth" is audio overdue / small-lead / queue-latency
  (`bench-perf-e2e`) and drawMaxMS / FPS-min (`bench-browser`); GC counters + heap high-water
  are the *mechanism*. On desktop the `/gc/pauses` histogram is trusted directly.

## Parameter grid

- **Desktop** (4 configs × BPM levels, long runs): `GOGC ∈ {50,100,200}` + one `GOMEMLIMIT` config.
- **WASM** (6 targeted configs): `ForceGCInterval ∈ {off,30s,10s,4s}` × `GOGC ∈ {30,50,100}`,
  bracketing "small-heap+frequent" vs "large-heap+timed", plus the forceGC-off control.
  `SetMemoryLimit` fixed at 1500 MB (safety cap) in all arms. 1–2 representative BPMs.
- Runs are **extended** (≥60 s desktop, long WASM soak of 60–90 s at the winner) to guarantee
  multiple GC cycles per config — short runs under-sample GC and make pause stats meaningless.

## Objective (winner selection — lexicographic)

1. Audio overdue / underruns ≈ 0 (hard gate).
2. Maximize FPS-min / minimize drawMaxMS spikes.
3. Heap high-water under ceiling.
Ties → favor more frequent, smaller GC.

## Deliverable

- Apply winning values to `runtime_defaults_wasm.go` (GOGC, memlimit) and
  `runtime_profile.go` (`ForceGCInterval`) — uncommitted.
- `bench-results/gc_tuning_2026-07.md` with full evidence tables + before/after at the winner.
