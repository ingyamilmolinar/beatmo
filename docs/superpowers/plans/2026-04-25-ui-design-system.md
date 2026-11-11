# UI Design System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a unified, consistently-applied UI design system covering color semantics, button taxonomy, icon usage, and panel patterns — codified in DESIGN.md and enforced via a Go `UIStyle` configuration layer that makes all button/menu/panel styling one-liner calls.

**Architecture:** Three layers — (1) `DESIGN.md` spec document for humans and AI agents, (2) `theme_tokens.go` with named semantic tokens on top of raw colors, (3) `ui_style.go` with `UIStyle`/`PanelSpec`/`MenuSpec` builder structs that wire theme tokens into drawable widgets. Existing `theme.go`, `components.go`, `drawing.go` stay intact; new files extend them.

**Tech Stack:** Go 1.23, Ebiten v2, existing `ButtonStyle`/`ButtonVisual`/`DrawIcon` infrastructure

---

## File Map

| File | Action | Purpose |
|---|---|---|
| `DESIGN.md` | **Create** | Human + AI design reference: tokens, button taxonomy, icon map, panel rules |
| `CLAUDE.md` | **Modify** | Add 2 lines documenting `make screenshot` / `make screenshots-all` |
| `src/go/internal/ui/theme_tokens.go` | **Create** | Named semantic tokens (ColorToken, SizeToken) derived from raw theme vars |
| `src/go/internal/ui/ui_style.go` | **Create** | `UIStyle`, `PanelSpec`, `MenuSpec` — one-call styling for buttons/panels/menus |
| `src/go/internal/ui/ui_style_test.go` | **Create** | Tests for UIStyle correctness |
| `src/go/internal/ui/row_rack_zone.go` | **Modify** | Replace `NewButton("M",…)`, `"S"`, `"FX"`, `"O"`, `"X"`, `"+"` with icon variants |
| `src/go/internal/ui/drumview_fx_panel.go` | **Modify** | Replace `"▶"/"▼"` chevron text, `"✕"` close, `"+ Add Effect"` with icon buttons |

---

## Task 1: Document Screenshot Infrastructure in CLAUDE.md

**Files:**
- Modify: `CLAUDE.md` (Quick Start table, ~line 17)

- [ ] **Step 1: Add screenshot rows to Quick Start table**

In `CLAUDE.md`, find the Quick Start table (around line 17) and add two rows after `| Full pipeline | \`make test\` |`:

```markdown
| Capture UI screenshots | `make screenshot` (desktop + browser; output → `screenshots/`) |
| Capture all UI scenes  | `make screenshots-all` (all scenes; `SCENES=name` to filter) |
```

- [ ] **Step 2: Verify no duplicate entries**

```bash
grep -n "screenshot" /home/ymolinar/Repos/beatmo/CLAUDE.md
```
Expected: lines in Quick Start table only — no duplicates.

- [ ] **Step 3: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add CLAUDE.md
git commit -m "docs: add screenshot infrastructure to CLAUDE.md Quick Start table"
```

---

## Task 2: Create DESIGN.md

**Files:**
- Create: `DESIGN.md` at repo root

- [ ] **Step 1: Create DESIGN.md**

Create `/home/ymolinar/Repos/beatmo/DESIGN.md` with the full design system document (content below). This is the authoritative reference for all future UI work.

```markdown
# Tunkul UI Design System

> This file is the authoritative design reference for the Tunkul UI. Any model or
> engineer touching UI code must follow these rules. Deviations require updating
> this document first.

---

## 1. Color Tokens

### Surface Hierarchy (dark theme, 4 levels)

| Token | Value | Usage |
|---|---|---|
| `colBGTop` / `colBGBottom` | `#0C0D10` | Grid pane, timeline canvas (deepest) |
| `colSurface1` | `#14151A` | Row rack, transport bar background |
| `colSurface2` | `#1C1E24` | Cards, buttons, input fields (elevated) |
| `colSurface3` | `#24262E` | Hover states, active surfaces (raised) |
| `colPanelBG` | `#181920FA` | Overlay panels, popups, bottom sheets |

**Rule:** Nest surfaces in order: Surface1 contains Surface2 buttons. Never use a lower-level surface inside a higher-level container.

### Accent Colors

| Token | Value | Semantic |
|---|---|---|
| `colAccent` | `#00C8FF` | Primary interactive / active state |
| `colAccentBright` | `#50DCFF` | Hover / focus ring |
| `colAccentDim` | `#008CC8` | Splitter, secondary emphasis |
| `colAccentSubtle` | `#00C8FF14` | Backgrounds, fills (8% opacity) |

**Rule:** Use `colAccent` for active/selected state. Use `colAccentBright` only for hover. Never use accent for decorative purposes.

### Semantic State Colors

| Token | Value | Role | Usage |
|---|---|---|---|
| `colPlayGreen` | `#3CD278` | Positive / running | Play icon tint (mobile), play-active indicator |
| `colStopRed` / `colStopButton` | `#DC4646` / `#B43737` | Stop / error | Stop icon tint (mobile), error text |
| `colMuteRed` / `colMuteActive` | `#A03C32` | Muted channel | Mute button active fill |
| `colDeleteFill` | `#782323` | Destructive action | Delete button background |
| `colDeleteBorder` | `#B43737` | Destructive border | Delete button border |
| `colRecordIdle` | `#C83232CC` | Record ready | Record icon tint |
| `colRecordActive` | `#F02828CC` | Recording | Record icon tint while active |

