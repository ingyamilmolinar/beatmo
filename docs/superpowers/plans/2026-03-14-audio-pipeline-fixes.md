# Audio Pipeline Scheduling Fixes Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix audio scheduling correctness issues: add drop observability, tighten adaptive lookahead, skip wasted computation, expose Go heap stats in WASM, and reduce JS-side minimum lead time.

**Architecture:** Five surgical changes to the audio scheduling pipeline. P1 adds a drop counter to `sendLatest`. P2 tightens the adaptive lookahead thresholds and lowers the max cap. P3 adds an early-return guard in `seqScheduleTime()` when the audio channel is near-full (avoids wasting CPU on events that will be dropped without permanently skipping beats). P4 exports the 4 existing Go heap stats fields through the WASM `perfStats()` JS bridge (the Go side already computes them). P5 reduces `AUDIO_MIN_LEAD_SEC` from 8ms to 4ms.

**Fallback strategy:** If browser benchmarks show overdue events after the lookahead changes, revert to 50ms base / 80ms cap as a middle ground before investigating further.

**Tech Stack:** Go 1.23 (WASM), JavaScript/WebAudio

**Key references:**
- `CLAUDE.md` — test commands, build commands, WASM path gotchas
- `PERF_REPORT.md` — profiling data that motivates each change
- Fast Go tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...`
- Browser test: `WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/<test>.browser.test.js`

---

## Chunk 1: P1 — sendLatest drop counter + P4 — perfStats heap fields

### Task 1: Add atomic drop counter to sendLatest and expose in PerfStats

**Files:**
- Modify: `src/go/internal/ui/game_channel_helpers.go`
- Modify: `src/go/internal/ui/perf.go` (PerfStats struct + perfCounters)
- Modify: `src/go/internal/ui/game_audio_schedule.go` (pass counter to sendLatest)
- Test: `src/go/internal/ui/game_channel_helpers_test.go` (new file)

- [ ] **Step 1: Write the failing test for sendLatest with drop counter**

Create `src/go/internal/ui/game_channel_helpers_test.go`:

```go
//go:build test

package ui

import (
	"sync/atomic"
	"testing"
)

func TestSendLatestCountsDrops(t *testing.T) {
	ch := make(chan int, 1)
	var drops atomic.Int64

	// Fill the channel
	ch <- 42

	// Send again — should drop the 42 and enqueue 99
	sendLatest(ch, 99, &drops)

	if drops.Load() != 1 {
		t.Fatalf("expected 1 drop, got %d", drops.Load())
	}
	got := <-ch
	if got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
}

func TestSendLatestNilDropsCounter(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 42

	// nil counter should not panic
	sendLatest(ch, 99, nil)

	got := <-ch
	if got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
}

