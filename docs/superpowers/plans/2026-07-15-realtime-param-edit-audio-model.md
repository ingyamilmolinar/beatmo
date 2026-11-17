# Real-time Param-Edit Audio Model (desktop) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a synth-knob or sampler edit during playback never crack the audio, by guaranteeing the audio dispatch goroutine never renders — and prove it with a measurement gate.

**Architecture:** Formalize the **Voice Binding** (`voiceCacheKey`) as the contract: a node's audio is fully determined at trigger time and is immutable afterward. Phase 1 adds render-origin instrumentation (`DispatchRenders` vs `PrewarmRenders`) plus an untagged param-edit-during-playback benchmark that **fails on the current tree**. Phase 2 makes it pass: non-blocking dispatch with a best-available fallback, bind-ahead prewarm on edit, eviction pinning, and removal of the redundant sampler blanket-invalidate.

**Tech Stack:** Go 1.26 (bundled at `.tools/go/bin/go`), CGo DSP (`render_modular_p`), oto audio backend, Ebiten UI.

**Spec:** `docs/superpowers/specs/2026-07-15-realtime-param-edit-audio-model-design.md`

## Global Constraints

- **NEVER `git add` / `git commit` / `git stash`.** The user's standing rule overrides the writing-plans default of committing per task. Leave all work uncommitted in the working tree. Every "Checkpoint" step below is a verification step, not a commit.
- **Always use the bundled Go:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`. Never the system `go`. Use `.tools/go/bin/gofmt` for formatting.
- **Verify by EXPLICIT package path, never `./...`.** This branch has a phantom duplicate package that fails to compile under `./...`. Use `./internal/audio/` and `./internal/ui/`.
- **`internal/ui` needs `-timeout 900s`.** The suite runs ~520–625s vs go test's 600s default. A 601s "FAIL" there is the watchdog, not a regression.
- **Two build worlds, and this matters constantly:**
  - `-tags test -modfile=go.test.mod` → **synth is STUBBED, no real render ever happens.** Fast path.
  - untagged (no `-tags`) → **real CGo DSP.** Any test measuring render cost MUST be untagged (`//go:build !test && !js`).
- **Gate discipline:** primary gates assert on **counts** (deterministic). Wall-clock is **reported, never asserted** — a timing assertion flakes under parallel suite load.
- **Never call `g.SetPlayFunc(...)` in these benchmarks.** It stubs out the audio trigger, so no render happens and the benchmark silently measures nothing.
- **`TestParamEditPlaybackCostTable` is INTENTIONALLY RED from Task 3 until Task 8.** That is the whole design: Phase 1 ships a failing gate (`DispatchRenders > 0`) that proves the defect, and Phase 2 makes it pass. **Do NOT "fix" it in Tasks 4-7, and do NOT report BLOCKED because of it.** Intermediate checkpoints in Tasks 4-7 therefore exclude it with `-run`. Only Task 8 is allowed to turn it green.
- **This branch carries a large amount of unrelated pre-existing work** (modified + staged files from other efforts). Only ever diff/verify the files YOUR task touches. Never `git checkout`/`git restore`/`git stash` anything, and never "clean up" a file your task does not list.
- **Cache-key rule:** a prewarm's key MUST be byte-identical to the key dispatch reads. Key computation lives in `tryRecipeVoicePitched` / `pitchedVoiceKey`; never fork it.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `src/go/internal/audio/render_latency.go` | Render counters + the new `renderOrigin` type and `DispatchRenders`/`PrewarmRenders` fields. Untagged (UI reads the type). | Modify |
| `src/go/internal/audio/synth_recipe_dispatch.go` | Voice dispatch + key computation. Gains origin threading; later, non-blocking mode + best-available fallback. `!test && !js`. | Modify |
| `src/go/internal/audio/voice_prewarm_desktop.go` | `PrewarmVoice`; gains `PrewarmVoiceParams` and reports `originPrewarm`. `!test && !js`. | Modify |
| `src/go/internal/audio/voice_prewarm_stub.go` | `-tags test` stubs; gains a `PrewarmVoiceParams` stub. | Modify |
| `src/go/internal/audio/voice_cache.go` | Cache + LRU eviction; gains the pin-set. `!test && !js`. | Modify |
| `src/go/internal/audio/sample_edit_descriptor.go` | Sampler descriptors; loses the redundant blanket-invalidate. Untagged. | Modify |
| `src/go/internal/audio/render_origin_test.go` | Origin-attribution unit test. `!test && !js`. | Create |
| `src/go/internal/audio/voice_binding_invariant_test.go` | I1/I2/I4 pin tests. `!test && !js`. | Create |
| `src/go/internal/audio/sample_edit_storm_test.go` | Sampler miss-ratio regression. `!test && !js`. | Create |
| `src/go/internal/ui/param_edit_playback_bench_test.go` | The benchmark: param edits during real playback. `!test && !js`. | Create |
| `src/go/internal/ui/game_update.go` | `[bench_json]` line gains render-latency fields. | Modify |

---

## PHASE 1 — MEASUREMENT

### Task 1: Render-origin instrumentation

**Why:** `recordVoiceRender` cannot currently tell whether a render ran on the audioLoop dispatch goroutine (the bug) or off-thread on the prewarm pool (healthy). Both call the same `newRecipeAwareVoice*` functions. Without this split there is no machine-independent gate. We thread the origin **explicitly** — do NOT sniff goroutine IDs, and do NOT use a package-level flag (prewarm runs on 2 concurrent pool workers, so a global would misattribute).

**Files:**
- Modify: `src/go/internal/audio/render_latency.go`
- Modify: `src/go/internal/audio/synth_recipe_dispatch.go:100-300`
- Modify: `src/go/internal/audio/voice_prewarm_desktop.go`
- Test: `src/go/internal/audio/render_origin_test.go` (create)