**Rule — three reds, three distinct roles:**
- `colStopRed` = stop/error text only (never a button fill)
- `colMuteActive` = mute button fill only
- `colDeleteFill` = destructive confirm button fill only

Never reuse a red variant for a different semantic role. Do not introduce a fourth red.

### Text Colors

| Token | Value | Usage |
|---|---|---|
| `colTextPrimary` | `#DCDEE4` | Main labels, button text |
| `colTextSecondary` | `#8C8E96` | Captions, secondary labels, inactive icons |
| `colTextDisabled` | `#50525A` | Grayed-out, non-interactive |
| `colTextAccent` | `#00C8FF` | Active section headers, links |

### Border Colors

| Token | Opacity | Usage |
|---|---|---|
| `colBorderSubtle` | white/8 | Section dividers |
| `colBorderMedium` | white/15 | Control borders (default) |
| `colBorderStrong` | white/25 | Focused input borders |
| `colPanelBorder` | white/20 | Overlay panel borders |

---

## 2. Typography Scale

All text uses the bundled debug font (6×16px cell per glyph) scaled via `TextScale`. Use these scales:

| Role | Scale | Usage |
|---|---|---|
| Panel title | 1.3× | Inspector / FX panel header |
| Section header | 1.0× | Collapsible section labels |
| Button label | 1.0× | Row controls, transport |
| Caption / value | 1.0× | BPM number, percentage values |
| Tooltip | 0.8× | Cursor labels, coordinates (desktop only) |

**Rule:** Never hardcode a pixel size. Always derive from `TextHeight()` / `TextWidth()` so the font can be swapped later.

---

## 3. Spacing & Radius Tokens

All spacing and radii are defined as package-level constants in `layout_profile.go` / `uigrid.go`. Use these — never magic numbers.

| Token | Value | Usage |
|---|---|---|
| `SpaceSM` | 4px | Tight gaps (button padding, icon inset) |
| `SpaceMD` | 8px | Normal gaps (section spacing) |
| `SpaceLG` | 12px | Loose gaps (panel padding) |
| `RadiusSM` | 4px | Small rounded corners (chips, pills) |
| `RadiusMD` | 8px | Medium rounded corners (popups) |
| `RadiusLG` | 12px | Large rounded corners (bottom sheets desktop) |
| `RadiusXL` | 16px | Extra-large (mobile bottom sheets) |
| `BtnHeightSM` | 28px | Small buttons (close, stepper) |
| `BtnHeightMD` | 32px | Medium buttons (row controls) |
| `BtnHeightLG` | 40px | Large buttons (transport, FAB) |
| `TouchMinTargetPx` | 44px | Minimum hit target on mobile |

---

## 4. Button Taxonomy

Every button in the UI falls into one of five categories. Use the matching `ButtonStyle` from `theme.go`.

### 4a. Primary (action)
- Style: `FABStyle` — `colAccent` fill, white border
- Use: Floating action buttons (mobile "+"), primary CTA
- Icon: mandatory (from icon set)

### 4b. Secondary (neutral transport / control)
- Style: `TransportPlayStyle` / `TransportMiscStyle` — `colSurface2` fill, `colBorderSubtle` border
- Use: Play, Stop, Record, BPM +/−, subdiv, view-switch
- Icon: mandatory (from icon set); text label only if icon is insufficient
- Desktop: `DrawTopEdgeHighlight = true` adds subtle 3D depth
- Mobile: flat look (`DrawTopEdgeHighlight = false`)

### 4c. Row control (inline)
- Style: `InstButtonStyle` — `colSurface2` fill, `colBorderSubtle` border
- Use: Mute, Solo, FX, Origin (target), Delete in row rack
- Icon: **always use icon — never text label** (see §5 Icon Map)
- Active state: override style — `MuteActiveStyle`, `SoloActiveStyle`, `FXActiveStyle`

### 4d. Destructive (delete / confirm)
- Style: `DeleteButtonStyle` — `colDeleteFill` fill, `colDeleteBorder` border
- Use: Delete row button, "Delete" context menu item, confirm dialogs
- Icon: `IconTrash` for delete buttons; text label acceptable in confirm dialogs
- Text color: `colStopRed`

### 4e. Disabled
- Style: `DisabledButtonStyle` — `color.RGBA{50,52,58,255}` fill, `colBorderSubtle` border
- Use: Any button that cannot be activated in current state
- No icon tint change required; opacity conveys state

### Button Hover / Press States

All `ButtonStyle.Draw` calls automatically handle hover (`+12` brightness) and press (`−20` brightness) via `adjustColor`. No additional code needed for standard buttons.

### Creating a New Button

```go
// Always use NewButton with an icon, never a text label for icon-capable slots.
btn := NewButton("", InstButtonStyle, nil)
btn.Icon = string(IconMute)
btn.IconColor = colTextSecondary  // or colTextPrimary when active

// For destructive:
del := NewButton("", DeleteButtonStyle, nil)
del.Icon = string(IconTrash)
del.IconColor = colStopRed
del.ConsumeOnPress = true
```

---

## 5. Icon Map — Required Usage

The icon set has 28 glyphs. Every icon has **exactly one** semantic role. Use the `DrawIcon(dst, id, r, col)` function or set `Button.Icon = string(IconID)`.

### 5a. Transport
| Icon | `IconID` | Usage | Never use for |
|---|---|---|---|
| Play triangle | `IconPlay` | Play button | Expand/collapse |
| Pause bars | `IconPause` | Pause (if added) | — |
| Stop square | `IconStop` | Stop button | Delete |
| Record dot+ring | `IconRecord` | Record button | — |

