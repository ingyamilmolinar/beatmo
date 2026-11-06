# Grid Pop-up Unification — P0+P1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one shared grid-aware anchored-popup helper and use it to make the Settings overlay and Logs panel drop from their grid-pane buttons instead of being pinned into the bottom drum pane.

**Architecture:** A pure positioner `anchoredPopupRect(anchor, w, h, winBounds, opts)` reuses the existing `AnchorPopupRect` (flip/clamp) + `avoidReservedRect` (exclusion zones). A thin `(*Game).gridAnchoredPopupRect` wrapper supplies the full window as the clamp box and the gear cluster as an Avoid zone. Settings gains an injectable positioner; Logs' `panelRect` calls the wrapper. Neither overlay's size or content changes — only its origin.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten v2, ebitenstub `-tags test` harness.

**Scope:** This plan covers spec phases **P0** (shared helper) and **P1** (Settings + Logs anchoring) from `docs/superpowers/specs/2026-07-20-grid-popup-unification-design.md`. P2 (group menu), P3 (node sidebar), P4 (long-press popup) are separate follow-on plans. The File/overflow menu already anchors correctly; refactoring it onto the helper is deferred (it right-aligns to the button rather than left-aligns, a nuance not needed to fix the reported bug).

## Global Constraints

- **Go binary:** always `../../.tools/go/bin/go` (run from `src/go`), never system `go`.
- **Fast test command:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...`
- **Never stage or commit.** Per project convention the agent does NOT run `git add`/`git commit`. Each task ends at green tests + (for UI) an inspected screenshot; the user commits.
- **Mandatory UI verification:** any change that alters what renders MUST be verified by capturing a real screenshot scene and opening the PNG (desktop + `MOBILE=1`). Green Go tests are not sufficient for UI.
- **No new color literals.** Add no `color.RGBA{}`/`color.NRGBA{}` literals (token-discipline ratchet). This plan adds none.
- **Coordinate space:** all popup geometry is screen-space; the grid pane's origin is screen `(0,0)` (`Splitter.GridRect`). `winBounds` is always `image.Rect(0, 0, g.winW, g.winH)`.

---

### Task 1: Pure `anchoredPopupRect` helper + `GridAnchorOpts`

**Files:**
- Modify: `src/go/internal/ui/overlay_anchor.go` (append after `ClampRectInto`)
- Test: `src/go/internal/ui/overlay_anchor_test.go` (append)

**Interfaces:**
- Consumes: `AnchorPopupRect` (`overlay_anchor.go:35`), `avoidReservedRect` (`grid_reserved_zone.go:44`).
- Produces: `type GridAnchorOpts struct { Prefer PopupSide; Avoid []image.Rectangle }` and `func anchoredPopupRect(anchor image.Rectangle, w, h int, winBounds image.Rectangle, opts GridAnchorOpts) image.Rectangle`.

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/overlay_anchor_test.go`:

```go
func TestAnchoredPopupRectDropsBelowAnchor(t *testing.T) {
	win := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(1180, 10, 1210, 40) // top-right button
	got := anchoredPopupRect(anchor, 200, 300, win, GridAnchorOpts{})
	if got.Min.Y < anchor.Max.Y {
		t.Fatalf("popup should drop below anchor; got %v (anchor %v)", got, anchor)
	}
	if !got.In(win) {
		t.Fatalf("popup escaped window: %v", got)
	}
}

func TestAnchoredPopupRectClearsAvoidZone(t *testing.T) {
	win := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(1180, 10, 1210, 40)
	zone := image.Rect(1170, 5, 1215, 45) // gear cluster around the button
	got := anchoredPopupRect(anchor, 200, 300, win, GridAnchorOpts{
		Avoid: []image.Rectangle{zone},
	})
	if got.Overlaps(zone) {
		t.Fatalf("popup %v overlaps avoid zone %v", got, zone)
	}
	if !got.In(win) {
		t.Fatalf("popup escaped window: %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestAnchoredPopupRect'`