**Interfaces:**
- Produces: `renderOrigin` (unexported enum: `originDispatch`, `originPrewarm`); `recordVoiceRender(d time.Duration, origin renderOrigin)`; `RenderLatencyMetrics.DispatchRenders int64`, `.PrewarmRenders int64`; unexported `newRecipeAwareVoiceOrigin(id string, bpm, sampleRate int, origin renderOrigin) Voice`, `newRecipeAwareVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) Voice`, `tryRecipeVoiceOptsOrigin(id string, bpm, sampleRate int, forceRecipe bool, origin renderOrigin) (Voice, bool)`, `tryRecipeVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) (Voice, bool)`.
- Consumes: nothing (first task).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/audio/render_origin_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// A render on the prewarm path must NOT be attributed to the dispatch
// goroutine, and vice versa. DispatchRenders is the gate for invariant I3
// ("audioLoop never calls recipe.Render"), so misattribution here would make
// the whole benchmark lie.
func TestRenderOriginAttribution(t *testing.T) {
	const id = "organ-church"
	const pitch = 3.0

	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Prewarm path → attributed to PrewarmRenders.
	PrewarmVoice(id, pitch)
	m := GetRenderLatencyMetrics()
	if m.PrewarmRenders != 1 {
		t.Fatalf("PrewarmVoice: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}
	if m.DispatchRenders != 0 {
		t.Fatalf("PrewarmVoice must not count as a dispatch render: DispatchRenders=%d want 0", m.DispatchRenders)
	}

	// Dispatch path on an ALREADY-WARM voice → a hit, no new render.
	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Dispatch path on a COLD voice → attributed to DispatchRenders.
	newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch)
	m = GetRenderLatencyMetrics()
	if m.DispatchRenders != 1 {
		t.Fatalf("dispatch cold render: DispatchRenders=%d want 1", m.DispatchRenders)
	}
	if m.PrewarmRenders != 0 {
		t.Fatalf("dispatch render must not count as prewarm: PrewarmRenders=%d want 0", m.PrewarmRenders)
	}

	// Totals stay consistent with the pre-existing miss counter.
	if got := m.DispatchRenders + m.PrewarmRenders; got != m.CacheMisses {
		t.Fatalf("origin split must partition CacheMisses: dispatch+prewarm=%d CacheMisses=%d", got, m.CacheMisses)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run TestRenderOriginAttribution -v ./internal/audio/
```
Expected: FAIL to COMPILE with `m.PrewarmRenders undefined (type RenderLatencyMetrics has no field or method PrewarmRenders)`.

- [ ] **Step 3: Add the origin type and counters**

In `src/go/internal/audio/render_latency.go`, add after the `RenderLatencyMetrics` struct:

```go
// renderOrigin identifies WHICH goroutine paid for a voice render.
//
// This is the whole point of the split: recipe.Render is a ~24ms CGo call
// (organ-church), and PlayBatch (batch_desktop.go) is a plain serial loop on
// the audioLoop dispatch goroutine. A render there delays every subsequent
// beat in the batch and every beat still queued in audioCh, which is what
// makes a knob drag audible as a crack. A render on the prewarm pool costs
// the same CPU but blocks nothing.
//
// Threaded EXPLICITLY through the dispatch helpers. Do not replace this with a
// package-level flag: the prewarm pool runs 2 workers concurrently with
// dispatch, so a global would misattribute.
type renderOrigin int

const (
	// originDispatch: rendered on the audioLoop dispatch goroutine. Any
	// non-zero DispatchRenders during playback is invariant I3 violated.
	originDispatch renderOrigin = iota
	// originPrewarm: rendered off-thread (async.Pool "audio.prewarm").
	originPrewarm
)
```

Replace the `RenderLatencyMetrics` struct with:

```go
// RenderLatencyMetrics is a snapshot of the voice-render counters.
type RenderLatencyMetrics struct {
	// CacheHits: triggers served from globalVoiceCache with no render.
	CacheHits int64
	// CacheMisses: triggers that had to render a voice (the cost a param edit
	// pays — one per distinct pitch/param-hash after an invalidation).
	// Always == DispatchRenders + PrewarmRenders.
	CacheMisses int64
	// RenderAvgNS / RenderMaxNS: cost of one cache-miss voice render (recipe
	// Render + normalize). The Max is the worst single stall on the trigger path.
	RenderAvgNS int64
	RenderMaxNS int64
	// DispatchRenders: renders that ran on the audioLoop dispatch goroutine.
	// MUST be 0 during playback (invariant I3) — each one delays the next beat.
	DispatchRenders int64
	// PrewarmRenders: renders that ran off-thread. Healthy; unbounded is fine.
	PrewarmRenders int64
}
```

Replace `renderLatencyCounters` with:

```go
type renderLatencyCounters struct {
	hits            atomic.Int64
	misses          atomic.Int64
	renderSum       atomic.Int64
	renderMax       atomic.Int64
	dispatchRenders atomic.Int64
	prewarmRenders  atomic.Int64
}
```

Replace `recordVoiceRender` with:

```go
// recordVoiceRender records one cache-miss render's duration (and bumps misses),
// attributed to the goroutine class that paid for it.
// Fast path: three atomic adds + one CAS loop for the max.
func recordVoiceRender(d time.Duration, origin renderOrigin) {
	renderLatency.misses.Add(1)
	switch origin {
	case originPrewarm:
		renderLatency.prewarmRenders.Add(1)
	default:
		renderLatency.dispatchRenders.Add(1)
	}
	ns := int64(d)
	renderLatency.renderSum.Add(ns)
	for {
		prev := renderLatency.renderMax.Load()
		if ns <= prev {
			return
		}
		if renderLatency.renderMax.CompareAndSwap(prev, ns) {
			return
		}
	}
}
```

In `GetRenderLatencyMetrics`, extend the returned struct:

```go
	return RenderLatencyMetrics{
		CacheHits:       renderLatency.hits.Load(),
		CacheMisses:     misses,
		RenderAvgNS:     avg,
		RenderMaxNS:     renderLatency.renderMax.Load(),
		DispatchRenders: renderLatency.dispatchRenders.Load(),
		PrewarmRenders:  renderLatency.prewarmRenders.Load(),
	}
```

In `ResetRenderLatencyMetrics`, add:

```go
	renderLatency.dispatchRenders.Store(0)
	renderLatency.prewarmRenders.Store(0)
```

- [ ] **Step 4: Thread the origin through the dispatch helpers**

In `src/go/internal/audio/synth_recipe_dispatch.go`:

Replace the body of `newRecipeAwareVoice` so the exported-shape wrapper stays and an origin-aware core appears:

```go
func newRecipeAwareVoice(id string, bpm, sampleRate int) Voice {
	return newRecipeAwareVoiceOrigin(id, bpm, sampleRate, originDispatch)
}

// newRecipeAwareVoiceOrigin is newRecipeAwareVoice with explicit render
// attribution. The key computation is IDENTICAL regardless of origin — origin
// affects only the metric, never the cache key.
func newRecipeAwareVoiceOrigin(id string, bpm, sampleRate int, origin renderOrigin) Voice {
	if v, ok := tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, false, origin); ok {
		if debugSynthDispatch {
			log.Printf("[SYNTH-DISPATCH] recipe path id=%q recipe=%q paramsHash=%x", id, RecipeForInstrument(id), hashRecipeParams(GetInstrumentParams(id)))
		}
		return v
	}
	if debugSynthDispatch {
		log.Printf("[SYNTH-DISPATCH] legacy path id=%q (no user params or no recipe binding)", id)
	}
	return legacyNewVoice(id, bpm, sampleRate)
}
```

Replace `newRecipeAwareVoicePitched` similarly:

```go
func newRecipeAwareVoicePitched(id string, bpm, sampleRate int, pitch float64) Voice {
	return newRecipeAwareVoicePitchedOrigin(id, bpm, sampleRate, pitch, originDispatch)
}

// newRecipeAwareVoicePitchedOrigin is newRecipeAwareVoicePitched with explicit
// render attribution.
func newRecipeAwareVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) Voice {
	recipeID := RecipeForInstrument(id)
	if pitchAwareRecipe(recipeID) {
		if v, ok := tryRecipeVoicePitchedOrigin(id, bpm, sampleRate, pitch, origin); ok {
			if debugSynthDispatch {
				log.Printf("[SYNTH-DISPATCH] melodic at-pitch path id=%q recipe=%q pitch=%.2f", id, recipeID, pitch)
			}
			return v
		}
	}
	return newRecipeAwareVoiceOrigin(id, bpm, sampleRate, origin)
}
```

Keep `tryRecipeVoicePitched`'s existing signature as a wrapper (existing tests call it), and rename the current implementation to take an origin:

```go
// tryRecipeVoicePitched renders a melodic recipe at the given node pitch.
// Existing-signature wrapper: dispatch attribution.
func tryRecipeVoicePitched(id string, bpm, sampleRate int, pitch float64) (Voice, bool) {
	return tryRecipeVoicePitchedOrigin(id, bpm, sampleRate, pitch, originDispatch)
}

func tryRecipeVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) (Voice, bool) {
	// ... EXISTING BODY UNCHANGED, except the two recordVoiceRender calls ...
}
```

In `tryRecipeVoicePitchedOrigin`, change both render sites (currently lines ~155 and ~165):

```go
		recordVoiceRender(time.Since(renderStart), origin)
```

Do the same for the non-pitched path. Keep the existing signatures as wrappers:

```go
func tryRecipeVoice(id string, bpm, sampleRate int) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, false, originDispatch)
}

func tryRecipeVoiceOpts(id string, bpm, sampleRate int, forceRecipe bool) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, forceRecipe, originDispatch)
}

func tryRecipeVoiceOptsOrigin(id string, bpm, sampleRate int, forceRecipe bool, origin renderOrigin) (Voice, bool) {
	// ... EXISTING tryRecipeVoiceOpts BODY UNCHANGED, except recordVoiceRender ...
}
```

and its render site (currently line ~283):

```go
	recordVoiceRender(time.Since(renderStart), origin)
```

- [ ] **Step 5: Attribute the prewarm path**

In `src/go/internal/audio/voice_prewarm_desktop.go`, change the two calls inside `PrewarmVoice`:

```go
	if pitchAwareRecipe(RecipeForInstrument(id)) {
		newRecipeAwareVoicePitchedOrigin(id, bpm, SampleRate(), pitch, originPrewarm)
		return
	}
	newRecipeAwareVoiceOrigin(id, bpm, SampleRate(), originPrewarm)
```

- [ ] **Step 6: Run test to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run TestRenderOriginAttribution -v ./internal/audio/
```
Expected: PASS.

- [ ] **Step 7: Verify no regression + formatting**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/render_latency.go internal/audio/synth_recipe_dispatch.go internal/audio/voice_prewarm_desktop.go
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/audio/
../../.tools/go/bin/go test -count=1 -timeout 900s ./internal/audio/
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /tmp/main-check.wasm ./cmd
CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo-check ./cmd
```
Expected: gofmt prints nothing; both test lines print `ok`; both builds succeed.

- [ ] **Step 8: Checkpoint (DO NOT COMMIT)**

Leave the work uncommitted. Confirm `git status --porcelain src/go/internal/audio/` lists the modified files and nothing is staged.

---

### Task 2: `[bench_json]` render-latency fields

**Why:** `RenderLatencyMetrics` is emitted only as a human-readable log line (`game_update.go:116`) and is **absent from the `[bench_json]` line** (`:124`), so `scripts/bench-desktop.sh` never captures it. Without this the new counters are invisible to tooling.

**Files:**
- Modify: `src/go/internal/ui/game_update.go:115-130`

**Interfaces:**
- Consumes: `audio.RenderLatencyMetrics.DispatchRenders`, `.PrewarmRenders` (Task 1).
- Produces: `[bench_json]` keys `renderCacheHits`, `renderCacheMisses`, `renderDispatchRenders`, `renderPrewarmRenders`, `renderAvgMS`, `renderMaxMS`.

- [ ] **Step 1: Extend the human-readable line**

In `src/go/internal/ui/game_update.go`, replace the RenderLatency `Infof` call:

```go
			rl := audio.GetRenderLatencyMetrics()
			g.logger.Infof("[bench] RenderLatency: cacheHits=%d cacheMisses=%d dispatchRenders=%d prewarmRenders=%d renderAvg=%.2fms renderMax=%.2fms",
				rl.CacheHits, rl.CacheMisses, rl.DispatchRenders, rl.PrewarmRenders,
				float64(rl.RenderAvgNS)/1e6, float64(rl.RenderMaxNS)/1e6)
```

- [ ] **Step 2: Extend the machine-readable line**

In the `[bench_json]` `Debugf` call, append these keys to the format string immediately before the closing `}"` (i.e. after `\"imagesByTag\":\"%s\"`):

```
,\"renderCacheHits\":%d,\"renderCacheMisses\":%d,\"renderDispatchRenders\":%d,\"renderPrewarmRenders\":%d,\"renderAvgMS\":%.3f,\"renderMaxMS\":%.3f
```

and append these args to the end of the argument list (after the `imagesByTag` arg):

```go
				rl.CacheHits, rl.CacheMisses, rl.DispatchRenders, rl.PrewarmRenders,
				float64(rl.RenderAvgNS)/1e6, float64(rl.RenderMaxNS)/1e6,
```

Note `rl` is already in scope from Step 1 — it is declared above the `[bench_json]` call.

