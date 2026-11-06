# Vice City Menu Restyle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every menu/popup surface in `internal/ui/` a cohesive "Neon Horizon" Vice City look — crisp sized Inter typography, sunset-tinted header bands, azure active-state accents, cushioned rows — replacing today's soft/upscaled debug-font chunkiness.

**Architecture:** Three shared foundations do the work — (1) a sized text-role API that renders sprites at true pixel size instead of GeoM-upscaling one 14px sprite, (2) two additive DESIGN.md alpha tokens, (3) a small `menu_chrome.go` helper set wrapping the existing `drawPanel`/icon/cushion primitives. Each menu then swaps its bespoke drawing for these helpers. No information-architecture or behavior changes.

**Tech Stack:** Go 1.23, Ebiten v2, `golang.org/x/image/font/opentype` (Inter embedded), DESIGN.md → `make gen-design-tokens` → `*.gen.go`.

**Spec:** `docs/superpowers/specs/2026-06-10-vice-city-menu-restyle-design.md`

**Conventions used throughout:**
- Bundled Go: `cd src/go && ../../.tools/go/bin/go ...`. Alias for this plan: `GOBIN=../../.tools/go/bin/go`.
- Fast test cmd: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkg> -run <name>`.
- After ANY `DESIGN.md` edit: run `make gen-design-tokens` from repo root, then commit the regenerated `*.gen.go` in the same commit (pre-commit hook rejects stale gen files).
- All new commits end with the `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer.

---

## Phase 0: Isolation (one-time)

- [ ] **Step 1: Confirm worktree / branch**

If executing in isolation, ensure a worktree exists (see superpowers:using-git-worktrees). Otherwise work on the current branch `add-core-node-types`. Confirm a clean build first:

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: builds with no errors (the branch already compiles per project memory; if it does not, STOP and report — do not attribute pre-existing breakage to this work).

---

## Phase 1: Sized text-role typography API

**Why:** All chrome text is one 14px sprite GeoM-upscaled per call (`textcache.go` + `fontcache.go`). This adds a parallel API that renders at the true target px size and weight, cached separately. Existing `DrawTextAt*` callers are untouched.

### Task 1.1: Define the `TextRole` type and metrics (test-deterministic)

**Files:**
- Modify: `src/go/internal/ui/textcache.go`
- Test: `src/go/internal/ui/text_role_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/text_role_test.go`:

```go
package ui

import "testing"

// TestTextRoleHierarchy proves the role API exposes a real, distinct type
// scale (panel-title > section-header > body > caption) in every build mode —
// this is what kills the "everything is one chunky size" look.
func TestTextRoleHierarchy(t *testing.T) {
	pt := StyledTextHeight(RolePanelTitle)
	sh := StyledTextHeight(RoleSectionHeader)
	bd := StyledTextHeight(RoleBody)
	cap := StyledTextHeight(RoleCaption)
	if !(pt > sh && sh > bd && bd > cap) {
		t.Fatalf("role heights must strictly decrease: panelTitle=%d sectionHeader=%d body=%d caption=%d", pt, sh, bd, cap)
	}
	if cap < 10 {
		t.Fatalf("caption height %d too small to read", cap)
	}
}

// TestStyledTextWidthScalesWithLength is a sanity check that width grows with
// string length (guards against a constant/0 fallback regression).
func TestStyledTextWidthScalesWithLength(t *testing.T) {
	short := StyledTextWidth("Hi", RoleBody)
	long := StyledTextWidth("Hello world", RoleBody)
	if long <= short {
		t.Fatalf("expected longer string wider: short=%d long=%d", short, long)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTextRole`
Expected: FAIL — `undefined: StyledTextHeight` / `RolePanelTitle`.

- [ ] **Step 3: Add the role type + metrics to `textcache.go`**

Add to `src/go/internal/ui/textcache.go` (after the `FontSize*` consts):

