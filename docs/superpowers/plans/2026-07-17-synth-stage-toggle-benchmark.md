# Synth stage-toggle & live-control benchmark + profiling — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add benchmark behaviors + profiling that reproduce and attribute the audio corruption (crackle/dropouts + FPS lag) caused by toggling synth-pipeline stage-enable pills and changing controls on enabled stages in real time, on both WASM (priority) and desktop.

**Architecture:** Three harnesses whose *deltas* do the diagnosis. **A** (bridge-only WASM) extends the existing `webaudio_render_latency_bench` to isolate pure DSP/render cost. **B** (real-UI WASM) is a new bench driving full `main.wasm` with real Synth-tab pill clicks + knob drags — the only harness that pays the commit + `synth.preview` mirror + Synth-tab draw cost, so the only one that can reproduce the crackle symptom. **C** (desktop) adds an opt-in param-mutation schedule to the `-bench-bpm` loop with pprof. The deliverable is a written attribution note from baseline numbers; **no fix is designed here** — evidence first.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), GOOS=js/wasm, Playwright (Chromium), WebAudio, existing `getRenderLatencyMetrics`/`getAudioScheduleMetrics`/`perfStats` instrumentation.

## Global Constraints

- **Bundled Go only:** `.tools/go/bin/go` (avoid version drift). Absolute `GO=` for browser tests: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- **Never stage or commit** (project convention `feedback_never_stage_or_commit`). Every "checkpoint" step below means *stop and let the reviewer inspect the uncommitted diff* — do **not** run `git add`/`git commit`. This overrides the writing-plans default commit step.
- **Evidence before change:** this plan produces benchmarks + a baseline profiling run + an attribution note ONLY. It designs no performance fix.
- **Production-faithful tests:** Harness B uses real `main.wasm`, real `AudioContext` (unlocked by a real `page.mouse.click`), real pointer clicks on real pill/knob rects — never synthetic stubs.
- **Structural invariants gate CI; wall-clock is recorded, not gated.** Headless Chromium is software-GL and amplifies draw cost — its FPS/draw-ms/RMS numbers are for attribution only.
- **Stage-enable ≡ scalar at the DSP layer:** a toggle is `audio.SetInstrumentParam(id, "<stage>_enabled", 0|1)` through the same `setInstrumentParam` export as a knob. Extra toggle cost is `commitInstrumentParams` + `requestSynthMirror` + draw, all above the audio layer.
- **Keep off-thread invariant:** `mainThreadRenders == 0` must hold for every synth scenario (from `project_synth_param_rerender_storm`).
- **Keep default output unchanged:** the desktop bench param-mutation schedule is opt-in (default off); existing `[bench_json]` shape must be a superset (added fields only).

**Canonical param keys** (verified in `modular_stage_params.go` / `synth_param_schema.go`):
- Enable flags: `filter_enabled`, `lfo_enabled`, `filtenv_enabled`, `osc_enabled`, `fm_enabled` (osc/fm/env/filter/drive default ON; lfo/filtenv/pitchenv/burst/formant default OFF).
- Enabled-stage controls: FILTER `filter_cutoff`, `filter_resonance`; LFO `lfo_rate`, `lfo_depth`; ENVELOPE `amp_attack`.

---

## File Structure

- `src/go/internal/ui/js_exports_eq_widgets.go` — **modify**: add `synthEnablePillRect()` export (Harness B needs the detail-header enable-pill rect to click).
- `src/js/wasm_bridge_smoke.browser.test.js` — **modify**: add catalogue entry for the new export (drift guard).
- `src/js/webaudio_render_latency_bench.browser.test.js` — **modify**: Harness A scenarios + `paramPushes`.
- `src/js/synth_stage_toggle_ui_bench.browser.test.js` — **create**: Harness B (real-UI).
- `src/go/internal/ui/game_struct.go` — **modify**: bench storm-schedule state fields.
- `src/go/internal/ui/demo.go` — **modify**: `RunBenchmark` gains a storm-config param.
- `src/go/internal/ui/game_update.go` — **modify**: drive `audio.SetInstrumentParam` on schedule during the bench window; count pushes into `[bench_json]`.
- `src/go/internal/ui/bench_synth_storm.go` — **create**: pure schedule logic (scenario → per-tick param mutation), unit-testable without Ebiten.
- `src/go/internal/ui/bench_synth_storm_test.go` — **create**: fast `-tags test` unit test for the schedule.
- `src/go/cmd/beatmo.go` — **modify**: `-bench-synth-storm` flag, plumbed to `RunBenchmark`.
- `scripts/bench-desktop.sh` — **modify**: optional storm pass.
- `Makefile` — **modify**: `bench-synth-storm` convenience target.