- [ ] **Step 3: Verify it compiles and the JSON is valid**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/ui/game_update.go
../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui/
CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo-check ./cmd
```
Expected: gofmt prints nothing, vet clean, build succeeds.

- [ ] **Step 4: Verify the emitted JSON parses**

Run a 3-second bench and validate the JSON line. **cwd MUST be `src/go`** — `go.mod` lives there, not at the repo root, so running `go run ./src/go/cmd` from the root fails module resolution. This mirrors how `Makefile:336`'s own `bench` target invokes it.
```bash
cd /home/ymolinar/Repos/beatmo/src/go && PERF_LOG=1 CGO_ENABLED=1 xvfb-run -a ../../.tools/go/bin/go run ./cmd -log DEBUG -bench-bpm 200 -bench-secs 3 2>&1 \
  | grep -o '\[bench_json\] .*' | sed 's/\[bench_json\] //' | python3 -m json.tool | grep render
```
Expected: prints `renderCacheHits`, `renderCacheMisses`, `renderDispatchRenders`, `renderPrewarmRenders`, `renderAvgMS`, `renderMaxMS` — and `python3 -m json.tool` does not error (proves the JSON is still well-formed).

- [ ] **Step 5: Checkpoint (DO NOT COMMIT)**

---

### Task 3: The param-edit-during-playback benchmark

**Why:** This is the deliverable the user asked for, and the gate that justifies Phase 2. It must be **untagged** — the existing `TestSoakHeapBounded_PlayingSynthTabParamChurn` churns knobs during playback but runs under `-tags test` where the synth is stubbed, so no real render ever happens. That is exactly why this defect was never caught.

**Files:**
- Create: `src/go/internal/ui/param_edit_playback_bench_test.go`

**Interfaces:**
- Consumes: `audio.GetRenderLatencyMetrics()`/`ResetRenderLatencyMetrics()` incl. `DispatchRenders`/`PrewarmRenders` (Task 1); `buildTestGame(t)` (`subdiv_safety_test.go:11`); `setPlayStartForAbs(g, abs)` (`testutil_test.go:568`); `advanceFrames(g, n)` (`testutil_test.go:232`); `assets_pkg.StartupDemoJSON`.
- Produces: `TestParamEditPlaybackCostTable` (the gate + evidence table).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/param_edit_playback_bench_test.go`:

```go
//go:build !test && !js

package ui

import (
	"fmt"
	"sort"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// param_edit_playback_bench_test.go — measures what a live synth-knob / sampler
// edit costs the AUDIO DISPATCH GOROUTINE during real playback.
//
// Scenario (field report): play the startup demo, drag a synth knob or change a
// sampler setting → audio cracks. The scheduler is innocent; the cost is that a
// param edit rotates the voice-cache key (hashRecipeParams covers the FULL
// merged param set), so the next trigger is a guaranteed cache MISS, and the
// miss renders SYNCHRONOUSLY on the audioLoop dispatch goroutine
// (batch_desktop.go's PlayBatch is a plain serial loop). recipe.Render is a
// ~24ms CGo call for organ-church. That does NOT underrun the oto device (a
// separate goroutine) — it makes beats LATE, which is the audible crack.
//
// MUST be untagged: under -tags test the synth is stubbed and no real render
// happens. MUST NOT call g.SetPlayFunc: that stubs the trigger and the render
// never happens, which would make this benchmark silently measure nothing.
//
// Run:
//   cd src/go && xvfb-run -a ../../.tools/go/bin/go test -count=1 \
//     -run TestParamEditPlaybackCostTable -v -timeout 900s ./internal/ui/

// churnFn mutates instrument state once per playback step, mimicking a user
// dragging a control at ~60Hz while the sequencer runs.
type churnFn func(t *testing.T, g *Game, id string, step int)

type paramEditScenario struct {
	name  string
	churn churnFn // nil = baseline (no edits)
}

// churnParamName picks a deterministic, real param of the instrument's recipe
// to drag. Derived from the recipe defaults rather than hardcoded, so this
// keeps working when the instrument's param schema changes. "pitch" is excluded
// because it is special-cased into the cache key.
func churnParamName(t *testing.T, id string) string {
	t.Helper()
	merged := audio.MergeRecipeDefaults(audio.RecipeForInstrument(id), nil)
	keys := make([]string, 0, len(merged))
	for k := range merged {
		if k == "pitch" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		t.Fatalf("instrument %q has no draggable recipe params", id)
	}
	sort.Strings(keys)
	return keys[0]
}

func TestParamEditPlaybackCostTable(t *testing.T) {
	const instID = "organ-church" // the priciest render in the demo → worst case
	const steps = 240             // playback steps per scenario

	knobParam := churnParamName(t, instID)
	t.Logf("dragging synth param %q on %s", knobParam, instID)

	scenarios := []paramEditScenario{
		{name: "baseline", churn: nil},
		{
			name: "synth-knob-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// Sweep a real param, new value every step → rotates the key.
				v := float64(step%100) / 100.0
				audio.SetInstrumentParam(id, knobParam, v)
			},
		},
		{
			name: "sampler-edit-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				// NOTE: SampleEdit.GainDB is float32 (sample_edit.go:22) while
				// StartFrac/EndFrac are float64 — do not unify the casts.
				// Also note SetSampleEdit treats an identity edit as a Clear, so
				// the (step%20==0 && step%6==0) case exercises ClearSampleEdit
				// too. That is intentional coverage, not a bug.
				audio.SetSampleEdit(id, audio.SampleEdit{
					StartFrac: 0,
					EndFrac:   1.0 - float64(step%20)*0.01,
					GainDB:    -float32(step % 6),
				})
			},
		},
		{
			// CONTROL scenario. Insert-FX params are LIVE per-block (they mutate
			// the running C instance via SetParam) and are NOT part of the voice
			// binding — so an FX drag must NOT rotate the cache key and must look
			// exactly like `baseline` here. If this scenario ever shows renders or
			// a raised missRatio, something has wrongly coupled FX params into the
			// voice binding.
			//
			// NOTE: this scenario does NOT assert anything about clicks. FX
			// discontinuity (14 of 24 C effects are unsmoothed; SetChannelEQ
			// zeroes biquad state) is the subject of a separate spec — see §6 of
			// the design doc. Gating it here would add a gate with no remediation
			// behind it.
			name: "fx-param-storm",
			churn: func(t *testing.T, g *Game, id string, step int) {
				if step == 1 {
					audio.AddInsertEffect(id, audio.EffectDistortion,
						map[string]float64{"drive": 1.4, "tone": 8000, "mix": 1})
				}
				audio.SetInsertEffectParam(id, 0, "mix", float64(step%100)/100.0)
			},
		},
	}

	fmt.Printf("\n=== param-edit during playback: audio-dispatch cost (%s, %d steps) ===\n", instID, steps)
	fmt.Printf("%-20s %-10s %-10s %-10s %-12s %-12s %-12s\n",
		"scenario", "triggers", "dispatchR", "prewarmR", "missRatio", "renderAvgMS", "renderMaxMS")

	for _, sc := range scenarios {
		// Each scenario starts from a clean instrument state, or the previous
		// scenario's edits leak in and every binding is already rotated.
		audio.ResetInstrumentParams(instID)
		audio.ClearSampleEdit(instID)
		audio.ResetAllInsertEffects()

		g := buildTestGame(t)
		if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
			t.Fatalf("%s: import startup demo: %v", sc.name, err)
		}
		g.updateBeatInfos()
		g.SetPlaying(true)
		advanceFrames(g, 5) // let playback settle before measuring

		audio.ResetRenderLatencyMetrics()

		start := g.elapsedBeats
		if start < 0 {
			start = 0
		}
		for i := 1; i <= steps; i++ {
			setPlayStartForAbs(g, start+i)
			_ = g.Update()
			if sc.churn != nil {
				sc.churn(t, g, instID, i)
			}
		}

		m := audio.GetRenderLatencyMetrics()
		triggers := m.CacheHits + m.CacheMisses
		missRatio := 0.0
		if triggers > 0 {
			missRatio = float64(m.CacheMisses) / float64(triggers)
		}

		fmt.Printf("%-20s %-10d %-10d %-10d %-12.3f %-12.3f %-12.3f\n",
			sc.name, triggers, m.DispatchRenders, m.PrewarmRenders, missRatio,
			float64(m.RenderAvgNS)/1e6, float64(m.RenderMaxNS)/1e6)

		// PRIMARY GATE (deterministic, machine-independent): invariant I3 —
		// the audioLoop dispatch goroutine must NEVER call recipe.Render.
		// Wall-clock is printed above as evidence but never asserted: a timing
		// assertion flakes under parallel suite load.
		if m.DispatchRenders != 0 {
			t.Errorf("%s: %d render(s) ran on the audio dispatch goroutine (want 0). "+
				"Each is a ~24ms CGo recipe.Render that delays every following beat "+
				"in the batch and everything queued behind it in audioCh — this is "+
				"the crack. renderMax=%.2fms",
				sc.name, m.DispatchRenders, float64(m.RenderMaxNS)/1e6)
		}

		stopPlaybackForTest(g)
	}

	// NOTE: exactMissRatio is RECORDED but deliberately NOT gated here. Phase 1
	// exists to produce the pre-fix baseline (expected ~1.0 for the sampler
	// storm, because SetSampleEdit blanket-invalidates every pitch). The gate
	// gets pinned just above the measured POST-fix value in Task 9, with
	// margin. Inventing the threshold before measuring yields a gate that
	// either never fires or flakes forever.
}
```

