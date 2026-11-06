# Neutral, Unified Button Styling — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every chrome / stepper / popup button share the neutral surface look; reserve color for Play (green), Stop (red), Record (red dot) and the close-X glyph (red); strip cyan from hover/toggle/track-lock.

**Architecture:** Beatmo's button visuals are codegen'd from `DESIGN.md` YAML front matter into `internal/ui/design_*.gen.go` via `make gen-design-tokens`, guarded by drift/spec/lint tests. The shared `*Button` type (`uigrid.go`) draws fill+border from a `ButtonStyle`/spec plus a cushion layer (`button_cushion.go`: drop shadow, glow ring, hover bloom, inner shadow). We (1) repoint the navy stepper recipe to neutral, (2) recolor the shared glow from cyan to neutral, (3) draw the node-sidebar +/− through the real `Button.Draw` path (the Button objects already exist in `sb.btns`), (4) enlarge + red-glyph the close button.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), DESIGN.md design-token codegen.

---

## Conventions for every task

- **Bundled Go:** `cd src/go && ../../.tools/go/bin/go ...`
- **Regen after any DESIGN.md edit:** `make gen-design-tokens` (from repo root).
- **Fast guard suite:**
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
    -run 'TestDesignMDDrift|TestComponentSpecsDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/
  ```
- **Build sanity (phantom `./...` build is broken on this branch — use the explicit package):**
  `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
- **DO NOT `git add` / `git commit` / `git stash` / `git restore`.** Leave all changes in the working tree. The "checkpoint" step in each task is a *verification*, not a commit. Revert mistakes manually with the Edit tool.
- The branch has **pre-existing, unrelated** test failures — always verify your own work with `-run` filters, never by a bare full-suite pass.

---

## Task 1: Neutralize the stepper recipe (BPM ± and timeline cell-size ±)

Repoint `button-stepper` (navy) to the neutral `button-secondary` recipe, and collapse the
mobile/desktop retint of the timeline cell-size buttons.

**Files:**
- Modify: `DESIGN.md` (the `button-stepper:` component, ~line 1300-1309)
- Regenerate: `src/go/internal/ui/design_components.gen.go`
- Modify: `src/go/internal/ui/drumview_layout.go:16-31` (`refreshLenButtonsStyle`)
- Modify: `src/go/internal/ui/drumview_ctor.go:1045-1052` (mobile rowZoom buttons)
- Test: `src/go/internal/ui/components_drift_test.go`, `testdata/design_md_lint.expected.json`

- [ ] **Step 1: Edit `button-stepper` in DESIGN.md to the neutral recipe.** Replace the
  `button-stepper:` block so it mirrors `button-secondary` (surface body, muted icon, thin
  border, btn-md, top highlight + interaction):

  ```yaml
  # Numeric stepper (BPM ±, Length ±) — neutral, identical recipe to
  # button-secondary so the +/- steppers read as ordinary chrome, not a
  # distinct cluster.
  button-stepper:
    category: button
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.on-surface}"
    iconColor: "{colors.on-surface-muted}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"
    topEdgeHighlight: true
    interaction:
      hover: { fillDelta: 12, borderDelta: 20 }
      press: { fillDelta: -20, highlightDelta: 10 }
  ```

- [ ] **Step 2: Regenerate tokens.** Run `make gen-design-tokens`. Confirm
  `git status` shows `src/go/internal/ui/design_components.gen.go` modified.

- [ ] **Step 3: Run the spec-drift guard, expect FAIL.**
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestComponentSpecsDrift' ./internal/ui/`
  Expected: FAIL — the test prints the new `ComponentButtonStepper` byte values vs. the
  hard-coded want.

- [ ] **Step 4: Update the want in `components_drift_test.go`.** Find the
  `ComponentButtonStepper` expected entry and set its Fill/Border/Icon/etc. to match the
  failure's "got" values (now equal to `ComponentButtonSecondary`). Re-run Step 3 → PASS.

