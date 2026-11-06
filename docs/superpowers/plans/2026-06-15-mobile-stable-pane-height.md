# Mobile Stable Pane Height Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the mobile bottom-pane height tab-independent (computed once per `Layout` at startup + resize, identical across all tabs so it never resizes on tab switch) and row-snapped (the drum rack shows whole rows, no half-blank strip).

**Architecture:** In `adaptiveMobilePortraitSplitY` (`game_layout.go`), drop the `MobileEQMode()` gate so the pane height is `max(rowsNeed, audioNeed)` capped at 60% of screen for every tab, then snap the height so the rack region (`drumH - headerH - barH`) is a whole multiple of the row height (bump up within the cap, else trim). It runs inside `Game.Layout`, so it's applied at startup and on resize.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `internal/ui` only.

**Conventions (read once):**
- Bundled Go from `src/go`: `../../.tools/go/bin/go`. Test: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.
- **Do NOT `git add`/commit/stash/restore/`rm`/move tracked files** (repo policy). Each "Commit" step is a **Checkpoint**: run the test, leave changes in the tree, report.
- Active concurrent i18n/wave WIP intermittently flap-breaks the `-tags test` build in shared files, and a **background cleanup deletes `zz_*` scratch files** — do NOT create throwaway probe files; put assertions in real test files. Verify ONLY your own tests with `-run`; trust `go test`/`go vet` over IDE diagnostics. If the build fails ONLY in a concurrent-WIP test file, move it aside + restore unconditionally in one command (`B=$(go vet -tags test ./internal/ui/ 2>&1 | grep -oE 'internal/ui/[a-z0-9_]+_test\.go' | sed 's#internal/ui/##' | sort -u); for f in $B; do mv internal/ui/$f /tmp/$f.bak; done; <run>; for f in $B; do mv /tmp/$f.bak internal/ui/$f; done`). **Do the work, verify with `-run`, then STOP — no background log-watch loops.**

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/internal/ui/game_layout.go` | mobile split | Unify `adaptiveMobilePortraitSplitY` (drop gate); add `snapDrumHToWholeRows` |
| `src/go/internal/ui/game_mobile_split_test.go` | split tests | Replace `PadsUnchanged` with `StableAcrossTabs`; add `AppliedAtStartupAndOnResize`, `RackShowsWholeRows`; extend `GridAtLeast40Percent` to all view modes |

---

## Task 1: Unify the height (tab-independent) + the stability tests

**Files:**
- Modify: `src/go/internal/ui/game_layout.go` (`adaptiveMobilePortraitSplitY`)
- Test: `src/go/internal/ui/game_mobile_split_test.go`

- [ ] **Step 1: Replace `PadsUnchanged` with the stability test + add startup/resize test**

In `game_mobile_split_test.go`: DELETE `TestMobileSplit_PadsUnchanged` (Pads now intentionally shares the unified height). Add:

```go
func TestMobileSplit_StableAcrossTabs(t *testing.T) {
	modes := []viewMode{viewModeRows, viewModeSynth, viewModeSampler, viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain}
	for _, h := range []int{720, 844} {
		var first int
		for i, mode := range modes {
			g := mobileSplitGame(t, 390, h, mode)
			y := adaptiveMobilePortraitSplitY(h, g)
			if i == 0 {
				first = y
				continue
			}
			if y != first {
				t.Errorf("h=%d: split differs by tab — %v gives %d, want %d (stable across tabs)", h, mode, y, first)
			}
		}
	}
}

func TestMobileSplit_AppliedAtStartupAndOnResize(t *testing.T) {
	// A fresh forced-mobile Layout sets the stable split immediately (not h/2),
	// and a resize recomputes it for the new height.
	g := newMobileSynthTabGameForTestSize(t, 390, 844) // forces mobile + auto-size + Layout
	want844 := adaptiveMobilePortraitSplitY(844, g)
	if g.split.Y != want844 {
		t.Fatalf("startup split.Y=%d, want stable %d (not applied at Layout)", g.split.Y, want844)
	}
	if g.split.Y == 844/2 {
		t.Fatalf("split is still the default 0.5 (%d) — stable height not applied at startup", g.split.Y)
	}
	g.Layout(390, 720)
	want720 := adaptiveMobilePortraitSplitY(720, g)
	if g.split.Y != want720 {
		t.Fatalf("after resize split.Y=%d, want %d (not recomputed on resize)", g.split.Y, want720)
	}
}
```

> Confirm `viewModeEQ/Wave/Spectrum/Meters/Chain` are the real enum names (`grep -n "viewMode[A-Z]" internal/ui/drumview.go`). If a tab uses a different identifier (e.g. `viewModeLevels` vs `viewModeMeters`), use the real one. `g.split.Y` is the split boundary (grid = `[0, split.Y]`); confirm `g.split` is reachable from the test (same package).

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_StableAcrossTabs|TestMobileSplit_AppliedAtStartupAndOnResize'`
Expected: `StableAcrossTabs` FAILS (today Pads differs from audio tabs because of the `MobileEQMode()` gate). `AppliedAtStartupAndOnResize` may pass or fail depending on current behavior — note which.

