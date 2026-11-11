# Node Popup + Confirmation Menus Consistency — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring the node click-and-hold quick-action popup, the modal confirmation dialogs, and the armed-mode banners into DESIGN.md compliance and visual/behavioral consistency with the rest of the app — reusing existing infrastructure (shared `Button`/keycap chrome, the portal + `buttonHitAdapter` pattern, the icon system, text roles, tokens, and the `gridModeManager` API).

**Architecture:** Introduce one shared `ConfirmDialog` (a `PortalOverlay` in the `settings_overlay.go` shape) that both hand-drawn confirm twins consume; upgrade the node popup's *draw* to icon+label keycaps (input untouched); consolidate three bespoke mode banners into one `drawGridModeBanner` driven by `g.modes`; add three icons; and clean up orphaned tokens. Every pop-up owns its own setup (`Open`/`begin*`) and cancellation (`onCancel`/`dismiss*`), and closes on an outside click via the existing tree `CloseTop` route.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten 2.9, the `-tags test` fast path, ebitenstub rendering harness.

## Global Constraints

- **Build/test with the bundled Go:** `.tools/go/bin/go`. Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.
- **No hex literals in `internal/ui/`** — use `Token*()` accessors / `gen*` constants / `WithAlpha`. `token_discipline_test.go` (ratchet) + `design_md_drift_test.go` gate every `internal/ui/` change.
- **No raw Unicode chrome glyphs** — `IconID` + `drawIconByID` only (`forbidden_glyphs_test.go`).
- **Text via roles** — `DrawTextStyled(dst, s, x, y, role, col)` + `StyledText{Width,Height}`; never `DrawTextAt`/`DrawTextAtScale` for new chrome.
- **Rounding/spacing via tokens** — `Radius*`, `Space*`, `BtnHeight*`; no numeric literals.
- **i18n** — every user-facing string is an `i18n.Key` present in `catalog_en.go` AND `catalog_es.go` AND `AllKeys()` (`catalog_test.go::TestCatalogCompleteness`).
- **Mode state mutated only via `gridModeManager.Enter/Cancel/Commit`** (`grid_mode_discipline_test.go`).
- **Never stage or commit on the user's behalf beyond the per-task commits in this plan.** Do not `git add`/`commit` unless the step says so. (Project convention: the assistant does not stage/commit unprompted — but this plan's tasks each end in an explicit commit the worker runs.)
- **Functional tests preferred** — drive real `Game.Update` over hand-constructed state where a flow can be exercised end-to-end (project convention).
- Spec: `docs/superpowers/specs/2026-07-18-node-popup-confirm-menus-consistency-design.md`.

### Test harness reference (use these EXACT internal-package helpers)

New tests live in `package ui` (internal). Use the internal helpers — NOT the
external `ui_test` `newTestGame` (which returns `*ui.Game` and is package-private
to `ui_test`):

- **Game constructor:** `g := newTestGameForUndo(t)` → `*Game` (internal; the
  canonical builder used by keyboard/flow tests). If a task needs a differently-
  seeded game, mirror an existing internal builder in the same file.
- **Mouse press+release through Update:** `clickAt(g, sx, sy)` — signature
  `func clickAt(g *Game, sx, sy int)` (`origin_select_test.go:12`); it wraps
  `SetInputForTest(...)` and drives a full press+release across two `g.Update()`
  calls. **No `t` argument.**
- **Keyboard through Update:** `restore := stubAllInput(x, y, pressed, just, mouseBool); defer restore(); g.Update()` where `pressed`/`just` are
  `map[ebiten.Key]bool` (`undo_keyboard_dispatch_test.go`). For Enter:
  `stubAllInput(0,0, nil, map[ebiten.Key]bool{ebiten.KeyEnter:true}, false)`;
  for Esc use `ebiten.KeyEscape`.
- **Raw input override (custom gestures):** `SetInputForTest(cursorFn, mouseFn, keyFn, charsFn, wheelFn, sizeFn) (restore func())` (see `origin_select_test.go`).

Wherever a task's test code below shows `newTestGameForUndo(t)`, `clickAt(t, g, …)`, or
`pressKey(t, g, …)`, substitute the real helpers above.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `internal/ui/drawing_icons.go` | icon draw funcs | add `drawMoveIcon`/`drawCopyIcon`/`drawConnectIcon` |
| `internal/ui/icons.go` | `IconID` enum + `drawIconByID` switch | add 3 enum consts + 3 cases |
| `internal/i18n/keys.go` + `catalog_en.go` + `catalog_es.go` | i18n | add 4 keys |
| `internal/ui/confirm_dialog.go` | **new** shared `ConfirmDialog` `PortalOverlay` | create |
| `internal/ui/node_clipboard.go` | paste-overwrite confirm | replace hand-drawn dialog with `ConfirmDialog` |
| `internal/ui/grid_move_break_confirm.go` | break-circuit confirm | replace hand-drawn dialog with `ConfirmDialog` |
| `internal/ui/game_input_shortcuts.go` | Esc/Enter ladder | consolidate the two headless confirm Esc rungs |
| `internal/ui/game_longpress_popup.go` | node quick-action popup | draw upgrade (icon+label) + outside-press dismissal |
| `internal/ui/grid_pane_draw.go` + `game_connect_mode.go` | mode banners | consolidate into `drawGridModeBanner` |
| `DESIGN.md` + `internal/ui/design_*.gen.go` | tokens/docs | remove orphaned tokens; document new component/icons |
| `internal/ui/design_md_drift_test.go`, `token_discipline_test.go` | guards | update |
| `internal/ui/scene_catalog.go` | scenes | add `paste_confirm_dialog`, `break_circuit_confirm` |

---

## Task 1: Three new icons (Move / Copy / Connect)

**Files:**
- Modify: `internal/ui/icons.go` (enum block ~line 46; switch ~line 130)
- Modify: `internal/ui/drawing_icons.go` (append draw funcs)
- Test: `internal/ui/icons_extras_test.go`

**Interfaces:**
- Produces: `IconMove`, `IconCopy`, `IconConnect` (`IconID`); `drawIconByID(dst, IconMove|IconCopy|IconConnect, r, col)` returns `true`.

- [ ] **Step 1: Write the failing test** — append to `internal/ui/icons_extras_test.go`:

```go
// TestIconNodeActionDispatch verifies the node quick-action icons are wired
// through drawIconByID (used by the long-press popup keycaps).
func TestIconNodeActionDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	col := color.NRGBA{255, 255, 255, 255}
	for _, id := range []IconID{IconMove, IconCopy, IconConnect} {
		if !drawIconByID(dst, id, r, col) {
			t.Errorf("drawIconByID returned false for %q; switch case or IconID missing", id)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestIconNodeActionDispatch`
