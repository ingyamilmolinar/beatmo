# Menu Consistency (FX + Template → Instrument-Menu Anatomy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the FX add-effect picker and the template menu onto the instrument selector's shared list-menu anatomy, and give both (plus logs) one shared, width-adaptive footer paginator — reusing existing infrastructure to avoid style/behavior drift.

**Architecture:** Extract the logs pop-up's footer pager into a pure-geometry shared helper (`menu_pager.go`) driven entirely by a caller-passed `footerRect` + pre-formatted label, so any menu width/density adapts. Rebuild the FX add-menu on the existing `drawMenuRow`/`drawMenuSearchBox`/`drawMenuGroupHeader` helpers (the same ones the instrument menu uses). Polish the template menu (already on `drawMenuRow`) and give it the shared pager.

**Tech Stack:** Go 1.26, Ebiten 2.9, `internal/ui` (ebitenstub fast-path tests via `-tags test`), design tokens from DESIGN.md, xvfb screenshot harness.

**Spec:** `docs/superpowers/specs/2026-07-22-menu-consistency-fx-template-design.md`

## Global Constraints

- **NEVER run `git add` / `git commit`.** The user manages all staging/commits. Each task ends with a verification checkpoint (tests + screenshot), NOT a commit. Leave all changes uncommitted.
- **Bundled Go only:** run Go via `.tools/go/bin/go` (from repo root) or `../../.tools/go/bin/go` (from `src/go`). Never system `go`.
- **Fast test command (from `src/go`):** `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
- **Screenshot build:** from `src/go`: `CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd`
- **Screenshot capture:** from repo root: `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; mobile pass adds nothing to the command — mobile scenes render via `runSceneMobile`; use `make screenshots-all SCENES=<name> MOBILE=1` for the mobile pass.
- **MANDATORY UI verification:** every visual task ends by building, capturing the named scene(s) desktop + mobile, and *opening the PNG to confirm it renders correctly* (text legible, no blank controls, no overlap, correct hover/active states). Green unit tests are NOT sufficient for UI.
- **Tokens, not literals:** no `color.RGBA{}` / `color.NRGBA{}` literals in `internal/ui/` outside `theme.go`, `theme_tokens.go`, `drawing.go`, `icons.go`. Route colors through `Token*()`/`colXxx`. `TestTokenDiscipline` guards this.
- **Icons via `IconID`** (`IconFx`, `IconChevronLeft`, `IconChevronRight`) — never raw glyphs.
- **Reuse, don't reinvent:** rows → `drawMenuRow`/`MenuRowSpec`; search → `ensureMenuSearchBox`/`drawMenuSearchBox` + `MenuSearcher`; group headers → `drawMenuGroupHeader`; pager → the new `drawMenuPagerFooter`. Do not hand-roll equivalents.
- **Instrument menu (`drumview_overlay_inst_comp.go`) is the reference — do NOT modify it** in this plan.

---

## File Structure

- **Create** `src/go/internal/ui/menu_pager.go` — shared, width-adaptive footer paginator (draw + geometry only).
- **Create** `src/go/internal/ui/menu_pager_test.go` — geometry tests at multiple widths + densities.
- **Modify** `src/go/internal/ui/logs_component.go` — `drawFooter` delegates to the shared pager (byte-identical output).
- **Modify** `src/go/internal/ui/drumview_fx_panel.go` — rebuild the `fxAddMenuOpen` list on shared helpers (rows, search, groups, empty state, pager, Back row, title middot).
- **Modify** `src/go/internal/ui/drumview_fx_panel_test.go` — FX add-menu behavior tests.
- **Modify** `src/go/internal/ui/drumview_context_menu.go` — template menu polish + shared pager.
- **Modify** `src/go/internal/ui/drumview_context_menu_test.go` — template pager/Esc tests.
- **Modify** `src/go/internal/ui/scene_catalog.go` — add FX search/page scenes (the `fx_add_effect_menu` scene already exists).

---

## Reference: existing helpers this plan consumes (verified signatures)

```go
// menu_chrome.go
func drawMenuRow(dst *ebiten.Image, btn *Button, spec MenuRowSpec)
type MenuRowSpec struct {
    Accent, Swatch, IconTint, LabelColor, TrailTint color.Color
    State       menuItemState // menuItemRest | menuItemHover | menuItemActive
    IconID, TrailIconID IconID
    Highlights  []int
    Label, TrailLabel string
    LabelRole   TextRole
    CenterLabel bool
}
func drawMenuGroupHeader(dst *ebiten.Image, r image.Rectangle, label string) // hairline + UPPERCASE caption
func drawMenuTitle(dst *ebiten.Image, headerRect image.Rectangle, label string, swatch color.Color)

// menu_search_field.go
func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle) *TextInput
func drawMenuSearchBox(dst *ebiten.Image, box *TextInput)

// fuzzy_search.go
type MenuSearchItem struct { Key, Label string }
type MenuSearchResult struct { Key, Label string; Score int; Highlights []int }
type MenuSearcher struct{}
func (s *MenuSearcher) Search(query string, items []MenuSearchItem) []MenuSearchResult

// logs_model.go (already shared/generic)
func pageCount(total, pageSize int) int              // >= 1
func pageSlice[T any](items []T, pageSize, page int) []T // clamps page to [1, pageCount]

// uigrid.go
func NewSpecButton(text string, id ComponentID, onClick func()) *Button // ComponentButtonSecondary for chevrons
// Button: .Text, .Icon (string, e.g. string(IconChevronLeft)), .OnClick, .Disabled,
//         .Rect() image.Rectangle, .SetRect(image.Rectangle), .Draw(*ebiten.Image), .Highlights []int

// text/token helpers used by the logs footer (reuse verbatim)
func TextWidth(s string) int
func TextHeight() int
func DrawTextColorAt(dst *ebiten.Image, s string, x, y int, col color.Color)
// tokens: SpaceXS(3) SpaceSM(6) SpaceMD(10) SpaceLG(14); colTextSecondary; colMenuIcon; colAccent

// audio effect registry
func audio.EffectTypeOrder() []audio.EffectType
func audio.EffectRegistrations() map[audio.EffectType]*audio.EffectRegistration
// EffectRegistration{ Type, DisplayName string, Category string /* creative|modulation|dynamics|utility */, ... }
func audio.AddInsertEffect(instID string, t audio.EffectType, params map[string]float64) /* appends one instance */
func audio.GetInsertEffects(instID string) []... // existing getter — used to count instances for the ×N badge
```