---

### Task 1: `synthEnablePillRect()` JS export (Harness B toggle target)

Clicking a synth **chip** only *selects* the stage (`synthChipHitAdapter.OnPress`). The enable toggle lives in the detail-header pill routed by `synthToggleHitAdapter`, whose rect is `dv.synthDetailEnablePillRect()` (`synth_panel_zone.go:3798`). Harness B must click that rect via real input, so expose it.

**Files:**
- Modify: `src/go/internal/ui/js_exports_eq_widgets.go` (near the `synthChipRects` block, ~line 139)
- Modify: `src/js/wasm_bridge_smoke.browser.test.js` (~line 170, next to `synthChipRects`)

**Interfaces:**
- Consumes: existing `(*DrumView).synthDetailEnablePillRect() image.Rectangle` and `g.drum` (`*DrumView`).
- Produces: JS global `synthEnablePillRect()` → `{x,y,w,h}` (all `int`) for the currently selected stage's enable pill; empty object `{x:0,y:0,w:0,h:0}` when no stage is selected or the pill is not interactive.

- [ ] **Step 1: Write the failing catalogue assertion**

Add to the catalogue array in `src/js/wasm_bridge_smoke.browser.test.js` right after the `selectSynthSection` entry (~line 171):

```javascript
    // Detail-header enable-pill rect for the selected stage; Harness B clicks
    // its centre to toggle a pipeline stage via real input.
    { name: "synthEnablePillRect", args: [], returns: "any" },
```