Expected: FAIL — `undefined: IconMove` (compile error).

- [ ] **Step 3: Add the enum consts** — in `internal/ui/icons.go`, after `IconTarget IconID = "target"` (line ~46):

```go
	IconMove    IconID = "move"
	IconCopy    IconID = "copy"
	IconConnect IconID = "connect"
```

- [ ] **Step 4: Add the switch cases** — in `internal/ui/icons.go` `drawIconByID`, after the `IconTarget` case:

```go
	case IconMove:
		drawMoveIcon(dst, r, col)
	case IconCopy:
		drawCopyIcon(dst, r, col)
	case IconConnect:
		drawConnectIcon(dst, r, col)
```

- [ ] **Step 5: Add the draw funcs** — append to `internal/ui/drawing_icons.go` (24×24 logical grid, same idiom as `drawTargetIcon`):

```go
// Move: a four-way arrow cross centered in the icon box — the universal
// "move / reposition" glyph. Central plus stem with four arrowheads.
var drawMoveIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Cross stems.
	iconStrokeLine(dst, c, 12, 4, 12, 20, col)
	iconStrokeLine(dst, c, 4, 12, 20, 12, col)
	// Four arrowheads (up, down, left, right).
	iconStrokePolyline(dst, c, [][2]float32{{9, 7}, {12, 4}, {15, 7}}, col)
	iconStrokePolyline(dst, c, [][2]float32{{9, 17}, {12, 20}, {15, 17}}, col)
	iconStrokePolyline(dst, c, [][2]float32{{7, 9}, {4, 12}, {7, 15}}, col)
	iconStrokePolyline(dst, c, [][2]float32{{17, 9}, {20, 12}, {17, 15}}, col)
}

// Copy: two offset rounded squares (the "duplicate" glyph). Back square
// upper-right, front square lower-left overlapping it.
var drawCopyIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	// Back sheet (upper-right).
	iconStrokeOpenRoundedRect(dst, c, 9, 4, 11, 11, 1.5, "", col)
	// Front sheet (lower-left), drawn over the back.
	iconFillRoundedRect(dst, c, 4, 9, 11, 11, 1.5, colPanelBG)
	iconStrokeOpenRoundedRect(dst, c, 4, 9, 11, 11, 1.5, "", col)
}

// Connect: two nodes joined by a link — two small circles at opposite
// corners with a connecting bar, the "wire two nodes" glyph.
var drawConnectIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeCircle(dst, c, 6, 6, 3, col)
	iconStrokeCircle(dst, c, 18, 18, 3, col)
	iconStrokeLine(dst, c, 8.5, 8.5, 15.5, 15.5, col)
}
```

> `iconStrokeOpenRoundedRect(..., omit "", ...)` strokes all four sides (no omitted edge). `colPanelBG` clears the front-sheet interior so the overlap reads cleanly.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestIconNodeActionDispatch|TestIconUnknownReturnsFalse|TestForbidden'`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/ui/icons.go src/go/internal/ui/drawing_icons.go src/go/internal/ui/icons_extras_test.go
git commit -m "feat(ui): add Move/Copy/Connect node-action icons"
```

---

## Task 2: i18n keys for confirm titles + banners

**Files:**
- Modify: `internal/i18n/keys.go` (consts + `AllKeys()`)
- Modify: `internal/i18n/catalog_en.go`, `internal/i18n/catalog_es.go`
- Test: `internal/i18n/catalog_test.go` (existing `TestCatalogCompleteness` covers it)

**Interfaces:**
- Produces: `i18n.KeyConfirmReplaceNode`, `i18n.KeyCapOverwrite`, `i18n.KeyBannerMoving`, `i18n.KeyBannerConnecting`.

- [ ] **Step 1: Add key consts** — in `internal/i18n/keys.go`, next to `KeyBreakCircuitTitle` (~line 624) and `KeyBannerPasting` (~line 619):

```go
	// KeyConfirmReplaceNode titles the paste-overwrite confirmation dialog.
	KeyConfirmReplaceNode Key = "confirm.replace_node"
	// KeyCapOverwrite labels the destructive "Overwrite" confirm button.
	KeyCapOverwrite Key = "cap.overwrite"
	// KeyBannerMoving is the move-mode grid banner. Format template: (%d,%d).
	KeyBannerMoving Key = "banner.moving"
	// KeyBannerConnecting is the connect-mode grid banner. Format: (%d,%d).
	KeyBannerConnecting Key = "banner.connecting"
	// KeyEscToCancel is the shared armed-mode banner Esc hint suffix.
	KeyEscToCancel Key = "banner.esc_to_cancel"
```

(add `KeyEscToCancel` to `AllKeys()` alongside the others, and to both catalogs:
en `"(Esc to cancel)"`, es `"(Esc para cancelar)"`.)

- [ ] **Step 2: Register in `AllKeys()`** — add the four keys to the returned slice in `internal/i18n/keys.go:637` (next to `KeyBannerPasting`, `KeyBreakCircuitTitle`):

```go
		KeyConfirmReplaceNode, KeyCapOverwrite,
		KeyBannerMoving, KeyBannerConnecting,
```

- [ ] **Step 3: Add English strings** — in `internal/i18n/catalog_en.go`:

```go
	KeyConfirmReplaceNode: "Replace node?",
	KeyCapOverwrite:       "Overwrite",
	KeyBannerMoving:       "Moving node (%d,%d) — tap a point on an edge",
	KeyBannerConnecting:   "Connect from (%d,%d) — tap a target node",
```

- [ ] **Step 4: Add Spanish strings** — in `internal/i18n/catalog_es.go`:

```go
	KeyConfirmReplaceNode: "¿Reemplazar nodo?",
	KeyCapOverwrite:       "Sobrescribir",
	KeyBannerMoving:       "Moviendo nodo (%d,%d) — toca un punto en una arista",
	KeyBannerConnecting:   "Conectar desde (%d,%d) — toca un nodo destino",
