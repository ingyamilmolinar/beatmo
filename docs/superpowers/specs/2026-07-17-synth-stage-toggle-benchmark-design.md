# Synth stage-toggle & live-control benchmark + profiling — design

Date: 2026-07-17
Branch: add-core-node-types
Status: design (pending implementation plan)

## Problem

User report: while playing, toggling synth-pipeline **stage enable pills** (FILTER
on/off, LFO on/off, …) on the Synth tab corrupts audio — the reported symptom is
**crackle / dropouts** (buffer underrun class, "B"), with FPS/rendering lag also
suspected. The user wants (a) new benchmark behaviors that exercise stage toggles
**and** live control changes on *enabled* stages, mutated in real time, on both
desktop and WASM (WASM prioritized — single thread), and (b) a **profiling pass**
that attributes the corruption to a concrete cause before any fix.

Constraints carried from the brief:
- **Evidence before change.** Baseline-profile and reproduce with numbers *before*
  optimizing anything. Every fix must show a before/after delta from these benches.
- **Production-faithful tests.** Real `main.wasm` + real `AudioContext` + real
  Synth-tab pill clicks for the UI half; no synthetic stubs where a real path exists.

## Key finding that shapes the design

A stage-enable toggle takes the **exact same audio-layer path as a scalar knob**:
`toggleSynthStage` → `audio.SetInstrumentParam(id, "filter_enabled", 0|1)` → same
`setInstrumentParam` JS export → same `paramsHash` change → same voice-cache keying.
The C engine reads `filt_on`/`lfo_on`/etc. as ordinary params and skips a disabled
stage — **no pipeline rebuild, no re-binding**. (`modular.c:710-741`,
`js_exports_synth_recipe.go:52`, `synth_recipe.go:221`.)

Therefore the toggle's *extra* cost over a knob is **above the DSP layer**, and this
is where symptom B most likely lives:
1. Each toggle also fires `commitInstrumentParams` (persisted commit: undo journal +
   export state) and `requestSynthMirror` (the `synth.preview` pool) — a mid-drag
   scalar tick often skips the commit.
2. On WASM, **every** param push runs `_updateInstrumentParams` →
   `bumpInstrumentParamsGen` + `delete samples[id]` + `rawRenderCache.delete(id)`
   (`audio.js:2818`) on the single thread; melodic instruments also drop every
   per-pitch variant via `evictRenderCache` on the restore branch.
3. It all happens **while the Synth tab is drawing** — the heaviest draw in the app;
   a prior investigation (`project_synth_tab_audio_starvation`) measured master RMS
   collapsing 50–150× under software GL purely from Synth-tab draw cost.

**Consequence for test design:** since toggle ≡ scalar at the DSP layer, a
**bridge-only bench cannot reproduce symptom B** — the damage is commit + mirror +
draw cost on the *real* Synth tab, which the bridge harness never exercises. This is
why we need **two harness families**, and the **delta between them is the diagnosis.**

## Approach (chosen: two-harness attribution, desktop + WASM)

### Harness A — bridge-only (WASM), extend `webaudio_render_latency_bench`

Add scenarios to the existing `src/js/webaudio_render_latency_bench.browser.test.js`
(drives `play_ui.wasm` via the JS bridge; already plumbs `getRenderLatencyMetrics`,
schedule metrics, and 8 ms event-loop gap sampling). New scenarios reuse the existing
`runScenario` shape (note cadence + spam timer):

| Scenario | id | spam | Purpose |
|---|---|---|---|
| `stage-toggle-storm` | cello | cyclic `filter_enabled` 1↔0 @ spam rate | Toggle-as-scalar churn: 2 alternating param hashes; measures render/defer under toggling |
| `stage-toggle-multi` | cello | rotate `filter_enabled`/`lfo_enabled`/`filtenv_enabled` on/off | Many distinct hashes → more cold renders; worst-case cache thrash |
| `enabled-ctrl-sweep` | cello | continuous `filter_cutoff` monotone sweep on an **enabled** FILTER | Real-time control on a live stage; leading-edge cold-render (monotone = worst for the LRU-key model) |
| `enabled-ctrl-cyclic` | cello | `lfo_rate` triangle sweep | Cyclic control drag (values revisit → cache warms) — contrast vs monotone |
| `combined-toggle+ctrl` | cello | toggle `filter_enabled` **and** sweep `filter_cutoff` together | The realistic "fiddling with the panel" storm |