- [ ] **Step 2: Run the drift guard to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestJSExportCatalogueDrift ./internal/ui/`
Expected: FAIL — catalogue lists `synthEnablePillRect` but no Go registration exists.

- [ ] **Step 3: Add the Go export**

In `src/go/internal/ui/js_exports_eq_widgets.go`, immediately after the `synthChipRects` registration block, add:

```go
	// synthEnablePillRect() – the detail-header enable pill rect {x,y,w,h}
	// for the currently selected pipeline stage. Empty (all zero) when no
	// stage is selected or the pill is non-interactive. Browser benches
	// click its centre to toggle a stage via real input (the chip itself
	// only selects; the pill toggles).
	js.Global().Set("synthEnablePillRect", jsFn(func(args jsArgs) any {
		obj := js.Global().Get("Object").New()
		var r image.Rectangle
		if g.drum != nil {
			r = g.drum.synthDetailEnablePillRect()
		}
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
```

(If `image` is not yet imported in this file, add `"image"` to its import block.)

- [ ] **Step 4: Run the drift guard to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestJSExportCatalogueDrift ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verify it compiles for the WASM target**

Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: exit 0 (no build errors).

- [ ] **Step 6: Checkpoint** — stop for review of the uncommitted diff. Do NOT commit.

---

### Task 2: Harness A — bridge-only stage-toggle & control scenarios

Extend the existing render-latency bench. It already drives `play_ui.wasm`, plumbs `getRenderLatencyMetrics`/`getAudioScheduleMetrics`, and samples event-loop gaps. We add spam modes and scenarios, and count param pushes in-test.

**Files:**
- Modify: `src/js/webaudio_render_latency_bench.browser.test.js`

**Interfaces:**
- Consumes: existing `runScenario(cfg)` (reads `cfg.spam`, `cfg.pitched`, `cfg.prewarm`, `cfg.noteMs`), `window.setInstrumentParam(id, name, value)`, `window.getRenderLatencyMetrics()`.
- Produces: new `results[name]` rows for `stage-toggle-storm`, `stage-toggle-multi`, `enabled-ctrl-sweep`, `enabled-ctrl-cyclic`, `combined-toggle+ctrl`, each carrying an added `paramPushes` field. Persisted into `bench-results/render_latency.json` under `scenarios`.

- [ ] **Step 1: Add the failing structural gate first**

At the bottom of the file, in the `// ── Gates ──` section (after the `warm` gate), add:

```javascript
// New synth-storm scenarios must actually spam params and keep renders
// off-thread (the project_synth_param_rerender_storm invariant).
for (const name of ["stage-toggle-storm", "stage-toggle-multi", "enabled-ctrl-sweep", "enabled-ctrl-cyclic", "combined-toggle+ctrl"]) {
  const r = results[name];
  if (!r) { failures.push(`${name}: scenario missing from results`); continue; }
  if ((r.paramPushes || 0) <= 0) failures.push(`${name}: expected paramPushes>0, got ${r.paramPushes}`);
  if ((r.mainThreadRenders || 0) > 0) failures.push(`${name}: ${r.mainThreadRenders} render(s) on the main thread (must stay off-thread)`);
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/webaudio_render_latency_bench.browser.test.js`
Expected: FAIL — `stage-toggle-storm: scenario missing from results` (scenarios not defined yet).

- [ ] **Step 3: Add the new spam modes inside `runScenario`**

In `runScenario`, extend the spam-timer block (after the existing `cfg.spam === "sampler"` branch). Add cases and a push counter. Change the local `let sp = 0;` region to also track pushes; add near the other counters `let paramPushes = 0;`:

```javascript
    } else if (cfg.spam === "toggle") {
      // Cyclic enable/disable of one stage — 2 alternating param hashes.
      spamTimer = setInterval(() => {
        sp = (sp + 1) % 2;
        setInstrumentParam(id, cfg.toggleParam || "filter_enabled", sp);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "toggle-multi") {
      // Rotate on/off across several stages → many distinct hashes.
      const stages = ["filter_enabled", "lfo_enabled", "filtenv_enabled"];
      let k = 0;
      spamTimer = setInterval(() => {
        const st = stages[k % stages.length];
        setInstrumentParam(id, st, (k >> 1) & 1 ? 0 : 1); // vary value too
        k++;
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "ctrl-sweep") {
      // Monotone sweep of a control on an ENABLED stage (worst for LRU key).
      let v = 0;
      spamTimer = setInterval(() => {
        v = (v + 0.03) % 1.0;
        setInstrumentParam(id, cfg.ctrlParam || "filter_cutoff", v);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "ctrl-cyclic") {
      // Triangle sweep — values revisit, so the cache warms.
      let v = 0, dir = 1;
      spamTimer = setInterval(() => {
        v += dir * 0.05; if (v >= 1) { v = 1; dir = -1; } else if (v <= 0) { v = 0; dir = 1; }
        setInstrumentParam(id, cfg.ctrlParam || "lfo_rate", v);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "combined") {
      // Toggle a stage AND sweep its control together (the realistic storm).
      let v = 0, t = 0;
      spamTimer = setInterval(() => {
        t = (t + 1) % 8;
        if (t === 0) setInstrumentParam(id, "filter_enabled", (t >> 2) & 1 ? 0 : 1);
        v = (v + 0.04) % 1.0;
        setInstrumentParam(id, "filter_cutoff", v);
        paramPushes += (t === 0 ? 2 : 1);
      }, SPAM_INTERVAL_MS);
    }
```

Then, before the existing `cfg.spam === "synth"` branch guard, ensure the existing `if (cfg.spam === "synth")` remains the first case (leave it). Add `paramPushes` to the returned object (in the `return { ... }` block, alongside `notes`):

```javascript
      paramPushes,
```

- [ ] **Step 4: Register the new scenarios**

In the `const SCENARIOS = [ ... ]` array, append (before the closing `]`):

```javascript
  { name: "stage-toggle-storm", id: "cello", pitched: true, spam: "toggle", prewarm: true, toggleParam: "filter_enabled" },
  { name: "stage-toggle-multi", id: "cello", pitched: true, spam: "toggle-multi", prewarm: true },
  { name: "enabled-ctrl-sweep", id: "cello", pitched: true, spam: "ctrl-sweep", prewarm: true, ctrlParam: "filter_cutoff" },
  { name: "enabled-ctrl-cyclic", id: "cello", pitched: true, spam: "ctrl-cyclic", prewarm: true, ctrlParam: "lfo_rate" },
  { name: "combined-toggle+ctrl", id: "cello", pitched: true, spam: "combined", prewarm: true },
```

- [ ] **Step 5: Add `paramPushes` to the report line**

In the report loop (the `for (const [name, r] of Object.entries(results))` block), append `pushes=${r.paramPushes}` to the printed line so it appears on stdout, e.g. change the trailing template to include:

```javascript
    `${String(r.workerQueuePeak).padStart(5)} ${fmt(r.maxGapMs)}ms  pushes=${r.paramPushes ?? 0}`
```

- [ ] **Step 6: Run to verify it passes**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/webaudio_render_latency_bench.browser.test.js`
Expected: PASS — the five new scenarios print with `pushes>0`, `mainThreadRenders==0`, and `bench-results/render_latency.json` now contains them.

- [ ] **Step 7: Checkpoint** — stop for review. Do NOT commit.

---

### Task 3: Harness B — real-UI stage-toggle bench (reproduces symptom B)

New Playwright bench driving full `main.wasm` with real Synth-tab pill clicks and knob drags, measuring audio health (in-graph RMS + drops), frame health (`perfStats`), render health (`getRenderLatencyMetrics`) while the tab is open. A Synth-tab-vs-EQ-tab control run isolates draw cost. Depends on Task 1's `synthEnablePillRect()`.

**Files:**
- Create: `src/js/synth_stage_toggle_ui_bench.browser.test.js`

**Interfaces:**
- Consumes: `real_input_test_helpers.js` (`buildMainWasm`, server + import-settling helpers — mirror an existing real-UI test's setup), and JS globals: `setRowInstrument(row,id)`, `startPlay()`, `setEQTab("synth"|"eq")`, `selectSynthSection(label)`, `synthChipRects()`, `synthEnablePillRect()`, `synthKnobRects()`, `perfStats()`, `getAudioScheduleMetrics()`, `getRenderLatencyMetrics()`, `window.__audioCtx`.
- Produces: `bench-results/synth_stage_toggle_ui.json` with `{ synth: metrics, eqControl: metrics }`, each holding `{ rmsAvg, rmsMin, fpsAvg, drawMaxMS, audioDrops, overdue, smallLead, maxGapMs, renders, mainThreadRenders, deferAbsMsP90, toggles, knobDrags }`.

- [ ] **Step 1: Scaffold the harness with a failing invariant**

Create `src/js/synth_stage_toggle_ui_bench.browser.test.js`. Model the boilerplate (server, `buildMainWasm`, page setup, real audio-unlock click, import settling, `setEQTab`/`selectSynthSection`/knob-rect usage) on `src/js/synth_generator_solo_realinput.browser.test.js` (verified to contain all these patterns). Core structure:

```javascript
/**
 * Harness B — real-UI synth stage-toggle bench (WASM, full main.wasm).
 *
 * Reproduces the user-reported crackle: while playing, real clicks on the
 * detail-header enable pill (via synthEnablePillRect) and real horizontal
 * knob drags on an enabled stage, WITH THE SYNTH TAB OPEN AND DRAWING.
 * A control run does the identical param churn on the cheap EQ tab to
 * isolate Synth-tab draw cost (project_synth_tab_audio_starvation).
 *
 * Structural gates only (software-GL amplifies draw cost — wall-clock is
 * recorded, not gated):
 *   - no page errors; AudioContext running
 *   - toggles>0 and knobDrags>0 (the storm actually ran)
 *   - audio recovers after the storm (RMS on a quiet tab > silence floor)
 *     — proves starvation, not a permanently dead instrument
 * Records: RMS, FPS, drawMaxMS, audioDrops, overdue, gaps, render metrics.
 */
import { chromium } from "playwright";
// ... imports mirroring synth_generator_solo_realinput.browser.test.js ...

// build main.wasm, start server, open page, unlock audio, settle imports.
// Put a melodic instrument on row 0 so the Synth tab edits a heavy voice:
await page.evaluate(() => setRowInstrument(0, "cello"));
await page.evaluate(() => startPlay());
```

Add a `runUIStorm(tabName)` that: `setEQTab(tabName)`; if synth, `selectSynthSection("FILTER")`; taps a **real AnalyserNode** into the master (or reuses the in-graph analyser the app already exposes — check `window.__audioCtx` graph as `synth_generator_solo_realinput` does) to sample RMS; then for `DURATION_MS` on `SPAM_INTERVAL_MS`:
- every 4th tick: click the centre of `synthEnablePillRect()` (skip if empty) via `page.mouse.click(x+w/2, y+h/2)`, `toggles++`;
- other ticks: perform a real horizontal knob drag on the FILTER cutoff knob located via `synthKnobRects()` (mirror the horizontal-axis drag from `synth_generator_solo_realinput`), `knobDrags++`.
Snapshot `perfStats()`, `getAudioScheduleMetrics()`, `getRenderLatencyMetrics()`, and the RMS accumulator at the end.

End with the failing gate (before implementing the storm body):

```javascript
const results = { synth: await runUIStorm("synth"), eqControl: await runUIStorm("eq") };
const failures = [];
if ((results.synth.toggles || 0) <= 0) failures.push("synth: no stage toggles fired");
if ((results.synth.knobDrags || 0) <= 0) failures.push("synth: no knob drags fired");
if ((results.synth.mainThreadRenders || 0) > 0) failures.push(`synth: ${results.synth.mainThreadRenders} main-thread renders`);
```

- [ ] **Step 2: Run to verify it fails**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/synth_stage_toggle_ui_bench.browser.test.js`
Expected: FAIL — either the storm body is a stub (toggles==0) or `synthEnablePillRect` returns empty because no stage is selected. This confirms the harness wiring is exercised.

- [ ] **Step 3: Implement the storm body + RMS tap + reporting**

Fill in `runUIStorm` per Step 1's description, add the RMS analyser tap (mirror the in-graph analyser approach in `synth_generator_solo_realinput.browser.test.js` so we read the *rendered* stream, not a synthetic buffer), write the JSON output, and print a comparison table:

```javascript
const outDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(path.join(outDir, "synth_stage_toggle_ui.json"), JSON.stringify(results, null, 2));
console.log("[TEST] tab      rmsAvg rmsMin fpsAvg drawMax drops overdue maxGap toggles drags renders mtRenders");
for (const [tab, r] of Object.entries(results)) {
  console.log(`[TEST] ${tab.padEnd(8)} ${r.rmsAvg?.toFixed?.(4)} ${r.rmsMin?.toFixed?.(4)} ${r.fpsAvg?.toFixed?.(1)} ${r.drawMaxMS?.toFixed?.(1)} ${r.audioDrops} ${r.overdue} ${r.maxGapMs?.toFixed?.(1)} ${r.toggles} ${r.knobDrags} ${r.renders} ${r.mainThreadRenders}`);
}
```

Add the recovery gate after the storm (leave a cheap tab, let audio settle, sample RMS):

```javascript
await page.evaluate(() => setEQTab("eq"));
await page.waitForTimeout(600);
const recoverRMS = await sampleRMS(page, 400); // helper added in Step 1's tap
if (recoverRMS < 1e-3) failures.push(`audio did not recover after storm (RMS ${recoverRMS}) — instrument may be dead, not merely starved`);
```

- [ ] **Step 4: Run to verify it passes**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/synth_stage_toggle_ui_bench.browser.test.js`
Expected: PASS — prints the synth-vs-eqControl table; `bench-results/synth_stage_toggle_ui.json` written; no page errors. (The synth-tab RMS may read much lower than eqControl under software GL — that is the attribution signal, recorded not gated.)

- [ ] **Step 5: Checkpoint** — stop for review. Do NOT commit.

---

### Task 4: Desktop bench param-mutation schedule (pure logic + Go unit test)

The desktop bench (`game_update.go`) only overrides BPM today. Add opt-in storm scheduling as a *pure* function first (unit-testable without Ebiten), then wire it into the loop in Task 5.

**Files:**
- Create: `src/go/internal/ui/bench_synth_storm.go`
- Create: `src/go/internal/ui/bench_synth_storm_test.go`

**Interfaces:**
- Produces:
  - `type benchStormScenario int` with consts `benchStormOff`, `benchStormToggle`, `benchStormCtrlSweep`, `benchStormCombined`.
  - `func parseBenchStormScenario(s string) benchStormScenario` — maps `""|"off"`→off, `"toggle"`→toggle, `"ctrl"|"ctrl-sweep"`→ctrl, `"combined"`→combined.
  - `type benchStormMutation struct { Param string; Value float64 }`
  - `func benchStormMutationsForTick(sc benchStormScenario, tick int) []benchStormMutation` — deterministic per-tick param writes (empty when off). Consumed by Task 5.

- [ ] **Step 1: Write the failing unit test**

Create `src/go/internal/ui/bench_synth_storm_test.go`:

```go
package ui

import "testing"

func TestParseBenchStormScenario(t *testing.T) {
	cases := map[string]benchStormScenario{
		"": benchStormOff, "off": benchStormOff,
		"toggle": benchStormToggle,
		"ctrl": benchStormCtrlSweep, "ctrl-sweep": benchStormCtrlSweep,
		"combined": benchStormCombined,
	}
	for in, want := range cases {
		if got := parseBenchStormScenario(in); got != want {
			t.Fatalf("parseBenchStormScenario(%q)=%v want %v", in, got, want)
		}
	}
}

func TestBenchStormMutations(t *testing.T) {
	if m := benchStormMutationsForTick(benchStormOff, 5); len(m) != 0 {
		t.Fatalf("off scenario must emit no mutations, got %d", len(m))
	}
	// Toggle alternates filter_enabled between 1 and 0 across ticks.
	m0 := benchStormMutationsForTick(benchStormToggle, 0)
	m1 := benchStormMutationsForTick(benchStormToggle, 1)
	if len(m0) != 1 || m0[0].Param != "filter_enabled" {
		t.Fatalf("toggle tick0 = %+v, want single filter_enabled", m0)
	}
	if m0[0].Value == m1[0].Value {
		t.Fatalf("toggle must alternate value across ticks: %v vs %v", m0[0].Value, m1[0].Value)
	}
	// Ctrl sweep drives filter_cutoff within [0,1].
	mc := benchStormMutationsForTick(benchStormCtrlSweep, 3)
	if len(mc) != 1 || mc[0].Param != "filter_cutoff" || mc[0].Value < 0 || mc[0].Value > 1 {
		t.Fatalf("ctrl sweep tick3 = %+v, want filter_cutoff in [0,1]", mc)
	}
	// Combined emits both a toggle (periodically) and a cutoff sweep.
	total := 0
	for tick := 0; tick < 8; tick++ {
		total += len(benchStormMutationsForTick(benchStormCombined, tick))
	}
	if total <= 8 {
		t.Fatalf("combined over 8 ticks should emit >8 mutations (sweep every tick + periodic toggle), got %d", total)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestParseBenchStormScenario|TestBenchStormMutations' ./internal/ui/`
Expected: FAIL — `undefined: benchStormOff` etc.

- [ ] **Step 3: Implement the pure logic**

Create `src/go/internal/ui/bench_synth_storm.go`:

```go
package ui

// benchStormScenario selects the opt-in synth param-mutation pattern the
// desktop bench replays during its measure window. Default (off) leaves the
// existing bench output unchanged.
type benchStormScenario int

const (
	benchStormOff benchStormScenario = iota
	benchStormToggle
	benchStormCtrlSweep
	benchStormCombined
)

func parseBenchStormScenario(s string) benchStormScenario {
	switch s {
	case "toggle":
		return benchStormToggle
	case "ctrl", "ctrl-sweep":
		return benchStormCtrlSweep
	case "combined":
		return benchStormCombined
	default:
		return benchStormOff
	}
}

// benchStormMutation is a single param write the bench applies via
// audio.SetInstrumentParam(instrumentID, Param, Value).
type benchStormMutation struct {
	Param string
	Value float64
}

// benchStormMutationsForTick returns the param writes for a given storm tick.
// Deterministic (no time/random) so the unit test and the live bench agree.
func benchStormMutationsForTick(sc benchStormScenario, tick int) []benchStormMutation {
	switch sc {
	case benchStormToggle:
		return []benchStormMutation{{Param: "filter_enabled", Value: float64(tick & 1) ^ 1}}
	case benchStormCtrlSweep:
		return []benchStormMutation{{Param: "filter_cutoff", Value: float64(tick%33) / 32.0}}
	case benchStormCombined:
		out := []benchStormMutation{{Param: "filter_cutoff", Value: float64(tick%25) / 24.0}}
		if tick%8 == 0 {
			out = append(out, benchStormMutation{Param: "filter_enabled", Value: float64((tick / 8) & 1) ^ 1})
		}
		return out
	default:
		return nil
	}
}
```

Note: `float64(tick & 1) ^ 1` is invalid Go (`^` on float). Use an int first:

```go
	case benchStormToggle:
		v := 1
		if tick&1 == 1 {
			v = 0
		}
		return []benchStormMutation{{Param: "filter_enabled", Value: float64(v)}}
```

Apply the same int-then-convert pattern in `benchStormCombined`'s toggle branch.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestParseBenchStormScenario|TestBenchStormMutations' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — stop for review. Do NOT commit.

---

### Task 5: Wire the storm into the desktop bench loop + flag + scripts

Plumb the pure logic from Task 4 into the bench lifecycle, add the CLI flag, count pushes into `[bench_json]`, and add the script/Makefile entry points.

**Files:**
- Modify: `src/go/internal/ui/game_struct.go` (bench state fields, ~line 141-148)
- Modify: `src/go/internal/ui/demo.go` (`RunBenchmark`, ~line 267-271)
- Modify: `src/go/internal/ui/game_update.go` (bench loop, ~line 88-157)
- Modify: `src/go/cmd/beatmo.go` (flag, ~line 39-43 + call site ~line 284)
- Modify: `scripts/bench-desktop.sh`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `parseBenchStormScenario`, `benchStormMutationsForTick`, `benchStormScenario` (Task 4); `audio.SetInstrumentParam(id, name string, value float64)`; `audio.GetRenderLatencyMetrics()`.
- Produces: `(*Game).RunBenchmark(bpm int, secs float64, storm benchStormScenario, stormInst string)`; new `[bench_json]` fields `stormPushes` (int) and `stormScenario` (string).

- [ ] **Step 1: Add bench state fields**

In `src/go/internal/ui/game_struct.go`, alongside the existing `benchBPM`/`benchDuration` fields, add:

```go
	benchStorm      benchStormScenario
	benchStormInst  string
	benchStormTick  int
	benchStormPushes int
```

- [ ] **Step 2: Extend `RunBenchmark`**

In `src/go/internal/ui/demo.go`, change `RunBenchmark` to accept and store the storm config:

```go
func (g *Game) RunBenchmark(bpm int, secs float64, storm benchStormScenario, stormInst string) {
	g.benchBPM = bpm
	g.benchDuration = secs
	g.benchStorm = storm
	g.benchStormInst = stormInst
	g.demoScheduled = true
}
```

- [ ] **Step 3: Drive mutations in the bench loop**

In `src/go/internal/ui/game_update.go`, inside the bench window (between "playback started" and the `benchDuration elapsed` branch — the per-frame region around line 104-120), add:

```go
	if g.benchStorm != benchStormOff && g.benchStormInst != "" {
		for _, m := range benchStormMutationsForTick(g.benchStorm, g.benchStormTick) {
			audio.SetInstrumentParam(g.benchStormInst, m.Param, m.Value)
			g.benchStormPushes++
		}
		g.benchStormTick++
	}
```

In the completion branch (where `[bench_json]` is printed, ~line 135-158), add the two fields to the JSON line (match the existing key:value style already used there):

```go
	// ... existing fields ...
	fmt.Sprintf(`,"stormScenario":%q,"stormPushes":%d`, benchStormName(g.benchStorm), g.benchStormPushes)
```

Add a tiny helper next to the loop (or in `bench_synth_storm.go`):

```go
func benchStormName(sc benchStormScenario) string {
	switch sc {
	case benchStormToggle:
		return "toggle"
	case benchStormCtrlSweep:
		return "ctrl-sweep"
	case benchStormCombined:
		return "combined"
	default:
		return "off"
	}
}
```

(Adapt the exact string-concatenation to however the existing `[bench_json]` line is assembled — append the two keys before the closing brace.)

- [ ] **Step 4: Add the CLI flag and pass it through**

In `src/go/cmd/beatmo.go`, near the other bench flags (~line 39-43):

```go
	benchStorm := flag.String("bench-synth-storm", "", "synth param-mutation storm during bench: off|toggle|ctrl|combined")
	benchStormInst := flag.String("bench-synth-storm-inst", "cello", "instrument id the synth storm mutates")
```

At the `RunBenchmark` call site (~line 284-285), pass them:

```go
	g.RunBenchmark(benchBPM, benchSecs, parseBenchStormScenarioFromCmd(*benchStorm), *benchStormInst)
```

Since `parseBenchStormScenario` lives in `internal/ui`, expose a thin exported wrapper there (add to `bench_synth_storm.go`):

```go
// ParseBenchStormScenario is the exported entry point for cmd/.
func ParseBenchStormScenario(s string) benchStormScenario { return parseBenchStormScenario(s) }
```

and call `ui.ParseBenchStormScenario(*benchStorm)` from cmd (the `internal/ui` import alias in `beatmo.go`).

- [ ] **Step 5: Verify desktop build + fast tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Bench|Storm'`
Then: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build ./cmd`
Expected: tests PASS; `cmd` builds (exit 0).

- [ ] **Step 6: Add the storm pass to `scripts/bench-desktop.sh` and a Makefile target**

In `scripts/bench-desktop.sh`, after the main BPM loop, add an optional storm pass gated by an env var so the default run is unchanged:

```bash
if [ -n "${BENCH_SYNTH_STORM:-}" ]; then
  echo "== synth storm pass: $BENCH_SYNTH_STORM =="
  ( cd src/go && PARITY_FATAL=0 PERF_LOG=1 CGO_ENABLED=1 "$GO" run ./cmd -log INFO \
      -bench-bpm "${STORM_BPM:-200}" -bench-secs "${DURATION}" \
      -bench-synth-storm "$BENCH_SYNTH_STORM" \
      -bench-prof "$RESULTS_DIR/cpu_storm_${BENCH_SYNTH_STORM}.prof" ) \
    2>&1 | tee "$RESULTS_DIR/desktop_storm_${BENCH_SYNTH_STORM}.log"
fi
```

In `Makefile`, near `bench-desktop` (~line 341), add:

```makefile
# Desktop synth param-mutation storm bench + pprof (toggle|ctrl|combined).
bench-synth-storm: $(C_LIB)
	GO=$(GO) BENCH_SYNTH_STORM=$(or $(STORM),combined) STORM_BPM=$(or $(BPM),200) DURATION=$(or $(DURATION),15) ./scripts/bench-desktop.sh
```

- [ ] **Step 7: Smoke-run the desktop storm bench (short)**

Run: `cd /home/ymolinar/Repos/beatmo && STORM=combined DURATION=4 make bench-synth-storm`
Expected: completes; `bench-results/desktop_storm_combined.log` contains a `[bench_json]` line with `"stormScenario":"combined"` and `"stormPushes"` > 0; `cpu_storm_combined.prof` exists.

- [ ] **Step 8: Checkpoint** — stop for review. Do NOT commit.

---

### Task 6: Baseline profiling run + attribution note (the evidence deliverable)

Run all three harnesses at baseline, capture results, and write the attribution note. Per the evidence-before-change constraint, **this is where the plan ends** — no fix is designed until this note exists.

**Files:**
- Create/refresh: `bench-results/render_latency.json`, `bench-results/synth_stage_toggle_ui.json`, `bench-results/desktop_storm_*.log`, `bench-results/cpu_storm_*.prof`
- Create: `bench-results/synth-toggle-attribution.md` (the written finding)

- [ ] **Step 1: Run Harness A (bridge, strict)**

Run: `RENDER_BENCH_STRICT=1 GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/webaudio_render_latency_bench.browser.test.js | tee bench-results/harnessA.log`
Record: renders/push, deferAbsMsP90, workerQueuePeak, maxGap for each new scenario.

- [ ] **Step 2: Run Harness B (real-UI)**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/synth_stage_toggle_ui_bench.browser.test.js | tee bench-results/harnessB.log`
Record: the synth-vs-eqControl RMS/FPS/drawMax/drops/overdue delta.

- [ ] **Step 3: Run Harness C (desktop pprof) for each scenario**

Run: `for s in toggle ctrl combined; do STORM=$s DURATION=15 make bench-synth-storm; done`
Then capture the hot path:
`go tool pprof -top -cum bench-results/cpu_storm_combined.prof | head -30 | tee bench-results/harnessC_pprof_top.log`

- [ ] **Step 4: Write the attribution note**

Create `bench-results/synth-toggle-attribution.md` answering, **from the numbers only**:
1. Does the corruption reproduce in Harness A (bridge)? If `deferAbsMsP90`/`maxGap` stay low and `mainThreadRenders==0`, the DSP layer is NOT the cause.
2. Does Harness B show the synth-tab RMS collapse / drops / FPS drop that Harness A lacks? If so, the damage is above the audio layer (commit + mirror + draw).
3. Does the Harness B **eqControl** run stay healthy under identical param churn? If yes, the culprit is **Synth-tab draw cost**, not the toggle mutation itself.
4. What does Harness C's pprof `-top -cum` attribute the desktop CPU to (SetInstrumentParam/commit/mirror vs. render vs. draw)?
Conclude with a one-paragraph verdict naming the dominant layer, and list the candidate fix directions (do NOT implement them).

- [ ] **Step 5: Save a project memory entry**

Write a `project_*` memory file summarizing the attribution finding + the three-harness bench commands, and add its one-line pointer to `MEMORY.md` (per the memory instructions), so the follow-up fix session starts with evidence.

- [ ] **Step 6: Final checkpoint** — present the attribution note to the user. Do NOT commit.

---

## Self-Review

**Spec coverage:**
- Harness A (bridge, stage-toggle + enabled-control, real-time) → Task 2. ✓
- Harness B (real-UI, real pill clicks, symptom-B reproduction, Synth-vs-EQ control) → Tasks 1 + 3. ✓
- Harness C (desktop, opt-in storm, pprof) → Tasks 4 + 5. ✓
- Profiling + attribution note, evidence-before-change → Task 6. ✓
- Structural-gate-only / wall-clock-recorded policy → encoded in Tasks 2, 3 gates. ✓
- Off-thread invariant (`mainThreadRenders==0`) → Tasks 2, 3. ✓
- Default desktop output unchanged (opt-in) → Task 4 (default off) + Task 5 (env-gated script pass). ✓

**Placeholder scan:** No "TBD"/"implement later". The one deliberate Go pitfall (`float64 ^ int`) is called out with its correction inline. Harness B's RMS-tap detail defers to a named reference file (`synth_generator_solo_realinput.browser.test.js`) rather than inventing an unverified analyser API — acceptable because the implementer verifies against a real, cited file.

**Type consistency:** `benchStormScenario`, `parseBenchStormScenario`/`ParseBenchStormScenario`, `benchStormMutationsForTick`, `benchStormMutation{Param,Value}`, `RunBenchmark(bpm, secs, storm, stormInst)` are used identically across Tasks 4–5. `synthEnablePillRect()` return shape `{x,y,w,h}` consistent between Task 1 (producer) and Task 3 (consumer). Metric field names (`mainThreadRenders`, `deferAbsMsP90`, `paramPushes`, `audioDrops`) match the existing `getRenderLatencyMetrics`/`perfStats` shapes verified during design.

**Never-commit note:** every task ends in a *checkpoint*, not a commit, per project convention.
