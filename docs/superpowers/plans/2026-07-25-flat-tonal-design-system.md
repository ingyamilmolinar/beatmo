# Flat-Tonal Design System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Beatmo's matte-keycap button chrome with a flat-tonal system app-wide, so every button, tab, menu, and control reads like the Synth tab's param cards.

**Architecture:** All five button variants keep flowing through the existing `ButtonStyle` / `Button.Draw` chokepoint, so most surfaces restyle without being edited. The keycap helpers in `button_cushion.go` are deleted and their eight call sites migrated to a new flat primitive. A new group-level `SegmentGroup` owns the shared track and sliding indicator for tab strips. Value-hint affordances move out of `param_card.go` into shared primitives. No input, layout, or tree changes.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten 2.9.9, generated design tokens (`make gen-design-tokens`), ebitenstub test harness (`-tags test -modfile=go.test.mod`), xvfb screenshot harness.

## Global Constraints

- **Never `git add` or `git commit`.** The user manages all staging. Where this plan's steps say "verify", stop there — do not commit. (Standing instruction, 2026-06-18.)
- **Bundled Go only:** `.tools/go/bin/go`, never system `go`.
- Fast test command: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
- **Zero idle work:** a control at rest or steadily latched must perform zero per-frame allocation and zero per-frame draw. Pinned by `button_keycap_alloc_test.go`.
- **No new opacity values.** Use `alpha:` buckets via `WithAlpha(token, AlphaX)`. Active/danger tint = `AlphaSubtle` (60).
- **No forbidden glyphs** (`▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←`) in `internal/ui/` — use `IconID` + `DrawIcon`.
- **Token discipline ratchet:** `TestTokenDiscipline` fails if a file's inline-literal count is *above or below* its budget. Every removal of a `color.RGBA{}` literal requires decrementing the file's entry in `allowedLiteralBudget`.
- Surface ladder: `background #120A1C` → `surface-1 #1C1230` → `surface-2 #281A40` → `surface-3 #382354`.
- **Visual verification is mandatory** (CLAUDE.md). Task 10 is a gate, not optional.

---

### Task 1: Flat button primitives

Additive only — nothing calls these yet, so the tree stays green.

**Files:**
- Create: `src/go/internal/ui/button_flat.go`
- Create: `src/go/internal/ui/button_flat_test.go`

**Interfaces:**
- Produces:
  - `func drawFlatButtonBody(dst *ebiten.Image, r image.Rectangle, fill color.Color, rad int)`
  - `func drawFlatSelectionBar(dst *ebiten.Image, r image.Rectangle, col color.Color, edge selectionEdge, frac float64)`
  - `type selectionEdge int` with `selectionEdgeBottom`, `selectionEdgeLeft`
  - `const flatSelectionBarPx = 2`
  - `func flatStateFill(base color.Color, st ComponentState) color.Color`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A flat body emits exactly one fill rect and never a border stroke.
func TestFlatButtonBodyHasNoBorder(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 90, 40)
	rects, filled := captureDrawRects(t, func() {
		drawFlatButtonBody(dst, r, colSurface2, RadiusSM)
	})
	if len(rects) != 1 {
		t.Fatalf("flat body emitted %d rects, want exactly 1 (no border stroke)", len(rects))
	}
	if !filled[0] {
		t.Fatal("flat body rect must be filled, not stroked")
	}
	if rects[0] != r {
		t.Fatalf("flat body rect = %v, want %v (no keycap wall inset)", rects[0], r)
	}
}

// The selection bar sits on the bottom edge, is flatSelectionBarPx tall, and
// scales its width by frac so hover can grow it 0 -> full.
func TestFlatSelectionBarBottomEdge(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeBottom, 1.0)
	})
	if len(rects) != 1 {
		t.Fatalf("selection bar emitted %d rects, want 1", len(rects))
	}
	got := rects[0]
	if got.Dy() != flatSelectionBarPx {
		t.Fatalf("bar height = %d, want %d", got.Dy(), flatSelectionBarPx)
	}
	if got.Max.Y != r.Max.Y {
		t.Fatalf("bar bottom = %d, want %d (flush with control bottom)", got.Max.Y, r.Max.Y)
	}
	if got.Dx() != r.Dx() {
		t.Fatalf("bar width at frac=1 = %d, want %d", got.Dx(), r.Dx())
	}
}

func TestFlatSelectionBarFractionalWidthCentered(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40) // 100 wide
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeBottom, 0.5)
	})
	got := rects[0]
	if got.Dx() != 50 {
		t.Fatalf("bar width at frac=0.5 = %d, want 50", got.Dx())
	}
	if cx := (got.Min.X + got.Max.X) / 2; cx != (r.Min.X+r.Max.X)/2 {
		t.Fatalf("bar center = %d, want %d (grows from center)", cx, (r.Min.X+r.Max.X)/2)
	}
}

func TestFlatSelectionBarLeftEdge(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeLeft, 1.0)
	})
	got := rects[0]
	if got.Dx() != flatSelectionBarPx {
		t.Fatalf("left bar width = %d, want %d", got.Dx(), flatSelectionBarPx)
	}
	if got.Min.X != r.Min.X {
		t.Fatalf("left bar x = %d, want %d", got.Min.X, r.Min.X)
	}
	if got.Dy() != r.Dy() {
		t.Fatalf("left bar height = %d, want full %d", got.Dy(), r.Dy())
	}
}

func TestFlatSelectionBarZeroFracDrawsNothing(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, image.Rect(10, 10, 110, 40), colAccent, selectionEdgeBottom, 0)
	})
	if len(rects) != 0 {
		t.Fatalf("frac=0 emitted %d rects, want 0 (zero idle work)", len(rects))
	}
}