- [ ] **Step 2: Run it and RECORD the pre-fix baseline**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && xvfb-run -a ../../.tools/go/bin/go test -count=1 -run TestParamEditPlaybackCostTable -v -timeout 900s ./internal/ui/ 2>&1 | grep -vE "DEBUG|INFO|TRACE"
```
Expected: **FAIL.** The table prints, and `synth-knob-storm` / `sampler-edit-storm` report `dispatchR > 0` with a `renderMaxMS` in the tens of ms.

**Paste the printed table into the task notes.** It is the "before" evidence and the source of the Phase-2 gate threshold. If `baseline` also reports `dispatchR > 0`, that is expected and correct — even without edits, the first trigger at each new pitch is a cold miss unless the prewarmer got there first.

If `triggers == 0`, STOP: playback is not reaching the audio path. Check that `SetPlayFunc` was not set and that `g.SetPlaying(true)` took effect.

- [ ] **Step 3: Checkpoint (DO NOT COMMIT)**

Phase 1 is complete: the defect is now measured, and there is a failing gate. Do NOT fix anything yet.

---

## PHASE 2 — THE MODEL CHANGE

### Task 4: Pin the invariants (I1, I2, I4)

**Why:** I1/I2/I4 already hold structurally, but nothing enforces them. These tests exist so Phase 2's changes (especially the best-available fallback, which hands out a *different* buffer than requested) cannot silently break the guarantee that an edit never touches already-triggered audio.

**Files:**
- Create: `src/go/internal/audio/voice_binding_invariant_test.go`

**Interfaces:**
- Consumes: `globalVoiceCache`, `voiceCacheKey`, `pitchedVoiceKey`, `SetInstrumentParam`, `newRecipeAwareVoicePitched`, `cVoice`.
- Produces: nothing (tests only).

- [ ] **Step 1: Write the tests**

Create `src/go/internal/audio/voice_binding_invariant_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// voice_binding_invariant_test.go pins the Voice Binding contract from
// docs/superpowers/specs/2026-07-15-realtime-param-edit-audio-model-design.md:
// a node's audio is fully determined at trigger time and immutable afterward.
//
// These invariants hold structurally today (a param edit renders into a NEW
// buffer under a NEW key; cached buffers are never written; eviction only drops
// the map lookup and Go's GC keeps the slice alive for a live reader). They are
// pinned here because Phase 2 adds a best-available fallback that deliberately
// hands out a different buffer than the exact binding requested — the one thing
// it must never do is mutate a buffer someone is already playing.

// I1 — once a voice is scheduled, nothing can alter its samples.
func TestI1_InFlightVoiceIsImmutableAcrossParamEdit(t *testing.T) {
	const id = "organ-church"
	const pitch = 2.0
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	v := newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch)
	cv, ok := v.(*cVoice)
	if !ok {
		t.Fatalf("expected *cVoice, got %T", v)
	}
	if len(cv.buf) == 0 {
		t.Fatal("voice buffer is empty")
	}
	before := append([]float32(nil), cv.buf...) // snapshot

	// Storm the params while the voice is "in flight".
	for i := 0; i < 50; i++ {
		SetInstrumentParam(id, churnableParam(t, id), float64(i)/50.0)
	}

	if len(cv.buf) != len(before) {
		t.Fatalf("in-flight voice buffer was resized: %d -> %d", len(before), len(cv.buf))
	}
	for i := range before {
		if cv.buf[i] != before[i] {
			t.Fatalf("param edit mutated an already-triggered voice at sample %d: %v -> %v",
				i, before[i], cv.buf[i])
		}
	}
}

// I2 — an edit is visible to the NEXT trigger, and N edits compose latest-wins
// into ONE binding.
func TestI2_EditAppliesToNextTriggerAndComposes(t *testing.T) {
	const id = "organ-church"
	const pitch = 2.0
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	p := churnableParam(t, id)

	k0, ok := pitchedVoiceKey(id, bpm, SampleRate(), pitch)
	if !ok {
		t.Fatal("pitchedVoiceKey failed for a melodic instrument")
	}

	SetInstrumentParam(id, p, 0.25)
	k1, _ := pitchedVoiceKey(id, bpm, SampleRate(), pitch)
	if k1 == k0 {
		t.Fatal("a param edit did not rotate the binding — the next trigger would read stale audio")
	}

	// Latest-wins: setting the same param again lands on ONE binding, and
	// returning to the original value returns to the original binding.
	SetInstrumentParam(id, p, 0.75)
	k2, _ := pitchedVoiceKey(id, bpm, SampleRate(), pitch)
	if k2 == k1 {
		t.Fatal("second edit did not rotate the binding")
	}
	SetInstrumentParam(id, p, 0.25)
	k3, _ := pitchedVoiceKey(id, bpm, SampleRate(), pitch)
	if k3 != k1 {
		t.Fatal("binding is not a pure function of the composed param state " +
			"(same value must yield the same binding)")
	}
}

// I4 — an edit to instrument A cannot change instrument B's binding.
func TestI4_EditIsolatedToItsInstrument(t *testing.T) {
	const a = "organ-church"
	const b = "snare"
	globalVoiceCache.Clear()
	ResetInstrumentParams(a)
	ResetInstrumentParams(b)
	t.Cleanup(func() { ResetInstrumentParams(a); ResetInstrumentParams(b) })

	bBefore := hashRecipeParams(MergeRecipeDefaults(RecipeForInstrument(b), GetInstrumentParams(b)))
	for i := 0; i < 20; i++ {
		SetInstrumentParam(a, churnableParam(t, a), float64(i)/20.0)
	}
	bAfter := hashRecipeParams(MergeRecipeDefaults(RecipeForInstrument(b), GetInstrumentParams(b)))
	if bAfter != bBefore {
		t.Fatalf("editing %q changed %q's binding (%x -> %x)", a, b, bBefore, bAfter)
	}
}

// churnableParam returns a deterministic real param of id's recipe, excluding
// the special-cased "pitch".
func churnableParam(t *testing.T, id string) string {
	t.Helper()
	merged := MergeRecipeDefaults(RecipeForInstrument(id), nil)
	best := ""
	for k := range merged {
		if k == "pitch" {
			continue
		}
		if best == "" || k < best {
			best = k
		}
	}
	if best == "" {
		t.Fatalf("instrument %q has no editable recipe params", id)
	}
	return best
}
```

- [ ] **Step 2: Run them — they must PASS on the current tree**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'TestI1_|TestI2_|TestI4_' -v ./internal/audio/
```
Expected: **PASS** — these pin behavior that already holds. If any FAILS, stop and investigate: the spec's claim that I1/I2/I4 hold structurally would be wrong, and Phase 2's design rests on it.

- [ ] **Step 3: Checkpoint (DO NOT COMMIT)**

---

### Task 5: Verify, then remove the redundant sampler blanket-invalidate

**Why:** `SetSampleEdit`/`ClearSampleEdit` call `voiceCacheInvalidate(instID)`, dropping **every pitch's** entry. But `hashSampleEdit(edit)` is already XORed into `paramsHash` (`synth_recipe_dispatch.go:138`, `:192`, `:264`), so the edit rotates the key by itself. The invalidate is redundant — and it recreates the exact 93%-miss blanket-clear storm that `SetInstrumentParam` was explicitly fixed to avoid (`synth_recipe.go:256-267`).

**VERIFY FIRST — do not assume.** The user-sample/WAV path goes through `reapplyUserSampleEdit` → `RegisterSamplePCM` (`sample_edit_descriptor.go:88-100`), which is **not** voice-cache-keyed. The invalidate may be load-bearing there.

**Files:**
- Create: `src/go/internal/audio/sample_edit_storm_test.go`
- Modify: `src/go/internal/audio/sample_edit_descriptor.go:65`, `:111`

**Interfaces:**
- Consumes: `SetSampleEdit`, `ClearSampleEdit`, `pitchedVoiceKey`, `globalVoiceCache`.
- Produces: nothing (behavior change only).

- [ ] **Step 1: Write the failing storm test**

Create `src/go/internal/audio/sample_edit_storm_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// A sample-edit must rotate the binding, NOT blanket-drop every other pitch's
// cached voice. hashSampleEdit is already XORed into paramsHash, so the key
// rotates by itself; an additional voiceCacheInvalidate is redundant and
// recreates the 93%-miss knob-drag storm that SetInstrumentParam was fixed to
// avoid (synth_recipe.go:256-267).
func TestSetSampleEditDoesNotBlanketInvalidateOtherPitches(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	ClearSampleEdit(id)
	ResetInstrumentParams(id)
	t.Cleanup(func() { ClearSampleEdit(id); ResetInstrumentParams(id) })

	// Warm three distinct pitches.
	pitches := []float64{-2, 0, 5}
	for _, p := range pitches {
		PrewarmVoice(id, p)
	}
	for _, p := range pitches {
		if !VoiceCached(id, p) {
			t.Fatalf("setup: pitch %v did not warm", p)
		}
	}

	// A sample edit rotates the binding for EVERY pitch (the edit hash is in
	// the key), so VoiceCached for the NEW binding is legitimately false.
	// What must NOT happen is the OLD entries being dropped: they are what a
	// pre-warmed neighbour relies on, and dropping them makes every subsequent
	// note a cold render.
	before := voiceCacheEntryCountForTest()
	SetSampleEdit(id, SampleEdit{StartFrac: 0, EndFrac: 0.9, GainDB: -3})
	after := voiceCacheEntryCountForTest()

	if after < before {
		t.Errorf("SetSampleEdit dropped %d cached voice entries (%d -> %d). "+
			"The edit hash already rotates the key; the blanket invalidate is "+
			"redundant and re-creates the knob-drag storm.", before-after, before, after)
	}
}

// voiceCacheEntryCountForTest reports the number of live cache entries.
func voiceCacheEntryCountForTest() int {
	globalVoiceCache.mu.RLock()
	defer globalVoiceCache.mu.RUnlock()
	return len(globalVoiceCache.entries)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run TestSetSampleEditDoesNotBlanketInvalidateOtherPitches -v ./internal/audio/
```
Expected: FAIL — `SetSampleEdit dropped 3 cached voice entries (3 -> 0)`.