Expected: FAIL — `undefined: anchoredPopupRect` / `undefined: GridAnchorOpts`.

- [ ] **Step 3: Implement the helper**

Append to `src/go/internal/ui/overlay_anchor.go`:

```go
// GridAnchorOpts configures anchoredPopupRect / (*Game).gridAnchoredPopupRect.
type GridAnchorOpts struct {
	// Prefer is the placement side relative to the anchor; the zero value
	// (PopupBelow) drops the popup under its trigger.
	Prefer PopupSide
	// Avoid lists exclusion zones the popup must clear (e.g. the grid-pane
	// gear/folder button cluster). Each is applied via avoidReservedRect.
	Avoid []image.Rectangle
}

// anchoredPopupRect positions a popup of size (w,h) adjacent to anchor inside
// winBounds, then nudges it clear of each Avoid zone. It reuses AnchorPopupRect
// for the flip/clamp and avoidReservedRect for exclusion, so a popup anchored to
// a grid-pane trigger drops next to that trigger instead of being pinned into
// the bottom drum pane (the dv.Bounds-clamp bug). winBounds MUST have origin
// (0,0) — it is the whole window; avoidReservedRect clamps against its extent.
func anchoredPopupRect(anchor image.Rectangle, w, h int, winBounds image.Rectangle, opts GridAnchorOpts) image.Rectangle {
	r := AnchorPopupRect(winBounds, anchor, w, h, opts.Prefer)
	for _, zone := range opts.Avoid {
		r = avoidReservedRect(r, zone, winBounds.Dx(), winBounds.Dy())
	}
	return r
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestAnchoredPopupRect|TestAnchorPopupRect|TestClampRectInto'`
Expected: PASS (new tests + the existing anchor/clamp tests still green).

- [ ] **Step 5: Checkpoint** — report green; leave staging/commit to the user.

---

### Task 2: `(*Game).gridAnchoredPopupRect` wrapper

**Files:**
- Modify: `src/go/internal/ui/overlay_anchor.go` (append) — or `game_input_shortcuts.go`; keep it next to the pure helper in `overlay_anchor.go`.
- Test: `src/go/internal/ui/overlay_anchor_test.go` (append)