// Rest returns the base untouched; hover lifts; press dims. Exact deltas
// mirror the retained DESIGN.md interaction values.
func TestFlatStateFillDeltas(t *testing.T) {
	base := colSurface2
	if got := flatStateFill(base, ComponentState{}); got != color.Color(base) {
		t.Fatalf("rest fill = %v, want base %v unchanged", got, base)
	}
	hover := flatStateFill(base, ComponentState{Hovered: true})
	if hover == color.Color(base) {
		t.Fatal("hover fill must differ from rest")
	}
	press := flatStateFill(base, ComponentState{Pressed: true})
	if press == color.Color(base) {
		t.Fatal("press fill must differ from rest")
	}
	// Press is darker than rest, hover is lighter.
	if lum(press) >= lum(base) {
		t.Fatalf("press fill must be darker than rest (%v vs %v)", press, base)
	}
	if lum(hover) <= lum(base) {
		t.Fatalf("hover fill must be lighter than rest (%v vs %v)", hover, base)
	}
}

// lum is a test-local luminance proxy.
func lum(c color.Color) int {
	r, g, b, _ := c.RGBA()
	return int(r>>8) + int(g>>8) + int(b>>8)
}
```

Add `"image/color"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFlat' 2>&1 | head -20
```
Expected: FAIL — `undefined: drawFlatButtonBody`, `undefined: drawFlatSelectionBar`, `undefined: selectionEdgeBottom`, `undefined: flatStateFill`.

- [ ] **Step 3: Write the implementation**

```go
package ui

// button_flat.go — the flat-tonal button primitives that replace the retired
// matte-keycap chrome (button_cushion.go). See DESIGN.md "Elevation & Depth".
//
// Doctrine: depth comes from TONE (a control sits one surface level above its
// container), feedback comes from MOTION (hover/press deltas + a growing
// selection bar), and the accent color marks VALUE, never chrome. A control
// draws no border at rest — the only outline left in the system is the
// hot-pink focus ring.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// flatSelectionBarPx is the thickness of the accent selection bar. Two px
// reads at every density without needing a token (it is a hairline constant
// like the 1px separator, not a sizing decision).
const flatSelectionBarPx = 2

// selectionEdge names which edge of a control carries its selection bar.
// Horizontal groups (tab strips, segmented controls) mark the BOTTOM; vertical
// lists (menus) mark the LEFT. One primitive, two orientations — see DESIGN.md
// "One selection primitive".
type selectionEdge int

const (
	selectionEdgeBottom selectionEdge = iota
	selectionEdgeLeft
)

// drawFlatButtonBody paints a control's body: a single filled rounded rect and
// nothing else. No socket wall, no contact shadow, no bevel, no border stroke.
// The control's silhouette is the tone step between fill and its container.
func drawFlatButtonBody(dst *ebiten.Image, r image.Rectangle, fill color.Color, rad int) {
	if r.Empty() || fill == nil {
		return
	}
	drawRoundedRect(dst, r, fill, rad, true)
}

// drawFlatSelectionBar paints the accent selection bar along r's edge. frac
// (0..1) scales the bar's length so the hover spring can grow it from nothing
// to full; a bottom bar grows outward from the center, a left bar is always
// full-height (a list row's stripe does not animate its length). frac <= 0
// draws nothing, which is what keeps an unselected control at zero draw cost.
func drawFlatSelectionBar(dst *ebiten.Image, r image.Rectangle, col color.Color, edge selectionEdge, frac float64) {
	if r.Empty() || col == nil || frac <= 0 {
		return
	}
	if frac > 1 {
		frac = 1
	}
	switch edge {
	case selectionEdgeLeft:
		drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+flatSelectionBarPx, r.Max.Y), col, true)
	default: // selectionEdgeBottom
		w := int(float64(r.Dx()) * frac)
		if w < 1 {
			return
		}
		cx := (r.Min.X + r.Max.X) / 2
		x0 := cx - w/2
		drawRect(dst, image.Rect(x0, r.Max.Y-flatSelectionBarPx, x0+w, r.Max.Y), col, true)
	}
}