Representative param keys (from `modular_stage_params.go`): FILTER `filter_cutoff`,
`filter_resonance`; LFO `lfo_rate`, `lfo_depth`; ENVELOPE `amp_attack`. Enable keys:
`filter_enabled`, `lfo_enabled`, `filtenv_enabled`, `osc_enabled`, `fm_enabled`.

Metrics recorded per scenario (already available): `renders`, `workerRenders`,
`mainThreadRenders`, `renderMs avg/p90/max`, `deferredPlays`, `deferMs`,
`deferAbsMsP90`, `workerQueuePeak`, `stalePlays`, `neighborPlays`, schedule
`overdue`/`smallLead`/`minLead`, and `maxGapMs`/`p90GapMs` (main-thread block proxy).
Add a `paramPushes` counter so renders/push and defers/push are derivable.

Keep the existing CPU-throttle variant idea: run the combined scenario at 4× CDP CPU
throttle to emulate a mobile single core.

### Harness B — real-UI (WASM), NEW `synth_stage_toggle_ui_bench.browser.test.js`

Drives the **full `main.wasm`** with real input (via `real_input_test_helpers.js`
`buildMainWasm` + audio unlock gesture). This is the only harness that reproduces
symptom B because it opens the Synth tab and clicks real pills.

Flow: build main.wasm → unlock audio (real `page.mouse.click`) → load a melodic demo
→ `startPlay()` → `setEQTab("synth")` → for the measure window, dispatch **real
pointer clicks** at the enable affordance of a stage chip (located via
`synthChipRects()` → `{name,x,y,w,h,enabled,selected}`) and **real horizontal knob
drags** on an enabled stage's control (`selectSynthSection` + `synthKnobRects`,
horizontal axis per the knob-vs-scroll lock).

Measured while the tab is open and being toggled:
- **Audio health:** in-graph AnalyserNode RMS (detects the starvation collapse), plus
  `getAudioScheduleMetrics()` overdue/smallLead — a rising overdue/underrun count is
  symptom B in numbers.
- **Frame health:** `perfStats()` FPS + avg/max draw ms, and an event-loop gap sampler
  (same 8 ms `setInterval` delta technique) to catch main-thread stalls that block
  both draw and the WebAudio callback pump.
- **Render health:** `getRenderLatencyMetrics()` (same counters as Harness A) so the
  UI-half numbers are directly comparable to the bridge half.

A **control scenario** (identical toggling, but `setEQTab("eq")` — cheap tab — before
the measure window) isolates draw cost: if audio is healthy on EQ and sick on Synth
with the *same* param churn, the draw path is the culprit, not the DSP.

> ⚠ **Gating policy:** headless Chromium falls back to software GL, which *amplifies*
> draw cost beyond a real machine. Harness B's wall-clock numbers (FPS, draw ms, RMS
> collapse ratio) are **recorded and profiled, not hard-gated**. Only **structural
> invariants** gate CI (see Gates). This keeps the bench honest about attribution
> without flaking on CI GPU variance.

### Harness C — desktop, extend the `-bench-bpm` loop

The desktop bench (`game_update.go:88-157`, run via `scripts/bench-desktop.sh` →
`cmd -bench-bpm -bench-secs -bench-prof`) currently only overrides BPM and plays the
static demo — **no param mutation hook**. Add an **opt-in param-mutation schedule**
gated by a new flag/env (e.g. `-bench-synth-storm` / `BENCH_SYNTH_STORM=…`) that,
during the bench window, calls `audio.SetInstrumentParam` on a fixed cadence to
replay the same scenarios (toggle-storm, enabled-ctrl-sweep, combined). Default off,
so existing desktop bench output is unchanged.

