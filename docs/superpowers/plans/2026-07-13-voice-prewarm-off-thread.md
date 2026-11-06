# Off-thread Voice Pre-warming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the `overdue/absmax` timing-parity panic by pre-rendering melody voices into `globalVoiceCache` off the `audioLoop` dispatch goroutine, so dispatch never blocks on a cold synth render.

**Architecture:** A new `audio.PrewarmVoice(id, pitch)` seam renders a voice into the existing thread-safe `globalVoiceCache` through the exact functions dispatch uses (so the warm key is byte-identical to the read key). Two producers in `internal/ui` feed a background `async.Pool` named `audio.prewarm`: (1) a graph-change enumerator that walks `beatInfosByRow` + `evalNodeParamsOnly` for each row's node pitches, and (2) a schedule-time backstop in `scheduleSound` for group-transposed/dynamic pitches. The timing gate is left untouched.

**Tech Stack:** Go 1.23, CGo C DSP renderer, `internal/async` pool/registry, existing `voice_cache.go` + `synth_recipe_dispatch.go`.

## Global Constraints

- Bundled Go only: `../../.tools/go/bin/go` (from `src/go`). Never system `go`.
- Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
- Native render tests (real C renderer, no `-tags test`): `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... -run <name>`
- Do **not** stage or commit (user preference). Each task's "Commit" step is replaced by "leave changes uncommitted; run the stated verification and report."
- Build-tag triad for any audio file with a native body: `//go:build !test && !js` (native), a `//go:build test` or default stub, and `//go:build js` variant — mirror `voice_warm_desktop.go` / `voice_warm_stub.go` / `voice_warm_js.go`.
- The warm path MUST call the same functions dispatch uses (`newRecipeAwareVoicePitched` for pitch-aware recipes, `newRecipeAwareVoice` otherwise) so cache keys match. This is the correctness linchpin.
- No change to `parityTimingCheck`, Stage C bounds, or fatal policy.
- `internal/audio` and `internal/ui` use `goleak.VerifyTestMain`; any transient named pool a test allocates must `t.Cleanup(func(){ async.DefaultRegistry().Release(name) })`.

---

## File Structure

- **Create** `internal/audio/voice_prewarm_desktop.go` — `PrewarmVoice` native impl.
- **Create** `internal/audio/voice_prewarm_stub.go` — `-tags test` no-op.
- **Create** `internal/audio/voice_prewarm_js.go` — js no-op.
- **Create** `internal/audio/voice_prewarm_desktop_test.go` — native key-match test.
- **Create** `internal/ui/voice_prewarmer.go` — `voicePrewarmer` type, pool wiring, `seen` dedup, `warmFromBeatInfos`, `warmPitch`, `invalidate`.
- **Create** `internal/ui/voice_prewarmer_test.go` — enumeration/dedup/backstop tests (fast path).
- **Modify** `internal/ui/game_struct.go` — add `voicePrewarmer *voicePrewarmer` field.
- **Modify** `internal/ui/game_new.go` — construct pool + prewarmer.
- **Modify** `internal/ui/game_close.go` — release pool on Close.
- **Modify** `internal/ui/game_graph_update_beat_infos.go` — call `warmFromBeatInfos()` at end of `(*playbackSlate).updateBeatInfos`.
- **Modify** `internal/ui/game_audio_schedule.go` — schedule-time backstop in `scheduleSound`.
- **Modify** `internal/ui/game_bpm_loop.go` — `invalidate()` on applied BPM change.
- **Create** `internal/ui/voice_prewarm_regression_test.go` — functional coverage test (fast path).

---

## Task 1: `audio.PrewarmVoice` seam + key-match proof

**Files:**
- Create: `internal/audio/voice_prewarm_desktop.go`
- Create: `internal/audio/voice_prewarm_stub.go`
- Create: `internal/audio/voice_prewarm_js.go`
- Test: `internal/audio/voice_prewarm_desktop_test.go`

**Interfaces:**
- Produces: `func PrewarmVoice(id string, pitch float64)` — renders `(id, pitch)` into `globalVoiceCache` via the same path dispatch uses. No-op under `-tags test` and `//go:build js`.

