# UX Polish Redesign — Design Spec

## Context

Beatmo's desktop UI has grown organically and accumulated visual inconsistencies: cramped 24px row heights, tiny transport buttons, competing accent colors (cyan steps, yellow highlights, orange EQ, yellow dropdown borders), raw dB numbers instead of visual volume indicators, and cryptic single-letter controls. The Go bundled font renders poorly at the small sizes used throughout. This redesign unifies the visual language, improves control sizes and legibility, and establishes a cohesive 3-color accent system — all while maintaining the existing dark theme personality and minimizing rendering cost.

**Target audience:** Dual — music producers and casual hobbyists, leaning professional.
**Design direction:** Modern Balanced (Direction C) — Bitwig-inspired, clean surfaces with rounded corners, cyan primary accent.
**Scope:** Desktop layout only. Mobile layout (`Profile().IsMobile()`) is unaffected — it already has its own sizing/layout via `touchRowHeightPx`, mobile header, and context menu infrastructure. Color and font changes are shared (both platforms).

---

## 1. Unified Color Palette

### 3-Color Accent System

| Role | Color | Hex | Usage |
|------|-------|-----|-------|
| **Primary** | Cyan | `#00b9eb` | Active steps, EQ curve, subdivision border, active FX state, timeline view window |
| **Signal** | Yellow | `#ffdc28` | Playhead highlight, beat position cursor, timeline cursor |
| **Interactive** | Light Blue | `#6ab0f0` | BPM +/−, follow-on lock, clickable affordances |

### Semantic Colors (Unchanged)

| Role | Color | Hex |
|------|-------|-----|
| Play | Green | `#2a8a54` (desaturated from `#28c864`) |
| Stop | Red | `#9a3030` (desaturated from `#dc3c3c`) |
| Mute active | Orange-red | `#b4462e` fill, `#ffa080` text |
| Solo active | Gold | `#c8aa28` fill, `#ffe060` text |
| Delete | Red | `#c83838` fill, `#ff6060` text |
| Error | Red | `#c83c3c` |

### Removed Colors

- `colDropdownEdge` (yellow `#ffdc28` on subdivision dropdown) → replaced by cyan border `rgba(0,185,235,0.25)`
- `colEQCurve` (orange `rgba(255,180,50,200)`) → replaced by cyan `#00b9eb` at 90% opacity
- `colEQCurveFill` (orange) → replaced by cyan `rgba(0,185,235,0.08)`
- `colEQHandle` (orange) → replaced by cyan `#00b9eb`
- `colEQHandleActive` → replaced by bright cyan `#40d0ff`

### Updated Surface Colors

| Token | Current | New | Notes |
|-------|---------|-----|-------|
| `colTransportSurface` | `RGB(34,34,42)` | `RGB(24,24,32)` → `#181820` | Slightly darker for container effect |
| Play button fill | `RGB(40,200,100)` | `RGB(42,138,84)` → `#2a8a54` | Desaturated, more professional |
| Play button border | `RGB(80,240,140)` | `RGB(60,180,110)` → `#3cb46e` | Matching desaturation |
| Stop button fill | `RGB(220,60,60)` | `RGB(154,48,48)` → `#9a3030` | Desaturated |
| Stop button border | `RGB(255,100,100)` | `RGB(190,70,70)` → `#be4646` | Matching |
| Inc/Dec fill | `RGB(60,120,200)` | `RGB(42,58,90)` → `#2a3a5a` | Darker, subtler background |
| Inc/Dec icon | `RGB(240,240,255)` | `RGB(106,176,240)` → `#6ab0f0` | Blue instead of white |
| File op buttons | `colDropdown` | `RGB(34,34,48)` → `#222230` | Uniform secondary button |
| Row control background | `colRackSurface` | Keep `RGB(22,22,28)` | Unchanged |

---

## 2. Typography

### Font

Replace Go's bundled font with **Inter** (or IBM Plex Sans as fallback). Both are SIL Open Font License, render crisply at 10-14px, and have tabular number figures as the default numeric spacing.

