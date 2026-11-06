# Audio business-logic coverage push — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lift `internal/audio` coverage from 85.4% → ≥ 92% by closing zero-coverage gaps on user-visible code paths (master LUFS, user-recipe Save/Save-As, trigger timestamps, channel EQ response, effect SetParam) and removing three small smells (`triggerLastEver` dead field, `lastSetEQ` legacy global, `abs64Diff` duplicate-of-`math.Abs`).

**Architecture:** Four phases of test-first additions plus opportunistic refactors. Each refactor ships in the same commit as the test that proves it. All work runs in the fast `-tags test` path (no xvfb, no WASM, no browser). Discipline: TDD red-green-commit per task; one commit per coherent unit.

**Tech Stack:** Go 1.23, `-tags test -modfile=go.test.mod`, the bundled `.tools/go/bin/go` toolchain. Test framework: standard `testing`; race detector via `-race`. Existing patterns to mirror: `internal/audio/recipe_overrides_test.go` for gate handling, `voiceCacheInvalidate` swap, and `withGate(t, on)` helper.

**Reference spec:** `docs/superpowers/specs/2026-05-22-audio-coverage-gaps-design.md`

---

## Conventions used throughout this plan

- Always run Go from the repo with the bundled toolchain via absolute paths to avoid the `cd src/go` working-directory drift seen during exploration:
  - **Test runner:** `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/...`
  - **Coverage runner:** `cd /home/ymolinar/Repos/beatmo && make coverage-go`
- All new test files start with the existing fast-path package header pattern (no build tags unless explicitly needed for `test || js` files).
- Each commit message follows the repo's existing `<type>(<scope>): <summary>` style; do not include any `Co-Authored-By` or `Generated with` lines.
- After every commit, run the targeted test directory once to confirm green:
  ```
  cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
  ```

---

## File Structure

**New files (created during this plan):**

| Path | Responsibility |
|---|---|
| `src/go/internal/audio/loudness_master_test.go` | Tests for the process-global master LUFS wrappers (`EnsureMasterLUFS`, `FeedMasterLUFS`, `MasterLUFSShortTerm`, `ResetLoudnessForTest`). 6 tests. |
| `src/go/internal/audio/recipe_overrides_actions_test.go` | Tests for `UpdateRecipeDefaultsAndInvalidate`, `RegisterUserRecipeFromBase`, `InstrumentParamsDiffer`, the gate setter, and the missing silent-skip branches of `ApplyUserRecipeOverrides`. 8 tests. |
| `src/go/internal/audio/trigger_timestamps_test.go` | Tests for `LastTriggerAt`, `SinceLastTrigger`, `ResetTriggerTimestamps`. 4 tests. |
| `src/go/internal/audio/eq_response_channel_test.go` | Tests for `GetChannelEQResponse`. 3 tests. |
| `src/go/internal/audio/insert_effects_passthrough_test.go` | Tests for `passThrough` fallback (4 method coverage in one test). |
| `src/go/internal/audio/fx_waveshaper_reset_test.go` | One test for `waveshaper.Reset` (build-tagged `test || js`). |
| `src/go/internal/audio/fx_setparam_table_test.go` | Catalog-driven `SetParam` coverage for every registered effect. 1 table-driven test. |

**Modified files:**

| Path | Change |
|---|---|
| `src/go/internal/audio/recipe_overrides.go` | Replace `abs64Diff` with `math.Abs`; swap `isFiniteParam` body to use `math.IsNaN` + `math.IsInf`. |
| `src/go/internal/audio/trigger_timestamps.go` | Delete the unread `triggerLastEver` field and its reset; delete its update inside `RecordVoiceTrigger`. |
| `src/go/internal/audio/eq_state.go` | Introduce public `ChannelEQ` type + `ChannelEQSnapshot(id)` accessor; delete `lastSetEQ` global and `LastSetEQ()` accessor; drop the legacy-comment block. |
| `src/go/internal/audio/stub.go` | Remove the `recordChannelEQ` call site dependency on `lastSetEQ` if any (none expected — to verify). |
| `src/go/internal/audio/insert_effects_helpers_test.go` | Migrate 3 sites from `LastSetEQ()` → `ChannelEQSnapshot("main")`. |
| `src/go/internal/ui/eq_export_import_test.go` | Migrate 1 site from `audio.LastSetEQ()` → `audio.ChannelEQSnapshot("main")`. |
| `src/go/internal/ui/drumview_eq_controls_test.go` | Migrate 2 sites from `audio.LastSetEQ()` → `audio.ChannelEQSnapshot("main")`. |

---

# Phase 1 — Master LUFS pipeline

## Task 1: Master LUFS test file

**Files:**
- Create: `src/go/internal/audio/loudness_master_test.go`

- [ ] **Step 1: Write the failing test file**

Write to `src/go/internal/audio/loudness_master_test.go`:

```go
package audio

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
)

// resetMasterLUFSForTest fully clears the process-global integrator,
// matching what ResetLoudnessForTest does but also nilling the
// integrator pointer so the "before any Ensure" path of
// MasterLUFSShortTerm is reachable. Tests use this for the
// "no-Ensure" path; otherwise they use ResetLoudnessForTest.
func resetMasterLUFSForTest(t *testing.T) {
	t.Helper()
	masterLUFSMu.Lock()
	masterLUFSCurrent = nil
	masterLUFSBits.Store(0)
	masterLUFSMu.Unlock()
}

// sineBlock returns `n` samples of a sine at freq Hz, sample rate sr,
// amplitude amp. Phase-continuous within a single call (each call
// restarts at zero phase — fine for steady-state tests).
func sineBlock(freq, sr, amp float64, n int) []float64 {
	buf := make([]float64, n)
	step := 2 * math.Pi * freq / sr
	for i := 0; i < n; i++ {
		buf[i] = amp * math.Sin(step*float64(i))
	}
	return buf
}

func TestEnsureMasterLUFS_IdempotentAtSameRate(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	// Feed enough to fill at least one block (4800 samples = 100ms at 48k).
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	first := MasterLUFSShortTerm()
	if first <= -120 {
		t.Fatalf("after one feed, MasterLUFSShortTerm = %.2f; want > -120", first)
	}
	// Second Ensure at the same sample rate must be a no-op — the
	// published value must persist.
	EnsureMasterLUFS(48000)
	second := MasterLUFSShortTerm()
	if math.Abs(second-first) > 0.001 {
		t.Errorf("idempotent Ensure changed published value: before=%.4f after=%.4f", first, second)
	}
}

func TestEnsureMasterLUFS_RateChangeResets(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	if MasterLUFSShortTerm() <= -120 {
		t.Fatalf("precondition: feed should have lifted LUFS above -120")
	}
	EnsureMasterLUFS(44100)
	// After rate change, the integrator is rebuilt and the atomic is
	// reset to bits-of(-120).
	got := MasterLUFSShortTerm()
	if got != -120 {
		t.Errorf("after rate change MasterLUFSShortTerm = %.2f; want -120", got)
	}
}

func TestMasterLUFSShortTerm_BeforeAnyEnsureReturnsFloor(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	// No Ensure has been called — masterLUFSBits is zero, hits the
	// short-circuit at line 60-62 of loudness.go.
	got := MasterLUFSShortTerm()
	if got != -120 {
		t.Errorf("MasterLUFSShortTerm before Ensure = %.2f; want -120", got)
	}
}

func TestFeedMasterLUFS_KnownTone(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	// 3 s of 997 Hz at -20 dBFS (amplitude = 0.1).
	const sr = 48000
	const dur = 3
	tone := sineBlock(997, sr, 0.1, sr*dur)
	FeedMasterLUFS(tone)

	lufs := MasterLUFSShortTerm()
	// Wide tolerance: K-weighting shelf around 1681 Hz adds 0-2 dB at
	// 997 Hz; analytic baseline for -20 dBFS sine ≈ -23 LUFS without
	// K-weighting; with K-weighting expect somewhere in [-24, -18].
	if lufs < -24 || lufs > -18 {
		t.Errorf("LUFS for 997 Hz @ -20 dBFS = %.2f; want in [-24, -18]", lufs)
	}
}

func TestFeedMasterLUFS_NoIntegratorIsNoOp(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	// No Ensure → integrator is nil. FeedMasterLUFS must early-return.
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	if got := MasterLUFSShortTerm(); got != -120 {
		t.Errorf("after feed without Ensure, MasterLUFSShortTerm = %.2f; want -120", got)
	}
}

func TestMasterLUFSShortTerm_CrossGoroutineRace(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)

	var reads atomic.Int64
	var stop atomic.Bool
	var sawAboveFloor atomic.Bool

	// Reader: poll MasterLUFSShortTerm; record observations.
	doneR := make(chan struct{})
	go func() {
		defer close(doneR)
		for !stop.Load() {
			v := MasterLUFSShortTerm()
			reads.Add(1)
			if v > -120 {
				sawAboveFloor.Store(true)
			}
		}
	}()

	// Writer: feed sine blocks for ~200 ms wall clock.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 4800))
	}
	stop.Store(true)
	<-doneR

	if reads.Load() == 0 {
		t.Fatalf("reader goroutine performed zero reads")
	}
	if !sawAboveFloor.Load() {
		t.Errorf("reader never observed LUFS > -120 across %d reads", reads.Load())
	}
}
```

- [ ] **Step 2: Run the test file**

Run:
```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run "TestEnsureMasterLUFS_IdempotentAtSameRate|TestEnsureMasterLUFS_RateChangeResets|TestMasterLUFSShortTerm_BeforeAnyEnsureReturnsFloor|TestFeedMasterLUFS_KnownTone|TestFeedMasterLUFS_NoIntegratorIsNoOp|TestMasterLUFSShortTerm_CrossGoroutineRace" -v
```
Expected: all six tests PASS. The functions `EnsureMasterLUFS` / `FeedMasterLUFS` / `MasterLUFSShortTerm` / `ResetLoudnessForTest` already exist in `loudness.go`; these tests only exercise them.

- [ ] **Step 3: Verify -race clean and no goroutine leak**

The race test spawns one goroutine, joins via channel. Confirm no failures across 5 runs:
```
cd /home/ymolinar/Repos/beatmo/src/go && for i in 1 2 3 4 5; do ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run TestMasterLUFSShortTerm_CrossGoroutineRace -count=1 || break; done
```
Expected: all 5 iterations PASS.

- [ ] **Step 4: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/loudness_master_test.go && git commit -m "test(audio): cover master LUFS process-global wrappers"
```

---

# Phase 2 — User recipe overrides

## Task 2: Recipe overrides actions test file

**Files:**
- Create: `src/go/internal/audio/recipe_overrides_actions_test.go`

The existing `recipe_overrides_test.go` already covers `ApplyUserRecipeOverrides` gate-off, gate-on, clamping, registration, unknown-base-skip, and nil-source paths. This file adds the missing zero-coverage entry points and the silent-skip branches.

- [ ] **Step 1: Write the failing test file**

Write to `src/go/internal/audio/recipe_overrides_actions_test.go`:

```go
package audio

import (
	"encoding/json"
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUseUserRecipeOverridesRoundTrip(t *testing.T) {
	prev := SwapUseUserRecipeOverridesForTest(true)
	t.Cleanup(func() { SwapUseUserRecipeOverridesForTest(prev) })
	if !UseUserRecipeOverrides() {
		t.Errorf("after Swap(true), UseUserRecipeOverrides() = false")
	}
	SetUseUserRecipeOverrides(false)
	if UseUserRecipeOverrides() {
		t.Errorf("after SetUseUserRecipeOverrides(false), UseUserRecipeOverrides() = true")
	}
	SetUseUserRecipeOverrides(true)
	if !UseUserRecipeOverrides() {
		t.Errorf("after SetUseUserRecipeOverrides(true), UseUserRecipeOverrides() = false")
	}
}

func TestApply_DocWithEmptyIDInheritsKey(t *testing.T) {
	const userID = "user.test.empty-id-key"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })
	withGate(t, true)

	doc := RecipeDoc{
		ID:         "", // intentionally empty; Apply should inherit the map key
		BaseRecipe: "drum-snare",
		Origin:     OriginUser,
	}
	raw, _ := json.Marshal(doc)
	src := &stubRecipeSource{recipes: map[string][]byte{userID: raw}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if NewRecipe(userID) == nil {
		t.Errorf("user recipe with empty ID should have inherited map key %q", userID)
	}
}