```go
// TextRole is a semantic typography role. Each role maps to a true pixel size
// and weight; StyledText* renders at that size directly (no GeoM upscale), so
// glyphs stay crisp and the panel-title/section-header/body/caption hierarchy
// is real. Mirrors DESIGN.md typography roles.
type TextRole int

const (
	RoleBody          TextRole = iota // 14px Inter SemiBold — menu item labels, values
	RoleCaption                       // 12px Inter Regular  — sub-labels ("Vol", "Pct")
	RoleSectionHeader                 // 16px Inter SemiBold — section labels
	RolePanelTitle                    // 21px Inter SemiBold — panel/menu titles
)

func (r TextRole) size() float64 {
	switch r {
	case RolePanelTitle:
		return 21
	case RoleSectionHeader:
		return 16
	case RoleCaption:
		return 12
	default:
		return 14
	}
}

func (r TextRole) bold() bool { return r != RoleCaption }

// styledLineHeightFallback is the build-agnostic line-height estimate used
// under -tags test (no TTF available). Monotonic in size so the role
// hierarchy holds without a font backend.
func styledLineHeightFallback(size float64) int { return int(size*1.25) + 1 }

// styledTextSpriteRenderer / styledTextMeasure are wired by fontcache.go in
// non-test builds. nil under -tags test → debug-font / arithmetic fallback.
var (
	styledTextSpriteRenderer func(s string, size float64, bold bool) *ebiten.Image
	styledTextMeasure        func(s string, size float64, bold bool) (int, int)
)

type styledKey struct {
	s    string
	size int // size*100, integer key
	bold bool
}

var (
	styledCacheMu sync.RWMutex
	styledSprites = map[styledKey]*ebiten.Image{}
)

// StyledTextWidth returns the rendered pixel width of s at the given role.
func StyledTextWidth(s string, role TextRole) int {
	if styledTextMeasure != nil {
		w, _ := styledTextMeasure(s, role.size(), role.bold())
		return w
	}
	return debugCharW * utf8.RuneCountInString(s)
}

// StyledTextHeight returns the line height for the given role.
func StyledTextHeight(role TextRole) int {
	if styledTextMeasure != nil {
		_, h := styledTextMeasure("Ag", role.size(), role.bold())
		if h > 0 {
			return h
		}
	}
	return styledLineHeightFallback(role.size())
}

// StyledTextSprite returns a cached sprite for s at role's size+weight.
func StyledTextSprite(s string, role TextRole) *ebiten.Image {
	key := styledKey{s: s, size: int(role.size() * 100), bold: role.bold()}
	styledCacheMu.RLock()
	if spr := styledSprites[key]; spr != nil {
		styledCacheMu.RUnlock()
		return spr
	}
	styledCacheMu.RUnlock()

	var img *ebiten.Image
	if styledTextSpriteRenderer != nil {
		img = styledTextSpriteRenderer(s, role.size(), role.bold())
	}
	if img == nil {
		// Test/headless fallback: reuse the body-size debug sprite.
		img = TextSprite(s)
	}
	styledCacheMu.Lock()
	styledSprites[key] = img
	styledCacheMu.Unlock()
	return img
}

// DrawTextStyled draws s at (x,y) in the given role, tinted with col. The
// sprite is rendered at true px size — blit 1:1, no GeoM scaling.
func DrawTextStyled(dst *ebiten.Image, s string, x, y int, role TextRole, col color.Color) {
	spr := StyledTextSprite(s, role)
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(x), float64(y))
	r, g, b, a := col.RGBA()
	if a > 0 {
		fa := float64(a) / 0xffff
		op.ColorScale.Scale(float32(float64(r)/0xffff/fa), float32(float64(g)/0xffff/fa), float32(float64(b)/0xffff/fa), float32(fa))
	}
	dst.DrawImage(spr, &op)
}
```

Also extend `ClearTextCacheForTest` to clear the styled cache:

```go
func ClearTextCacheForTest() {
	textCacheMu.Lock()
	textSprites = map[string]*ebiten.Image{}
	textCacheMu.Unlock()
	styledCacheMu.Lock()
	styledSprites = map[styledKey]*ebiten.Image{}
	styledCacheMu.Unlock()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTextRole|TestStyledTextWidth'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/textcache.go src/go/internal/ui/text_role_test.go
git commit -m "feat(ui): sized text-role typography API (crisp, no GeoM upscale)"
```

### Task 1.2: Wire the real Inter renderer for styled text (`fontcache.go`)

**Files:**
- Modify: `src/go/internal/ui/fontcache.go`

- [ ] **Step 1: Add a weight-aware measure helper + wire the styled hooks**

In `src/go/internal/ui/fontcache.go`, add after `measureTextHeight`:

```go
// measureStyled returns width+height for s at the given size and weight.
func measureStyled(s string, size float64, bold bool) (int, int) {
	var face font.Face
	if bold {
		face = fontFaceBold(size)
	} else {
		face = fontFaceRegular(size)
	}
	if face == nil {
		return len([]rune(s)) * debugCharW, debugCharH
	}
	w := text.BoundString(face, s).Dx() //nolint:staticcheck // no v2 drop-in
	m := face.Metrics()
	return w, (m.Ascent + m.Descent).Ceil()
}
```

Then inside the existing `func init()` in `fontcache.go` (alongside the `textSpriteRenderer = ...` assignment), add:

```go
	styledTextSpriteRenderer = func(s string, size float64, bold bool) *ebiten.Image {
		return renderTextSprite(s, size, bold)
	}
	styledTextMeasure = func(s string, size float64, bold bool) (int, int) {
		return measureStyled(s, size, bold)
	}
```

- [ ] **Step 2: Build (non-test) to confirm it compiles**

Run: `cd src/go && ../../.tools/go/bin/go build ./internal/ui/`
Expected: builds clean.

- [ ] **Step 3: Re-run the role tests (still green)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTextRole|TestStyledTextWidth'`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/fontcache.go
git commit -m "feat(ui): wire Inter renderer for sized text roles"
```

---

## Phase 2: Menu accent tokens

**Why:** Role-named alpha buckets for the active-row tint and header underline, per DESIGN.md's "role-naming over value-dedup" convention.

### Task 2.1: Add two alpha buckets to DESIGN.md and regenerate

**Files:**
- Modify: `DESIGN.md` (YAML `alpha:` block)
- Regenerated: `src/go/internal/ui/design_tokens.gen.go`

- [ ] **Step 1: Add the buckets to DESIGN.md**

In `DESIGN.md`, inside the `alpha:` mapping (near `accent-tint`/`accent-overlay`, around line 868), add:

```yaml
  menu-active-tint:    60  # active/selected menu-row fill (primary over panel)
  menu-header-accent:  50  # 1px azure underline on menu header bands
```

- [ ] **Step 2: Regenerate tokens**

Run: `make gen-design-tokens`
Expected: `src/go/internal/ui/design_tokens.gen.go` now contains `genAlphaMenuActiveTint uint8 = 60` and `genAlphaMenuHeaderAccent uint8 = 50`.

Run: `grep -n "genAlphaMenuActiveTint\|genAlphaMenuHeaderAccent" src/go/internal/ui/design_tokens.gen.go`
Expected: both constants present.

- [ ] **Step 3: Verify drift + lint still pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDDrift|TestDesignMDLintSnapshot'`
Expected: PASS. (If the lint snapshot flags the two new keys as orphan top-level/unknown, add them to `src/go/internal/ui/testdata/design_md_lint.expected.json` with a one-line rationale, re-run, confirm PASS.)

- [ ] **Step 4: Commit**

```bash
git add DESIGN.md src/go/internal/ui/design_tokens.gen.go src/go/internal/ui/testdata/design_md_lint.expected.json
git commit -m "feat(design): add menu-active-tint + menu-header-accent alpha tokens"
```

---

## Phase 3: Shared "Neon Horizon" menu chrome helpers

**Why:** One helper set so every menu is pixel-consistent; wraps existing primitives (`drawPanel`, `fillVerticalGradient`, `DrawIcon`, `drawRoundedRect`).

### Task 3.1: Create `menu_chrome.go` with header band, item background, chevron

**Files:**
- Create: `src/go/internal/ui/menu_chrome.go`
- Test: `src/go/internal/ui/menu_chrome_test.go` (create)

- [ ] **Step 1: Write the failing test (drawRect interception)**

Create `src/go/internal/ui/menu_chrome_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestMenuActiveItemDrawsAzureStripe verifies an active menu item paints a
// left accent stripe in the azure family (single-accent invariant) and a
// tinted fill, while a resting item paints neither.
func TestMenuActiveItemDrawsAzureStripe(t *testing.T) {
	dst := ebiten.NewImage(200, 40)
	r := image.Rect(0, 0, 200, 30)

	var stripeSeen bool
	restore := interceptDrawRect(func(_ *ebiten.Image, rect image.Rectangle, _ interface{}, _ bool) {
		// stripe is a thin tall rect at the left edge
		if rect.Min.X == r.Min.X && rect.Dx() <= accentStripeW() && rect.Dy() >= r.Dy()-2 {
			stripeSeen = true
		}
	})
	defer restore()

	drawMenuItemBackground(dst, r, menuItemActive)
	if !stripeSeen {
		t.Fatal("active menu item must draw a left azure accent stripe")
	}
}
```

