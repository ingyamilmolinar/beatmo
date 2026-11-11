# Mobile Synth Side-by-Side Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-arrange the mobile synth tab so the chip-strip pipeline spans the top, knobs occupy a full-height left column, and both wave graphs ("Your sound" + selected-knob focus) sit stacked in a right column — fixing the unusable ~46px knob row.

**Architecture:** The draw path already stacks the mirror+focus into `dv.instEditorMobileFocusRect` via `splitSynthRightPane`. This change only moves *where* that rect is computed — from a full-width band carved off the TOP of the sections area (in `buildSynthTab`) to a RIGHT column carved off the detail body (in `layoutSynthSections`). The knob grid then takes the left column with full body height.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/ui` only.

**Conventions (read once):**
- Bundled Go from `src/go`: `../../.tools/go/bin/go`. Test: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- **Do NOT `git add`/commit/stash/restore/`rm`/move tracked files** (repo policy). Each "Commit" step is a **Checkpoint**: run the test, leave changes in the tree, report.
- The full `internal/ui` suite has pre-existing failures + active concurrent i18n/wave WIP that intermittently flap-breaks the `-tags test` build in shared files (`synth_panel_zone.go`, `synth_preview_layer.go`, `i18n_*`/`wave_*`/`freeze_*` tests). Verify ONLY your own tests with `-run`; trust `go test`/`go vet` over IDE diagnostics. If the build fails ONLY in a concurrent-WIP file, move it aside + restore unconditionally in one command. **Do the work, verify with `-run`, then STOP — never spawn background log-watch loops.**

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/internal/ui/synth_panel_zone.go` | synth tab layout | `buildSynthTab` mobile branch (drop top band); `layoutSynthSections` (carve right wave column); `drawSynthTab` mobile comment |
| `src/go/internal/ui/synth_selected_knob.go` | `synthMobileFocusRect()` accessor | doc-comment update only |
| `src/go/internal/ui/synth_mobile_focus_test.go` | mobile geometry tests | replace the "above grid" test with "right of knobs"; keep the render test |
| `src/go/internal/ui/scene_catalog.go` | (no change expected) | mobile scenes re-exercise the new rect automatically |

---

## Task 1: Move the wave rect from a top band to a right column

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`buildSynthTab` mobile branch `:720-767`; `layoutSynthSections` near `:1245-1249`; draw comment `:1534-1535`)
- Test: `src/go/internal/ui/synth_mobile_focus_test.go`

- [ ] **Step 1: Write the failing test**

