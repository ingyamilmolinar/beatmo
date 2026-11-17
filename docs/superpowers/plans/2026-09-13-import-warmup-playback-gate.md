# Import Warm-Up + Playback Readiness Gate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Never let the user start playback on a cold voice cache — enumerate the loaded circuit's full working set, render it before Play is enabled, show determinate progress through the notification system, and skip anything already cached.

**Architecture:** A `warmSet` (deterministic list of `(instrument, pitch)` pairs for the whole circuit) is built from the existing `beatInfosByRow` walk with the predictor horizon extended to a full loop. A `warmupController` diffs it against the voice cache using the *same* key computation dispatch uses, feeds the remainder to a per-platform backend that cannot drop work, and flips a readiness flag consumed at the single `g.drum.PlayPressed()` chokepoint. Progress renders through `logsToast`, extended in place with a sticky determinate task mode.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten 2.9, cgo + miniaudio C DSP, WASM + WebAudio (`src/js/audio.js`), `internal/async` pools, `internal/hooks` + `internal/eventlogger`, `internal/i18n`.

**Spec:** `docs/superpowers/specs/2026-09-13-import-warmup-playback-gate-design.md`

## Global Constraints

- **NEVER run `git add`, `git commit`, or any git write operation.** The user manages all staging and commits. Every task ends with verification, not a commit. Leave the working tree dirty.
- **Always use `.tools/go/bin/go`** (bundled toolchain), never the system `go`. Use `.tools/go/bin/gofmt` for formatting.
- Fast Go tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
- Real-render Go tests (needs the C lib + X11): `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/`
- C changes require `make build/libdrums.a` before they take effect (cgo links a **prebuilt** `.a`). No C changes are planned here.
- Browser tests need an **absolute** GO path: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<file>.browser.test.js`
- `-tags test` **skips** `!test && !js` files silently. Any invariant that must hold in production needs a real-build test too.
- Every test that constructs a `Game` must `t.Cleanup(g.CloseForTest)`.
- Every test that allocates a transient named async pool must `t.Cleanup(func(){ async.DefaultRegistry().Release(name) })`.
- No new `color.RGBA{}` / `color.NRGBA{}` literals in `internal/ui/` (`TestTokenDiscipline` ratchets per-file budgets both directions).
- No forbidden glyphs (`▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←`) in `internal/ui/` source — use `IconID` + `DrawIcon`.
- Corner radius by ROLE: `RadiusControl` (4) for interactive controls, `RadiusContainer` (8) for panels/toasts. Never pick a raw `RadiusXXS…RadiusXL` at a call site.
- All user-facing strings go through `internal/i18n` keys, added to **both** `catalog_en.go` and `catalog_es.go`.
- **Every UI change must be verified by rendering a screenshot scene and opening the PNG.** Green Go tests are not sufficient. Run screenshot captures with `XDG_CONFIG_HOME=<scratch dir>` so the sweep never edits the developer's real `~/.config/beatmo/prefs.json`.

---

## File Structure

**New files:**
- `src/go/internal/ui/warm_set.go` — `warmItem`, `warmSet`, `(*Game).buildWarmSet()`. Enumeration only; no rendering, no state.
- `src/go/internal/ui/warmup_controller.go` — `warmupController` (diff, feed, deadline, progress, cancel) + the `warmBackend` interface. Platform-agnostic; backend injected.
- `src/go/internal/ui/warmup_backend_desktop.go` (`//go:build !js`) — pool-fed backend.
- `src/go/internal/ui/warmup_backend_js.go` (`//go:build js`) — `warmInstrumentBatch` bridge backend.
- `src/js/webaudio_import_cold_bench.browser.test.js` — permanent import-cold browser bench.

**Modified files:**
- `src/go/internal/audio/render_latency.go` — nothing (already exposes `StalePlays`).
- `src/go/internal/ui/game_update.go` — bench JSON line (+`stalePlays` etc.), metrics reset at bench start, the Play gate at `:826`, `warmup.Tick`.
- `src/go/internal/ui/logs_toast.go` — sticky determinate task mode.
- `src/go/internal/ui/grid_pane_layers.go`, `grid_tree.go`, `grid_pane_draw.go` — own `grid-toast` layer.
- `src/go/internal/ui/grid_layer_zorder_test.go` — new layer row.
- `src/go/internal/ui/runtime_profile.go` — `PrewarmWorkers`, `WarmupDeadline`.
- `src/go/internal/ui/voice_prewarmer.go` — pool worker count from profile.
- `src/go/internal/async/registry.go`, `registry_footprint_test.go` — budget floor.
- `src/go/internal/audio/voice_prewarm_js.go` — de-stub.
- `src/js/audio.js` — `warmInstrumentBatch`, `voiceCachedFor`, worker pool.
- `src/go/internal/hooks/events.go`, `actions.go`; `src/go/internal/eventlogger/format.go` — warm-up events.
- `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go` — new strings.
- `src/go/internal/ui/import.go` — trip the gate.
- `src/go/internal/ui/scene_catalog.go` — warm-up scenes.
- `CLAUDE.md` — standing-pool table row.

---

## Task 1: Surface the missing warm-up metrics in the desktop bench

The bench prints `RenderLatency` but omits `StalePlays` — the counter that says "we played the wrong buffer" — and never resets render metrics at bench start, so startup renders pollute the window.

**Files:**
- Modify: `src/go/internal/ui/game_update.go` (bench start block ~`:92-105`, bench complete block ~`:120-165`)
- Test: `src/go/internal/ui/bench_json_metrics_test.go` (create)

**Interfaces:**
- Consumes: `audio.GetRenderLatencyMetrics() audio.RenderLatencyMetrics` (fields `CacheHits, CacheMisses, RenderAvgNS, RenderMaxNS, DispatchRenders, PrewarmRenders, StalePlays`), `audio.ResetRenderLatencyMetrics()`.
- Produces: `[bench_json]` keys `renderStalePlays`, `warmSetSize`, `warmSkippedCached`, `warmMS` (the last three emit `0` until Task 4 populates them).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/bench_json_metrics_test.go`:

```go
//go:build test

package ui

import (
	"strings"
	"testing"
)