**Simplification: 2 weights only.** The existing `fontcache.go` uses a `bold bool` key — Regular (400) and Bold (700). Rather than restructuring the entire font pipeline, keep this 2-weight system but swap the font files:

- `Inter-Regular.ttf` (~100KB) → replaces `GoRegular.ttf` as `assets.GoRegularTTF`
- `Inter-SemiBold.ttf` (~100KB) → replaces `GoBold.ttf` as `assets.GoBoldTTF`

The face cache key (`faceCacheKey`) and `renderTextSprite(s, size, bold)` stay unchanged. "Bold" in the code maps to Inter SemiBold (600) visually.

**Font embed changes** (`src/go/internal/assets/font_embed.go`):
- Replace `fonts/GoRegular.ttf` with `fonts/Inter-Regular.ttf`
- Replace `fonts/GoBold.ttf` with `fonts/Inter-SemiBold.ttf`
- Variable names (`GoRegularTTF`, `GoBoldTTF`) stay the same to avoid cascading renames

### Size Scale

Map the new visual scale to the existing `FontSize*` constants in `textcache.go`:

| New Size | Existing Constant | Current Value | New Value | Usage |
|----------|-------------------|---------------|-----------|-------|
| 10px | `FontSizeCaption` | 11 | **10** | Micro labels: BPM suffix, dB values, frequency labels |
| 12px | (new) `FontSizeSmall` | N/A | **12** | Row labels, button text, popup content |
| 14px | `FontSizeBody` | 16 | **14** | Emphasis: BPM value, subdivision number |
| 16px | `FontSizeLabel` | 18 | **16** | Headings: overlay titles, popup titles |
| 18px | `FontSizeTitle` | 21 | **18** | Large headings (rare) |
| 20px | `FontSizeHeading` | 24 | **20** | Extra-large headings (rare) |

**Note:** `FontSizeBody` drops from 16→14. The test `TestFontSizeBodyAdequateForDesktopRow` checks `FontSizeBody * 1.2 <= desktopRowHeightPx`. With 14×1.2=16.8 ≤ 28, this passes. The test `TestFontSizeHierarchy` checks strict ordering (Caption < Body < Label < Title < Heading). With the new values 10 < 12 < 14 < 16 < 18 < 20, this passes (the new `FontSizeSmall` needs to be added to the hierarchy check).

The default `textSpriteRenderer` and `textMeasureWidth` use `FontSizeBody` (the init in `fontcache.go`). Callers that need other sizes use `renderTextSprite(s, FontSizeCaption, false)` etc. directly.

### Weight Usage

| Code | Font Face | Visual Weight | Usage |
|------|-----------|---------------|-------|
| `bold: false` | Inter Regular (400) | Normal | Secondary labels, dB values, descriptions, row labels |
| `bold: true` | Inter SemiBold (600) | Emphasis | BPM value, active button states, button text |

### Tabular Numbers

Inter defaults to tabular (fixed-width) number glyphs. No OpenType feature flag needed — Go's `opentype.NewFace` renders them correctly out of the box. This prevents layout shifts in BPM display, dB values, beat counters, and volume percentages.

---

## 3. Transport Controls

### Layout: Single Row

Collapse 2 toolbar rows into 1 horizontal strip inside a contained surface (radius 8px, border `rgba(255,255,255,0.06)`).

**Header height change:** `desktopHeaderH`: 72 → **50px**. Single row with 34px buttons + 8px top/bottom padding = 50px. This cascades to: `game_layout.go` (split calculation), `drumview_geometry.go` (header bounds), `drumview.go` (constant definition), and the `TransportZone` in `transport_zone.go` which builds the layout grid. The `outerGrid` in `drumview_layout.go` changes from 2 rows (`[]float64{1, 1}`) to 1 row (`[]float64{1}`).