- [ ] **Step 5: Collapse the mobile/desktop retint.** In
  `drumview_layout.go` `refreshLenButtonsStyle`, both branches now want the same neutral
  stepper. Replace the body with a single assignment:

  ```go
  func (dv *DrumView) refreshLenButtonsStyle() {
      if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
          return
      }
      dv.lenDecBtn.Style = LenDecStyle
      dv.lenDecBtn.IconColor = colIncDecIcon
      dv.lenIncBtn.Style = LenIncStyle
      dv.lenIncBtn.IconColor = colIncDecIcon
  }
  ```
  (`LenDecStyle`/`LenIncStyle` still resolve to `ComponentButtonStepper`, now neutral.
  `colIncDecIcon` = `colTextPrimary`, a neutral on-surface glyph.)

- [ ] **Step 6: Point mobile rowZoom buttons at the neutral stepper.** In
  `drumview_ctor.go:1045-1052`, swap `TransportIncStyle`/`TransportDecStyle` for
  `LenIncStyle`/`LenDecStyle` so the mobile chips match BPM/cell-size exactly:

  ```go
  dv.rowZoomIncBtn = IconOnlyButton(IconPlus, LenIncStyle)
  // ...
  dv.rowZoomDecBtn = IconOnlyButton(IconMinus, LenDecStyle)
  ```

- [ ] **Step 7: Refresh the lint snapshot if the linter set changed.**
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDLintSnapshot' ./internal/ui/`
  If it FAILs only because warnings changed, update `testdata/design_md_lint.expected.json`
  to the new set (the test prints the diff). If it PASSes, skip.

- [ ] **Step 8: Checkpoint (no commit).** Run the full guard suite + vet:
  ```bash
  cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/ && \
  ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
    -run 'TestDesignMDDrift|TestComponentSpecsDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/
  ```
  Expected: PASS. Leave changes uncommitted.

---

## Task 2: Neutralize the shared hover glow + toggle + track-lock

The cushion glow is cyan (`TokenAccent`/`TokenAccentBright`). Recolor it neutral; redefine
the track-follow-on style so ON reads as surface-3, not cyan.

**Files:**
- Modify: `src/go/internal/ui/button_cushion.go:66-143` (`buttonGlowRingColor`, `drawButtonHoverGlow`)
- Modify: `DESIGN.md` (`button-transport-follow-on:` ~line 1372) + regen
- Test: `src/go/internal/ui/button_anim_test.go` (glow color assertions), `components_drift_test.go`

- [ ] **Step 1: Write/adjust a failing test for neutral glow.** In `button_anim_test.go`
  add (or modify the existing color assertion) so it pins the *neutral* ring color:

  ```go
  func TestButtonGlowRingColorIsNeutral(t *testing.T) {
      // Hover and toggle rings must be neutral (no cyan accent) — chrome
      // de-accent pass 2026-06-15.
      hover := buttonGlowRingColor(false)
      tog := buttonGlowRingColor(true)
      cyan := TokenAccentBright()
      if hover == cyan || tog == TokenAccent() {
          t.Fatalf("glow ring still cyan: hover=%v tog=%v", hover, tog)
      }
      if hover != TokenTextPrimary() {
          t.Fatalf("hover ring want neutral TokenTextPrimary, got %v", hover)
      }
  }
  ```

- [ ] **Step 2: Run it, expect FAIL.**
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestButtonGlowRingColorIsNeutral' ./internal/ui/`
  Expected: FAIL (still returns cyan).

- [ ] **Step 3: Recolor `buttonGlowRingColor`.** Replace its body so both states return a
  neutral white lift (token-driven, no new literal):

  ```go
  // buttonGlowRingColor returns the neutral hover/toggle ring color. The
  // chrome de-accent pass removed cyan from interaction feedback: hover and
  // latched states both use a neutral white lift; affordance comes from
  // brightness + (for toggles) the active style's surface-3 fill.
  func buttonGlowRingColor(toggled bool) color.RGBA {
      return TokenTextPrimary()
  }
  ```

- [ ] **Step 4: Recolor the hover bloom.** In `drawButtonHoverGlow`, change
  `bright := TokenAccentBright()` to `bright := TokenTextPrimary()` so the bloom/ring/bevel
  are neutral white instead of cyan.

- [ ] **Step 5: Run the test, expect PASS.** Re-run Step 2's command → PASS.

