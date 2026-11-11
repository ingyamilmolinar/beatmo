# Mobile Multi-Column Knob Grids Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the mobile Sampler and Synth tab knob grids use 2–4 width-adaptive columns instead of a hard single column, filling the empty horizontal space.

**Architecture:** Both tabs already lay knobs out through `ControlGrid`, whose `Layout` natively picks `cols = clamp((rectW+hGap)/(idealCellW+hGap), 2, maxCols)`. The only change is the mobile `SetMaxCols` cap: 1 → 4 at both call sites. Cell internals (label band / pill band / caption), the value-pill buttons, wheel popups, undo wiring, and all desktop layouts are untouched.

**Tech Stack:** Go (Ebiten UI), fast stubbed test path (`-tags test -modfile=go.test.mod`), bundled Go at `.tools/go/bin/go`.

**Spec:** `docs/superpowers/specs/2026-07-07-mobile-multicolumn-knob-grids-design.md`

## Global Constraints

- **NEVER `git add` / `git commit` / `git revert` / `rm`** — leave all changes uncommitted in the working tree (standing user rule; overrides the usual commit-per-task steps).
- All Go commands use the bundled toolchain: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- Fast test command shape: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<Regex>'`.
- Mobile-only behavior change. Desktop: sampler keeps its bespoke one-row branch; synth keeps `maxCols = 2`.
- Do not modify `*.gen.go` files or design tokens — no new tokens are needed.
- Useful token values (Spacious density, the mobile default): `SynthKnobIdeal = 88`, `SpaceSM = 6`, `SpaceXS = 3`. A 390-px-wide panel therefore adapts to 4 columns; narrower rects settle at 3 or the 2-column floor.
- Pre-existing failures on this branch are known (see memory `project_add_core_node_types_preexisting_failures`; e.g. `TestEllipsisMenuDesktopItems`, `TestPanelTabAlwaysTallHeight`, flaky `TestParityHighlightInFlightGraceFreshAudio`, `TestAudioPanelTabsBlankWithoutAudio/Spectrum`) — do not chase failures that reproduce on a clean checkout of files this plan doesn't touch.

---

### Task 1: Sampler tab — mobile grid cap 1 → 4

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go:610-631` (`layoutSamplerKnobsScrolling` doc comment + `SetMaxCols`)
- Modify: `src/go/internal/ui/sampler_scroll_test.go:26-54` (replace `TestSamplerKnobsSingleColumnMobile`)

**Interfaces:**
- Consumes: `ControlGrid.SetMaxCols(int)`, `ControlGrid.Cols()`, existing test harness `samplerLayoutGame(t)` / `withSmallScreen(t, true)` / `SetDensityForTest(DensitySpacious)` / `visibleSamplerKnobRects(dv)` (all already in the package).
- Produces: mobile sampler knob grid reporting 2–4 columns; later tasks rely on nothing new from this task.

- [x] **Step 1: Replace the single-column pin test with a multi-column test**

In `src/go/internal/ui/sampler_scroll_test.go`, delete `TestSamplerKnobsSingleColumnMobile` (lines 26–54) and put this in its place:

```go
// TestSamplerKnobsMultiColumnMobile pins the mobile Sampler tab to a
// width-adaptive multi-column knob grid (2–4 columns, ControlGrid's native
// behaviour capped at 4) so the panel's horizontal space is used instead of
// stacking one pill per row. On a 390-px panel at Spacious density the grid
// adapts to 4 columns and all five knobs fit in two rows without scrolling.
func TestSamplerKnobsMultiColumnMobile(t *testing.T) {
	g := samplerLayoutGame(t)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	g.drum.sampler.captureFromSynth("kick")
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 300), "kick")

	grid := g.drum.sampler.knobGrid
	if grid == nil {
		t.Fatal("sampler knob grid not created on mobile")
	}
	if c := grid.Cols(); c < 2 || c > 4 {
		t.Errorf("mobile sampler grid Cols()=%d, want 2..4 (adaptive, capped at 4)", c)
	}
	vis := visibleSamplerKnobRects(g.drum)
	if len(vis) != samplerKnobCount {
		t.Errorf("all %d knobs should fit the two-row multi-column grid, got %d visible", samplerKnobCount, len(vis))
	}
	xs := map[int]bool{}
	for _, r := range vis {
		xs[r.Min.X] = true
	}
	if len(xs) < 2 {
		t.Errorf("mobile sampler knobs must span multiple columns; all share one X (rects=%v)", vis)
	}
	// Knobs 0 and 1 sit in the same (first) row → same top edge.
	if vis[0].Min.Y != vis[1].Min.Y {
		t.Errorf("knobs 0 and 1 should share the first row: Y %d vs %d", vis[0].Min.Y, vis[1].Min.Y)
	}
}
```

- [x] **Step 2: Run the new test to verify it fails**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerKnobsMultiColumnMobile' -v
```