func TestSendLatestNoDropWhenEmpty(t *testing.T) {
	ch := make(chan int, 1)
	var drops atomic.Int64

	sendLatest(ch, 7, &drops)

	if drops.Load() != 0 {
		t.Fatalf("expected 0 drops, got %d", drops.Load())
	}
	got := <-ch
	if got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestSendLatest" -v`
Expected: FAIL — `sendLatest` signature doesn't accept drops parameter.

- [ ] **Step 3: Update sendLatest signature to accept drop counter**

Modify `src/go/internal/ui/game_channel_helpers.go`:

Change `sendLatest` to:

```go
// sendLatest writes v to ch, dropping an existing item if the buffer is full.
// It never blocks and will silently discard v if the channel remains full.
// If drops is non-nil, it is incremented each time an item is dropped.
func sendLatest[T any](ch chan T, v T, drops *atomic.Int64) {
	select {
	case ch <- v:
		return
	default:
		if drops != nil {
			drops.Add(1)
		}
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}
```

Add `"sync/atomic"` to imports.

- [ ] **Step 4: Fix all call sites of sendLatest**

There are 4 callers. Update each:

In `src/go/internal/ui/game_audio_schedule.go` (lines 37 and 48), both `queueSoundParams` and `queueSoundAtParams` call `sendLatest(g.audioCh, req)`. Update both to:

```go
sendLatest(g.audioCh, req, &g.perf.aDrops)
```

In `src/go/internal/ui/game_bpm_loop.go` (lines 30 and 50), both calls `sendLatest(g.bpmAck, b)`. Update both to:

```go
sendLatest(g.bpmAck, b, nil)
```

- [ ] **Step 5: Add drop counter field to perfCounters and PerfStats**

In `src/go/internal/ui/perf.go`:

Add to `PerfStats` struct (after `AudioCallMax`):
```go
AudioDrops int64 `json:"audioDrops"` // audio events dropped by sendLatest
```

Add to `perfCounters` struct (after `aCallMaxNS`):
```go
// aDrops uses atomic.Int64 (not bare int64 like the other fields) because
// sendLatest increments it directly via pointer from the scheduling goroutine.
aDrops atomic.Int64
```

In `perfCounters.reset()`, add:
```go
p.aDrops.Store(0)
```

In `perfCounters.snapshot()`, add (after the `callMaxMS` line):
```go
s.AudioDrops = p.aDrops.Load()
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestSendLatest" -v`
Expected: PASS

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -count=1`
Expected: PASS (all existing tests still work)

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/ui/game_channel_helpers.go src/go/internal/ui/game_channel_helpers_test.go src/go/internal/ui/perf.go src/go/internal/ui/game_audio_schedule.go src/go/internal/ui/game_bpm_loop.go
git commit -m "feat: add atomic drop counter to sendLatest for audio event observability"
```

---

### Task 2: Expose Go heap stats and audio drops in WASM perfStats()

**Files:**
- Modify: `src/go/internal/ui/js_exports_playback_perf.go:218-251`
- Modify: `src/js/audio_profile_cdp.browser.test.js` (update log lines that read heap stats)

- [ ] **Step 1: Add missing heap + drop fields to perfStats JS export**

In `src/go/internal/ui/js_exports_playback_perf.go`, inside the `perfStats` FuncOf (after line 233 `obj.Set("audioCallMax", ...)`), add:

```go
obj.Set("audioDrops", int(s.AudioDrops))
obj.Set("heapAllocKB", int(s.HeapAllocKB))
obj.Set("heapSysKB", int(s.HeapSysKB))
obj.Set("heapObjects", int(s.HeapObjects))
obj.Set("goroutines", s.Goroutines)
```

- [ ] **Step 2: Build WASM and run the CDP profiler to verify**

Run:
```bash
make wasm
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/audio_profile_cdp.browser.test.js 2>&1 | grep -E "heap|Goroutines|audioDrops|heapAllocKB"
```

Expected: `heapAllocKB` now shows a real number instead of `undefined`.

- [ ] **Step 3: Run existing Go tests to verify no breakage**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/js_exports_playback_perf.go
git commit -m "feat: expose Go heap stats and audio drop counter in WASM perfStats()"
```

---

## Chunk 2: P2 — Tighten adaptive lookahead + P5 — Reduce JS AUDIO_MIN_LEAD_SEC

### Task 3: Tighten adaptive lookahead thresholds and lower max cap

**Files:**
- Modify: `src/go/internal/ui/game_audio_schedule.go:51-87`
- Test: `src/go/internal/ui/game_audio_schedule_test.go` (new file)

- [ ] **Step 1: Write test for runtimeAudioLookahead thresholds**

Create `src/go/internal/ui/game_audio_schedule_test.go`:

```go
//go:build test

package ui

import (
	"sync/atomic"
	"testing"
)

func TestRuntimeAudioLookaheadMaxCap(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0.04

	// Simulate high draw times to trigger max extra.
	// snapshot() computes drawAvgMS = drawSumNS / frames / 1e6.
	// To get drawAvgMS=25.0: frames=10, drawSumNS = 25 * 10 * 1e6 = 250_000_000
	g.perf.reset()
	atomic.StoreInt64(&g.perf.frames, 10)
	atomic.StoreInt64(&g.perf.drawSumNS, 250_000_000) // 25ms avg

	got := g.runtimeAudioLookahead()
	// With 40ms base + 30ms extra (DrawAvg > 18) = 70ms, but cap is 60ms
	if got > 0.061 {
		t.Errorf("expected max cap of 60ms, got %.3f", got)
	}
}

func TestRuntimeAudioLookaheadZeroRespected(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0

	got := g.runtimeAudioLookahead()
	if got != 0 {
		t.Errorf("expected 0 when base is 0, got %f", got)
	}
}

func TestRuntimeAudioLookaheadNotPlayingCap(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)
	g.audioLookaheadSec = 0.04

	// Not playing: should cap at 60ms (new cap)
	got := g.runtimeAudioLookahead()
	if got > 0.061 {
		t.Errorf("expected not-playing cap of 60ms, got %.3f", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (cap is currently 120ms)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestRuntimeAudioLookahead" -v`
Expected: FAIL — `TestRuntimeAudioLookaheadMaxCap` fails because current cap is 120ms.

- [ ] **Step 3: Tighten the adaptive lookahead thresholds**

In `src/go/internal/ui/game_audio_schedule.go`, replace `runtimeAudioLookahead()`:

```go
func (g *Game) runtimeAudioLookahead() float64 {
	look := g.audioLookaheadSec
	if look < 0 {
		look = 0
	}
	if !g.Playing() {
		if look > 0.06 {
			return 0.06
		}
		return look
	}
	// When audioLookaheadSec is explicitly 0, respect it without adding
	// performance-based extras. Tests use setAudioLookahead(0) to get
	// immediate highlight visibility in syncAndFindHighlights().
	if look == 0 {
		return 0
	}
	s := g.perf.snapshot()
	extra := 0.0
	if s.AudioQLatMax > 18 {
		extra = math.Max(extra, 0.03)
	} else if s.AudioQLatMax > 12 {
		extra = math.Max(extra, 0.02)
	}
	if s.DrawAvgMS > 18.0 || s.UpdateMaxMS > 25.0 {
		extra = math.Max(extra, 0.03)
	} else if s.DrawAvgMS > 14.0 || s.UpdateMaxMS > 18.0 {
		extra = math.Max(extra, 0.02)
	} else if s.DrawAvgMS > 11.0 || s.UpdateAvgMS > 8.0 {
		extra = math.Max(extra, 0.01)
	}
	look += extra
	if look > 0.06 {
		look = 0.06
	}
	return look
}
```

Changes from old:
- Max cap: 120ms → **60ms**
- Not-playing cap: 120ms → **60ms**
- Extra tiers: 60/40/20ms → **30/20/10ms**

Also update the WASM default base lookahead in `src/go/internal/ui/game_new.go` line 218:

```go
g.audioLookaheadSec = 0.04
```

Changed from 0.08 (80ms) → **0.04 (40ms)**. Combined with max 30ms dynamic extra, total max is 70ms → capped to 60ms. This halves the perceived latency compared to the previous 80ms + 60ms = 140ms → capped 120ms.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestRuntimeAudioLookahead" -v`
Expected: PASS

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -count=1`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_audio_schedule.go src/go/internal/ui/game_audio_schedule_test.go src/go/internal/ui/game_new.go
git commit -m "perf: halve adaptive audio lookahead (120ms→60ms cap, 80ms→40ms base)"
```

---

### Task 4: Reduce JS AUDIO_MIN_LEAD_SEC from 8ms to 4ms

**Files:**
- Modify: `src/js/audio.js:217`

- [ ] **Step 1: Change the constant**

In `src/js/audio.js`, line 217, change:

```js
const AUDIO_MIN_LEAD_SEC = 0.008;
```

to:

```js
const AUDIO_MIN_LEAD_SEC = 0.004;
```

- [ ] **Step 2: Build WASM and run the benchmark to verify no overdue events**

```bash
make wasm
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/bench_startup_demo.browser.test.js 2>&1 | grep -E "overdue|lagP90|lagP99|minLead"
```

Expected: 0 overdue events at all BPM levels. `minLead` should now show ~4ms instead of ~8ms.

- [ ] **Step 3: Run the audio timing stress test to verify**

```bash
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/audio_timing_stress.browser.test.js
```

Expected: All scenarios pass with 0 overdue events.

- [ ] **Step 4: Commit**

```bash
git add src/js/audio.js
git commit -m "perf: reduce AUDIO_MIN_LEAD_SEC from 8ms to 4ms for lower perceived latency"
```

---

## Chunk 3: P3 — Early-return from seqScheduleTime when audioCh near-full

### Task 5: Add channel capacity guard as early-return in seqScheduleTime

The guard must be an **early return** at the top of `seqScheduleTime()`, NOT a per-event skip inside the row loop. A per-event skip would advance `seqNextIdxs[row]` and permanently drop that beat (it would never be rescheduled). An early return skips the entire tick — the 4ms ticker will retry shortly, and no `seqNextIdxs` are advanced, so no beats are lost.

**Files:**
- Modify: `src/go/internal/ui/game_sequencer_schedule.go` (early return)
- Modify: `src/go/internal/ui/game_audio_schedule.go` (helper function)
- Test: `src/go/internal/ui/game_audio_schedule_test.go` (add test)

- [ ] **Step 1: Write a test for the audioChNearFull helper**

Add to existing `src/go/internal/ui/game_audio_schedule_test.go`:

```go
func TestAudioChNearFull(t *testing.T) {
	assertDefaultParityState(t)
	g := buildTestGame(t)

	// Fill audioCh to >75% capacity (128 * 0.75 = 96)
	for i := 0; i < 100; i++ {
		select {
		case g.audioCh <- soundReq{id: "fill"}:
		default:
		}
	}

	if !g.audioChNearFull() {
		t.Fatal("expected audioChNearFull() to return true")
	}

	// Drain channel
	for len(g.audioCh) > 0 {
		<-g.audioCh
	}

	if g.audioChNearFull() {
		t.Fatal("expected audioChNearFull() to return false after drain")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestAudioChNearFull" -v`
Expected: FAIL — `audioChNearFull` not defined.

- [ ] **Step 3: Add audioChNearFull helper**

Add to `src/go/internal/ui/game_audio_schedule.go` (after the `runtimeAudioLookahead` function):

```go
// audioChNearFull returns true if the audio channel is above 75% capacity.
// Used by seqScheduleTime to skip the entire tick when the audioLoop
// goroutine hasn't drained the channel yet. The tick retries in 4ms
// without advancing any seqNextIdxs, so no beats are lost.
func (g *Game) audioChNearFull() bool {
	return len(g.audioCh) >= cap(g.audioCh)*3/4
}
```

- [ ] **Step 4: Add early-return guard at top of seqScheduleTime**

In `src/go/internal/ui/game_sequencer_schedule.go`, inside `seqScheduleTime()`, add an early return **after the `g.seqMu.TryLock()` block and `parityEnabled` line, but before the `div := g.grid.MaxDiv()` line** (around line 29-32). Find:

```go
	parityEnabled := !runningUnderGoTest() || g.parityWatch != parityWatchOff
	if g.drum == nil {
```

Insert **between** these two lines:

```go
	// Skip entire tick when the audio channel is near-full. The audioLoop
	// goroutine hasn't drained pending events yet; computing more would
	// waste CPU since sendLatest will drop them. The 4ms ticker retries
	// shortly, and no seqNextIdxs are advanced, so no beats are lost.
	if g.audioChNearFull() {
		return
	}
```

- [ ] **Step 5: Run tests to verify everything passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run "TestAudioChNearFull" -v`
Expected: PASS

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -count=1`
Expected: PASS (all tests)

- [ ] **Step 6: Build WASM and run benchmark to verify no regressions**

```bash
make wasm
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/bench_startup_demo.browser.test.js
```

Expected: All BPM levels pass. Overdue count stays 0.

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/ui/game_audio_schedule.go src/go/internal/ui/game_audio_schedule_test.go src/go/internal/ui/game_sequencer_schedule.go
git commit -m "perf: early-return from seqScheduleTime when audioCh is near-full"
```

---

## Chunk 4: Final verification

### Task 6: Full pipeline verification

- [ ] **Step 1: Run all Go tests**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```

Expected: PASS

- [ ] **Step 2: Build WASM and run full benchmark suite**

```bash
make wasm
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/audio_profile_cdp.browser.test.js
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/bench_startup_demo.browser.test.js
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/audio_timing_stress.browser.test.js
WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node src/js/stress_complex.browser.test.js
```

Expected: All pass with 0 overdue events. `minLead` should be ~4ms. `heapAllocKB` shows real values. `audioDrops` is visible in perfStats.

- [ ] **Step 3: Verify latency improvements in CDP profiler**

Check the output of `audio_profile_cdp.browser.test.js`:
- `avgLead` should be ~40-60ms (down from ~80-130ms)
- `minLead` should be ~4ms (down from ~8ms)
- Total perceived latency = `avgLead` + `minLead` ≈ 44-64ms (down from ~88-138ms)

- [ ] **Step 4: Commit any remaining changes and update PERF_REPORT.md with new numbers**

Run the profiler, capture new numbers, and update `PERF_REPORT.md` with before/after comparison.