- [ ] **Step 3: Prove whether the invalidate is load-bearing for WAV instruments**

Before removing anything, establish whether a WAV-backed (user-sample) instrument depends on the invalidate. Run the existing sampler/sample-edit suites:

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'SampleEdit|Sampler|UserSample' -v ./internal/audio/ 2>&1 | grep -E "^--- |^ok|^FAIL"
```
Record which tests exist and pass. These are the guard for Step 4.

Reason about the code path and write the conclusion into the task notes:
- Synth-backed instruments render through `tryRecipeVoice*`, whose key already folds `hashSampleEdit` → the invalidate is redundant.
- WAV-backed instruments are re-derived by `reapplyUserSampleEdit` → `RegisterSamplePCM`, which replaces the playable PCM directly and does **not** consult `globalVoiceCache`.

- [ ] **Step 4: Remove the redundant invalidate**

In `src/go/internal/audio/sample_edit_descriptor.go`, in `SetSampleEdit`, delete the line:

```go
	voiceCacheInvalidate(instID)
```

and replace it with this comment above `reapplyUserSampleEdit(instID, e)`:

```go
	// NOTE: no voiceCacheInvalidate here — deliberately. hashSampleEdit(e) is
	// already XORed into the voice-cache key (synth_recipe_dispatch.go:138,
	// :192, :264), so a new edit maps to a DIFFERENT key: dispatch renders the
	// new sound there and the old-value entry is simply not read. Blanket-
	// clearing dropped EVERY pitch's entry, making each following note a cold
	// ~24ms render on the audio dispatch goroutine — the same 93%-miss storm
	// SetInstrumentParam was fixed to avoid (synth_recipe.go:256-267).
	// WAV/user-sample instruments do not rely on the cache here: they are
	// re-derived below through RegisterSamplePCM, which is not cache-keyed.
```

Apply the identical removal + comment in `ClearSampleEdit`.

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -count=1 -run TestSetSampleEditDoesNotBlanketInvalidateOtherPitches -v ./internal/audio/
../../.tools/go/bin/go test -count=1 -run 'SampleEdit|Sampler|UserSample' ./internal/audio/
```
Expected: both PASS. If any sampler test from Step 3 now fails, the invalidate WAS load-bearing for that path — restore it **narrowed to the user-sample case only**:

```go
	if _, isUserSample := UserSamplePCM(instID); isUserSample {
		voiceCacheInvalidate(instID)
	}
```

- [ ] **Step 6: Re-run the benchmark as a MEASUREMENT (it is still expected to FAIL)**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && xvfb-run -a ../../.tools/go/bin/go test -count=1 -run TestParamEditPlaybackCostTable -v -timeout 900s ./internal/ui/ 2>&1 | grep -vE "DEBUG|INFO|TRACE"
```
Expected: **still FAIL on `dispatchR > 0`** — that gate is Task 8's job, not this task's. What you are checking here is the table: `sampler-edit-storm`'s `missRatio` must drop sharply versus the Task-3 baseline. Record the table in your report. **Do not attempt to make this test pass.**

- [ ] **Step 7: Full verification + Checkpoint (DO NOT COMMIT)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/sample_edit_descriptor.go
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/audio/
../../.tools/go/bin/go test -count=1 -timeout 900s ./internal/audio/
```
Expected: gofmt silent, both `ok`. (The `internal/ui` suite is not run here — `TestParamEditPlaybackCostTable` is intentionally red until Task 8.)

---

### Task 6: `PrewarmVoiceParams` — warm an explicit binding

**Why:** Bind-ahead prewarm (Task 7) must render the binding an edit *just created*. `PrewarmVoice` reads `GetInstrumentParams(id)` internally, so it always warms the CURRENT params — racy if another edit lands first, and it cannot warm a binding on behalf of a caller holding a snapshot. **The warm key must be byte-identical to the key dispatch reads**; warming a key nobody reads is this design's primary failure mode.

**Files:**
- Modify: `src/go/internal/audio/voice_prewarm_desktop.go`
- Modify: `src/go/internal/audio/voice_prewarm_stub.go`
- Modify: `src/go/internal/audio/synth_recipe_dispatch.go`
- Test: `src/go/internal/audio/render_origin_test.go` (add to it)

**Interfaces:**
- Consumes: `renderOrigin`, `originPrewarm` (Task 1).
- Produces: `func PrewarmVoiceParams(id string, pitch float64, params RecipeParams)` (both builds); `func pitchedVoiceKeyForParams(id string, bpm, sampleRate int, pitch float64, params RecipeParams) (voiceCacheKey, bool)`.

- [ ] **Step 1: Write the failing linchpin test**

Append to `src/go/internal/audio/render_origin_test.go`:

```go
// THE LINCHPIN. PrewarmVoiceParams must produce the byte-identical cache key
// that dispatch later reads. If the two key computations drift, prewarm renders
// into a slot nobody reads: the cache "works", CPU is burned, and dispatch
// still cold-renders. Guard both directions: warm-then-dispatch must be a HIT.
func TestPrewarmVoiceParamsKeyMatchesDispatch(t *testing.T) {
	const id = "organ-church"
	const pitch = -2.0

	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	p := churnableParam(t, id)
	SetInstrumentParam(id, p, 0.375)
	params := GetInstrumentParams(id)

	ResetRenderLatencyMetrics()
	PrewarmVoiceParams(id, pitch, params)
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders != 1 {
		t.Fatalf("PrewarmVoiceParams did not render off-thread: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}

	// Dispatch must now HIT the warmed key.
	ResetRenderLatencyMetrics()
	newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch)
	m := GetRenderLatencyMetrics()
	if m.CacheHits != 1 || m.DispatchRenders != 0 {
		t.Fatalf("warm key != read key: dispatch got hits=%d dispatchRenders=%d "+
			"(want hits=1 dispatchRenders=0). PrewarmVoiceParams warmed a slot "+
			"dispatch does not read.", m.CacheHits, m.DispatchRenders)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run TestPrewarmVoiceParamsKeyMatchesDispatch -v ./internal/audio/
```
Expected: FAIL to compile — `undefined: PrewarmVoiceParams`.

- [ ] **Step 3: Extract a params-taking render core**

In `src/go/internal/audio/synth_recipe_dispatch.go`, add below `pitchedVoiceKey`:

```go
// pitchedVoiceKeyForParams is pitchedVoiceKey with the params supplied by the
// caller instead of read from the manager. The ONLY difference is the params
// source — every other term (merge, pitch override, hash, edit fold, rounding)
// is computed identically, so a key produced here is byte-identical to the key
// dispatch reads for the same param state. Keep these two functions adjacent
// and edit them together; drift here silently defeats prewarming
// (TestPrewarmVoiceParamsKeyMatchesDispatch guards it).
func pitchedVoiceKeyForParams(id string, bpm, sampleRate int, pitch float64, params RecipeParams) (voiceCacheKey, bool) {
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return voiceCacheKey{}, false
	}
	if NewRecipe(recipeID) == nil {
		return voiceCacheKey{}, false
	}
	samples, bpmKey := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return voiceCacheKey{}, false
	}
	merged := MergeRecipeDefaults(recipeID, params)
	merged["pitch"] = pitch
	ph := hashRecipeParams(merged)
	if edit, hasEdit := SampleEditFor(id); hasEdit {
		ph ^= hashSampleEdit(edit)
	}
	return voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   ph,
		pitch:        roundPitchForCache(pitch),
	}, true
}

// renderPitchedIntoCacheForParams renders (id, pitch) under the binding implied
// by params and stores it in globalVoiceCache. No-op when already cached.
// Returns false when id has no renderable pitch-aware recipe.
func renderPitchedIntoCacheForParams(id string, bpm, sampleRate int, pitch float64, params RecipeParams, origin renderOrigin) bool {
	recipeID := RecipeForInstrument(id)
	if !pitchAwareRecipe(recipeID) {
		return false
	}
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		return false
	}
	key, ok := pitchedVoiceKeyForParams(id, bpm, sampleRate, pitch, params)
	if !ok {
		return false
	}
	if globalVoiceCache.Has(key) {
		return true // already warm — cheap no-op
	}
	samples, _ := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return false
	}
	merged := MergeRecipeDefaults(recipeID, params)
	merged["pitch"] = pitch

	buf := make([]float32, samples)
	renderStart := time.Now()
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	recordVoiceRender(time.Since(renderStart), origin)
	if edit, hasEdit := SampleEditFor(id); hasEdit {
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
	}
	globalVoiceCache.Put(key, buf)
	return true
}
```

- [ ] **Step 4: Add `PrewarmVoiceParams`**

In `src/go/internal/audio/voice_prewarm_desktop.go`, append:

```go
// PrewarmVoiceParams renders (id, pitch) under an EXPLICIT param snapshot into
// globalVoiceCache, off the caller's hot path. Unlike PrewarmVoice (which reads
// the current params internally), this warms the binding a specific edit
// created, so a fast drag cannot race the warm onto the wrong binding.
//
// The key is computed by pitchedVoiceKeyForParams, which is byte-identical to
// the dispatch key computation — see TestPrewarmVoiceParamsKeyMatchesDispatch.
// Non-pitch-aware instruments fall back to PrewarmVoice (pitch is a post-render
// resample for them, so the base render is the only binding that matters).
func PrewarmVoiceParams(id string, pitch float64, params RecipeParams) {
	if id == "" {
		return
	}
	if !pitchAwareRecipe(RecipeForInstrument(id)) {
		PrewarmVoice(id, pitch)
		return
	}
	renderPitchedIntoCacheForParams(id, bpm, SampleRate(), pitch, params, originPrewarm)
}
```