### 5b. Row Controls (mandatory — no text alternatives)
| Icon | `IconID` | Usage | Active tint |
|---|---|---|---|
| Speaker+strikethrough | `IconMute` | Mute button | `colMuteActive` bg, `colTextPrimary` icon |
| Headphones | `IconSolo` | Solo button | `colSoloActive` bg, `colAccentBright` icon |
| 4-point sparkle | `IconFx` | FX panel toggle | `color.RGBA{20,40,56}` bg, `colAccent` icon |
| Crosshair | `IconTarget` | Origin button | `colAccentSubtle` bg, `colAccent` icon |
| Trash bin | `IconTrash` | Delete row | `colDeleteFill` bg, `colStopRed` icon |

### 5c. Navigation / Control
| Icon | `IconID` | Usage |
|---|---|---|
| 3 dots vertical | `IconOverflow` | Context menu trigger |
| + cross | `IconPlus` | Add row, Add Effect button |
| − bar | `IconMinus` | Remove item |
| Chevron up | `IconChevronUp` | Stepper increment, collapsed→open (never text "^") |
| Chevron down | `IconChevronDown` | Stepper decrement, open→collapsed (never text "v") |
| × diagonals | `IconClose` | Close any panel/popup (never text "X" or "✕") |

### 5d. File Operations
| Icon | `IconID` | Usage |
|---|---|---|
| Up-arrow + bar | `IconUpload` | Upload / share |
| Down-arrow + tray | `IconImport` | Import JSON |
| Up-arrow + tray | `IconExport` | Export JSON |
| Floppy disk | `IconSave` | Save |

### 5e. View / State
| Icon | `IconID` | Usage |
|---|---|---|
| 3 lines | `IconRows` | Switch to row view |
| EQ bars | `IconAudio` | Switch to EQ/audio view |
| Closed padlock | `IconTrack` | Follow-on (locked scroll) |
| Open padlock | `IconTrackOff` | Follow-off (free scroll) |
| Speaker+waves | `IconSpeaker` | Volume on / audible |
| Speaker body only | `IconSpeakerOff` | Volume zero / muted |
| Musical note | `IconNote` | Pitch / note indicator |
| Filled circle | `IconCircle` | Color swatch proxy in menus |

### Icon Rules
1. **Never use raw Unicode** (`▶`, `▼`, `✕`, `✓`, `≡`, `+`) where an `IconID` exists.
2. All icons are drawn white-on-transparent via `iconSprite` and tinted at call-site via `ColorScale` or `IconColor`.
3. Icon inset: ~20% (`dim/5`) — already built into all `draw*Icon` functions.
4. Stroke weight: `iconStroke(r)` = `dim/8`, floor 2px — shared across the whole set.
5. Minimum icon button size: `BtnHeightSM` (28px) desktop, `TouchMinTargetPx` (44px) mobile.

---

## 6. Panel & Overlay Rules

### Floating Panel (popup / inspector)
```
drawPanel(dst, r)  // rounded corners (RadiusLG desktop, RadiusXL mobile) + shadow + border
```
- Header: `colPanelBG` background, title in `colTextPrimary` at 1.3× scale
- Close button: top-right corner, `closeButtonRect(panelRect, pad)`, icon `IconClose`
- Section toggles: `IconChevronDown` (open) / `IconChevronUp` (closed) — never `>` / `v` text
- Sections expand vertically; collapsed section shows badge pill (value summary)

### Bottom Sheet (mobile)
```
drawBottomSheetPanel(dst, r)  // top-only rounded corners (RadiusXL)
```
- Drag handle: centered pill at top of sheet
- Title bar: row name in `colTextPrimary`, close `IconClose` top-right

### Context Menu
- Desktop: floating `drawPanel`, ~180px wide, items stacked vertically
- Mobile: full-width `drawBottomSheetPanel`
- Each item: 40px tall (desktop), 48px tall (mobile)
- Leading icon (20×20px): left-aligned, `colTextSecondary` tint
- Label: `colTextPrimary`, single line
- Destructive item ("Delete"): `colDeleteFill` background, `colStopRed` text, `IconTrash` icon
- Separator between groups: 1px `colBorderSubtle`

### EQ Panel Tabs
Both the left filter tabs (`Master`/`HP`/`LP`) and the right view tabs (`EQ`/`Wave`/`Spectrum`/`Meters`/`Scope`) must use the **same visual affordance**: pill-shaped buttons with `InstButtonStyle`, active state highlighted with `colAccent` border.

### Inspector Section Toggles
Use the drawn triangle chevron (`drawFilledTriangleDown` / `drawFilledTriangleRight`) already in `game_node_sidebar.go`. Do **not** use `IconChevronUp`/`IconChevronDown` in the sidebar — the filled triangle is intentional. The sidebar is correct; enforce this pattern everywhere else.

---

## 7. UIStyle Configuration Layer

`ui_style.go` provides builder structs for one-call widget construction. Use these instead of inline `ButtonStyle{Fill:…, Border:…}` literals.

```go
// Create a row control button with icon:
btn := UIStyle.RowControl(IconMute).Button()
btn.OnClick = onMute

// Create a panel spec:
spec := PanelSpec{Title: "FX: Kick-deep", Width: 260}
r := spec.Draw(dst, anchorPt)

// Create a menu:
items := []MenuItemSpec{
    {Label: "Instrument", Icon: IconNote},
    {Label: "Rename",     Icon: IconPencil},
    {Label: "Color",      Icon: IconCircle},
    {Label: "Mute",       Icon: IconMute},
    {Label: "Solo",       Icon: IconSolo},
    {Label: "Effects",    Icon: IconFx},
    {Label: "Origin",     Icon: IconTarget},
    {Label: "Delete",     Icon: IconTrash, Destructive: true},
}
MenuSpec{Items: items}.Draw(dst, anchorPt)
```