**Group order (left to right):**
1. **Playback**: Play (36×34px, radius 7px) | Stop (36×34px, radius 7px)
2. **Divider** (1px × 26px, `rgba(255,255,255,0.08)`)
3. **Tempo**: BPM input (min-width 40px, radius 5px, 15px font semibold) | "BPM" suffix label (10px, `#666`) | ▲/▼ chevrons (24×15px, radius 4px, `#2a3a5a` fill, `#6ab0f0` icon)
4. **Divider**
5. **Subdivision**: "÷8" display (radius 5px, cyan border `rgba(0,185,235,0.25)`, cyan text `#00b9eb`, 14px font)
6. **Flexible spacer** (min 16px)
7. **Volume**: Speaker icon (10px, `#666`) | Slider (80px wide, 6px track height, rounded ends, cyan fill) | Percentage (11px, `#888`, min-width 30px)
8. **Divider**
9. **File operations**: Import / Export / Upload as 28×28px square buttons (radius 5px, `#222230` fill, `#888` icon)

### Internal Padding

- Container padding: 8px vertical, 12px horizontal
- Group divider margin: 10px each side

### Button Sizing

| Button | Current | New |
|--------|---------|-----|
| Play/Stop | 36×24px | 36×34px |
| BPM +/− | 18×11px | 24×15px |
| File ops | 28×18px | 28×28px |
| Volume slider track | 4px height | 6px height |

### Icon Changes

- BPM +/−: Replace "+" and "−" text with `chevron-up` / `chevron-down` icons
- Subdivision: Replace "≡" hamburger with "÷" division symbol prefix
- File ops: Keep existing `import`/`export`/`upload` icons — they render better at 28px

---

## 4. Row Controls

### Row Height

`desktopRowHeightPx`: 24px → **28px**

### Column Layout (5 grid columns, down from 9)

The accent stripe is drawn as a fixed 3px element *before* the grid (matching the existing `drawAccentStripe` pattern used on mobile). The grid proper has 5 weighted columns:

| Grid Col | Content | Weight |
|----------|---------|--------|
| 0 | Instrument label | 6 (min 80px) |
| 1 | Volume mini-bar | 3 (50px target) |
| 2 | Mute button | 1.5 |
| 3 | Solo button | 1.5 |
| 4 | FX button | 1.5 |
| 5 | Overflow (⋯) | 1 |

**Migration from 9 columns:** `rowControlWeights()` in `drumview_layout.go` returns a new 6-element array. `positionRowWidgets()` in `row_rack_zone.go` updates all `g.Cell(N, 0)` indices. The removed widgets (editBtn, saveBtn, colorBtn, originBtn, deleteBtn) are hidden (not created) on desktop — their functionality moves to the overflow context menu.

**Overflow menu implementation:** Add a `menuBtn` (⋯ icon) at grid column 5. On click, open the existing desktop popup menu (`drumview_context_menu.go`) but with expanded items: Rename, Color, Set Origin, Save Instrument (new entry), and Delete Row. The desktop context menu currently exists but is only used on mobile — it needs to be wired to the overflow button for desktop too.

### Removed from Inline

Moved to overflow menu (⋯):
- Edit/pencil icon → "Rename" menu item
- Color swatch → "Color" menu item
- Origin ("O") → "Set Origin" menu item
- Delete ("X") → "Delete Row" menu item (red text)
- Save icon → "Save Instrument" menu item

### Volume Display

Replace raw dB number + separate volume icon with a **50px colored mini-bar**:
- Bar fill color = instrument accent color
- Bar fill width = volume percentage (0-100%)
- Background: `#1e1e28`, height 3px, border-radius 2px

**Interaction:** Keep the existing `Slider` widget but restyle it as a colored bar (set track height to 3px, hide thumb, use instrument color as fill). The `SliderGroup` and `rowVolIconHitAdapter` click-to-open-popup logic stays wired through the slider rect — no hit area reimplementation needed. The slider itself becomes a visual indicator that doubles as a click target.

### Button Styling (M/S/FX)