```

- [ ] **Step 5: Run the completeness test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/ -run TestCatalogCompleteness`
Expected: PASS (both locales have all four keys).

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/i18n/keys.go src/go/internal/i18n/catalog_en.go src/go/internal/i18n/catalog_es.go
git commit -m "i18n: add replace-node/overwrite + moving/connecting banner keys"
```

---

## Task 3: Shared `ConfirmDialog` component

**Files:**
- Create: `internal/ui/confirm_dialog.go`
- Test: `internal/ui/confirm_dialog_test.go`

**Interfaces:**
- Consumes: `NewButtonKey`, `ComponentButtonDestructive/Primary/Secondary`, `buttonHitAdapter`, `drawScrim`, `drawPanel`, `drawMenuTitle`, `closeButtonRect`, `closeIconColor`, `PortalOverlay`, `portalEnterHandler`, `portalEscapeHandler`.
- Produces:
  - `type confirmConfig struct { id string; titleKey, confirmKey i18n.Key; destructive bool; swatch color.Color; onConfirm, onCancel func() }`
  - `func newConfirmDialog(cfg confirmConfig) *ConfirmDialog`
  - `type ConfirmDialog struct { ... }` implementing `Layout`, `HitAreas`, `Draw`, `ShouldClose`, `HandleEnter`, `HandleEscape`.
  - `func (d *ConfirmDialog) portalEntry(centerIn func() image.Rectangle) PortalEntry`

- [ ] **Step 1: Write the failing test** — `internal/ui/confirm_dialog_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func newTestConfirm(t *testing.T) (*ConfirmDialog, *int, *int) {
	t.Helper()
	confirms, cancels := 0, 0
	d := newConfirmDialog(confirmConfig{
		id:          "test-confirm",
		titleKey:    i18n.KeyConfirmReplaceNode,
		confirmKey:  i18n.KeyCapOverwrite,
		destructive: true,
		onConfirm:   func() { confirms++ },
		onCancel:    func() { cancels++ },
	})
	d.Layout(image.Rectangle{}, image.Rect(0, 0, 1280, 720))
	return d, &confirms, &cancels
}

func TestConfirmDialog_ConfirmFires(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	d.confirm()
	if *confirms != 1 || *cancels != 0 {
		t.Fatalf("confirm(): confirms=%d cancels=%d, want 1/0", *confirms, *cancels)
	}
	if !d.ShouldClose() {
		t.Fatal("confirm() must mark the dialog closed")
	}
}

func TestConfirmDialog_CancelFires(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	d.cancel()
	if *confirms != 0 || *cancels != 1 {
		t.Fatalf("cancel(): confirms=%d cancels=%d, want 0/1", *confirms, *cancels)
	}
	if !d.ShouldClose() {
		t.Fatal("cancel() must mark the dialog closed")
	}
}

func TestConfirmDialog_EnterConfirmsEscCancels(t *testing.T) {
	d, confirms, cancels := newTestConfirm(t)
	if !d.HandleEnter() {
		t.Fatal("HandleEnter must consume the key")
	}
	if *confirms != 1 {
		t.Fatalf("HandleEnter → confirm: got %d", *confirms)
	}
	d2, _, cancels2 := newTestConfirm(t)
	if d2.HandleEscape() {
		t.Fatal("HandleEscape must return false so CloseTop removes the entry")
	}
	if *cancels2 != 1 {
		t.Fatalf("HandleEscape → cancel: got %d", *cancels2)
	}
	_ = cancels
}