// TestBenchJSONCarriesWarmupMetrics pins the machine-readable bench line's
// key set. scripts/bench-desktop.sh greps stdout for "[bench_json] "; a key
// that silently disappears breaks the regression tracking without failing
// anything, so the key list is asserted here.
func TestBenchJSONCarriesWarmupMetrics(t *testing.T) {
	for _, key := range []string{
		"renderStalePlays", "warmSetSize", "warmSkippedCached", "warmMS",
	} {
		if !strings.Contains(benchJSONFormat, `"`+key+`":`) {
			t.Errorf("bench_json format string missing key %q", key)
		}
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBenchJSONCarriesWarmupMetrics ./internal/ui/`
Expected: FAIL — `undefined: benchJSONFormat`.

- [ ] **Step 3: Extract the format string and add the keys**

In `game_update.go`, hoist the giant inline `fmt.Printf` format into a package-level constant so it is testable, and append the four new keys:

```go
// benchJSONFormat is the machine-readable bench line consumed by
// scripts/bench-desktop.sh (which greps stdout for "[bench_json] "). Hoisted
// to a constant so bench_json_metrics_test.go can pin its key set — a key
// vanishing here breaks regression tracking silently otherwise.
const benchJSONFormat = `[bench_json] {"bpm":%d,` /* …existing keys verbatim… */ +
	`,"renderStalePlays":%d,"warmSetSize":%d,"warmSkippedCached":%d,"warmMS":%.3f` +
	`,"stormScenario":%q,"stormPushes":%d}` + "\n"
```

Pass the new values at the call site. **This task emits literal zeros for the
three warm keys** — Task 4 replaces them once `warmupController` exists. Keeping
the forward dependency out means Task 1 stands alone and can be reviewed alone:

```go
// warmSetSize / warmSkippedCached / warmMS are wired to the warm-up controller
// in Task 4; zeros here keep the key set stable for bench-desktop.sh from the
// first commit onward.
fmt.Printf(benchJSONFormat, /* …existing args… */, rl.StalePlays, 0, 0, 0.0,
	benchStormName(g.benchStorm), g.benchStormPushes)
```

Add `stalePlays` to the human-readable INFO line too:

```go
g.logger.Infof("[bench] RenderLatency: cacheHits=%d cacheMisses=%d dispatchRenders=%d prewarmRenders=%d stalePlays=%d renderAvg=%.2fms renderMax=%.2fms",
	rl.CacheHits, rl.CacheMisses, rl.DispatchRenders, rl.PrewarmRenders, rl.StalePlays,
	float64(rl.RenderAvgNS)/1e6, float64(rl.RenderMaxNS)/1e6)
```

In the **bench start** block (next to `g.perf.reset()`), add:

```go
audio.ResetRenderLatencyMetrics() // exclude startup renders from the window
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBenchJSONCarriesWarmupMetrics ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verify the real bench still emits a parseable line**

```bash
cd /home/ymolinar/Repos/beatmo
CGO_ENABLED=1 .tools/go/bin/go build -C src/go -o /tmp/beatmo_bench ./cmd
BEATMO_DEMO_CONFIG=$(pwd)/src/go/internal/assets/templates/beach-gaelic.json \
  xvfb-run -a /tmp/beatmo_bench -log INFO -bench-bpm 180 -bench-secs 5 2>&1 \
  | grep '\[bench_json\]' | sed 's/^\[bench_json\] //' | python3 -m json.tool | head -20
```
Expected: valid JSON containing `renderStalePlays`. Leave uncommitted.

---

## Task 2: Permanent `import-cold` browser bench

Reproduces the measured baseline (66 wrong-pitch notes, 288 ms deferrals) as a tracked artifact instead of a throwaway probe.

**Files:**
- Create: `src/js/webaudio_import_cold_bench.browser.test.js`
- Modify: `Makefile` (add `bench-import-cold` target near `bench-render`)

**Interfaces:**
- Consumes: `setupFullWasm`, `clickAt` from `src/js/real_input_test_helpers.js`; page globals `importJSON`, `startPlay`, `getRenderLatencyMetrics`, `perfStats`, `resetPerfStats`.
- Produces: `bench-results/import_cold.json` with `{ template, pairs, samples: [{tSec, renders, neighborPlays, deferredPlays, deferMsAvg, renderMsAvg, workerQueuePeak, updateAvgMS, drawAvgMS}] }`.

- [ ] **Step 1: Write the bench**

Create `src/js/webaudio_import_cold_bench.browser.test.js`:

```js
/**
 * Import-cold bench: load a heavy circuit, immediately play, and record how
 * long the engine keeps rendering while the user is listening.
 *
 * BASELINE 2026-09-13 (beach-gaelic, 91 distinct (instrument,pitch) pairs):
 *   neighborPlays=66 (notes played at the WRONG PITCH), deferredPlays=4 at
 *   288ms off-grid, renderMsAvg=270ms, workerQueuePeak=12, still rendering at
 *   t=27s. See docs/superpowers/specs/2026-09-13-import-warmup-playback-gate-design.md
 *
 * TARGET after the warm-up gate: neighborPlays=0, deferredPlays=0, 0 renders
 * during playback.
 */
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm, clickAt } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const tplDir = path.resolve(__dirname, "../go/internal/assets/templates");
const outDir = path.resolve(__dirname, "../../bench-results");
const TPL = process.env.TPL || "beach-gaelic.json";
const SECS = parseInt(process.env.SECS || "30", 10);

(async () => {
  const { page, cleanup } = await setupFullWasm({ logLevel: "ERROR" });
  const samples = [];
  try {
    const raw = fs.readFileSync(path.join(tplDir, TPL), "utf8");
    await clickAt(page, 640, 400);          // user gesture unlocks AudioContext
    await page.waitForTimeout(300);
    await page.evaluate((j) => window.importJSON(j), raw);

    // Wait for the readiness gate (absent pre-Task-10: falls through at once).
    const gateT0 = Date.now();
    await page.waitForFunction(
      () => !window.warmupProgress || window.warmupProgress().done >= window.warmupProgress().total,
      { timeout: 60000 },
    );
    const gateMs = Date.now() - gateT0;

    await page.evaluate(() => {
      if (window.resetPerfStats) window.resetPerfStats();
      window.__audioMetrics = { renders: {}, cacheHits: {} };
      window.startPlay();
    });
    const t0 = Date.now();
    for (let i = 0; i < SECS; i++) {
      await page.waitForTimeout(1000);
      const s = await page.evaluate(() => {
        const rl = window.getRenderLatencyMetrics ? window.getRenderLatencyMetrics() : {};
        const ps = window.perfStats ? window.perfStats() : {};
        const am = window.__audioMetrics || {};
        return {
          renders: Object.values(am.renders || {}).reduce((a, b) => a + b, 0),
          neighborPlays: rl.neighborPlays | 0,
          deferredPlays: rl.deferredPlays | 0,
          deferMsAvg: rl.deferMsAvg || 0,
          renderMsAvg: rl.renderMsAvg || 0,
          workerQueuePeak: rl.workerQueuePeak | 0,
          updateAvgMS: ps.updateAvgMS ?? null,
          drawAvgMS: ps.drawAvgMS ?? null,
        };
      });
      samples.push({ tSec: (Date.now() - t0) / 1000, ...s });
    }
    const last = samples[samples.length - 1];
    console.log(`[import-cold] ${TPL} gateMs=${gateMs} renders=${last.renders} ` +
      `neighborPlays=${last.neighborPlays} deferredPlays=${last.deferredPlays} ` +
      `renderMsAvg=${last.renderMsAvg.toFixed(1)} queuePeak=${last.workerQueuePeak}`);
    fs.mkdirSync(outDir, { recursive: true });
    fs.writeFileSync(path.join(outDir, "import_cold.json"),
      JSON.stringify({ template: TPL, gateMs, samples }, null, 2));
  } finally {
    await cleanup();
  }
})();
```

- [ ] **Step 2: Add the Make target**

In `Makefile`, next to `bench-render`:

```make
# Import-cold bench: load a heavy circuit, play immediately, record how long
# the engine keeps rendering under the user. Produces bench-results/import_cold.json.
bench-import-cold: wasm
	@mkdir -p bench-results
	GO=$(GO) node src/js/webaudio_import_cold_bench.browser.test.js
```

- [ ] **Step 3: Run it and record the baseline**

```bash
cd /home/ymolinar/Repos/beatmo
GO=$(pwd)/.tools/go/bin/go WASM_PREBUILT=1 node src/js/webaudio_import_cold_bench.browser.test.js
```
Expected (pre-fix): `neighborPlays` in the 50–70 range, `renders` still climbing at the last sample. Record the printed line in the task notes — it is the "before" number Task 15 compares against.

- [ ] **Step 4: Verify the manifest guard still passes**

The deploy manifest test parses runtime refs; a new `*.browser.test.js` is not a runtime ref, so this should be unaffected — confirm:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/deploy/`
Expected: PASS. Leave uncommitted.

---

## Task 3: Warm-set enumeration over a full loop

**Files:**
- Create: `src/go/internal/ui/warm_set.go`
- Create: `src/go/internal/ui/warm_set_test.go`
- Modify: `src/go/internal/ui/voice_prewarmer.go` (reuse `roundWarmPitch`)

**Interfaces:**
- Consumes: `g.beatInfosByRow [][]model.BeatInfo`, `g.drum.Rows[row].Instrument`, `(*Game).evalNodeParamsOnly(row, idx int, info model.BeatInfo) (vol, pitch, dur float64)`, `roundWarmPitch(float64) float64`, `model.NodeTypeRegular`.
- Produces:
  ```go
  type warmItem struct { inst string; pitch float64 }
  type warmSet  struct { items []warmItem; bpm int; truncated bool }
  const maxWarmSetItems = 512
  func (g *Game) buildWarmSet() warmSet
  func (s warmSet) Len() int
  ```

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/warm_set_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBuildWarmSetCoversEveryRowPitch asserts the warm set enumerates every
// distinct (instrument, rounded-pitch) pair the circuit can trigger — NOT just
// the pairs inside the current predictor render window. The window horizon was
// the original bug: pitches outside it were only discovered mid-playback.
func TestBuildWarmSetCoversEveryRowPitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	set := g.buildWarmSet()
	if set.Len() == 0 {
		t.Fatal("warm set empty for the default circuit")
	}
	// Every item must name a row instrument that actually exists.
	known := map[string]bool{}
	for _, r := range g.drum.Rows {
		known[r.Instrument] = true
	}
	for _, it := range set.items {
		if it.inst == "" || !known[it.inst] {
			t.Errorf("warm item names unknown instrument %q", it.inst)
		}
		if it.pitch != roundWarmPitch(it.pitch) {
			t.Errorf("warm item pitch %v is not rounded to 0.5st", it.pitch)
		}
	}
}

// TestBuildWarmSetDeduplicates pins that a looping melody producing the same
// (instrument, pitch) on many beats yields ONE item, so a graph edit cannot
// burst-flood the warm backend.
func TestBuildWarmSetDeduplicates(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	set := g.buildWarmSet()
	seen := map[warmItem]bool{}
	for _, it := range set.items {
		if seen[it] {
			t.Fatalf("duplicate warm item %+v", it)
		}
		seen[it] = true
	}
}

// TestBuildWarmSetRespectsCap pins the bound so a pathological circuit cannot
// make the readiness gate effectively infinite.
func TestBuildWarmSetRespectsCap(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	set := g.buildWarmSet()
	if set.Len() > maxWarmSetItems {
		t.Fatalf("warm set size %d exceeds cap %d", set.Len(), maxWarmSetItems)
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBuildWarmSet ./internal/ui/`
Expected: FAIL — `g.buildWarmSet undefined`.

- [ ] **Step 3: Implement the enumeration**

Create `src/go/internal/ui/warm_set.go`:

```go
package ui

import "github.com/ingyamilmolinar/beatmo/core/model"

// maxWarmSetItems bounds the gated warm set so a pathological circuit cannot
// make the readiness gate effectively infinite. Overflow items are NOT dropped
// — they fall through to the existing background prewarm after the gate opens.
// 512 is ~5x the largest built-in template's 91 pairs (beach-gaelic).
const maxWarmSetItems = 512

// warmItem is one distinct voice the circuit can trigger.
type warmItem struct {
	inst  string
	pitch float64 // rounded to 0.5 st (roundWarmPitch)
}

// warmSet is the deterministic, de-duplicated working set of a loaded circuit.
// Order is stable: rows in row order, pitches in first-appearance order, so a
// truncated set always warms the same prefix.
type warmSet struct {
	items     []warmItem
	bpm       int
	truncated bool // the cap was hit; the tail warms in background post-gate
}

func (s warmSet) Len() int { return len(s.items) }

// buildWarmSet enumerates every (instrument, pitch) the loaded circuit can
// trigger.
//
// It deliberately reuses the SAME walk the incremental prewarmer uses
// (beatInfosByRow + evalNodeParamsOnly) rather than re-deriving pitch: that
// function is the only correct computation (node pitch + model.GroupEffect
// windowed group transpose + the ±24 clamp), and a second derivation would
// drift from dispatch. The ONLY difference from warmFromBeatInfos is the
// horizon — callers must have ensured beatInfosByRow spans a full loop first.
//
// The ±1 pitch neighborhood is deliberately NOT included: it exists so a
// one-tick pitch EDIT lands warm, it is not needed for playback correctness,
// and including it triples the gate wait (91 → 273 items on beach-gaelic).
// The neighborhood keeps warming in the background via warmFromBeatInfos.
func (g *Game) buildWarmSet() warmSet {
	out := warmSet{bpm: g.bpm}
	if g == nil || g.drum == nil {
		return out
	}
	seen := make(map[warmItem]struct{})
	rows := g.beatInfosByRow
	for row := range rows {
		if row >= len(g.drum.Rows) {
			break
		}
		inst := g.drum.Rows[row].Instrument
		if inst == "" {
			continue
		}
		for idx, info := range rows[row] {
			if info.NodeType != model.NodeTypeRegular {
				continue // mute/other node types never schedule a voice
			}
			_, pitch, _ := g.evalNodeParamsOnly(row, idx, info)
			it := warmItem{inst: inst, pitch: roundWarmPitch(pitch)}
			if _, dup := seen[it]; dup {
				continue
			}
			seen[it] = struct{}{}
			if len(out.items) >= maxWarmSetItems {
				out.truncated = true
				return out
			}
			out.items = append(out.items, it)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestBuildWarmSet ./internal/ui/`
Expected: PASS (3 tests).

- [ ] **Step 5: Pin that the warm set is NOT scoped to the visible window**

**Correction to this task as originally written:** an earlier draft added an
`ensureWarmHorizon` helper on the belief that `beatInfosByRow` was a render
window. It is not — it is `graph.CalculateBeatRowFrom(origin)` truncated by
`rawBeatLen` (`game_graph_update_beat_infos.go:94-97`), i.e. the complete
per-row graph path including the loop, and it is the predictor's *input* via
`SetPaths`. **Do not add `ensureWarmHorizon`.** The walk already covers the
circuit.

What is worth pinning is that it stays that way. Append to `warm_set_test.go`:

```go
// TestWarmSetIgnoresTheVisibleWindow pins that the warm set covers the row's
// whole graph path, not the slice of it currently on screen. beatInfosByRow is
// the full path from graph.CalculateBeatRowFrom (NOT a render window), so
// scrolling or shrinking the timeline must not shrink the set. Guards against a
// future "optimization" that scopes warming to the viewport — which would put
// every off-screen pitch back on the cold-render path during playback.
func TestWarmSetIgnoresTheVisibleWindow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	full := g.buildWarmSet().Len()
	if full == 0 {
		t.Fatal("warm set is empty for the default circuit")
	}

	g.drum.Length = max1(g.drum.Length / 4)
	g.drum.Offset += 2
	g.updateBeatInfos()

	if got := g.buildWarmSet().Len(); got != full {
		t.Errorf("warm set changed with the visible window: got %d want %d", got, full)
	}
}
```

- [ ] **Step 6: Run the tests and confirm they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestBuildWarmSet|TestWarmSetIgnoresTheVisibleWindow' ./internal/ui/`
Expected: PASS (4 tests). Leave uncommitted.

---

## Task 4: The warm-up controller (diff, feed, deadline, progress)

**Files:**
- Create: `src/go/internal/ui/warmup_controller.go`
- Create: `src/go/internal/ui/warmup_controller_test.go`
- Modify: `src/go/internal/ui/game_struct.go` (add the `warmup` field)
- Modify: `src/go/internal/ui/game_update.go` (call `g.warmup.Tick`)

**Interfaces:**
- Consumes: `warmSet` / `warmItem` / `(*Game).buildWarmSet()` / `(*Game).ensureWarmHorizon()` (Task 3).
- Produces:
  ```go
  type warmBackend interface {
      Cached(inst string, pitch float64) bool
      Warm(inst string, pitch float64, done func())
  }
  type warmupController struct{ /* unexported */ }
  func newWarmupController(g *Game, be warmBackend) *warmupController
  func (w *warmupController) Begin()                         // builds the set + starts warming
  func (w *warmupController) Tick(nowMs int64)               // deadline + completion; call once per frame
  func (w *warmupController) Ready() bool
  func (w *warmupController) Progress() (done, total int)
  func (w *warmupController) Cancel()
  func (w *warmupController) BenchStats() (size, skippedCached int, ms float64)
  ```

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/warmup_controller_test.go`:

```go
//go:build test

package ui

import (
	"sync"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// fakeWarmBackend records warm requests and completes them on demand, so the
// controller's diff/progress/deadline logic is tested without a real render.
type fakeWarmBackend struct {
	mu      sync.Mutex
	cached  map[warmItem]bool
	asked   []warmItem
	pending []func()
}

func newFakeWarmBackend() *fakeWarmBackend {
	return &fakeWarmBackend{cached: map[warmItem]bool{}}
}

func (f *fakeWarmBackend) Cached(inst string, pitch float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cached[warmItem{inst: inst, pitch: pitch}]
}

func (f *fakeWarmBackend) Warm(inst string, pitch float64, done func()) {
	f.mu.Lock()
	f.asked = append(f.asked, warmItem{inst: inst, pitch: pitch})
	f.pending = append(f.pending, done)
	f.mu.Unlock()
}

// completeAll fires every queued completion callback.
func (f *fakeWarmBackend) completeAll() {
	f.mu.Lock()
	todo := f.pending
	f.pending = nil
	f.mu.Unlock()
	for _, d := range todo {
		d()
	}
}

func newWarmupTestGame(t *testing.T) (*Game, *fakeWarmBackend) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	return g, be
}

// TestWarmupNotReadyUntilEveryItemCompletes is the core gate invariant.
func TestWarmupNotReadyUntilEveryItemCompletes(t *testing.T) {
	g, be := newWarmupTestGame(t)
	g.warmup.Begin()
	done, total := g.warmup.Progress()
	if total == 0 {
		t.Fatal("warm-up began with an empty set")
	}
	if done == total {
		t.Fatal("warm-up reported complete before any item finished")
	}
	if g.warmup.Ready() {
		t.Fatal("Ready() true while items are outstanding")
	}
	be.completeAll()
	g.warmup.Tick(1)
	if !g.warmup.Ready() {
		t.Fatal("Ready() false after every item completed")
	}
}

// TestWarmupSkipsAlreadyCachedItems is the cache-hit requirement: an item the
// backend already holds must never be re-rendered.
func TestWarmupSkipsAlreadyCachedItems(t *testing.T) {
	g, be := newWarmupTestGame(t)
	set := g.buildWarmSet()
	for _, it := range set.items {
		be.cached[it] = true // pretend the whole circuit is already warm
	}
	g.warmup.Begin()
	if len(be.asked) != 0 {
		t.Fatalf("re-warmed %d already-cached items, want 0", len(be.asked))
	}
	if !g.warmup.Ready() {
		t.Fatal("gate did not open immediately for a fully cached circuit")
	}
	_, skipped, _ := g.warmup.BenchStats()
	if skipped != set.Len() {
		t.Errorf("BenchStats skippedCached = %d, want %d", skipped, set.Len())
	}
}

// TestWarmupDeadlineOpensTheGate pins the never-deadlock rule: a backend that
// never completes must still let the user press Play once the deadline passes.
func TestWarmupDeadlineOpensTheGate(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	g.warmup.Begin() // fake backend never calls done()
	if g.warmup.Ready() {
		t.Fatal("Ready() true before any completion")
	}
	g.warmup.Tick(warmupDeadlineMS + 1)
	if !g.warmup.Ready() {
		t.Fatal("gate stayed shut past the deadline — playback is deadlocked")
	}
}

// TestWarmupCancelOpensTheGate pins that a user abort releases the gate.
func TestWarmupCancelOpensTheGate(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	g.warmup.Begin()
	g.warmup.Cancel()
	if !g.warmup.Ready() {
		t.Fatal("gate stayed shut after Cancel")
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWarmup ./internal/ui/`
Expected: FAIL — `newWarmupController undefined`.

- [ ] **Step 3: Implement the controller**

Create `src/go/internal/ui/warmup_controller.go`:

```go
package ui

import (
	"sync"
	"time"
)

// warmupDeadlineMS bounds how long the readiness gate may hold playback. The
// gate MUST never deadlock: a backend that cannot make progress (no render
// worker, WASM module unavailable, throttled background tab) opens the gate
// anyway and playback proceeds with the existing serveBestAvailable /
// neighbor-substitution behaviour — degraded, not blocked.
const warmupDeadlineMS int64 = 15000

// warmBackend renders voices into the platform's voice cache. Cached must use
// the SAME key computation the dispatch path uses, or the diff will re-render
// voices that are already warm (or, worse, skip ones that are not).
type warmBackend interface {
	Cached(inst string, pitch float64) bool
	Warm(inst string, pitch float64, done func())
}

// warmupController gates playback on a warm voice cache after a wholesale
// cache-invalidating event (import, BPM change, sample-rate change).
type warmupController struct {
	g  *Game
	be warmBackend

	mu        sync.Mutex
	active    bool
	total     int
	completed int
	skipped   int
	startMs   int64
	elapsedMs float64
	timedOut  bool

	// onBegin/onUpdate/onEnd drive the progress toast (Task 8). nil-safe.
	onBegin  func(total int)
	onUpdate func(done, total int)
	onEnd    func(elapsedMs float64, rendered, skipped int, timedOut bool)
}

func newWarmupController(g *Game, be warmBackend) *warmupController {
	return &warmupController{g: g, be: be}
}

// Begin rebuilds the warm set, diffs it against the cache, and starts warming
// whatever is missing. A fully cached circuit opens the gate synchronously.
func (w *warmupController) Begin() {
	if w == nil || w.g == nil || w.be == nil {
		return
	}
	w.g.ensureWarmHorizon()
	set := w.g.buildWarmSet()

	todo := make([]warmItem, 0, set.Len())
	skipped := 0
	for _, it := range set.items {
		if w.be.Cached(it.inst, it.pitch) {
			skipped++
			continue
		}
		todo = append(todo, it)
	}

	w.mu.Lock()
	w.total = len(todo)
	w.completed = 0
	w.skipped = skipped
	w.timedOut = false
	w.startMs = time.Now().UnixMilli()
	w.active = len(todo) > 0
	begin, onBegin := w.active, w.onBegin
	w.mu.Unlock()

	if !begin {
		// Everything already cached: no toast, no wait.
		w.finish(false)
		return
	}
	if onBegin != nil {
		onBegin(len(todo))
	}
	for _, it := range todo {
		item := it
		w.be.Warm(item.inst, item.pitch, func() { w.itemDone() })
	}
}

func (w *warmupController) itemDone() {
	w.mu.Lock()
	if !w.active {
		w.mu.Unlock()
		return
	}
	w.completed++
	done, total := w.completed, w.total
	onUpdate := w.onUpdate
	complete := done >= total
	w.mu.Unlock()

	if onUpdate != nil {
		onUpdate(done, total)
	}
	if complete {
		w.finish(false)
	}
}

// Tick enforces the deadline. nowMs is milliseconds since Begin for tests; pass
// 0 in production and the controller reads the wall clock.
func (w *warmupController) Tick(nowMs int64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.active {
		w.mu.Unlock()
		return
	}
	elapsed := nowMs
	if elapsed == 0 {
		elapsed = time.Now().UnixMilli() - w.startMs
	}
	over := elapsed > warmupDeadlineMS
	w.mu.Unlock()
	if over {
		w.finish(true)
	}
}

// Cancel opens the gate immediately (user abort). Outstanding items stay queued
// in the backend as ordinary background warms.
func (w *warmupController) Cancel() { w.finish(false) }

func (w *warmupController) finish(timedOut bool) {
	w.mu.Lock()
	if !w.active && w.startMs == 0 {
		w.mu.Unlock()
		return
	}
	w.active = false
	w.timedOut = timedOut
	if w.startMs > 0 {
		w.elapsedMs = float64(time.Now().UnixMilli() - w.startMs)
	}
	onEnd := w.onEnd
	elapsed, rendered, skipped := w.elapsedMs, w.completed, w.skipped
	w.mu.Unlock()
	if onEnd != nil {
		onEnd(elapsed, rendered, skipped, timedOut)
	}
}

// Ready reports whether playback may start.
func (w *warmupController) Ready() bool {
	if w == nil {
		return true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.active
}

// Progress reports (completed, total) for the progress toast.
func (w *warmupController) Progress() (int, int) {
	if w == nil {
		return 0, 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.completed, w.total
}

// BenchStats feeds the [bench_json] line (Task 1).
func (w *warmupController) BenchStats() (int, int, float64) {
	if w == nil {
		return 0, 0, 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.total + w.skipped, w.skipped, w.elapsedMs
}
```

- [ ] **Step 4: Wire the field, the per-frame Tick, and the bench stats**

Replace Task 1's placeholder zeros in `game_update.go`'s bench line with the
real values:

```go
warmSize, warmSkipped, warmMS := 0, 0, 0.0
if g.warmup != nil {
	warmSize, warmSkipped, warmMS = g.warmup.BenchStats()
}
fmt.Printf(benchJSONFormat, /* …existing args… */, rl.StalePlays,
	warmSize, warmSkipped, warmMS, benchStormName(g.benchStorm), g.benchStormPushes)
```

In `game_struct.go`, next to `voicePrewarmer`:

```go
// warmup gates playback on a warm voice cache after a wholesale
// cache-invalidating event (warmup_controller.go).
warmup *warmupController
```

In `game_update.go`, immediately after `g.drum.logsToast.tick()` (~line 76):

```go
g.warmup.Tick(0) // nil-safe; enforces the readiness deadline
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWarmup ./internal/ui/`
Expected: PASS (4 tests).

- [ ] **Step 6: Run the full UI suite for regressions**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. Leave uncommitted.

---

## Task 5: Desktop warm backend — non-dropping, profile-sized pool

`async.Pool.Submit` drops on backpressure by contract (`ErrBackpressure` + a drop counter). The gated warm-up must never drop: the controller's feeder blocks instead.

**Files:**
- Create: `src/go/internal/ui/warmup_backend_desktop.go` (`//go:build !js`)
- Create: `src/go/internal/ui/warmup_backend_desktop_test.go`
- Modify: `src/go/internal/ui/runtime_profile.go` (add `PrewarmWorkers`)
- Modify: `src/go/internal/ui/voice_prewarmer.go` (read the profile)
- Modify: `src/go/internal/async/registry.go` (`minDefaultBudget`)
- Modify: `src/go/internal/async/registry_footprint_test.go` (`standingFootprint`)
- Modify: `CLAUDE.md` (standing-pool table)

**Interfaces:**
- Consumes: `audio.VoiceCached(id string, pitch float64) bool`, `audio.PrewarmVoice(id string, pitch float64)`, `async.Pool.SubmitBlocking(ctx, Job) error`, `RuntimeProf().PrewarmWorkers`.
- Produces: `func newDesktopWarmBackend(pool *async.Pool) warmBackend`.

- [ ] **Step 1: Write the failing budget test**

The three-edit rule is enforced by an existing test; make it fail first by raising the footprint. In `src/go/internal/async/registry_footprint_test.go`, change the `audio.prewarm` row:

```go
	{"audio.prewarm", 4, "internal/ui/voice_prewarmer.go (RuntimeProf().PrewarmWorkers, desktop)"},
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDefaultBudgetFloor|TestDefaultRegistryFits' ./internal/async/`
Expected: FAIL — `minDefaultBudget = 11, want >= 13`.

- [ ] **Step 3: Bump the floor and document it**

In `src/go/internal/async/registry.go`, update the constant **and its comment arithmetic**:

```go
// minDefaultBudget is the floor for the default budget (see NewRegistry).
// It MUST cover the whole standing-pool footprint documented in CLAUDE.md:
//
//	hooks.fanout 2 + eventlogger.format 1 + eventstream.persist 1 +
//	audio.timers 1 + audio.prewarm 4 + synth.preview 1 +
//	recording.lifecycle 1 + ui.dialog 1 + userprefs.persist 1 = 13
//
// audio.prewarm went 2 → 4 with the import warm-up gate (2026-09-13): the
// gated warm set is fed with SubmitBlocking, so worker count is the gate's
// wall-clock divisor.
const minDefaultBudget = 13
```

In `CLAUDE.md`, update the standing-pool table row:

```
| `audio.prewarm` | `internal/ui/voice_prewarmer.go` | 4 / 64 — predictive voice render + gated import warm-up |
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/async/`
Expected: PASS.

- [ ] **Step 4: Add the RuntimeProfile field**

In `src/go/internal/ui/runtime_profile.go`, add to the struct (next to `PredictiveWarmTicks`):

```go
	// PrewarmWorkers is the audio.prewarm pool's worker count. It is the
	// gated warm-up's wall-clock divisor: the measured desktop warm set is
	// ~1.4 s of serial CGo for 96 pairs, so 4 workers put a cold import
	// under the ~600 ms target. Browser keeps 1 — Go goroutines do not
	// parallelise on js/wasm (NumCPU()==1); the browser parallelises in its
	// own render-worker pool instead (see audio.js).
	PrewarmWorkers int
```

In `browserRuntimeProfile()` next to `PredictiveWarmTicks: 1,`:

```go
		PrewarmWorkers:                     1, // js/wasm is single-threaded; parallelism lives in audio.js workers
```

In `desktopRuntimeProfile()`:

```go
		PrewarmWorkers:             4, // 96-pair warm set: 1.42s serial → ~0.4s (measured 2026-09-13)
```

In `voice_prewarmer.go`, replace the hardcoded worker count:

```go
	// Workers come from the runtime profile: the gated import warm-up (see
	// warmup_controller.go) feeds this pool with SubmitBlocking, so worker
	// count is the readiness gate's wall-clock divisor.
	workers := RuntimeProf().PrewarmWorkers
	if workers < 1 {
		workers = 1
	}
	opts := async.Options{MaxConcurrent: workers, QueueSize: 64, Name: prewarmPoolName}
```

- [ ] **Step 5: Write the failing backend test**

Create `src/go/internal/ui/warmup_backend_desktop_test.go`:

```go
//go:build test && !js

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// TestDesktopWarmBackendNeverDropsWork pins the non-dropping contract: the
// gated warm set must be fed with SubmitBlocking, never Submit. Submit drops on
// backpressure by design (ErrBackpressure + a drop counter), which is exactly
// how 273 warm requests into a 64-deep queue silently lost most of the working
// set before this change.
func TestDesktopWarmBackendNeverDropsWork(t *testing.T) {
	const poolName = "test.warmup.nodrop"
	pool, err := async.DefaultRegistry().Get(poolName, async.Options{
		MaxConcurrent: 1, QueueSize: 2, Name: poolName,
	})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(func() { async.DefaultRegistry().Release(poolName) })

	be := newDesktopWarmBackend(pool)
	const n = 64 // 32x the queue depth
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		be.Warm("kick", float64(i)/2, func() { done <- struct{}{} })
	}
	for i := 0; i < n; i++ {
		<-done // blocks forever (test timeout) if any item was dropped
	}
	if d := pool.Stats().Dropped; d != 0 {
		t.Errorf("pool dropped %d jobs; the gated warm set must never drop", d)
	}
}
```

- [ ] **Step 6: Run it and confirm it fails, then implement**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesktopWarmBackend ./internal/ui/`
Expected: FAIL — `newDesktopWarmBackend undefined`.

Create `src/go/internal/ui/warmup_backend_desktop.go`:

```go
//go:build !js

package ui

import (
	"context"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// desktopWarmBackend renders gated warm items on the audio.prewarm pool.
//
// It uses SubmitBlocking, NOT Submit. Submit is non-blocking by contract and
// DROPS on backpressure — that is how the pre-gate prewarmer lost most of a
// 273-item working set into a 64-deep queue. A gated warm set must complete,
// so the feeder blocks instead. Blocking is safe here: Warm is called from
// warmupController.Begin on the UI goroutine at import time, when no frame
// deadline is being missed (the toast is already up and Play is already
// disabled), and the queue drains at worker speed.
type desktopWarmBackend struct{ pool *async.Pool }

func newDesktopWarmBackend(pool *async.Pool) warmBackend {
	return &desktopWarmBackend{pool: pool}
}

// Cached delegates to the same probe the dispatch path reads, so an item
// already rendered under the current bpm/params/pitch key is never re-rendered.
func (b *desktopWarmBackend) Cached(inst string, pitch float64) bool {
	return audio.VoiceCached(inst, pitch)
}

func (b *desktopWarmBackend) Warm(inst string, pitch float64, done func()) {
	if b.pool == nil {
		done() // no pool: count it complete so the gate cannot deadlock
		return
	}
	err := b.pool.SubmitBlocking(context.Background(), func(context.Context) {
		audio.PrewarmVoice(inst, pitch)
		done()
	})
	if err != nil {
		done() // closed pool: never leave the gate waiting on a job that cannot run
	}
}
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesktopWarmBackend ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Verify the whole async + ui suites**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/async/ ./internal/ui/`
Expected: PASS. Leave uncommitted.

---

## Task 6: Browser warm backend — de-stub the js prewarm path

`voice_prewarm_js.go` stubs the entire prewarmer to no-ops, and `VoiceCached` returns a hardcoded `false`, so the browser cannot tell what is already cached.

**Files:**
- Modify: `src/go/internal/audio/voice_prewarm_js.go`
- Modify: `src/js/audio.js` (add `window.warmInstrumentBatch`, `window.voiceCachedFor`)
- Create: `src/go/internal/ui/warmup_backend_js.go` (`//go:build js`)
- Create: `src/js/warm_batch.browser.test.js`

**Interfaces:**
- Consumes: JS globals `warmInstrumentBatch(itemsJSON, doneCallbackName)` and `voiceCachedFor(id, pitch) -> bool`; `ensureRenderReady(id, pitch)`, `cacheKeyFor`, `roundPitchForCache`, `renderCache`, `paramsGenOf`, `renderRecordStale` (all already in `audio.js`).
- Produces: `func newBrowserWarmBackend() warmBackend`; `audio.VoiceCached` becomes a real query on js.

- [ ] **Step 1: Write the failing browser test**

Create `src/js/warm_batch.browser.test.js`:

```js
/**
 * Pins the browser warm bridge: warmInstrumentBatch must render every
 * requested (instrument, pitch) and report completion, and voiceCachedFor must
 * report TRUE for an item that is already in renderCache.
 *
 * Before this change voice_prewarm_js.go stubbed VoiceCached to a hardcoded
 * false, so the browser re-rendered voices it already held.
 */
import assert from "assert";
import { setupFullWasm, clickAt } from "./real_input_test_helpers.js";

(async () => {
  const { page, cleanup } = await setupFullWasm({ logLevel: "ERROR" });
  try {
    await clickAt(page, 640, 400);
    await page.waitForTimeout(300);

    const res = await page.evaluate(async () => {
      const items = [["cello", 0], ["cello", 7], ["cello", 12]];
      const before = items.map(([id, p]) => window.voiceCachedFor(id, p));
      let completed = 0;
      await new Promise((resolve) => {
        window.__warmBatchDone = () => { completed++; if (completed === items.length) resolve(); };
        window.warmInstrumentBatch(JSON.stringify(items), "__warmBatchDone");
        setTimeout(resolve, 60000);
      });
      const after = items.map(([id, p]) => window.voiceCachedFor(id, p));
      return { before, after, completed, n: items.length };
    });

    assert.strictEqual(res.completed, res.n, `only ${res.completed}/${res.n} warms completed`);
    assert.ok(res.after.every(Boolean), `voiceCachedFor false after warm: ${JSON.stringify(res.after)}`);
    console.log("[warm-batch] PASS", JSON.stringify(res));
  } finally {
    await cleanup();
  }
})();
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/warm_batch.browser.test.js`
Expected: FAIL — `window.warmInstrumentBatch is not a function`.

- [ ] **Step 3: Add the JS bridge**

In `src/js/audio.js`, next to `window.warmInstrument` (~line 2721):

```js
// voiceCachedFor answers "is this exact (instrument, pitch) already rendered
// and fresh?" using the SAME key the trigger path reads (cacheKeyFor over the
// rounded pitch) plus the param-generation staleness check. Go's
// audio.VoiceCached delegates here on js; before this existed it returned a
// hardcoded false, so the warm-up re-rendered voices already in the cache.
window.voiceCachedFor = (id, pitch) => {
  try {
    if (!RENDER[id]) return false;
    const rp = (isMelodicInstrument(id) && Number.isFinite(pitch)) ? roundPitchForCache(pitch) : 0;
    const rec = renderCache.get(cacheKeyFor(id, rp));
    return !!rec && !renderRecordStale(id, rec);
  } catch (_) { return false; }
};

// warmInstrumentBatch renders a whole working set off-thread and invokes the
// named global callback ONCE PER ITEM (success or failure). Failures still call
// back: the readiness gate must never wait on an item that can never complete.
// itemsJSON is [[instrumentID, pitch], …].
window.warmInstrumentBatch = (itemsJSON, doneFnName) => {
  let items = [];
  try { items = JSON.parse(itemsJSON); } catch (_) { items = []; }
  const fire = () => {
    try { const f = window[doneFnName]; if (typeof f === "function") f(); }
    catch (err) { dbg('warm.batch.cb.error', { err: String(err) }); }
  };
  if (!Array.isArray(items) || items.length === 0) return;
  for (const entry of items) {
    const id = entry && entry[0];
    const pitch = entry && entry[1];
    if (!id || !RENDER[id]) { fire(); continue; }
    const melodic = isMelodicInstrument(id);
    ensureRenderReady(id, melodic ? pitch : undefined).then(fire).catch(fire);
  }
};
```

- [ ] **Step 4: De-stub the Go js side**

Replace the bodies in `src/go/internal/audio/voice_prewarm_js.go`:

```go
//go:build js

package audio

import (
	"encoding/json"
	"syscall/js"
)

// PrewarmVoice renders (id, pitch) via the JS render worker. It is fire-and-
// forget; the gated warm-up uses WarmVoiceBatch instead, which reports
// completion. Before 2026-09-13 this was a no-op, so the browser did no
// Go-driven warming at all.
func PrewarmVoice(id string, pitch float64) {
	if id == "" {
		return
	}
	fn := js.Global().Get("warmInstrument")
	if !fn.Truthy() {
		return
	}
	data, err := json.Marshal([]float64{pitch})
	if err != nil {
		return
	}
	fn.Invoke(id, string(data))
}

// PrewarmVoiceParams mirrors PrewarmVoice on browser: per-instrument params are
// pushed to audio.js separately (updateInstrumentParams), so the param snapshot
// is already live by the time the render runs.
func PrewarmVoiceParams(id string, pitch float64, params RecipeParams) {
	PrewarmVoice(id, pitch)
}

// VoiceCached queries the JS renderCache through the SAME key the trigger path
// reads. Returning a hardcoded false (the pre-2026-09-13 behaviour) made every
// already-warm voice look cold, so the warm-up could not skip anything.
func VoiceCached(id string, pitch float64) bool {
	fn := js.Global().Get("voiceCachedFor")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(id, pitch).Truthy()
}

// IsMelodicInstrument reports whether id renders at-pitch, so only melodic
// instruments get per-pitch warm items. Delegates to audio.js, which owns the
// RENDER_INFO table the render path uses.
func IsMelodicInstrument(id string) bool {
	fn := js.Global().Get("isMelodicInstrumentJS")
	if !fn.Truthy() {
		return false
	}
	return fn.Invoke(id).Truthy()
}

// WarmVoiceBatch renders every item off-thread, invoking done once per item
// (success or failure). Used by the readiness gate.
func WarmVoiceBatch(items [][2]any, done func()) {
	fn := js.Global().Get("warmInstrumentBatch")
	if !fn.Truthy() {
		for range items {
			done()
		}
		return
	}
	const cbName = "__beatmoWarmBatchDone"
	js.Global().Set(cbName, js.FuncOf(func(js.Value, []js.Value) any { done(); return nil }))
	data, err := json.Marshal(items)
	if err != nil {
		for range items {
			done()
		}
		return
	}
	fn.Invoke(string(data), cbName)
}

// SetDispatchMustNotRender is a no-op on browser (renders already run on the
// render worker; there is no dispatch-goroutine render path to gate).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op on browser, mirroring SetDispatchMustNotRender.
func SetAsyncRenderSubmit(f func(id string, pitch float64, params RecipeParams)) {}
```

Add the companion JS export used by `IsMelodicInstrument` in `audio.js`:

```js
// isMelodicInstrumentJS exposes the render layer's melodic predicate to Go-WASM
// so the warm set only mints per-pitch items for instruments that actually keep
// a per-pitch cache.
window.isMelodicInstrumentJS = (id) => { try { return !!isMelodicInstrument(id); } catch (_) { return false; } };
```

- [ ] **Step 5: Add the browser backend**

Create `src/go/internal/ui/warmup_backend_js.go`:

```go
//go:build js

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// browserWarmBackend renders gated warm items through the JS render worker
// pool. Unlike desktop there is no Go-side pool: js/wasm is single-threaded, so
// parallelism lives entirely in audio.js's render workers.
type browserWarmBackend struct{}

func newBrowserWarmBackend() warmBackend { return browserWarmBackend{} }

func (browserWarmBackend) Cached(inst string, pitch float64) bool {
	return audio.VoiceCached(inst, pitch)
}

func (browserWarmBackend) Warm(inst string, pitch float64, done func()) {
	audio.WarmVoiceBatch([][2]any{{inst, pitch}}, done)
}
```

- [ ] **Step 6: Rebuild WASM and run the test**

```bash
cd /home/ymolinar/Repos/beatmo && make wasm
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/warm_batch.browser.test.js
```
Expected: `[warm-batch] PASS` with `after` all `true`.

- [ ] **Step 7: Run the bridge catalogue + drift guards**

```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run JSExports
```
Per the JS Test Charter, add `warmInstrumentBatch`, `voiceCachedFor` and `isMelodicInstrumentJS` to the catalogue in `wasm_bridge_smoke.browser.test.js` rather than creating more dedicated files. Leave uncommitted.

---

## Task 7: Widen the browser render-worker pool

Measured: 1 worker → 108 ms/pair → **7.79 s** for a 91-pair circuit.

**Files:**
- Modify: `src/js/audio.js` (render worker creation + `renderVoiceInWorker` dispatch)
- Modify: `src/js/webaudio_import_cold_bench.browser.test.js` (no change needed; it reports the improvement)

**Interfaces:**
- Consumes: existing `synthWorkerPending`, `renderVoiceInWorker(renderFn, sr, frames, paramArr, allocCount, ampVal)`.
- Produces: same `renderVoiceInWorker` signature — callers unchanged.

- [ ] **Step 1: Locate the current single-worker construction**

Run: `grep -n "renderVoiceInWorker\|synthWorkerPending\|new Worker" src/js/audio.js`
Read the worker creation site and the pending-map keying before editing. The pool must preserve: the existing message protocol, `synthWorkerPending.size` as the queue-depth metric, and the promise-per-request contract.

- [ ] **Step 2: Replace the single worker with a round-robin pool**

Keep the existing per-request protocol; change only *which* worker receives it:

```js
// Render worker pool. ONE worker serialized a 91-pair circuit warm at
// ~108ms/pair = 7.8s (measured 2026-09-13, beach-gaelic). Requests are
// independent pure renders, so they round-robin across a small pool.
// hardwareConcurrency-1 leaves a core for the main thread + audio thread.
const RENDER_WORKER_MAX = 4;
function renderWorkerCount() {
  const hc = (typeof navigator !== 'undefined' && navigator.hardwareConcurrency) || 2;
  return Math.max(1, Math.min(RENDER_WORKER_MAX, hc - 1));
}
let _renderWorkers = null;
let _renderWorkerNext = 0;
function renderWorkerPool() {
  if (!_renderWorkers) {
    _renderWorkers = [];
    for (let i = 0; i < renderWorkerCount(); i++) _renderWorkers.push(createRenderWorker());
  }
  return _renderWorkers;
}
function nextRenderWorker() {
  const pool = renderWorkerPool();
  const w = pool[_renderWorkerNext % pool.length];
  _renderWorkerNext++;
  return w;
}
```

Refactor the existing single-worker creation into `createRenderWorker()` (same `new Worker(...)`, same `onmessage` handler resolving out of `synthWorkerPending`), and change `renderVoiceInWorker` to post to `nextRenderWorker()` instead of the module-level worker. **Do not change the request id scheme** — `synthWorkerPending` stays a single map keyed by request id, so `workerQueuePeak` keeps its meaning across the whole pool.

- [ ] **Step 3: Verify the existing worker tests still pass**

```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/worklet_insert_effects.browser.test.js
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/webaudio_smoke.browser.test.js
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/warm_batch.browser.test.js
```
Expected: all PASS.

- [ ] **Step 4: Verify cross-platform parity is unaffected**

Renders must stay bit-identical — the pool changes *where* a render runs, never *what* it computes.

```bash
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/xplat_insert_fx_parity.browser.test.js
```
Expected: PASS. If either fails, the pool changed render inputs — revert and re-derive.

- [ ] **Step 5: Re-measure the eager warm**

```bash
cd /home/ymolinar/Repos/beatmo
GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_import_cold_bench.browser.test.js
```
Expected: `gateMs` materially below the 7790 ms single-worker baseline (target ~2000 ms). Record the number. Leave uncommitted.

- [ ] **Step 6: Attribute the main-thread stall**

The baseline measured a **716 ms max main-thread event-loop gap** during eager warm. Determine the cause before attempting a fix — do not guess. Run the bench with Playwright tracing or a `performance.now()` sampling loop and check, in order: (a) the `Float32Array` result transfer out of the worker (is it structured-cloned instead of transferred?), (b) `ensureAudioBuffer` creating AudioBuffers synchronously in a burst, (c) headless-Chrome CPU contention (compare `renderWorkerCount()` of 1 vs 4). **Record the finding in the task notes.** If it is (a) or (b), fix it; if it is (c), document it as a harness artifact and move on.

---

## Task 8: `logsToast` sticky determinate task mode + its own grid layer

**Files:**
- Modify: `src/go/internal/ui/logs_toast.go`
- Modify: `src/go/internal/ui/grid_tree.go` (add `GZToast`)
- Modify: `src/go/internal/ui/grid_pane_layers.go` (register `grid-toast`)
- Modify: `src/go/internal/ui/grid_pane_draw.go` (remove the toast draw from `drawGridHelpButton`; add `drawGridToast`)
- Modify: `src/go/internal/ui/grid_layer_zorder_test.go` (new row)
- Create: `src/go/internal/ui/logs_toast_task_test.go`

**Interfaces:**
- Consumes: `drawRect`, `drawRoundedRect`, `fitRadius`, `RadiusContainer`, `DrawTextColorAt`, `TextWidth`, `TextHeight`, `SpaceSM`, `SpaceMD`, `colSurface3`, `colError`, `colPrimary`, `colTextPrimary`, `i18n.Tf`, `i18n.Key`.
- Produces:
  ```go
  func (t *logsToast) beginTask(key i18n.Key, total int)
  func (t *logsToast) updateTask(done int)
  func (t *logsToast) endTask()
  func (t *logsToast) taskActive() bool
  func (g *Game) drawGridToast(dst *ebiten.Image)
  const GZToast = 85
  ```

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/logs_toast_task_test.go`:

```go
//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestToastTaskIsSticky pins that a task toast does NOT expire on the error
// toast's TTL. A progress indicator that vanishes after 4s while the work is
// still running is worse than none.
func TestToastTaskIsSticky(t *testing.T) {
	var tt logsToast
	tt.beginTask(i18n.KeyNotifCircuitLoading, 10)
	for i := 0; i < logsToastTTLFrames*3; i++ {
		tt.tick()
	}
	if !tt.active() {
		t.Fatal("task toast expired on the error TTL; it must be sticky")
	}
	tt.endTask()
	if tt.active() {
		t.Fatal("task toast still active after endTask")
	}
}

// TestToastTaskRendersLocalisedProgress pins that the label is built from an
// i18n key each frame (so a mid-load locale switch is reflected) and carries
// the done/total counts.
func TestToastTaskRendersLocalisedProgress(t *testing.T) {
	var tt logsToast
	tt.beginTask(i18n.KeyNotifCircuitLoading, 120)
	tt.updateTask(42)
	got := tt.label()
	if !strings.Contains(got, "42") || !strings.Contains(got, "120") {
		t.Errorf("task label %q missing progress counts", got)
	}
}

// TestToastErrorPathUnchanged pins that the pre-existing error toast behaviour
// (plain string, TTL expiry) is untouched by the task mode.
func TestToastErrorPathUnchanged(t *testing.T) {
	var tt logsToast
	tt.push("boom")
	if !tt.active() {
		t.Fatal("error toast inactive right after push")
	}
	for i := 0; i < logsToastTTLFrames+1; i++ {
		tt.tick()
	}
	if tt.active() {
		t.Fatal("error toast did not expire on its TTL")
	}
}
```

- [ ] **Step 2: Add the i18n keys**

In `src/go/internal/i18n/keys.go`, next to the other `KeyNotif*` entries:

```go
	KeyNotifCircuitLoading         Key = "notif.circuit_loading"
	KeyNotifCircuitReady           Key = "notif.circuit_ready"
	KeyNotifCircuitWarmupTimedOut  Key = "notif.circuit_warmup_timed_out"
```

In `catalog_en.go`:

```go
	KeyNotifCircuitLoading:        "Loading circuit… %s/%s",
	KeyNotifCircuitReady:          "Circuit ready (%s sounds in %s ms, %s cached)",
	KeyNotifCircuitWarmupTimedOut: "Circuit still loading — starting anyway (%s/%s sounds ready)",
```

In `catalog_es.go`:

```go
	KeyNotifCircuitLoading:        "Cargando circuito… %s/%s",
	KeyNotifCircuitReady:          "Circuito listo (%s sonidos en %s ms, %s en caché)",
	KeyNotifCircuitWarmupTimedOut: "El circuito sigue cargando — iniciando igualmente (%s/%s sonidos listos)",
```

- [ ] **Step 3: Run the test and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestToast ./internal/ui/`
Expected: FAIL — `tt.beginTask undefined`.

- [ ] **Step 4: Implement the task mode**

Rewrite `src/go/internal/ui/logs_toast.go`:

```go
package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logsToastTTLFrames is how long an ERROR toast stays visible (~4s at 60fps).
// Task toasts ignore it entirely — see sticky.
const logsToastTTLFrames = 240

// toastProgressBarH is the progress track height in pixels. Drawn with drawRect
// (not drawRoundedRect) so the collectFilledRects test interceptor can
// pixel-assert it: text drawn to a SubImage does NOT propagate to the parent
// under ebitenstub, so the bar is the only assertable evidence the toast
// rendered.
const toastProgressBarH = 4

// logsToast is a single-slot transient status surface at the top of the grid
// pane. It holds EITHER a transient error alert (plain text, TTL) OR a sticky
// determinate task (i18n key + done/total, no TTL, cleared by endTask).
//
// One slot on purpose: an error during a load replaces the progress line, which
// is the right priority. The full message of both lives on in the Logs panel.
type logsToast struct {
	text   string // error mode: frozen string
	frames int    // error mode: remaining visible frames; 0 = inactive

	sticky bool     // task mode: ignore the TTL
	key    i18n.Key // task mode: re-rendered every frame, so a locale switch mid-task is reflected
	done   int
	total  int
}

func (t *logsToast) active() bool { return t.sticky || t.frames > 0 }

// taskActive reports whether a sticky task (not an error) is showing.
func (t *logsToast) taskActive() bool { return t.sticky }

// dismiss hides the toast immediately (used by "Clear history").
func (t *logsToast) dismiss() { t.frames = 0; t.text = ""; t.sticky = false; t.key = "" }

// push shows an ERROR toast for the full TTL. An error takes the slot even from
// a running task — a failure is more urgent than progress.
func (t *logsToast) push(text string) {
	t.text = text
	t.frames = logsToastTTLFrames
	t.sticky = false
	t.key = ""
}

// beginTask starts a sticky determinate task. total <= 0 renders as
// indeterminate (label only, no bar).
func (t *logsToast) beginTask(key i18n.Key, total int) {
	t.key = key
	t.total = total
	t.done = 0
	t.sticky = true
	t.text = ""
	t.frames = 0
}

func (t *logsToast) updateTask(done int) {
	if !t.sticky {
		return
	}
	if done < 0 {
		done = 0
	}
	if t.total > 0 && done > t.total {
		done = t.total
	}
	t.done = done
}

func (t *logsToast) endTask() {
	if !t.sticky {
		return
	}
	t.sticky = false
	t.key = ""
	t.done, t.total = 0, 0
}

// tick counts down one frame; call once per frame. A sticky task never expires.
func (t *logsToast) tick() {
	if t.sticky {
		return
	}
	if t.frames > 0 {
		t.frames--
	}
}

// label renders the current line in the CURRENT locale. Task mode re-resolves
// the i18n key every call so a mid-task language switch is reflected; error
// mode returns its frozen string.
func (t *logsToast) label() string {
	if t.sticky && t.key != "" {
		return i18n.Tf(t.key, strconv.Itoa(t.done), strconv.Itoa(t.total))
	}
	return t.text
}

// progress returns the 0..1 fill fraction, and false when indeterminate.
func (t *logsToast) progress() (float64, bool) {
	if !t.sticky || t.total <= 0 {
		return 0, false
	}
	return float64(t.done) / float64(t.total), true
}

// rectIn returns the toast's on-screen rect within the grid pane (empty when
// inactive). The toast is passive: it registers no hit area and never takes
// input.
func (t *logsToast) rectIn(gridRect image.Rectangle) image.Rectangle {
	if !t.active() {
		return image.Rectangle{}
	}
	label := t.label()
	if label == "" {
		return image.Rectangle{}
	}
	w := TextWidth(label) + 2*SpaceMD
	h := TextHeight() + 2*SpaceSM
	if _, ok := t.progress(); ok {
		h += toastProgressBarH + SpaceSM
	}
	cx := gridRect.Min.X + gridRect.Dx()/2
	top := gridRect.Min.Y + SpaceMD
	return image.Rect(cx-w/2, top, cx+w/2, top+h)
}

// draw paints the toast at the top-centre of the supplied host rect.
func (t *logsToast) draw(dst *ebiten.Image, hostRect image.Rectangle) {
	r := t.rectIn(hostRect)
	if r.Empty() {
		return
	}
	accent := colError
	if t.sticky {
		accent = colPrimary
	}
	rad := fitRadius(RadiusContainer, r)
	drawRoundedRect(dst, r, colSurface3, rad, true)
	drawRoundedRect(dst, r, accent, rad, false)

	textCol := colError
	if t.sticky {
		textCol = colTextPrimary
	}
	DrawTextColorAt(dst, t.label(), r.Min.X+SpaceMD, r.Min.Y+SpaceSM, textCol)

	if frac, ok := t.progress(); ok {
		bar := image.Rect(r.Min.X+SpaceMD, r.Max.Y-SpaceSM-toastProgressBarH,
			r.Max.X-SpaceMD, r.Max.Y-SpaceSM)
		// drawRect (not rounded): the pixel-test interceptor collects filled
		// rects, and this bar is the toast's only assertable evidence.
		drawRect(dst, bar, colSurface2)
		fillW := int(float64(bar.Dx()) * frac)
		if fillW > 0 {
			drawRect(dst, image.Rect(bar.Min.X, bar.Min.Y, bar.Min.X+fillW, bar.Max.Y), colPrimary)
		}
	}
}
```

- [ ] **Step 5: Give the toast its own grid layer**

In `grid_tree.go`, add after `GZGridHelpButton = 80`:

```go
	GZToast = 85
```

In `grid_pane_draw.go`, delete the trailing toast block from `drawGridHelpButton`:

```go
	// DELETE these lines from drawGridHelpButton:
	//	if g.drum != nil {
	//		g.drum.logsToast.draw(dst, g.split.GridRect(g.winW, g.winH))
	//	}
```

and add a dedicated draw method:

```go
// drawGridToast paints the transient status toast (error alert or sticky task
// progress). Its own layer at GZToast, ABOVE the top-right button cluster: it
// used to be painted from inside drawGridHelpButton, so a long toast could be
// overdrawn by the cluster.
func (g *Game) drawGridToast(dst *ebiten.Image) {
	if g.drum == nil {
		return
	}
	g.drum.logsToast.draw(dst, g.split.GridRect(g.winW, g.winH))
}
```

In `grid_pane_layers.go`, after the help-button registration:

```go
	g.gridTree.RegisterLayer(gridLayer{id: "grid-toast", z: GZToast, clip: true, draw: g.drawGridToast})
```

In `grid_layer_zorder_test.go`, append to `want`:

```go
		{"grid-toast", GZToast},
```

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestToast|TestGridLayerZOrder|TestGridTreeDrawHasSingleDispatchSite' ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Run the token/glyph/geometry discipline guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestNoPillShapedControls|TestNoForbiddenChromeGlyphs' ./internal/ui/
```
Expected: PASS. If `TestTokenDiscipline` fails, the ratchet needs the per-file budget updated in `token_discipline_test.go` — do not add a literal to satisfy it. Leave uncommitted.

---

## Task 9: Mobile fallback placement

On mobile the bottom nav switches view modes; a grid-pane-only toast is invisible exactly when the user is waiting.

**Files:**
- Modify: `src/go/internal/ui/drumview_tree.go` (register a toast layer) or `src/go/internal/ui/grid_pane_draw.go` (host rect selection)
- Create: `src/go/internal/ui/logs_toast_mobile_test.go`

**Interfaces:**
- Consumes: `Profile().IsMobile()`, `dv.currentViewMode`, `dv.Bounds`, `(*logsToast).draw(dst, hostRect)` (Task 8).
- Produces: `func (g *Game) toastHostRect() image.Rectangle`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/logs_toast_mobile_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestToastVisibleInEveryMobileViewMode pins the placement rule: the progress
// toast must land inside the currently VISIBLE surface in every mobile view
// mode. A grid-pane-only toast is invisible in EQ/Wave/Spectrum modes — exactly
// when the user is staring at a disabled Play button wondering why.
func TestToastVisibleInEveryMobileViewMode(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844) // portrait mobile

	g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, 100)
	g.drum.logsToast.updateTask(42)

	host := g.toastHostRect()
	if host.Empty() {
		t.Fatal("toast host rect is empty on mobile")
	}
	r := g.drum.logsToast.rectIn(host)
	if r.Empty() {
		t.Fatal("toast rect empty despite an active task")
	}
	screen := image.Rect(0, 0, 390, 844)
	if !r.In(screen) {
		t.Errorf("toast rect %v is outside the screen %v", r, screen)
	}
}
```

Add `"image"` to the import block.

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestToastVisibleInEveryMobileViewMode ./internal/ui/`
Expected: FAIL — `g.toastHostRect undefined`.

- [ ] **Step 3: Implement the host-rect selection**

In `grid_pane_draw.go` next to `drawGridToast`:

```go
// toastHostRect picks the surface the status toast renders into.
//
// Desktop, and mobile while the grid pane is the active view: the grid pane,
// top-centre — the historical position, zero new layout.
//
// Mobile in a non-grid view mode (EQ / Wave / Spectrum / Levels / Chain /
// Synth): the drum pane instead, so the toast is pinned above the bottom nav
// and stays visible. One toast object, one draw method, two host rects.
func (g *Game) toastHostRect() image.Rectangle {
	gridRect := g.split.GridRect(g.winW, g.winH)
	if !Profile().IsMobile() || g.drum == nil {
		return gridRect
	}
	if g.drum.gridPaneVisibleForToast() {
		return gridRect
	}
	return g.drum.Bounds
}
```

Add the predicate in `drumview_tree.go` (or next to the view-mode enum), expressed via the existing visibility predicate rather than a new `if-else` ladder:

```go
// gridPaneVisibleForToast reports whether the grid pane is the surface the user
// is currently looking at on mobile. Mirrors the view-mode visibility
// predicates the tree already uses — do NOT add a per-mode switch here.
func (dv *DrumView) gridPaneVisibleForToast() bool {
	return !Profile().IsMobile() || dv.currentViewMode == viewModePads
}
```

Change `drawGridToast` to use it:

```go
	g.drum.logsToast.draw(dst, g.toastHostRect())
```

If the toast host is the drum pane, the toast must also be drawn by the DrumViewTree — register a passive layer at the top z in `drumview_tree.go`:

```go
	dv.tree.RegisterLayerVisible(toastLayer{dv: dv}, DVZToast, func() bool {
		return Profile().IsMobile() && !dv.gridPaneVisibleForToast() && dv.logsToast.active()
	})
```

Add `DVZToast` above the existing top DrumView z constant. Find the current top
constant and the expected-list format with:

```bash
grep -n "DVZ[A-Za-z]* *=" src/go/internal/ui/drumview_tree.go | tail -5
grep -n "want := \[\]expected{" -A40 src/go/internal/ui/drumview_layer_zorder_test.go | tail -12
```

Then add the matching `{"drumview-toast", DVZToast},` row at the END of that
`want` slice (it is the topmost layer). `TestDrumViewLayerZOrder` asserts both
count and order, so the row is mandatory, not optional.

- [ ] **Step 4: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestToast|TestDrumViewLayerZOrder|TestZoneInvisibleEqualsNoInput' ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Verify input isolation is unaffected**

The toast is passive — it must register **no** hit area, so it cannot swallow a tap.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestZoneInputIsolation|TestZoneInvisibleNotInHitIndex|TestPadsInput' ./internal/ui/`
Expected: PASS. Leave uncommitted.

---

## Task 10: The playback gate

`dv.playPressed` is the single chokepoint: the Play button, spacebar (`game_update.go:998`), the `startPlay`/`togglePlay` JS exports, `TriggerPlayPause`, and the media-session handler all set it, and `Game.Update` consumes it at `game_update.go:826`.

**Files:**
- Modify: `src/go/internal/ui/game_update.go:826`
- Modify: `src/go/internal/ui/transport_zone.go` (Play button `Disabled`)
- Create: `src/go/internal/ui/warmup_gate_test.go`

**Interfaces:**
- Consumes: `(*warmupController).Ready()`, `(*warmupController).Progress()`, `Button.Disabled`.
- Produces: `func (g *Game) playbackReady() bool`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/warmup_gate_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestPlayBlockedWhileWarming drives the REAL transport path (playPressed →
// Game.Update) rather than poking state, per the project's end-to-end testing
// rule: a component test can pass while the feature is dead.
func TestPlayBlockedWhileWarming(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.warmup.Begin()
	if g.playbackReady() {
		t.Fatal("playbackReady() true while the warm set is outstanding")
	}

	g.drum.TriggerPlayPause()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.Playing() {
		t.Fatal("playback started while the voice cache was cold")
	}

	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("playbackReady() false after the warm set completed")
	}
	g.drum.TriggerPlayPause()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.Playing() {
		t.Fatal("playback did not start after the gate opened")
	}
	stopPlaybackForTest(t, g)
}

// TestPlayButtonDisabledWhileWarming pins the visible affordance.
func TestPlayButtonDisabledWhileWarming(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.warmup.Begin()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.drum.transportZone.playBtn.Disabled {
		t.Fatal("Play button not disabled while warming")
	}
	be.completeAll()
	g.warmup.Tick(1)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.drum.transportZone.playBtn.Disabled {
		t.Fatal("Play button still disabled after the gate opened")
	}
	stopPlaybackForTest(t, g)
}

// TestSetPlayingBypassesTheGate pins that the gate lives at the USER entry
// point only. Tests, scenes, parity harnesses and the bench drive SetPlaying
// directly and must keep working.
func TestSetPlayingBypassesTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	g.warmup = newWarmupController(g, newFakeWarmBackend())
	g.warmup.Begin()
	g.SetPlaying(true)
	if !g.Playing() {
		t.Fatal("SetPlaying(true) was blocked by the gate; it must not be")
	}
	stopPlaybackForTest(t, g)
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPlayBlocked|TestPlayButtonDisabled|TestSetPlayingBypasses' ./internal/ui/`
Expected: FAIL — `g.playbackReady undefined`.

- [ ] **Step 3: Implement the gate**

In `game_update.go`, add the predicate near the other transport helpers:

```go
// playbackReady reports whether the user may start playback. False only while
// the readiness gate is warming the voice cache after a wholesale invalidation
// (import / BPM / sample-rate change). The gate is enforced at the single
// PlayPressed() chokepoint below — NOT inside SetPlaying, which tests, scenes,
// parity harnesses and the bench drive directly.
func (g *Game) playbackReady() bool { return g.warmup == nil || g.warmup.Ready() }
```

Change the consumption site at `:826`:

```go
	if g.drum.PlayPressed() {
		if !g.playbackReady() {
			// Consume the press and ignore it: the progress toast already
			// explains why, and the button renders disabled. Never queue the
			// press — a play that fires seconds later, unprompted, is worse
			// than one that did not fire.
			done, total := g.warmup.Progress()
			g.logger.Debugf("[warmup] play ignored while warming (%d/%d)", done, total)
		} else if g.Playing() {
			// …existing pause branch, unchanged…
```

Keep the rest of the existing `if g.Playing() { … } else { … }` body intact under the new `else if` / `else`.

- [ ] **Step 4: Disable the Play button**

In `transport_zone.go`, inside the per-frame style refresh (the block around `:564-578` that already sets `playBtn.Icon` / `Style` / `IconColor`), add a callback-driven disable. Add to the zone's callbacks struct:

```go
	// PlaybackReady reports whether the readiness gate allows playback. nil ⇒
	// always ready (tests / harnesses that build a bare zone).
	PlaybackReady func() bool
```

and in the refresh:

```go
	z.playBtn.Disabled = z.callbacks.PlaybackReady != nil && !z.callbacks.PlaybackReady()
```

Wire it where the other transport callbacks are constructed in `DrumView` (search for `OnPlayToggle:` to find the literal):

```go
		PlaybackReady: func() bool { return dv.game == nil || dv.game.playbackReady() },
```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPlayBlocked|TestPlayButtonDisabled|TestSetPlayingBypasses' ./internal/ui/`
Expected: PASS (3 tests).

- [ ] **Step 6: Run the transport + cluster guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestTransport|TestVolumeButtonsAndInputsShareKeycapBody' ./internal/ui/
```
Expected: PASS — `TestTransportClusterSharesBounds` in particular (a disabled button must not change its bounds). Leave uncommitted.

---

## Task 11: Notification + hooks wiring

**Files:**
- Modify: `src/go/internal/hooks/events.go` (2 kinds, `KindAll`, `NumNonVerbose` 76 → 78, payload)
- Modify: `src/go/internal/hooks/actions.go` (2 registry rows, tag `warmup`)
- Modify: `src/go/internal/eventlogger/format.go` (2 formatters)
- Modify: `src/go/internal/ui/warmup_controller.go` (wire `onBegin`/`onUpdate`/`onEnd`)
- Modify: `CLAUDE.md` (tag taxonomy row)
- Create: `src/go/internal/ui/warmup_notification_test.go`

**Interfaces:**
- Consumes: `(*DrumView).notifyInfoKey(key i18n.Key, args ...string)`, `(*DrumView).notifyErrorKey`, `(*logsToast).beginTask/updateTask/endTask` (Task 8), `hooks.Publish`.
- Produces:
  ```go
  hooks.EventWarmupStarted  Kind = "warmup.started"
  hooks.EventWarmupComplete Kind = "warmup.complete"
  type hooks.WarmupPayload struct{ Total, Rendered, Cached int; ElapsedMS float64; TimedOut bool }
  ```

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/warmup_notification_test.go`:

```go
//go:build test

package ui

import (
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestWarmupPushesExactlyOneNotification pins the anti-spam rule: START pushes
// nothing into the notification ring (the toast carries it), COMPLETION pushes
// exactly one durable line for the Logs panel.
func TestWarmupPushesExactlyOneNotification(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()

	before := g.drum.notifStore.Len()
	g.warmup.Begin()
	if got := g.drum.notifStore.Len(); got != before {
		t.Errorf("warm-up start pushed %d notifications, want 0", got-before)
	}
	if !g.drum.logsToast.taskActive() {
		t.Fatal("progress toast not showing during warm-up")
	}
	be.completeAll()
	g.warmup.Tick(1)
	if got := g.drum.notifStore.Len(); got != before+1 {
		t.Errorf("warm-up completion pushed %d notifications, want 1", got-before)
	}
	if g.drum.logsToast.taskActive() {
		t.Fatal("progress toast still showing after completion")
	}
	latest := g.drum.notifStore.Latest()
	if latest == nil || latest.isErr {
		t.Fatal("completion notification missing or marked as an error")
	}
}

// TestWarmupTimeoutPushesAnError pins that a gate that opened on the deadline
// tells the user, rather than silently degrading.
func TestWarmupTimeoutPushesAnError(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	g.warmup = newWarmupController(g, newFakeWarmBackend()) // never completes
	g.wireWarmupNotifications()
	g.warmup.Begin()
	g.warmup.Tick(warmupDeadlineMS + 1)

	latest := g.drum.notifStore.Latest()
	if latest == nil || !latest.isErr {
		t.Fatal("timed-out warm-up did not push an error notification")
	}
	if !strings.Contains(strings.ToLower(latest.display()), "circuit") {
		t.Errorf("timeout notification %q does not mention the circuit", latest.display())
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestWarmup ./internal/ui/`
Expected: FAIL — `g.wireWarmupNotifications undefined`.

- [ ] **Step 3: Add the hooks kinds**

In `src/go/internal/hooks/events.go`, next to the other session kinds:

```go
	// EventWarmupStarted / EventWarmupComplete bracket the playback readiness
	// gate: the engine is rendering the loaded circuit's working set before the
	// user may press Play. Complete carries whether the gate opened on its
	// deadline rather than on completion.
	EventWarmupStarted  Kind = "warmup.started"
	EventWarmupComplete Kind = "warmup.complete"
```

Add both to `KindAll` **in the non-verbose section** (before the verbose tail that starts at `EventCameraPan`), and bump:

```go
// numNonVerbose … Phase 2026-09-13 added 2 (warmup started/complete).
const NumNonVerbose = 78
```

Add the payload next to `LanguagePayload`:

```go
// WarmupPayload describes one playback-readiness warm-up pass.
type WarmupPayload struct {
	Total     int     `json:"total"`     // items that needed rendering
	Rendered  int     `json:"rendered"`  // items completed before the gate opened
	Cached    int     `json:"cached"`    // items skipped because they were already warm
	ElapsedMS float64 `json:"elapsed_ms"`
	TimedOut  bool    `json:"timed_out"`
}
```

In `src/go/internal/hooks/actions.go`, next to the other session rows:

```go
	{EventWarmupStarted, "warm up circuit", "warmup", ScopeSession, ""},
	{EventWarmupComplete, "circuit ready", "warmup", ScopeSession, ""},
```

In `src/go/internal/eventlogger/format.go`:

```go
	hooks.EventWarmupStarted: func(p any) string {
		w, _ := p.(hooks.WarmupPayload)
		return fmt.Sprintf("warming %d sounds (%d already cached)", w.Total, w.Cached)
	},
	hooks.EventWarmupComplete: func(p any) string {
		w, _ := p.(hooks.WarmupPayload)
		if w.TimedOut {
			return fmt.Sprintf("circuit ready on deadline — %d/%d sounds warm after %.0fms",
				w.Rendered, w.Total, w.ElapsedMS)
		}
		return fmt.Sprintf("circuit ready — %d sounds in %.0fms (%d cached)",
			w.Rendered, w.ElapsedMS, w.Cached)
	},
```

In `CLAUDE.md`'s tag taxonomy table, add:

```
| `warmup` | playback readiness gate: circuit warm-up started / complete |
```

- [ ] **Step 4: Wire the controller callbacks**

Add to `warmup_controller.go`:

```go
// wireWarmupNotifications connects the controller to the progress toast, the
// notification ring, and the hooks bus.
//
// Deliberately asymmetric: START shows the toast and publishes a hooks event
// but pushes NOTHING into the notification ring (it would spam the Logs panel
// on every import). COMPLETION pushes exactly one durable, translatable line.
func (g *Game) wireWarmupNotifications() {
	if g == nil || g.warmup == nil || g.drum == nil {
		return
	}
	w := g.warmup
	w.onBegin = func(total int) {
		g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, total)
		_, skipped, _ := w.BenchStats()
		hooks.Publish(hooks.EventWarmupStarted, hooks.WarmupPayload{Total: total, Cached: skipped})
	}
	w.onUpdate = func(done, total int) {
		g.drum.logsToast.updateTask(done)
	}
	w.onEnd = func(elapsedMS float64, rendered, skipped int, timedOut bool) {
		g.drum.logsToast.endTask()
		total := rendered
		if d, tt := w.Progress(); tt > 0 {
			total, rendered = tt, d
		}
		if timedOut {
			g.drum.notifyErrorKey(i18n.KeyNotifCircuitWarmupTimedOut,
				strconv.Itoa(rendered), strconv.Itoa(total))
		} else if total > 0 {
			g.drum.notifyInfoKey(i18n.KeyNotifCircuitReady,
				strconv.Itoa(rendered), strconv.FormatFloat(elapsedMS, 'f', 0, 64),
				strconv.Itoa(skipped))
		}
		hooks.Publish(hooks.EventWarmupComplete, hooks.WarmupPayload{
			Total: total, Rendered: rendered, Cached: skipped,
			ElapsedMS: elapsedMS, TimedOut: timedOut,
		})
	}
}
```

Add the imports (`strconv`, `internal/hooks`, `internal/i18n`). Use the project's existing publish helper if `hooks.Publish` is not the right entry point — check `internal/ui/event_helpers.go` and prefer an `emitWarmup*` helper there, matching `emitNodeAdded`'s shape.

- [ ] **Step 5: Run the tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run TestWarmup ./internal/ui/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/
```
Expected: PASS, including `TestEveryKindClassified`, `TestEveryActionHasLowercaseTag`, `TestHooks_NumNonVerboseUpdated`, and eventlogger's `coverage_test.go`.

- [ ] **Step 6: Run the i18n + log-tag guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/ ./internal/log/
```
Expected: PASS — `TestNoUppercaseInfoTags` and the catalog completeness tests (every key present in both `en` and `es`). Leave uncommitted.

---

## Task 12: Trip the gate on import, BPM and sample-rate changes

**Files:**
- Modify: `src/go/internal/ui/import.go` (tail, after `updateBeatInfos`)
- Modify: `src/go/internal/ui/game_new.go` (construct the controller)
- Modify: wherever `SetBPM` commits (find with `grep -n "func (dv \*DrumView) SetBPM" src/go/internal/ui/*.go`)
- Create: `src/go/internal/ui/warmup_triggers_test.go`

**Interfaces:**
- Consumes: `newWarmupController`, `newDesktopWarmBackend` / `newBrowserWarmBackend`, `(*Game).wireWarmupNotifications()`.
- Produces: `func (g *Game) beginWarmup(reason string)`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/warmup_triggers_test.go`:

```go
//go:build test

package ui

import (
	"os"
	"path/filepath"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestImportTripsTheGate pins the primary trigger: loading a circuit must leave
// playback gated until the working set is warm.
func TestImportTripsTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()

	data, err := os.ReadFile(filepath.Join("..", "assets", "templates", "beethoven-ode-to-joy.json"))
	if err != nil {
		t.Skipf("template unavailable: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if g.playbackReady() {
		t.Fatal("import left playback ungated — the voice cache is cold")
	}
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("gate did not open after the warm set completed")
	}
}

// TestBPMChangeTripsTheGate pins the second wholesale invalidator: the voice
// cache key carries bpm, so a BPM change re-keys every variant instrument.
func TestBPMChangeTripsTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()
	be.completeAll()
	g.warmup.Tick(1)

	g.drum.SetBPM(g.bpm + 40)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.playbackReady() {
		t.Fatal("BPM change left playback ungated despite a re-keyed voice cache")
	}
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestImportTrips|TestBPMChangeTrips' ./internal/ui/`
Expected: FAIL — playback ready right after import.

- [ ] **Step 3: Construct the controller at startup**

In `game_new.go`, right after `g.voicePrewarmer = newVoicePrewarmer(g)`:

```go
	g.warmup = newWarmupController(g, newWarmBackend(g))
```

Add the platform selector — `warmup_backend_desktop.go`:

```go
// newWarmBackend selects the platform warm backend. Desktop renders on the
// audio.prewarm pool; browser renders on audio.js's render workers.
func newWarmBackend(g *Game) warmBackend {
	if g != nil && g.voicePrewarmer != nil {
		return newDesktopWarmBackend(g.voicePrewarmer.pool)
	}
	return newDesktopWarmBackend(nil)
}
```

and `warmup_backend_js.go`:

```go
func newWarmBackend(g *Game) warmBackend { return newBrowserWarmBackend() }
```

Call `g.wireWarmupNotifications()` after `g.drum` exists (in `New`, after the DrumView is constructed).

- [ ] **Step 4: Add the trigger helper and call it**

In `warmup_controller.go`:

```go
// beginWarmup trips the readiness gate after a wholesale cache invalidation.
// reason is for the debug log only. Single instrument switches and one-off
// param edits deliberately do NOT come through here — they keep the existing
// non-blocking background warm plus the serveBestAvailable fallback.
func (g *Game) beginWarmup(reason string) {
	if g == nil || g.warmup == nil {
		return
	}
	g.logger.Debugf("[warmup] begin (%s)", reason)
	g.warmup.Begin()
}
```

In `import.go`, immediately after the existing `g.updateBeatInfos()` + `g.paramsDirty = false` block near the tail:

```go
	// Trip the playback readiness gate: import is replace-not-merge, so the
	// voice cache is cold for every instrument/pitch this circuit introduces.
	// Runs AFTER updateBeatInfos so beatInfosByRow is populated for the
	// full-loop enumeration.
	g.beginWarmup("import")
```

In `DrumView.SetBPM` (or its Game-side commit — find the real chokepoint first), after the BPM is applied:

```go
	// The voice cache key carries bpm, so a tempo change re-keys every
	// variant instrument. Same wholesale invalidation as an import.
	if dv.game != nil {
		dv.game.beginWarmup("bpm")
	}
```

**Guard against a warm-up storm from the BPM drag**: if `SetBPM` is called per-frame while dragging, debounce — only call `beginWarmup` when the BPM value actually changed AND no drag is in progress. Check how `EventBPMChanged` is emitted and mirror that commit point exactly rather than the raw setter.

**Sample-rate trigger (browser-critical).** The voice cache key carries
`sampleRate`, and on browser the rate is not known until the `AudioContext`
exists. A circuit imported *before* the audio-unlock gesture warms at the
assumed rate; when the context then comes up at 48 kHz, every key rotates and
the whole warm set is cold again — silently, with Play already enabled. Trip the
gate when the observed rate changes:

```go
// warmupSampleRate is the rate the last warm set was rendered at. The voice
// cache key carries sampleRate, so a rate change re-keys EVERYTHING. On
// browser the rate is unknown until the AudioContext exists, so a pre-unlock
// import warms at the assumed rate and goes cold the moment the real context
// appears. Checked every frame; the compare is a single atomic load.
func (g *Game) checkWarmupSampleRate() {
	sr := audio.SampleRate()
	if sr <= 0 || sr == g.warmupSampleRate {
		return
	}
	if g.warmupSampleRate != 0 {
		g.beginWarmup("sample-rate")
	}
	g.warmupSampleRate = sr
}
```

Add the `warmupSampleRate int` field to `game_struct.go` beside `warmup`, and
call `g.checkWarmupSampleRate()` from `game_update.go` immediately before
`g.warmup.Tick(0)`.

Add the regression test to `warmup_triggers_test.go`:

```go
// TestSampleRateChangeTripsTheGate pins the browser-critical case: a circuit
// imported before the AudioContext exists warms at the assumed rate, and every
// cache key rotates when the real rate arrives.
func TestSampleRateChangeTripsTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()
	g.warmupSampleRate = 44100
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("gate shut before the rate change")
	}

	restore := audio.SetSampleRateForTest(48000)
	t.Cleanup(restore)
	g.checkWarmupSampleRate()
	if g.playbackReady() {
		t.Fatal("sample-rate change left playback ungated despite a re-keyed voice cache")
	}
}
```

If `audio.SetSampleRateForTest(int) func()` does not exist, add it next to
`audio.SampleRate()` in the same file, guarded to the test/stub builds — do not
mutate the production global from a test without a restore closure.

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestImportTrips|TestBPMChangeTrips|TestWarmup|TestPlayBlocked' ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Flush deferred browser channel ops before the gate opens**

In `import.go`, next to the existing `reapplyAnalyzerTaps()` call, force the AudioContext so `pendingChannelOps` (channel + insert-FX subgraph construction deferred when no context existed) drains before playback rather than bursting at the first note:

```go
	// Browser: channel + insert-FX node construction is deferred into
	// pendingChannelOps while no AudioContext exists, so a pre-unlock import
	// would build the whole graph in a burst at the first note. Force the
	// context now, inside the gate, where a stall is invisible.
	audio.EnsureAudioGraphReady()
```

Implement `EnsureAudioGraphReady()` as a no-op on desktop (`//go:build !js`, processors are already eager in `SetInsertEffects`) and, on js, as a call to a new `window.flushPendingChannelOps()` in `audio.js` that calls `getCtx()` and drains the queue. Add both to the bridge catalogue in `wasm_bridge_smoke.browser.test.js`.

- [ ] **Step 7: Run the full UI suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. Leave uncommitted.

---

## Task 13: Cache-hit identity tests

The spec's guarantees are only real if they are pinned. These need the **real** render path, so they run without `-tags test`.

**Files:**
- Create: `src/go/internal/audio/warmup_cache_identity_test.go` (no build tags → real build only)

**Interfaces:**
- Consumes: `GetRenderLatencyMetrics()`, `ResetRenderLatencyMetrics()`, `PrewarmVoice`, `VoiceCached`, `SetInsertEffects`, `SetChannelPan`, `EffectSlot`.
- Produces: nothing (tests only).

- [ ] **Step 1: Write the tests**

Create `src/go/internal/audio/warmup_cache_identity_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// TestSecondWarmOfIdenticalSetRendersNothing pins the headline cache-hit
// guarantee: re-loading a document whose instruments, params and pitches are
// unchanged must render ZERO voices.
func TestSecondWarmOfIdenticalSetRendersNothing(t *testing.T) {
	pitches := []float64{-12, -5, 0, 4, 7, 12}
	for _, p := range pitches {
		PrewarmVoice("cello", p)
	}
	ResetRenderLatencyMetrics()
	for _, p := range pitches {
		if !VoiceCached("cello", p) {
			t.Fatalf("cello@%v not cached after the first warm", p)
		}
		PrewarmVoice("cello", p)
	}
	if m := GetRenderLatencyMetrics(); m.CacheMisses != 0 {
		t.Errorf("second warm of an identical set rendered %d voices, want 0", m.CacheMisses)
	}
}

// TestEffectsChangeDoesNotInvalidateVoices pins that insert effects are
// post-voice processing and are correctly ABSENT from voiceCacheKey. Changing
// only the effect chain must not re-render a single voice — if this ever fails,
// someone "fixed" the key by adding effects to it and made every FX tweak cost
// a full circuit re-render.
func TestEffectsChangeDoesNotInvalidateVoices(t *testing.T) {
	const id = "violin"
	PrewarmVoice(id, 0)
	PrewarmVoice(id, 7)
	ResetRenderLatencyMetrics()

	SetInsertEffects(id, []EffectSlot{{Type: EffectTypeDelay, Enabled: true,
		Params: map[string]float64{"mix": 0.4}}})
	SetChannelPan(id, -0.5)

	if !VoiceCached(id, 0) || !VoiceCached(id, 7) {
		t.Fatal("an effects/pan change invalidated the voice cache")
	}
	PrewarmVoice(id, 0)
	PrewarmVoice(id, 7)
	if m := GetRenderLatencyMetrics(); m.CacheMisses != 0 {
		t.Errorf("effects change caused %d voice re-renders, want 0", m.CacheMisses)
	}
}
```

Replace `EffectTypeDelay` with the real constant — confirm with `grep -n "EffectType.* EffectType = " src/go/internal/audio/effect_registry_entries.go`.

- [ ] **Step 2: Run them against the real renderer**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run 'TestSecondWarmOfIdenticalSet|TestEffectsChangeDoesNotInvalidate' -v
```
Expected: PASS. If `TestEffectsChangeDoesNotInvalidateVoices` fails, the voice cache key has picked up an effects dependency — that is a real regression in the design's core assumption; stop and report rather than adapting the test.

- [ ] **Step 3: Confirm the `-tags test` path skips them cleanly**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/`
Expected: PASS (these files are excluded by their build tag — that is why they also need the real-build run above). Leave uncommitted.

---

## Task 14: Screenshot scenes + visual verification

**MANDATORY.** Green Go tests do not prove that text, layout, colors or the progress bar actually render.

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go`
- Modify: `src/go/internal/ui/screenshot_subjects.go` (add the crop subject)

**Interfaces:**
- Consumes: `SceneSpec{Name, Description, SettleFrames, Setup, MobileSetup, Subject}`, `(*Game).SubjectRect`.
- Produces: scenes `warmup_progress`, `crop_warmup_progress`; `Subject` const `SubjectWarmupToast`.

- [ ] **Step 1: Add the scenes**

In `scene_catalog.go`, next to the `logs_panel` entries:

```go
	{Name: "warmup_progress", Description: "circuit warm-up in progress — sticky progress toast with determinate bar, Play button disabled",
		SettleFrames: 8,
		Setup: func(g *Game) {
			g.warmup = newWarmupController(g, newBlockedWarmBackendForScene())
			g.wireWarmupNotifications()
			g.warmup.Begin()
			g.warmup.onUpdate = nil // freeze the bar mid-progress for a stable capture
			g.drum.logsToast.updateTask(42)
		}},
	{Name: "crop_warmup_progress", Description: "warm-up progress toast, cropped", Subject: SubjectWarmupToast,
		SettleFrames: 8,
		Setup: func(g *Game) {
			g.warmup = newWarmupController(g, newBlockedWarmBackendForScene())
			g.wireWarmupNotifications()
			g.warmup.Begin()
			g.warmup.onUpdate = nil
			g.drum.logsToast.updateTask(42)
		}},
```

Add the scene-only backend next to the scene helpers (`scopeZoneOf`'s neighbourhood):

```go
// newBlockedWarmBackendForScene reports nothing cached and never completes, so
// a screenshot scene can hold the progress toast open at a stable frame.
type blockedWarmBackend struct{}

func (blockedWarmBackend) Cached(string, float64) bool     { return false }
func (blockedWarmBackend) Warm(string, float64, func())    {}
func newBlockedWarmBackendForScene() warmBackend           { return blockedWarmBackend{} }
```

`onUpdate` is an unexported field of a same-package type, so the scene may set
it directly — no accessor needed. Nulling it freezes the bar at the value the
scene sets, which is what makes the capture byte-stable across runs.

- [ ] **Step 2: Add the crop subject**

In `screenshot_subjects.go`:

```go
	// SubjectWarmupToast crops to the warm-up progress toast.
	SubjectWarmupToast Subject = "warmup_toast"
```

and in `(*Game).SubjectRect`:

```go
	case SubjectWarmupToast:
		if g.drum == nil {
			return image.Rectangle{}
		}
		return g.drum.logsToast.rectIn(g.toastHostRect())
```

- [ ] **Step 3: Run the automated crop check**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSceneCrop|TestSceneCatalog' ./internal/ui/`
Expected: PASS — every `crop_*` scene must yield a visible subject rect strictly smaller than the framebuffer.

- [ ] **Step 4: Capture the desktop screenshot and LOOK AT IT**

```bash
cd /home/ymolinar/Repos/beatmo
CGO_ENABLED=1 .tools/go/bin/go build -C src/go -o /tmp/beatmo_screenshot ./cmd
SCRATCH=$(mktemp -d)
XDG_CONFIG_HOME=$SCRATCH xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot \
  -log ERROR -scene warmup_progress -screenshot /tmp/warmup_desktop.png
XDG_CONFIG_HOME=$SCRATCH xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot \
  -log ERROR -scene crop_warmup_progress -screenshot /tmp/warmup_crop.png
```

**Open both PNGs and confirm:** the label text is present and legible (not blank), the counts read `42/…`, the progress bar track and fill are both visible with the fill proportional to 42/total, the toast does not overlap the top-right button cluster, the Play button renders visibly disabled, and colors follow the theme. **Fix and re-capture until it looks right.**

- [ ] **Step 5: Capture the mobile screenshot in a NON-GRID view mode and LOOK AT IT**

The mobile fallback is the whole point of Task 9 — a capture in Pads mode proves nothing.

```bash
XDG_CONFIG_HOME=$SCRATCH xvfb-run -a -s "-screen 0 390x844x24" /tmp/beatmo_screenshot \
  -log ERROR -mobile -win-w 390 -win-h 844 -scene warmup_progress -screenshot /tmp/warmup_mobile.png
```

If the scene's default view mode is Pads, add a `MobileSetup` to the catalog entry that switches to the EQ view first (mirroring `mobileAudioPanelPlaySetup`), then re-capture. **Open the PNG** and confirm the toast is visible above the bottom nav, is not clipped, and does not overlap the nav or the audio panel.

- [ ] **Step 6: Run the pixel-content guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestSceneCrop|TestNotificationBandPixels|TestThemeApplyRepaintsEverySurface' ./internal/ui/
```
Expected: PASS. Leave uncommitted.

---

## Task 15: Re-measure against the acceptance table

**Files:**
- Create: `bench-results/import_warmup_2026-09-13.md`
- Modify: `CLAUDE.md` (Quick Start row for the new bench target)

**Interfaces:**
- Consumes: `make bench-import-cold`, the desktop bench, `bench-results/import_cold.json`.
- Produces: a written before/after record.

- [ ] **Step 1: Re-measure desktop**

```bash
cd /home/ymolinar/Repos/beatmo
CGO_ENABLED=1 .tools/go/bin/go build -C src/go -o /tmp/beatmo_bench ./cmd
BEATMO_DEMO_CONFIG=$(pwd)/src/go/internal/assets/templates/beach-gaelic.json \
  xvfb-run -a /tmp/beatmo_bench -log INFO -bench-bpm 180 -bench-secs 15 2>&1 \
  | grep -E '\[bench\] (Complete|RenderLatency)'
```

Acceptance: `prewarmRenders` during the playback window **0** (baseline 175); `dispatchRenders` **0** (must stay 0); `stalePlays` **0**; `warmMS` ≤ ~600.

- [ ] **Step 2: Re-measure browser**

```bash
cd /home/ymolinar/Repos/beatmo && make wasm
GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_import_cold_bench.browser.test.js
```

Acceptance: `neighborPlays` **0** (baseline 66); `deferredPlays` **0** (baseline 4 @ 288 ms); `renders` during playback **0** (baseline 75+, still climbing); `gateMs` ≤ ~2000 (baseline 7790 eager, unbounded on-demand).

- [ ] **Step 3: Verify the re-import cache hit end to end**

```bash
TPL=beach-gaelic.json GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_import_cold_bench.browser.test.js
# then immediately re-run — the second run's gateMs must be near-zero if the
# page is reused; for a fresh page, assert the in-session second import instead.
```
Add an in-page assertion to the bench: import the same document twice in one session and require the second `warmupProgress().total` to be **0**. This is the "same instruments, same params, same notes ⇒ cache hit" requirement from the original request, measured rather than assumed.

- [ ] **Step 4: Write the record**

Create `bench-results/import_warmup_2026-09-13.md` with a before/after table for both platforms, the exact commands used, the machine's core count, and the main-thread-stall finding from Task 7 Step 6.

- [ ] **Step 5: Add the Quick Start row**

In `CLAUDE.md`'s Quick Start table:

```
| Import-cold bench (browser) | `make bench-import-cold` (load circuit → play immediately; → `bench-results/import_cold.json`) |
```

- [ ] **Step 6: Full-suite regression run**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/
cd /home/ymolinar/Repos/beatmo && make test-parity
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/e2e_transport.browser.test.js
```
Expected: all PASS. Investigate any failure — per the project's rule, never assume a failure is pre-existing; prove causation by checking the same test before your change. Leave everything uncommitted for the user to review.