---

## 8. Mobile vs. Desktop Intentional Differences

These differences are **intentional** and must be preserved:

| Aspect | Desktop | Mobile |
|---|---|---|
| Row height | `desktopRowHeightPx` (40px) | `touchRowHeightPx` (56px) |
| Button depth | `DrawTopEdgeHighlight = true` | `DrawTopEdgeHighlight = false` |
| Splitter color | `colSplitterHandle` (gray pill) | `colSplitterHandleMobile` (cyan pill) |
| Splitter grip | Yes (3 tick marks) | No |
| Context menu | Floating panel | Full-width bottom sheet |
| Row controls | Mute/Solo/FX/Origin/Delete visible | Context menu only (via overflow button) |
| Accent stripe | 3px, no inset | 5px, 2px vertical inset |
| Popup text | 1.0× scale | 1.3–1.7× scale |

All other visual differences are **accidental** and should be unified.

---

## 9. Screenshot Infrastructure

```bash
# Single shot (desktop native + browser desktop + browser mobile)
make screenshot               # → screenshots/{desktop,browser_desktop,browser_mobile}.png

# All UI scenes
make screenshots-all          # → screenshots/all/{desktop,mobile}/<scene>.png
make screenshots-all SCENES=transport_idle,context_menu_open   # filtered
make screenshots-all MOBILE=1  # adds mobile viewport pass

# CLI flags for the binary
./beatmo -screenshot /tmp/out.png           # capture and exit
./beatmo -scene transport_idle -screenshot /tmp/out.png
./beatmo -list-scenes                       # list all scene names
```

Add `.superpowers/` to `.gitignore` to exclude brainstorm artifacts.
```

- [ ] **Step 2: Run a sanity check**

```bash
wc -l /home/ymolinar/Repos/beatmo/DESIGN.md
```
Expected: > 200 lines (document is substantive).

- [ ] **Step 3: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add DESIGN.md
git commit -m "docs: add unified UI DESIGN.md — color tokens, button taxonomy, icon map, panel rules"
```

---

## Task 3: Create `theme_tokens.go` — Semantic Token Layer

**Files:**
- Create: `src/go/internal/ui/theme_tokens.go`

This file exposes named semantic tokens so callers never reference raw `colXxx` vars from `theme.go` directly. It is the single bridge between the raw palette and the rest of the UI code.

- [ ] **Step 1: Write failing test**

Create `src/go/internal/ui/theme_tokens_test.go`:

```go
//go:build test

package ui

import (
	"image/color"
	"testing"
)

func TestTokenDestructiveColors(t *testing.T) {
	if TokenDeleteFill() == (color.RGBA{}) {
		t.Fatal("TokenDeleteFill must not be zero")
	}
	if TokenMuteActiveFill() == (color.RGBA{}) {
		t.Fatal("TokenMuteActiveFill must not be zero")
	}
	// The three reds must be distinct.
	stop := TokenStopRed()
	mute := TokenMuteActiveFill()
	del := TokenDeleteFill()
	if stop == mute || mute == del || stop == del {
		t.Fatal("stop/mute/delete reds must be three distinct colors")
	}
}

func TestTokenSurfaces(t *testing.T) {
	s1, s2, s3 := TokenSurface1(), TokenSurface2(), TokenSurface3()
	// Each level should be brighter than the one below.
	if s1.R >= s2.R || s2.R >= s3.R {
		t.Errorf("surface levels must increase in brightness: s1=%v s2=%v s3=%v", s1, s2, s3)
	}
}

func TestTokenAccentConsistency(t *testing.T) {
	acc := TokenAccent()
	if acc.R != 0 || acc.G != 200 || acc.B != 255 {
		t.Errorf("TokenAccent must be #00C8FF, got %v", acc)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestToken -v 2>&1 | tail -15
```
Expected: FAIL — `TokenDeleteFill`, `TokenAccent`, etc. undefined.

- [ ] **Step 3: Create `theme_tokens.go`**

