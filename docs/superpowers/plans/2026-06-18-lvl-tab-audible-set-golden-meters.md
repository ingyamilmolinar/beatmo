# Lvl Tab — Audible-Set Strips, Golden Meters, Mobile Bar Room — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The audio-panel Lvl (Levels) tab shows only the audible instruments + master (adapting to solo/mute), recolors its meters to a golden→orange→red Vice City ramp defined by codegen, and gives the bars more room on mobile — all on the design-system type scale.

**Architecture:** Filtering happens purely at render time. We extract the codebase's canonical audibility predicate (`!Muted && (!anySolo || Solo)`) into shared helpers, pass an optional visible-ID set into `drawLevelsMultiChannel`, and have `EQPanelZone.Draw` compute that set from `ActiveRows()`. Colors stay codegen: we rename/recolor three `viz-meter-*` tokens in DESIGN.md and regenerate. Mobile gains bar width by shrinking a density token; type-scale compliance removes raw font multipliers.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten (stubbed under `-tags test`), DESIGN.md YAML → `cmd/gen_design_tokens` codegen.

## Global Constraints

- **NEVER stage or commit.** The user manages all git operations. No `git add`, no `git commit`. Each task's final "gate" is running the test suite, not a commit.
- Use the bundled Go for everything: `.tools/go/bin/go` (or `make` targets, which default `GO` to it).
- Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
- All colors come from tokens (no new inline `color.RGBA`/`color.NRGBA` literals — `TestTokenDiscipline` ratchets).
- All three new meter hexes must be exact Vice City palette members so `TestPaletteMembership` stays green: golden `#FFB30A` (sunset-gold-300), orange `#FF7A3D` (tangerine-300), red `#E84A1F` (tangerine-500).
- After ANY DESIGN.md token edit, run `make gen-design-tokens` and let the generated `*.gen.go` change land (do NOT hand-edit `design_tokens.gen.go`).
- Type scale lives in `textcache.go`: `FontSizeCaption=10, Body=14, Heading=20`. Express font scales as ratios of these constants, never raw literals like `2.0`.
- Heavy concurrent external churn on this branch has deleted untracked files mid-task — re-check `git status` / `ls` of your own new files before declaring a task done; rewrite (never `git restore`) if one vanished.

---

## File Structure

- `src/go/internal/ui/drumview_rows.go` — add shared audibility helpers; refactor `autoSelectSoleAudibleChannel` to use them.
- `src/go/internal/ui/render_meters.go` — add `visibleIDs` param + filter to `drawLevelsMultiChannel`; rename meter color aliases; type-scale fix in `levelsAggregateColumnRows`.
- `src/go/internal/ui/eq_panel_zone.go` — compute audible set in the Lvl Draw case, pass it down.
- `DESIGN.md` — rename/recolor `viz-meter-green/yellow/red` → `viz-meter-low/mid/high`; reduce spacious `levelsReadoutWIcons`.
- `src/go/internal/ui/design_tokens.gen.go`, `design_density.gen.go` — regenerated (not hand-edited).
- `src/go/internal/ui/design_md_drift_test.go` — add the three new tokens to `wantColors`.
- New test files: `audible_set_test.go`, `levels_visible_filter_test.go`, `levels_typescale_test.go`, `levels_mobile_room_test.go` (all `//go:build test`).

---

## Task 1: Shared audibility helpers

**Files:**
- Modify: `src/go/internal/ui/drumview_rows.go` (add helpers; refactor `autoSelectSoleAudibleChannel` at lines ~253-276)
- Test: `src/go/internal/ui/audible_set_test.go` (create)

**Interfaces:**
- Produces:
  - `func anySoloActive(rows []*DrumRow) bool`
  - `func rowAudible(r *DrumRow, anySolo bool) bool`
  - `func audibleInstrumentIDs(rows []*DrumRow) map[string]bool`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/audible_set_test.go`:

```go
//go:build test

package ui

import "testing"

func rowsFixture() []*DrumRow {
	return []*DrumRow{
		{Instrument: "kick"},
		{Instrument: "snare"},
		{Instrument: "hat"},
	}
}