- [ ] **Step 1: Write the failing native test**

`internal/audio/voice_prewarm_desktop_test.go`:

```go
//go:build !test && !js

package audio

import "testing"

// TestPrewarmVoiceKeyMatchesDispatch proves the linchpin: warming (id, pitch)
// populates the exact cache slot the dispatch read (newRecipeAwareVoicePitched)
// later hits — so a pre-warmed melody note is a cache HIT at dispatch, never a
// cold inline render on the audioLoop goroutine.
func TestPrewarmVoiceKeyMatchesDispatch(t *testing.T) {
	const (
		id = "organ-church" // melodic, pitch-aware (the instrument that crashed at pitch -2)
		sr = 44100
	)
	if !pitchAwareRecipe(RecipeForInstrument(id)) {
		t.Fatalf("precondition: %q must be a pitch-aware recipe", id)
	}
	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Warm the exact pitch the melody will play.
	PrewarmVoice(id, -2)
	m := GetRenderLatencyMetrics()
	if m.CacheMisses < 1 {
		t.Fatalf("PrewarmVoice should render once (cache miss); got %+v", m)
	}
	missesAfterWarm := m.CacheMisses

	// Dispatch reads the SAME (id, pitch) via the function playParamsIdentity
	// calls. It must be a cache HIT (warm key == dispatch key), no re-render.
	if v := newRecipeAwareVoicePitched(id, bpm, sr, -2); v == nil {
		t.Fatal("dispatch render returned nil")
	}
	m2 := GetRenderLatencyMetrics()
	if m2.CacheHits < 1 {
		t.Fatalf("dispatch of a pre-warmed pitch must be a cache hit; got %+v", m2)
	}
	if m2.CacheMisses != missesAfterWarm {
		t.Fatalf("dispatch must not re-render a warmed pitch: misses %d -> %d", missesAfterWarm, m2.CacheMisses)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run TestPrewarmVoiceKeyMatchesDispatch -v`
Expected: FAIL — `undefined: PrewarmVoice`.

- [ ] **Step 3: Write the native implementation**

`internal/audio/voice_prewarm_desktop.go`:

```go
//go:build !test && !js

package audio

// PrewarmVoice renders (id, pitch) into globalVoiceCache off the caller's hot
// path. It calls the SAME functions the dispatch path uses
// (newRecipeAwareVoicePitched for pitch-aware recipes, newRecipeAwareVoice
// otherwise), so the produced cache key is byte-identical to the one the
// sequencer's PlayBatch -> playParamsIdentity later reads. The returned voice is
// discarded — the render + globalVoiceCache.Put is the whole point.
//
// A cache hit is a cheap no-op, so re-warming an already-warm pitch is safe.
// Reads the bpm/sampleRate globals exactly like the audioLoop dispatch already
// does; the cache is RWMutex-guarded, so concurrent warm + dispatch is safe.
func PrewarmVoice(id string, pitch float64) {
	if id == "" {
		return
	}
	if pitchAwareRecipe(RecipeForInstrument(id)) {
		newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch)
		return
	}
	newRecipeAwareVoice(id, bpm, SampleRate())
}
```

`internal/audio/voice_prewarm_stub.go`:

```go
//go:build test

package audio

// PrewarmVoice is a no-op under the fast (-tags test) build: the C renderer is
// stubbed, and UI tests observe the prewarm decision via voicePrewarmer's
// injectable warmFn seam rather than through a real render.
func PrewarmVoice(id string, pitch float64) {}
```

`internal/audio/voice_prewarm_js.go`:

```go
//go:build js

package audio

// PrewarmVoice is a no-op on browser: the WASM build already renders voice
// warms off-thread on its render worker (see voice_warm_js.go). Extending
// per-note browser coverage is out of scope for the desktop overdue fix.
func PrewarmVoice(id string, pitch float64) {}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run TestPrewarmVoiceKeyMatchesDispatch -v`
Expected: PASS.

- [ ] **Step 5: Verify all build tags compile**

