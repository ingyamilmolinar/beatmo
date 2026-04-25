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
| `colPanelBG` | `#181920` (250 alpha) | Overlay panels, popups, bottom sheets |

**Rule:** Nest surfaces in order — Surface1 contains Surface2 buttons. Never place a lower-level surface inside a higher-level container.

### Accent Colors

| Token | Value | Semantic |
|---|---|---|
| `colAccent` | `#00C8FF` | Primary interactive / active state |
| `colAccentBright` | `#50DCFF` | Hover / focus ring |
| `colAccentDim` | `#008CC8` | Splitter, secondary emphasis |
| `colAccentSubtle` | `#00C8FF` (8% opacity) | Backgrounds, fills |

**Rule:** Use `colAccent` for active/selected state. Use `colAccentBright` only for hover. Never use accent for decorative purposes.

### Semantic State Colors

| Token | Value | Role | Usage |
|---|---|---|---|
| `colPlayGreen` | `#3CD278` | Positive / running | Play icon tint (mobile), play-active indicator |
| `colStopRed` | `#DC4646` | Stop / error text | Stop icon tint (mobile), error text — **never a button fill** |
| `colStopButton` | `#B43737` | Stop button fill | Desktop stop button background |
| `colMuteRed` / `colMuteActive` | `#A03C32` | Muted channel | Mute button active fill only |
| `colDeleteFill` | `#782323` | Destructive action fill | Delete button background only |
| `colDeleteBorder` | `#B43737` | Destructive border | Delete button border |
| `colRecordIdle` | `#C83232` (80% alpha) | Record ready | Record icon tint |
| `colRecordActive` | `#F02828` (80% alpha) | Recording | Record icon tint while active |

**Rule — three reds, three distinct roles:**
- `colStopRed` → stop/error **text** only
- `colMuteActive` → mute button **fill** only
- `colDeleteFill` → destructive confirm button **fill** only

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
| `colBorderSubtle` | white / 8 | Section dividers |
| `colBorderMedium` | white / 15 | Control borders (default) |
| `colBorderStrong` | white / 25 | Focused input borders |
| `colPanelBorder` | white / 20 | Overlay panel borders |

---

## 2. Typography Scale

All text uses the bundled debug font (6×16 px cell per glyph) scaled via `TextScale`. Use these scales consistently — never hardcode a pixel size.

| Role | Scale | Usage |
|---|---|---|
| Panel title | 1.3× | Inspector / FX panel header |
| Section header | 1.0× | Collapsible section labels |
| Button label | 1.0× | Row controls, transport |
| Caption / value | 1.0× | BPM number, percentage values |
| Tooltip | 0.8× | Cursor labels, coordinates (desktop only) |

**Rule:** Derive all sizes from `TextHeight()` / `TextWidth()` so the font can be swapped later without hunting for hardcoded constants.

---

## 3. Spacing & Radius Tokens

All spacing and radii are package-level constants. Use these — never magic numbers.

| Token | Value | Usage |
|---|---|---|
| `SpaceSM` | 4 px | Tight gaps (button padding, icon inset) |
| `SpaceMD` | 8 px | Normal gaps (section spacing) |
| `SpaceLG` | 12 px | Loose gaps (panel padding) |
| `RadiusSM` | 4 px | Small corners (chips, pills) |
| `RadiusMD` | 8 px | Medium corners (popups) |
| `RadiusLG` | 12 px | Large corners (bottom sheets desktop) |
| `RadiusXL` | 16 px | Extra-large (mobile bottom sheets) |
| `BtnHeightSM` | 28 px | Small buttons (close, stepper) |
| `BtnHeightMD` | 32 px | Medium buttons (row controls) |
| `BtnHeightLG` | 40 px | Large buttons (transport, FAB) |
| `TouchMinTargetPx` | 44 px | Minimum hit target on mobile |

---

## 4. Button Taxonomy

Every button falls into one of five categories. Use the matching `ButtonStyle` from `theme.go`.

### 4a. Primary (action)
- **Style:** `FABStyle` — `colAccent` fill, white border
- **Use:** Floating action buttons (mobile "+"), primary CTA
- **Icon:** mandatory

### 4b. Secondary (neutral transport / control)
- **Style:** `TransportPlayStyle` / `TransportMiscStyle` — `colSurface2` fill, `colBorderSubtle` border
- **Use:** Play, Stop, Record, BPM +/−, subdiv, view-switch
- **Icon:** mandatory; text only if icon is genuinely insufficient
- Desktop: `DrawTopEdgeHighlight = true` adds subtle 3D depth
- Mobile: flat (`DrawTopEdgeHighlight = false`)

### 4c. Row control (inline)
- **Style:** `InstButtonStyle` — `colSurface2` fill, `colBorderSubtle` border
- **Use:** Mute, Solo, FX, Origin (target), Delete in the row rack
- **Icon:** **always icon — never a text label** (see §5 Icon Map)
- Active states: `MuteActiveStyle`, `SoloActiveStyle`, `FXActiveStyle`