```go
package ui

import "image/color"

// Semantic token accessors. Use these instead of raw colXxx variables so
// the palette can be changed in one place without grepping call sites.
//
// Rule: one token per semantic role. Never add a new token that duplicates
// an existing role — update the existing token's underlying value instead.

// ── Surfaces ──────────────────────────────────────────────────────────────

func TokenSurface1() color.RGBA { return colSurface1 }
func TokenSurface2() color.RGBA { return colSurface2 }
func TokenSurface3() color.RGBA { return colSurface3 }
func TokenPanelBG() color.RGBA  { return colPanelBG }
func TokenPanelBorder() color.NRGBA {
	r, g, b, a := colPanelBorder.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// ── Accent ─────────────────────────────────────────────────────────────────

func TokenAccent() color.RGBA       { return colAccent }
func TokenAccentBright() color.RGBA { return colAccentBright }
func TokenAccentDim() color.RGBA    { return colAccentDim }

// ── Text ───────────────────────────────────────────────────────────────────

func TokenTextPrimary() color.RGBA   { return colTextPrimary }
func TokenTextSecondary() color.RGBA { return colTextSecondary }
func TokenTextDisabled() color.RGBA  { return colTextDisabled }
func TokenTextAccent() color.RGBA    { return colTextAccent }

// ── Semantic state ─────────────────────────────────────────────────────────

// TokenPlayGreen is the icon tint for the play button in active/playing state.
func TokenPlayGreen() color.RGBA { return colPlayGreen }

// TokenStopRed is the icon tint for the stop button and error text.
// Never use as a button fill — use TokenDeleteFill for destructive fills.
func TokenStopRed() color.RGBA { return colStopRed }

// TokenMuteActiveFill is the fill color for an active mute button.
// Distinct from TokenStopRed (stop) and TokenDeleteFill (destructive).
func TokenMuteActiveFill() color.RGBA { return colMuteActive }

// TokenDeleteFill is the fill for destructive action buttons (delete confirm, etc.).
func TokenDeleteFill() color.RGBA { return colDeleteFill }

// TokenSoloActiveFill is the fill for an active solo button.
func TokenSoloActiveFill() color.RGBA { return colSoloActive }

// ── Borders ─────────────────────────────────────────────────────────────────

func TokenBorderSubtle() color.NRGBA {
	r, g, b, a := colBorderSubtle.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}
func TokenBorderMedium() color.NRGBA {
	r, g, b, a := colBorderMedium.RGBA()
	return color.NRGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestToken -v 2>&1 | tail -15
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/theme_tokens.go src/go/internal/ui/theme_tokens_test.go
git commit -m "feat(ui): add semantic token accessors in theme_tokens.go"
```

---

## Task 4: Create `ui_style.go` — Configuration Layer for Buttons, Panels, Menus

**Files:**
- Create: `src/go/internal/ui/ui_style.go`
- Create: `src/go/internal/ui/ui_style_test.go`

This is the one-call styling layer described in DESIGN.md §7.

- [ ] **Step 1: Write failing tests**

Create `src/go/internal/ui/ui_style_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestRowControlButtonHasIcon(t *testing.T) {
	btn := RowControlButton(IconMute)
	if btn.Icon != string(IconMute) {
		t.Errorf("RowControlButton(IconMute) must set Icon field, got %q", btn.Icon)
	}
	if btn.Text != "" {
		t.Errorf("RowControlButton must not set Text, got %q", btn.Text)
	}
}

func TestDestructiveButtonHasDeleteStyle(t *testing.T) {
	btn := DestructiveButton()
	if btn.Icon != string(IconTrash) {
		t.Errorf("DestructiveButton must use IconTrash, got %q", btn.Icon)
	}
	if btn.ConsumeOnPress != true {
		t.Error("DestructiveButton must set ConsumeOnPress = true")
	}
}

func TestMenuSpecItemCount(t *testing.T) {
	spec := MenuSpec{Items: []MenuItemSpec{
		{Label: "Foo", Icon: IconClose},
		{Label: "Bar", Icon: IconMute},
	}}
	if len(spec.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(spec.Items))
	}
}

func TestPanelSpecTitleNotEmpty(t *testing.T) {
	spec := PanelSpec{Title: "FX: Kick", Width: 200}
	if spec.Title == "" {
		t.Error("PanelSpec Title must not be empty")
	}
}

func TestRowControlButtonActiveStyle(t *testing.T) {
	btn := RowControlButton(IconMute)
	ActiveRowControl(btn, true)
	// After activation, style should be MuteActiveStyle
	wantFill := MuteActiveStyle.Fill
	if got, ok := btn.Style.(ButtonStyle); !ok || got.Fill != wantFill {
		t.Errorf("ActiveRowControl(mute, true) must apply MuteActiveStyle")
	}
}

func TestIconOnlyAddButton(t *testing.T) {
	btn := IconOnlyButton(IconPlus, InstButtonStyle)
	if btn.Text != "" {
		t.Errorf("IconOnlyButton must have empty text, got %q", btn.Text)
	}
	if btn.Icon != string(IconPlus) {
		t.Errorf("IconOnlyButton must set Icon, got %q", btn.Icon)
	}
}

// Dummy: just verify the rect-returning draw helper doesn't panic on empty image.
func TestPanelSpecDrawNoPanic(t *testing.T) {
	assertDefaultParityState(t)
	spec := PanelSpec{Title: "Test", Width: 100, MinHeight: 80}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PanelSpec.HeaderRect panicked: %v", r)
		}
	}()
	_ = spec.HeaderRect(image.Pt(10, 10))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run "TestRowControl|TestDestructive|TestMenuSpec|TestPanelSpec|TestIconOnly" -v 2>&1 | tail -20
```
Expected: FAIL — `RowControlButton`, `DestructiveButton`, `MenuSpec`, `PanelSpec`, `IconOnlyButton`, `ActiveRowControl` undefined.

- [ ] **Step 3: Create `ui_style.go`**