---

## Task 1: Shared width-adaptive footer pager + logs refactor

**Files:**
- Create: `src/go/internal/ui/menu_pager.go`
- Create: `src/go/internal/ui/menu_pager_test.go`
- Modify: `src/go/internal/ui/logs_component.go` (`drawFooter`, ~lines 606-624)

**Interfaces:**
- Produces:
  - `func menuPagerFooterH() int` — reserved footer band height (== current `logsFooterH()` value: `TextHeight()+SpaceMD`).
  - `func menuPagerBtnSize() int` — prev/next button side (== current `logsBtnSize()`: `TextHeight()+SpaceSM`).
  - `func drawMenuPagerFooter(dst *ebiten.Image, footerRect image.Rectangle, label string, prev, next *Button) (prevRect, nextRect image.Rectangle)` — centers `label` on `footerRect`, flanks it with `prev`/`next` (clamped inside `footerRect`), draws all three, returns the button rects. Pure geometry; no i18n/page knowledge.
- Consumes: `TextWidth`, `TextHeight`, `DrawTextColorAt`, `SpaceXS/SM/MD/LG`, `colTextSecondary`, `Button`.

- [ ] **Step 1: Write the failing geometry test**

Create `src/go/internal/ui/menu_pager_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

// buildPagerButtons returns two ComponentButtonSecondary chevron buttons.
func buildPagerButtons() (*Button, *Button) {
	p := NewSpecButton("", ComponentButtonSecondary, func() {})
	p.Icon = string(IconChevronLeft)
	n := NewSpecButton("", ComponentButtonSecondary, func() {})
	n.Icon = string(IconChevronRight)
	return p, n
}

func TestMenuPagerFooter_ButtonsStayInsideFooterAtEveryWidth(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1)) // not drawn onto; geometry only
	_ = dst
	for _, w := range []int{160, 220, 340, 640} {
		footer := image.Rect(0, 100, w, 100+menuPagerFooterH())
		prev, next := buildPagerButtons()
		pr, nr := drawMenuPagerFooter(nil, footer, "Page 2 / 5", prev, next)
		if !pr.In(footer) {
			t.Fatalf("w=%d prev rect %v not inside footer %v", w, pr, footer)
		}
		if !nr.In(footer) {
			t.Fatalf("w=%d next rect %v not inside footer %v", w, nr, footer)
		}
		if pr.Max.X > nr.Min.X { // prev must sit left of next
			t.Fatalf("w=%d prev %v overlaps/crosses next %v", w, pr, nr)
		}
	}
}

// The footer band must always reserve enough height to contain its own
// contents (the prev/next buttons and the page label), at any font metric.
// NOTE: menuPagerFooterH/menuPagerBtnSize derive from TextHeight()+Space* which
// are density-INVARIANT in this codebase (font metric + fixed spacing tokens),
// so the pager is font-fixed like the logs pager it was extracted from — that
// is the intended no-drift behavior, not a bug. This test asserts the real
// band-fits-contents invariant instead of a (false) density-scaling one.
func TestMenuPagerFooter_BandFitsItsContents(t *testing.T) {
	if h, b := menuPagerFooterH(), menuPagerBtnSize(); h < b {
		t.Fatalf("footer band %d must be tall enough to contain the button %d", h, b)
	}
	if h := menuPagerFooterH(); h < TextHeight() {
		t.Fatalf("footer band %d must be tall enough to contain the page label %d", h, TextHeight())
	}
}
```

> `image.Rectangle.In` returns true when the receiver is fully contained. `drawMenuPagerFooter(nil, …)` must not draw when `dst == nil` (guard), so the geometry path is testable headless.

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuPagerFooter -v`
Expected: FAIL — `undefined: drawMenuPagerFooter` / `menuPagerFooterH`.

- [ ] **Step 3: Implement `menu_pager.go`**

Create `src/go/internal/ui/menu_pager.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// menuPagerFooterH is the reserved footer-band height shared by every list
// menu that shows the page navigator (logs, FX add-menu, template menu). Equal
// to the value the logs pop-up used before extraction (TextHeight()+SpaceMD),
// so logs stays pixel-identical. Density-aware via TextHeight().
func menuPagerFooterH() int { return TextHeight() + SpaceMD }

// menuPagerBtnSize is the prev/next chevron button side length (== the logs
// pop-up's former logsBtnSize()).
func menuPagerBtnSize() int { return TextHeight() + SpaceSM }