Run:
```bash
cd src/go && ../../.tools/go/bin/go build ./internal/audio/ \
  && ../../.tools/go/bin/go vet -tags test ./internal/audio/ \
  && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/audio/
```
Expected: no output (all three build cleanly).

- [ ] **Step 6: Leave uncommitted; report**

Report the passing native test and the three-tag build result. Do not `git add`/`commit`.

---

## Task 2: `voicePrewarmer` type + pool wiring + Close release

**Files:**
- Create: `internal/ui/voice_prewarmer.go`
- Modify: `internal/ui/game_struct.go` (add field)
- Modify: `internal/ui/game_new.go:216-221` region (construct)
- Modify: `internal/ui/game_close.go:23` region (release)
- Test: `internal/ui/voice_prewarmer_test.go`

**Interfaces:**
- Consumes: `audio.PrewarmVoice(id string, pitch float64)` (Task 1).
- Produces:
  - `type voicePrewarmer struct { … }`
  - `func newVoicePrewarmer(g *Game) *voicePrewarmer`
  - `func (p *voicePrewarmer) warmPitch(id string, pitch float64)` — dedup + submit one warm.
  - `func (p *voicePrewarmer) invalidate()` — clear the `seen` dedup set.
  - `func (p *voicePrewarmer) release()` — release the pool from the registry.
  - `func (p *voicePrewarmer) warmFromBeatInfos()` (added in Task 3).
  - `Game.voicePrewarmer *voicePrewarmer` field.

- [ ] **Step 1: Write the failing test**

`internal/ui/voice_prewarmer_test.go`:

```go
package ui

import (
	"sync"
	"testing"
)

// TestVoicePrewarmerDedup verifies warmPitch submits a given (id, roundedPitch)
// exactly once until invalidate() clears the seen set.
func TestVoicePrewarmerDedup(t *testing.T) {
	var mu sync.Mutex
	var calls [][2]any
	p := &voicePrewarmer{
		seen: make(map[warmKey]struct{}),
		warmFn: func(id string, pitch float64) {
			mu.Lock()
			calls = append(calls, [2]any{id, pitch})
			mu.Unlock()
		},
	}

	p.warmPitch("organ-church", -2.0)
	p.warmPitch("organ-church", -2.0)  // dup -> dropped
	p.warmPitch("organ-church", -1.9)  // rounds to -2.0 (0.5-step) -> dup -> dropped
	p.warmPitch("organ-church", 0.0)   // distinct

	mu.Lock()
	got := len(calls)
	mu.Unlock()
	if got != 2 {
		t.Fatalf("expected 2 distinct warms (-2.0, 0.0); got %d: %v", got, calls)
	}

	p.invalidate()
	p.warmPitch("organ-church", -2.0) // seen cleared -> submits again
	mu.Lock()
	got = len(calls)
	mu.Unlock()
	if got != 3 {
		t.Fatalf("expected re-submit after invalidate; got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestVoicePrewarmerDedup -v`
Expected: FAIL — `undefined: voicePrewarmer` / `warmKey`.

- [ ] **Step 3: Write `voice_prewarmer.go`**

`internal/ui/voice_prewarmer.go`:

```go
package ui

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// voicePrewarmer renders upcoming melody voices into audio.globalVoiceCache off
// the audioLoop dispatch goroutine, so dispatch never pays a cold synth render
// (the ~64ms church-organ render that made a beat land overdue and panicked the
// timing gate — see docs/superpowers/specs/2026-07-13-voice-prewarm-off-thread-design.md).
//
// Two producers call warmPitch: warmFromBeatInfos (graph-change enumeration of
// each row's node pitches) and the scheduleSound backstop (the exact post-effect
// pitch, covering group-transposed/dynamic pitches). Enumeration runs only on
// the UI goroutine; warmPitch's seen-map + pool.Submit are the only cross-
// goroutine touchpoints and are mutex/pool-safe.
type voicePrewarmer struct {
	pool     *async.Pool
	poolName string
	private  bool // pool is a private fallback (not registry-owned)

	mu   sync.Mutex
	seen map[warmKey]struct{}

	// warmFn performs one warm. Production submits audio.PrewarmVoice to the
	// pool; tests inject a recorder to observe decisions without rendering.
	warmFn func(id string, pitch float64)

	g *Game
}

// warmKey dedups a warm request. pitch is rounded to 0.5 st to match
// audio.roundPitchForCache — the cache key's pitch resolution — so we never
// submit two renders that would collapse to the same cache slot.
type warmKey struct {
	id    string
	pitch float64
}

// roundWarmPitch mirrors audio.roundPitchForCache (unexported there): round to
// the nearest 0.5 semitone.
func roundWarmPitch(p float64) float64 { return math.Round(p*2) / 2 }

const prewarmPoolName = "audio.prewarm"

func newVoicePrewarmer(g *Game) *voicePrewarmer {
	p := &voicePrewarmer{
		seen:     make(map[warmKey]struct{}),
		poolName: prewarmPoolName,
		g:        g,
	}
	// 2 workers so a burst of distinct pitches warms in parallel without
	// starving the single audio.timers worker; queue 64 mirrors audioCh depth.
	opts := async.Options{MaxConcurrent: 2, QueueSize: 64, Name: prewarmPoolName}
	pool, err := async.DefaultRegistry().Get(prewarmPoolName, opts)
	if err != nil {
		pool = async.NewPool(nil, opts)
		p.private = true
	}
	p.pool = pool
	p.warmFn = func(id string, pitch float64) {
		// Non-blocking: on backpressure the warm is dropped and dispatch
		// degrades to today's inline render — never worse, never blocks.
		_ = p.pool.Submit(func() { audio.PrewarmVoice(id, pitch) })
	}
	return p
}

// warmPitch submits one warm for (id, pitch), deduped against the seen set.
func (p *voicePrewarmer) warmPitch(id string, pitch float64) {
	if id == "" || p.warmFn == nil {
		return
	}
	k := warmKey{id: id, pitch: roundWarmPitch(pitch)}
	p.mu.Lock()
	if _, ok := p.seen[k]; ok {
		p.mu.Unlock()
		return
	}
	p.seen[k] = struct{}{}
	fn := p.warmFn
	p.mu.Unlock()
	fn(id, k.pitch)
}

// invalidate clears the dedup set so the next enumeration/backstop re-warms
// (used on BPM change and instrument change, which move the cache key).
func (p *voicePrewarmer) invalidate() {
	p.mu.Lock()
	p.seen = make(map[warmKey]struct{})
	p.mu.Unlock()
}

// release frees the registry budget for the prewarm pool (no-op for a private
// fallback pool, which is GC'd with the Game).
func (p *voicePrewarmer) release() {
	if p == nil {
		return
	}
	if p.private {
		p.pool.Close()
		return
	}
	async.DefaultRegistry().Release(p.poolName)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestVoicePrewarmerDedup -v`
Expected: PASS.

- [ ] **Step 5: Wire into Game struct + constructor + Close**

In `internal/ui/game_struct.go`, add to the `Game` struct (near other background-pool fields):

```go
	// voicePrewarmer renders upcoming melody voices off the audioLoop
	// dispatch goroutine so dispatch never cold-renders (fixes overdue panic).
	voicePrewarmer *voicePrewarmer
```

In `internal/ui/game_new.go`, immediately after the `g.audioScheduler = async.NewScheduler(timerPool)` line (game_new.go:221):

```go
	g.voicePrewarmer = newVoicePrewarmer(g)
```

In `internal/ui/game_close.go`, inside `func (g *Game) Close()` (after existing teardown, before it returns):

```go
	if g.voicePrewarmer != nil {
		g.voicePrewarmer.release()
	}
```

- [ ] **Step 6: Verify build + no leak**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestVoicePrewarmer|TestGameNew|Close' -v
```
Expected: PASS, no goleak failure.

- [ ] **Step 7: Leave uncommitted; report.**

---

## Task 3: Graph-change enumeration (`warmFromBeatInfos`) + hook

**Files:**
- Modify: `internal/ui/voice_prewarmer.go` (add `warmFromBeatInfos`)
- Modify: `internal/ui/game_graph_update_beat_infos.go` (call at end of `(*playbackSlate).updateBeatInfos`)
- Test: `internal/ui/voice_prewarmer_test.go` (add enumeration test)

**Interfaces:**
- Consumes: `g.beatInfosByRow [][]model.BeatInfo`, `g.drum.Rows[row].Instrument string`, `g.evalNodeParamsOnly(row, idx, info) (vol, pitch, dur float64)`.
- Produces: `func (p *voicePrewarmer) warmFromBeatInfos()`.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/voice_prewarmer_test.go`:

```go
// TestWarmFromBeatInfosEnumeratesNodePitches drives the real enumeration over a
// Game whose row plays a node at pitch -2 (the crashing organ pitch) and asserts
// the prewarmer warmed exactly that (instrument, pitch).
func TestWarmFromBeatInfosEnumeratesNodePitches(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, "organ-church", -2)
	defer g.CloseForTest()

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id string, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	g.updateBeatInfos() // builds beatInfosByRow, then warmFromBeatInfos()

	mu.Lock()
	_, ok := warmed[warmKey{id: "organ-church", pitch: -2.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("expected warm for (organ-church, -2.0); got %v", warmed)
	}
}
```

Note for implementer: `newTestGameWithOrganAtPitch` is a test helper you write in this file. Build a `Game` via the existing test constructor used by other `internal/ui` tests (grep `newTestGame`/`buildTestGame` in `internal/ui/*_test.go` for the canonical helper and mirror it), set one row's `Instrument = id`, add one regular node on that row's path with `Params.Pitch = float64(pitch)`, and call `g.updateBeatInfos()` once to populate paths. If an existing helper already builds a single-row circuit with a node, extend it rather than duplicating.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestWarmFromBeatInfosEnumeratesNodePitches -v`
Expected: FAIL — `warmFromBeatInfos` not called / not defined.

- [ ] **Step 3: Implement `warmFromBeatInfos`**

Add to `internal/ui/voice_prewarmer.go`:

```go
// warmFromBeatInfos enumerates each row's firing pitches from the current
// beatInfosByRow window and warms the distinct (rowInstrument, pitch) pairs.
// evalNodeParamsOnly is pure param math (node pitch + windowed group transpose,
// NO render), so this is cheap; the render is what warmPitch offloads. Runs on
// the UI goroutine (updateBeatInfos caller). Safe to call every graph change:
// warmPitch dedups against seen.
func (p *voicePrewarmer) warmFromBeatInfos() {
	g := p.g
	if g == nil || g.drum == nil {
		return
	}
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
				continue // mute/other node types don't schedule a voice
			}
			_, pitch, _ := g.evalNodeParamsOnly(row, idx, info)
			p.warmPitch(inst, pitch)
		}
	}
}
```

Add `"github.com/ingyamilmolinar/beatmo/core/model"` to the import block if not already present.

- [ ] **Step 4: Hook into `updateBeatInfos`**

In `internal/ui/game_graph_update_beat_infos.go`, at the very end of `func (s *playbackSlate) updateBeatInfos()` (after the `if !g.Playing() { … refreshDrumRow … }` block, line ~575), add:

```go
	// Warm upcoming melody voices off the audioLoop dispatch goroutine so the
	// sequencer's PlayBatch hits a warm cache instead of cold-rendering inline
	// (the overdue-panic root cause). Enumeration is cheap; render is offloaded.
	if g.voicePrewarmer != nil {
		g.voicePrewarmer.warmFromBeatInfos()
	}
```

(`s.g` is `g` within that method — match the local name already used in the function; it binds `g := s.g` at the top.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWarmFromBeatInfos|TestVoicePrewarmerDedup' -v`
Expected: PASS.

- [ ] **Step 6: Full ui + audio fast suite (no regressions)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/`
Expected: PASS (allowing only the pre-existing unrelated failures recorded in memory: TestFocusGraph_RespondsToEveryKnob, TestInstrumentCategoriesScrollMovesWindow, TestRowControls_NoCramping_AllProfiles/mobile, TestMobileRowNamesDoNotTruncate — verify each failure, if any, is one of these and predates this change).

- [ ] **Step 7: Leave uncommitted; report.**

---

## Task 4: Schedule-time backstop

**Files:**
- Modify: `internal/ui/game_audio_schedule.go` (`(*playbackSlate).scheduleSound`)
- Test: `internal/ui/voice_prewarmer_test.go` (add backstop test)

**Interfaces:**
- Consumes: `voicePrewarmer.warmPitch` (Task 2), the `inst`/`pitch` params already passed to `scheduleSound`.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/voice_prewarmer_test.go`:

```go
// TestScheduleSoundBackstopWarmsExactPitch verifies the schedule-time backstop
// warms the exact post-effect pitch handed to scheduleSound — the coverage for
// group-transposed/dynamic pitches beyond the enumerated window.
func TestScheduleSoundBackstopWarmsExactPitch(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, "organ-church", 0)
	defer g.CloseForTest()

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id string, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	info := model.BeatInfo{NodeType: model.NodeTypeRegular}
	// A pitch the enumeration would not have produced (simulated group transpose).
	g.scheduleSound(0, 0, info, "organ-church", 0.8, 7.0, 1.0, math.NaN(), true)

	mu.Lock()
	_, ok := warmed[warmKey{id: "organ-church", pitch: 7.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("backstop should warm (organ-church, 7.0); got %v", warmed)
	}
}
```

Add `"math"` and `"github.com/ingyamilmolinar/beatmo/core/model"` imports to the test file if not present.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestScheduleSoundBackstopWarmsExactPitch -v`
Expected: FAIL — backstop not wired.

- [ ] **Step 3: Add the backstop to `scheduleSound`**

In `internal/ui/game_audio_schedule.go`, inside `func (s *playbackSlate) scheduleSound(...)`, at the start of the function body after `g := s.g` and after the `NodeTypeMute` early-return block (so mute nodes, which return early, don't warm), add — placing it near the existing `if sequencer { … openTimingTrace … }` region so it only fires for sequencer beats:

```go
	// Backstop the graph-change enumeration: warm the exact post-effect pitch
	// (includes group transpose / logic decisions) off the dispatch goroutine.
	// Only for real sequencer beats — fire-and-forget UI sounds (sequencer=false)
	// play immediately and don't need pre-warming.
	if sequencer && g.voicePrewarmer != nil {
		g.voicePrewarmer.warmPitch(inst, pitch)
	}