- [ ] **Step 3: Unify `adaptiveMobilePortraitSplitY` (drop the gate)**

Read the current `adaptiveMobilePortraitSplitY` body (a prior task modified it with a `MobileEQMode()`-gated branch that set `needed = audioNeed` and relaxed `minDrum`). Replace everything from the `barH := TouchMinTarget()` line to the final `return h - drumH` with this tab-independent version:

```go
	barH := TouchMinTarget() // bottom action bar height
	rowsNeed := headerH + (numRows+1)*rh + barH
	// The pane height is tab-INDEPENDENT: it fits both the drum rows AND the
	// tallest audio tab's content, so switching tabs never resizes the panel.
	// Computed in Layout → applied at startup and on every window resize.
	audioNeed := headerH + mobileAudioPanelMinContentH() + barH
	needed := rowsNeed
	if audioNeed > needed {
		needed = audioNeed
	}
	maxDrum := h * 60 / 100 // grid always >= 40% on EVERY tab
	minDrum := h * 30 / 100
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	// Snap so the rack shows whole rows (no half-blank strip): bump up to the next
	// full row when it fits under the 40%-grid cap, else trim down to whole rows.
	drumH = snapDrumHToWholeRows(drumH, headerH, barH, rh, maxDrum)
	return h - drumH
```

Remove any now-unused `g.drum.MobileEQMode()` reference in this function. (`g` may now be unused if nothing else in the function reads it — keep the `g *Game` parameter for `numRows`/the signature; `numRows` already reads `g.drum`.)

- [ ] **Step 4: Add `snapDrumHToWholeRows`**

Add near `adaptiveMobilePortraitSplitY` in `game_layout.go`:

```go
// snapDrumHToWholeRows adjusts the drum-pane height so its rack region (the area
// below the transport header and above the bottom action bar) holds a WHOLE number
// of drum rows — no partial-row blank strip. Rounds UP to include the next full row
// when that still fits under maxDrum; otherwise rounds DOWN to the last whole row.
// Returns drumH unchanged when the rack is already row-aligned or too small for a row.
func snapDrumHToWholeRows(drumH, headerH, barH, rh, maxDrum int) int {
	if rh <= 0 {
		return drumH
	}
	rackH := drumH - headerH - barH
	if rackH < rh {
		return drumH
	}
	rem := rackH % rh
	if rem == 0 {
		return drumH
	}
	if up := drumH + (rh - rem); up <= maxDrum {
		return up
	}
	return drumH - rem
}
```

- [ ] **Step 5: Run the stability tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_StableAcrossTabs|TestMobileSplit_AppliedAtStartupAndOnResize'`
Expected: both PASS.

- [ ] **Step 6: Add the whole-rows test**

Append to `game_mobile_split_test.go`:

```go
func TestMobileSplit_RackShowsWholeRows(t *testing.T) {
	rh := TouchRowHeight()
	headerH := Profile().HeaderMaxH
	barH := TouchMinTarget()
	for _, h := range []int{640, 720, 844, 960} {
		g := mobileSplitGame(t, 390, h, viewModeRows)
		y := adaptiveMobilePortraitSplitY(h, g)
		drumH := h - y
		rackH := drumH - headerH - barH
		if rackH >= rh && rackH%rh != 0 {
			t.Errorf("h=%d: rack %d not a whole number of rows (rh=%d, rem=%d)", h, rackH, rh, rackH%rh)
		}
		if y < h*40/100 {
			t.Errorf("h=%d: snap pushed grid below 40%% (grid=%d)", h, y)
		}
	}
}
```

> This recomputes `headerH`/`barH`/`rh` the same way `adaptiveMobilePortraitSplitY` does. If the function derives `headerH` differently (it has a `if headerH <= 0 { headerH = mobileHeaderH }` fallback), mirror that fallback here so the rack math matches. Run it; it must PASS.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSplit_RackShowsWholeRows`
Expected: PASS.

- [ ] **Step 7: vet + checkpoint**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | grep -i game_layout`
Expected: empty. Report; do NOT commit.

---

## Task 2: Reconcile the existing split + content tests

**Files:**
- Modify: `src/go/internal/ui/game_mobile_split_test.go` (extend `GridAtLeast40Percent`)

- [ ] **Step 1: Extend `GridAtLeast40Percent` to every tab (incl. Pads)**