Expected: FAIL — `Cols()=1, want 2..4` (and only a subset of knobs visible).

- [x] **Step 3: Raise the cap in `layoutSamplerKnobsScrolling`**

In `src/go/internal/ui/sampler_panel_zone.go` change line 630:

```go
	grid.SetMaxCols(1)
```

to:

```go
	grid.SetMaxCols(4)
```

and rewrite the function's doc comment (lines 610–615) to match the new behavior:

```go
// layoutSamplerKnobsScrolling lays the five knobs out in a width-adaptive
// grid (ControlGrid's native column selection, capped at 4) inside gridR,
// scrolling when the rows overflow. Each cell reserves a band above the
// control for the group label (TRIM/TUNE/LEVEL), the control band at the
// density ideal, and a caption band below. Off-window knobs get an empty rect
// so drawSamplerKnobs / samplerTabHitAreas skip them. Mirrors the Synth tab's
// ControlGrid usage (same 4-column mobile cap).
```

- [x] **Step 4: Run the new test to verify it passes**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerKnobsMultiColumnMobile' -v
```

Expected: PASS.

- [x] **Step 5: Run every sampler test and reconcile**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sampler' -v 2>&1 | tail -40
```

Expected: PASS across the board. Notes on the tests most likely to trip, and what to do if they do:
- `TestSamplerKnobsScrollOverflowMobile` / `TestSamplerScrollRevealsLaterKnobs` (`samplerScrollGame`, 390×210 rect): still valid under multi-column — with 3–4 columns knob 0 (Start) is in row 0 and knob 4 (Gain) in row 1, and one ~130-px-tall row per 210-px window still overflows. If the short rect no longer overflows, shorten `contentR` in `samplerScrollGame` (e.g. `image.Rect(0, 0, 390, 150)`) rather than weakening assertions.
- `TestSamplerKnobUsableSizeMobileAllLanguages` / `TestSamplerKnobSizeLanguageInvariantMobile`: column count depends only on rect width + density, never on locale, and at 4 columns the 93-px cell still holds the 88-px ideal dial band. Should pass unchanged; if a clamp fires, report the numbers before touching the test.
- `TestSamplerKnobCellsNoOverlapMobile`, `TestSamplerKnobCellReservesGroupLabelBandMobile`, `sampler_wheel_popup_test.go`: geometry-per-cell and interaction wiring are unchanged; expected green.

Do NOT commit (standing user rule).

---