- [ ] **Step 6: Neutralize track-follow-on in DESIGN.md.** Edit `button-transport-follow-on:`
  so ON is a neutral filled surface, not cyan:
  ```yaml
  button-transport-follow-on:
    category: button
    backgroundColor: "{colors.surface-3}"
    textColor: "{colors.on-surface}"
    iconColor: "{colors.on-surface}"
    border:
      color: "{colors.border}"
      alpha: border-thin
    rounded: "{rounded.md}"
    height: "{spacing.btn-md}"
  ```
  Run `make gen-design-tokens`, then `TestComponentSpecsDrift` (FAIL → update the
  `ComponentButtonTransportFollowOn` want to the new values → PASS). If `accent-overlay`
  alpha (line ~921, "button-transport-follow-on border") becomes orphaned, leave it (other
  notes may reference it) unless `TestTokenDiscipline`/lint flags it.

- [ ] **Step 7: Checkpoint (no commit).** Run the full guard suite + vet (command from
  Task 1 Step 8) + `-run 'TestButtonGlowRingColorIsNeutral'`. Expected: PASS. Leave uncommitted.

---

## Task 3: Draw node-sidebar +/− as real Buttons (icons, not glyph rects)

The Buttons already exist in `sb.btns["vol-"]` etc. with `PopupButtonStyle` (neutral). Switch
their drawing from custom `drawIncDecBtn` to `Button.Draw`, give them plus/minus icons, and
round the value pill.

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`mk` ~387-408; the 6 inc/dec draw
  call sites at 1073/1086/1099/1146/1153/1192; `drawIncDecBtn` 1439-1462; `drawValuePill` 1467-1480)
- Test: new `src/go/internal/ui/node_sidebar_incdec_button_test.go`

- [ ] **Step 1: Write a failing test that the sidebar +/− are real, icon-bearing Buttons.**

  ```go
  package ui

  import "testing"

  // The node-sidebar +/- steppers must be real *Button instances drawn via
  // Button.Draw with plus/minus icons (not hand-drawn glyph rects). Chrome
  // unification 2026-06-15.
  func TestSidebarIncDecAreIconButtons(t *testing.T) {
      g := newTestGameWithNodeSelected(t) // helper: builds a Game, selects a node, opens sidebar
      sb := g.nodeSidebar()               // accessor for *NodeSidebar
      g.forceLayoutAndDraw(t)             // run a layout+draw pass so mk() populates sb.btns

      for _, key := range []string{"vol-", "vol+", "pit-", "pit+", "dur-", "dur+"} {
          b := sb.btns[key]
          if b == nil {
              t.Fatalf("sidebar button %q missing", key)
          }
          wantIcon := "plus"
          if key[len(key)-1] == '-' {
              wantIcon = "minus"
          }
          if b.Icon != wantIcon {
              t.Errorf("%q icon = %q, want %q", key, b.Icon, wantIcon)
          }
          if b.Text != "" {
              t.Errorf("%q still carries text label %q (should be icon-only)", key, b.Text)
          }
      }
  }
  ```
  Note: use the existing sidebar test scaffolding for the helpers — search
  `node_sidebar*_test.go` for the real constructor/accessor names and adapt
  (`newTestGameWithNodeSelected`/`nodeSidebar`/`forceLayoutAndDraw` are placeholders for
  whatever the existing tests already use; do not invent new harness if one exists).

- [ ] **Step 2: Run it, expect FAIL** (`Icon = ""`, `Text = "-"`).
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestSidebarIncDecAreIconButtons' ./internal/ui/`

- [ ] **Step 3: Give the inc/dec buttons icons in `mk`.** In `mk` (game_node_sidebar.go:387),
  after the button is created/updated, set the icon from the id suffix and clear the text
  label so they render icon-only:

  ```go
  // +/- steppers render as icon-only neutral buttons (chrome unification).
  if strings.HasSuffix(id, "-") {
      sb.btns[id].Icon = "minus"
      sb.btns[id].Text = ""
  } else if strings.HasSuffix(id, "+") {
      sb.btns[id].Icon = "plus"
      sb.btns[id].Text = ""
  }
  sb.btns[id].IconColor = colTextPrimary
  ```
  Ensure `"strings"` is imported (it almost certainly already is; check the import block).