### 4d. Destructive (delete / confirm)
- **Style:** `DeleteButtonStyle` — `colDeleteFill` fill, `colDeleteBorder` border
- **Use:** Delete row, "Delete" context menu item, confirm dialogs
- **Icon:** `IconTrash` for delete buttons; text label acceptable in confirm dialogs
- **Text color:** `colStopRed`

### 4e. Disabled
- **Style:** `DisabledButtonStyle` — `{50,52,58}` fill, `colBorderSubtle` border
- **Use:** Any button that cannot activate in the current state

### Button Hover / Press States

`ButtonStyle.Draw` handles hover (+12 brightness) and press (−20 brightness) via `adjustColor` automatically. No additional code required.

### Creating a New Button

```go
// Row control with icon — never use text for these:
btn := RowControlButton(IconMute)
btn.OnClick = onMute

// Destructive:
del := DestructiveButton()
del.OnClick = onDelete

// Icon-only generic:
btn := IconOnlyButton(IconPlus, InstButtonStyle)
```

---

## 5. Icon Map — Required Usage

The icon set has 28 glyphs defined in `icons.go`. Every icon has **exactly one** semantic role. Use `DrawIcon(dst, id, r, col)` or set `Button.Icon = string(IconID)`.

### 5a. Transport

| Icon | `IconID` | Usage |
|---|---|---|
| Play triangle | `IconPlay` | Play button |
| Pause bars | `IconPause` | Pause state |
| Stop square | `IconStop` | Stop button |
| Record dot+ring | `IconRecord` | Record button |

### 5b. Row Controls (icon mandatory — no text alternatives)

| Icon | `IconID` | Inactive tint | Active style |
|---|---|---|---|
| Speaker+strikethrough | `IconMute` | `colTextSecondary` | `MuteActiveStyle`, `colTextPrimary` |
| Headphones | `IconSolo` | `colTextSecondary` | `SoloActiveStyle`, `colAccentBright` |
| 4-point sparkle | `IconFx` | `colTextSecondary` | `FXActiveStyle`, `colAccent` |
| Crosshair | `IconTarget` | `colTextSecondary` | accent subtle bg, `colAccent` |
| Trash bin | `IconTrash` | `colStopRed` | `DeleteButtonStyle` bg |

### 5c. Navigation / Control

| Icon | `IconID` | Usage |
|---|---|---|
| 3 dots vertical | `IconOverflow` | Context menu trigger |
| + cross | `IconPlus` | Add row, Add Effect — **never raw `+` character** |
| − bar | `IconMinus` | Remove item |
| Chevron up | `IconChevronUp` | Increment, collapsed→open — **never `^` or `>` text** |
| Chevron down | `IconChevronDown` | Decrement, open→collapsed — **never `v` text** |
| × diagonals | `IconClose` | Close any panel/popup — **never `X`, `✕`, or `×` text** |

### 5d. File Operations

| Icon | `IconID` | Usage |
|---|---|---|
| Up-arrow + dashed bar | `IconUpload` | Upload / share |
| Down-arrow + tray | `IconImport` | Import JSON |
| Up-arrow + tray | `IconExport` | Export JSON |
| Floppy disk | `IconSave` | Save |

### 5e. View / State

| Icon | `IconID` | Usage |
|---|---|---|
| 3 horizontal lines | `IconRows` | Switch to row view |
| EQ bars | `IconAudio` | Switch to EQ/audio view |
| Closed padlock | `IconTrack` | Follow-on (locked scroll) |
| Open padlock | `IconTrackOff` | Follow-off (free scroll) |
| Speaker+waves | `IconSpeaker` | Volume on / audible |
| Speaker body only | `IconSpeakerOff` | Volume zero / muted |
| Musical note | `IconNote` | Pitch / note indicator |
| Filled circle | `IconCircle` | Color swatch proxy in menus |
| Pencil | `IconPencil` | Rename / edit |

### Icon Rules

1. **Never use raw Unicode** (`▶`, `▼`, `✕`, `✓`, `≡`, `+`) where an `IconID` exists.
2. Icons are drawn white-on-transparent via `iconSprite` and tinted at call-site via `IconColor`.
3. Icon inset: ~20% (`dim/5`) — built into all `draw*Icon` functions.
4. Stroke weight: `iconStroke(r)` = `dim/8`, floor 2 px — shared across the whole set.
5. Minimum icon button: `BtnHeightSM` (28 px) desktop, `TouchMinTargetPx` (44 px) mobile.

---

## 6. Panel & Overlay Rules

### Floating Panel (popup / inspector)

```go
drawPanel(dst, r)  // rounded corners + shadow + border
```

- **Header:** `colPanelBG` background, title `colTextPrimary` at 1.3× scale
- **Close button:** top-right via `closeButtonRect(panelRect, pad)`, icon `IconClose`
- **Section toggles:** filled triangle chevron (`drawFilledTriangleDown`/`Right` in sidebar) — **never `>` or `v` text**
- Collapsed sections show a badge pill (value summary, right-aligned)