- Size: 22×20px (current) → **22×20px** (kept, but with rounded corners)
- Corner radius: 0 → **3px**
- Inactive: fill `#1e1e28`, text `#777`, font-size 10px
- Mute active: fill `#b4462e`, text `#ffa080`, font-weight 600
- Solo active: fill `#c8aa28`, text `#ffe060`, font-weight 600
- FX active: fill `#1a3040`, text `#00b9eb`, font-weight 600

### Label Styling

- Font: 12px Medium (500)
- Color: `#d0d0d8` (brighter than current `#aaa`)
- Minimum width: 80px (up from effective ~50-60px)
- Overflow: ellipsis (but wider column means fewer truncations)

### Add Row Button

- Keep "+" at bottom of row list
- Slightly more visible: text color `#555` → `#777`, hover to `#aaa`

---

## 5. EQ Panel

### Tab Buttons

- Shape: Pill (radius 4px, padding 4px 10px)
- Active: fill `#1a2430`, text `#00b9eb`, border `rgba(0,185,235,0.2)`
- Inactive: fill `#1a1a24`, text `#666`, no border
- Font: 11px Medium

### EQ Curve

- Line color: `#00b9eb` at 90% opacity (from orange `rgba(255,180,50,200)`)
- Fill: `rgba(0,185,235,0.08)` (from orange `rgba(255,180,50,30)`)
- Line width: 2px (the renderer uses integer pixels, so the current 1px line becomes 2px)
- 0dB reference: dashed line, `rgba(120,120,130,0.15)`

### EQ Handles

- Radius: 5px → **5px** (kept, but with visible contrast ring)
- Color: `#00b9eb` (from orange `#ffc050`)
- Active/dragging: `#40d0ff` (from `#fff08c`)
- Border ring: 2px of graph background color for clear separation
- Filter handles (HP/LP): Keep teal `#50dcdc` — distinguishes from parametric

### dB Values

- Render in micro-pills: background `#1a1a24`, padding 1px 4px, radius 2px
- Font: 9px Regular
- Per-band mute: Remove inline "M" buttons. Mute accessible via double-click on dB value.

### Frequency Labels

- Font: 10px Regular (from 9px)
- Color: `#666` (unchanged)

### Graph Height

- Add 10px to EQ panel height for more curve precision

---

## 6. Graph Grid Pane

### Beat Markers

- Render beat numbers (1, 2, 3, 4...) along the top edge of the grid pane
- Font: 10px Regular, color `#555`
- Positioned at each beat column center

### Node Coloring

- Node fill: Tint with desaturated instrument color instead of uniform `RGB(50,50,62)`
- Blend function: `blendColor(base, accent color.RGBA, t float64) color.RGBA` where `result.R = base.R + uint8(float64(accent.R-base.R)*t)` etc.
- Factor `t = 0.3` (30% instrument color, 70% base gray)
- Example: Kick node = `blendColor(RGB(50,50,62), RGB(55,210,110), 0.3)` → `RGB(52,98,76)`

### Edge Visibility

- Edge opacity: `RGBA(130,130,150,100)` → `RGBA(130,130,150,110)` — modest bump

### Selection State

- Selected/hovered node: 2px cyan outline ring (`#00b9eb`)
- Replaces any current selection indicator

### Zoom Indicator

- Show "×1.0" (or current zoom level) in bottom-right corner of grid pane
- Font: 10px Regular, color `#555`
- Fade to invisible at default zoom (opacity 0 when zoom = 1.0)

---

## 7. Timeline & Beat Counter

### Beat Counter

- Wrap in a pill container: background `rgba(0,185,235,0.1)`, padding 3px 8px, radius 4px
- Beat number in cyan, remaining text in `#aaa`
- Example: "Beat **4**/16 | 0:02" where "4" is `#00b9eb`

### Timeline Bar

- Height: current + 2px for easier scrubbing
- View window: Rounded ends (radius 2px)
- Keep existing color scheme (cyan view window, yellow cursor)

---

## 8. Spacing Token Updates