- [ ] **Step 4: Draw them via `Button.Draw`.** Replace the six `sb.drawIncDecBtn(dst, "X", "…")`
  call sites (lines 1073, 1086, 1099, 1146, 1153, 1192 and their `+` siblings) with
  `sb.drawBtn(dst, "X")` (the existing helper at line ~1424 that calls `b.Draw`). Example for
  the volume block:
  ```go
  sb.drawBtn(dst, "vol-")
  sb.drawValuePill(dst, "volval", fmt.Sprintf("%d%%", pct))
  sb.drawBtn(dst, "vol+")
  ```

- [ ] **Step 5: Round the value pill.** In `drawValuePill`, swap the sharp `drawRect` for the
  rounded helper so it matches button chrome:
  ```go
  drawRoundedRect(dst, r, colSurface2, RadiusSM, true)
  drawRoundedRect(dst, r, colBorderMedium, RadiusSM, false)
  ```
  (Leave the text drawing as-is.)

- [ ] **Step 6: Delete the now-unused `drawIncDecBtn`.** Remove the function
  (game_node_sidebar.go:1439-1462). Confirm no remaining callers:
  `cd src/go && grep -rn "drawIncDecBtn" internal/ui/` → no hits.

- [ ] **Step 7: Run the test, expect PASS.** Re-run Step 2's command → PASS.

- [ ] **Step 8: Checkpoint (no commit).** `go vet -tags test ./internal/ui/` then run any
  existing `node_sidebar*_test.go` to catch regressions:
  `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Sidebar|NodeSidebar' ./internal/ui/`
  Expected: PASS (or only pre-existing unrelated fails — confirm by checking the same `-run`
  on a clean checkout if unsure). Leave uncommitted.

---

## Task 4: Close "X" — enlarge, red glyph, unified positioning

**Files:**
- Modify: `DESIGN.md` (`profileOverrides.*.closeButtonSize` and `densities.*.closeButtonSize`) + regen
- Modify: `src/go/internal/ui/drumview_overlay_color_comp.go:295-307` (color-wheel close → `closeButtonRect`)
- Modify: every close-button creation site to set red IconColor (see Step 3)
- Test: new `src/go/internal/ui/close_button_style_test.go`; profile/density token tests

- [ ] **Step 1: Bump `CloseButtonSize` in DESIGN.md.** In the YAML front matter set:
  - `profileOverrides.desktop.closeButtonSize: 22` (was 16)
  - `profileOverrides.mobile.closeButtonSize: 28` (unchanged)
  - `densities.comfortable.closeButtonSize: 22` (was 16)
  - `densities.compact.closeButtonSize: 20` (was 14)
  - `densities.spacious.closeButtonSize: 28` (unchanged)
  Run `make gen-design-tokens`; confirm `design_profile.gen.go` + `design_density.gen.go`
  show the new values.

- [ ] **Step 2: Run the profile/density token guards, expect FAIL.**
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|Profile|Density' ./internal/ui/`
  Update whichever want-map asserts `CloseButtonSize` (search the failing test for
  `CloseButtonSize`) to the new numbers. Re-run → PASS.

- [ ] **Step 3: Write a failing test that the close glyph is red.**

  ```go
  package ui

  import "testing"

  // Close "X" buttons keep a neutral body but a red glyph (reusing the
  // existing error/stop red — no 4th red). Chrome unification 2026-06-15.
  func TestCloseButtonGlyphIsRed(t *testing.T) {
      // closeIconColor is the single source for every close button's icon tint.
      got := closeIconColor()
      if got != colError { // colError == genColorError (the stop/error red)
          t.Fatalf("close glyph = %v, want red colError %v", got, colError)
      }
  }
  ```

- [ ] **Step 4: Add a `closeIconColor()` helper and use it everywhere.** Add to
  `drawing.go` (next to `closeButtonRect`):
  ```go
  // closeIconColor is the shared tint for every popup close "X" glyph: the
  // existing error/stop red on an otherwise-neutral button body. Reusing the
  // error red avoids introducing a 4th red (DESIGN.md three-reds invariant).
  func closeIconColor() color.Color { return colError }
  ```
  Then set `IconColor: closeIconColor()` at each close-button creation site:
  - `drumview_overlay_inst_comp.go:985-990`
  - `drumview_context_menu.go:947-955` and `:179-187`
  - `drumview_fx_panel.go:635-642`
  - `drumview_overlay_color_comp.go:303-307`
  - `game_node_sidebar.go:667` (replace `colButtonBorder` with `closeIconColor()`)
  Confirm `colError` exists: `cd src/go && grep -n "colError" internal/ui/theme.go` (it is
  the `error` role color; if the symbol differs, use the actual `error`-token accessor).