// flatStateFill applies the interaction brightness deltas to a base fill.
// Values are the historical ButtonStyle deltas retained verbatim from
// DESIGN.md `button-secondary` (hover +12, press -20) so the restyle changes
// the button's GEOMETRY, not its interaction feel. Disabled short-circuits to
// surface-1 so a dead control reads as part of its container.
func flatStateFill(base color.Color, st ComponentState) color.Color {
	if st.Disabled {
		return colSurface1
	}
	switch {
	case st.Pressed:
		return adjustColor(base, -20)
	case st.Hovered:
		return adjustColor(base, 12)
	}
	return base
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFlat' -v 2>&1 | tail -25
```
Expected: all six PASS.

If `ComponentState` has no `Disabled` field, check `components.go` and use the field that exists; if none, drop the disabled branch from `flatStateFill` and from the test.

- [ ] **Step 5: Verify the whole package still builds**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -5
```
Expected: `ok` — Task 1 is purely additive.

---

### Task 2: Migrate `Button.Draw` to the flat path

**Files:**
- Modify: `src/go/internal/ui/uigrid.go:226-340` (`(*Button).Draw`)
- Modify: `src/go/internal/ui/button_depth_test.go`
- Test: `src/go/internal/ui/button_flat_draw_test.go` (create)

**Interfaces:**
- Consumes: `drawFlatButtonBody`, `drawFlatSelectionBar`, `selectionEdgeBottom`, `flatSelectionBarPx` (Task 1)
- Produces: `(*Button).Draw` renders flat. The label/icon rect becomes `b.r` itself rather than a derived `cap` rect — every downstream helper that centered on `cap` now centers on `b.r`.

- [ ] **Step 1: Write the failing test**

Create `button_flat_draw_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A resting button draws its body at its FULL rect — no keycap wall inset,
// no socket, no contact shadow.
func TestButtonDrawFlatUsesFullRect(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	b := NewButton("Play", InstButtonStyle, nil)
	r := image.Rect(20, 20, 100, 52)
	b.SetRect(r)

	rects, _ := captureDrawRects(t, func() { b.Draw(dst) })
	if len(rects) == 0 {
		t.Fatal("button drew nothing")
	}
	// The body must cover the full button rect on at least one emitted rect.
	var covered bool
	for _, rc := range rects {
		if rc == r {
			covered = true
		}
		if rc.Max.Y > r.Max.Y {
			t.Fatalf("rect %v escapes button rect %v (contact shadow not removed)", rc, r)
		}
	}
	if !covered {
		t.Fatalf("no emitted rect equals the full button rect %v; got %v", r, rects)
	}
}

// A latched button must NOT flood itself with the accent. It gets a tinted
// fill plus a 2px accent bar.
func TestButtonDrawActiveIsTintPlusBarNotFlood(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	b := NewButton("EQ", InstButtonStyle, nil)
	r := image.Rect(20, 20, 100, 52)
	b.SetRect(r)
	b.SetToggled(true)

	rects, _ := captureDrawRects(t, func() { b.Draw(dst) })
	var bar bool
	for _, rc := range rects {
		if rc.Dy() == flatSelectionBarPx && rc.Max.Y == r.Max.Y {
			bar = true
		}
	}
	if !bar {
		t.Fatalf("latched button drew no %dpx bottom selection bar; rects=%v", flatSelectionBarPx, rects)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestButtonDrawFlat|TestButtonDrawActive' 2>&1 | head -20
```
Expected: FAIL — the current keycap path insets the body by `genGeomKeycapWallDepth` and draws a contact shadow one px below `r.Max.Y`.

- [ ] **Step 3: Replace the keycap block in `Button.Draw`**

In `uigrid.go`, replace lines 232–263 (from `rad := RadiusSM` through the closing brace of the `if !b.Disabled { ... }` bevel block) with:

```go
	rad := RadiusSM
	if b.SquareCorners {
		rad = 0
	}
	// Flat-tonal body: one filled rounded rect at the FULL button rect. The
	// silhouette is the tone step against the container, not a wall or a
	// border. See DESIGN.md "Elevation & Depth".
	cap := b.r
	st := ComponentState{Pressed: b.pressed, Hovered: b.hovered, Active: b.toggled}
	switch {
	case b.Disabled:
		drawFlatButtonBody(dst, cap, colSurface1, rad)
	case b.hasSpecID:
		Render(dst, cap, Spec(b.SpecID), st)
	case b.Style != nil:
		b.Style.Draw(dst, cap, b.pressed, b.hovered)
	}
	if !b.Disabled && b.toggled {
		// Latched: accent TINT over the body plus a 2px accent bar. Never a
		// flood — the accent belongs to values, not chrome.
		drawRoundedRect(dst, cap, WithAlpha(colAccent, AlphaSubtle), rad, true)
		drawFlatSelectionBar(dst, cap, colAccent, selectionEdgeBottom, 1)
	}
```

Keep the rest of `Draw` (label, icon) unchanged — it already centers on `cap`, which is now `b.r`.

- [ ] **Step 4: Fix the toggled text color**

Further down in the same function, the label and icon force a dark legend on the latched amber cap. A tinted body needs a *bright* label, not a dark one. Change both occurrences of:

```go
			textCol = genColorBackground
```
to:
```go
			textCol = colTextPrimary
```
and:
```go
			col = genColorBackground
```
to:
```go
			col = colAccent
```

- [ ] **Step 5: Run the tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestButtonDraw' -v 2>&1 | tail -20
```
Expected: both PASS.

- [ ] **Step 6: Run the full ui package and triage**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -40
```
Expected: failures in `button_depth_test.go`, `button_keycap_test.go`, and any pixel test asserting keycap geometry. Record the list — Task 3 rewrites them. Do NOT delete assertions wholesale; each one that pinned keycap geometry gets a flat equivalent.

---

### Task 3: Delete the keycap helpers and migrate the remaining seven call sites

**Files:**
- Modify: `src/go/internal/ui/button_cushion.go` (delete keycap functions)
- Modify: `src/go/internal/ui/audio_sticky_bar.go:341,491-527`
- Modify: `src/go/internal/ui/segmented_control.go:150-199`
- Modify: `src/go/internal/ui/synth_panel_zone.go:3155-3165`
- Modify: `src/go/internal/ui/param_card.go:66-80`
- Modify: `src/go/internal/ui/synth_macro_render.go:389-405`
- Modify: `src/go/internal/ui/timeline_zone.go:587-595`
- Modify: `src/go/internal/ui/game_longpress_popup.go:205-215`
- Delete: `src/go/internal/ui/button_keycap_test.go`
- Modify: `src/go/internal/ui/button_depth_test.go`

**Interfaces:**
- Consumes: `drawFlatButtonBody` (Task 1)
- Produces: `func drawFlatPanelButton(dst *ebiten.Image, r image.Rectangle, fill color.Color, hovered bool) image.Rectangle` — the flat replacement for `drawKeycapPanelButton`, returning `r` so callers keep centering labels on the returned rect.

- [ ] **Step 1: Add the flat panel-button replacement**

In `button_flat.go`, append:

```go
// drawFlatPanelButton renders a transient popup/panel action as a flat tonal
// chip and returns the content rect so callers can center a label on it.
// Replaces drawKeycapPanelButton; returns the full rect because there is no
// longer a cap face distinct from the button rect.
func drawFlatPanelButton(dst *ebiten.Image, r image.Rectangle, fill color.Color, hovered bool) image.Rectangle {
	rad := popupButtonRadius()
	if hovered {
		fill = adjustColor(fill, 12)
	}
	drawFlatButtonBody(dst, r, fill, rad)
	return r
}
```

- [ ] **Step 2: Migrate `param_card.go` and `synth_macro_render.go` first**

These two are the reference aesthetic — migrating them first proves the flat path looks right on the surface the whole restyle is modeled on.

In `param_card.go`, replace lines 69–77 (`rad := ...` through the `drawRoundedRect(dst, capR, rim, rad, false)` line) with:

```go
	rad := RadiusMD / 2
	capR := r
	drawFlatButtonBody(dst, capR, TokenSurface3(), rad)
	if st.Selected {
		drawFlatSelectionBar(dst, capR, colConceptStroke, selectionEdgeBottom, 1)
	}
```

In `synth_macro_render.go`, replace the identical block at lines 392–405 with the same five lines.

Both files previously derived `shell := adjustColor(capFill, -45)` — delete that line and the now-unused `capFill` if the compiler flags it.

- [ ] **Step 3: Verify the synth cards still render**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ParamCard|Macro' 2>&1 | tail -20
```
Expected: PASS, or pixel-count failures that you update to the new (lower) rect counts.

- [ ] **Step 4: Migrate `segmented_control.go`**

Replace lines 176–190 (the `capR := keycapCapRect(...)` block through the bevel `if`) with:

```go
		// Flat-tonal segment: body fill only, plus a 2px accent bar when
		// active. No socket, no bevel — see DESIGN.md "Elevation & Depth".
		capR := cell
		drawFlatButtonBody(dst, capR, capFill, pillRadius)
		if active {
			drawFlatSelectionBar(dst, capR, colAccent, selectionEdgeBottom, 1)
		}
```

Also drop the tray *border* on line 154 (`drawRoundedRect(dst, s.rect, colBorderMedium, RadiusMD, false)`) — the recessed tray is a tone step now, not an outline.

- [ ] **Step 5: Migrate `audio_sticky_bar.go`**

At line 341 replace `capR := keycapCapRect(r, b.channelBtn.pressTravelPx(), false)` with `capR := r`.

At lines 491–527, replace the `capR := keycapCapRect(...)` block, the shell `drawRoundedButton`, `drawKeycapContactShadow`, and the active-inset/bevel `if` with:

```go
	capR := r
	drawFlatButtonBody(dst, capR, capFill, pillRadius)
	if active {
		drawFlatSelectionBar(dst, capR, colAccent, selectionEdgeBottom, 1)
	}
```

Keep the existing `pillCapFill*` / `pillText*` palette variables; only the shell/bevel drawing goes. Delete the now-unused `pillShell*` palette vars if the compiler flags them.

- [ ] **Step 6: Migrate the three remaining call sites**

- `synth_panel_zone.go:3159-3163`: replace `capR := keycapCapRect(r, 0, true)` + contact shadow + bevel with `capR := r` + `drawFlatButtonBody(dst, capR, <existing fill>, rad)`.
- `timeline_zone.go:592`: `drawKeycapPanelButton(` → `drawFlatPanelButton(`, dropping the now-extra `border` argument.
- `game_longpress_popup.go:213`: same substitution, same dropped argument.

- [ ] **Step 7: Delete the keycap helpers**

In `button_cushion.go`, delete: `keycapCapRect`, `drawKeycapShell`, `drawKeycapContactShadow`, `drawKeycapBevel`, `drawKeycapActiveInset`, `drawKeycapPanelButton`, `drawKeycapEngageFlash`, and the `keycapBevelTopCol` / `keycapBevelBotCol` / `keycapActiveFill` / `keycapActiveBotLightCol` var block.

Keep: `buttonGlowAlpha`, `buttonGlowAlphaAnimated`, `buttonGlowRingColor`, `keycapHoverLiftAlpha`, `drawButtonHoverGlow`, `drawButtonInnerShadow` — the hover overlay still uses them.

Update the file's header comment to describe it as the hover/press helper file rather than the keycap file.

- [ ] **Step 8: Delete and rewrite the keycap contract tests**

```bash
rm src/go/internal/ui/button_keycap_test.go
```

`button_keycap_test.go` pinned `keycapCapRect` geometry and the press-travel spring. The travel spring itself (`pressDepth`, `AdvancePressAnim`, `pressTravelPx`) still exists and still matters, so preserve *those* assertions by moving `TestButtonPressDepthTravel` into `button_flat_draw_test.go` before deleting the file, dropping only the `keycapCapRect` assertions.

- [ ] **Step 9: Run the full package**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -40
```
Expected: green, except for pixel/alloc tests that Task 4 and Task 10 address. Fix compile errors immediately; triage assertion failures against "is the new behavior correct?" — never relax an assertion to make it pass without confirming the new value is right.

- [ ] **Step 10: Update the token-discipline ratchet**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTokenDiscipline 2>&1 | tail -20
```
The removed keycap color vars drop literal counts in `button_cushion.go`. Decrement that file's entry in `allowedLiteralBudget` in `token_discipline_test.go` to the reported actual.

---

### Task 4: `SegmentGroup` — shared track and sliding indicator

**Files:**
- Create: `src/go/internal/ui/segment_group.go`
- Create: `src/go/internal/ui/segment_group_test.go`

**Interfaces:**
- Consumes: `drawFlatButtonBody`, `drawFlatSelectionBar`, `selectionEdgeBottom` (Task 1)
- Produces:
  - `type SegmentGroup struct { ... }`
  - `func NewSegmentGroup() *SegmentGroup`
  - `func (g *SegmentGroup) Layout(track image.Rectangle, n int)`
  - `func (g *SegmentGroup) SegmentRect(i int) image.Rectangle`
  - `func (g *SegmentGroup) SetActive(i int)`
  - `func (g *SegmentGroup) Advance()` — eases the indicator, no-op when settled
  - `func (g *SegmentGroup) Settled() bool`
  - `func (g *SegmentGroup) DrawTrack(dst *ebiten.Image)`
  - `func (g *SegmentGroup) DrawIndicator(dst *ebiten.Image, col color.Color)`

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSegmentGroupLayoutSplitsTrackEvenly(t *testing.T) {
	g := NewSegmentGroup()
	g.Layout(image.Rect(0, 0, 300, 30), 3)
	for i, want := range []int{0, 100, 200} {
		if got := g.SegmentRect(i).Min.X; got != want {
			t.Fatalf("segment %d x = %d, want %d", i, got, want)
		}
	}
	if g.SegmentRect(2).Max.X != 300 {
		t.Fatalf("last segment must reach track end, got %d", g.SegmentRect(2).Max.X)
	}
}

// The indicator starts settled on the active segment, and a SetActive kicks it
// into motion; Advance eases it until Settled() reports true again.
func TestSegmentGroupIndicatorSlidesThenSettles(t *testing.T) {
	g := NewSegmentGroup()
	g.Layout(image.Rect(0, 0, 300, 30), 3)
	g.SetActive(0)
	for i := 0; i < 200 && !g.Settled(); i++ {
		g.Advance()
	}
	if !g.Settled() {
		t.Fatal("indicator never settled on initial active")
	}
	g.SetActive(2)
	if g.Settled() {
		t.Fatal("SetActive to a different index must unsettle the indicator")
	}
	n := 0
	for ; n < 500 && !g.Settled(); n++ {
		g.Advance()
	}
	if !g.Settled() {
		t.Fatalf("indicator did not settle after %d frames", n)
	}
	if n < 2 {
		t.Fatal("indicator snapped instantly; it must animate")
	}
	// Settled position must equal the target segment.
	want := g.SegmentRect(2)
	if got := g.indicatorRect(); got.Min.X != want.Min.X || got.Dx() != want.Dx() {
		t.Fatalf("settled indicator = %v, want to match segment 2 %v", got, want)
	}
}

// Zero idle work: a settled group's Advance must not change state.
func TestSegmentGroupSettledAdvanceIsNoop(t *testing.T) {
	g := NewSegmentGroup()
	g.Layout(image.Rect(0, 0, 300, 30), 3)
	g.SetActive(1)
	for i := 0; i < 500 && !g.Settled(); i++ {
		g.Advance()
	}
	before := g.indicatorRect()
	g.Advance()
	if g.indicatorRect() != before {
		t.Fatal("settled Advance changed the indicator (violates zero-idle-work)")
	}
}

// A settled, unselected group draws its track but the indicator is a single
// bar — never a per-segment outline.
func TestSegmentGroupDrawEmitsTrackPlusOneBar(t *testing.T) {
	dst := ebiten.NewImage(400, 60)
	g := NewSegmentGroup()
	g.Layout(image.Rect(0, 0, 300, 30), 3)
	g.SetActive(1)
	for i := 0; i < 500 && !g.Settled(); i++ {
		g.Advance()
	}
	rects, _ := captureDrawRects(t, func() {
		g.DrawTrack(dst)
		g.DrawIndicator(dst, colAccent)
	})
	if len(rects) != 2 {
		t.Fatalf("group emitted %d rects, want exactly 2 (track + indicator)", len(rects))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSegmentGroup 2>&1 | head -20
```
Expected: FAIL — `undefined: NewSegmentGroup`.

- [ ] **Step 3: Write the implementation**

```go
package ui

// segment_group.go — the flat-tonal replacement for a row of individually
// outlined pills. The GROUP owns the recessed track and the single accent
// indicator; members are plain labels drawn inside it. Selection animates by
// sliding one shared bar rather than repainting two pills.
//
// This exists as a group-level primitive (not a Button flag) because the track
// and indicator belong to the collection, not to any member. Members remain
// ordinary Buttons for hit-testing, so the input-tree contract is untouched.

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// segmentSettleEpsilon is the sub-pixel distance below which the sliding
// indicator is considered settled and stops doing per-frame work.
const segmentSettleEpsilon = 0.5

type SegmentGroup struct {
	track image.Rectangle
	n     int
	// active is the target index; curX/curW are the indicator's live
	// (float) geometry easing toward the active segment.
	active     int
	curX, curW float64
	settled    bool
}

func NewSegmentGroup() *SegmentGroup { return &SegmentGroup{active: -1} }

// Layout sets the group's track rect and member count. Re-layout preserves the
// active index and snaps the indicator if the geometry changed, so a resize
// does not animate.
func (g *SegmentGroup) Layout(track image.Rectangle, n int) {
	resized := g.track != track || g.n != n
	g.track, g.n = track, n
	if resized {
		g.snap()
	}
}

// SegmentRect returns member i's rect. The last member absorbs the rounding
// remainder so the group always fills its track exactly.
func (g *SegmentGroup) SegmentRect(i int) image.Rectangle {
	if g.n <= 0 || i < 0 || i >= g.n {
		return image.Rectangle{}
	}
	w := g.track.Dx() / g.n
	x0 := g.track.Min.X + i*w
	x1 := x0 + w
	if i == g.n-1 {
		x1 = g.track.Max.X
	}
	return image.Rect(x0, g.track.Min.Y, x1, g.track.Max.Y)
}

// SetActive targets member i. A change unsettles the indicator so Advance
// animates it across; setting the same index is a no-op.
func (g *SegmentGroup) SetActive(i int) {
	if i == g.active {
		return
	}
	g.active = i
	g.settled = false
}

func (g *SegmentGroup) Settled() bool { return g.settled }

// snap places the indicator on the active segment immediately.
func (g *SegmentGroup) snap() {
	r := g.SegmentRect(g.active)
	g.curX, g.curW = float64(r.Min.X), float64(r.Dx())
	g.settled = true
}

// Advance eases the indicator toward the active segment using the
// segment-indicator-slide token. No-ops once settled — this is what keeps a
// steady group at zero per-frame work.
func (g *SegmentGroup) Advance() {
	if g.settled {
		return
	}
	tgt := g.SegmentRect(g.active)
	if tgt.Empty() {
		g.settled = true
		return
	}
	rate := float64(genAnimSegmentIndicatorSlide.Rate)
	g.curX += (float64(tgt.Min.X) - g.curX) * (1 - rate)
	g.curW += (float64(tgt.Dx()) - g.curW) * (1 - rate)
	if math.Abs(float64(tgt.Min.X)-g.curX) < segmentSettleEpsilon &&
		math.Abs(float64(tgt.Dx())-g.curW) < segmentSettleEpsilon {
		g.snap()
	}
}

// indicatorRect is the indicator's live geometry, rounded to pixels.
func (g *SegmentGroup) indicatorRect() image.Rectangle {
	x := int(math.Round(g.curX))
	w := int(math.Round(g.curW))
	return image.Rect(x, g.track.Min.Y, x+w, g.track.Max.Y)
}

// DrawTrack paints the recessed surface-1 track the members sit in. One filled
// rounded rect, no outline.
func (g *SegmentGroup) DrawTrack(dst *ebiten.Image) {
	drawFlatButtonBody(dst, g.track, colSurface1, RadiusMD)
}

// DrawIndicator paints the single sliding accent bar on the bottom edge of the
// indicator's live rect. Nothing is drawn when no member is active.
func (g *SegmentGroup) DrawIndicator(dst *ebiten.Image, col color.Color) {
	if g.active < 0 || g.n <= 0 {
		return
	}
	drawFlatSelectionBar(dst, g.indicatorRect(), col, selectionEdgeBottom, 1)
}
```

- [ ] **Step 4: Add the animation token**

`genAnimSegmentIndicatorSlide` does not exist yet. Add to `DESIGN.md`'s `animations:` block, immediately after `button-press-decay`:

```yaml
  # Segment-group indicator slide — when the selection moves between members of
  # a segmented control (audio tab strip, mobile bottom nav, view switcher), the
  # single shared accent bar EASES from the old member to the new one rather
  # than teleporting. Rate 0.72 gives a ~0.25s glide @60fps, slightly slower
  # than button-press-decay so the travel reads as deliberate. Drives
  # (*SegmentGroup).Advance in segment_group.go; settles via
  # segmentSettleEpsilon (sub-pixel), after which the group does zero per-frame
  # work.
  segment-indicator-slide:
    kind: exp-decay
    rate: 0.72
    threshold: 0.01
```

Then regenerate and update the drift guard:

```bash
make gen-design-tokens
```

Add the new token to the want-map in `design_md_drift_test.go` if that test enumerates animations (check first — it may only cover colors/spacing/rounded/alpha, in which case no change is needed).

- [ ] **Step 5: Run the tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSegmentGroup|TestDesignMDDrift' -v 2>&1 | tail -25
```
Expected: all PASS.

---

### Task 5: Apply `SegmentGroup` to the tab strips

**Files:**
- Modify: `src/go/internal/ui/audio_sticky_bar.go` (tab pill row)
- Modify: `src/go/internal/ui/segmented_control.go` (mobile view switcher)
- Test: `src/go/internal/ui/segment_group_apply_test.go` (create)

**Interfaces:**
- Consumes: `SegmentGroup` (Task 4)
- Produces: no new exported API; the sticky bar and segmented control each hold a `*SegmentGroup` field.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

// Switching the audio panel tab must move the shared indicator, not repaint
// two independent pills. Asserted structurally: the sticky bar owns exactly
// one SegmentGroup and its active index tracks the panel's active tab.
func TestStickyBarTabIndicatorTracksActiveTab(t *testing.T) {
	g := newTestGame(t)
	dv := g.drum
	bar := dv.eqPanelZone.stickyBar()
	if bar.tabGroup == nil {
		t.Fatal("sticky bar has no SegmentGroup")
	}
	dv.eqPanelZone.SetActiveTab(TabSpectrum)
	g.Update()
	want := indexOfTab(TabSpectrum)
	if got := bar.tabGroup.active; got != want {
		t.Fatalf("indicator active = %d, want %d", got, want)
	}
}
```

Adapt `newTestGame`, `stickyBar()`, and `indexOfTab` to whatever the package already provides — check `audio_sticky_bar_phase1_test.go` for the established accessor pattern and reuse it rather than adding new test-only exports.

- [ ] **Step 2: Run to verify it fails**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStickyBarTabIndicator 2>&1 | head -20
```
Expected: FAIL — `bar.tabGroup` undefined.

- [ ] **Step 3: Wire the group into `AudioStickyBar`**

Add a `tabGroup *SegmentGroup` field, initialize with `NewSegmentGroup()` in `NewAudioStickyBar`, call `tabGroup.Layout(tabRowRect, len(tabs))` in the bar's Layout, call `tabGroup.SetActive(idx)` wherever `activeTab` is assigned, and call `tabGroup.Advance()` once per frame in the bar's Draw. Draw order inside the tab row becomes: `tabGroup.DrawTrack` → per-member label → `tabGroup.DrawIndicator`.

Remove the per-pill body fill and the per-pill active bar added in Task 3 step 5 — the group now owns both.

- [ ] **Step 4: Do the same for `SegmentedControl`**

Replace its per-cell `drawFlatButtonBody` + `drawFlatSelectionBar` (added in Task 3 step 4) with a `*SegmentGroup` following the same pattern. `SegmentedControl.SegmentRect(i)` should now delegate to the group so hit-testing and drawing share one geometry source.

- [ ] **Step 5: Run tests**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -30
```
Expected: green. Investigate any hit-testing failure carefully — `SegmentRect` is used by input routing, so a geometry change there is a real behavior change.

---

### Task 6: Flatten menu and panel chrome

**Files:**
- Modify: `src/go/internal/ui/menu_search_field.go` (or wherever `menu-search-field` renders — locate with grep)
- Modify: `src/go/internal/ui/insert_effects_*.go` (FX card ▲▼/× and `+ Add Effect`)
- Modify: `src/go/internal/ui/theme.go` (close-× style)
- Test: `src/go/internal/ui/menu_chrome_flat_test.go` (create)

**Interfaces:**
- Consumes: `drawFlatButtonBody` (Task 1)
- Produces: `var QuietButtonStyle ButtonStyle` — the `button-quiet` variant (no fill at rest).

- [ ] **Step 1: Locate the surfaces**

Run:
```bash
cd src/go/internal/ui && grep -rn 'menu-search-field\|MenuSearchField\|AddEffect\|newPopupCloseButton' --include=*.go . | grep -v _test | head -20
```
Record the exact files and line numbers before editing.

- [ ] **Step 2: Add the quiet style**

In `button_styles.go`:

```go
// QuietButtonStyle is the `button-quiet` variant: no fill at rest, so
// icon-only chrome inside a dense strip (kebab, close ×, FX card controls,
// "+ Add Effect") reads as a glyph rather than a box. Hover and press supply
// the fill via the interaction deltas, so the control is still discoverable
// on desktop; on mobile the same lift is press-driven.
var QuietButtonStyle = ButtonStyle{
	Fill:   color.Transparent,
	Border: color.Transparent,
	Radius: RadiusSM,
}
```

Add `"image/color"` to the imports.

- [ ] **Step 3: Write the failing test**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A quiet button at rest paints nothing; hovering it paints a body.
func TestQuietButtonRestPaintsNothing(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	b := NewButton("", QuietButtonStyle, nil)
	b.SetRect(image.Rect(10, 10, 40, 40))
	rects, _ := captureDrawRects(t, func() { b.Draw(dst) })
	for _, rc := range rects {
		t.Fatalf("quiet button at rest painted %v, want nothing", rc)
	}
}
```

- [ ] **Step 4: Run, implement, re-run**

Run the test, confirm it fails, then apply `QuietButtonStyle` at the sites found in Step 1 and re-run:

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestQuietButton' -v 2>&1 | tail -10
```

If a transparent fill still emits a `drawRect`, add an early return for fully transparent fills in `drawFlatButtonBody`:

```go
	if _, _, _, a := fill.RGBA(); a == 0 {
		return
	}
```

- [ ] **Step 5: Drop the search-field outline**

Change the `menu-search-field` render to a recessed `colSurface1` body with no border, matching a `SegmentGroup` track. Keep the focus ring — it is the one surviving outline.

- [ ] **Step 6: Full package run**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -30
```

---

### Task 7: Promote the value-hint primitives

**Files:**
- Create: `src/go/internal/ui/control_value_hint.go`
- Modify: `src/go/internal/ui/param_card.go` (delegate to the shared primitives)
- Create: `src/go/internal/ui/control_value_hint_test.go`

**Interfaces:**
- Produces:
  - `type ValueHint struct { Kind valueHintKind; Frac, Zero float64; Count, Idx int }`
  - `func drawValueGauge(dst *ebiten.Image, r image.Rectangle, h ValueHint, dim bool)`
  - `valueHintKind` constants `hintFill`, `hintBipolar`, `hintDots`
- Consumes: nothing new.

- [ ] **Step 1: Move, don't rewrite**

`drawParamCardGauge` in `param_card.go:113-160` is already the correct implementation. Move it verbatim into `control_value_hint.go`, renaming `paramCardGauge` → `ValueHint`, `paramCardGaugeKind` → `valueHintKind`, `gaugeFill`/`gaugeBipolar`/`gaugeDots` → `hintFill`/`hintBipolar`/`hintDots`, and `drawParamCardGauge` → `drawValueGauge`. Keep `paramCardGaugeFor` in `param_card.go` as the synth-specific adapter that builds a `ValueHint` from a `*Knob`.

Moving rather than reimplementing means the Synth tab — the reference surface — is byte-identical afterward, which is the point of the extraction.

- [ ] **Step 2: Run the existing param-card tests unchanged**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ParamCard' -v 2>&1 | tail -15
```
Expected: PASS with **no test edits**. If a test needed editing, the move was not faithful — revert and redo.

- [ ] **Step 3: Write the discipline test**

```go
//go:build test

package ui

import "testing"

// Every control that carries a value must show at least one hint at rest.
// A control whose value is discoverable only by opening it is a defect.
// See DESIGN.md "Value-hint system".
func TestValuedControlsDeclareAHint(t *testing.T) {
	for _, c := range valuedControlRegistry() {
		if !c.HasGauge && !c.HasGloss && !c.HasGraph {
			t.Errorf("valued control %q shows no value hint at rest", c.Name)
		}
	}
}
```

- [ ] **Step 4: Build the registry**

Add to `control_value_hint.go`:

```go
// valuedControl describes one control that carries a user-visible value, and
// which at-rest hint affordances it provides. The registry backs
// TestValuedControlsDeclareAHint — the enforcement point for the DESIGN.md
// rule that a value must never be hidden behind an open gesture.
type valuedControl struct {
	Name                        string
	HasGauge, HasGloss, HasGraph bool
}

func valuedControlRegistry() []valuedControl {
	return []valuedControl{
		{Name: "synth-param-card", HasGauge: true, HasGloss: true},
		{Name: "synth-macro-card", HasGloss: true, HasGraph: true},
		{Name: "row-volume-slider", HasGauge: true},
		{Name: "master-volume", HasGauge: true},
		{Name: "bpm-input", HasGloss: true},
		{Name: "eq-band-db", HasGauge: true},
	}
}
```

Populate the list from the actual valued controls in the app; add an entry as each is given a hint. An entry with all three false is the failing state that drives the work.

- [ ] **Step 5: Run and fix**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestValuedControls -v 2>&1 | tail -20
```
For each failure, add the missing gauge via `drawValueGauge` at that control's draw site, then flip its registry flag.

---

### Task 8: Retire the dead tokens

Runs *after* the renderers stop referencing them, so the tree is green throughout.

**Files:**
- Modify: `DESIGN.md` (`geometry:` and `animations:` blocks)
- Modify: `src/go/internal/ui/design_tokens.gen.go` (regenerated)
- Modify: `src/go/internal/ui/design_md_drift_test.go`
- Modify: `src/go/internal/ui/testdata/design_md_lint.expected.json`

- [ ] **Step 1: Confirm each token is unreferenced**

```bash
cd src/go/internal/ui && for t in genGeomKeycapWallDepth genGeomKeycapActiveRaise genGeomKeycapReleaseOvershootFrac genGeomButtonPressScale genGeomButtonReleaseOvershoot genAnimButtonEngageFlash genAnimButtonTogglePulse genAnimButtonGlowRest; do echo "== $t: $(grep -rl "$t" --include=*.go . | grep -v design_tokens.gen.go | tr '\n' ' ')"; done
```
Expected: every line empty after the colon. **Any token still referenced stays** — remove it from the retirement list rather than forcing the deletion.

- [ ] **Step 2: Delete the confirmed-dead entries from DESIGN.md**

Remove `keycap-wall-depth`, `keycap-active-raise`, `keycap-release-overshoot-frac`, `button-press-scale`, `button-release-overshoot` from `geometry:`, and `button-engage-flash`, `button-toggle-pulse`, `button-glow-rest` from `animations:`.

Note: `buttonGlowAlphaAnimated` currently reads `genAnimButtonTogglePulse` and `genAnimButtonGlowRest`. Task 3 kept that function. Either simplify it to drop the toggled pulse branch (a latched control no longer pulses — the static bar is the signal) or keep those two tokens. Decide from the Step 1 output.

- [ ] **Step 3: Regenerate and update guards**

```bash
make gen-design-tokens
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignMDDrift|TestDesignMDLintSnapshot' -v 2>&1 | tail -25
```
Update the drift want-map for removed entries. For the lint snapshot, run the lint directly and re-pin:

```bash
npx -y @google/design.md@0.1.1 lint DESIGN.md
```
Write the new warning set to `testdata/design_md_lint.expected.json`.

---

### Task 9: Rewrite the DESIGN.md doctrine and component recipes

**Files:**
- Modify: `DESIGN.md` — "Elevation & Depth" prose (~line 2100-2240), `components:` block (~line 1133-1470)

- [ ] **Step 1: Replace the "Elevation & Depth" section**

Replace the matte-hardware doctrine with the flat-tonal one. Required content, verbatim where quoted:

> **Flat-tonal.** Depth comes from **tone**, feedback comes from **motion**, and the accent color belongs to **values, not chrome**.

Three rules: (1) a control's silhouette is a tone step, never a line — no border at rest, the only surviving outlines are the hot-pink focus ring and the 1px group separator; (2) nothing is 3D — the keycap helpers are deleted, with the audit note that the drum pads render through `drawDrumCell` and never used them; (3) gold is data ink — `primary` marks gauges, curves, indicator bars and meter fills, and a control never floods itself with the accent except `button-primary`, rationed to one per surface.

Keep the "Surface elevation cues", "Hover / press brightness deltas", and "WASM idle budget rule" subsections — all three survive unchanged. Delete "Keycap elevation", "Active latched amber state", and the keycap paragraphs of the hover subsection.

Add the selection primitive statement:

> Selection is marked by a 2 px accent bar on the **leading edge** of the selected element: **left** in a vertical list, **bottom** in a horizontal group. Fill is a tint, never a flood. In a group with exactly one selection, the bar is a single shared element that animates between members.

And the value rule:

> Every control that carries a value must show at least one hint at rest. A control whose value is discoverable only by opening it is a defect.

- [ ] **Step 2: Collapse the component recipes**

Remove every `border:` block from `components:` except `input-field-focused` (the focus ring). Consolidate the button recipes to the five variants: `button`, `button-quiet`, `button-primary`, `button-danger`, `segment`. Retain the existing IDs as aliases where Go code references them by `ComponentID` — check with:

```bash
cd src/go/internal/ui && grep -rn 'ComponentButton' --include=*.go . | grep -v gen.go | head -30
```

Only retire an ID that has no Go referent; otherwise re-point its recipe at the new variant's values and leave the ID in place. This keeps the change a *restyle*, not a rename sweep.

- [ ] **Step 3: Regenerate and verify**

```bash
make gen-design-tokens
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -30
```

---

### Task 10: Visual verification gate

**Not optional.** Per CLAUDE.md, no UI change is complete without an inspected screenshot. Green Go tests do not close this work.

- [ ] **Step 1: Build the screenshot binary**

```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
```

- [ ] **Step 2: Capture the four reference scenes**

```bash
cd /tmp && \
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene multi_row_full_grid -screenshot after_desktop_main.png && \
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene eq_tab_synth -screenshot after_desktop_synth.png && \
xvfb-run -a -s "-screen 0 500x950x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene mobile_default -screenshot after_mobile_main.png && \
xvfb-run -a -s "-screen 0 500x950x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene mobile_bottom_nav_synth -screenshot after_mobile_synth.png
```

- [ ] **Step 3: OPEN each PNG and inspect**

Check every item: labels present and legible; no blank controls; tab strip shows one track + one indicator, not seven boxes; active states read as tint+bar, not flood; row chips legible without borders; no overlapping chrome; the synth cards unchanged from the "before" captures.

**Re-capture until it looks right.** This loop is the deliverable.

- [ ] **Step 4: Sweep the wider catalog**

```bash
make screenshots-all
MOBILE=1 make screenshots-all
```
Inspect the menu, FX, transport, and overflow scenes specifically — those carry the most button chrome.

- [ ] **Step 5: Run the crop-scene and pixel guards**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSceneCrop|AudioPanelRenderPixels|SynthArmedPixels' 2>&1 | tail -20
```

- [ ] **Step 6: Full pipeline**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... 2>&1 | tail -20
```

---

## Self-Review

**Spec coverage:** Doctrine → Task 9. Five button variants → Tasks 2, 6, 9. Segment group → Tasks 4, 5. Selection primitive → Tasks 1 (`drawFlatSelectionBar`, both edges), 9 (doctrine). State model → Tasks 1 (`flatStateFill`), 2. Token retirement → Task 8. Menu chrome → Task 6. Menu row value slot → **gap, folded into Task 6 Step 5's scope note** — implement alongside the search field. Value hints → Task 7. Performance contract → Tasks 1 (frac<=0 early return), 4 (`Settled`), plus the retained alloc test. Visual verification → Task 10.

**Type consistency:** `drawFlatButtonBody(dst, r, fill, rad)` used identically in Tasks 1, 3, 4, 6. `drawFlatSelectionBar(dst, r, col, edge, frac)` identical in Tasks 1, 2, 3, 4. `ValueHint` named consistently in Task 7. `SegmentGroup` method names match between Task 4's interface block and Task 5's usage.

**Known ordering risk:** Task 3 removes per-pill bodies that Task 5 then re-owns at group level. This is deliberate — Task 3 keeps the app visually coherent between the two tasks rather than leaving tab pills unpainted.