In `src/go/internal/audio/voice_prewarm_stub.go`, append:

```go
// PrewarmVoiceParams is a no-op under -tags test (the C renderer is stubbed).
func PrewarmVoiceParams(id string, pitch float64, params RecipeParams) {}
```

- [ ] **Step 5: Run the test to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'TestPrewarmVoiceParamsKeyMatchesDispatch|TestRenderOriginAttribution|TestVoiceCachedMatchesDispatch|TestPrewarmVoiceKeyMatchesDispatch' -v ./internal/audio/
```
Expected: all PASS.

- [ ] **Step 6: Checkpoint (DO NOT COMMIT)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod ./internal/audio/
../../.tools/go/bin/go test -count=1 -timeout 900s ./internal/audio/
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /tmp/main-check.wasm ./cmd
```

---

### Task 7: Bind-ahead prewarm on param edit

**Why:** Today nothing renders the binding an edit creates until the trigger demands it. This wires the edit to the prewarm pool so the render is already done by the next trigger — which is what makes "the edit applies on the next trigger" true in the common case rather than only in principle.

**Files:**
- Modify: `src/go/internal/audio/synth_recipe.go` (publish the observer call)
- Create: `src/go/internal/audio/binding_observer.go` (untagged)
- Modify: `src/go/internal/ui/voice_prewarmer.go`
- Test: `src/go/internal/ui/voice_prewarmer_binding_test.go` (create)

**Interfaces:**
- Consumes: `PrewarmVoiceParams` (Task 6); `voicePrewarmer.pitchesByInst`, `warmPitch` (`internal/ui/voice_prewarmer.go`).
- Produces: `var InstrumentBindingChangedObserver func(id string, params RecipeParams)`; `notifyInstrumentBindingChanged(id string, params RecipeParams)`; `(*voicePrewarmer).warmBinding(id string, params audio.RecipeParams)`.

- [ ] **Step 1: Add the observer hook**

Create `src/go/internal/audio/binding_observer.go`:

```go
package audio

// binding_observer.go — notification that an instrument's VOICE BINDING changed
// (its params rotated the cache key), so an interested party can render the new
// binding off-thread BEFORE the next trigger asks for it.
//
// Why not reuse VoiceCacheInvalidateObserver: SetInstrumentParam deliberately
// does NOT invalidate (synth_recipe.go:256-267), so that observer never fires
// on the knob-drag hot path — the exact path that needs warming.
//
// Why not reuse platformInstrumentParamsChanged: that is the JS bridge. Hanging
// desktop prewarm off a platform bridge conflates two unrelated concerns.
//
// Set by internal/ui at startup; nil in tests that don't need it.
var InstrumentBindingChangedObserver func(id string, params RecipeParams)

// notifyInstrumentBindingChanged fires the observer if one is set. Called AFTER
// the params-manager lock is released, so the observer may re-enter the audio
// package safely.
func notifyInstrumentBindingChanged(id string, params RecipeParams) {
	if f := InstrumentBindingChangedObserver; f != nil {
		f(id, params)
	}
}
```

- [ ] **Step 2: Fire it from the edit paths**

In `src/go/internal/audio/synth_recipe.go`, in `SetInstrumentParam`, immediately after `platformInstrumentParamsChanged(instrumentID, snapshot)` (the end of the function), add:

```go
	// Render the binding this edit just created, off-thread, so the next
	// trigger is a cache HIT instead of a ~24ms cold render on the audio
	// dispatch goroutine. Passes the SNAPSHOT (not the live map) so a faster
	// edit landing behind us cannot make us warm the wrong binding.
	notifyInstrumentBindingChanged(instrumentID, snapshot)
```

Add the identical call at the end of `SetInstrumentParams` and `ResetInstrumentParams` (both already compute `snapshot`; `ResetInstrumentParams` passes `RecipeParams{}`).

- [ ] **Step 3: Write the failing prewarmer test**

Create `src/go/internal/ui/voice_prewarmer_binding_test.go`:

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A param edit must warm the NEW binding at every pitch the instrument is
// currently enumerated to play, so the next trigger hits the cache instead of
// cold-rendering on the audio dispatch goroutine.
func TestParamEditWarmsNewBindingAtEnumeratedPitches(t *testing.T) {
	g := buildTestGame(t)
	p := g.voicePrewarmer
	if p == nil {
		t.Fatal("game has no voicePrewarmer")
	}

	type warm struct {
		id    string
		pitch float64
	}
	var got []warm
	p.mu.Lock()
	p.pitchesByInst = map[string][]float64{"organ-church": {-2, 0, 5}}
	p.mu.Unlock()
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) {
		got = append(got, warm{id, pitch})
	}

	p.warmBinding("organ-church", audio.RecipeParams{"drive": 0.5})

	if len(got) != 3 {
		t.Fatalf("warmBinding warmed %d pitches, want 3 (the enumerated set): %+v", len(got), got)
	}
	for _, w := range got {
		if w.id != "organ-church" {
			t.Fatalf("warmed the wrong instrument: %q", w.id)
		}
	}
}

// An edit to an instrument with no enumerated pitches must not warm anything
// (nothing is going to trigger it).
func TestParamEditOnUnenumeratedInstrumentWarmsNothing(t *testing.T) {
	g := buildTestGame(t)
	p := g.voicePrewarmer
	called := 0
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) { called++ }
	p.warmBinding("not-in-the-graph", audio.RecipeParams{"drive": 0.5})
	if called != 0 {
		t.Fatalf("warmed %d voices for an instrument that never triggers", called)
	}
}
```

- [ ] **Step 4: Run it to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -run 'TestParamEditWarms|TestParamEditOnUnenumerated' -v ./internal/ui/
```
Expected: FAIL to compile — `p.warmParamsFn undefined` and `p.warmBinding undefined`.

- [ ] **Step 5: Implement `warmBinding`**

In `src/go/internal/ui/voice_prewarmer.go`, add a field to the `voicePrewarmer` struct next to `warmFn`:

```go
	// warmParamsFn warms an EXPLICIT binding. Production submits
	// audio.PrewarmVoiceParams to the pool; tests inject a recorder.
	warmParamsFn func(id string, pitch float64, params audio.RecipeParams)
```

In `newVoicePrewarmer`, after the existing `p.warmFn = ...` assignment, add:

```go
	p.warmParamsFn = func(id string, pitch float64, params audio.RecipeParams) {
		// Non-blocking: on backpressure the warm is dropped and dispatch
		// degrades to the best-available fallback — never worse, never blocks.
		_ = p.pool.Submit(func(context.Context) { audio.PrewarmVoiceParams(id, pitch, params) })
	}
	// Warm the new binding whenever a param edit rotates an instrument's key.
	audio.InstrumentBindingChangedObserver = func(id string, params audio.RecipeParams) {
		p.warmBinding(id, params)
	}
```

In `release()`, alongside `audio.VoiceCacheInvalidateObserver = nil`, add:

```go
	audio.InstrumentBindingChangedObserver = nil
```

Add the method:

```go
// warmBinding renders the binding implied by params at every pitch id is
// currently enumerated to play, off-thread, so the next trigger is a cache HIT
// rather than a ~24ms cold render on the audio dispatch goroutine.
//
// Reads only the prewarmer's own recorded pitch enumeration, so it is safe to
// call from the edit goroutine (which is the UI goroutine, possibly under
// seqMu) without touching Game state.
func (p *voicePrewarmer) warmBinding(id string, params audio.RecipeParams) {
	if id == "" || p.warmParamsFn == nil {
		return
	}
	p.mu.Lock()
	pitches := append([]float64(nil), p.pitchesByInst[id]...)
	p.mu.Unlock()
	for _, pitch := range pitches {
		p.warmParamsFn(id, pitch, params)
	}
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -run 'TestParamEditWarms|TestParamEditOnUnenumerated' -v ./internal/ui/
```
Expected: PASS.

- [ ] **Step 7: Checkpoint (DO NOT COMMIT)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/ internal/ui/
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/ui/
```
Expected: gofmt silent, `ok`. The `-tags test` build does not compile `param_edit_playback_bench_test.go` at all (it is `//go:build !test && !js`), so the intentionally-red Phase-1 gate cannot affect this run.

---

### Task 8: Non-blocking dispatch + best-available fallback

**Why:** This is invariant I3 and the core of the whole plan. Prewarm makes misses rare but cannot make them impossible (a fast full-range sweep outruns any warm). Without this, the gate `DispatchRenders == 0` can never be guaranteed — only made likely. User decision: **never block; play best-available on time and converge next trigger.**

**Files:**
- Modify: `src/go/internal/audio/synth_recipe_dispatch.go`
- Modify: `src/go/internal/audio/render_latency.go` (add `StalePlays`)
- Modify: `src/go/internal/audio/voice_cache.go` (add `getAnyForInstrumentPitch`)
- Test: `src/go/internal/audio/nonblocking_dispatch_test.go` (create)