func TestApply_DocWithEmptyBaseRecipeSkipped(t *testing.T) {
	const userID = "user.test.empty-base"
	t.Cleanup(func() { UnregisterRecipeForTest(userID) })
	withGate(t, true)

	doc := RecipeDoc{
		ID:         userID,
		BaseRecipe: "", // intentionally empty; Apply must silently skip
		Origin:     OriginUser,
	}
	raw, _ := json.Marshal(doc)
	src := &stubRecipeSource{recipes: map[string][]byte{userID: raw}}
	if err := ApplyUserRecipeOverrides(src); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if NewRecipe(userID) != nil {
		t.Errorf("user recipe with empty BaseRecipe should not be registered")
	}
}

func TestUpdateRecipeDefaultsAndInvalidate_HappyPath(t *testing.T) {
	// drum-snare has a "decay" param via GenericSynthParamDefs.
	const recipeID = "drum-snare"
	originalDefault := -1.0
	for _, p := range RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			originalDefault = p.Default
		}
	}
	if originalDefault < 0 {
		t.Fatalf("precondition: %s has no decay param", recipeID)
	}
	t.Cleanup(func() {
		// Restore the original default through the same code path so
		// the cache invalidation runs once more (predictable cleanup).
		UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": originalDefault})
	})

	var invalidations atomic.Int64
	prevInvalidator := voiceCacheInvalidate
	voiceCacheInvalidate = func(instID string) { invalidations.Add(1) }
	t.Cleanup(func() { voiceCacheInvalidate = prevInvalidator })

	changed := UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": 1.7})
	if !changed {
		t.Errorf("changed = false; want true")
	}
	if got := RecipeDefaultParams(recipeID)["decay"]; got != 1.7 {
		t.Errorf("decay default = %v; want 1.7", got)
	}
	// builtinInstrumentRecipeBindings binds at least "snare" → drum-snare.
	if invalidations.Load() < 1 {
		t.Errorf("voiceCacheInvalidate fired %d times; want >= 1", invalidations.Load())
	}
}

func TestUpdateRecipeDefaultsAndInvalidate_NoChangeReturnsFalse(t *testing.T) {
	const recipeID = "drum-snare"
	currentDefault := -1.0
	for _, p := range RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			currentDefault = p.Default
		}
	}
	if currentDefault < 0 {
		t.Fatalf("precondition: %s has no decay param", recipeID)
	}
	var invalidations atomic.Int64
	prevInvalidator := voiceCacheInvalidate
	voiceCacheInvalidate = func(instID string) { invalidations.Add(1) }
	t.Cleanup(func() { voiceCacheInvalidate = prevInvalidator })

	// Set the default to its current value — no change should be reported.
	changed := UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": currentDefault})
	if changed {
		t.Errorf("changed = true; want false (override equals current default)")
	}
	if invalidations.Load() != 0 {
		t.Errorf("voiceCacheInvalidate fired %d times; want 0", invalidations.Load())
	}
}

func TestRegisterUserRecipeFromBase_HappyPath(t *testing.T) {
	const newID = "user.test.register-from-base.happy"
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	doc, err := RegisterUserRecipeFromBase(newID, "Happy Test", "drum-snare", map[string]float64{
		"decay": 1.5,
	})
	if err != nil {
		t.Fatalf("RegisterUserRecipeFromBase: %v", err)
	}
	if doc.ID != newID {
		t.Errorf("doc.ID = %q want %q", doc.ID, newID)
	}
	if doc.Origin != OriginUser {
		t.Errorf("doc.Origin = %v want OriginUser", doc.Origin)
	}
	if doc.BaseRecipe != "drum-snare" {
		t.Errorf("doc.BaseRecipe = %q want drum-snare", doc.BaseRecipe)
	}
	if len(doc.ParamDefs) != len(RecipeRegistrations()["drum-snare"].Params) {
		t.Errorf("ParamDefs length = %d want %d", len(doc.ParamDefs), len(RecipeRegistrations()["drum-snare"].Params))
	}
	if doc.ParamSeed["decay"] != 1.5 {
		t.Errorf("ParamSeed[decay] = %v want 1.5", doc.ParamSeed["decay"])
	}
	if r := NewRecipe(newID); r == nil {
		t.Errorf("NewRecipe(%q) = nil after registration", newID)
	}
}

func TestRegisterUserRecipeFromBase_UnknownBase(t *testing.T) {
	_, err := RegisterUserRecipeFromBase("user.test.bad-base", "Bad", "does-not-exist", nil)
	if err == nil {
		t.Fatalf("expected error for unknown base recipe")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q does not mention the unknown base id", err.Error())
	}
}

func TestRegisterUserRecipeFromBase_StripsNonFiniteSeed(t *testing.T) {
	const newID = "user.test.strip-nonfinite"
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	doc, err := RegisterUserRecipeFromBase(newID, "Strip", "drum-snare", map[string]float64{
		"decay": 1.5,
		"tone":  math.NaN(),
		"drive": math.Inf(1),
		"pitch": math.Inf(-1),
	})
	if err != nil {
		t.Fatalf("RegisterUserRecipeFromBase: %v", err)
	}
	if _, ok := doc.ParamSeed["decay"]; !ok {
		t.Errorf("ParamSeed lost finite key 'decay'")
	}
	for _, badKey := range []string{"tone", "drive", "pitch"} {
		if _, ok := doc.ParamSeed[badKey]; ok {
			t.Errorf("ParamSeed retained non-finite key %q", badKey)
		}
	}
}