### Task 2: Synth tab — mobile stage-card grids cap 1 → 4

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:1704-1712` (`placeKnobsInSection` maxCols block)
- Modify: `src/go/internal/ui/synth_mobile_focus_test.go` (add one test; adjust `TestMobileSynthTab_KnobRendersAtIdealSize` only if Step 5 shows it width-clamped)

**Interfaces:**
- Consumes: `dv.sectionGrid(s.id) *ControlGrid`, `dv.synthSelectedSection() *synthSection`, harness `newMobileSynthTabGameForTestSize(t, 390, 844)` (all already in the package).
- Produces: every laid-out synth stage grid reports 2–4 columns on mobile.

- [x] **Step 1: Write the failing test**

Append to `src/go/internal/ui/synth_mobile_focus_test.go`:

```go
// TestMobileSynthSectionGridMultiColumn pins the mobile Synth stage-card knob
// grid to ControlGrid's width-adaptive column count capped at 4 (was a hard
// single column), matching the Sampler tab, so stage cards use the panel's
// width instead of one knob per row.
func TestMobileSynthSectionGridMultiColumn(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("no open section with >=2 knobs on mobile")
	}
	grid := dv.sectionGrid(sec.id)
	if grid == nil {
		t.Fatal("selected synth section has no grid")
	}
	if c := grid.Cols(); c < 2 || c > 4 {
		t.Errorf("mobile synth section grid Cols()=%d, want 2..4 (adaptive, capped at 4)", c)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSynthSectionGridMultiColumn' -v
```

Expected: FAIL — `Cols()=1, want 2..4`.

- [x] **Step 3: Raise the mobile cap in `placeKnobsInSection`**

In `src/go/internal/ui/synth_panel_zone.go`, change the block at lines 1704–1712:

```go
	// Fewer, WIDER knobs per row so each cell has room for its purpose line +
	// mini-visual: cap the grid to 2 columns on desktop, 1 on mobile (the cap
	// overrides ControlGrid's 2-column floor). Overflow scrolls — the intended
	// "show a few, scroll for more" behaviour.
	maxCols := 2
	if mobile {
		maxCols = 1
	}
	grid.SetMaxCols(maxCols)
```

to:

```go
	// Desktop: cap at 2 columns so each cell has room for its purpose line +
	// mini-visual. Mobile: width-adaptive up to 4 columns (the pill buttons
	// self-clamp to narrower cells), so stage cards use the panel width
	// instead of one knob per row. Overflow scrolls — the intended
	// "show a few, scroll for more" behaviour.
	maxCols := 2
	if mobile {
		maxCols = 4
	}
	grid.SetMaxCols(maxCols)
```

- [x] **Step 4: Run test to verify it passes**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSynthSectionGridMultiColumn' -v
```

Expected: PASS.

- [x] **Step 5: Run the synth mobile tests and reconcile the ideal-size pin**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'MobileSynth|SynthTab|SynthKnob|SynthSection' -v 2>&1 | tail -60
```

Known likely casualty: `TestMobileSynthTab_KnobRendersAtIdealSize` (`synth_mobile_focus_test.go:180`) asserts `k.Rect().Dx() >= d.SynthKnobIdeal` (88 at Spacious). At 4 columns on a 390-px viewport the cell is 88 px without a scrollbar but ~85 px when the grid scrolls (scrollbar width is reserved after the column count is chosen). If and only if it fails, replace the final assertion with a width-aware one that preserves the test's original intent (the vertical content floor must not shrink the knob):

```go
	// Multi-column mobile: the dial band may be WIDTH-bound (cell = panel
	// width / columns, minus scrollbar), which is fine — the pill self-clamps.
	// What this test guards is the vertical content floor: the HEIGHT budget
	// must never be what shrinks the knob below ideal.
	want := d.SynthKnobIdeal
	if grid := dv.sectionGrid(sec.id); grid != nil && grid.Cols() > 1 {
		if cw := k.Rect().Dx(); cw < want {
			// Accept a width-bound dial only if it fills its cell.
			cell, ok := grid.CellRect(0)
			if !ok || cw < cell.Dx() {
				t.Fatalf("knob diameter %d < ideal %d and not width-bound (cell %v)", cw, want, cell)
			}
			return
		}
	}
	if k.Rect().Dx() < want {
		t.Fatalf("knob diameter %d < ideal %d (still shrinking)", k.Rect().Dx(), want)
	}
```

(Keep the original single assertion if the run is green — don't touch a passing test.)

Do NOT commit (standing user rule).

---

### Task 3: Full-package regression + visual verification

**Files:**
- Test: whole `internal/ui` package (no new files expected).

**Interfaces:**
- Consumes: Tasks 1–2 landed.
- Produces: green fast-path suite (minus pre-existing branch failures) + screenshots confirming the layout on mobile in EN and ES.

- [x] **Step 1: Run the full fast UI suite**

```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20
```

Expected: PASS, or failures limited to the known pre-existing set listed in Global Constraints. Any NEW failure must be fixed before proceeding — check first whether it encodes a single-column assumption (fix the test's intent, not just its numbers), and report anything ambiguous.

- [x] **Step 2: Capture mobile screenshots of both tabs**

```bash
cd /home/ymolinar/Repos/beatmo && make screenshot >/dev/null 2>&1 || true
./tmp/beatmo_screenshot -list-scenes | grep -i 'sampler\|synth'
```

Then capture the sampler + synth scenes found (crop scenes preferred), mobile pass included:

```bash
make screenshots-all SCENES=<comma-separated sampler+synth scenes from the list> MOBILE=1
```

- [x] **Step 3: Inspect the screenshots**

Read the produced PNGs under `screenshots/` (mobile variants) and verify:
- Sampler: 3–4 pill columns, all five knobs visible, group labels (TRIM/TUNE/LEVEL) legible per cell, no scrollbar on a full-height panel.
- Synth: selected stage card shows multiple pill columns; purpose gloss / concept mini-visual under each caption either fits or degrades cleanly (existing width guards) — no text painting outside its cell.
- Repeat the sampler check in Spanish (longest label "Ganancia +0 dB"): switch locale in the scene if the catalog supports it, or at minimum confirm via `TestSamplerKnobUsableSizeMobileAllLanguages` staying green + an EN screenshot showing pills clamp with room to spare.

If pills look cramped at 4 columns: fall back per spec — raise the effective ideal cell width at the two call sites (e.g. pass `dv2.SynthKnobIdeal + SpaceLG` as `cellIdealW`) so a 390-px phone settles at 3 columns, and re-run Tasks 1–2 test steps.

- [x] **Step 4: Report**

Summarize: column counts observed at 390 px, test deltas made, screenshot paths. Leave everything uncommitted.