**Interfaces:**
- Consumes: `anchoredPopupRect` (Task 1), `(*Game).gridHelpReservedRect` (`grid_reserved_zone.go:18`), `Game.winW`/`Game.winH`.
- Produces: `func (g *Game) gridAnchoredPopupRect(anchor image.Rectangle, w, h int, opts GridAnchorOpts) image.Rectangle` — supplies the full window as clamp box and auto-appends the gear cluster zone to `opts.Avoid`.

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/overlay_anchor_test.go` (this test drives a real `Game`, so it needs the standard imports — add `game_log "github.com/ingyamilmolinar/beatmo/internal/log"` to the test file's import block if not already present):

```go
func TestGridAnchoredPopupRectDropsBelowGridButton(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		t.Fatal("gear button rect empty after layout")
	}
	got := g.gridAnchoredPopupRect(gear, 200, 260, GridAnchorOpts{})

	// Drops at/below the gear top and clears the reserved gear zone.
	if got.Min.Y < gear.Min.Y {
		t.Fatalf("popup %v should be at/below the gear top %v", got, gear)
	}
	if z := g.gridHelpReservedRect(); !z.Empty() && got.Overlaps(z) {
		t.Fatalf("popup %v overlaps gear reserved zone %v", got, z)
	}
	// Stays within the window (NOT pinned into the bottom drum pane).
	if win := image.Rect(0, 0, 1280, 720); !got.In(win) {
		t.Fatalf("popup %v escaped window %v", got, win)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridAnchoredPopupRectDropsBelowGridButton'`
Expected: FAIL — `g.gridAnchoredPopupRect undefined`.

- [ ] **Step 3: Implement the wrapper**

Append to `src/go/internal/ui/overlay_anchor.go`:

```go
// gridAnchoredPopupRect positions a popup anchored to a grid-pane trigger rect.
// It clamps to the WHOLE window (not dv.Bounds, which is only the bottom drum
// pane) so the popup drops into the grid pane beside its trigger, and it always
// dodges the reserved gear/folder cluster. This is the single positioning path
// for every grid-pane-button menu (File, Settings, Logs). Bottom-pane menus that
// correctly clamp to dv.Bounds keep using AnchorPopupRect directly.
func (g *Game) gridAnchoredPopupRect(anchor image.Rectangle, w, h int, opts GridAnchorOpts) image.Rectangle {
	if z := g.gridHelpReservedRect(); !z.Empty() {
		opts.Avoid = append(opts.Avoid, z)
	}
	win := image.Rect(0, 0, g.winW, g.winH)
	return anchoredPopupRect(anchor, w, h, win, opts)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestGridAnchoredPopupRect'`
Expected: PASS.

- [ ] **Step 5: Checkpoint** — report green; leave staging/commit to the user.

---

### Task 3: Settings overlay anchors under the gear

**Files:**
- Modify: `src/go/internal/ui/settings_overlay.go` (add `place` field + `SetPositioner`; use it in `Layout`)
- Modify: `src/go/internal/ui/game_input_shortcuts.go:390-415` (`openSettingsOverlay` wires the positioner on desktop)
- Test: `src/go/internal/ui/settings_overlay_modal_close_test.go` (append)
- Scene (existing, reused): `shortcuts_overlay` (`scene_catalog.go:48`)

**Interfaces:**
- Consumes: `(*Game).gridAnchoredPopupRect` (Task 2), `(*Game).gridHelpButtonRect` (`game_input_shortcuts.go:296`), `Profile().UseBottomSheet`.
- Produces: `func (o *SettingsOverlay) SetPositioner(place func(w, h int) image.Rectangle)`.

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/settings_overlay_modal_close_test.go`:

```go
// TestSettingsOverlayAnchorsUnderGear asserts the settings panel drops from the
// grid-pane gear (top-right) instead of centering — the bottom-panel-pin bug.
func TestSettingsOverlayAnchorsUnderGear(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay after toggle")
	}

	gear := g.gridHelpButtonRect()
	// Anchored at/below the gear, hugging the right side — not centered.
	if ov.rect.Min.Y < gear.Min.Y {
		t.Fatalf("settings panel %v should be at/below the gear %v", ov.rect, gear)
	}
	if ov.rect.Min.X < 1280/2 {
		t.Fatalf("settings panel %v should hug the right side under the gear, not center", ov.rect)
	}
	if z := g.gridHelpReservedRect(); !z.Empty() && ov.rect.Overlaps(z) {
		t.Fatalf("settings panel %v overlaps gear zone %v", ov.rect, z)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSettingsOverlayAnchorsUnderGear'`
Expected: FAIL — the panel is centered, so `ov.rect.Min.X < 640` (or it sits above/around the gear).

- [ ] **Step 3: Add the injectable positioner to SettingsOverlay**

In `src/go/internal/ui/settings_overlay.go`, add a field to the struct (after `closeBtn *Button`):

```go
	// place, when set, positions the panel (owner-injected so the overlay can
	// anchor to a grid-pane button via the shared helper without importing Game).
	// nil → the panel centers on screenBounds (mobile / no owner).
	place func(w, h int) image.Rectangle
```

Add the setter (after `NewSettingsOverlay`):

```go
// SetPositioner injects an owner-supplied positioner: place(w,h) returns the
// panel rect for a panel of the given size. When set, Layout uses it instead of
// centering — this is how the desktop overlay anchors under the grid gear.
func (o *SettingsOverlay) SetPositioner(place func(w, h int) image.Rectangle) { o.place = place }
```

Replace the centering line in `Layout` (currently `settings_overlay.go:80-82`):

```go
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
```

with:

```go
	if o.place != nil {
		o.rect = o.place(w, h)
	} else {
		cx := (b.Min.X + b.Max.X) / 2
		cy := (b.Min.Y + b.Max.Y) / 2
		o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	}
```

- [ ] **Step 4: Wire the positioner in `openSettingsOverlay`**

In `src/go/internal/ui/game_input_shortcuts.go`, rewrite the `p.Open(PortalEntry{...})` block in `openSettingsOverlay` (currently `:398-414`) to build a local overlay and attach the positioner on desktop:

```go
	ov := NewSettingsOverlay(func(l i18n.Locale) {
		old := i18n.ActiveLocale()
		i18n.SetLocale(l)
		persistLanguage(l)
		emitLanguageChanged(old.String(), l.String())
	})
	// Desktop: drop the panel from the grid gear via the shared helper. Mobile
	// (UseBottomSheet) keeps the centered form — a 480px panel anchored to a
	// corner button is worse on a phone (see spec risk 3).
	if g := dv.game; g != nil && !Profile().UseBottomSheet {
		ov.SetPositioner(func(w, h int) image.Rectangle {
			return g.gridAnchoredPopupRect(g.gridHelpButtonRect(), w, h, GridAnchorOpts{})
		})
	}
	p.Open(PortalEntry{
		ID:      settingsOverlayID,
		Overlay: ov,
		Modal:   true,
		Scrim:   false,
	})
```

- [ ] **Step 5: Run the test + the existing settings suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSettingsOverlay|TestSettingsLanguage'`
Expected: PASS — new anchor test green; existing close-button / modal / real-click tests still green (they read rects off `o.rect`, wherever it is).

- [ ] **Step 6: Screenshot verification (MANDATORY)**

Build and capture desktop + mobile:

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene shortcuts_overlay -screenshot /tmp/settings_desktop.png
MOBILE=1 xvfb-run -a -s "-screen 0 414x896x24" /tmp/beatmo_screenshot -log ERROR -scene shortcuts_overlay -screenshot /tmp/settings_mobile.png
```

Open `/tmp/settings_desktop.png` and confirm: the Settings panel drops from the top-right gear, fully legible, not pinned to the bottom panel, not overlapping the gear cluster. Open `/tmp/settings_mobile.png` and confirm the mobile form is unchanged (centered). Fix + re-capture until correct.

- [ ] **Step 7: Checkpoint** — report green + attach both PNGs; leave staging/commit to the user.

---

### Task 4: Logs panel anchors under the logs button

**Files:**
- Modify: `src/go/internal/ui/logs_component.go:287-295` (`panelRect`)
- Test: `src/go/internal/ui/logs_panel_functional_test.go` (append)
- Scene (existing, reused): `logs_panel` (`scene_catalog.go:52`)

**Interfaces:**
- Consumes: `(*Game).gridAnchoredPopupRect` (Task 2), `(*Game).gridLogsButtonRect` (`game_input_shortcuts.go:339`), `DrumView.game`, `Profile().UseBottomSheet`.
- Produces: no new exported symbol; `panelRect` behavior change only.

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/logs_panel_functional_test.go` (mirror the import block of the existing file; it already drives a `Game`):

```go
// TestLogsPanelAnchorsUnderLogsButton asserts the logs panel drops from the
// grid-pane logs button and paints into the grid pane, not centered in the
// bottom drum pane (the dv.Bounds-clamp bug).
func TestLogsPanelAnchorsUnderLogsButton(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleLogsPanel()
	advanceFrames(g, 2)

	logsBtn := g.gridLogsButtonRect()
	if logsBtn.Empty() {
		t.Fatal("logs button rect empty after layout")
	}
	r := g.drum.ensureLogsComp().panelRect()

	if r.Min.Y < logsBtn.Min.Y {
		t.Fatalf("logs panel %v should be at/below the logs button %v", r, logsBtn)
	}
	// Panel STARTS in the grid pane (above the split line), proving it is no
	// longer clamped into the bottom drum pane.
	if r.Min.Y >= g.drum.Bounds.Min.Y {
		t.Fatalf("logs panel %v is pinned into the bottom drum pane (top %d); expected it to start in the grid pane",
			r, g.drum.Bounds.Min.Y)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLogsPanelAnchorsUnderLogsButton'`
Expected: FAIL — `panelRect` centers in `dv.Bounds`, so `r.Min.Y >= g.drum.Bounds.Min.Y`.

- [ ] **Step 3: Anchor `panelRect` to the logs button**

Replace `panelRect` in `src/go/internal/ui/logs_component.go:287-295` with:

```go
// panelRect returns the logs panel rect. Same size as before (3/4 of the drum
// pane); on desktop it now drops from the grid-pane logs button via the shared
// helper instead of centering in the drum pane. Mobile (UseBottomSheet) or a
// missing owner falls back to the centered form.
func (c *LogsComponent) panelRect() image.Rectangle {
	b := c.dv.Bounds
	w := b.Dx() * 3 / 4
	h := b.Dy() * 3 / 4
	if g := c.dv.game; g != nil && !Profile().UseBottomSheet {
		if anchor := g.gridLogsButtonRect(); !anchor.Empty() {
			return g.gridAnchoredPopupRect(anchor, w, h, GridAnchorOpts{})
		}
	}
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	return image.Rect(cx-w/2, cy-h/2, cx+w/2, cy+h/2)
}
```

- [ ] **Step 4: Run the test + the existing logs suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLogs'`
Expected: PASS — new anchor test green; existing logs functional tests still green (they read hit/close rects off `panelRect`, wherever it is).

- [ ] **Step 5: Screenshot verification (MANDATORY)**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene logs_panel -screenshot /tmp/logs_desktop.png
MOBILE=1 xvfb-run -a -s "-screen 0 414x896x24" /tmp/beatmo_screenshot -log ERROR -scene logs_panel -screenshot /tmp/logs_mobile.png
```

Open `/tmp/logs_desktop.png`: the logs panel drops from the top-right logs button, list + search + paging legible, not pinned to the bottom. Open `/tmp/logs_mobile.png`: mobile form unchanged. Fix + re-capture until correct.

- [ ] **Step 6: Full-package regression + checkpoint**

Run the whole UI fast suite to catch anything the position change disturbs:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...`
Expected: PASS. Report green + attach both PNGs; leave staging/commit to the user.

---

## Self-Review

**Spec coverage (P0+P1 scope):**
- P0 shared helper → Task 1 (pure) + Task 2 (Game wrapper). ✓ Subsumes the full-window clamp of `overflowPopupRect` and the gear-avoidance of `placeGroupMenu`'s building blocks (`avoidReservedRect`). `KeepVisible` (member-avoidance) is intentionally deferred to the P2 group-menu plan (YAGNI — nothing in P0/P1 needs it); `GridAnchorOpts` gains that field then.
- P1 Settings anchoring → Task 3. ✓
- P1 Logs anchoring → Task 4. ✓
- Mobile-bottom-sheet risk (spec risk 3) → both Task 3 and Task 4 guard `!Profile().UseBottomSheet` and fall back to the existing centered form. ✓
- File-menu refactor → explicitly deferred (documented in Scope); not required to fix the bug. ✓

**Placeholder scan:** none — every code step shows complete code; every run step shows the command + expected result.

**Type consistency:** `GridAnchorOpts{Prefer PopupSide; Avoid []image.Rectangle}` and `anchoredPopupRect(anchor, w, h, winBounds, opts)` are defined in Task 1 and consumed unchanged in Task 2; `(*Game).gridAnchoredPopupRect(anchor, w, h, opts)` defined in Task 2 and consumed unchanged in Tasks 3–4. `SetPositioner(func(w, h int) image.Rectangle)` defined and consumed in Task 3. Names match across tasks. ✓