func TestInstrumentParamsDiffer(t *testing.T) {
	const instID = "test.differ.inst"
	const recipeID = "drum-snare"

	cases := []struct {
		name    string
		setup   func()
		instArg string
		recArg  string
		want    bool
	}{
		{
			name:    "empty instID returns false",
			setup:   func() {},
			instArg: "",
			recArg:  recipeID,
			want:    false,
		},
		{
			name:    "empty recipeID returns false",
			setup:   func() {},
			instArg: instID,
			recArg:  "",
			want:    false,
		},
		{
			name:    "no overlay returns false",
			setup:   func() { ResetInstrumentParams(instID) },
			instArg: instID,
			recArg:  recipeID,
			want:    false,
		},
		{
			name: "overlay with key absent from defaults returns true",
			setup: func() {
				ResetInstrumentParams(instID)
				SetInstrumentParam(instID, "decay", 1.5)
			},
			instArg: instID,
			recArg:  recipeID,
			want:    true,
		},
	}
	t.Cleanup(func() { ResetInstrumentParams(instID) })
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.setup()
			got := InstrumentParamsDiffer(c.instArg, c.recArg)
			if got != c.want {
				t.Errorf("InstrumentParamsDiffer(%q, %q) = %v; want %v", c.instArg, c.recArg, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run all new tests**

Run:
```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run "TestUseUserRecipeOverridesRoundTrip|TestApply_DocWithEmptyIDInheritsKey|TestApply_DocWithEmptyBaseRecipeSkipped|TestUpdateRecipeDefaultsAndInvalidate_HappyPath|TestUpdateRecipeDefaultsAndInvalidate_NoChangeReturnsFalse|TestRegisterUserRecipeFromBase_HappyPath|TestRegisterUserRecipeFromBase_UnknownBase|TestRegisterUserRecipeFromBase_StripsNonFiniteSeed|TestInstrumentParamsDiffer" -v
```
Expected: all eight tests PASS. (`InstrumentParamsDiffer` has four sub-tests.)

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/recipe_overrides_actions_test.go && git commit -m "test(audio): cover recipe-override action APIs (Save/Save-As/Differ)"
```

## Task 3: Replace `abs64Diff` with `math.Abs`

**Files:**
- Modify: `src/go/internal/audio/recipe_overrides.go`

- [ ] **Step 1: Verify the only call site**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "abs64Diff" src/go/ 2>/dev/null
```
Expected: two hits — the definition at `recipe_overrides.go:318` and the call at `recipe_overrides.go:311`.

- [ ] **Step 2: Edit the call site and delete the helper**

In `src/go/internal/audio/recipe_overrides.go`:

Replace this block (around line 311):
```go
		if abs64Diff(v-ref) > 1e-6 {
			return true
		}
```
with:
```go
		if math.Abs(v-ref) > 1e-6 {
			return true
		}
```

Delete the helper at lines 318-323:
```go
func abs64Diff(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
```

Add `"math"` to the existing imports block at the top of `recipe_overrides.go`. The current imports are:
```go
import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)
```
Change to:
```go
import (
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
)
```

- [ ] **Step 3: Run all audio tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
```
Expected: PASS (including the `TestInstrumentParamsDiffer` case from Task 2 which exercises the comparator).

- [ ] **Step 4: Confirm helper is gone**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "abs64Diff" src/go/
```
Expected: no matches.

- [ ] **Step 5: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/recipe_overrides.go && git commit -m "refactor(audio): use math.Abs instead of abs64Diff helper"
```

## Task 4: Replace `isFiniteParam` body with stdlib NaN/Inf checks

**Files:**
- Modify: `src/go/internal/audio/recipe_overrides.go`

- [ ] **Step 1: Edit the helper body**

In `src/go/internal/audio/recipe_overrides.go`, locate the existing helper:
```go
func isFiniteParam(v float64) bool {
	if v != v {
		return false
	}
	if v > 1e308 || v < -1e308 {
		return false
	}
	return true
}
```
Replace its body with the stdlib equivalent (keeping the function name to preserve the documented boundary-sanitiser intent):
```go
func isFiniteParam(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
```

- [ ] **Step 2: Run audio tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
```
Expected: PASS — in particular `TestApplyClampsAndDropsBadValues` (existing) and `TestRegisterUserRecipeFromBase_StripsNonFiniteSeed` (Task 2) verify the sanitiser still rejects NaN/Inf.

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/recipe_overrides.go && git commit -m "refactor(audio): use math.IsNaN/IsInf in isFiniteParam"
```

---

# Phase 3 — Trigger timestamps + channel EQ + passThrough + waveshaper

## Task 5: Trigger timestamps test file

**Files:**
- Create: `src/go/internal/audio/trigger_timestamps_test.go`

- [ ] **Step 1: Write the test file**

Write to `src/go/internal/audio/trigger_timestamps_test.go`:

```go
package audio

import (
	"testing"
	"time"
)

func TestLastTriggerAt_UnknownIDIsZero(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)
	got := LastTriggerAt("does-not-exist")
	if !got.IsZero() {
		t.Errorf("LastTriggerAt(unknown) = %v; want zero time", got)
	}
}

func TestSinceLastTrigger_UnknownIDIsFar(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)
	got := SinceLastTrigger("does-not-exist")
	// Implementation returns 10*time.Hour for the never-triggered case.
	if got < 10*time.Hour {
		t.Errorf("SinceLastTrigger(unknown) = %v; want >= 10h", got)
	}
}

func TestRecordVoiceTriggerThenReads(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)

	before := time.Now()
	RecordVoiceTrigger("kick")
	after := time.Now()

	got := LastTriggerAt("kick")
	if got.IsZero() {
		t.Fatalf("LastTriggerAt(kick) = zero after RecordVoiceTrigger")
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("LastTriggerAt(kick) = %v; want in [%v, %v]", got, before, after)
	}
	since := SinceLastTrigger("kick")
	if since > time.Second {
		t.Errorf("SinceLastTrigger(kick) = %v; want < 1s", since)
	}
}

func TestResetTriggerTimestamps_ClearsAll(t *testing.T) {
	ResetTriggerTimestamps()
	t.Cleanup(ResetTriggerTimestamps)

	RecordVoiceTrigger("a")
	RecordVoiceTrigger("b")
	if LastTriggerAt("a").IsZero() || LastTriggerAt("b").IsZero() {
		t.Fatalf("preconditions failed: triggers not recorded")
	}
	ResetTriggerTimestamps()
	if !LastTriggerAt("a").IsZero() {
		t.Errorf("after reset, LastTriggerAt(a) = %v; want zero", LastTriggerAt("a"))
	}
	if !LastTriggerAt("b").IsZero() {
		t.Errorf("after reset, LastTriggerAt(b) = %v; want zero", LastTriggerAt("b"))
	}
	if SinceLastTrigger("a") < 10*time.Hour {
		t.Errorf("after reset, SinceLastTrigger(a) = %v; want >= 10h", SinceLastTrigger("a"))
	}
}
```

- [ ] **Step 2: Run the tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run "TestLastTriggerAt_UnknownIDIsZero|TestSinceLastTrigger_UnknownIDIsFar|TestRecordVoiceTriggerThenReads|TestResetTriggerTimestamps_ClearsAll" -v
```
Expected: all four PASS.

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/trigger_timestamps_test.go && git commit -m "test(audio): cover trigger-timestamp read/reset APIs"
```

## Task 6: Delete the unread `triggerLastEver` field

**Files:**
- Modify: `src/go/internal/audio/trigger_timestamps.go`

- [ ] **Step 1: Confirm the field is unread outside its own file**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "triggerLastEver" src/go/
```
Expected: three hits, all in `src/go/internal/audio/trigger_timestamps.go` (declaration + two writes — `RecordVoiceTrigger` and `ResetTriggerTimestamps`). No reads anywhere.

- [ ] **Step 2: Remove the field and its writes**

In `src/go/internal/audio/trigger_timestamps.go`:

Change the var block from:
```go
var (
	triggerMu       sync.RWMutex
	triggerByID     = map[string]time.Time{}
	triggerLastEver time.Time
)
```
to:
```go
var (
	triggerMu   sync.RWMutex
	triggerByID = map[string]time.Time{}
)
```

In `RecordVoiceTrigger`, change:
```go
func RecordVoiceTrigger(id string) {
	if id == "" {
		return
	}
	now := time.Now()
	triggerMu.Lock()
	triggerByID[id] = now
	if now.After(triggerLastEver) {
		triggerLastEver = now
	}
	triggerMu.Unlock()
}
```
to:
```go
func RecordVoiceTrigger(id string) {
	if id == "" {
		return
	}
	now := time.Now()
	triggerMu.Lock()
	triggerByID[id] = now
	triggerMu.Unlock()
}
```

In `ResetTriggerTimestamps`, change:
```go
func ResetTriggerTimestamps() {
	triggerMu.Lock()
	defer triggerMu.Unlock()
	triggerByID = map[string]time.Time{}
	triggerLastEver = time.Time{}
}
```
to:
```go
func ResetTriggerTimestamps() {
	triggerMu.Lock()
	defer triggerMu.Unlock()
	triggerByID = map[string]time.Time{}
}
```

- [ ] **Step 3: Run audio tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...
```
Expected: PASS — the Task 5 tests still cover the trimmed API.

- [ ] **Step 4: Confirm field is gone**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "triggerLastEver" src/go/
```
Expected: no matches.

- [ ] **Step 5: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/trigger_timestamps.go && git commit -m "refactor(audio): drop unread triggerLastEver field"
```

## Task 7: Channel EQ frequency response test file

**Files:**
- Create: `src/go/internal/audio/eq_response_channel_test.go`

- [ ] **Step 1: Write the test file**

Write to `src/go/internal/audio/eq_response_channel_test.go`:

```go
package audio

import (
	"math"
	"testing"
)

func TestGetChannelEQResponse_BadInputs(t *testing.T) {
	const channelID = "test.eqresp.bad"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	// numPoints <= 0 → nil
	if r := GetChannelEQResponse(channelID, 0, 20, 20000); r != nil {
		t.Errorf("numPoints=0: got non-nil response of length %d", len(r))
	}
	if r := GetChannelEQResponse(channelID, -10, 20, 20000); r != nil {
		t.Errorf("numPoints=-10: got non-nil response of length %d", len(r))
	}

	// No EQ set for this channel → nil
	if r := GetChannelEQResponse(channelID, 32, 20, 20000); r != nil {
		t.Errorf("unset channel: got non-nil response of length %d", len(r))
	}
}

func TestGetChannelEQResponse_BasicShape(t *testing.T) {
	const channelID = "test.eqresp.shape"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	SetChannelEQ(channelID, 48000,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 6},
	)

	r := GetChannelEQResponse(channelID, 32, 20, 20000)
	if len(r) != 32 {
		t.Fatalf("response length = %d; want 32", len(r))
	}

	// Frequencies must be strictly increasing (log-spaced).
	for i := 1; i < len(r); i++ {
		if r[i].Freq <= r[i-1].Freq {
			t.Errorf("freq not strictly increasing at i=%d: %.3f <= %.3f", i, r[i].Freq, r[i-1].Freq)
		}
	}

	// First frequency near 20 Hz, last near 20 kHz.
	if math.Abs(r[0].Freq-20) > 1e-6 {
		t.Errorf("first freq = %.6f; want 20", r[0].Freq)
	}
	if math.Abs(r[len(r)-1].Freq-20000) > 1e-6 {
		t.Errorf("last freq = %.6f; want 20000", r[len(r)-1].Freq)
	}

	// Magnitude at the band centre (~1 kHz, around index 17 on a log
	// spread from 20 to 20000) should be positive (boost).
	bestIdx, bestDist := 0, math.Inf(1)
	for i, p := range r {
		d := math.Abs(math.Log10(p.Freq) - math.Log10(1000))
		if d < bestDist {
			bestDist, bestIdx = d, i
		}
	}
	if r[bestIdx].MagnitudeDB <= 0 {
		t.Errorf("peaking +6dB at 1kHz: mag at nearest bin (%.1fHz) = %.2fdB; want > 0", r[bestIdx].Freq, r[bestIdx].MagnitudeDB)
	}
}