**Interfaces:**
- Consumes: `renderOrigin` (Task 1); `PrewarmVoiceParams` (Task 6).
- Produces: `var dispatchMustNotRender atomic.Bool`; `RenderLatencyMetrics.StalePlays int64`; `func (vc *voiceCache) getAnyForInstrumentPitch(id string, pitch float64) ([]float32, bool)`; `var asyncRenderSubmit func(id string, pitch float64, params RecipeParams)`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/audio/nonblocking_dispatch_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// INVARIANT I3: with non-blocking dispatch armed, a cache miss must NEVER
// render on the caller's goroutine. It must serve the best available buffer and
// hand the exact render off-thread.
func TestNonBlockingDispatchNeverRendersInline(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() {
		ResetInstrumentParams(id)
		dispatchMustNotRender.Store(false)
		asyncRenderSubmit = nil
	})

	// Warm ONE binding at pitch 0 so a best-available candidate exists.
	PrewarmVoice(id, 0)

	var submitted int
	asyncRenderSubmit = func(id string, pitch float64, params RecipeParams) { submitted++ }
	dispatchMustNotRender.Store(true)

	// Rotate the binding so the next trigger is a guaranteed miss.
	SetInstrumentParam(id, churnableParam(t, id), 0.61)

	ResetRenderLatencyMetrics()
	v := newRecipeAwareVoicePitched(id, bpm, SampleRate(), 0)
	m := GetRenderLatencyMetrics()

	if v == nil {
		t.Fatal("dispatch returned no voice — a miss must still produce audio, on time")
	}
	if m.DispatchRenders != 0 {
		t.Errorf("dispatch rendered inline despite the non-blocking gate: DispatchRenders=%d want 0", m.DispatchRenders)
	}
	if m.StalePlays != 1 {
		t.Errorf("StalePlays=%d want 1 — a best-available fallback must be counted, "+
			"otherwise a permanently-stale instrument looks healthy", m.StalePlays)
	}
	if submitted != 1 {
		t.Errorf("asyncRenderSubmit called %d times, want 1 — without the hand-off "+
			"the exact binding never renders and the instrument stays stale forever", submitted)
	}
}