func TestConfirmDialog_PublishesButtonHitAreas(t *testing.T) {
	d, _, _ := newTestConfirm(t)
	areas := d.HitAreas()
	// confirm + cancel + close + panel catch-all == 4 areas; no full-screen backdrop.
	if len(areas) != 4 {
		t.Fatalf("HitAreas len = %d, want 4 (confirm/cancel/close/panel)", len(areas))
	}
	for _, a := range areas {
		if a.Rect.Dx() >= 1280 && a.Rect.Dy() >= 720 {
			t.Fatal("no full-screen backdrop allowed — it would swallow outside taps")
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConfirmDialog`
Expected: FAIL — `undefined: newConfirmDialog`.

- [ ] **Step 3: Implement `confirm_dialog.go`**

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"image/color"
)

// confirmConfig declares a confirmation dialog. onConfirm/onCancel are the
// pluggable domain effects (the dialog itself never touches the graph or the
// mode manager); the caller owns whatever mode state sits behind the dialog.
type confirmConfig struct {
	id          string
	titleKey    i18n.Key
	confirmKey  i18n.Key
	destructive bool
	swatch      color.Color // optional instrument accent under the title; nil = none
	onConfirm   func()
	onCancel    func()
}

// ConfirmDialog is the single shared modal confirmation, rendered as a
// PortalOverlay in the settings-overlay shape: real *Buttons with keycap chrome,
// per-button hit areas + a panel catch-all (NO full-screen backdrop, so a press
// outside the panel misses every area and the tree's CloseTop route fires
// OnClose → onCancel — the shared click-outside contract). Implements
// portalEnterHandler (Enter → confirm) and portalEscapeHandler (Esc → cancel).
type ConfirmDialog struct {
	cfg        confirmConfig
	confirmBtn *Button
	cancelBtn  *Button
	closeBtn   *Button
	rect       image.Rectangle
	closed     bool
}

func newConfirmDialog(cfg confirmConfig) *ConfirmDialog {
	d := &ConfirmDialog{cfg: cfg}
	confirmSpec := ComponentButtonPrimary
	if cfg.destructive {
		confirmSpec = ComponentButtonDestructive
	}
	d.confirmBtn = NewButtonKey(cfg.confirmKey, confirmSpec, d.confirm)
	d.cancelBtn = NewButtonKey(i18n.KeyCancel, ComponentButtonSecondary, d.cancel)
	d.confirmBtn.SetTextRole(RoleBody)
	d.cancelBtn.SetTextRole(RoleBody)
	d.closeBtn = NewSpecButton("", ComponentButtonSecondary, d.cancel)
	d.closeBtn.Icon = string(IconClose)
	d.closeBtn.IconColor = closeIconColor()
	return d
}

// confirm/cancel fire the pluggable callback exactly once, then flag closed so
// the portal sweep (ShouldClose) removes the entry. Idempotent.
func (d *ConfirmDialog) confirm() {
	if d.closed {
		return
	}
	d.closed = true
	if d.cfg.onConfirm != nil {
		d.cfg.onConfirm()
	}
}

func (d *ConfirmDialog) cancel() {
	if d.closed {
		return
	}
	d.closed = true
	if d.cfg.onCancel != nil {
		d.cfg.onCancel()
	}
}

// SetRectProvider lets the caller center the dialog over its own pane. The
// provider returns the panel rect each Layout so the dialog re-centers on resize.
func (d *ConfirmDialog) Layout(_ , screenBounds image.Rectangle) {
	// Token-driven sizing: title width + both buttons + padding.
	title := i18n.T(d.cfg.titleKey)
	titleW := StyledTextWidth(title, RoleSectionHeader)
	btnH := BtnHeightMD
	btnW := 120
	if bw := StyledTextWidth(i18n.T(d.cfg.confirmKey), RoleBody) + 2*SpaceMD; bw > btnW {
		btnW = bw
	}
	w := titleW + 2*SpaceMD + SpaceLG
	if min := 2*btnW + SpaceMD + 2*SpaceMD; w < min {
		w = min
	}
	h := SpaceMD + StyledTextHeight(RoleSectionHeader) + SpaceLG + btnH + SpaceMD
	b := screenBounds
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	d.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	// Buttons: Cancel left, Confirm right, bottom row.
	by := d.rect.Max.Y - SpaceMD - btnH
	d.cancelBtn.SetRect(image.Rect(d.rect.Min.X+SpaceMD, by, d.rect.Min.X+SpaceMD+btnW, by+btnH))
	d.confirmBtn.SetRect(image.Rect(d.rect.Max.X-SpaceMD-btnW, by, d.rect.Max.X-SpaceMD, by+btnH))
	d.closeBtn.SetRect(closeButtonRect(d.rect, SpaceXS))
}

func (d *ConfirmDialog) HitAreas() []HitArea {
	if d.rect.Empty() {
		return nil
	}
	return []HitArea{
		{Rect: d.confirmBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-confirm",
			Handler: &buttonHitAdapter{btn: d.confirmBtn}},
		{Rect: d.cancelBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-cancel",
			Handler: &buttonHitAdapter{btn: d.cancelBtn}},
		{Rect: d.closeBtn.Rect(), ZIndex: ZOverlayMin, Tag: d.cfg.id + "-close",
			Handler: &buttonHitAdapter{btn: d.closeBtn}},
		// Panel catch-all LAST: swallows in-panel taps that miss a button so
		// they never fall through, but covers ONLY the panel rect — outside
		// taps still yield len(hits)==0 → tree CloseTop → OnClose → cancel.
		{Rect: d.rect, ZIndex: ZOverlayMin, Tag: d.cfg.id + "-panel",
			Handler: confirmPanelHandler{}},
	}
}

// HoverAt refreshes desktop hover for the three buttons (portalHoverer).
func (d *ConfirmDialog) HoverAt(mx, my int) {
	d.confirmBtn.UpdateHover(mx, my)
	d.cancelBtn.UpdateHover(mx, my)
	d.closeBtn.UpdateHover(mx, my)
}

func (d *ConfirmDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.rect.Empty() {
		return
	}
	drawScrim(screen)
	drawPanel(screen, d.rect)
	headerRect := image.Rect(d.rect.Min.X, d.rect.Min.Y, d.rect.Max.X, d.rect.Min.Y+SpaceMD+StyledTextHeight(RoleSectionHeader))
	drawMenuTitle(screen, headerRect, i18n.T(d.cfg.titleKey), d.cfg.swatch)
	d.cancelBtn.Draw(screen)
	d.confirmBtn.Draw(screen)
	d.closeBtn.Draw(screen)
}

func (d *ConfirmDialog) ShouldClose() bool { return d.closed }

// HandleEnter (portalEnterHandler): Enter accepts.
func (d *ConfirmDialog) HandleEnter() bool { d.confirm(); return true }

// HandleEscape (portalEscapeHandler): Esc cancels; return false so the universal
// handler's CloseTop removes the (still-topmost) entry — the proven mid-stack
// contract (see grid_move_break_confirm.go history).
func (d *ConfirmDialog) HandleEscape() bool { d.cancel(); return false }

// confirmPanelHandler swallows taps on the panel background (missing both
// buttons) so they don't fall through to a lower surface.
type confirmPanelHandler struct{}

func (confirmPanelHandler) OnPress(x, y int) InputResult         { return InputConsumed }
func (confirmPanelHandler) OnDrag(x, y int)                      {}
func (confirmPanelHandler) OnRelease(x, y int)                   {}
func (confirmPanelHandler) OnWheel(x, y, steps int) InputResult  { return InputConsumed }
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestConfirmDialog`
Expected: PASS. (If `SpaceLG` is absent, use `SpaceMD*2`; verify token names against `touch_sizes.go`.)

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/confirm_dialog.go src/go/internal/ui/confirm_dialog_test.go
git commit -m "feat(ui): shared ConfirmDialog portal overlay (reusable modal confirm)"
```

---

## Task 4: Refactor paste-overwrite onto `ConfirmDialog`

**Files:**
- Modify: `internal/ui/node_clipboard.go` (lines ~171-309)
- Test: `internal/ui/node_clipboard_test.go` (add), existing paste tests still pass

**Interfaces:**
- Consumes: `newConfirmDialog`, `ConfirmDialog`.
- Keeps: `pasteConfirmActive()`, `beginPasteConfirm`, `ConfirmPasteOverwrite`, `CancelPasteOverwrite`.

- [ ] **Step 1: Add a field for the dialog** — in the `Game` struct's paste state (where `pastePending` lives), add:

```go
	pasteConfirmDlg *ConfirmDialog
```

- [ ] **Step 2: Rewrite `beginPasteConfirm` + portal open** — replace `openPasteConfirmPortal`, `pasteConfirmRect`, `pasteConfirmOverwriteBtnRect`, `pasteConfirmCancelBtnRect`, `drawPasteConfirm`, and the manual `inputFn`/`dvOverlayPortal` block with:

```go
func (g *Game) beginPasteConfirm(i, j int) {
	g.pastePending = pastePendingState{active: true, i: i, j: j, snap: g.clipboard}
	g.openPasteConfirmPortal()
}

func (g *Game) openPasteConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.pasteConfirmDlg = newConfirmDialog(confirmConfig{
		id:          pasteConfirmPortalID,
		titleKey:    i18n.KeyConfirmReplaceNode,
		confirmKey:  i18n.KeyCapOverwrite,
		destructive: true,
		onConfirm:   g.ConfirmPasteOverwrite,
		onCancel:    g.CancelPasteOverwrite,
	})
	// Center over the grid content pane.
	g.pasteConfirmDlg.Layout(image.Rectangle{}, g.gridPaneScreenRect())
	g.drum.portal().Open(PortalEntry{
		ID:      pasteConfirmPortalID,
		Modal:   true,
		Scrim:   true,
		Overlay: g.pasteConfirmDlg,
		OnClose: g.CancelPasteOverwrite, // outside-click / CloseTop path
	})
}
```

> `gridPaneScreenRect()` — reuse the existing grid-pane rect helper the old `pasteConfirmRect` used (`g.split.GridRect(...)` + `gridTopOffset()`); if no single-call helper exists, add a tiny `func (g *Game) gridPaneScreenRect() image.Rectangle` returning that rect and reuse it in break-circuit too. `ConfirmPasteOverwrite`/`CancelPasteOverwrite` already close the portal and clear `pastePending`; make them idempotent (guard on `pastePending.active`, already present) and also nil `g.pasteConfirmDlg`.

- [ ] **Step 3: Ensure Confirm/Cancel clear the dialog ref** — in both `ConfirmPasteOverwrite` and `CancelPasteOverwrite`, after `g.closePasteConfirmPortal()` add `g.pasteConfirmDlg = nil`. Keep `closePasteConfirmPortal` (calls `portal().Close(pasteConfirmPortalID)`).

- [ ] **Step 4: Write a functional test** — `internal/ui/node_clipboard_test.go`:

```go
func TestPasteConfirm_OutsideClickCancels(t *testing.T) {
	g := newTestGameForUndo(t) // existing helper; see testutil_test.go
	// Arrange an occupied target so paste routes to the confirm.
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	if n == nil {
		t.Fatal("setup node nil")
	}
	g.snapshotNodeToClipboard(n)
	g.beginPasteConfirm(3, 1)
	if !g.pasteConfirmActive() {
		t.Fatal("paste confirm should be active")
	}
	// A press well outside the dialog panel must cancel (tree CloseTop → OnClose).
	clickAt(g, 5, 5) // helper: injects a press+release, drives Update
	if g.pasteConfirmActive() {
		t.Fatal("outside click must cancel the paste confirm")
	}
	if g.nodeAt(3, 1) != n {
		t.Fatal("cancel must not mutate the graph")
	}
}
```

> Use the project's existing game/test helpers (`newTestGame`, `clickAt`/tap injection, `assertDefaultParityState`) — check `testutil_test.go` header for the exact names and substitute. If a `clickAt` helper does not exist, drive `g.Update()` after setting the input override (see `game_input_touch_test.go` for the pattern).

- [ ] **Step 5: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Paste|Clipboard'`
Expected: PASS (new outside-click test + existing paste tests).

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/node_clipboard.go src/go/internal/ui/node_clipboard_test.go
git commit -m "refactor(ui): paste-overwrite confirm uses shared ConfirmDialog + click-outside cancel"
```

---

## Task 5: Refactor break-circuit onto `ConfirmDialog` (mode-preserving)

**Files:**
- Modify: `internal/ui/grid_move_break_confirm.go`
- Test: `internal/ui/grid_move_break_confirm_test.go` (add), existing move/break tests still pass

**Interfaces:**
- Keeps: `breakConfirmModeObserver`, `beginBreakConfirm`, `ConfirmBreakCircuit`, `CancelBreakCircuit`, `breakConfirmActive()`.

- [ ] **Step 1: Add a dialog field** next to `breakPending`:

```go
	breakConfirmDlg *ConfirmDialog
```

- [ ] **Step 2: Replace the hand-drawn dialog** — delete `breakConfirmRect`, `breakConfirmBreakBtnRect`, `breakConfirmCancelBtnRect`, `drawBreakConfirm`, and the `dvOverlayPortal` block in `openBreakConfirmPortal`; replace with:

```go
func (g *Game) openBreakConfirmPortal() {
	if g.drum == nil || g.drum.portal() == nil {
		return
	}
	g.breakConfirmDlg = newConfirmDialog(confirmConfig{
		id:          breakConfirmPortalID,
		titleKey:    i18n.KeyBreakCircuitTitle,
		confirmKey:  i18n.KeyCapBreak,
		destructive: true,
		onConfirm:   g.ConfirmBreakCircuit,
		onCancel:    g.CancelBreakCircuit,
	})
	g.breakConfirmDlg.Layout(image.Rectangle{}, g.gridPaneScreenRect())
	g.drum.portal().Open(PortalEntry{
		ID:      breakConfirmPortalID,
		Modal:   true,
		Scrim:   true,
		Overlay: g.breakConfirmDlg,
		OnClose: g.CancelBreakCircuit, // outside-click path: cancel; Move STAYS ARMED
	})
}
```

- [ ] **Step 3: Preserve mode semantics** — `CancelBreakCircuit` must clear only the dialog (`breakPending` + `g.breakConfirmDlg = nil` + `closeBreakConfirmPortal`) and **must not** cancel Move mode (the user retries). `ConfirmBreakCircuit` keeps its `moveNode` + `g.modes.Commit()` path and nils `g.breakConfirmDlg`. The `breakConfirmModeObserver` is unchanged (still `Register`ed in `game_new.go`). Because the dialog no longer defines `escapeFn`, the mid-stack Esc contract now lives in `ConfirmDialog.HandleEscape` (returns false → CloseTop), which is behaviorally identical.

- [ ] **Step 4: Write a functional test** — `internal/ui/grid_move_break_confirm_test.go`:

```go
func TestBreakConfirm_OutsideClickCancels_MoveStaysArmed(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.modes.Enter(GridModeMove, n)
	g.beginBreakConfirm(n, 4, 1)
	if !g.breakConfirmActive() {
		t.Fatal("break confirm should be active")
	}
	clickAt(g, 5, 5) // outside the panel
	if g.breakConfirmActive() {
		t.Fatal("outside click must cancel the break confirm")
	}
	if !g.modes.Is(GridModeMove) {
		t.Fatal("Move mode must stay armed after break-confirm cancel")
	}
}
```

- [ ] **Step 5: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Break|Move'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/grid_move_break_confirm.go src/go/internal/ui/grid_move_break_confirm_test.go
git commit -m "refactor(ui): break-circuit confirm uses shared ConfirmDialog; Move stays armed on cancel"
```

---

## Task 6: Consolidate Esc/Enter handling for confirms

**Files:**
- Modify: `internal/ui/game_input_shortcuts.go` (rungs ~411-420; Enter handler ~512-523)
- Test: `internal/ui/confirm_keyboard_test.go` (add)

**Interfaces:**
- Consumes: `ConfirmDialog.HandleEnter/HandleEscape` (via the portal `portalEnterHandler`/`portalEscapeHandler` seams, already dispatched by `handleEnter`/`handleEscape`).

- [ ] **Step 1: Replace the two headless Esc rungs with one** — where `handleEscape` has the `pasteConfirmActive → CancelPasteOverwrite` and `breakConfirmActive → CancelBreakCircuit` rungs (~411-420), collapse to a single headless fallback (the portal path already handles the normal case via `HandleEscape`):

```go
	// Headless / no-portal fallback: with a portal present, the topmost
	// overlay's HandleEscape (ConfirmDialog) already ran above. This covers
	// the portal==nil path (tests) where no overlay exists to receive Esc.
	if g.drum.portal() == nil {
		if g.pasteConfirmActive() {
			g.CancelPasteOverwrite()
			return true
		}
		if g.breakConfirmActive() {
			g.CancelBreakCircuit()
			return true
		}
	}
```

- [ ] **Step 2: Write the failing test** — `internal/ui/confirm_keyboard_test.go`:

```go
func TestConfirm_EnterConfirms_EscCancels(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.snapshotNodeToClipboard(n)

	// Enter accepts the overwrite.
	g.beginPasteConfirm(3, 1)
	pressKey(t, g, ebiten.KeyEnter) // helper drives Update with the key edge
	if g.pasteConfirmActive() {
		t.Fatal("Enter must confirm (dialog closed)")
	}

	// Esc cancels.
	m := g.tryAddNode(5, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.snapshotNodeToClipboard(m)
	g.beginPasteConfirm(5, 1)
	pressKey(t, g, ebiten.KeyEscape)
	if g.pasteConfirmActive() {
		t.Fatal("Esc must cancel (dialog closed)")
	}
}
```

> Use the project's existing key-injection helper (check `undo_keyboard_dispatch_test.go` / `game_input_shortcuts` tests for the real `pressKey` equivalent) and substitute. This test both fixes the previously-broken Enter-to-confirm and pins Esc.

- [ ] **Step 3: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Confirm|Escape|Enter'`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/game_input_shortcuts.go src/go/internal/ui/confirm_keyboard_test.go
git commit -m "feat(ui): Enter-to-confirm for confirm dialogs; consolidate headless Esc fallback"
```

---

## Task 7: Node popup — icon+label keycap draw upgrade

**Files:**
- Modify: `internal/ui/game_longpress_popup.go` (`showLongPressPopup` sizing ~29-41; `drawLongPressPopup` ~197-247)
- Test: `internal/ui/longpress_popup_render_test.go` (extend)

**Interfaces:**
- Consumes: `drawIconByID`, `IconMove/IconCopy/IconConnect/IconTrash`, `DrawTextStyled`.

- [ ] **Step 1: Widen the keycaps for icon+label** — in `showLongPressPopup`, bump the ideal `btnW` so an icon (IconSizeMD) stacked above a `RoleBody` label fits; keep the mobile width-clamp. Change `btnW := 80` to a token-derived width and add vertical room:

```go
	btnW := 88
	btnH := Profile().DensityValues().PopupBtnH + IconSizeMD + SpaceXS // icon row + label row
```

(Keep the existing `maxBtnW` clamp and `panelW/panelH` math — they derive from `btnW`/`btnH`.)

- [ ] **Step 2: Add a keycap-with-icon helper** — in `game_longpress_popup.go`, factor the repeated per-button block into:

```go
// drawPopupActionCap renders one quick-action keycap: shared keycap chrome,
// a leading/top icon, and a centered label. tint colors BOTH icon and label
// (instrument accent on hover); delete overrides via its own fill/label.
func drawPopupActionCap(dst *ebiten.Image, rect image.Rectangle, icon IconID, label string, fill, border, tint color.Color, hovered bool) {
	cap := drawKeycapPanelButton(dst, rect, fill, border, hovered)
	iconSz := IconSizeMD
	ix := cap.Min.X + (cap.Dx()-iconSz)/2
	iy := cap.Min.Y + SpaceXS
	drawIconByID(dst, icon, image.Rect(ix, iy, ix+iconSz, iy+iconSz), tint)
	lw := StyledTextWidth(label, RoleBody)
	lx := cap.Min.X + (cap.Dx()-lw)/2
	ly := iy + iconSz + SpaceXS
	DrawTextStyled(dst, label, lx, ly, RoleBody, tint)
}
```

- [ ] **Step 3: Rewrite `drawLongPressPopup` body** to call the helper 4×:

```go
func (g *Game) drawLongPressPopup(dst *ebiten.Image) {
	if !g.longPressPopup {
		return
	}
	accent := g.longPressPopupAccent()
	drawScrim(dst)
	drawPanel(dst, g.longPressPopupRect)

	tint := func(hover bool) color.Color {
		if hover {
			return accent
		}
		return color.Color(genColorPopupTextSecondary)
	}
	drawPopupActionCap(dst, g.longPressPopupMove, IconMove, i18n.T(i18n.KeyCapMove),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "move"), g.longPressPopupHover == "move")
	drawPopupActionCap(dst, g.longPressPopupCopy, IconCopy, i18n.T(i18n.KeyCapCopy),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "copy"), g.longPressPopupHover == "copy")
	drawPopupActionCap(dst, g.longPressPopupConn, IconConnect, i18n.T(i18n.KeyCapConnect),
		colDropdown, colTransportBorder, tint(g.longPressPopupHover == "connect"), g.longPressPopupHover == "connect")
	// Delete: destructive fill + dark-on-neon icon+label.
	drawPopupActionCap(dst, g.longPressPopupDel, IconTrash, i18n.T(i18n.KeyMenuDelete),
		colDeleteFill, colDeleteBorder, longPressDeleteTextColor(), g.longPressPopupHover == "delete")
}
```

- [ ] **Step 4: Extend the render test** — in `longpress_popup_render_test.go`, assert the popup draws the Delete keycap with the destructive fill and that each keycap's rect is non-empty after `showLongPressPopup` (keep the existing hit-rect/geometry assertions; adjust expected sizes for the taller keycaps). Add:

```go
func TestLongPressPopup_KeycapsHaveIconRoom(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	sx1, sy1, _, _ := g.nodeScreenRect(n)
	g.showLongPressPopup(n, int(sx1), int(sy1))
	// Each keycap must be tall enough for icon (IconSizeMD) + label row.
	if g.longPressPopupMove.Dy() < IconSizeMD+StyledTextHeight(RoleBody) {
		t.Fatalf("move keycap too short for icon+label: %d", g.longPressPopupMove.Dy())
	}
}
```

- [ ] **Step 5: Run tests + capture a screenshot**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'LongPress|Longpress'`
Then: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd && xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene node_longpress_menu -screenshot /tmp/node_longpress_after.png`
Expected: PASS; the screenshot shows icon+label keycaps.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/game_longpress_popup.go src/go/internal/ui/longpress_popup_render_test.go
git commit -m "feat(ui): node quick-action popup keycaps gain leading icons"
```