func TestGetChannelEQResponse_DefaultSampleRate(t *testing.T) {
	const channelID = "test.eqresp.defaultsr"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	// Sample rate of 0 should fall back to 48000 inside GetChannelEQResponse.
	SetChannelEQ(channelID, 0,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 6},
	)
	r := GetChannelEQResponse(channelID, 16, 100, 10000)
	if r == nil {
		t.Fatalf("response is nil; expected fallback to 48k sample rate")
	}
	if len(r) != 16 {
		t.Errorf("response length = %d; want 16", len(r))
	}
}
```

- [ ] **Step 2: Run the tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run "TestGetChannelEQResponse_" -v
```
Expected: all three PASS.

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/eq_response_channel_test.go && git commit -m "test(audio): cover GetChannelEQResponse public API"
```

## Task 8: passThrough fallback test

**Files:**
- Create: `src/go/internal/audio/insert_effects_passthrough_test.go`

- [ ] **Step 1: Write the test file**

Write to `src/go/internal/audio/insert_effects_passthrough_test.go`:

```go
package audio

import (
	"testing"
)

// TestPassThroughFallback exercises every method on the unregistered-
// effect fallback returned by NewEffectProcessor.
func TestPassThroughFallback(t *testing.T) {
	eff := NewEffectProcessor(EffectSlot{Type: EffectType("__unregistered__")}, 48000)

	// ProcessSample must be identity.
	for _, x := range []float64{-1.0, 0, 0.5, 1.0} {
		if got := eff.ProcessSample(x); got != x {
			t.Errorf("ProcessSample(%v) = %v; want %v (identity)", x, got, x)
		}
	}

	// Reset and SetParam must not panic and must not alter behaviour.
	eff.Reset()
	eff.SetParam("nope", 123.45)
	if got := eff.ProcessSample(0.3); got != 0.3 {
		t.Errorf("after Reset/SetParam, ProcessSample(0.3) = %v; want 0.3", got)
	}

	// ProcessBlockBuf (BlockProcessor optional interface) must copy.
	bp, ok := eff.(BlockProcessor)
	if !ok {
		t.Fatalf("passThrough must implement BlockProcessor")
	}
	in := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	out := make([]float32, len(in))
	bp.ProcessBlockBuf(in, out, len(in))
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("ProcessBlockBuf[%d] = %v; want %v", i, out[i], in[i])
		}
	}
}
```

- [ ] **Step 2: Confirm `BlockProcessor` interface name**

The interface might be named differently. Verify with:
```
cd /home/ymolinar/Repos/beatmo && grep -n "ProcessBlockBuf(" src/go/internal/audio/insert_effects.go
```
Expected: signature shows `ProcessBlockBuf(in, out []float32, samples int)`.

```
cd /home/ymolinar/Repos/beatmo && grep -n "type BlockProcessor" src/go/internal/audio/
```
If `BlockProcessor` is not defined, fall back to a structural type assertion:
```go
bp, ok := eff.(interface {
	ProcessBlockBuf(in, out []float32, samples int)
})
```
Use whichever the package exposes.

- [ ] **Step 3: Run the test**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run TestPassThroughFallback -v
```
Expected: PASS.