- [ ] **Step 5: Route the color-wheel close through `closeButtonRect`.** In
  `drumview_overlay_color_comp.go:295-307` replace the hand-computed rect (the `-3` inset
  expression) with `closeButtonRect(titleRect, SpaceXS)` so it matches every other popup.

- [ ] **Step 6: Run the test, expect PASS.** Re-run Step 3's command → PASS.

- [ ] **Step 7: Checkpoint (no commit).** Full guard suite + vet (Task 1 Step 8 command) plus
  `-run 'Close|Sidebar'`. Expected: PASS. Leave uncommitted.

---

## Task 5: Retire orphaned navy tokens + final sweep

After Tasks 1 & 3, the navy `stepper-fill`/`stepper-border` tokens and `colIncDec`/
`colIncDecBorder` (and `colIncDecIconHi` if now unreferenced) are dead.

**Files:**
- Modify: `src/go/internal/ui/theme.go:172-173,191-192`
- Modify: `DESIGN.md` (`stepper-fill`, `stepper-border`, `incdec-icon-hi` color entries) + regen
- Modify: `src/go/internal/ui/design_md_drift_test.go` (want-map, e.g. line 84 `incdec-icon-hi`)

- [ ] **Step 1: Confirm the tokens are orphaned.**
  ```bash
  cd src/go && grep -rn "colIncDec\b\|colIncDecBorder\|colIncDecIconHi\|genColorStepperFill\|genColorStepperBorder\|genColorIncdecIconHi" internal/ui/ | grep -v "_test.go\|\.gen\.go"
  ```
  Expect only the definitions in `theme.go` (no live consumers). If `colIncDecIcon` (without
  `Hi`) is still used by Task 1's `refreshLenButtonsStyle`, keep it.

- [ ] **Step 2: Remove the dead Go aliases** in `theme.go` (`colIncDec`, `colIncDecBorder`,
  and `colIncDecIconHi` if Step 1 showed it orphaned). Leave `colIncDecIcon` if still used.

- [ ] **Step 3: Remove the dead color tokens from DESIGN.md** (`stepper-fill`,
  `stepper-border`, and `incdec-icon-hi` if its Go alias was removed). Run
  `make gen-design-tokens`.

- [ ] **Step 4: Run the drift guard, expect FAIL, then fix the want-map.**
  Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift' ./internal/ui/`
  Remove the corresponding want-map entries (e.g. `"incdec-icon-hi": colIncDecIconHi,` at
  line ~84) for any token you deleted. Re-run → PASS.

- [ ] **Step 5: Build sanity.** `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
  Expected: clean build (no "declared and not used" / undefined symbols).

- [ ] **Step 6: Full guard sweep (no commit).**
  ```bash
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
    -run 'TestDesignMDDrift|TestComponentSpecsDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/
  ```
  Expected: PASS. Leave everything uncommitted in the working tree.

---

## Final verification (after all tasks)

- [ ] `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` → clean.
- [ ] Guard suite (Task 1 Step 8 command) → PASS.
- [ ] Targeted behavior tests added in Tasks 2–4 → PASS:
  `-run 'TestButtonGlowRingColorIsNeutral|TestSidebarIncDecAreIconButtons|TestCloseButtonGlyphIsRed'`
- [ ] Optional visual confirmation: `make screenshots-all SCENES=<transport/sidebar/popup scenes>`
  and eyeball that steppers are neutral, X is red-on-neutral, hover is neutral.
- [ ] **Do not commit.** Report the uncommitted diff summary to the user.

## Self-review notes (coverage vs. spec)

- Spec A (steppers neutral) → Task 1. Spec B (sidebar real Buttons) → Task 3.
  Spec C (close enlarge/red/unify) → Task 4. Spec D (hover/toggle/track-lock neutral) → Task 2.
  Token cleanup → Task 5.
- Play/Stop/Record untouched (no task changes them — confirmed out of scope).
- Row mute/solo/FX instrument colors untouched (no task edits `*RowControl*`/instrument shades).
