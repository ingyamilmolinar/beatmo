# Mobile Bottom-Pane Minimum Height Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On a mobile audio tab, size the drum/audio pane to comfortably fit its content (a full-size `SynthKnobIdeal` knob + chrome, un-overlapped sampler buttons) and no larger, while keeping the node graph always visible at ≥ 40% of screen height.

**Architecture:** The audio panel lives inside the drum pane, whose mobile height is set by `adaptiveMobilePortraitSplitY` (`game_layout.go`) from drum-row count — ignoring audio content. Add a `mobileAudioPanelMinContentH()` floor (synth-driven, the tallest tab) and, when `MobileEQMode()`, size the pane to `max(rows-need, audio-need)` capped at 60% of height (grid ≥ 40%); the pane takes only what the content needs.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/ui` only.

**Conventions (read once):**
- Bundled Go from `src/go`: `../../.tools/go/bin/go`. Test: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- **Do NOT `git add`/commit/stash/restore/`rm`/move tracked files** (repo policy). Each "Commit" step is a **Checkpoint**: run the test, leave changes in the tree, report.
- Active concurrent i18n/wave WIP intermittently flap-breaks the `-tags test` build in shared files (`synth_panel_zone.go`, `game_*`, `i18n_*`/`wave_*`/`freeze_*` tests), and a **background cleanup deletes `zz_*` scratch files** — do NOT rely on throwaway probe files; put assertions in the real test file. Verify ONLY your own tests with `-run`; trust `go test`/`go vet` over IDE diagnostics. If the build fails ONLY in a concurrent-WIP file, move it aside + restore unconditionally in one command. **Do the work, verify with `-run`, then STOP — never spawn background log-watch loops.**

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/internal/ui/game_layout.go` | mobile grid/drum split | Add `mobileAudioPanelMinContentH()`; audio-tab branch in `adaptiveMobilePortraitSplitY` |
| `src/go/internal/ui/game_mobile_split_test.go` | split-height tests | **Create** (`//go:build test`) |
| `src/go/internal/ui/synth_mobile_focus_test.go` | full-knob test | Add `TestMobileSynthTab_KnobRendersAtIdealSize` |
| `src/go/internal/ui/sampler_panel_zone_test.go` (or wherever `TestSamplerButtonsDoNotOverlapMobile` lives) | sampler overlap | Extend to assert button row clears the knob row |

---

## Task 1: `mobileAudioPanelMinContentH` + audio-tab split sizing

**Files:**
- Modify: `src/go/internal/ui/game_layout.go` (`adaptiveMobilePortraitSplitY:176-198`)
- Test: `src/go/internal/ui/game_mobile_split_test.go` (Create)

- [ ] **Step 1: Write the failing tests**

Create `src/go/internal/ui/game_mobile_split_test.go`:

```go
//go:build test

package ui

import "testing"

// mobileSplitGame builds a forced-mobile Game laid out at w×h on the given view
// mode, returning the Game so tests can call adaptiveMobilePortraitSplitY.
func mobileSplitGame(t *testing.T, w, h int, mode viewMode) *Game {
	t.Helper()
	g := newMobileSynthTabGameForTestSize(t, w, h) // existing helper: forces mobile + lays out synth
	g.drum.setViewMode(mode)
	g.Layout(w, h)
	return g
}

func TestMobileSplit_GridAtLeast40PercentOnAudioTabs(t *testing.T) {
	for _, h := range []int{720, 844} {
		for _, mode := range []viewMode{viewModeSynth, viewModeSampler} {
			g := mobileSplitGame(t, 390, h, mode)
			y := adaptiveMobilePortraitSplitY(h, g) // grid = [0, y], drum = [y, h]
			grid := y
			if grid < h*40/100 {
				t.Errorf("h=%d mode=%v: grid height %d < 40%% of %d", h, mode, grid, h)
			}
			if grid <= 0 {
				t.Errorf("h=%d mode=%v: grid not visible (height %d)", h, mode, grid)
			}
		}
	}
}

func TestMobileSplit_DoesNotOverExpand(t *testing.T) {
	// On a tall viewport the audio content needs less than 60%, so the drum pane
	// must equal the content need (grid keeps the surplus, strictly > 40%).
	h := 1280
	g := mobileSplitGame(t, 390, h, viewModeSynth)
	y := adaptiveMobilePortraitSplitY(h, g)
	drumH := h - y
	audioNeed := Profile().HeaderMaxH + mobileAudioPanelMinContentH() + TouchMinTarget()
	if drumH > audioNeed+2 { // allow rounding slack
		t.Errorf("drum pane %d over-expanded past content need %d", drumH, audioNeed)
	}
	if y <= h*40/100 {
		t.Errorf("tall viewport: grid %d should exceed 40%% (%d)", y, h*40/100)
	}
}

func TestMobileSplit_PadsUnchanged(t *testing.T) {
	// The Pads (rows) path must be byte-for-byte the rows-based value.
	h := 844
	g := mobileSplitGame(t, 390, h, viewModeRows)
	got := adaptiveMobilePortraitSplitY(h, g)
	// Recompute the legacy rows-based expectation inline.
	rh := TouchRowHeight()
	numRows := len(g.drum.Rows)
	headerH := Profile().HeaderMaxH
	barH := TouchMinTarget()
	needed := headerH + (numRows+1)*rh + barH
	maxDrum, minDrum := h*65/100, h*30/100
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	want := h - drumH
	if got != want {
		t.Errorf("Pads split changed: got %d want %d", got, want)
	}
}
```