// With the gate DISARMED (e.g. prewarm pool calls), rendering inline is correct.
func TestNonBlockingGateDoesNotAffectPrewarm(t *testing.T) {
	const id = "organ-church"
	globalVoiceCache.Clear()
	dispatchMustNotRender.Store(false)
	t.Cleanup(func() { dispatchMustNotRender.Store(false) })

	ResetRenderLatencyMetrics()
	PrewarmVoice(id, 4)
	if m := GetRenderLatencyMetrics(); m.PrewarmRenders != 1 {
		t.Fatalf("prewarm must still render: PrewarmRenders=%d want 1", m.PrewarmRenders)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'TestNonBlockingDispatch|TestNonBlockingGate' -v ./internal/audio/
```
Expected: FAIL to compile — `undefined: dispatchMustNotRender`, `asyncRenderSubmit`, `m.StalePlays`.

- [ ] **Step 3: Add the `StalePlays` counter**

In `src/go/internal/audio/render_latency.go`: add `stalePlays atomic.Int64` to `renderLatencyCounters`; add to `RenderLatencyMetrics`:

```go
	// StalePlays: triggers served by the best-available fallback because the
	// exact binding was not rendered yet. Low is healthy; persistently high
	// means prewarm is not keeping up and edits are landing a trigger late.
	StalePlays int64
```

add the recorder:

```go
// recordStalePlay counts one best-available fallback (the exact binding was not
// ready, so an older/nearest render played ON TIME instead of blocking).
func recordStalePlay() { renderLatency.stalePlays.Add(1) }
```

wire it into `GetRenderLatencyMetrics` (`StalePlays: renderLatency.stalePlays.Load(),`) and `ResetRenderLatencyMetrics` (`renderLatency.stalePlays.Store(0)`).

- [ ] **Step 4: Add best-available lookup to the cache**

In `src/go/internal/audio/voice_cache.go`, append:

```go
// getAnyForInstrumentPitch returns any cached buffer for (instrumentID, pitch)
// under ANY paramsHash, preferring the most-recently-used. This is the
// best-available fallback source: when the exact binding is not rendered yet,
// an older-params render of the SAME note at the SAME pitch is far closer to
// correct than either blocking (a late beat) or silence (a dropped beat).
func (vc *voiceCache) getAnyForInstrumentPitch(id string, pitch float64) ([]float32, bool) {
	rounded := roundPitchForCache(pitch)
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	var best *roundRobinEntry
	var bestNS int64
	for k, e := range vc.entries {
		if k.instrumentID != id || k.pitch != rounded || len(e.variants) == 0 {
			continue
		}
		if ln := e.lastNS.Load(); best == nil || ln > bestNS {
			best, bestNS = e, ln
		}
	}
	if best == nil {
		return nil, false
	}
	idx := best.counter.Add(1) % uint64(len(best.variants))
	return best.variants[idx], true
}
```

- [ ] **Step 5: Add the gate and the fallback to dispatch**

In `src/go/internal/audio/synth_recipe_dispatch.go`, add near the top:

```go
// dispatchMustNotRender arms invariant I3: when true, a cache miss on the
// dispatch path must NOT call recipe.Render (a ~24ms CGo call that would delay
// every following beat). Instead it serves the best available buffer ON TIME
// and hands the exact render off-thread. Armed by the UI once the prewarmer
// exists; left false in tests and tools that render synchronously on purpose.
var dispatchMustNotRender atomic.Bool

// asyncRenderSubmit hands an exact (id, pitch, params) render to an off-thread
// pool. Set by internal/ui to the prewarm pool. When nil the fallback still
// plays on time; the binding simply warms on a later edit/enumeration.
var asyncRenderSubmit func(id string, pitch float64, params RecipeParams)

// serveBestAvailable implements the miss policy for a non-blocking dispatch:
// play the closest already-rendered buffer at the correct time, count it, and
// submit the exact render off-thread so the NEXT trigger is correct.
// Returns (nil, false) when nothing is cached for this note at all — the caller
// must then fall through to a normal (blocking) render, because silence is
// worse than a late beat.
func serveBestAvailable(id string, pitch float64) (Voice, bool) {
	buf, ok := globalVoiceCache.getAnyForInstrumentPitch(id, pitch)
	if !ok {
		return nil, false
	}
	recordStalePlay()
	if f := asyncRenderSubmit; f != nil {
		f(id, pitch, GetInstrumentParams(id))
	}
	return &cVoice{buf: buf}, true
}
```

Add `"sync/atomic"` to the imports.

In `tryRecipeVoicePitchedOrigin`, insert the gate immediately before **each** of the two `buf := make([]float32, samples)` render blocks (the folded-key block and the plain-key block):

```go
	if origin == originDispatch && dispatchMustNotRender.Load() {
		if v, ok := serveBestAvailable(id, pitch); ok {
			return v, true
		}
		// Nothing cached for this note at all — fall through and render.
		// A late first note beats no note.
	}
```

- [ ] **Step 6: Arm the gate from the UI**

In `src/go/internal/ui/voice_prewarmer.go`, in `newVoicePrewarmer`, after the `warmParamsFn` assignment, add:

```go
	// Hand exact renders to the prewarm pool and arm invariant I3: from here
	// on, the audioLoop dispatch goroutine must never call recipe.Render.
	audio.SetAsyncRenderSubmit(func(id string, pitch float64, params audio.RecipeParams) {
		_ = p.pool.Submit(func(context.Context) { audio.PrewarmVoiceParams(id, pitch, params) })
	})
	audio.SetDispatchMustNotRender(true)
```

In `release()`, add:

```go
	audio.SetDispatchMustNotRender(false)
	audio.SetAsyncRenderSubmit(nil)
```

Add the exported setters. In `src/go/internal/audio/synth_recipe_dispatch.go`:

```go
// SetDispatchMustNotRender arms/disarms invariant I3 (see dispatchMustNotRender).
func SetDispatchMustNotRender(on bool) { dispatchMustNotRender.Store(on) }

// SetAsyncRenderSubmit installs the off-thread render hand-off (see asyncRenderSubmit).
func SetAsyncRenderSubmit(f func(id string, pitch float64, params RecipeParams)) {
	asyncRenderSubmit = f
}
```

And matching no-op stubs in `src/go/internal/audio/voice_prewarm_stub.go` (so the `-tags test` UI build links):

```go
// SetDispatchMustNotRender is a no-op under -tags test (the C renderer is stubbed).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op under -tags test.
func SetAsyncRenderSubmit(f func(id string, pitch float64, params RecipeParams)) {}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'TestNonBlockingDispatch|TestNonBlockingGate|TestI1_|TestI2_|TestI4_|TestRenderOrigin|TestPrewarmVoiceParams' -v ./internal/audio/
```
Expected: all PASS. **I1 must still pass** — the fallback hands out a shared buffer, and if it ever mutated one, I1 fails.

- [ ] **Step 8: Run the benchmark — the primary gate must now pass**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && xvfb-run -a ../../.tools/go/bin/go test -count=1 -run TestParamEditPlaybackCostTable -v -timeout 900s ./internal/ui/ 2>&1 | grep -vE "DEBUG|INFO|TRACE"
```
Expected: **PASS** — `dispatchR == 0` for every scenario. Record the table; compare against the Task-3 baseline.

- [ ] **Step 9: Checkpoint (DO NOT COMMIT)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/ internal/ui/
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/audio/ 
../../.tools/go/bin/go test -count=1 -timeout 900s ./internal/audio/
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/ui/
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /tmp/main-check.wasm ./cmd
CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo-check ./cmd
```

---

### Task 9: Eviction pinning (the `exactMissRatio` gate was dropped — see Step 5)

**Why:** `evictToBudgetLocked`/`evictLRULocked` drop entries with **no** `notifyVoiceCacheInvalidated` call, so an evicted voice is never re-warmed and cold-renders on the next trigger — a silent source of `StalePlays`. Pinning the bindings the next triggers will read closes it. Then, with the system fixed, the miss-ratio gate can finally be set from data.

**Files:**
- Modify: `src/go/internal/audio/voice_cache.go`
- Modify: `src/go/internal/ui/param_edit_playback_bench_test.go`
- Test: `src/go/internal/audio/voice_cache_pin_test.go` (create)

**Interfaces:**
- Consumes: `voiceCacheKey`, `evictLRULocked`.
- Produces: `func PinVoiceBindings(keys []voiceCacheKey)` — internal; exported wrapper `func SetPinnedInstrumentPitches(id string, pitches []float64)`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/audio/voice_cache_pin_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// A pinned binding (one the next triggers will read) must survive LRU eviction.
// Eviction fires with no observer, so an evicted binding is never re-warmed and
// cold-renders on the next trigger.
func TestPinnedBindingSurvivesEviction(t *testing.T) {
	vc := &voiceCache{
		entries:     make(map[voiceCacheKey]*roundRobinEntry),
		maxEntries:  defaultVoiceCacheMaxEntries,
		budgetBytes: 1 << 20, // 1MB — small enough to force eviction
		lastSamples: make(map[string][]float32),
	}
	pinned := voiceCacheKey{instrumentID: "organ-church", pitch: 0, paramsHash: 1}
	vc.pin(map[voiceCacheKey]struct{}{pinned: {}})

	// Insert the pinned entry first so it is the LRU victim by age.
	vc.Put(pinned, make([]float32, 64*1024)) // 256KB

	// Flood with other entries to blow the budget.
	for i := 2; i < 12; i++ {
		vc.Put(voiceCacheKey{instrumentID: "filler", pitch: float64(i), paramsHash: uint64(i)},
			make([]float32, 64*1024))
	}

	vc.mu.RLock()
	_, stillThere := vc.entries[pinned]
	vc.mu.RUnlock()
	if !stillThere {
		t.Error("LRU eviction dropped a PINNED binding — the next trigger will cold-render " +
			"on the audio dispatch goroutine and nothing will re-warm it (eviction has no observer)")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run TestPinnedBindingSurvivesEviction -v ./internal/audio/
```
Expected: FAIL to compile — `vc.pin undefined`.

- [ ] **Step 3: Implement pinning**

In `src/go/internal/audio/voice_cache.go`, add a field to `voiceCache`:

```go
	pinned map[voiceCacheKey]struct{} // bindings the next triggers will read; never evicted
```

Add:

```go
// pin replaces the pinned set: the bindings the next triggers are expected to
// read. Pinned entries are never LRU-evicted, because eviction fires no
// observer — an evicted binding is never re-warmed and cold-renders on the next
// trigger. Bounded by the enumerated node count, so the byte budget stays
// meaningful.
func (vc *voiceCache) pin(keys map[voiceCacheKey]struct{}) {
	vc.mu.Lock()
	vc.pinned = keys
	vc.mu.Unlock()
}

// isPinnedLocked reports whether k is pinned. Caller holds vc.mu.
func (vc *voiceCache) isPinnedLocked(k voiceCacheKey) bool {
	if vc.pinned == nil {
		return false
	}
	_, ok := vc.pinned[k]
	return ok
}
```

In `evictLRULocked`, skip pinned keys — change the loop guard from:

```go
		if k == keep {
			continue
		}
```
to:
```go
		if k == keep || vc.isPinnedLocked(k) {
			continue
		}
```

In `evictToBudgetLocked`, the existing `len(vc.entries) > 1` guard plus the `before == len(vc.entries)` break already terminate when only pinned entries remain — no change needed, but add this comment above the loop:

```go
	// If everything left is pinned, evictLRULocked finds no victim and the
	// break below fires: we go over budget rather than evict a binding the next
	// trigger needs. Pinned set is bounded by node count, so this is bounded.
```

- [ ] **Step 4: Run it to verify it passes**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -count=1 -run 'TestPinnedBindingSurvivesEviction|TestVoiceCache' -v ./internal/audio/
```
Expected: PASS.

- [ ] **Step 5: Tighten the benchmark's dispatch-render gate from measurement**

**There is no `exactMissRatio` gate. Do not add one.** (Plan amended 2026-07-15, user-approved — see
§3.3 of the spec.) `missRatio`/`triggers` are racy (same baseline gave 0.667/0.179/0.090 across runs
because `audioLoop` races the test's `Update()` loop). They stay printed as evidence only.

The gate is on `DispatchRenders`. **Context (read this — the story changed after Task 8):** the original
design made dispatch NEVER render (a clean `== 0`). A user decision then reverted the "nothing cached at
all" tier from silence to a fall-through render — so `recipe.Render` on the dispatch goroutine is
reachable again, but ONLY for (a) an instrument's first-ever trigger and (b) a binding the LRU dropped
before its next trigger. Case (b) is exactly what THIS task's eviction pinning removes. So after your
pinning change, dispatch renders should drop to the cold-start floor and stay there regardless of edit
rate. The benchmark currently carries a deliberately loose placeholder ceiling (`DispatchRenders > 15`);
your job is to replace it with a real, measured one.

Measure first (the discipline that saved us from the racy missRatio gate — do not guess):
```bash
cd /home/ymolinar/Repos/beatmo/src/go && for i in 1 2 3 4 5; do xvfb-run -a ../../.tools/go/bin/go test -count=1 -run TestParamEditPlaybackCostTable ./internal/ui/ 2>&1 | grep -E "^(baseline|synth-knob-storm|sampler-edit-storm|fx-param-storm)"; echo "--- run $i ---"; done
```
Record the highest `dispatchR` any scenario reaches across the 5 runs (pre-pinning it was 0-1). Then set
the ceiling in `param_edit_playback_bench_test.go` to `max(3, observed_max + 2)` — low enough that the
pre-fix storm (4-44, scaling with edits) would trip it, high enough to absorb cold-start noise. Replace
the placeholder:
```go
		// GATE: an edit storm must not SCALE dispatch renders with edit rate.
		// Pre-fix the storms produced 4-44 dispatch renders that grew with
		// editing; now edit-induced misses are absorbed by best-available and
		// eviction pinning keeps live bindings warm, leaving only cold-start.
		// Ceiling pinned from measurement (5 runs, this task), NOT guessed.
		const maxDispatchRenders = <observed_max + 2, floor 3>
		if m.DispatchRenders > maxDispatchRenders {
			t.Errorf("%s: %d dispatch renders (ceiling %d) — edits are leaking onto the "+
				"audio dispatch goroutine; best-available + eviction pinning should absorb them",
				sc.name, m.DispatchRenders, maxDispatchRenders)
		}
```
`<observed_max + 2, floor 3>` is not valid Go — substitute your measured integer. Then run the 5-run
loop again and confirm every run is under the new ceiling.

- [ ] **Step 6: Final verification + Checkpoint (DO NOT COMMIT)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/gofmt -l internal/audio/ internal/ui/
../../.tools/go/bin/go test -count=1 -tags test -modfile=go.test.mod -timeout 900s ./internal/audio/
../../.tools/go/bin/go test -count=1 -timeout 900s ./internal/audio/
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /tmp/main-check.wasm ./cmd
CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo-check ./cmd
```
Expected: gofmt silent, both `ok`, both builds succeed.

**Verification scope note:** the full `./internal/ui/` suite is NOT a gate for this plan — the branch
owner has unrelated in-flight work there that is independently red. Verify this plan's UI-side work with
targeted `-run` filters (`TestParamEditPlaybackCostTable`, `TestParamEditWarms...`) plus the
`./internal/audio/` suites, which this plan owns end to end.

Then confirm end-to-end by ear/log on the real app:
```bash
cd /home/ymolinar/Repos/beatmo && make run-debug
```
Play the demo, open the Synth tab on Organ Church, drag a knob during playback. Expected: no crack; the `[bench] RenderLatency` line reports `dispatchRenders=0`.

---

## Self-Review Notes

**Spec coverage:**
- §2.2 I1/I2/I4 → Task 4. I3 → Task 8 (+ gate in Task 3).
- §3.1 render origin → Task 1. §3.2 benchmark → Task 3. §3.3 gates → Task 3 (DispatchRenders == 0, the SOLE gate; exactMissRatio measured in Phase 1 and found NOT gateable — demoted to reported evidence, user-approved 2026-07-15). §3.4 bench_json → Task 2.
- §4.1 non-blocking + fallback → Task 8. §4.2 bind-ahead prewarm → Tasks 6+7. §4.3 eviction pinning → Task 9. §4.4 sampler invalidate → Task 5.
- §5.3 sampler storm regression → Task 5 Step 1.
- §3.2 `fx-param-storm` scenario → Task 3, present as a **control**: FX params are live-per-block and not part of the voice binding, so it must track `baseline`. §3.3's "FX discontinuity magnitude" is deliberately NOT asserted — §6 remediation is a separate spec, and a gate with no remediation behind it is just a broken build.
- §6 (FX remediation: 14 unsmoothed effects, `SetChannelEQ` state-zeroing, `param_smooth.go` `test||js` parity gap, the `SetParam`/`ProcessBlockBuf` cross-mutex race) → intentionally out of scope for this plan.

**Type consistency check:** `renderOrigin`/`originDispatch`/`originPrewarm` (Task 1) used in Tasks 6, 8. `PrewarmVoiceParams(id string, pitch float64, params RecipeParams)` (Task 6) used in Tasks 7, 8. `warmParamsFn`/`warmBinding` (Task 7) consistent. `dispatchMustNotRender`/`asyncRenderSubmit` + their `Set*` wrappers (Task 8) consistent across audio + stub + ui. `pitchedVoiceKeyForParams` (Task 6) used by `renderPitchedIntoCacheForParams` (Task 6). `churnableParam` defined once in Task 4's file and reused by Tasks 6/8 tests (same package).

**Risk carried from the spec:** Task 5 Step 3 explicitly verifies the WAV/user-sample path before removing the invalidate, with a narrowed fallback in Step 5 — no assumption.