```go
package ui

import "image"

// ui_style.go — One-call constructors for the four button categories
// (Primary, Secondary, RowControl, Destructive) and the two overlay
// specs (PanelSpec, MenuSpec). Follow DESIGN.md §4-7.

// ── Button constructors ───────────────────────────────────────────────────

// IconOnlyButton creates a Button with no text label, only an icon.
// Use for any slot where DESIGN.md specifies icon-mandatory.
func IconOnlyButton(id IconID, style ButtonVisual) *Button {
	b := NewButton("", style, nil)
	b.Icon = string(id)
	b.IconColor = colTextSecondary
	return b
}

// RowControlButton creates a row-control button (mute/solo/fx/origin/delete)
// using InstButtonStyle and the given icon. The caller wires OnClick.
func RowControlButton(id IconID) *Button {
	b := IconOnlyButton(id, InstButtonStyle)
	b.IconColor = colTextSecondary
	return b
}

// ActiveRowControl updates a row-control button's style to reflect its active
// state. Call on every frame (or when state changes) before drawing.
// id must match the icon passed to RowControlButton.
func ActiveRowControl(b *Button, active bool) {
	if b == nil {
		return
	}
	if !active {
		b.Style = InstButtonStyle
		b.IconColor = colTextSecondary
		return
	}
	switch IconID(b.Icon) {
	case IconMute:
		b.Style = MuteActiveStyle
		b.IconColor = colTextPrimary
	case IconSolo:
		b.Style = SoloActiveStyle
		b.IconColor = colAccentBright
	case IconFx:
		b.Style = FXActiveStyle
		b.IconColor = colAccent
	case IconTarget:
		b.Style = ButtonStyle{Fill: colAccentSubtle, Border: colAccent}
		b.IconColor = colAccent
	default:
		b.Style = InstButtonStyle
		b.IconColor = colTextPrimary
	}
}

// DestructiveButton creates a delete-row button: red fill, trash icon,
// ConsumeOnPress = true so rapid clicks don't cascade.
func DestructiveButton() *Button {
	b := NewButton("", DeleteButtonStyle, nil)
	b.Icon = string(IconTrash)
	b.IconColor = colStopRed
	b.ConsumeOnPress = true
	return b
}

// AddButton creates an icon-only "+" button using IconPlus and InstButtonStyle.
// Use for "Add row", "Add Effect", etc. — never the raw "+" character.
func AddButton() *Button {
	return IconOnlyButton(IconPlus, InstButtonStyle)
}

// CloseButton creates an icon-only close (×) button.
// Pass into panel headers; the caller sets the OnClick handler.
func CloseButton() *Button {
	b := IconOnlyButton(IconClose, TransportMiscStyle)
	b.IconColor = colTextSecondary
	return b
}

// ── Panel spec ────────────────────────────────────────────────────────────

// PanelSpec describes an overlay panel. Call HeaderRect to get layout
// coordinates for your own drawing; use drawPanel for the chrome.
//
// Panels follow DESIGN.md §6: rounded corners, shadow, border, title bar.
type PanelSpec struct {
	Title     string
	Width     int
	MinHeight int
}

// HeaderRect returns the title-bar rect for a panel anchored at origin.
// Height is `Profile().PopupBtnH + 2*SpaceSM`.
func (s PanelSpec) HeaderRect(origin image.Point) image.Rectangle {
	h := Profile().PopupBtnH + 2*SpaceSM
	return image.Rect(origin.X, origin.Y, origin.X+s.Width, origin.Y+h)
}

// ContentRect returns the area below the header for the panel body.
func (s PanelSpec) ContentRect(origin image.Point, bodyHeight int) image.Rectangle {
	hdr := s.HeaderRect(origin)
	return image.Rect(hdr.Min.X, hdr.Max.Y, hdr.Min.X+s.Width, hdr.Max.Y+bodyHeight)
}

// ── Menu spec ─────────────────────────────────────────────────────────────

// MenuItemSpec describes one item in a context menu.
type MenuItemSpec struct {
	Label       string
	Icon        IconID
	Destructive bool // true → red background (Delete-style)
	OnClick     func()
}

// MenuSpec describes a context menu (desktop floating or mobile bottom sheet).
// Call ItemHeight() to compute total panel height.
type MenuSpec struct {
	Title string // optional — shown as header on mobile bottom sheet
	Items []MenuItemSpec
}

// ItemHeight returns the per-item height for the current profile.
func (s MenuSpec) ItemHeight() int {
	if Profile().IsMobile() {
		return BtnHeightLG + SpaceMD // 48px touch target
	}
	return BtnHeightMD + SpaceSM // 40px desktop
}

// TotalHeight returns the full panel height: header (if title set) + all items.
func (s MenuSpec) TotalHeight() int {
	itemsH := len(s.Items) * s.ItemHeight()
	if s.Title != "" {
		return Profile().PopupBtnH + itemsH
	}
	return itemsH
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run "TestRowControl|TestDestructive|TestMenuSpec|TestPanelSpec|TestIconOnly" -v 2>&1 | tail -20
```
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/ui_style.go src/go/internal/ui/ui_style_test.go
git commit -m "feat(ui): add UIStyle layer — RowControlButton, PanelSpec, MenuSpec, DestructiveButton"
```

---

## Task 5: Apply Icons to Row Rack Buttons (M → IconMute, S → IconSolo, FX → IconFx, O → IconTarget, X → IconTrash, + → IconPlus)

**Files:**
- Modify: `src/go/internal/ui/row_rack_zone.go`

This replaces the 6 text-label button definitions identified in the audit.

- [ ] **Step 1: Write a failing test that verifies icon usage**

Add to `src/go/internal/ui/ui_style_test.go` (append; do not replace):

```go
func TestRowRackButtonsUseIcons(t *testing.T) {
	assertDefaultParityState(t)
	g := newTestGame(t)
	rows := g.drum.Rows()
	if len(rows) == 0 {
		t.Skip("no rows")
	}
	// Grab the first row's mute/solo/fx buttons via the RowButtonGroup.
	rbg := g.drum.RowButtonGroupAt(0)
	if rbg.Mute == nil {
		t.Skip("mute button nil (may be hidden in this layout)")
	}
	if rbg.Mute.Text != "" {
		t.Errorf("mute button must use icon, not text %q", rbg.Mute.Text)
	}
	if rbg.Mute.Icon == "" {
		t.Errorf("mute button must have Icon set")
	}
	if rbg.Solo.Text != "" {
		t.Errorf("solo button must use icon, not text %q", rbg.Solo.Text)
	}
	if rbg.FX.Text != "" {
		t.Errorf("fx button must use icon, not text %q", rbg.FX.Text)
	}
}
```

- [ ] **Step 2: Confirm test fails**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestRowRackButtonsUseIcons -v 2>&1 | tail -15
```
Expected: FAIL (`mute button must use icon, not text "M"`).