> If `viewModeSampler` isn't the exact enum name, grep `grep -n "viewModeSynth\|viewModeSampler\|viewModeRows" internal/ui/*.go` and use the real identifiers. `newMobileSynthTabGameForTestSize(t, w, h)` exists (used by the mobile-synth tests).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_'`
Expected: `GridAtLeast40Percent` may already pass (current cap is 65% → grid ≥35%, could be <40% at some sizes → FAIL), `DoesNotOverExpand` FAILS (`mobileAudioPanelMinContentH` undefined → compile fail). Compile failure first.

- [ ] **Step 3: Add the helper + audio-tab branch**

In `game_layout.go`, add the helper above `adaptiveMobilePortraitSplitY`:

```go
// mobileAudioPanelMinContentH is the comfortable minimum CONTENT height (excluding
// the transport header + bottom action bar) for the mobile audio panel. Sized so
// the synth knob renders at its full SynthKnobIdeal diameter (not the shrunk
// SynthKnobMin): a ~2-row chip strip + the detail header + one full knob cell +
// section paddings. Synth is the tallest audio tab, so this single floor keeps
// every audio tab roomy (and the sampler control buttons clear of the knob labels).
func mobileAudioPanelMinContentH() int {
	d := Profile().DensityValues()
	chipStrip := 2 * d.SynthChipStripH                 // pipeline wraps to ~2 rows at phone width
	knobCell := d.SynthKnobIdeal + d.SynthKnobCaptionH // a FULL-size dial + its caption
	return chipStrip + d.SynthDetailHeaderH + knobCell + 4*SpaceSM
}
```

Then modify `adaptiveMobilePortraitSplitY`. The current body (lines ~186-197) is:

```go
	barH := TouchMinTarget() // bottom action bar height
	needed := headerH + (numRows+1)*rh + barH
	maxDrum := h * 65 / 100 // cap drum at 65% — drum is the primary edit surface
	minDrum := h * 30 / 100 // floor drum at 30%
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	return h - drumH
```

Replace it with:

```go
	barH := TouchMinTarget() // bottom action bar height
	needed := headerH + (numRows+1)*rh + barH
	maxDrum := h * 65 / 100 // cap drum at 65% — drum is the primary edit surface
	// On a mobile audio tab the drum pane hosts the synth/sampler panel, not the
	// drum rows — size it to fit that content comfortably (a full-size knob +
	// chrome) instead of the row count, so the knob never shrinks. Keep the grid
	// >= 40% (cap the drum at 60%); the pane never expands past what it needs.
	if g.drum != nil && g.drum.MobileEQMode() {
		if audioNeed := headerH + mobileAudioPanelMinContentH() + barH; audioNeed > needed {
			needed = audioNeed
		}
		maxDrum = h * 60 / 100 // grid always >= 40% on audio tabs
	}
	minDrum := h * 30 / 100 // floor drum at 30%
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	return h - drumH
```

(`g.drum.MobileEQMode()` is reachable — `g` is `*Game`. No `eqPanelZone` access needed; the single synth-driven floor applies to every audio tab.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_'`
Expected: all three PASS.