`TestMobileSplit_GridAtLeast40PercentOnAudioTabs` currently loops `viewModeSynth`/`viewModeSampler`. Since the cap now applies to all tabs, broaden it (rename to `TestMobileSplit_GridAtLeast40PercentAllTabs`) to also include `viewModeRows` and the analysis tabs:

```go
func TestMobileSplit_GridAtLeast40PercentAllTabs(t *testing.T) {
	modes := []viewMode{viewModeRows, viewModeSynth, viewModeSampler, viewModeEQ, viewModeWave, viewModeSpectrum, viewModeMeters, viewModeChain}
	for _, h := range []int{720, 844} {
		for _, mode := range modes {
			g := mobileSplitGame(t, 390, h, mode)
			y := adaptiveMobilePortraitSplitY(h, g)
			if y < h*40/100 {
				t.Errorf("h=%d mode=%v: grid %d < 40%% of %d", h, mode, y, h)
			}
			if y <= 0 {
				t.Errorf("h=%d mode=%v: grid not visible", h, mode)
			}
		}
	}
}
```

(Use the real enum names confirmed in Task 1.)

- [ ] **Step 2: Run the full split + content suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_|TestMobileSynthTab_KnobRendersAtIdealSize|TestSamplerControlsClearKnobRowMobile|TestDoesNotOverExpand' 2>&1 | tail -15`
Expected: PASS for all — `StableAcrossTabs`, `AppliedAtStartupAndOnResize`, `RackShowsWholeRows`, `GridAtLeast40PercentAllTabs`, `DoesNotOverExpand`, `KnobRendersAtIdealSize`, `SamplerControlsClearKnobRowMobile`. If `DoesNotOverExpand` now fails because the snap bumps `drumH` above `audioNeed` by up to one row, that's expected/acceptable — update its tolerance to allow `+rh` (one row of snap slack) with a one-line comment, OR assert `drumH <= max(audioNeed, rowsNeed) rounded up to a whole row`. Keep the intent: the pane isn't arbitrarily larger than the tallest-tab content (plus at most one snap row).

- [ ] **Step 3: Regression — other mobile-synth tests still green**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSynthTab_|TestSynthChips_MobileWrap|TestSynthTab_KnobDragViaTreeDispatch_Mobile' 2>&1 | tail -8`
Expected: PASS. (These use `newMobileSynthTabGameForTestSize`, which drives the real split.)

- [ ] **Step 4: vet + checkpoint** — `go vet -tags test ./internal/ui/` clean for your files. Report; do NOT commit.

---

## Task 3: Visual verification (no resize across tabs, clean rows)

**Files:** none (capture + confirm)

- [ ] **Step 1: Recapture the three mobile tabs**

Run (repo root): `make screenshots-all MOBILE=1 SCENES=mobile_bottom_nav_pads,mobile_bottom_nav_synth,mobile_bottom_nav_sampler 2>&1 | tail -6`
Read all three PNGs under `screenshots/all/mobile/`. Confirm:
- The **grid/pane split is at the same Y** in all three (no resize between Pads, Synth, Sampler) — eyeball the boundary between the node graph and the bottom panel; it should sit at the same height.
- **Pads**: the drum rack shows whole rows — no thin half-row blank strip at the bottom of the rack.
- **Synth**: full-size dial on the left, waves on the right.
- **Sampler**: control buttons clear of the knob labels.
Report what you see per image (especially whether the split Y matches across the three).

- [ ] **Step 2: Final regression + vet**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMobileSplit_|TestMobileSynthTab_|TestSampler|TestSynthChips_MobileWrap' 2>&1 | tail -6`
Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ 2>&1 | head -5`
Expected: PASS; vet clean for your files.

- [ ] **Step 3: Final checkpoint** — summarize: split Y identical across Pads/Synth/Sampler (no resize), rack shows whole rows, grid ≥ 40% everywhere, knob ideal + sampler clear still hold. Hand the working tree to the user. Do NOT commit.

---

## Self-Review — spec coverage

| Spec requirement | Task |
|---|---|
| Remove per-tab gate; one tab-independent height | 1 Step 3 |
| `max(rowsNeed, audioNeed)`, 60% cap all tabs | 1 Step 3 |
| Applied at startup + resize (in Layout) | 1 (`AppliedAtStartupAndOnResize`) |
| Stable across tab switch | 1 (`StableAcrossTabs`) |
| Row-snap (whole rows, no half-blank) | 1 Step 4 (`snapDrumHToWholeRows`), test `RackShowsWholeRows` |
| Grid ≥ 40% on every tab incl. Pads | 2 (`GridAtLeast40PercentAllTabs`) |
| Replace old `PadsUnchanged` test | 1 Step 1 |
| Knob ideal + sampler-clear still hold | 2 Step 2/3 |
| Visual: no resize, clean rows | 3 |