### Bottom Sheet (mobile)

```go
drawBottomSheetPanel(dst, r)  // top-only rounded corners (RadiusXL)
```

- Drag handle: centered pill at top
- Title bar: row name `colTextPrimary`, close `IconClose` top-right

### Context Menu

- **Desktop:** floating `drawPanel`, ~180 px wide, items stacked vertically
- **Mobile:** full-width `drawBottomSheetPanel`
- Item height: 40 px desktop, 48 px mobile
- Leading icon (20×20 px): `colTextSecondary` tint
- Label: `colTextPrimary`, single line, no truncation
- Destructive item ("Delete"): `colDeleteFill` bg, `colStopRed` text, `IconTrash`
- Group separator: 1 px `colBorderSubtle`

### EQ Panel Tabs

Both the left filter tabs (`Master`/`HP`/`LP`) and the right view tabs (`EQ`/`Wave`/`Spectrum`/`Meters`/`Scope`) must use the **same visual affordance**: pill-shaped buttons with `InstButtonStyle`, active state `colAccent` border.

### Inspector Section Toggles

Use the filled triangle chevron already in `game_node_sidebar.go` (`drawFilledTriangleDown`/`drawFilledTriangleRight`). This is correct and intentional; enforce this pattern everywhere else. Do **not** use raw text `>` / `v` / `▶` / `▼` for collapsibles.

---

## 7. UIStyle Configuration Layer

`ui_style.go` provides one-call constructors. Use these instead of inline `ButtonStyle{Fill:…, Border:…}` literals scattered across the codebase.

```go
// Row control button:
btn := RowControlButton(IconMute)
btn.OnClick = onMute

// Update active state (call each frame or on state change):
ActiveRowControl(btn, isMuted)

// Destructive:
del := DestructiveButton()

// Close button for panel header:
cls := CloseButton()

// Add / plus button:
add := AddButton()

// Generic icon-only:
btn := IconOnlyButton(IconChevronDown, InstButtonStyle)

// Panel geometry helpers:
spec := PanelSpec{Title: "FX: Kick-deep", Width: 260, MinHeight: 80}
hdrR  := spec.HeaderRect(anchor)
bodyR := spec.ContentRect(anchor, bodyH)

// Menu height calculation:
m := MenuSpec{Items: []MenuItemSpec{
    {Label: "Mute",   Icon: IconMute},
    {Label: "Delete", Icon: IconTrash, Destructive: true},
}}
totalH := m.TotalHeight()
itemH  := m.ItemHeight()
```

---

## 8. Mobile vs. Desktop: Intentional Differences

These differences are **intentional** and must be preserved:

| Aspect | Desktop | Mobile |
|---|---|---|
| Row height | 40 px (`desktopRowHeightPx`) | 56 px (`touchRowHeightPx`) |
| Button depth | `DrawTopEdgeHighlight = true` | `DrawTopEdgeHighlight = false` (flat) |
| Splitter handle color | `colSplitterHandle` (gray pill) | `colSplitterHandleMobile` (cyan pill) |
| Splitter grip marks | Yes (3 tick marks) | No |
| Context menu style | Floating panel | Full-width bottom sheet |
| Row controls visible | Mute/Solo/FX/Origin/Delete | Overflow button only |
| Accent stripe | 3 px, flush | 5 px, 2 px vertical inset |
| Popup text scale | 1.0× | 1.3–1.7× |
| Timeline default | No cap (full circuit) | 8 beats |

All other visual differences are **accidental** and should be unified.

---

## 9. Screenshot Infrastructure

```bash
# Single shot: desktop native + browser desktop + browser mobile
make screenshot               # → screenshots/{desktop,browser_desktop,browser_mobile}.png

# All UI scenes
make screenshots-all          # → screenshots/all/{desktop,mobile}/<scene>.png
make screenshots-all SCENES=transport_idle,context_menu_open   # filtered
make screenshots-all MOBILE=1  # adds mobile viewport pass

# Direct CLI flags on the binary
./beatmo -screenshot /tmp/out.png           # capture and exit
./beatmo -scene transport_idle -screenshot /tmp/scene.png
./beatmo -list-scenes                       # list all registered scene names
```

Add `.superpowers/` to `.gitignore` to keep brainstorm artifacts out of commits.

---

## 10. Adding New UI Elements — Checklist

Before adding any new button, panel, icon, or menu item:

- [ ] Does an `IconID` already exist for the semantic action? Use it.
- [ ] Which button category (§4) does this fall into? Use the matching `ButtonStyle`.
- [ ] Does a `ui_style.go` constructor already cover this? Use it instead of inline literals.
- [ ] Is this desktop-only or mobile-only? Document the intentional difference in §8.
- [ ] Does the new color (if any) map to an existing token? If not, add a token first.
- [ ] Run `make screenshots-all` and compare to baseline before merging.