- [ ] **Step 4: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/insert_effects_passthrough_test.go && git commit -m "test(audio): cover passThrough fallback for unregistered effect types"
```

## Task 9: Add `ChannelEQ` + `ChannelEQSnapshot`, plus tests

**Files:**
- Modify: `src/go/internal/audio/eq_state.go`
- Modify: `src/go/internal/audio/eq_response_channel_test.go` (extend with a snapshot test)

- [ ] **Step 1: Add the public type and accessor**

In `src/go/internal/audio/eq_state.go`, append (after the existing `lookupChannelEQ` function):

```go
// ChannelEQ is the public snapshot type returned by ChannelEQSnapshot.
// Mirrors the internal eqRecord but with exported fields so callers
// outside the audio package can read it.
type ChannelEQ struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

// ChannelEQSnapshot returns the most recently applied EQ for the named
// channel. Returns the zero value when no EQ has been set for the
// channel. Callers receive an independent copy of the Bands slice so
// later mutations to the internal record do not alias the snapshot.
func ChannelEQSnapshot(id string) ChannelEQ {
	rec := lookupChannelEQ(id)
	out := ChannelEQ{ID: rec.ID, SampleRate: rec.SampleRate}
	if len(rec.Bands) > 0 {
		out.Bands = append([]EQBand(nil), rec.Bands...)
	}
	return out
}
```

- [ ] **Step 2: Add a snapshot test**

Append to `src/go/internal/audio/eq_response_channel_test.go`:

```go
func TestChannelEQSnapshot(t *testing.T) {
	const channelID = "test.eqsnap"
	t.Cleanup(func() { ClearChannelProcessors(channelID) })

	// Empty before any SetChannelEQ
	got := ChannelEQSnapshot(channelID)
	if got.ID != "" || got.SampleRate != 0 || len(got.Bands) != 0 {
		t.Errorf("empty snapshot: got %+v; want zero value", got)
	}

	SetChannelEQ(channelID, 48000,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 3},
		EQBand{Kind: EQHighShelf, Freq: 8000, Q: 0.7, GainDB: -2},
	)
	got = ChannelEQSnapshot(channelID)
	if got.ID != channelID {
		t.Errorf("ID = %q; want %q", got.ID, channelID)
	}
	if got.SampleRate != 48000 {
		t.Errorf("SampleRate = %d; want 48000", got.SampleRate)
	}
	if len(got.Bands) != 2 {
		t.Fatalf("Bands length = %d; want 2", len(got.Bands))
	}
	if got.Bands[0].GainDB != 3 || got.Bands[1].GainDB != -2 {
		t.Errorf("Bands gains = %+v; want [3, -2]", got.Bands)
	}

	// Mutating the snapshot's slice must not affect the internal record.
	got.Bands[0].GainDB = 99
	again := ChannelEQSnapshot(channelID)
	if again.Bands[0].GainDB != 3 {
		t.Errorf("snapshot aliased internal slice: re-read gain = %v; want 3", again.Bands[0].GainDB)
	}
}
```

- [ ] **Step 3: Run audio tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run "TestChannelEQSnapshot|TestGetChannelEQResponse_" -v
```
Expected: all four PASS.