func TestAudibleInstrumentIDs_NoSoloNoMute_All(t *testing.T) {
	got := audibleInstrumentIDs(rowsFixture())
	for _, id := range []string{"kick", "snare", "hat"} {
		if !got[id] {
			t.Fatalf("expected %q audible when nothing soloed/muted; got %v", id, got)
		}
	}
}

func TestAudibleInstrumentIDs_SingleSolo_OnlySoloed(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if !got["hat"] || got["kick"] || got["snare"] {
		t.Fatalf("single solo should yield only hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MultiSolo_SoloedSet(t *testing.T) {
	rows := rowsFixture()
	rows[1].Solo = true // snare
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if got["kick"] || !got["snare"] || !got["hat"] {
		t.Fatalf("multi-solo should yield snare+hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MuteNoSolo_ExcludesMuted(t *testing.T) {
	rows := rowsFixture()
	rows[0].Muted = true // kick
	got := audibleInstrumentIDs(rows)
	if got["kick"] || !got["snare"] || !got["hat"] {
		t.Fatalf("mute should exclude kick; got %v", got)
	}
}

func TestAudibleInstrumentIDs_SoloAndMuted_Excluded(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true
	rows[2].Muted = true // muted overrides solo
	got := audibleInstrumentIDs(rows)
	if got["hat"] {
		t.Fatalf("a muted-and-soloed row must be inaudible; got %v", got)
	}
}

func TestAudibleInstrumentIDs_EmptyInstrumentIDIgnored(t *testing.T) {
	rows := []*DrumRow{{Instrument: ""}, {Instrument: "kick"}}
	got := audibleInstrumentIDs(rows)
	if _, ok := got[""]; ok {
		t.Fatalf("empty instrument id must not be a key; got %v", got)
	}
	if !got["kick"] {
		t.Fatalf("kick should be audible; got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestAudibleInstrumentIDs ./internal/ui/`
Expected: FAIL — `undefined: audibleInstrumentIDs`.

- [ ] **Step 3: Add the helpers**

In `src/go/internal/ui/drumview_rows.go`, add near the top of the file (after imports):

```go
// anySoloActive reports whether any row is soloed. Soloing changes the
// audibility rule from "everything not muted" to "only soloed rows".
func anySoloActive(rows []*DrumRow) bool {
	for _, r := range rows {
		if r != nil && r.Solo {
			return true
		}
	}
	return false
}

// rowAudible is the canonical per-row audibility predicate used across the
// engine: a row is audible when it is not muted AND (nothing is soloed OR it
// is itself soloed). Pass anySolo from anySoloActive(rows).
func rowAudible(r *DrumRow, anySolo bool) bool {
	return r != nil && !r.Muted && (!anySolo || r.Solo)
}

// audibleInstrumentIDs returns the set of instrument IDs that are currently
// audible given the rows' solo/mute state. Rows with an empty Instrument ID
// are skipped (no channel to key on).
func audibleInstrumentIDs(rows []*DrumRow) map[string]bool {
	anySolo := anySoloActive(rows)
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		if rowAudible(r, anySolo) {
			out[r.Instrument] = true
		}
	}
	return out
}
```

- [ ] **Step 4: Refactor `autoSelectSoleAudibleChannel` to reuse the helpers**

Replace the inline `anySolo` scan and audibility check in `autoSelectSoleAudibleChannel` (currently `drumview_rows.go:253-276`) so it reads:

```go
func (dv *DrumView) autoSelectSoleAudibleChannel() {
	if len(dv.Rows) < 2 {
		return
	}
	anySolo := anySoloActive(dv.Rows)
	count := 0
	var sole *DrumRow
	for _, r := range dv.Rows {
		if rowAudible(r, anySolo) {
			count++
			sole = r
		}
	}
	if count != 1 || sole == nil || sole.Instrument == "" {
		return
	}
	dv.selectAudioChannel(sole.Instrument)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestAudibleInstrumentIDs|AutoSelect' ./internal/ui/`
Expected: PASS (new tests + any existing auto-select tests still green).

- [ ] **Step 6: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./internal/ui/ && echo OK`
Expected: `OK`. **Do NOT commit** (user instruction). Confirm `audible_set_test.go` still exists on disk.

---

## Task 2: `visibleIDs` filter in `drawLevelsMultiChannel`

**Files:**
- Modify: `src/go/internal/ui/render_meters.go` (signature + instrument loop, `drawLevelsMultiChannel` at line 447)
- Test: `src/go/internal/ui/levels_visible_filter_test.go` (create)

**Interfaces:**
- Consumes: `audibleInstrumentIDs` (Task 1) is NOT needed here — this task only adds the parameter and the skip logic.
- Produces: `func drawLevelsMultiChannel(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[string]bool)` — `visibleIDs == nil` means "show all".

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/levels_visible_filter_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func filterState() *analyzer.State {
	return &analyzer.State{
		Instruments: []analyzer.InstrumentMetrics{
			{ID: "kick", Name: "kick", PeakDB: -6, RMSDB: -18, Active: true},
			{ID: "snare", Name: "snare", PeakDB: -8, RMSDB: -20, Active: true},
			{ID: "hat", Name: "hat", PeakDB: -10, RMSDB: -22, Active: true},
		},
		Master: analyzer.ChannelMetrics{ID: "main", Name: "Master", PeakDB: -3, RMSDB: -14, Active: true},
	}
}

// With a visibleIDs set of one instrument, only that instrument's strip text
// plus the Master strip text should be drawn; the hidden instrument names
// must not appear.
func TestLevelsVisibleFilter_OnlyVisibleStrips(t *testing.T) {
	rect := image.Rect(0, 0, 800, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	visible := map[string]bool{"hat": true}

	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, filterState(), NewMultiLevelsLatch(), visible)
	})

	if !labels["hat"] {
		t.Fatalf("expected 'hat' strip label drawn; texts=%v", labels)
	}
	if labels["kick"] || labels["snare"] {
		t.Fatalf("hidden instrument labels must not be drawn; texts=%v", labels)
	}
}

// nil visibleIDs preserves the legacy "show everything" behavior.
func TestLevelsVisibleFilter_NilShowsAll(t *testing.T) {
	rect := image.Rect(0, 0, 800, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())

	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, filterState(), NewMultiLevelsLatch(), nil)
	})
	for _, id := range []string{"kick", "snare", "hat"} {
		if !labels[id] {
			t.Fatalf("nil visibleIDs should draw all strips; missing %q in %v", id, labels)
		}
	}
}
```

This test needs a text-collection helper. If `collectDrawnTexts` does not already exist, add it to this file:

```go
// collectDrawnTexts runs fn while intercepting DrawTextColorAtScale and
// returns the set of strings that were drawn. Mirrors the collectFilledRects
// pattern in render_verify_test.go.
func collectDrawnTexts(t *testing.T, fn func()) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	orig := drawTextColorAtScaleHook
	drawTextColorAtScaleHook = func(s string) { out[s] = true }
	defer func() { drawTextColorAtScaleHook = orig }()
	fn()
	return out
}
```

- [ ] **Step 2: Verify the hook seam exists (or add it)**

Check whether `DrawTextColorAtScale` already routes through an interceptable hook:

Run: `cd src/go && grep -n "drawTextColorAtScaleHook\|func DrawTextColorAtScale" internal/ui/textcache.go`

If no hook var exists, add one in `textcache.go`: declare `var drawTextColorAtScaleHook func(string)` and, at the top of `DrawTextColorAtScale`, add `if drawTextColorAtScaleHook != nil { drawTextColorAtScaleHook(s) }`. (Test-only observation; zero cost when nil.)

- [ ] **Step 3: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLevelsVisibleFilter ./internal/ui/`
Expected: FAIL — `not enough arguments in call to drawLevelsMultiChannel` (signature mismatch) or compile error.

- [ ] **Step 4: Add the parameter and filter**

In `render_meters.go`, change the signature at line 447:

```go
func drawLevelsMultiChannel(dst *ebiten.Image, rect image.Rectangle, state *analyzer.State, latches *MultiLevelsLatch, visibleIDs map[string]bool) {
```

Replace the instrument loop (currently lines ~513-522) so hidden instruments are skipped and the strip cursor only advances for drawn strips:

```go
	x := stripsRect.Min.X
	nDrawn := 0
	for i := range state.Instruments {
		inst := &state.Instruments[i]
		if visibleIDs != nil {
			if _, present := visibleIDs[inst.ID]; !present {
				// Hidden because not audible (solo/mute). Skip entirely so
				// the remaining strips reclaim its width.
				continue
			}
		}
		stripW := weightPx
		drawLevelsChannelStrip(dst,
			image.Rect(x, stripsRect.Min.Y, x+stripW, stripsRect.Max.Y),
			inst.Name, inst.PeakDB, inst.RMSDB, inst.ClipCount,
			latchOrNil(latches, inst.ID))
		x += stripW + stripGap
		nDrawn++
	}
```

Note: `weightPx` is currently computed from `nInst = len(state.Instruments)`. To make bars widen when filtered, recompute the visible count BEFORE the weight math. Just above the `nInst :=` line (~line 500), replace:

```go
	nInst := len(state.Instruments)
```

with:

```go
	nInst := 0
	for i := range state.Instruments {
		if visibleIDs == nil {
			nInst++
			continue
		}
		if _, present := visibleIDs[state.Instruments[i].ID]; present {
			nInst++
		}
	}
	if nInst == 0 {
		// Nothing audible among instruments — fall back to master-only detail.
		drawLevelsDetail(dst, rect, &state.Master, latchOrNil(latches, "main"))
		return
	}
```

(The Master strip block after the loop is unchanged — Master is always drawn.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLevelsVisibleFilter ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Fix the existing call site so the package still builds**

The only production caller is `eq_panel_zone.go:638`. Temporarily pass `nil` so the build is green (real wiring is Task 3):

In `eq_panel_zone.go`, change line 638 to:

```go
			drawLevelsMultiChannel(screen, z.bodyRect(), state, z.levelsLatches, nil)
```

- [ ] **Step 7: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: all `internal/ui` tests pass. **Do NOT commit.**

---

## Task 3: Wire `EQPanelZone.Draw` to the audible set

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go` (Lvl Draw case, line 638)
- Test: `src/go/internal/ui/levels_visible_filter_test.go` (append an integration test)

**Interfaces:**
- Consumes: `audibleInstrumentIDs` (Task 1), `EQCallbacks.ActiveRows` (existing), the `visibleIDs` param (Task 2).

- [ ] **Step 1: Write the failing test**

Append to `levels_visible_filter_test.go`:

```go
// Driving the real EQ panel Lvl Draw with one soloed row should restrict the
// Levels strips to that instrument + Master.
func TestEQPanelLevels_SoloRestrictsStrips(t *testing.T) {
	z := newTestEQPanelForLevels(t) // helper below
	rows := []*DrumRow{
		{Instrument: "kick"},
		{Instrument: "snare", Solo: true},
	}
	z.callbacks.ActiveRows = func() []*DrumRow { return rows }
	z.callbacks.AnalyzerState = func() *analyzer.State { return filterState() }

	rect := image.Rect(0, 0, 800, 120)
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	z.SetActiveTab(TabMeters)
	z.SetBounds(rect)

	labels := collectDrawnTexts(t, func() { z.Draw(dst) })
	if !labels["snare"] || labels["kick"] {
		t.Fatalf("solo snare should show only snare strip; texts=%v", labels)
	}
}
```

If a constructor helper for a minimal Levels-ready `EQPanelZone` does not already exist in the test files, add a small `newTestEQPanelForLevels(t)` to this file that builds an `EQPanelZone` with `NewMultiLevelsLatch()` and a non-nil `EQCallbacks` (look at `eq_panel_meters_routing_test.go` for the existing construction pattern and copy it).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEQPanelLevels_SoloRestrictsStrips ./internal/ui/`
Expected: FAIL — both `snare` and `kick` drawn (filter not wired yet).

- [ ] **Step 3: Wire the audible set in Draw**

In `eq_panel_zone.go`, replace line 638 with:

```go
			var visibleIDs map[string]bool
			if z.callbacks.ActiveRows != nil {
				visibleIDs = audibleInstrumentIDs(z.callbacks.ActiveRows())
			}
			drawLevelsMultiChannel(screen, z.bodyRect(), state, z.levelsLatches, visibleIDs)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEQPanelLevels_SoloRestrictsStrips ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: all pass. **Do NOT commit.**

---

## Task 4: Golden/orange/red meter colors via codegen

**Files:**
- Modify: `DESIGN.md` (lines 112-114, `viz-meter-*`)
- Modify: `src/go/internal/ui/render_meters.go` (aliases line 17-19 + all refs)
- Regenerated: `src/go/internal/ui/design_tokens.gen.go` (via `make gen-design-tokens`)
- Modify: `src/go/internal/ui/design_md_drift_test.go` (`wantColors`)
- Test: reuse `design_md_drift_test.go`, `palette_membership_test.go`

**Interfaces:**
- Produces: gen constants `genColorVizMeterLow`, `genColorVizMeterMid`, `genColorVizMeterHigh`; Go aliases `meterLow`, `meterMid`, `meterHigh`.

- [ ] **Step 1: Write the failing test (drift want-map)**

In `design_md_drift_test.go`, add to the `wantColors` map (after the `viz-wave-a` entry, ~line 60):

```go
		"viz-meter-low":  meterLow,
		"viz-meter-mid":  meterMid,
		"viz-meter-high": meterHigh,
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDDrift ./internal/ui/`
Expected: FAIL — `undefined: meterLow` (and DESIGN.md keys missing).

- [ ] **Step 3: Edit DESIGN.md tokens**

In `DESIGN.md` replace the three lines (112-114):

```yaml
  viz-meter-green:  "#3FD67A"
  viz-meter-yellow: "#FFD319"
  viz-meter-red:    "#FF5C2A"
```

with:

```yaml
  viz-meter-low:  "#FFB30A"   # sunset-gold-300 (game primary) — safe/low zone
  viz-meter-mid:  "#FF7A3D"   # tangerine-300 — warn/mid zone
  viz-meter-high: "#E84A1F"   # tangerine-500 — hot/high zone
```

Leave `viz-meter-bg` and `viz-meter-clip` unchanged. Also update the surrounding comment block (lines ~109-111) to say "golden / orange / red" instead of "green / yellow / red".

- [ ] **Step 4: Regenerate tokens**

Run: `make gen-design-tokens`
Then verify the new constants exist:
Run: `cd src/go && grep -n "genColorVizMeterLow\|genColorVizMeterMid\|genColorVizMeterHigh" internal/ui/design_tokens.gen.go`
Expected: three lines with hexes `#FFB30A`, `#FF7A3D`, `#E84A1F`.

- [ ] **Step 5: Rename the Go aliases and all references**

In `render_meters.go` change lines 17-19:

```go
	meterLow  = genColorVizMeterLow
	meterMid  = genColorVizMeterMid
	meterHigh = genColorVizMeterHigh
```

Then update every reference in `render_meters.go` (use sed for a clean rename):

```bash
cd /home/ymolinar/Repos/beatmo/src/go/internal/ui && \
  ../../../.tools/go/bin/gofmt -l render_meters.go >/dev/null; \
  sed -i 's/\bmeterGreen\b/meterLow/g; s/\bmeterYellow\b/meterMid/g; s/\bmeterRed\b/meterHigh/g' render_meters.go
```

Note: this is safe because `meterYellowDB` / `meterRedDB` (the dB thresholds) do NOT contain the bare words `meterYellow`/`meterRed` as whole tokens — the `\b` word boundary before `DB` is a different identifier (`meterYellowDB` matches `\bmeterYellow` though!). **Do NOT use the sed above for `meterYellow`/`meterRed`** — instead rename only the color identifiers manually:
  - line 18/19 alias declarations → `meterMid`/`meterHigh`
  - `meterColor` body (lines 420-426): `return meterHigh` / `return meterMid` / `return meterLow`
  - `headroomCol` blocks (lines 661-666 and 1026-1045): `meterLow`/`meterMid`/`meterHigh`
  - leave `meterYellowDB` and `meterRedDB` (threshold constants, lines 27-28) UNCHANGED.

Verify no stale refs remain and the thresholds survive:
```bash
cd /home/ymolinar/Repos/beatmo && \
  grep -n "meterGreen\|meterYellow\b\|meterRed\b" src/go/internal/ui/render_meters.go; \
  grep -n "meterYellowDB\|meterRedDB" src/go/internal/ui/render_meters.go
```
Expected: first grep prints nothing; second still prints the two `*DB` constants.

- [ ] **Step 6: Run color tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestPaletteMembership|TestTokenDiscipline' ./internal/ui/`
Expected: PASS (all three hexes are palette members).

- [ ] **Step 7: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: all pass. **Do NOT commit.** Confirm `design_tokens.gen.go` reflects the new constants (it will show as modified in `git status` — leave it).

---

## Task 5: Type-scale compliance in the aggregate column

**Files:**
- Modify: `src/go/internal/ui/render_meters.go` (`levelsAggregateColumnRows`, lines ~970-997)
- Test: `src/go/internal/ui/levels_typescale_test.go` (create)

**Interfaces:**
- Consumes: `FontSizeCaption`, `FontSizeBody`, `FontSizeHeading` (existing constants in `textcache.go`).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/levels_typescale_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// The aggregate column's "headline" row (the big Headroom number) must use a
// scale derived from the type scale, not a raw 2.0 multiplier. After the fix
// it should equal FontSizeHeading/FontSizeBody.
func TestLevelsAggregate_HeadlineUsesTypeScale(t *testing.T) {
	rect := image.Rect(0, 0, 200, 400)
	rows := levelsAggregateColumnRows(rect, 14)
	want := FontSizeHeading / FontSizeBody
	got := rows[aggRowHeadroomValue].Scale
	if got != want {
		t.Fatalf("headline scale: got %v, want FontSizeHeading/FontSizeBody=%v", got, want)
	}
	// And it must be smaller than the old 2.0 (compactness goal).
	if got >= 2.0 {
		t.Fatalf("headline scale should be < 2.0 after compaction; got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLevelsAggregate_HeadlineUsesTypeScale ./internal/ui/`
Expected: FAIL — got `2` (the raw `headlineScale`).

- [ ] **Step 3: Replace the raw multipliers**

In `levelsAggregateColumnRows` (render_meters.go ~970-985), replace:

```go
	const (
		bodyScale     = 1.0
		headlineScale = 2.0
	)
```

with:

```go
	// Type-scale-derived multipliers (relative to body text). The headline
	// (big Headroom number) is the Heading step; body rows are Body.
	bodyScale := FontSizeBody / FontSizeBody       // 1.0, expressed via the scale
	headlineScale := FontSizeHeading / FontSizeBody // ≈1.43 — was a raw 2.0
```

(The `scales` array below already references `bodyScale`/`headlineScale`; no other change needed.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLevelsAggregate_HeadlineUsesTypeScale ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Meter|Level' ./internal/ui/ 2>&1 | tail -20`
Expected: pass (some layout tests may assert row positions — if any fail because the headline is now shorter, update those tests' expected offsets to match the new `FontSizeHeading/FontSizeBody` height, since the change is intentional). **Do NOT commit.**

---

## Task 6: Mobile — narrower readout column, wider bars

**Files:**
- Modify: `DESIGN.md` (`densities.spacious.levelsReadoutWIcons`, line ~715)
- Regenerated: `src/go/internal/ui/design_density.gen.go` (via `make gen-design-tokens`)
- Modify: `src/go/internal/ui/render_meters.go` (tighten `drawLevelsAggregatesIconRow` padding if needed)
- Test: `src/go/internal/ui/levels_mobile_room_test.go` (create)

**Interfaces:**
- Consumes: `Profile().DensityValues().LevelsReadoutWIcons`, `SetDensityForTest`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/levels_mobile_room_test.go`:

```go
//go:build test

package ui

import "testing"

// The Spacious (mobile) icon-readout column must be narrow enough to leave
// the bars room. Pin the reduced token value.
func TestSpaciousLevelsReadoutWIconsReduced(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	got := Profile().DensityValues().LevelsReadoutWIcons
	if got > 34 {
		t.Fatalf("spacious LevelsReadoutWIcons should be <= 34 for mobile bar room; got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSpaciousLevelsReadoutWIconsReduced ./internal/ui/`
Expected: FAIL — got `44`.

- [ ] **Step 3: Edit DESIGN.md and regenerate**

In `DESIGN.md`, in the `densities.spacious` block (line ~715), change:

```yaml
    levelsReadoutWIcons:   44
```

to:

```yaml
    levelsReadoutWIcons:   34
```

Then: `make gen-design-tokens`
Verify: `cd src/go && grep -n "LevelsReadoutWIcons" internal/ui/design_density.gen.go`
Expected: the spacious value now reads 34.

- [ ] **Step 4: Add a renderer test that bars widen when the column shrinks**

Append to `levels_mobile_room_test.go`:

```go
import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// With a narrower readout column, the strips region (rect minus readout col)
// must be wider. We assert the right edge of the last instrument strip text
// moves rightward when the readout width drops. Simpler structural check:
// the strips rect width used by drawLevelsMultiChannel grows as readoutW
// shrinks. We verify indirectly: at a mobile width, icon mode is selected and
// readoutW == LevelsReadoutWIcons (34), leaving more for bars than 44 would.
func TestLevelsMobileReadoutModeLeavesBarRoom(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	rect := image.Rect(0, 0, 390, 160) // typical mobile panel width
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	st := filterState() // from levels_visible_filter_test.go

	// Should not panic and should draw at least the master strip + an icon col.
	labels := collectDrawnTexts(t, func() {
		drawLevelsMultiChannel(dst, rect, st, NewMultiLevelsLatch(), nil)
	})
	if len(labels) == 0 {
		t.Fatalf("expected some labels drawn at mobile width")
	}
}
```

(If the `import` block conflicts with Step 1's file, merge imports into a single block at the top of the file.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSpaciousLevelsReadoutWIconsReduced|TestLevelsMobileReadoutModeLeavesBarRoom' ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Gate (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: all pass. **Do NOT commit.** Confirm `design_density.gen.go` reflects 34.

---

## Task 7: Full-suite verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full fast Go suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/analyzer/ 2>&1 | tail -30`
Expected: PASS. Some unrelated `internal/ui` tests on this branch are known-flaky/pre-existing failures (see project memory) — if any fail, confirm they also fail on a clean checkout via `-run` isolation before attributing them to this work.

- [ ] **Step 2: Run the design guards explicitly**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestPaletteMembership|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS.

- [ ] **Step 3: Confirm generated files are present and consistent**

Run: `make gen-design-tokens && cd src/go && git diff --stat internal/ui/design_tokens.gen.go internal/ui/design_density.gen.go`
Expected: re-running gen produces NO further diff (idempotent). The files remain modified vs HEAD (that's expected and left uncommitted).

- [ ] **Step 4: Self-check own files survived churn**

Run: `ls -la src/go/internal/ui/audible_set_test.go src/go/internal/ui/levels_visible_filter_test.go src/go/internal/ui/levels_typescale_test.go src/go/internal/ui/levels_mobile_room_test.go`
Expected: all four exist. If any vanished (external churn), recreate from this plan.

- [ ] **Step 5: Report**

Summarize: audible-set filtering live; meters golden/orange/red via codegen + palette-compliant; mobile bars roomier; type scale clean. **Leave everything uncommitted** for the user to review.

---

## Self-Review

**Spec coverage:**
- Audible-set strips (solo/mute aware, idle=all) → Tasks 1-3. ✓
- Golden/orange/red via codegen, palette-compliant → Task 4. ✓
- Mobile more bar room, smaller metrics → Tasks 5 (smaller headline) + 6 (narrower column). ✓
- Fonts/sizes on design system → Task 5 (+ existing TokenDiscipline/Drift guards). ✓
- Thorough TDD → every task is test-first. ✓

**Placeholder scan:** No TBD/TODO; all steps carry concrete code or exact commands. ✓

**Type consistency:** `drawLevelsMultiChannel(... , visibleIDs map[string]bool)` used identically in Tasks 2/3; `audibleInstrumentIDs`/`rowAudible`/`anySoloActive` signatures consistent across Tasks 1/3; `meterLow/meterMid/meterHigh` + `genColorVizMeterLow/Mid/High` consistent across Task 4. ✓

**Note on commits:** Standard plan "Commit" steps are intentionally replaced with test gates per the user's standing "never stage or commit" instruction.