---

## Task 8: Node popup — dismiss on outside press (pop-up contract)

**Files:**
- Modify: `internal/ui/game_longpress_popup.go` (`updateLongPressPopup`)
- Test: `internal/ui/longpress_popup_test.go` (extend)

**Interfaces:**
- Keeps: `showLongPressPopup` (setup), `dismissLongPressPopup(reason)` (cancellation).

- [ ] **Step 1: Write the failing test** — in `longpress_popup_test.go`:

```go
func TestLongPressPopup_OutsidePressDismisses(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	sx1, sy1, _, _ := g.nodeScreenRect(n)
	g.showLongPressPopup(n, int(sx1), int(sy1))
	if !g.longPressPopup {
		t.Fatal("popup should be open")
	}
	// A fresh press that lands OUTSIDE the panel dismisses it (click-outside).
	outX, outY := g.longPressPopupRect.Min.X-40, g.longPressPopupRect.Min.Y-40
	if outX < 0 { outX = g.longPressPopupRect.Max.X + 40 }
	g.leftPrev = false
	g.updateLongPressPopup(outX, outY, true) // press-down outside
	if g.longPressPopup {
		t.Fatal("press outside the panel must dismiss the popup")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLongPressPopup_OutsidePressDismisses`
Expected: FAIL — popup still open (current code only dismisses on release).