- [ ] **Step 3: Edit `row_rack_zone.go` — replace text buttons with icon constructors**

In `row_rack_zone.go`, find the block starting with `mute := NewButton("M", InstButtonStyle, nil)` (around line 507) and replace the six button creations:

```go
// BEFORE:
mute := NewButton("M", InstButtonStyle, nil)
// ...
solo := NewButton("S", InstButtonStyle, nil)
// ...
origin := NewButton("O", InstButtonStyle, nil)
// ...
del := NewButton("X", InstButtonStyle, nil)
// ...
fx := NewButton("FX", InstButtonStyle, nil)
```

```go
// AFTER:
mute := RowControlButton(IconMute)
mute.OnClick = func() {
    if z.callbacks.OnMuteToggle != nil {
        z.callbacks.OnMuteToggle(idx)
    }
}

solo := RowControlButton(IconSolo)
solo.OnClick = func() {
    if z.callbacks.OnSoloToggle != nil {
        z.callbacks.OnSoloToggle(idx)
    }
}

origin := RowControlButton(IconTarget)
origin.OnClick = func() {
    if z.callbacks.OnOriginReq != nil {
        z.callbacks.OnOriginReq(idx)
    }
}

del := DestructiveButton()
if len(rows) > 1 {
    del.OnClick = func() {
        if z.callbacks.OnDeleteRow != nil {
            z.callbacks.OnDeleteRow(idx)
        }
        z.MarkDirty()
    }
} else {
    del.Style = DisabledButtonStyle
}

fx := RowControlButton(IconFx)
fx.OnClick = func() {
    if z.callbacks.OnFXPanelToggle != nil {
        z.callbacks.OnFXPanelToggle(idx)
    }
}
```

Also replace the `+` add-row button (~line 721):
```go
// BEFORE:
z.addRowBtn.Text = "+"

// AFTER:
z.addRowBtn.Icon = string(IconPlus)
z.addRowBtn.Text = ""
```

And the delete-row text assignments (~lines 1157–1160 and 1245–1248):
```go
// BEFORE:
e.deleteBtn.Text = "!!"
// ...
e.deleteBtn.Text = "X"

// AFTER:
e.deleteBtn.Icon = string(IconTrash)
e.deleteBtn.Text = ""
```

- [ ] **Step 4: Update active-state styling in the draw loop**

In `row_rack_zone.go`, find the block (~line 1129) where mute/solo/fx styles are set. Replace the direct `Style` assignments with `ActiveRowControl` calls:

```go
// BEFORE:
if !e.muteBtn.Rect().Empty() {
    e.muteBtn.pressed = rows[i].Muted
    if rows[i].Muted {
        e.muteBtn.Style = MuteActiveStyle
    } else {
        e.muteBtn.Style = InstButtonStyle
    }
    drawBtnOff(cache, e.muteBtn, offsetX, offsetY)
}
if !e.soloBtn.Rect().Empty() {
    e.soloBtn.pressed = rows[i].Solo
    if rows[i].Solo {
        e.soloBtn.Style = SoloActiveStyle
    } else {
        e.soloBtn.Style = InstButtonStyle
    }
    drawBtnOff(cache, e.soloBtn, offsetX, offsetY)
}
if !e.fxBtn.Rect().Empty() {
    if fxActive {
        e.fxBtn.Style = FXActiveStyle
    } else {
        e.fxBtn.Style = InstButtonStyle
    }
    drawBtnOff(cache, e.fxBtn, offsetX, offsetY)
}

// AFTER:
if !e.muteBtn.Rect().Empty() {
    e.muteBtn.pressed = rows[i].Muted
    ActiveRowControl(e.muteBtn, rows[i].Muted)
    drawBtnOff(cache, e.muteBtn, offsetX, offsetY)
}
if !e.soloBtn.Rect().Empty() {
    e.soloBtn.pressed = rows[i].Solo
    ActiveRowControl(e.soloBtn, rows[i].Solo)
    drawBtnOff(cache, e.soloBtn, offsetX, offsetY)
}
if !e.fxBtn.Rect().Empty() {
    ActiveRowControl(e.fxBtn, fxActive)
    drawBtnOff(cache, e.fxBtn, offsetX, offsetY)
}
```

- [ ] **Step 5: Run the test and all existing tests**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestRowRack -v 2>&1 | tail -15
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... 2>&1 | tail -20
```
Expected: TestRowRackButtonsUseIcons PASS; full suite green.

- [ ] **Step 6: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/row_rack_zone.go src/go/internal/ui/ui_style_test.go
git commit -m "feat(ui): replace text labels M/S/FX/O/X/+ with icon buttons in row rack"
```

---

## Task 6: Apply Icons to FX Panel (▶/▼ chevrons, ✕ close, + Add Effect)

**Files:**
- Modify: `src/go/internal/ui/drumview_fx_panel.go`

- [ ] **Step 1: Write failing test**