- [ ] **Step 5: vet + checkpoint**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -i game_layout`
Expected: empty. Report; do NOT commit.

---

## Task 2: Full-size synth knob + sampler buttons clear the knob row

**Files:**
- Modify: `src/go/internal/ui/synth_mobile_focus_test.go`
- Modify: the test file holding `TestSamplerButtonsDoNotOverlapMobile` (find via `grep -rln "TestSamplerButtonsDoNotOverlapMobile" internal/ui`)

- [ ] **Step 1: Write the full-knob test**

Append to `src/go/internal/ui/synth_mobile_focus_test.go`:

```go
func TestMobileSynthTab_KnobRendersAtIdealSize(t *testing.T) {
	// With the bottom-pane min height, a 390x844 mobile synth tab must render the
	// first knob at the FULL SynthKnobIdeal diameter, not the shrunk SynthKnobMin.
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	d := Profile().DensityValues()
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no section knobs")
	}
	k := dv.instEditorKnobs[sec.knobIdxs[0]]
	if k.Rect().Empty() {
		t.Fatalf("first knob not laid out")
	}
	if k.Rect().Dx() < d.SynthKnobIdeal {
		t.Fatalf("knob diameter %d < ideal %d (still shrinking)", k.Rect().Dx(), d.SynthKnobIdeal)
	}
}
```

> The knob's drawn rect includes the caption row in some code paths; if `k.Rect().Dx()` is the dial width it should equal `SynthKnobIdeal` when the cell is tall enough. If `Rect()` returns a square that includes the caption in `.Dy()`, assert on `.Dx()` (the horizontal dial diameter) as above. If at 844 the 60% cap still can't fit the ideal (it should: audioNeed ≈ 56+250+44 = 350 < 0.60×844 = 506), investigate the floor composition before weakening the assertion.

- [ ] **Step 2: Run it (should pass after Task 1)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSynthTab_KnobRendersAtIdealSize -v`
Expected: PASS (Task 1 gave the pane enough height). If FAIL with diameter == `SynthKnobMin`, the pane still isn't tall enough — verify `MobileEQMode()` is true in this helper (the synth tab is active) and that `mobileAudioPanelMinContentH` feeds the split; fix Task 1, don't weaken this test.

- [ ] **Step 3: Extend the sampler overlap test**

Open the file with `TestSamplerButtonsDoNotOverlapMobile`. It already asserts no two laid-out sampler buttons intersect. Add a sibling assertion (same setup, mobile, build the sampler tab at 390×844): the control-button row rect must NOT intersect the knob-row rect. Use the real accessors the sampler layout exposes — find the button rects (`dv.samplerButtonByTag("sampler-reverse").Rect()` etc.) and the knob rects (the sampler TRIM/TUNE/LEVEL knobs); assert the topmost button's `Min.Y >= the lowest knob caption's Max.Y` (buttons sit below the knob labels, not overlapping). If the existing test already builds the sampler tab, reuse its setup; otherwise mirror the mobile-sampler scene setup (`mobileSamplerTabSetup(true)` from scene_catalog.go) on a Game laid out at 390×844.

```go
func TestSamplerControlsClearKnobRowMobile(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844) // forces mobile; reuse as a mobile Game
	dv := g.drum
	mobileSamplerTabSetup(true)(g) // activate the Sampler tab (confirm the helper name)
	g.Layout(390, 844)
	g.Update()
	// Collect laid-out sampler control buttons.
	var lowestBtnTop = 1 << 30
	for _, tag := range []string{"sampler-reverse", "sampler-normalize", "sampler-fade", "sampler-preview"} {
		b := dv.samplerButtonByTag(tag)
		if b == nil || b.Rect().Empty() {
			continue
		}
		if b.Rect().Min.Y < lowestBtnTop {
			lowestBtnTop = b.Rect().Min.Y
		}
	}
	if lowestBtnTop == 1<<30 {
		t.Skip("no sampler control buttons laid out")
	}
	// The button row must start below the sampler knob captions (no overlap).
	knobBottom := dv.samplerKnobRowBottom() // add/confirm a test-reachable accessor for the knob row's Max.Y
	if lowestBtnTop < knobBottom {
		t.Fatalf("sampler buttons (top %d) overlap the knob row (bottom %d)", lowestBtnTop, knobBottom)
	}
}
```

> Confirm the real names: `samplerButtonByTag`, `mobileSamplerTabSetup`, and a way to get the knob row's bottom. If there's no `samplerKnobRowBottom()` accessor, compute the knob-row bottom from the laid-out sampler knob rects (the sampler stores its knobs similarly to the synth — grep `sampler.*[Kk]nob`, `samplerControls`). If exposing a clean accessor is simplest, add a tiny one in `sampler_panel_zone.go`. Keep the assertion meaningful (buttons strictly below the knob labels).

- [ ] **Step 4: Run the sampler test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSamplerButtonsDoNotOverlapMobile|TestSamplerControlsClearKnobRowMobile' -v`
Expected: PASS. If `TestSamplerControlsClearKnobRowMobile` fails, the sampler content still doesn't fit — bump `mobileAudioPanelMinContentH` (e.g. it must also reserve the sampler button row); since synth drives the single floor, add the button-row allowance if needed and re-run both Task 1 + Task 2 tests. Report the adjustment.