```

Place this block after the `if info.NodeType == model.NodeTypeMute { … return }` block and before or alongside the `if sequencer {` parity block — it must be reached for regular sequencer beats. Do NOT place it before the mute return.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestScheduleSoundBackstopWarmsExactPitch -v`
Expected: PASS.

- [ ] **Step 5: Leave uncommitted; report.**

---

## Task 5: BPM-change invalidate + functional regression proof

**Files:**
- Modify: `internal/ui/game_bpm_loop.go`
- Create: `internal/ui/voice_prewarm_regression_test.go`

**Interfaces:**
- Consumes: `voicePrewarmer.invalidate()` (Task 2), `voicePrewarmer.warmFromBeatInfos` (Task 3).

- [ ] **Step 1: Write the failing functional regression test**

`internal/ui/voice_prewarm_regression_test.go`:

```go
package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestOverduePanicFixed_OrganAtMinus2WarmedBeforeDispatch is the functional
// proof for the overdue/absmax crash: a circuit whose organ row fires a node at
// pitch -2 (the exact crash pitch) must have that voice WARMED by the time the
// sequencer would dispatch it — so dispatch is a cache hit and no beat lands
// overdue. Drives the real graph->updateBeatInfos->enumeration path.
func TestOverduePanicFixed_OrganAtMinus2WarmedBeforeDispatch(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, "organ-church", -2)
	defer g.CloseForTest()

	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id string, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}

	g.updateBeatInfos()

	mu.Lock()
	_, ok := warmed[warmKey{id: "organ-church", pitch: -2.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("crash pitch -2 was not pre-warmed before dispatch; got %v", warmed)
	}
}

// TestBPMChangeReWarms verifies a BPM change clears the dedup set so voices are
// re-warmed at the new BPM-derived cache key.
func TestBPMChangeReWarms(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, "organ-church", -2)
	defer g.CloseForTest()

	g.voicePrewarmer.warmPitch("organ-church", -2) // populate seen
	g.voicePrewarmer.invalidate()

	var got bool
	g.voicePrewarmer.warmFn = func(id string, pitch float64) {
		if id == "organ-church" && pitch == -2.0 {
			got = true
		}
	}
	g.voicePrewarmer.warmPitch("organ-church", -2) // seen cleared -> submits
	if !got {
		t.Fatal("expected re-warm after invalidate (BPM-change path)")
	}
	_ = model.NodeTypeRegular // keep model import used if helper is elsewhere
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestOverduePanicFixed|TestBPMChangeReWarms' -v`
Expected: `TestBPMChangeReWarms` PASSES already (invalidate exists from Task 2); `TestOverduePanicFixed…` PASSES if Task 3 landed. If both already pass, this task's code change (BPM hook) is still required for correctness — proceed to Step 3. (These tests lock behavior; the BPM hook below prevents a future regression where a live BPM change leaves stale-BPM cache keys unwarmed.)

- [ ] **Step 3: Add the BPM-change invalidate hook**

In `internal/ui/game_bpm_loop.go`, immediately after each `audio.SetBPM(b)` call (there are two: the apply at ~line 33 and the reconcile at ~line 53), add:

```go
				if g.voicePrewarmer != nil {
					g.voicePrewarmer.invalidate()
				}
```

Match the indentation of the surrounding block at each site. This clears the dedup set so the next `updateBeatInfos`/backstop re-warms at the new BPM-derived key. (Enumeration itself is not called from this background goroutine — only the mutex-guarded `invalidate` — so no cross-goroutine enumeration race is introduced.)

- [ ] **Step 4: Run tests to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestOverduePanicFixed|TestBPMChangeReWarms|TestVoicePrewarmer|TestWarmFromBeatInfos|TestScheduleSoundBackstop' -v`
Expected: all PASS.

- [ ] **Step 5: Full fast suite + native audio suite**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/ \
  && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run 'Prewarm|RenderLatency'
```
Expected: PASS (fast suite modulo the known pre-existing failures listed in Task 3 Step 6; native audio Prewarm/RenderLatency green).

- [ ] **Step 6: Manual verification (the real crash)**

Run the debug build that originally panicked and play the startup demo for >60 s:
```bash
make run-debug
```
Expected: no `audio timing parity breach: overdue/absmax` panic. Observe DEBUG logs — the `organ-church` at pitch -2 should no longer coincide with a 60 ms+ `bridge.max` spike, and `overdueCount` stays 0. (If a display isn't available in your environment, note this step was not runnable and rely on the functional + native cache-hit tests as the proof.)

- [ ] **Step 7: Leave uncommitted; report full results.**

---

## Self-Review notes (author)

- **Spec coverage:** PrewarmVoice seam (Task 1) ✓; pool `audio.prewarm` + Close release (Task 2) ✓; graph-change enumeration via `beatInfosByRow`+`evalNodeParamsOnly` (Task 3) ✓; schedule-time backstop (Task 4) ✓; BPM invalidate (Task 5) ✓; key-match linchpin test (Task 1) ✓; enumeration/backstop/dedup tests (Tasks 3/4/2) ✓; functional regression (Task 5) ✓; degradation-on-backpressure (Task 2 `warmFn` drop) ✓; concurrency note honored (enumeration UI-goroutine only; only `seen`+pool cross-goroutine) ✓.
- **Type consistency:** `warmKey{id,pitch}`, `warmPitch(id,pitch)`, `warmFromBeatInfos()`, `invalidate()`, `release()`, `newVoicePrewarmer(g)`, `PrewarmVoice(id,pitch)` used identically across tasks.
- **Known gap handed to implementer:** `newTestGameWithOrganAtPitch` helper must be written by mirroring the existing `internal/ui` test Game constructor (Task 3 Step 1 note). If `organ-church` is not registered/melodic in the test build, substitute the melodic instrument id used by `melodic_pitch_rerender_test.go` and update the three tests + the native key-match test consistently.