Add to `src/go/internal/ui/ui_style_test.go`:

```go
func TestFXPanelButtonsUseIcons(t *testing.T) {
	assertDefaultParityState(t)
	// Verify CloseButton() and AddButton() from ui_style.go produce icon-only buttons.
	close := CloseButton()
	if close.Text != "" {
		t.Errorf("CloseButton must have no text, got %q", close.Text)
	}
	if close.Icon != string(IconClose) {
		t.Errorf("CloseButton must use IconClose, got %q", close.Icon)
	}
	add := AddButton()
	if add.Text != "" {
		t.Errorf("AddButton must have no text, got %q", add.Text)
	}
	if add.Icon != string(IconPlus) {
		t.Errorf("AddButton must use IconPlus, got %q", add.Icon)
	}
}
```

- [ ] **Step 2: Confirm test passes (CloseButton/AddButton are already in ui_style.go)**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run TestFXPanelButtonsUseIcons -v 2>&1 | tail -10
```
Expected: PASS (testing the factory functions, not the panel usage).

- [ ] **Step 3: Find and replace in `drumview_fx_panel.go`**

Find the chevron expand button (~line 393):
```go
// BEFORE:
chevron := "▶"
if expanded {
    chevron = "▼"
}
expBtn := NewButton(chevron, InstButtonStyle, nil)
```

```go
// AFTER:
iconID := IconChevronDown
if !expanded {
    iconID = IconChevronUp
}
expBtn := IconOnlyButton(iconID, InstButtonStyle)
```

Find the remove button (~line 440):
```go
// BEFORE:
remove := NewButton("✕", InstButtonStyle, nil)
```

```go
// AFTER:
remove := CloseButton()
```

Find the reorder button (~line 427):
```go
// BEFORE:
down := NewButton("▼", InstButtonStyle, nil)
```

```go
// AFTER:
down := IconOnlyButton(IconChevronDown, InstButtonStyle)
```

Find the "+ Add Effect" button. The label text can stay as it provides useful context in this panel (it is not a bare `+`), but replace any standalone `"+"` labels:

Search for `"+ Add Effect"` — if it's a `NewButton("+ Add Effect", …)`, update:
```go
// BEFORE (if plain "+" or "▶"/"▼" only):
addFX := NewButton("+ Add Effect", InstButtonStyle, nil)

// KEEP AS IS if it says "+ Add Effect" — the text is informative label.
// Only replace if it's bare "+":
addFX := NewButton("+", InstButtonStyle, nil)
// AFTER:
addFX := AddButton()
addFX.Text = "Add Effect"  // keep descriptive text alongside icon
```

- [ ] **Step 4: Build to confirm no compile errors**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/... 2>&1
```
Expected: no errors.

- [ ] **Step 5: Run full test suite**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... 2>&1 | tail -20
```
Expected: all green.

- [ ] **Step 6: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/drumview_fx_panel.go src/go/internal/ui/ui_style_test.go
git commit -m "feat(ui): replace Unicode chevron/close/plus chars in FX panel with icon buttons"
```

---

## Task 7: Verification — Take Screenshots and Confirm Visual Improvement

- [ ] **Step 1: Run fast Go test suite to confirm no regressions**

```bash
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... 2>&1 | tail -20
```
Expected: all green.

- [ ] **Step 2: Take new screenshots**

```bash
cd /home/ymolinar/Repos/beatmo
make screenshot
```
Expected: `screenshots/desktop.png`, `screenshots/browser_desktop.png`, `screenshots/browser_mobile.png` refreshed.

- [ ] **Step 3: Visual inspection checklist**

Open `screenshots/desktop.png` and verify:
- [ ] Row rack buttons show icons (mute = speaker+slash, solo = headphones, FX = sparkle, origin = crosshair, delete = trash bin)
- [ ] Add-row button shows `+` icon, no text
- [ ] No `M`, `S`, `FX`, `O`, `X` text labels visible in row controls

Open `screenshots/browser_desktop.png` and verify same checklist in browser build.

- [ ] **Step 4: Commit the updated screenshots**

```bash
cd /home/ymolinar/Repos/beatmo
git add screenshots/desktop.png screenshots/browser_desktop.png screenshots/browser_mobile.png
git commit -m "chore: update screenshots after icon-ification of row rack and FX panel"
```

---

## Self-Review

**Spec coverage:**
- §1 Color tokens → Task 3 `theme_tokens.go` ✓
- §2 Typography → documented in DESIGN.md, no code change needed (existing `TextHeight()` API is correct) ✓
- §3 Spacing tokens → already exist as constants; DESIGN.md documents them ✓
- §4 Button taxonomy → Task 4 `ui_style.go` ✓
- §5 Icon map → Tasks 5–6 apply icons, DESIGN.md documents rules ✓
- §6 Panel rules → `drawPanel`/`drawBottomSheetPanel`/`closeButtonRect` already correct; DESIGN.md codifies what's right ✓
- §7 UIStyle layer → Task 4 ✓
- §8 Mobile/desktop intentional delta → DESIGN.md only ✓
- §9 Screenshot infra → Task 1 CLAUDE.md + DESIGN.md §9 ✓

**Placeholder scan:** No TBDs or incomplete steps found.

**Type consistency:** `RowControlButton` returns `*Button`, `ActiveRowControl` takes `*Button` — consistent. `MenuSpec.ItemHeight()` / `TotalHeight()` use `Profile()` consistently. `PanelSpec.HeaderRect` / `ContentRect` both take `image.Point` — consistent.