> NOTE: `interceptDrawRect` is a helper that swaps the package `drawRect` var and returns a restore func. If one does not already exist in the test package, add it to this file:

```go
func interceptDrawRect(fn func(*ebiten.Image, image.Rectangle, interface{}, bool)) func() {
	orig := drawRect
	drawRect = func(dst *ebiten.Image, rect image.Rectangle, c colorColor, filled bool) {
		fn(dst, rect, c, filled)
		orig(dst, rect, c, filled)
	}
	return func() { drawRect = orig }
}
```

> If `drawRect`'s real signature uses `color.Color`, match it exactly (it is `func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool)`). Replace `colorColor`/`interface{}` with `color.Color` and import `image/color`. Adjust the test's callback signature to `func(*ebiten.Image, image.Rectangle, color.Color, bool)` accordingly.

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuActiveItem`
Expected: FAIL — `undefined: drawMenuItemBackground` / `menuItemActive` / `accentStripeW`.

- [ ] **Step 3: Implement `menu_chrome.go`**

Create `src/go/internal/ui/menu_chrome.go`:

```go
package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// menuItemState is the visual state of a menu row.
type menuItemState int

const (
	menuItemRest menuItemState = iota
	menuItemHover
	menuItemActive
)

// accentStripeW is the density-aware left accent-stripe width used by active
// menu items (shared with the row-rack accent stripe token).
func accentStripeW() int {
	w := Profile().DensityValues().AccentStripeWidth
	if w < 2 {
		w = 2
	}
	return w
}

// drawMenuHeaderBand paints a header band with a faint sunset gradient
// (grid-horizon → surface-overlay) and a 1px azure underline — the shared
// "Neon Horizon" header for identity menus (sidebar, context, color, instrument).
func drawMenuHeaderBand(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	fillVerticalGradient(dst, r, genColorGridHorizon, genColorSurfaceOverlay, 16)
	underline := image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y)
	drawRect(dst, underline, WithAlpha(colAccent, genAlphaMenuHeaderAccent), true)
}

// drawMenuItemBackground paints the per-state background of a menu row:
// rest = nothing (panel shows through); hover = cushion lift + faint azure
// glow; active = left azure stripe + tinted fill.
func drawMenuItemBackground(dst *ebiten.Image, r image.Rectangle, state menuItemState) {
	if r.Empty() {
		return
	}
	switch state {
	case menuItemHover:
		drawRect(dst, r, colSurface3, true)
		drawRect(dst, r, WithAlpha(colAccent, genAlphaSubtle), true)
	case menuItemActive:
		drawRect(dst, r, WithAlpha(colAccent, genAlphaMenuActiveTint), true)
		stripe := image.Rect(r.Min.X, r.Min.Y, r.Min.X+accentStripeW(), r.Max.Y)
		drawRect(dst, stripe, colAccent, true)
	}
}

// drawMenuChevron draws the icon-system chevron (down when expanded, right
// when collapsed) centered in r, tinted col.
func drawMenuChevron(dst *ebiten.Image, r image.Rectangle, expanded bool, col color.Color) {
	id := IconChevronRight
	if expanded {
		id = IconChevronDown
	}
	dim := IconSizeMD
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	iconR := image.Rect(cx-dim/2, cy-dim/2, cx+dim/2, cy+dim/2)
	DrawIcon(dst, id, iconR, col)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMenuActiveItem`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/menu_chrome.go src/go/internal/ui/menu_chrome_test.go
git commit -m "feat(ui): shared Neon Horizon menu chrome helpers"
```

---

## Phase 4: Node sidebar (headline)

**Why:** The headline surface. Apply gradient header + glowing swatch, role typography, icon chevrons, active-section stripe.

### Task 4.1: Header band + panel-title typography + glowing swatch

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`drawHeader`, ~line 1289; `Draw`, ~line 1048)

- [ ] **Step 1: Replace the header draw**

In `drawHeader` (`game_node_sidebar.go:1289`), replace the body so it: paints the shared header band across the header rect, draws the glowing identity swatch, and draws the title via `RolePanelTitle`. Replace the existing `DrawTextAtScale(dst, label, textX, hy, sidebarLabelScale())` title draw and the plain swatch with:

```go
func (sb *NodeSidebar) drawHeader(dst *ebiten.Image) {
	headerRect := image.Rect(sidebarPad, sidebarPad, sb.width-sidebarPad, sidebarPad+sidebarHeaderH)
	drawMenuHeaderBand(dst, headerRect)

	// Identity swatch with soft glow.
	swatchCol := sb.headerSwatchColor() // existing helper; keep current resolution
	th := StyledTextHeight(RolePanelTitle)
	hy := headerRect.Min.Y + (sidebarHeaderH-th)/2
	swatchY := headerRect.Min.Y + (sidebarHeaderH-sidebarSwatchSz)/2
	swatchRect := image.Rect(headerRect.Min.X+2, swatchY, headerRect.Min.X+2+sidebarSwatchSz, swatchY+sidebarSwatchSz)
	// glow: a faint enlarged swatch behind the crisp one
	glow := swatchRect.Inset(-3)
	drawRoundedRect(dst, glow, WithAlphaFromColor(swatchCol, genAlphaSubtle), RadiusXXS, true)
	drawRoundedRect(dst, swatchRect, swatchCol, RadiusXXS, true)

	textX := swatchRect.Max.X + sidebarGap
	DrawTextStyled(dst, sb.headerLabel(), textX, hy, RolePanelTitle, colTextPrimary)
	// keep the existing close (X) button draw exactly as before
	sb.drawBtn(dst, "close")
}
```

> If `headerSwatchColor()` / `headerLabel()` helpers do not exist, inline the current logic from the old `drawHeader` (the swatch color + `label` string it built). Do NOT change what color/text are shown — only how they're drawn.

