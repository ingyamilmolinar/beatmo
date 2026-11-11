# Task 4 Report — Full-suite verification + desktop sizing sanity

Plan: `docs/superpowers/plans/2026-07-02-eq-wheel-popup-value-editor.md`, Task 4.

## 1. Desktop-rect test

Added `TestEQWheelPopup_DesktopRectFitsBounds` to
`src/go/internal/ui/eq_wheel_popup_test.go`, placed after the existing
`TestEQWheelPopup_CenterBoxOpensNumericEditor`.

The brief's snippet (`dv.SetActiveTab(TabEQ)` + `dv.Layout(dv.Bounds)`) does not
exist in this codebase. The sibling tests in the same file already use a
canonical helper, `setupEQTabForTest(g, w, h)` (defined at the top of the file),
which drives `g.Layout` → `recalcButtons` → `tabState.SetActiveTab(TabEQ)` →
`Invalidate` → `eqPanelZone.Layout`. The new test copies that exact pattern
verbatim (no `forceSmallScreenForTest`, so it runs on the default desktop /
comfortable-density profile):

```go
func TestEQWheelPopup_DesktopRectFitsBounds(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	dv.openEQKnobWheelPopup(0)
	if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
		t.Fatal("EQ wheel popup did not open on desktop")
	}

	r := dv.EQWheelPopupRect()
	if r.Empty() {
		t.Fatal("desktop EQ wheel popup rect is empty")
	}
	if !r.In(dv.Bounds) {
		t.Fatalf("desktop EQ wheel popup rect %v not inside DrumView bounds %v", r, dv.Bounds)
	}
}
```

**Bounds assertion used and why:** `dv.Bounds` is the `image.Rectangle` field
on `DrumView` (same field the sibling `TestEQWheelPopup_OpensOnBothProfiles`-style
tests and the ctor's `Layout` logic operate on — it is literally what
`openEQKnobWheelPopup` passes into `dv.eqWheelPopup.Open(binding, anchor, dv.Bounds,
dv.headerH)` to keep the popup clamped in the first place). `EQWheelPopupRect()`
returns `dv.eqWheelPopup.Rect()`, which is already the widget's post-clamp
screen rect. The strict form `r.In(dv.Bounds)` (Go's `image.Rectangle.In`,
true iff `r` is a subset of `dv.Bounds`, including the empty-rect edge case
which is excluded separately via `r.Empty()`) was tried first per the plan's
instruction and **passed** — no relaxation was needed.

Isolated run (5x, no flakes):
```
go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup_DesktopRectFitsBounds' ./internal/ui/
--- PASS: TestEQWheelPopup_DesktopRectFitsBounds (0.00s)
PASS
```

No design-token change was needed — the comfortable-density `MobileWheelW/MobileWheelH`
tokens already fit inside the desktop DrumView bounds.

## 2. Full `internal/ui` fast suite