- [ ] **Step 3: Add outside-press dismissal** — in `updateLongPressPopup`, after computing hover but before the release branch, add a press-edge outside check:

```go
	// Click-outside contract: a NEW press (down-edge) that lands outside the
	// panel dismisses the popup, matching every other pop-up. The slide-release
	// path below still handles the hold-slide-release gesture.
	pressEdge := left && !g.leftPrev
	if pressEdge && !pt.In(g.longPressPopupRect) {
		g.dismissLongPressPopup("outside-press")
		g.leftPrev = left
		return
	}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'LongPress|Longpress'`
Expected: PASS (new outside-press test + existing slide-release tests unaffected — those open under a held finger, so `pressEdge` is false).

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_longpress_popup.go src/go/internal/ui/longpress_popup_test.go
git commit -m "feat(ui): node popup dismisses on outside press (shared click-outside contract)"
```

---

## Task 9: Consolidate armed-mode banners into `drawGridModeBanner`

**Files:**
- Modify: `internal/ui/grid_pane_draw.go` (`drawGridMoveMode` banner blocks ~1238-1283)
- Modify: `internal/ui/game_connect_mode.go` (`drawConnectMode` banner ~64-74)
- Test: `internal/ui/grid_mode_banner_test.go` (add)

**Interfaces:**
- Produces: `func (g *Game) drawGridModeBanner(dst *ebiten.Image)` — draws one styled banner for the active mode; returns early when `GridModeNone`.

- [ ] **Step 1: Write the failing test** — `internal/ui/grid_mode_banner_test.go`:

```go
func TestGridModeBanner_TextIsStyledAndLocalized(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()
	// No banner when idle.
	if got := g.gridModeBannerText(); got != "" {
		t.Fatalf("idle banner = %q, want empty", got)
	}
	g.modes.Enter(GridModeMove, n)
	if got := g.gridModeBannerText(); got == "" || got == "MOVING NODE (3,1) — TAP A POINT ON AN EDGE" {
		t.Fatalf("move banner should be localized title-case, got %q", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestGridModeBanner`
Expected: FAIL — `undefined: gridModeBannerText`.

- [ ] **Step 3: Implement the shared banner** — add to `grid_pane_draw.go`:

```go
// gridModeBannerText returns the localized banner string for the active armed
// mode, or "" when idle. Single source of truth for all three modes.
func (g *Game) gridModeBannerText() string {
	s := g.modes.Subject()
	switch g.modes.Active() {
	case GridModeMove:
		return fmt.Sprintf(i18n.T(i18n.KeyBannerMoving), s.I, s.J)
	case GridModeCopy:
		return i18n.T(i18n.KeyBannerPasting)
	case GridModeConnect:
		return fmt.Sprintf(i18n.T(i18n.KeyBannerConnecting), s.I, s.J)
	}
	return ""
}

// drawGridModeBanner renders the armed-mode banner (styled text + panel chrome)
// centered at the top of the grid pane. Replaces the three bespoke banners.
func (g *Game) drawGridModeBanner(dst *ebiten.Image) {
	text := g.gridModeBannerText()
	if text == "" {
		return
	}
	if Profile().ShowEscHint {
		text += "  " + i18n.T(i18n.KeyEscToCancel) // reuse existing esc-hint key if present; else inline "(Esc)"
	}
	w := StyledTextWidth(text, RoleBody) + 2*SpaceMD
	x := (g.split.GridW(g.winW) - w) / 2
	if x < 0 {
		x = 0
	}
	rect := image.Rect(x, SpaceXS, x+w, SpaceXS+StyledTextHeight(RoleBody)+2*SpaceXS)
	drawPanel(dst, rect)
	DrawTextStyled(dst, text, rect.Min.X+SpaceMD, rect.Min.Y+SpaceXS, RoleBody, colTextPrimary)
}
```

> If no `KeyEscToCancel` exists, add it in Task 2's key set (`"(Esc to cancel)"` / `"(Esc para cancelar)"`) rather than appending raw text — keep it i18n. Verify against `i18n/keys.go`.

- [ ] **Step 4: Delete the three old banner blocks** — in `drawGridMoveMode` remove the Move `bannerText`/`DrawTextAt` block (keep the moving-node outline + ghost) and the Copy `bannerText`/`DrawTextAt` block (keep the ghost); in `drawConnectMode` remove its banner block (keep the source-node outline). Call `g.drawGridModeBanner(dst)` once from the grid layer that previously drew them (the `drawGridMoveMode` layer site) — or register it so it draws for all three modes regardless of which layer runs. Confirm via `grep` that `DrawTextAt(` is gone from these banner sites.

- [ ] **Step 5: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'GridModeBanner|Move|Connect|Copy'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/grid_pane_draw.go src/go/internal/ui/game_connect_mode.go src/go/internal/ui/grid_mode_banner_test.go
git commit -m "refactor(ui): single styled armed-mode banner driven by gridModeManager"
```

---

## Task 10: DESIGN.md + token cleanup, component docs

**Files:**
- Modify: `DESIGN.md` (remove `viz-confirm-green`/`viz-cancel-red`; doc component + icons)
- Modify: `internal/ui/design_colors.gen.go` (regen) — via `make gen-design-tokens`
- Modify: `internal/ui/design_md_drift_test.go` (want-map), `token_discipline_test.go` (budgets)

**Interfaces:** none (docs/tokens).

- [ ] **Step 1: Confirm the tokens are unused** —

Run: `cd src/go && grep -rn "VizConfirm\|VizCancel\|viz-confirm\|viz-cancel\|ConfirmGreen\|CancelRed" internal/ ../../DESIGN.md | grep -v _test`
Expected: only DESIGN.md YAML + `design_colors.gen.go` definitions (no runtime call sites). If any runtime use appears, STOP and reassess.

- [ ] **Step 2: Remove the tokens from DESIGN.md** — delete the `viz-confirm-green:` and `viz-cancel-red:` YAML lines and their comment block (`DESIGN.md` ~275-279).

- [ ] **Step 3: Regenerate tokens**

Run: `cd /home/ymolinar/Repos/beatmo && make gen-design-tokens`
Expected: `design_colors.gen.go` no longer defines the two constants.

- [ ] **Step 4: Update the drift test want-map** — in `design_md_drift_test.go`, remove the two removed tokens from the expected color map so the count matches.

- [ ] **Step 5: Document the new component + icons in DESIGN.md** —
  - Under **Components**, add a `confirm-dialog` note: "Shared modal confirmation (`confirm_dialog.go`). `surface-overlay` panel via `drawPanel`, `section-header` title via `drawMenuTitle`, two keycap `*Button`s (Cancel = secondary, Confirm = destructive/primary), close-X = cancel. Closes on Confirm/Cancel/close-X/Esc/Enter and on click-outside (tree `CloseTop`). Consumers: paste-overwrite, break-circuit."
  - Under the **node pop-up** invariant (line ~1855) and the Icon Map appendix, add `IconMove`/`IconCopy`/`IconConnect` and note the quick-action bar is icon+label keycaps.

- [ ] **Step 6: Run the guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS. If `TestTokenDiscipline` reports a file below budget (literals removed from the confirm twins), ratchet its `allowedLiteralBudget` down to the new count.

- [ ] **Step 7: Commit**

```bash
git add DESIGN.md src/go/internal/ui/design_colors.gen.go src/go/internal/ui/design_md_drift_test.go src/go/internal/ui/token_discipline_test.go
git commit -m "docs(design): remove orphaned viz-confirm/viz-cancel tokens; document ConfirmDialog + node-action icons"
```

---

## Task 11: Scenes + visual verification

**Files:**
- Modify: `internal/ui/scene_catalog.go` (add two scenes)
- Test: `scene_crop_test.go` / existing scene enumeration covers new names

**Interfaces:** none.

- [ ] **Step 1: Add the two confirmation scenes** — in `scene_catalog.go`, in the graph/node section:

```go
	{Name: "paste_confirm_dialog", Description: "paste-overwrite confirmation modal", Mobile: true, SettleFrames: 90,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				g.snapshotNodeToClipboard(n)
				g.beginPasteConfirm(3, 1)
			}
		}},
	{Name: "break_circuit_confirm", Description: "break-the-circuit confirmation modal", Mobile: true, SettleFrames: 90,
		Setup: func(g *Game) {
			n := g.tryAddNode(3, 1, model.NodeTypeRegular)
			g.updateBeatInfos()
			if n != nil {
				g.modes.Enter(GridModeMove, n)
				g.beginBreakConfirm(n, 4, 1)
			}
		}},
```

- [ ] **Step 2: Build + capture before/after screenshots**

Run:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd
for s in node_longpress_menu paste_confirm_dialog break_circuit_confirm context_menu_open; do
  xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene "$s" -screenshot "/tmp/after_$s.png"
done
```
Expected: four PNGs. Visually verify: popup = icon+label keycaps in a panel; both confirms = `surface-overlay` panel, section-header title, red Confirm, neutral Cancel, close-X — matching `context_menu_open`.

- [ ] **Step 3: Run the scene tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Scene|Crop'`
Expected: PASS (new scenes enumerated; setups produce visible surfaces).

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/scene_catalog.go
git commit -m "test(ui): scenes for paste-overwrite + break-circuit confirm dialogs"
```

---

## Task 12: Full-suite verification + cross-platform bridge

**Files:** none (verification only).

- [ ] **Step 1: Full fast Go suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/i18n/`
Expected: PASS. Investigate any failure before proceeding (do not dismiss as pre-existing — prove causation).

- [ ] **Step 2: Bridge smoke (no new JS exports expected, but confirm nothing drifted)**

Run: `GO=$(pwd)/../../.tools/go/bin/go node ../js/wasm_bridge_smoke.browser.test.js` (from `src/go`, adjust path; or from repo root `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`)
Expected: PASS (this change adds no JS exports; the catalogue is unchanged).

- [ ] **Step 3: Run the design guards + forbidden glyphs once more**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestForbidden|TestCatalogCompleteness|TestGridMode' ./internal/ui/ ./internal/i18n/`
Expected: PASS.

- [ ] **Step 4: Verify via the run/verify skill** — drive the desktop app, open a node popup (click-hold a node), confirm icons render and click-outside dismisses; trigger a paste-overwrite and a break-circuit confirm, verify Confirm/Cancel/Esc/Enter/click-outside all behave. Capture a final screenshot set.

- [ ] **Step 5: Final commit (if any verification-driven tweaks were needed)**

```bash
git add -A
git commit -m "chore(ui): node popup + confirm menus consistency — verification pass"
```

---

## Self-Review Notes (author)

- **Spec coverage:** A (ConfirmDialog) → Tasks 3-6; B (popup draw) → Task 7; popup dismissal → Task 8; C (icons) → Task 1; D (banner) → Task 9; E (token/doc cleanup) → Task 10; F (i18n) → Task 2; scenes/tests → Tasks 11-12; click-outside contract → Tasks 3 (HitAreas shape), 4/5 (OnClose), 8 (popup). ✓
- **Placeholders:** helper names flagged for substitution (`newTestGame`, `clickAt`, `pressKey`, `gridPaneScreenRect`, `KeyEscToCancel`) point the worker to the exact existing files to confirm the real names — resolve at implementation, do not invent.
- **Type consistency:** `confirmConfig`/`ConfirmDialog`/`newConfirmDialog` used identically across Tasks 3-6; `drawPopupActionCap` defined and used in Task 7; `gridModeBannerText`/`drawGridModeBanner` defined and used in Task 9.
- **Token names** (`SpaceLG`, `IconSizeMD`, `BtnHeightMD`, `ZOverlayMin`) must be confirmed against `touch_sizes.go` / the tree z-constants at implementation; substitute the nearest existing token if a name differs.