// drawMenuPagerFooter draws a centered "Page X / M" label inside footerRect and
// flanks it with the caller-owned prev/next buttons, returning their rects. All
// geometry derives from footerRect (its center-x and bottom edge) so the pager
// adapts to any menu width; the button rects are clamped to stay inside
// footerRect at narrow widths. Pure layout+draw: the caller formats `label`
// (i18n) and owns page state / button OnClick. When dst is nil only the
// geometry is computed (headless tests).
func drawMenuPagerFooter(dst *ebiten.Image, footerRect image.Rectangle, label string, prev, next *Button) (prevRect, nextRect image.Rectangle) {
	lw := TextWidth(label)
	fy := footerRect.Max.Y - menuPagerFooterH() + SpaceXS
	cx := footerRect.Min.X + footerRect.Dx()/2
	bsz := menuPagerBtnSize()
	by := fy + (TextHeight()-bsz)/2

	prevRect = image.Rect(cx-lw/2-SpaceLG-bsz, by, cx-lw/2-SpaceLG, by+bsz)
	nextRect = image.Rect(cx+lw/2+SpaceLG, by, cx+lw/2+SpaceLG+bsz, by+bsz)

	// Clamp horizontally so neither button escapes a narrow footer.
	if prevRect.Min.X < footerRect.Min.X {
		dx := footerRect.Min.X - prevRect.Min.X
		prevRect = prevRect.Add(image.Pt(dx, 0))
	}
	if nextRect.Max.X > footerRect.Max.X {
		dx := nextRect.Max.X - footerRect.Max.X
		nextRect = nextRect.Sub(image.Pt(dx, 0))
	}

	if dst == nil {
		return prevRect, nextRect
	}
	DrawTextColorAt(dst, label, cx-lw/2, fy, colTextSecondary)
	prev.SetRect(prevRect)
	next.SetRect(nextRect)
	prev.Draw(dst)
	next.Draw(dst)
	return prevRect, nextRect
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuPagerFooter -v`
Expected: PASS (both tests).

- [ ] **Step 5: Refactor logs `drawFooter` to delegate**

In `src/go/internal/ui/logs_component.go`, replace the body of `drawFooter` (currently lines ~606-624) with a delegating call. Keep the `i18n.Tf(i18n.KeyLogsPage, …)` label so logs' output is identical:

```go
// drawFooter paints the shared "Page X / M" navigator (extracted to
// drawMenuPagerFooter so logs, the FX add-menu, and the template menu share one
// width-adaptive pager). Records prev/next hit rects.
func (c *LogsComponent) drawFooter(dst *ebiten.Image, r image.Rectangle) {
	label := i18n.Tf(i18n.KeyLogsPage, c.page, c.pageCountNow())
	c.prevRect, c.nextRect = drawMenuPagerFooter(dst, r, label, c.prevBtn, c.nextBtn)
}
```

Then update `logsFooterH()` and `logsBtnSize()` to delegate to the shared values (single source of truth), leaving the rest of `logs_component.go` untouched:

```go
func logsFooterH() int { return menuPagerFooterH() }
func logsBtnSize() int { return menuPagerBtnSize() }
```

> Both shared values use the identical formulas the logs helpers used, so this is a no-op numerically; it just makes the pager the single source.

- [ ] **Step 6: Run logs + package tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Logs|MenuPager' -v`
Expected: PASS. Then full package once: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/` → PASS.

- [ ] **Step 7: Screenshot regression — logs pager unchanged**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
cd /home/ymolinar/Repos/beatmo
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene logs_panel_page2 -screenshot /tmp/shots/logs_panel_page2.png
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene logs_panel_cleared -screenshot /tmp/shots/logs_panel_cleared.png
```
Open both PNGs: `‹ Page 2 / 2 ›` and `‹ Page 1 / 1 ›` must render exactly as before (centered label, chevron buttons flanking, inside the panel). **Verification checkpoint — leave uncommitted.**

---

## Task 2: FX add-menu rows → shared flat rows + Back row + title middot

**Files:**
- Modify: `src/go/internal/ui/drumview_fx_panel.go` (add-menu build ~lines 658-716; add-menu draw ~lines 761-956; title band)
- Modify: `src/go/internal/ui/drumview_fx_panel_test.go`

**Interfaces:**
- Consumes: `drawMenuRow`/`MenuRowSpec`, `drawMenuTitle`, `IconFx`, `IconChevronLeft`.
- Produces: FX add-menu rows rendered via `drawMenuRow` (leading `IconFx`, `menuItemRest`/`menuItemHover`, no active); a "Back" row (leading `IconChevronLeft`) that sets `fxAddMenuOpen = false` + rebuilds; title text `"FX · <rowName>"`.

- [ ] **Step 1: Write the failing test — rows are flat menu rows, not centered keycaps**

Add to `src/go/internal/ui/drumview_fx_panel_test.go`. (Mirror the existing FX-panel test setup in that file — reuse its helper that builds a `*Game`/`*DrumView` with row 0 and opens the FX panel; call the same accessors the file already uses.)

```go
//go:build test

// TestFXAddMenu_TitleUsesMiddot verifies the add-menu header reads "FX · <row>"
// (middot), matching the DESIGN.md menu anatomy, not the old "FX:" colon.
func TestFXAddMenu_TitleUsesMiddot(t *testing.T) {
	g := newFXTestGame(t)          // existing helper in this test file
	ensureRowForFX(t, g, 0)        // existing helper
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	g.drum.buildFXPanel()
	title := g.drum.fxAddMenuTitle() // NEW tiny accessor returning the header string
	if !strings.Contains(title, " · ") {
		t.Fatalf("FX add-menu title should use middot separator, got %q", title)
	}
	if strings.Contains(title, "FX:") {
		t.Fatalf("FX add-menu title should not use the old colon form, got %q", title)
	}
}
```

> If the test file lacks `newFXTestGame`/`ensureRowForFX`, use whatever constructor the existing FX tests in this file already use (read the top of `drumview_fx_panel_test.go` first and match it). Add a minimal `func (dv *DrumView) fxAddMenuTitle() string` accessor that returns the same string the header draws.

- [ ] **Step 2: Run it — fails (no `fxAddMenuTitle`, title still colon)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu_TitleUsesMiddot -v`
Expected: FAIL (undefined `fxAddMenuTitle`, or middot missing).

- [ ] **Step 3: Implement — convert rows, Back row, title**

In `drumview_fx_panel.go`:

1. **Title:** where the add-menu header string is built (currently `"FX: " + rowName`), change to a shared middot form and add the accessor:

```go
func (dv *DrumView) fxAddMenuTitle() string {
	return "FX · " + dv.fxRowName() // dv.fxRowName(): existing source of the row/instrument label used in the header
}
```
Use `dv.fxAddMenuTitle()` at the header draw site, drawn via the existing `drawMenuTitle(dst, headerRect, dv.fxAddMenuTitle(), swatch)` (the swatch dot already exists — keep it).

2. **Rows:** in `buildFXPanel`'s add-menu branch (~658-696), keep creating one `*Button` per `effectTypeOrder()` entry for hit-testing, but stop styling them as centered `InstButtonStyle` keycaps. Store the effect type on each button (a parallel slice `dv.fxAddRowEffect []audio.EffectType` indexed with `dv.fxPanelBtns`, or reuse an existing per-button tag) so the draw pass can build the `MenuRowSpec`. Set each row rect to the full content width (left-aligned), height `fxPanelMetrics().lineH`.

3. **Back row:** replace the `Cancel` keycap (~687-695) with a Back row — a `*Button` whose `OnClick` sets `dv.fxAddMenuOpen = false; dv.buildFXPanel(); dv.refreshFXPortalHitAreas()`. Render it (Step below) via `drawMenuRow` with `IconID: IconChevronLeft`, `Label: i18n.T(i18n.KeyBack)` (reuse the existing Back key the template menu uses; grep `KeyBack`), placed as the first row.

4. **Draw:** in `drawFXPanel`'s add-menu branch (where it currently calls `btn.Draw(dst)` for each picker row, ~929), route each effect row through:

```go
et := dv.fxAddRowEffect[i]
reg := audio.EffectRegistrations()[et]
state := menuItemRest
if btn.hovered || btn.pressed { state = menuItemHover }
drawMenuRow(dst, btn, MenuRowSpec{
	Accent:   colAccent,
	State:    state,
	IconID:   IconFx,
	IconTint: colMenuIcon,
	Label:    reg.DisplayName,
})
```
and the Back row via `drawMenuRow(dst, backBtn, MenuRowSpec{Accent: colAccent, State: hoverStateOf(backBtn), IconID: IconChevronLeft, Label: i18n.T(i18n.KeyBack)})`. Add a small local `hoverStateOf(b *Button) menuItemState` helper (rest unless `b.hovered||b.pressed`). The button's own `.Text` must be blank so `btn.Draw` doesn't double-draw a keycap — `drawMenuRow` draws the label itself (see `menu_chrome.go` doc: "btn.Text is blanked during btn.Draw").

> Do NOT call `btn.Draw(dst)` for these rows anymore — `drawMenuRow` is the renderer. Keep the buttons only for geometry + input.

- [ ] **Step 4: Run the test — passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu -v`
Expected: PASS. Then package: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/` → PASS.

- [ ] **Step 5: Screenshot — FX add-menu restyled (desktop + mobile)**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
cd /home/ymolinar/Repos/beatmo
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene fx_add_effect_menu -screenshot /tmp/shots/fx_add_effect_menu.png
make screenshots-all SCENES=fx_add_effect_menu MOBILE=1
```
Open the PNG(s): rows must be **flat, left-aligned, each with a leading FX icon**; hover tint on the hovered row; a leading **Back** row with a left-chevron; title reads **"FX · Dnb Kick"**. No centered keycaps, no double-drawn labels. Fix + re-capture until correct. **Verification checkpoint — leave uncommitted.**

---

## Task 3: FX add-menu search field + auto-focus + Esc + empty state

**Files:**
- Modify: `src/go/internal/ui/drumview_fx_panel.go`
- Modify: `src/go/internal/ui/drumview_fx_panel_test.go`
- Modify: `src/go/internal/ui/scene_catalog.go` (add `fx_add_effect_menu_search`)

**Interfaces:**
- Consumes: `ensureMenuSearchBox`, `drawMenuSearchBox`, `MenuSearcher`, `TextInput`.
- Produces: `dv.fxAddSearch *TextInput` (query state); a filtered+score-sorted effect list when the query is non-empty; empty-state render; Esc handling (`fxAddMenuHandleEscape`).

- [ ] **Step 1: Write the failing test — search filters the effect list**

Add to `drumview_fx_panel_test.go`:

```go
// TestFXAddMenu_SearchFiltersEffects verifies typing narrows the effect list by
// fuzzy match over DisplayName.
func TestFXAddMenu_SearchFiltersEffects(t *testing.T) {
	g := newFXTestGame(t)
	ensureRowForFX(t, g, 0)
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	g.drum.buildFXPanel()

	all := g.drum.fxAddVisibleEffects() // NEW: effect types currently listed (respects query)
	g.drum.setFXAddSearchQuery("rev")   // NEW: sets query + rebuilds
	got := g.drum.fxAddVisibleEffects()
	if len(got) >= len(all) {
		t.Fatalf("query should narrow list: all=%d got=%d", len(all), len(got))
	}
	if len(got) == 0 {
		t.Fatalf("query 'rev' should still match Reverb")
	}
	names := fxEffectNames(got, audio.EffectRegistrations()) // local helper: []DisplayName
	if !containsFold(names, "Reverb") {
		t.Fatalf("query 'rev' should match Reverb; got %v", names)
	}
}

// TestFXAddMenu_EmptyStateWhenNoMatch verifies a no-match query yields an empty
// visible list (so the empty-state renders).
func TestFXAddMenu_EmptyStateWhenNoMatch(t *testing.T) {
	g := newFXTestGame(t)
	ensureRowForFX(t, g, 0)
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	g.drum.buildFXPanel()
	g.drum.setFXAddSearchQuery("zzzzzz")
	if got := g.drum.fxAddVisibleEffects(); len(got) != 0 {
		t.Fatalf("no-match query should yield 0 effects, got %d", len(got))
	}
}
```

> `containsFold`/`fxEffectNames` are tiny local test helpers — define them in the test file if not already present.

- [ ] **Step 2: Run — fails (undefined `fxAddVisibleEffects`/`setFXAddSearchQuery`)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu_Search -v`
Expected: FAIL — undefined methods.

- [ ] **Step 3: Implement search**

In `drumview_fx_panel.go`:

1. Add fields on `DrumView`: `fxAddSearch *TextInput` and cache `fxAddSearchQuery string`.
2. `func (dv *DrumView) fxAddVisibleEffects() []audio.EffectType`:

```go
func (dv *DrumView) fxAddVisibleEffects() []audio.EffectType {
	order := effectTypeOrder()
	q := strings.TrimSpace(dv.fxAddSearchQuery)
	if q == "" {
		return order
	}
	regs := audio.EffectRegistrations()
	items := make([]MenuSearchItem, 0, len(order))
	for _, et := range order {
		if r := regs[et]; r != nil {
			items = append(items, MenuSearchItem{Key: string(et), Label: r.DisplayName})
		}
	}
	res := (&MenuSearcher{}).Search(q, items)
	out := make([]audio.EffectType, 0, len(res))
	for _, r := range res {
		out = append(out, audio.EffectType(r.Key))
	}
	return out
}
```
Store per-result `Highlights` too (keep a `map[audio.EffectType][]int` populated during the same pass) so rows can pass `MenuRowSpec.Highlights` when searching.

3. `func (dv *DrumView) setFXAddSearchQuery(q string)` sets `fxAddSearchQuery`, resets the add-menu scroll + page to top/1, and calls `dv.buildFXPanel()`.

4. In `buildFXPanel` add-menu branch: build the search box rect at the top (below the title), `dv.fxAddSearch = ensureMenuSearchBox(dv.fxAddSearch, searchRect)`, and drive `dv.fxAddSearchQuery = dv.fxAddSearch.Value()` each build. Build rows from `fxAddVisibleEffects()` instead of `effectTypeOrder()`.

5. In `drawFXPanel` add-menu branch: `drawMenuSearchBox(dst, dv.fxAddSearch)` above the rows. When `len(fxAddVisibleEffects()) == 0`, draw the empty state centered in the list viewport:

```go
msg := i18n.T(i18n.KeyNoResults) // reuse existing "No results"/"No effects" key; grep for the one the inst menu uses
drawCenteredEmptyState(dst, dv.fxViewportRect, msg) // reuse the instrument menu's empty-state helper if one exists; else inline a centered DrawTextStyled(RoleBody) + caption
```
> Grep the instrument menu for its empty-state helper and key and reuse them; do not invent a new string key if one exists.

6. **Auto-focus (improvement C):** when the add-menu opens (the `+ Add Effect` `OnClick` at ~708 and any programmatic open), focus the search box (`dv.fxAddSearch.Focus()` / the same focus call the instrument menu uses on open — match it).

7. **Esc (improvement B):** add `func (dv *DrumView) fxAddMenuHandleEscape() bool`: if the query is non-empty, clear it (`setFXAddSearchQuery("")`) and return true; else set `fxAddMenuOpen = false` (back to chain) and return true. Wire it into the FX portal's escape hook (find where the FX portal registers `HandleEscape`/escape in `drumview_portal_open.go` / `refreshFXPortalHitAreas` and route to this when `fxAddMenuOpen`).

- [ ] **Step 4: Run tests — pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu -v`
Expected: PASS. Full package → PASS.

- [ ] **Step 5: Add the search scene + screenshot**

In `scene_catalog.go`, add next to `fx_add_effect_menu`:

```go
{Name: "fx_add_effect_menu_search", Description: "FX add-effect picker with an active search query (flat, filtered)", Mobile: true, SettleFrames: 120,
	Setup: func(g *Game) {
		ensureRow(g, 0)
		g.drum.OpenFXPanel(0)
		g.drum.fxAddMenuOpen = true
		g.drum.buildFXPanel()
		g.drum.setFXAddSearchQuery("re")
		g.drum.buildFXPanel()
		g.drum.refreshFXPortalHitAreas()
	}},
```

Build + capture `fx_add_effect_menu` (auto-focused search box visible) and `fx_add_effect_menu_search` (desktop + mobile). Open PNGs: search box with magnifier + caret; filtered rows with fuzzy highlight cells; no group headers while searching. **Verification checkpoint — leave uncommitted.**

---

## Task 4: FX add-menu category group headers + trailing captions + instance-count badge

**Files:**
- Modify: `src/go/internal/ui/drumview_fx_panel.go`
- Modify: `src/go/internal/ui/drumview_fx_panel_test.go`

**Interfaces:**
- Consumes: `drawMenuGroupHeader`, `audio.GetInsertEffects`.
- Produces: category-grouped layout when idle (headers `CREATIVE`/`MODULATION`/`DYNAMICS`/`UTILITY` in fixed order); category as `TrailLabel` only while searching; `×N` `TrailLabel` when idle and the chain already holds N of that effect.

- [ ] **Step 1: Write the failing test — grouping toggles with search; instance count present**

```go
// fxCategoryOrder is the fixed display order.
// TestFXAddMenu_GroupHeadersIdleOnly verifies category headers appear when idle
// and are absent while searching.
func TestFXAddMenu_GroupHeadersIdleOnly(t *testing.T) {
	g := newFXTestGame(t)
	ensureRowForFX(t, g, 0)
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	g.drum.buildFXPanel()
	if hs := g.drum.fxAddGroupHeaders(); len(hs) == 0 { // NEW: []string of headers in current layout
		t.Fatalf("idle add-menu should show category group headers")
	}
	g.drum.setFXAddSearchQuery("re")
	if hs := g.drum.fxAddGroupHeaders(); len(hs) != 0 {
		t.Fatalf("searching add-menu must suppress group headers, got %v", hs)
	}
}

// TestFXAddMenu_InstanceCountBadge verifies an already-present effect shows ×N.
func TestFXAddMenu_InstanceCountBadge(t *testing.T) {
	g := newFXTestGame(t)
	ensureRowForFX(t, g, 0)
	instID := g.drum.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	g.drum.buildFXPanel()
	if got := g.drum.fxAddInstanceCount(audio.EffectDelay); got != 2 { // NEW
		t.Fatalf("Delay instance count = %d, want 2", got)
	}
	trail := g.drum.fxAddRowTrail(audio.EffectDelay) // NEW: idle trail label for a row
	if trail != "×2" {
		t.Fatalf("idle Delay trail = %q, want \"×2\"", trail)
	}
}
```

- [ ] **Step 2: Run — fails (undefined `fxAddGroupHeaders`/`fxAddInstanceCount`/`fxAddRowTrail`)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu_Group\|TestFXAddMenu_Instance -v`
Expected: FAIL.

- [ ] **Step 3: Implement grouping + trailing captions**

In `drumview_fx_panel.go`:

1. Fixed order + helpers:

```go
var fxCategoryOrder = []string{"creative", "modulation", "dynamics", "utility"}
var fxCategoryDisplay = map[string]string{
	"creative": "Creative", "modulation": "Modulation",
	"dynamics": "Dynamics", "utility": "Utility",
}

func (dv *DrumView) fxAddInstanceCount(et audio.EffectType) int {
	n := 0
	for _, e := range audio.GetInsertEffects(dv.fxInstID()) { // dv.fxInstID(): current row's instrument id
		if e.Type == et { n++ } // match the field the getter exposes for the type
	}
	return n
}

func (dv *DrumView) fxAddRowTrail(et audio.EffectType) string {
	if strings.TrimSpace(dv.fxAddSearchQuery) != "" {
		if r := audio.EffectRegistrations()[et]; r != nil {
			return fxCategoryDisplay[r.Category] // category caption while searching
		}
		return ""
	}
	if n := dv.fxAddInstanceCount(et); n > 0 {
		return "×" + strconv.Itoa(n) // instance-count badge while idle
	}
	return ""
}
```
> Confirm the element type returned by `audio.GetInsertEffects` and its type field (grep the getter). If it returns descriptors keyed differently, adapt the comparison.

2. **Layout:** when idle (`fxAddSearchQuery == ""`), build the row list grouped: for each category in `fxCategoryOrder`, if it has ≥1 effect, emit a group-header pseudo-row (height = `fxPanelMetrics().lineH`) then its effect rows (effects keep `effectTypeOrder()` order within the category). Track which layout entries are headers vs rows (parallel slice, e.g. `fxAddIsHeader []bool` + `fxAddHeaderLabel []string`). When searching, emit a flat list (no headers). Expose `func (dv *DrumView) fxAddGroupHeaders() []string` returning the header labels currently in the layout (empty when searching).

3. **Draw:** in the add-menu draw loop, for header entries call `drawMenuGroupHeader(dst, rowRect, headerLabel)`; for effect entries call `drawMenuRow` with `TrailLabel: dv.fxAddRowTrail(et)` and (when searching) `Highlights:` from the stored map.

- [ ] **Step 4: Run tests — pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu -v`
Expected: PASS. Full package → PASS.

- [ ] **Step 5: Screenshot — grouped idle vs flat search**

Rebuild; capture `fx_add_effect_menu` (idle: CREATIVE/MODULATION/… headers, `×N` badge if an effect pre-exists — extend the scene to pre-add a Delay so the badge shows) and `fx_add_effect_menu_search` (flat + category trail). Desktop + mobile. Open PNGs and confirm headers, trailing captions, and badges render and align. **Verification checkpoint — leave uncommitted.**

> Update the `fx_add_effect_menu` scene `Setup` to pre-add one Delay effect before opening the add-menu, so the idle `×1` badge is visible in the regression shot.

---

## Task 5: FX add-menu footer pager (viewport-fit page size)

> **DROPPED (user decision, 2026-07-22):** the FX add-menu renders the effect chain cards + the 18-item picker in one shared scroll region, so a footer pager fits poorly and the benefit is marginal (~2 pages). The FX add-menu keeps its existing smooth continuous scroll. The shared pager (Task 1) is applied to the template menu only (Task 6). Skip this task.

**Files:**
- Modify: `src/go/internal/ui/drumview_fx_panel.go`
- Modify: `src/go/internal/ui/drumview_fx_panel_test.go`
- Modify: `src/go/internal/ui/scene_catalog.go` (`fx_add_effect_menu_page2`)

**Interfaces:**
- Consumes: `pageCount`, `pageSlice`, `drawMenuPagerFooter`, `menuPagerFooterH`, `NewSpecButton`.
- Produces: `dv.fxAddPage int`, `dv.fxAddPrevBtn`/`fxAddNextBtn`; footer shown only when `pageCount > 1`.

- [ ] **Step 1: Write the failing test — page math + footer visibility**

```go
// TestFXAddMenu_PaginatesWhenOverflowing verifies the page model engages when
// the effect list exceeds one viewport-fit page.
func TestFXAddMenu_PaginatesWhenOverflowing(t *testing.T) {
	g := newFXTestGame(t)
	ensureRowForFX(t, g, 0)
	g.drum.OpenFXPanel(0)
	g.drum.fxAddMenuOpen = true
	// Force a tiny page size so 18 effects need >1 page regardless of viewport.
	g.drum.fxAddForcePageSizeForTest(4) // NEW test hook
	g.drum.buildFXPanel()
	if pc := g.drum.fxAddPageCount(); pc <= 1 { // NEW
		t.Fatalf("expected >1 page with pageSize 4 over 18 effects, got %d", pc)
	}
	g.drum.fxAddPageNext() // NEW
	if g.drum.fxAddPage != 2 {
		t.Fatalf("pageNext should advance to page 2, got %d", g.drum.fxAddPage)
	}
}
```

- [ ] **Step 2: Run — fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu_Paginates -v`
Expected: FAIL.

- [ ] **Step 3: Implement pager wiring**

In `drumview_fx_panel.go`:

1. Fields: `fxAddPage int` (1-based), `fxAddPrevBtn, fxAddNextBtn *Button`, `fxAddForcedPageSize int` (0 = auto).
2. Page size (viewport-fit):

```go
func (dv *DrumView) fxAddPageSize() int {
	if dv.fxAddForcedPageSize > 0 { return dv.fxAddForcedPageSize }
	lh := fxPanelMetrics().lineH
	if lh < 1 { lh = 1 }
	// Reserve the footer band when it will show; conservative floor of 1.
	usable := dv.fxViewportRect.Dy()
	n := usable / lh
	if n < 1 { n = 1 }
	return n
}
func (dv *DrumView) fxAddForcePageSizeForTest(n int) { dv.fxAddForcedPageSize = n }
func (dv *DrumView) fxAddPageCount() int { return pageCount(len(dv.fxAddLayoutEntries()), dv.fxAddPageSize()) }
func (dv *DrumView) fxAddPageNext() { if dv.fxAddPage < dv.fxAddPageCount() { dv.fxAddPage++; dv.buildFXPanel() } }
func (dv *DrumView) fxAddPagePrev() { if dv.fxAddPage > 1 { dv.fxAddPage--; dv.buildFXPanel() } }
```
> `fxAddLayoutEntries()` is the combined header+row list from Task 4 (paginate over the full layout so a page is one screenful including its headers). Reset `fxAddPage = 1` in `setFXAddSearchQuery` and when opening the add-menu.

3. In `buildFXPanel`: slice the layout with `pageSlice(entries, dv.fxAddPageSize(), dv.fxAddPage)`; lazily create `fxAddPrevBtn`/`fxAddNextBtn` (`NewSpecButton("", ComponentButtonSecondary, dv.fxAddPagePrev/Next)` with `.Icon = string(IconChevronLeft/Right)`), reserving `menuPagerFooterH()` at the panel bottom only when `fxAddPageCount() > 1`.
4. In `drawFXPanel`: when `fxAddPageCount() > 1`, `drawMenuPagerFooter(dst, dv.fxPanelRect, i18n.Tf(i18n.KeyLogsPage, dv.fxAddPage, dv.fxAddPageCount()), dv.fxAddPrevBtn, dv.fxAddNextBtn)`.

> Reuse `i18n.KeyLogsPage` ("Page %d / %d") for the label — one string already in every locale; no new key (minimizes i18n drift per the "reuse infrastructure" directive). The pager helper itself is key-agnostic.

- [ ] **Step 4: Run tests — pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFXAddMenu -v`
Expected: PASS. Full package → PASS.

- [ ] **Step 5: Add page-2 scene + screenshot**

```go
{Name: "fx_add_effect_menu_page2", Description: "FX add-effect picker paginated to page 2 (footer pager engaged)", SettleFrames: 120,
	Setup: func(g *Game) {
		ensureRow(g, 0)
		g.drum.OpenFXPanel(0)
		g.drum.fxAddMenuOpen = true
		g.drum.fxAddForcePageSizeForTest(6)
		g.drum.buildFXPanel()
		g.drum.fxAddPageNext()
		g.drum.buildFXPanel()
		g.drum.refreshFXPortalHitAreas()
	}},
```

Build + capture `fx_add_effect_menu_page2` (desktop). Open PNG: `‹ Page 2 / N ›` footer, identical style to the logs pager, inside the FX panel, rows above it. **Verification checkpoint — leave uncommitted.**

---

## Task 6: Template menu polish + shared footer pager + Esc

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go`
- Modify: `src/go/internal/ui/drumview_context_menu_test.go`
- Modify: `src/go/internal/ui/scene_catalog.go` (`template_menu_page2`)

**Interfaces:**
- Consumes: `pageCount`, `pageSlice`, `drawMenuPagerFooter`, `menuPagerFooterH`, `NewSpecButton`.
- Produces: `overflowTemplatePage int`, prev/next buttons; footer shown when template `pageCount > 1`; Esc clears query → back to page-0 → close.

- [ ] **Step 1: Write the failing test — template paginates**

In `drumview_context_menu_test.go` (match the existing overflow-menu test setup helpers in that file):

```go
// TestTemplateMenu_PaginatesLongList verifies the shared footer pager engages
// for the template page when the list exceeds one page.
func TestTemplateMenu_PaginatesLongList(t *testing.T) {
	g := newOverflowTestGame(t)         // existing helper style in this file
	g.drum.overflowPage = 1             // template page
	g.drum.OpenOverflowMenu()
	g.drum.overflowForcePageSizeForTest(5) // NEW test hook
	g.drum.rebuildOverflowBtns()
	if pc := g.drum.overflowTemplatePageCount(); pc <= 1 { // NEW
		t.Fatalf("expected >1 template page with pageSize 5, got %d", pc)
	}
	g.drum.overflowTemplatePageNext() // NEW
	if g.drum.overflowTemplatePage != 2 {
		t.Fatalf("template pageNext should reach page 2, got %d", g.drum.overflowTemplatePage)
	}
}
```

- [ ] **Step 2: Run — fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_Paginates -v`
Expected: FAIL.

- [ ] **Step 3: Implement template pager + Esc + polish**

In `drumview_context_menu.go`:

1. Fields: `overflowTemplatePage int`, `overflowTemplatePrevBtn/NextBtn *Button`, `overflowForcedPageSize int`.
2. Mirror Task 5's helpers for the template item list (the scrollable template rows only — the pinned title + search band stay above the paged region):

```go
func (dv *DrumView) overflowTemplatePageSize() int {
	if dv.overflowForcedPageSize > 0 { return dv.overflowForcedPageSize }
	rh := touchMinTargetPx // template row height used today
	usable := dv.overflowScrollViewportH() // existing scroll-region height accessor; add if missing
	n := usable / rh; if n < 1 { n = 1 }
	return n
}
func (dv *DrumView) overflowForcePageSizeForTest(n int) { dv.overflowForcedPageSize = n }
func (dv *DrumView) overflowTemplatePageCount() int { return pageCount(dv.overflowTemplateItemCount(), dv.overflowTemplatePageSize()) }
func (dv *DrumView) overflowTemplatePageNext() { if dv.overflowTemplatePage < dv.overflowTemplatePageCount() { dv.overflowTemplatePage++; dv.rebuildOverflowBtns() } }
func (dv *DrumView) overflowTemplatePagePrev() { if dv.overflowTemplatePage > 1 { dv.overflowTemplatePage--; dv.rebuildOverflowBtns() } }
```
`overflowTemplateItemCount()` = the count of template *rows* (excluding pinned band + group headers, or including headers if you paginate the full scroll list — choose the same "one screenful" model as FX for consistency; document the choice in a code comment).

3. In `overflowItems`/`rebuildOverflowBtns`: when `overflowPage == 1`, slice the template rows with `pageSlice(..., overflowTemplatePageSize(), overflowTemplatePage)`. Reset `overflowTemplatePage = 1` on query change and on entering page-1.
4. In `drawOverflowMenu`: when `overflowPage == 1 && overflowTemplatePageCount() > 1`, reserve `menuPagerFooterH()` and `drawMenuPagerFooter(dst, overflowPopupRect(), i18n.Tf(i18n.KeyLogsPage, dv.overflowTemplatePage, dv.overflowTemplatePageCount()), prevBtn, nextBtn)`.
5. **Esc:** in the overflow menu's escape hook (`openOverflowMenuPortal` in `drumview_portal_open.go`), ensure the order: non-empty template query → clear query; else page-1 → return to page-0; else close. (Confirm current behavior first; only adjust to match this order.)
6. **Polish:** verify template rows already pass through `drawMenuRow` (they do) and that row insets/active-stripe match the instrument menu — if any inset constant differs, align it to the shared `SpaceMD`/`menuRowIconRect` used by `drawMenuRow`. No row-renderer changes.

- [ ] **Step 4: Run tests — pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu|TestOverflow' -v`
Expected: PASS. Full package → PASS.

- [ ] **Step 5: Add page-2 scene + screenshots**

```go
{Name: "template_menu_page2", Description: "template menu paginated to page 2 (shared footer pager)", SettleFrames: 120,
	Setup: func(g *Game) {
		g.drum.overflowPage = 1
		g.drum.OpenOverflowMenu()
		g.drum.overflowForcePageSizeForTest(6)
		g.drum.rebuildOverflowBtns()
		g.drum.overflowTemplatePageNext()
		g.drum.rebuildOverflowBtns()
	}},
```

Build + capture `template_menu_open` (polish intact) and `template_menu_page2` (pager engaged), desktop + mobile. Open PNGs: pager identical in style to logs/FX; rows/search/group headers unchanged; active template still tinted + striped. **Verification checkpoint — leave uncommitted.**

---

## Task 7: Full-suite verification + cross-cutting guards

**Files:** none (verification only).

- [ ] **Step 1: Full fast-path package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (all tests).

- [ ] **Step 2: Token + menu discipline guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestTokenDiscipline|TestDesignMDDrift|TestDrawMenuRowIsFlatNoKeycap' ./internal/ui/`
Expected: PASS. (If `TestTokenDiscipline` flags a new literal, replace it with a `Token*()`/`colXxx` accessor.)

- [ ] **Step 3: Broader Go suite (touched packages)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/audio ./internal/i18n`
Expected: PASS.

- [ ] **Step 4: Browser bridge smoke (only if a new JS export was added)**

No new JS export is expected in this plan. If one was added (e.g. an FX page-state export), register it in `src/js/wasm_bridge_smoke.browser.test.js`'s catalogue and run:
`GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` → PASS. Otherwise skip.

- [ ] **Step 5: Final screenshot sweep (desktop + mobile)**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
cd /home/ymolinar/Repos/beatmo
for s in instrument_menu_open template_menu_open template_menu_page2 fx_add_effect_menu fx_add_effect_menu_search fx_add_effect_menu_page2 logs_panel_page2; do
  xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene $s -screenshot /tmp/shots/$s.png
done
make screenshots-all SCENES=fx_add_effect_menu,fx_add_effect_menu_search,template_menu_open MOBILE=1
```
Open every PNG. Confirm the three menus now read as one family (flat rows, leading icon/swatch, search, group headers, identical footer pager) and the instrument menu + logs are unchanged. Fix + re-capture any that look wrong. **Final verification checkpoint — leave everything uncommitted for the user.**

---

## Self-review notes (author)

- **Spec coverage:** pager extraction+adaptability (T1), FX rows/Back/title (T2), FX search/focus/Esc/empty (T3), FX groups/trail/×N (T4), FX pager (T5), template polish+pager+Esc (T6), guards+screenshots (T7). Instrument menu untouched; instrument page-model unification explicitly deferred (spec "Future work"). All spec sections mapped.
- **i18n:** reuse `KeyLogsPage` for the shared "Page %d / %d" label (already in en+es) rather than adding `KeyMenuPage` — lower drift, pager is key-agnostic. (Refines the spec's tentative `KeyMenuPage`; noted so spec + plan agree.)
- **Type consistency:** `drawMenuPagerFooter`, `menuPagerFooterH`, `menuPagerBtnSize`, `fxAdd*`, `overflowTemplate*` names used identically across tasks. `pageCount`/`pageSlice` reused, not redefined.
- **Assumptions to verify at execution time (grep first):** exact FX test-harness constructors in `drumview_fx_panel_test.go`; `dv.fxRowName()`/`fxInstID()`/`fxViewportRect`/`fxPanelRect` accessor names; `audio.GetInsertEffects` element type + type field; the existing "no results" i18n key + empty-state helper in the instrument menu; the overflow menu's current escape-hook order; `KeyBack` key name. Each task says to match existing names when they differ from the placeholders here.