In `src/go/internal/ui/synth_mobile_focus_test.go`, REPLACE the existing `TestMobileSynthTab_HasFocusBandAboveGrid` with the new geometry test (keep the file's existing imports + the mobile-game helper it already uses — find it via `grep -n "func.*[Mm]obileSynthTab\|SetForceMobileProfile\|newMobileSynthTabGameForTest" synth_mobile_focus_test.go`; reuse that exact setup). The new test:

```go
func TestMobileSynthTab_WavesRightOfKnobs(t *testing.T) {
	g := newMobileSynthTabGameForTest(t) // reuse the existing mobile setup helper
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	waves := dv.synthMobileFocusRect()
	knobs := dv.instEditorDetailR
	if waves.Empty() {
		t.Fatalf("mobile wave column is empty — focus graph unreachable on mobile")
	}
	if knobs.Empty() {
		t.Fatalf("mobile knob/detail column is empty")
	}
	// Waves sit to the RIGHT of the knob column (not above it).
	if waves.Min.X < knobs.Max.X {
		t.Fatalf("wave column (%v) must be right of the knob column (%v)", waves, knobs)
	}
	// Waves span (roughly) the detail body height, not a thin top strip:
	// the column should be at least half the knob column's height.
	if waves.Dy() < knobs.Dy()/2 {
		t.Fatalf("wave column height %d too short vs knob column %d (expected a full-height side column)", waves.Dy(), knobs.Dy())
	}
}
```

If the helper is named differently (e.g. it inlines `SetRuntimeProfileForTest(browserRuntimeProfile())` + `SetForceMobileProfile(true)` + `g.Layout(390, 844)` + `SetActiveTab(TabSynth)` + `Update()`), reuse that exact inline sequence — the prior test in this file already builds a mobile synth tab; copy its construction verbatim.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSynthTab_WavesRightOfKnobs`
Expected: FAIL — with the current top-band layout, `waves.Min.X` is the panel's left edge (not right of the knobs), so the `waves.Min.X < knobs.Max.X` assertion fails.

- [ ] **Step 3: Drop the top-band carve in `buildSynthTab`**

In `synth_panel_zone.go`, the mobile branch currently spans `:720-767` (the `} else if mobile {` block that computes `minBandH`/`minGridH`/`bandH` and sets `instEditorMobileFocusRect` to a top band, shrinking `sectionsRect.Min.Y`). Replace that ENTIRE `} else if mobile {` block with:

```go
	} else if mobile {
		dv.instEditorPreviewRect = image.Rectangle{}
		// Mobile: the "Your sound" mirror + focus graph live in a RIGHT column
		// beside the knob grid, set in layoutSynthSections once detailR is known
		// (NOT a top band). Cleared here; the sections area keeps its full height
		// so the chip strip + knob grid get the room.
		dv.instEditorMobileFocusRect = image.Rectangle{}
	} else {
```

(Leave the `if previewW > 0 {` desktop branch above it and the trailing `} else {` narrow-desktop branch unchanged.)

- [ ] **Step 4: Carve the right wave column in `layoutSynthSections`**

In `layoutSynthSections`, find where `detailR` is finalized (around `:1245-1249`):

```go
	detailR := image.Rectangle{}
	if top := stripBottom + SpaceSM; top < rowR.Max.Y {
		detailR = image.Rect(rowR.Min.X, top, rowR.Max.X, rowR.Max.Y)
	}
	dv.instEditorDetailR = detailR
```

Replace the `dv.instEditorDetailR = detailR` line with the mobile split (carve the right column, then assign the shrunk left column):

```go
	if mobile && !detailR.Empty() {
		// Right column for the stacked waves (Your sound + focus graph); the knob
		// grid keeps the left column at full body height. Width ~43% of the detail
		// body, clamped so the knob column keeps room for one good-sized dial +
		// label and the wave column stays legible. If the body is too narrow for
		// both, skip the split (knobs take the whole body; waves omitted at that
		// pathological width).
		const minKnobColW, minWaveColW = 150, 110
		waveW := detailR.Dx() * 43 / 100
		if waveW > detailR.Dx()-minKnobColW {
			waveW = detailR.Dx() - minKnobColW
		}
		if waveW >= minWaveColW {
			dv.instEditorMobileFocusRect = image.Rect(
				detailR.Max.X-waveW, detailR.Min.Y, detailR.Max.X, detailR.Max.Y,
			)
			detailR.Max.X = dv.instEditorMobileFocusRect.Min.X - SpaceSM
		}
	}
	dv.instEditorDetailR = detailR
```

Note: `instEditorMobileFocusRect` is only ever set non-empty here on `mobile`; on desktop `buildSynthTab` already cleared it. `placeKnobsInSection` reads `instEditorDetailR` (now the left column) and lays knobs there automatically.

- [ ] **Step 5: Update the stale draw comment**

In `drawSynthTab` (around `:1534-1535`), update the comment above the `if mb := dv.instEditorMobileFocusRect; !mb.Empty() {` block from "stacked band reserved at the top of the sections area" to:

```go
	// Mobile: no side preview pane — the "Your sound" mirror + focus graph live in
	// a RIGHT column beside the knob grid (instEditorMobileFocusRect), split
	// vertically into mirror (top) + focus graph (bottom) by splitSynthRightPane.
```

(The block's CODE is unchanged — it already does `splitSynthRightPane(mb)` → mirror + focus.)

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSynthTab_WavesRightOfKnobs`
Expected: PASS.

- [ ] **Step 7: vet + checkpoint**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -i panel_zone`
Expected: empty (ignore unrelated concurrent-WIP lines). Report; do NOT commit.

---

## Task 2: Knobs get real height + waves render in the right column + existing guards

**Files:**
- Modify: `src/go/internal/ui/synth_mobile_focus_test.go`
- Modify: `src/go/internal/ui/synth_selected_knob.go` (doc comment only)

- [ ] **Step 1: Write the "knobs get real height" test**

Append to `synth_mobile_focus_test.go`:

```go
func TestMobileSynthTab_KnobColumnHasUsableHeight(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	// The knob/detail column body (minus the header) must be tall enough for at
	// least two knob rows now that the waves moved to the side. Use the density
	// minimum dial + caption as the per-row unit.
	d := Profile().DensityValues()
	rowUnit := d.SynthKnobMin + d.SynthKnobCaptionH
	body := dv.instEditorDetailR.Dy() - dv.instEditorDetailHeaderH
	if body < 2*rowUnit {
		t.Fatalf("knob column body height %d < 2 knob rows (%d) — knobs still cramped", body, 2*rowUnit)
	}
	// And the first laid-out knob has a non-empty, tappable rect.
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no section knobs")
	}
	k := dv.instEditorKnobs[sec.knobIdxs[0]]
	if k.Rect().Empty() || k.Rect().Dx() < d.SynthKnobMin {
		t.Fatalf("first knob rect %v too small (min dia %d)", k.Rect(), d.SynthKnobMin)
	}
}
```

> If `instEditorDetailHeaderH`/`SynthKnobCaptionH`/`SynthKnobMin` field names differ, confirm via `grep -n "SynthKnobMin\|SynthKnobCaptionH\|instEditorDetailHeaderH" internal/ui/*.go` and match. The threshold `2*rowUnit` is the floor the side-by-side layout must beat — if a real mobile panel genuinely can't fit two rows after the split, lower to `1.5*rowUnit` and note why (but with the top band gone it should comfortably exceed two).

- [ ] **Step 2: Update the wave-render test to assert the RIGHT column**

In `synth_mobile_focus_test.go`, find `TestMobileSynthTab_FocusGraphRespondsToKnobSelect` (added in the phase-2 mobile task). It currently draws `drawSynthTab` and asserts ink inside `synthMobileFocusRect()`. That still holds (the rect is now the right column) — just confirm it still passes. If it asserted anything position-specific about a "top band", update it to the right-column rect. Leave the ink-presence assertion intact.

- [ ] **Step 3: Run the mobile suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSynthTab_' -v 2>&1 | tail -15`
Expected: PASS for `WavesRightOfKnobs`, `KnobColumnHasUsableHeight`, `FocusGraphRespondsToKnobSelect`.

- [ ] **Step 4: Run the existing mobile guards (must not regress)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthChips_MobileWrapAllStagesVisible|TestSynthTab_KnobDragViaTreeDispatch_Mobile|TestDrawSynthTab_RendersFocusGraph' 2>&1 | tail -10`
Expected: PASS. These exercise all-stages-visible + a tree-dispatch knob drag + desktop focus graph. If one fails because the increased body height shifted a pinned expectation, update that expectation to the new (larger) geometry with a one-line rationale — do NOT weaken the assertion's intent (e.g. a drag must still change the param; all stages must still be visible).

- [ ] **Step 5: Update the accessor doc comment**

In `synth_selected_knob.go`, the `synthMobileFocusRect()` accessor's comment says the band is "above the knob grid" — update to "the right column beside the knob grid (empty on desktop)". Code unchanged.

- [ ] **Step 6: vet + checkpoint** — `go vet -tags test ./internal/ui/` clean for your files. Report; do NOT commit.

---

## Task 3: Visual verification (desktop unaffected + mobile re-arranged)

**Files:** none (capture + confirm)

- [ ] **Step 1: Confirm desktop is untouched**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDrawSynthTab_RendersFocusGraphInLowerBand|TestSynthPreviewWidth|SceneCrop' 2>&1 | tail -6`
Expected: PASS (desktop preview pane + crop scenes unchanged; `instEditorMobileFocusRect` stays empty on desktop).

- [ ] **Step 2: Recapture the full mobile synth tab**

Run (repo root): `make screenshots-all MOBILE=1 SCENES=mobile_bottom_nav_synth 2>&1 | tail -8`
Then Read `screenshots/all/mobile/mobile_bottom_nav_synth.browser.png`. Confirm:
- Chip strip (pipeline) spans the top, full width.
- A **big, tappable dial on the LEFT** with its label (no longer a clipped sliver).
- **"Your sound" + the focus graph stacked on the RIGHT**, beside the knobs.
Report what you see. If `make wasm` fails on concurrent WIP, retry once or capture via `scripts/capture_all_ui.sh MOBILE=1` against the existing `src/js/main.wasm`.

- [ ] **Step 3: Recapture a mobile focus crop**

Run (repo root): `make screenshots-all MOBILE=1 SCENES=crop_synth_focus_filter 2>&1 | tail -8`
Read `screenshots/all/mobile/crop_synth_focus_filter.browser.png` and confirm the focus graph (Cutoff curve) renders in the right-column wave area with a visible knob to its left. Report what you see.

- [ ] **Step 4: Final regression + vet**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Synth|Mobile|Focus|SceneCrop' 2>&1 | tail -6`
Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | head -5`
Expected: PASS; vet clean for your files.

- [ ] **Step 5: Final checkpoint** — summarize: mobile knob column height before/after, the screenshots confirmed, desktop unaffected. Hand the working tree to the user. Do NOT commit.

---

## Self-Review — spec coverage

| Spec requirement | Task |
|---|---|
| Pipeline (chips) on top, full width | unchanged (verified Task 2 Step 4: all stages visible) |
| Knobs → left column, full body height | 1 (detailR = left column), 2 (height test) |
| Both waves → right column, stacked | 1 (instEditorMobileFocusRect = right column; draw via splitSynthRightPane) |
| Remove the top focus band | 1 Step 3 |
| Width-guard fallback (narrow panels) | 1 Step 4 (`minKnobColW`/`minWaveColW`) |
| Desktop unaffected | 3 Step 1 |
| Existing mobile guards stay green | 2 Step 4 |
| Visual confirmation desktop + mobile | 3 |
| Accessor/comment semantics updated | 1 Step 5, 2 Step 5 |