| Token | Current | New |
|-------|---------|-----|
| `buttonPad` | 2px | 3px |
| `desktopRowHeightPx` | 24px | 28px |
| Transport container padding | ~6px | 8px vert, 12px horiz |
| Group divider margin | N/A | 10px each side |
| EQ panel top padding | ~4px | 8px |
| Row separator | border-bottom 1px | Keep, enforce `#1e1e26` |

---

## 9. Files to Modify

| File | Changes |
|------|---------|
| `theme.go` | Update color values, add new tokens, remove deprecated colors |
| `touch_sizes.go` | Update `desktopRowHeightPx` to 28, transport sizing constants |
| `drawing.go` | Update `drawButton` for consistent rounded corners, add beat counter pill rendering, add `blendColor()` helper |
| `drumview.go` | Update `desktopHeaderH` from 72 to 50, `buttonPad` from 2 to 3 |
| `drumview_layout.go` | Single-row transport layout (`outerGrid` → 1 row), update `rowControlWeights()` to 6 cols, label width minimum |
| `transport_zone.go` | Primary transport layout logic — update grid from 2-row to 1-row, group ordering, divider rendering |
| `drumview_toolbar.go` | Delegate to updated `TransportZone`, update button styles |
| `drumview_rows.go` | Volume mini-bar rendering, overflow menu trigger |
| `row_rack_zone.go` | Update `positionRowWidgets()` indices (9→6 cols), hide desktop edit/save/color/origin/delete, add overflow button |
| `drumview_draw_controls_eq.go` | Cyan curve, tab pill buttons, dB micro-pills, handle sizing |
| `eq_panel_zone.go` | EQ curve drawing colors, line width, handle radius |
| `drumview_context_menu.go` | Add rename/color/origin/save-instrument entries, wire overflow button for desktop |
| `game_draw_grid_pane.go` | Beat markers, node color tinting, zoom indicator |
| `drumview_timeline_info.go` | Beat counter pill styling |
| `textcache.go` | Update `FontSize*` constants: Caption→10, add FontSizeSmall=12, Body→14, Label→16, Title→18, Heading→20 |
| `fontcache.go` | No structural changes — font files swap via embed (same `fontRegular`/`fontBold` vars) |
| `font_size_test.go` | Update hierarchy test for new values, add `FontSizeSmall` to ordering check |
| `game_layout.go` | Update header height calculation for single-row transport |
| `drumview_geometry.go` | Header bounds update for new `desktopHeaderH` |
| `src/go/internal/assets/font_embed.go` | Point embeds at `fonts/Inter-Regular.ttf` and `fonts/Inter-SemiBold.ttf` |

**New assets:**
- `src/go/internal/assets/fonts/Inter-Regular.ttf` (~100KB) — replaces GoRegular.ttf
- `src/go/internal/assets/fonts/Inter-SemiBold.ttf` (~100KB) — replaces GoBold.ttf
- Total ~200KB. Increases WASM binary size by ~200KB (font data is `//go:embed`'ed).

**Removed assets:**
- `src/go/internal/assets/fonts/GoRegular.ttf` — replaced by Inter
- `src/go/internal/assets/fonts/GoBold.ttf` — replaced by Inter

---

## 10. Verification Plan

### Unit Tests

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...
```

Many row-related and layout tests reference `desktopRowHeightPx = 24` — these will need updating to 28. Transport layout tests that assume 2-row layout will need updating.

### Visual Verification

1. `make run` — verify desktop appearance matches mockups
2. Check transport bar: single row, grouped controls, dividers visible
3. Check row controls: 28px height, mini-bar volumes, M/S/FX pills, overflow ⋯
4. Check EQ: cyan curve, pill tabs, dB micro-pills
5. Check grid: beat markers, instrument-tinted nodes
6. `make wasm && make test-browser-list` — verify WASM build + run browser tests

### Regression

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -count=1
GO=$(pwd)/.tools/go/bin/go node src/js/highlight_playback.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/live_edit_sync_variants.browser.test.js
```