- [ ] **Step 4: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/eq_state.go src/go/internal/audio/eq_response_channel_test.go && git commit -m "feat(audio): add public ChannelEQ + ChannelEQSnapshot accessor"
```

## Task 10: Migrate `LastSetEQ()` callers and retire the legacy global

**Files:**
- Modify: `src/go/internal/audio/insert_effects_helpers_test.go`
- Modify: `src/go/internal/ui/eq_export_import_test.go`
- Modify: `src/go/internal/ui/drumview_eq_controls_test.go`
- Modify: `src/go/internal/audio/eq_state.go`

- [ ] **Step 1: Migrate `insert_effects_helpers_test.go`**

Open `src/go/internal/audio/insert_effects_helpers_test.go` and replace the three `LastSetEQ()` usages at line 88 area. Read the surrounding context first, then:

Replace:
```go
got := LastSetEQ()
if got.ID != "main" {
	t.Errorf("LastSetEQ().ID = %q, want %q", got.ID, "main")
}
if got.SampleRate != 48000 {
	t.Errorf("LastSetEQ().SampleRate = %d, want 48000", got.SampleRate)
}
if len(got.Bands) != 1 || got.Bands[0].GainDB != 3 {
	t.Errorf("LastSetEQ().Bands = %+v, want one band with GainDB=3", got.Bands)
}
```
with:
```go
got := ChannelEQSnapshot("main")
if got.ID != "main" {
	t.Errorf("ChannelEQSnapshot(\"main\").ID = %q, want %q", got.ID, "main")
}
if got.SampleRate != 48000 {
	t.Errorf("ChannelEQSnapshot(\"main\").SampleRate = %d, want 48000", got.SampleRate)
}
if len(got.Bands) != 1 || got.Bands[0].GainDB != 3 {
	t.Errorf("ChannelEQSnapshot(\"main\").Bands = %+v, want one band with GainDB=3", got.Bands)
}
```

- [ ] **Step 2: Migrate `eq_export_import_test.go`**

Open `src/go/internal/ui/eq_export_import_test.go` and replace at line 70:

Replace:
```go
rec := audio.LastSetEQ()
if rec.ID != "main" {
	t.Fatalf("expected main EQ applied")
}
```
with:
```go
rec := audio.ChannelEQSnapshot("main")
if rec.ID != "main" {
	t.Fatalf("expected main EQ applied")
}
```

- [ ] **Step 3: Migrate `drumview_eq_controls_test.go`**

Open `src/go/internal/ui/drumview_eq_controls_test.go` and replace at lines 24 and 38:

Replace (first occurrence around line 24):
```go
rec := audio.LastSetEQ()
if rec.ID != "main" {
	t.Fatalf("expected main EQ to be applied, got id=%s", rec.ID)
}
```
with:
```go
rec := audio.ChannelEQSnapshot("main")
if rec.ID != "main" {
	t.Fatalf("expected main EQ to be applied, got id=%s", rec.ID)
}
```

Replace (second occurrence around line 38):
```go
rec = audio.LastSetEQ()
if rec.Bands[last].GainDB >= 0 {
```
with:
```go
rec = audio.ChannelEQSnapshot("main")
if rec.Bands[last].GainDB >= 0 {
```

- [ ] **Step 4: Confirm no callers remain before deleting the global**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "LastSetEQ\|lastSetEQ" src/go/
```
Expected: only the definition in `eq_state.go` remains. If any other caller appears, migrate it the same way before proceeding.

- [ ] **Step 5: Delete the legacy global from `eq_state.go`**

In `src/go/internal/audio/eq_state.go`:

Change the var block from:
```go
var (
	channelEQMu  sync.RWMutex
	channelEQMap = make(map[string]eqRecord)
	lastSetEQ    eqRecord
)
```
to:
```go
var (
	channelEQMu  sync.RWMutex
	channelEQMap = make(map[string]eqRecord)
)
```

Delete the `LastSetEQ` function:
```go
// LastSetEQ returns the most recent EQ settings applied via SetChannelEQ.
// Kept for backward compatibility with internal/ui tests; for new code use
// lookupChannelEQ to fetch the EQ of a specific channel.
func LastSetEQ() eqRecord { return lastSetEQ }
```

Change `recordChannelEQ` from:
```go
func recordChannelEQ(id string, rec eqRecord) {
	channelEQMu.Lock()
	if len(rec.Bands) == 0 {
		delete(channelEQMap, id)
	} else {
		channelEQMap[id] = rec
	}
	lastSetEQ = rec
	channelEQMu.Unlock()
}
```
to:
```go
func recordChannelEQ(id string, rec eqRecord) {
	channelEQMu.Lock()
	if len(rec.Bands) == 0 {
		delete(channelEQMap, id)
	} else {
		channelEQMap[id] = rec
	}
	channelEQMu.Unlock()
}
```

Also clean up the now-obsolete legacy comment block in the same file (the paragraph beginning "lastSetEQ is retained as a process-wide…" through "use lookupChannelEQ.") since the legacy is gone. Replace the comment block with a brief explanation of why the per-channel map is the structural invariant — the existing first paragraph of the comment already explains that and stays.

- [ ] **Step 6: Run audio + ui tests**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/... ./internal/ui/...
```
Expected: PASS.

- [ ] **Step 7: Confirm cleanup is complete**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "LastSetEQ\|lastSetEQ" src/go/
```
Expected: no matches.

- [ ] **Step 8: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/eq_state.go src/go/internal/audio/insert_effects_helpers_test.go src/go/internal/ui/eq_export_import_test.go src/go/internal/ui/drumview_eq_controls_test.go && git commit -m "refactor(audio): retire lastSetEQ global, migrate callers to ChannelEQSnapshot"
```

## Task 11: Waveshaper Reset test

**Files:**
- Create: `src/go/internal/audio/fx_waveshaper_reset_test.go`

- [ ] **Step 1: Write the test file**

Write to `src/go/internal/audio/fx_waveshaper_reset_test.go`:

```go
//go:build test || js

package audio

import "testing"

// TestWaveshaperReset is a structural test — waveshaper is stateless,
// so Reset() must be a no-op. We assert (a) no panic and (b) that
// ProcessSample's output is unchanged across a Reset.
func TestWaveshaperReset(t *testing.T) {
	w := newWaveshaper(48000, map[string]float64{
		"curve": 0, // soft (tanh)
		"drive": 4,
		"mix":   1.0,
	})

	before := w.ProcessSample(0.5)
	w.Reset()
	after := w.ProcessSample(0.5)

	if before != after {
		t.Errorf("waveshaper output diverged across Reset: before=%.6f after=%.6f", before, after)
	}
}
```

- [ ] **Step 2: Run the test**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run TestWaveshaperReset -v
```
Expected: PASS.

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/fx_waveshaper_reset_test.go && git commit -m "test(audio): cover waveshaper.Reset (stateless no-op)"
```

---

# Phase 4 — Effect SetParam catalog table coverage

## Task 12: Catalog-driven SetParam table test

**Files:**
- Create: `src/go/internal/audio/fx_setparam_table_test.go`

- [ ] **Step 1: Write the test file**

Write to `src/go/internal/audio/fx_setparam_table_test.go`:

```go
package audio

import (
	"math"
	"testing"
)

// TestEffectSetParamCatalogCoverage walks every registered effect type
// and exercises SetParam at the min, max, and midpoint of every
// declared parameter, plus an unknown-name pass to cover the default
// switch arm. After each SetParam we render a single sample to confirm
// the effect did not enter a NaN/Inf state.
//
// The test self-extends to any new effect added to the catalog: register
// it with EffectRegistrations and it is automatically covered.
func TestEffectSetParamCatalogCoverage(t *testing.T) {
	catalog := InsertEffectCatalog()
	if len(catalog) == 0 {
		t.Fatalf("InsertEffectCatalog is empty")
	}

	for effectType, params := range catalog {
		t.Run(string(effectType), func(t *testing.T) {
			eff := NewEffectProcessor(EffectSlot{Type: effectType, Enabled: true}, 48000)
			if eff == nil {
				t.Fatalf("NewEffectProcessor returned nil for %q", effectType)
			}

			assertFiniteOutput := func(stage string) {
				t.Helper()
				out := eff.ProcessSample(0.1)
				if math.IsNaN(out) || math.IsInf(out, 0) {
					t.Errorf("%s: ProcessSample(0.1) returned non-finite %v", stage, out)
				}
			}

			for _, def := range params {
				eff.SetParam(def.Name, def.Min)
				assertFiniteOutput("SetParam(" + def.Name + ", Min)")
				eff.SetParam(def.Name, def.Max)
				assertFiniteOutput("SetParam(" + def.Name + ", Max)")
				mid := (def.Min + def.Max) / 2
				eff.SetParam(def.Name, mid)
				assertFiniteOutput("SetParam(" + def.Name + ", mid)")
			}

			// Unknown name covers the default arm of every switch.
			eff.SetParam("__unknown_param_name__", 0)
			assertFiniteOutput("SetParam(unknown)")

			// Reset must leave the effect functional.
			eff.Reset()
			assertFiniteOutput("after Reset")
		})
	}
}
```

- [ ] **Step 2: Run the table test**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/ -run TestEffectSetParamCatalogCoverage -v
```
Expected: every sub-test (one per effect type) PASS.

- [ ] **Step 3: Commit**

```
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/audio/fx_setparam_table_test.go && git commit -m "test(audio): catalog-driven SetParam coverage for all effects"
```

---

# Final verification

## Task 13: Coverage delta verification

**Files:** none modified — read-only validation.

- [ ] **Step 1: Run full audio test suite under -race**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/audio/...
```
Expected: PASS, no race warnings, no goroutine leaks.

- [ ] **Step 2: Run the broader fast-path suite**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```
Expected: PASS — confirms that Task 10's UI test migration did not break unrelated packages.

- [ ] **Step 3: Run full coverage and confirm targets met**

```
cd /home/ymolinar/Repos/beatmo && make coverage-go
```
Then inspect the totals:
```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go tool cover -func=../../coverage/go.out | grep -E "internal/audio|^total:"
```
Expected:
- `internal/audio` line coverage ≥ 92% (baseline 85.4%).
- `total:` line ≥ 82.0% (baseline 80.4%).

- [ ] **Step 4: Confirm zero-coverage gaps closed for targeted functions**

```
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go tool cover -func=../../coverage/go.out | awk '$NF == "0.0%"' | grep -E "loudness\.go|recipe_overrides\.go|trigger_timestamps\.go|eq_state\.go|fx_waveshaper\.go|insert_effects\.go:115|insert_effects\.go:116"
```
Expected: no matches. Every targeted function is now > 0%.

- [ ] **Step 5: Confirm cleanup landed**

```
cd /home/ymolinar/Repos/beatmo && grep -rn "abs64Diff\|triggerLastEver\|LastSetEQ\|lastSetEQ" src/go/
```
Expected: no matches.

- [ ] **Step 6: No new commit (verification only)**

If all steps above pass, this plan is complete. If `internal/audio` is below 92% or total is below 82%, inspect the per-file coverage output to identify which targeted functions still have gaps and add follow-up tests in the relevant phase's commit style.

---

## Self-review checklist (already executed)

1. **Spec coverage:** Every phase of the spec maps to one or more tasks above. Phase 1 → Task 1. Phase 2 → Tasks 2-4. Phase 3 → Tasks 5-11. Phase 4 → Task 12. Final verification → Task 13.
2. **Placeholder scan:** No `TBD`, no `TODO`, no "implement later". Every test function body, every refactor edit, every command shown in full.
3. **Type consistency:** `ChannelEQ` / `ChannelEQSnapshot` referenced consistently across Tasks 9 and 10. `voiceCacheInvalidate` swap pattern matches `recipe_overrides_test.go:97-99`. `ResetTriggerTimestamps`, `RecordVoiceTrigger`, `LastTriggerAt`, `SinceLastTrigger` all match the actual signatures in `trigger_timestamps.go`.
4. **Spec-vs-plan deltas captured:** Phase 2 in the spec mentions covering "Apply with nil source" — but `recipe_overrides_test.go:240-247` already has `TestApplyNilSource_NoOp`. The plan substitutes the still-uncovered silent-skip branches (`Apply_DocWithEmptyIDInheritsKey`, `Apply_DocWithEmptyBaseRecipeSkipped`) which together hit the remaining branches inside `ApplyUserRecipeOverrides`.