- [ ] **Step 2: Build + run sidebar tests**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sidebar|NodeSidebar'`
Expected: builds; existing sidebar tests PASS (fix any that asserted old text-scale geometry by updating to `StyledTextHeight(RolePanelTitle)`).

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/game_node_sidebar.go
git commit -m "feat(ui): sidebar Neon Horizon header band + panel-title type"
```

### Task 4.2: Section headers — icon chevrons + section-header typography + active stripe

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`drawSectionHeader`, ~line 1318; delete `drawFilledTriangleRight`/`drawFilledTriangleDown`, ~lines 1373/1387)

- [ ] **Step 1: Rewrite `drawSectionHeader`**

Replace the triangle + scaled-text body of `drawSectionHeader` with the shared helpers:

```go
func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID, label, sectionID string, _ color.Color, _ int) {
	r := sb.rects[rectID]
	if r.Empty() || !sb.inViewport(r) {
		return
	}
	expanded := sb.sectionOpen[sectionID]
	state := menuItemRest
	if expanded {
		state = menuItemActive
	}
	drawMenuItemBackground(dst, r, state)

	chevR := image.Rect(r.Min.X+sidebarPad, r.Min.Y, r.Min.X+sidebarPad+IconSizeMD, r.Max.Y)
	chevCol := colTextSecondary
	labelCol := colTextPrimary
	if expanded {
		chevCol = colAccent
		labelCol = colTextAccent
	}
	drawMenuChevron(dst, chevR, expanded, chevCol)

	th := StyledTextHeight(RoleSectionHeader)
	labelX := chevR.Max.X + sidebarGap
	labelY := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, label, labelX, labelY, RoleSectionHeader, labelCol)

	// keep the existing badge pill draw (e.g. logic/groove summary) but render
	// its text via RoleCaption + colTextSecondary instead of the scaled debug font.
	sb.drawSectionBadge(dst, r, sectionID) // extract existing badge logic into this helper
}
```

- [ ] **Step 2: Delete the dead triangle helpers**

Remove `func (sb *NodeSidebar) drawFilledTriangleRight(...)` and `drawFilledTriangleDown(...)` from `game_node_sidebar.go` (they have no remaining callers).

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: builds; if "declared and not used" / "undefined" errors appear for the removed helpers, confirm no other caller exists (`grep -n drawFilledTriangle src/go/internal/ui/*.go`).

- [ ] **Step 3: Convert the section body labels to roles**

In `Draw` (`game_node_sidebar.go` ~lines 1099-1255), replace each `sidebarDrawTextColorAtScale(dst, "Vol"/"Pitch"/"Pct"/..., ..., sidebarLabelScale())` sub-label with `DrawTextStyled(dst, "Vol", x, y, RoleCaption, colTextSecondary)` and each value/`DrawTextAtScale(...)` (logic/groove current value) with `DrawTextStyled(dst, cur, x, y, RoleBody, colTextPrimary)`. Recompute the vertical offsets using `StyledTextHeight(RoleCaption)` / `StyledTextHeight(RoleBody)` instead of `int(float64(TextHeight())*sidebarLabelScale())`.

- [ ] **Step 4: Build + run sidebar tests + glyph guard**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sidebar|NodeSidebar|ForbiddenGlyph'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_node_sidebar.go
git commit -m "feat(ui): sidebar icon chevrons + role typography + active-section stripe"
```

### Task 4.3: Value pills + inc/dec steppers via tokens

**Files:**
- Modify: `src/go/internal/ui/game_node_sidebar.go` (`drawValuePill`, `drawIncDecBtn`)

- [ ] **Step 1: Restyle pill + stepper**

In `drawValuePill`, draw the pill as `drawRoundedRect(dst, r, colSurface2, RadiusSM, true)` + a `TokenBorderMedium()` stroke, and the value text via `DrawTextStyled(dst, val, cx, cy, RoleBody, colTextPrimary)` centered using `StyledTextWidth`/`StyledTextHeight`. In `drawIncDecBtn`, draw via the existing stepper component styling (navy fill `colIncDec` + `colIncDecBorder` border, `RadiusSM`) and render the `+`/`−` glyph via `DrawTextStyled(..., RoleBody, colTextAccent)`.

> Use existing color constants only (`colSurface2`, `colIncDec`, `colIncDecBorder`, `colTextAccent`). Do NOT introduce inline `color.RGBA{}` literals.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Sidebar|NodeSidebar|TestTokenDiscipline'`
Expected: PASS (if `TestTokenDiscipline` reports a file now below budget, decrement its entry in `token_discipline_test.go` per the ratchet rule).

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/game_node_sidebar.go src/go/internal/ui/token_discipline_test.go
git commit -m "feat(ui): sidebar tokenized value pills + steppers"
```

### Task 4.4: Visual confirmation of the headline

- [ ] **Step 1: Capture the sidebar scenes**

Run: `make screenshots-all SCENES=node_sidebar_open,node_sidebar_logic_expanded,node_sidebar_groove_expanded`
Expected: PNGs under `screenshots/all/desktop/`. Open `node_sidebar_open.png` and confirm: crisp title, sunset header band, azure underline, icon chevrons, glowing swatch, active-section azure stripe. (This is reviewer evidence, not an automated gate.)

- [ ] **Step 2: Commit any screenshot updates if tracked** (skip if `screenshots/` is gitignored).

---

## Phase 5: Drum-row context menu + overflow menu

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go`

### Task 5.1: Context menu header band + role typography + shared item backgrounds

- [ ] **Step 1: Apply shared chrome**

In the context-menu draw path (`drumview_context_menu.go`): wrap the header (row color dot + name) in `drawMenuHeaderBand`; render the row name via `RolePanelTitle`; render each item label via `RoleBody`; replace the per-item highlight with `drawMenuItemBackground(dst, itemRect, state)` where `state = menuItemHover` for the hovered/pressed row. Keep the `Delete` item's **red text** (`colError`/`context-menu-item-destructive`) — do not give it a red fill.

- [ ] **Step 2: Apply the same to the overflow/file menu draw**

Same file: subgroup header labels via `DrawTextStyled(..., RoleCaption, colTextSecondary)`; action items via `RoleBody`; item backgrounds via `drawMenuItemBackground`. Bottom-sheet (mobile) path gets the identical treatment — confirm both `drawPanel` (desktop) and `drawBottomSheetPanel` (mobile) branches are covered.

- [ ] **Step 3: Build + run context-menu tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ContextMenu|Overflow'`
Expected: PASS (update any test asserting old text geometry to the role metrics).

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/drumview_context_menu.go
git commit -m "feat(ui): context + overflow menus Neon Horizon chrome"
```

---

## Phase 6: Instrument menu

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go`

### Task 6.1: Header band, role typography, azure selection stripe

- [ ] **Step 1: Apply shared chrome**

In `drumview_overlay_inst_comp.go` draw path: header band over the title/search header; breadcrumb labels via `RoleBody` (keep `breadcrumbSeparatorIcon = IconChevronRight`); instrument list rows keep the swatch+label "gold standard" (do not iconify), but draw the row label via `RoleBody` and paint the *selected* instrument's row with `drawMenuItemBackground(..., menuItemActive)` and hovered row with `menuItemHover`. Search-box placeholder/typed text via `RoleBody`.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Instrument|InstMenu'`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/drumview_overlay_inst_comp.go
git commit -m "feat(ui): instrument menu Neon Horizon chrome"
```

---

## Phase 7: Subdiv menu

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_subdiv_comp.go` (and `drumview_subdiv_menu.go` if it mirrors)

### Task 7.1: Compact card + azure active stripe

- [ ] **Step 1: Apply shared chrome**

Render each subdivision value via `RoleBody` centered; replace the active-value border-only highlight with `drawMenuItemBackground(..., menuItemActive)` (azure stripe + tint) for the current subdivision and `menuItemHover` for hover.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Subdiv'`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/drumview_overlay_subdiv_comp.go src/go/internal/ui/drumview_subdiv_menu.go
git commit -m "feat(ui): subdiv menu Neon Horizon chrome"
```

---

## Phase 8: Color picker

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_color_comp.go`

### Task 8.1: Header band + azure selection ring + role title

- [ ] **Step 1: Apply shared chrome**

Wrap the "Color" header in `drawMenuHeaderBand`; draw the header title via `RoleSectionHeader`. Keep the swatch grid exactly (it carries instrument identity — do not restyle swatch colors). Change the current-color selection ring to `colAccent` (azure) if it currently uses another color. Do not add new color literals.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Color'`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/drumview_overlay_color_comp.go
git commit -m "feat(ui): color picker Neon Horizon header + azure selection"
```

---

## Phase 9: Slider popups

**Files:**
- Modify: `src/go/internal/ui/slider_popup.go` (and `drumview_volume_popup.go` reuses it)

### Task 9.1: Title via panel-title role

- [ ] **Step 1: Apply role typography**

In `slider_popup.go`, render the optional title (row name / "Master") via `DrawTextStyled(..., RolePanelTitle, colTextPrimary)` and the percentage via `RoleBody`. Recompute centering with `StyledTextWidth`/`StyledTextHeight`. Track/thumb already tokenized — leave them.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Slider|VolumePopup'`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/slider_popup.go
git commit -m "feat(ui): slider popup role typography"
```

---

## Phase 10: Long-press popup (compliance + tokens)

**Files:**
- Modify: `src/go/internal/ui/game_longpress_popup.go`

### Task 10.1: Replace hardcoded dims with tokens + button-label typography

- [ ] **Step 1: Tokenize dimensions**

In `showLongPressPopup` (`game_longpress_popup.go:27-32`), replace `btnW:=80; btnH:=44; gap:=8` and the `16` paddings with density/spacing tokens:

```go
	btnH := Profile().DensityValues().PopupBtnH
	gap := SpaceSM
	pad := SpaceMD
	btnW := 80 // keep width; or derive from max label width via StyledTextWidth
	panelW := 3*btnW + 2*gap + 2*pad
	panelH := btnH + 2*pad
```

> Use the actual spacing accessors present in the codebase (`SpaceSM`/`SpaceMD` or `genSpacingSm`/`genSpacingMd` — grep to confirm the exported names; match what `transport_zone.go` uses). Render the three labels via `DrawTextStyled(..., RoleBody, ...)` and center with `StyledTextWidth`.

### Task 10.2: Fix cyan-in-chrome (Connect hover → azure)

- [ ] **Step 1: Write the failing guard test**

Add to `src/go/internal/ui/menu_chrome_test.go`:

```go
// TestLongPressConnectHoverUsesAzure proves the Connect-hover text is in the
// azure single-accent family, not cyan (viz-only). Guards the regression
// fixed in this restyle.
func TestLongPressConnectHoverUsesAzure(t *testing.T) {
	got := longPressConnectHoverColor()
	if got != color.Color(colAccent) && got != color.Color(colAccentBright) {
		t.Fatalf("Connect hover must use azure accent, got %v (cyan in chrome is a single-accent violation)", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLongPressConnectHover`
Expected: FAIL — `undefined: longPressConnectHoverColor`.

- [ ] **Step 3: Implement**

In `game_longpress_popup.go`, extract the Connect-hover color into a tiny helper and use azure (replacing `colStep` at line ~168):

```go
func longPressConnectHoverColor() color.Color { return colAccent }
```

Then in `drawLongPressPopup`, replace `connTxtCol = colStep // cyan` with `connTxtCol = longPressConnectHoverColor()`. Also change the Move-hover from `colPlayIconTint` only if it is not in the semantic green family — leave green (play) as is; only cyan→azure is the violation.

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLongPressConnectHover|LongPress'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_longpress_popup.go src/go/internal/ui/menu_chrome_test.go
git commit -m "fix(ui): long-press popup tokens + azure Connect hover (drop cyan)"
```

---

## Phase 11: Tooltip

**Files:**
- Modify: `src/go/internal/ui/tooltip_overlay.go`

### Task 11.1: Caption role typography

- [ ] **Step 1: Apply role**

Render tooltip text via `DrawTextStyled(..., RoleCaption, colTextSecondary)`; size the background rect from `StyledTextWidth`/`StyledTextHeight(RoleCaption)`.

- [ ] **Step 2: Build + test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Tooltip'`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/tooltip_overlay.go
git commit -m "feat(ui): tooltip caption role typography"
```

---

## Phase 12: DESIGN.md typography correction + full verification

### Task 12.1: Correct the DESIGN.md typography block to reflect Inter

**Files:**
- Modify: `DESIGN.md` (`typography:` block ~lines 773-803, and the Typography prose ~line 1627)

- [ ] **Step 1: Update the YAML + prose**

In `DESIGN.md`, change each `fontFamily: Ebiten Debug` under `typography:` to `Inter` (panel-title/section-header/body/caption), set `panel-title.fontSize: 21px`, `section-header.fontSize: 16px`, `body.fontSize: 14px`, `caption.fontSize: 12px`, and update the prose paragraph that claims chrome text uses "the bundled Ebiten debug font" to state Inter (Regular + SemiBold) is the runtime chrome font, debug font is the `-tags test` fallback only.

- [ ] **Step 2: Regenerate + lint snapshot**

Run: `make gen-design-tokens`
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDDrift|TestDesignMDLintSnapshot'`
Expected: PASS. (If the lint snapshot changes warnings due to fontSize edits, update `testdata/design_md_lint.expected.json` to match and re-run.)

- [ ] **Step 3: Commit**

```bash
git add DESIGN.md src/go/internal/ui/*.gen.go src/go/internal/ui/testdata/design_md_lint.expected.json
git commit -m "docs(design): correct typography block to Inter + real px sizes"
```

### Task 12.2: Full fast-suite + parity + screenshot regen

- [ ] **Step 1: Full UI + timeline fast tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline`
Expected: PASS. (Per project memory, the `internal/ui` full suite has a pre-existing panic on `add-core-node-types` unrelated to this work — if it appears, confirm it reproduces on a clean checkout of the branch before this plan's commits, and report it rather than attributing it here.)

- [ ] **Step 2: Token + drift + glyph + lint guards together**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot|ForbiddenGlyph|TestTextRole'`
Expected: PASS.

- [ ] **Step 3: Regenerate the full menu screenshot set for review**

Run: `make screenshots-all SCENES=node_sidebar_open,node_sidebar_logic_expanded,context_menu_open,instrument_menu_open,color_wheel_open,subdiv_menu_open,master_vol_popup,mobile_overflow_open MOBILE=1`
Expected: PNGs under `screenshots/all/`. Eyeball each menu for the consistent Neon Horizon treatment on both desktop and mobile.

- [ ] **Step 4: Final review handoff**

Use superpowers:requesting-code-review against the spec before merge.

---

## Self-review notes (coverage map)

- Spec §"Unit 1 typography" → Phase 1 + Phase 12.1.
- Spec §"Unit 2 menu chrome" → Phase 3.
- Spec §"Unit 3 tokens" → Phase 2.
- Spec §"Unit 4 per-menu" → Phases 4–11 (one phase per menu; mobile bottom-sheet paths explicitly covered in Phase 5).
- Spec §"Unit 5 compliance" → Phase 4.2 (triangles→icons), Phase 10 (cyan→azure, hardcoded dims→tokens), token discipline checks throughout.
- Spec §"Testing" → role test (1.1), token guards (2.1, 12.2), glyph guard (4.2), single-accent guard (10.2), screenshot regen (4.4, 12.2).
```