Command: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`

The suite is large enough (~440–600s wall time on this machine, under heavy
concurrent external churn per repo history) that it occasionally exceeds Go's
default 10-minute per-package test timeout when nothing aborts it early. Three
runs were taken to separate real signal from timing/order noise:

- **Run 1** (default timeout): aborted early via a panic inside
  `TestParityHighlightInFlightGraceFreshAudio` (`scheduler/drumview parity
  mismatch`) before the suite finished — this test only re-panics on an
  already-recovered panic path (`emitParityFatal`), so a single flaky parity
  mismatch under load kills the whole binary. Re-run in isolation
  (`-count=5`): **5/5 PASS**. This is the known pre-existing flake documented
  in project memory ("Parity audio_missing in-flight false positive" / general
  `internal/ui` flakiness under concurrent load) — not caused by this change.
- **Run 2** (default timeout): ran further (no early parity abort this time)
  but hit the 10-minute default timeout while `TestZoomDoesNotAllocPerFrame`
  was mid-run — an unrelated perf/alloc test, purely a wall-clock budget
  issue on a loaded machine, not a hang.
- **Run 3** (`-timeout 25m`, to get a clean completion signal): completed in
  441s, exit code 1, with exactly 6 failing top-level tests, all reproduced
  deterministically:

  | Test | File | Isolated re-run | EQ/wheel/portal/dB related? |
  |---|---|---|---|
  | `TestInstrumentCategoriesScrollMovesWindow` | `drumview_test.go` | 3/3 FAIL (deterministic) | No — instrument category-menu scroll/catalog isolation bug (`withAudioCatalog` leak: expected `"Cat-03"`, got `"Cymbal"`) |
  | `TestImportEmbeddedPCM` | `import_embedded_pcm_test.go` | 3/3 FAIL (deterministic) | No — import perf budget (307ms vs 250ms budget), machine-speed related |
  | `TestRowControls_NoCramping_AllProfiles` | `row_controls_layout_test.go` | 3/3 FAIL (deterministic) | No — mobile row mute/solo/fx/menu button width sizing |
  | `TestMobileRowNamesDoNotTruncate` | `row_typography_test.go` | 3/3 FAIL (deterministic) | No — row label cell width/typography |
  | `TestEveryRecipeGroupHasExplicitConceptRenderer` | `synth_concept_viz_test.go` | 3/3 FAIL (deterministic) | No — synth "kick" concept-renderer registration gap |
  | `TestFocusGraph_RespondsToEveryKnob` | `synth_focus_sweep_test.go` | not re-isolated individually (4s, large output); consistent across runs 2 & 3 | No — synth knob focus-graph responsiveness (`osc_type` across ~25 recipes) |

  A 7th test, `TestLongPressNotFiredOnSlowDragAtTapBoundary` (`gesture_test.go`),
  appeared in Run 2 only and passed both in Run 3's full pass and in a 3x
  isolated re-run — confirmed **flaky/order-dependent**, not a real failure,
  and unrelated to EQ (gesture drag-vs-longpress timing boundary).

  None of the 6 deterministic failures touch any file this feature (or the
  earlier Tasks 1–3) modified (`eq_panel_zone.go`, `eq_wheel_popup.go`,
  `drumview.go`, `drumview_ctor.go`, `drumview_portal_open.go`,
  `drumview_close_popups.go`). All are pre-existing, caused by unrelated
  concurrent churn already present in the working tree at session start (per
  `git status`, dozens of unrelated files — C DSP, synth recipes, row
  layout/typography — are modified outside this feature's scope).

**Every EQ-wheel-popup-related test passed in all runs**: `TestApplyBandGainLive_*`,
`TestEQDBOpenAdapter_*`, `TestEQWheelPopup_OpensOnBothProfiles` (desktop +
mobile), `TestEQWheelPopup_SharesSourceOfTruthWithCurve`,
`TestEQWheelPopup_CenterBoxOpensNumericEditor`,
`TestEQWheelPopup_DesktopRectFitsBounds` (new), `TestEQWheelPopup_IsModalBlocking`,
`TestEQWheelPopup_BlocksGridTap`, and the EQ/db/audio-apply regression sweep
from Task 1 Step 7 / Task 2 Step 10.

## 3. Real regression found and fixed (in scope)

`TestFunctional_SoftKeyboardArrowMovesCaret`
(`src/go/internal/ui/arrow_key_navigation_functional_test.go`) failed
deterministically (5/5 in isolation) with `click did not open dB editor`. Root
cause: this test predates the Task 1–3 routing change (`eqDBOpenAdapter.OnPress`
now opens the wheel popup instead of the numeric editor directly when
`OnOpenValueWheel` is wired) and still asserted the *old* direct-open
behavior. Its sibling test one function up,
`TestFunctional_ArrowKeys_EQdBEditor`, had already been updated to the new
two-step flow (click readout → wheel popup opens → click center box → numeric
editor opens) but this one test was missed.

**Fix**: updated `TestFunctional_SoftKeyboardArrowMovesCaret` to mirror
`TestFunctional_ArrowKeys_EQdBEditor`'s flow exactly — click the dB readout,
assert the wheel popup opened, click `eqWheelPopup.centerH`, then assert the
numeric editor is active before driving the soft-keyboard arrow signals.

Verified: `go test -run 'TestFunctional_SoftKeyboardArrowMovesCaret|TestFunctional_ArrowKeys_EQdBEditor' -count=5` → **10/10 PASS**.

## Files changed (Task 4 only)

- `src/go/internal/ui/eq_wheel_popup_test.go` — added `TestEQWheelPopup_DesktopRectFitsBounds`.
- `src/go/internal/ui/arrow_key_navigation_functional_test.go` — fixed `TestFunctional_SoftKeyboardArrowMovesCaret` to route through the wheel-popup open flow (real regression from Tasks 1–3, fixed here).

Nothing staged, committed, or stashed — all changes left in the working tree
for review per standing repo rule.

## Concerns

- The `internal/ui` package's default `go test` invocation (no `-timeout`
  override) can exceed Go's 10-minute default timeout on this machine under
  concurrent load, aborting the run with a `panic: test timed out` rather than
  a clean FAIL summary. Not introduced by this change, but worth flagging —
  future full-suite verifications on this branch may need `-timeout 20m`+ to
  get a clean signal.
- 6 pre-existing, deterministic, unrelated failures remain in `internal/ui`
  (listed above) — none touch EQ/wheel/portal/dB code, so they are out of
  this task's scope per the assignment, but they mean the raw `go test
  ./internal/ui/` invocation is NOT currently green on this branch
  independent of this feature.

## Final-review fixes

Three findings from final review of the EQ wheel-popup feature.

### Finding 1 (Important) — restore persisted step resolution

`dv.eqWheelStepBadge` (built in `drumview_ctor.go`) saved the chosen
step-resolution rung (`OnResolution: func() { dv.persistKnobStep(badge) }`,
`eq_wheel_popup.go`) but never restored it — new sessions always started at
the default rung. The synth/sampler wheels restore via `dv.knobStepPref(name)`
+ `badge.SetStep(persisted)` (`synth_panel_zone.go:1006-1007`,
`sampler_panel_zone.go:323-324`), reading `dv.knobStepPref` which wraps the
process-global `activeKnobStepSink()` (`knob_step_sink.go`).

Added a `eqWheelStepRestored bool` field to `DrumView` (`drumview.go`, next to
`eqWheelStepBadge`) and a lazy, one-shot restore at the top of
`openEQKnobWheelPopup` (`eq_wheel_popup.go`), done lazily (not in the ctor)
because the ctor can run before `SetKnobStepSink` installs the global sink:

```go
badge := dv.eqWheelStepBadge
// Lazily restore the persisted step-resolution rung on first open, not in
// the ctor: the ctor may run before SetKnobStepSink installs the global
// sink, so an eager read there would always miss.
if !dv.eqWheelStepRestored {
	if persisted, ok := dv.knobStepPref("eq_band_gain"); ok && badge != nil {
		badge.SetStep(persisted)
	}
	dv.eqWheelStepRestored = true
}
```

The pref key `"eq_band_gain"` matches both the `ParamDef.Name` used to
construct `eqWheelStepBadge` in `drumview_ctor.go` (`NewKnobStepBadge(audio.ParamDef{Name: "eq_band_gain", ...})`)
and the key `persistKnobStep` saves under (`b.ParamName()`, i.e. `b.def.Name`)
— confirmed by reading `persistKnobStep`/`knobStepPref` in `synth_panel_zone.go:2456-2480`.

**Test added**: `TestEQWheelPopup_RestoresPersistedStep` in
`src/go/internal/ui/eq_wheel_popup_test.go`, plus a `stubKnobStepSink` helper
type (`LoadKnobSteps`/`SaveKnobStep` in-memory map — no existing test stub for
`KnobStepSaveSink` was found in the repo, so this is new but minimal, mirroring
`userprefs.fileStore`/`localStorageStore`'s shape).

The test:
1. Computes `eq_band_gain`'s default rung (`0.05`, the ladder-index closest to
   400 notches over the `[-12,12]` dB span whose ladder is
   `{0.02,0.05,0.1,0.2,0.5,1,2}`).
2. Installs a `stubKnobStepSink` via `SetKnobStepSink` (cleaned up via
   `t.Cleanup(func() { SetKnobStepSink(nil) })`) with `"eq_band_gain": 1.0`
   persisted — an exact, distinct ladder rung.
3. Asserts `dv.eqWheelStepRestored` starts `false` on a fresh `DrumView`.
4. Calls `dv.openEQKnobWheelPopup(1)` and asserts `eqWheelStepRestored` flips
   to `true` and `eqWheelStepBadge.Step() == 1.0` (not the default `0.05`).
5. Mutates the stub's persisted value to `0.02` and opens the popup again on
   band 2, asserting the badge stays at `1.0` — proving the restore is
   one-shot (doesn't re-read the sink on every open, which would fight a
   user's in-session cycling of the badge).

**Proof the test catches the regression**: temporarily reverted the restore
block in `eq_wheel_popup.go` (kept everything else) and re-ran just the new
test — it failed as expected (`badge step 0.05 equals default 0.05; persisted
value was not restored` — the exact assertion in step 4). Restored the fix
afterward; re-ran the full `TestEQWheelPopup` suite, all green again.

### Finding 2 (Minor) — named constants for the clamp

`EQPanelZone.applyBandGainLive` (`eq_panel_zone.go`) clamped with the literals
`clampF64(v, -12.0, 12.0)`. Changed to `clampF64(v, eqDBMin, eqDBMax)` (consts
defined in `eq_wheel_popup.go`, same package). `eqDBSpec()`'s own literals were
left untouched per the finding's explicit scope note.

### Finding 3 (Minor) — stale doc comment

The const block in `eq_wheel_popup.go` was commented `// eqDBRange is the EQ
band gain domain...` — `eqDBRange` doesn't exist; the actual consts are
`eqDBMin`/`eqDBMax`/`eqDBSpan`. Reworded to:

```go
// eqDBMin/eqDBMax bound the EQ band gain domain, matching eqDBSpec()'s clamp;
// eqDBSpan is their derived width, used to map the endless knob's normalized
// [0,1] value onto real dB units.
const (
	eqDBMin  = -12.0
	eqDBMax  = 12.0
	eqDBSpan = eqDBMax - eqDBMin
)
```

### Files changed (final-review fixes)

- `src/go/internal/ui/drumview.go` — added `eqWheelStepRestored bool` field.
- `src/go/internal/ui/eq_wheel_popup.go` — added lazy one-shot restore call
  (Finding 1); reworded the const-block doc comment (Finding 3).
- `src/go/internal/ui/eq_panel_zone.go` — `applyBandGainLive` clamp now uses
  `eqDBMin`/`eqDBMax` (Finding 2).
- `src/go/internal/ui/eq_wheel_popup_test.go` — added `stubKnobStepSink` +
  `TestEQWheelPopup_RestoresPersistedStep`.

### Verify command + output

```
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup' ./internal/ui/ -v
```

```
--- PASS: TestEQWheelPopup_IsModalBlocking (0.00s)
--- PASS: TestEQWheelPopup_BlocksGridTap (0.00s)
--- PASS: TestEQWheelPopup_AppliesToMasterChannel (0.00s)
--- PASS: TestEQWheelPopup_AppliesToRowChannel (0.00s)
--- PASS: TestEQWheelPopup_OpensOnBothProfiles (0.00s)
    --- PASS: TestEQWheelPopup_OpensOnBothProfiles/desktop (0.00s)
    --- PASS: TestEQWheelPopup_OpensOnBothProfiles/mobile (0.00s)
--- PASS: TestEQWheelPopup_SharesSourceOfTruthWithCurve (0.00s)
--- PASS: TestEQWheelPopup_CenterBoxOpensNumericEditor (0.00s)
--- PASS: TestEQWheelPopup_DesktopRectFitsBounds (0.00s)
--- PASS: TestEQWheelPopup_RestoresPersistedStep (0.00s)
PASS
ok  	github.com/ingyamilmolinar/beatmo/internal/ui	0.192s
```

All 10 `TestEQWheelPopup*` tests PASS, including the new restore test.
`go build -tags test -modfile=go.test.mod ./internal/ui/...` also clean.