- [ ] **Step 5: vet + checkpoint** — `go vet -tags test ./internal/ui/` clean for your files. Report; do NOT commit.

---

## Task 3: Visual verification

**Files:** none (capture + confirm)

- [ ] **Step 1: Recapture the mobile synth tab**

Run (repo root): `make screenshots-all MOBILE=1 SCENES=mobile_bottom_nav_synth 2>&1 | tail -6`
Read `screenshots/all/mobile/mobile_bottom_nav_synth.browser.png`. Confirm: the node graph still occupies the top (≥ ~40% of the screen, clearly visible); the synth panel below now shows a **full-size dial** on the left (not a shrunken sliver) with both waves stacked on the right. Report what you see + the rough grid/panel proportion.

- [ ] **Step 2: Recapture the mobile sampler tab**

Run (repo root): `make screenshots-all MOBILE=1 SCENES=mobile_bottom_nav_sampler 2>&1 | tail -6`
Read `screenshots/all/mobile/mobile_bottom_nav_sampler.browser.png`. Confirm: the TRIM/TUNE/LEVEL knob sub-labels ("Start 0%", "Pitch +0 st") are clear of the Rev/Norm/Fade/Preview/Save row — no overlap, no clipped stray text. Report what you see.

- [ ] **Step 3: Regression + vet**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_|TestMobileSynthTab_|TestSampler|TestSynthChips_MobileWrap|TestSynthTab_KnobDragViaTreeDispatch_Mobile' 2>&1 | tail -8`
Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | head -5`
Expected: PASS; vet clean for your files. (Isolate any concurrent-WIP-broken test file via move-aside + restore if the build flaps.)

- [ ] **Step 4: Final checkpoint** — summarize: grid/panel proportions at 720/844, knob now `SynthKnobIdeal`, sampler buttons cleared, Pads unchanged, screenshots confirmed. Hand the working tree to the user. Do NOT commit.

---

## Self-Review — spec coverage

| Spec requirement | Task |
|---|---|
| Grid always visible ≥ 40% on audio tabs | 1 (`maxDrum = 60%`), test `GridAtLeast40Percent` |
| Don't over-expand (pane = content need) | 1 (`drumH = needed`, capped), test `DoesNotOverExpand` |
| `mobileAudioPanelMinH` from density tokens | 1 (`mobileAudioPanelMinContentH`) |
| Applies to audio-tab path only; Pads unchanged | 1 (`MobileEQMode()` gate), test `PadsUnchanged` |
| Full-size `SynthKnobIdeal` knob | 2 (`KnobRendersAtIdealSize`) |
| Sampler buttons clear the knob row | 2 (`SamplerControlsClearKnobRowMobile`) |
| Synth drives a single floor for all audio tabs | 1 (one helper, no per-tab branch) |
| Visual confirmation (synth + sampler) | 3 |