Desktop already emits `[bench] RenderLatency` (`audio.GetRenderLatencyMetrics()`) and
the machine-readable `[bench_json]` line (FPS, update/draw ms, overdue, GC, heap).
Add the new counters (paramPushes, renders/push) to that line. `-bench-prof` gives a
pprof CPU profile for the storm — this is the desktop profiling deliverable.

## Profiling deliverables (evidence-first)

1. **Baseline run of all three harnesses**, results committed to `bench-results/`:
   - `bench-results/render_latency.json` gains the new scenarios (Harness A).
   - `bench-results/synth_stage_toggle_ui.json` new (Harness B): per-scenario RMS,
     FPS, draw ms, gaps, render/schedule metrics, Synth-vs-EQ control delta.
   - `bench-results/desktop_synth_storm.*` + `cpu_<bpm>.prof` (Harness C pprof).
2. A short written **attribution note** (in the same bench-results dir or the memory
   system) stating, from the numbers, which layer dominates the corruption: DSP
   render/defer (Harness A hot), commit+mirror (delta A→B without draw), or Synth-tab
   draw cost (Harness B Synth-vs-EQ delta). **No fix is designed until this note
   exists** — per the evidence-before-change constraint.
3. Only *after* attribution, a follow-up (separate spec/plan) proposes the fix and
   uses these same benches to prove the before/after delta.

## Testing / gates

Structural invariants only (machine-independent), matching the existing bench's
philosophy:
- **Harness A:** no page errors; `mainThreadRenders == 0` for every synth scenario
  (renders must stay off-thread — the invariant from `project_synth_param_rerender_storm`);
  `paramPushes > 0` (scenario actually spammed). `RENDER_BENCH_STRICT=1` additionally
  gates `deferAbsMsP90` for the new storm scenarios at the existing `DEFER_P90_MAX_MS`.
- **Harness B:** no page errors; AudioContext reaches `running`; the toggle path
  executes without throwing (real pills clicked, `enabled` state observed to flip in
  `synthChipRects()`); audio not permanently silenced after the storm (RMS recovers on
  tab-leave — proves it's starvation, not a dead instrument). Wall-clock perf recorded,
  not gated.
- **Harness C:** desktop bench with storm flag runs to completion and emits the new
  `[bench_json]` fields; existing default-path output byte-shape unchanged when the
  flag is off (a `-tags test` guard test asserts the new flag defaults off).

Fast-path Go coverage for the new desktop hook: a `-tags test` unit test drives the
bench param-mutation schedule against a fake `SetInstrumentParam` to assert cadence &
scenario selection, so the schedule logic is testable without a 15 s Ebiten run.

## Files touched (anticipated)

- `src/js/webaudio_render_latency_bench.browser.test.js` — add scenarios + `paramPushes`.
- `src/js/synth_stage_toggle_ui_bench.browser.test.js` — NEW (Harness B).
- `src/go/internal/ui/game_update.go` — bench param-mutation schedule (opt-in).
- `src/go/cmd/beatmo.go` — new `-bench-synth-storm` flag; `internal/ui/demo.go` /
  `game_struct.go` bench-state fields.
- `scripts/bench-desktop.sh` — optional storm pass; `Makefile` bench targets
  (`bench-synth-storm` convenience target).
- `src/go/internal/audio/render_latency.go` — add `paramPushes` counter if not
  derivable from existing fields.
- Possibly a tiny JS export for the stage enable-pill **sub-rect** if clicking a chip
  only selects it rather than toggling (TBD in plan — verify against `toggleSynthStage`
  hit geometry first).

## Out of scope

- The actual performance fix. This spec delivers **benchmarks + profiling +
  attribution only.** The fix is a follow-up, justified by these numbers.
- New DSP stages or UI redesign.

## Open implementation questions (resolve in plan, not now)

1. Does a real click on a `synthChipRects()` chip rect **toggle** the stage or only
   **select** it? If select-only, we need the enable-pill sub-rect. Verify against the
   chip hit geometry in `synth_panel_zone.go` before writing Harness B.
2. Which demo circuit for Harness B — reuse the startup demo (has melodic rows) or a
   purpose-built minimal 1-melodic-row circuit for a tighter signal? Prefer the
   smallest circuit that still shows the collapse, for a cleaner attribution delta.
