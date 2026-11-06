# Drum View Panel Design Refresh

## Context

The drum view panel has several visual inconsistencies that undermine the otherwise strong dark+cyan design language established by the EQ/Wave panels. The most jarring issues are: a saturated yellow playback highlight that clashes with the cool palette, clunky flat-rect transport buttons with no visual grouping, a context menu with off-brand yellow/amber borders and no hierarchy, and fully-saturated instrument colors that create visual noise at high subdivisions.

This spec addresses all four issues while preserving the existing layout geometry, mobile touch targets, and test infrastructure.

## Scope

**In scope:** 4 visual changes to the drum view panel (highlight, transport, context menu, instrument colors) plus cascading color updates to eliminate yellow from the system.

**Out of scope:** Row controls (M/S/FX), EQ panel, grid pane, node rendering, audio, layout geometry changes.

---

## 1. Playback Highlight: White Flash

### Problem
`colHighlight = color.RGBA{255, 220, 40, 255}` is a warm saturated yellow that clashes with the cool dark+cyan theme. It's the loudest element on screen.

### Solution
Replace with a near-white highlight that pops against any instrument color.

### Changes

**`theme.go`:**
- `colHighlight`: `{255, 220, 40, 255}` → `{240, 242, 248, 255}` (cool near-white)
- `colMuteHighlight`: `{90, 90, 100, 160}` → `{180, 182, 190, 160}` (brighter to match white highlight)
- `DrumCellUI.Highlight` automatically picks up the new `colHighlight`

**`drumview_draw.go`:**
- Play button glow color (hardcoded `NRGBA{40, 200, 100, alpha}` at ~line 97): no change needed (green glow is fine, it's on the play button not the grid)

### Mobile impact
None. Highlight rendering is platform-independent — same code path.

---

## 2. Transport Bar: Grouped Pills

### Problem
Play/stop are flat sharp-cornered rects. BPM +/- chevrons are visually disconnected from the BPM input. Upload/import/export icons are indistinguishable. No visual grouping between control categories.

### Solution
Group buttons into pill-shaped containers by function with rounded corners (6-8px radius). Three groups: **transport** (play+stop+record), **BPM** (dec/value/inc as connected stepper), **file ops** (upload/import/export/overflow).

### Changes

**`theme.go`:**
- Add new color: `colTransportGroupBG = color.NRGBA{255, 255, 255, 10}` (pill container background)
- Add new color: `colTransportGroupBorder = color.NRGBA{255, 255, 255, 15}` (pill container border)
- `PlayButtonStyle`: Keep fill `{36, 160, 90, 255}` but use 6px corner radius
- `StopButtonStyle`: Keep fill `{180, 55, 55, 255}` but use 6px corner radius
- `BPMDecStyle` / `BPMIncStyle`: Keep blue fill `{30, 42, 68, 255}` but render as connected stepper ends (left-rounded / right-rounded)
- File ops buttons: Use `colSurface2` fill with no individual border, grouped in shared pill container

**`transport_zone.go`:**
- Modify `Draw()` to render pill container backgrounds behind each button group
- Container radius: 8px (desktop), 10px (mobile)
- Inner button radius: 6px (desktop), 8px (mobile)
- Gap between buttons within a group: 2px
- Gap between groups: 10px (desktop), 12px (mobile)
- BPM stepper: dec button gets `border-radius: 6px 0 0 6px`, value box is flat, inc button gets `border-radius: 0 6px 6px 0`
- Add thin vertical divider (1px, `colTransportDivider`) between export and overflow within file ops group

**`drawing.go`:**
- May need a `drawPillContainer(dst, rect, radius, bg, border)` helper that draws a rounded rect background behind a group of buttons
- Alternatively, reuse existing `drawRoundedRect` with the new container colors

**`components.go`:**
- `drawButton()`: When inside a pill group, suppress individual borders and top-edge highlight. Only the container has the border.
- Consider adding a `ButtonGroup` concept or simply handle it in the transport zone draw code

### Mobile impact
Same visual language at 44px+ touch targets. Mobile transport already uses `TransportPlayStyle` etc. with `colSurface2` — the grouped pill containers provide better visual organization at the larger size. Container radius bumps to 10px for mobile feel.

---

## 3. Context Menu: Grouped & Compact

### Problem
Context menu items use `DropdownStyle` with `colDropdownEdge` (cyan-tinted border) that looks like yellow/amber on the screenshots. Every item has the same visual weight. No semantic grouping. Excessive horizontal whitespace on mobile bottom sheet.

### Solution
Replace individual bordered items with borderless items inside grouped containers (iOS-style grouped list). Add leading icons. Semantic grouping: Identity (instrument/rename/color), Playback (mute/solo), Routing (effects/origin), Danger (delete).

### Changes

**`theme.go`:**
- Add: `colMenuGroupBG = color.NRGBA{255, 255, 255, 8}` (group container background)
- Add: `colMenuGroupDeleteBG = color.NRGBA{220, 50, 50, 15}` (delete group tint)
- Add: `ContextMenuItemStyle = ButtonStyle{Fill: color.RGBA{0, 0, 0, 0}, Border: color.NRGBA{0, 0, 0, 0}}` (transparent — no individual borders)
- Add: `colMenuIcon = colTextDisabled` (icon tint, `{80, 82, 90, 255}`)

**`drumview_context_menu.go`:**
- `contextMenuItems()`: Change `itemStyle` from `ButtonVisual(DropdownStyle)` to the new transparent `ContextMenuItemStyle`
- Item layout: items within a group get no border. The group container provides the visual boundary.
- Group containers: draw rounded rect backgrounds (`colMenuGroupBG`) behind each semantic group
- Group radius: 6px (desktop), 8px (mobile)
- Group padding: 2px internal
- Group gap: 4px between groups
- Item padding: `6px 8px` (desktop), `10px 10px` (mobile) — left-aligned text with leading icon
- Delete group: uses `colMenuGroupDeleteBG` for red tint
- Header (mobile bottom sheet): add row color dot (8px circle) before row name, close button in rounded rect

**`drumview_context_menu.go` draw function:**
- Before drawing individual buttons, draw the group container backgrounds
- Track group boundaries from the divider items
- Draw rounded rect for each group span

**Icon rendering:**
- Icons already exist in the `contextMenuIcons` field and `contextMenuItem.icon` — they're already being set ("note", "pencil", "color", "mute", "solo", "fx", "target", "trash")
- Ensure icon rendering uses `colMenuIcon` tint color (secondary/disabled text)
- Icon width: 14px (desktop), 16px (mobile) — left of label with 8px gap

### Mobile impact
- Bottom sheet uses same grouped structure with larger padding (10px vs 6px)
- Items are left-aligned (no centering) — eliminates the whitespace issue
- Touch targets remain 44px minimum per item
- Header gets row color dot for identification

### Spacing (user feedback: reduce whitespace)
- Group gap: 4px (not 8px) between groups
- Internal group padding: 2px
- Item padding: compact — `6px 8px` desktop, `10px 10px` mobile
- No extra margin between header and first group beyond the divider line

---

## 4. Instrument Colors: Desaturated ~20%

### Problem
Default instrument colors are fully saturated (`{60, 210, 120}`, `{235, 85, 95}`, etc.) creating a loud, fatiguing grid. At high subdivisions, the saturation competes with the playback highlight.

### Solution
Reduce saturation ~20% on all default instrument colors while keeping the same hues. The white highlight will now have even more contrast.

### Changes

**`theme.go` — `instColors` map:**
| Instrument | Before | After |
|-----------|--------|-------|
| kick | `{60, 210, 120}` | `{50, 170, 100}` |
| snare | `{235, 85, 95}` | `{195, 72, 80}` |
| hihat | `{240, 200, 50}` | `{200, 168, 45}` |
| tom | `{80, 120, 235}` | `{68, 100, 195}` |
| clap | `{220, 80, 200}` | `{185, 68, 168}` |

**`theme.go` — `customPalette`:**
| Color | Before | After |
|-------|--------|-------|
| cyan | `{80, 200, 200}` | `{68, 168, 168}` |
| orange | `{200, 130, 80}` | `{170, 110, 68}` |
| purple | `{130, 80, 200}` | `{110, 68, 168}` |
| pink | `{200, 80, 130}` | `{170, 68, 110}` |
| spring green | `{80, 200, 130}` | `{68, 168, 110}` |

### Mobile impact
None. Colors are shared across platforms.

---

## 5. Cascading Color Updates

These values must change to eliminate yellow/amber from the design system:

**`theme.go`:**
- `colTimelineCursor`: `{255, 220, 40}` → `{80, 220, 255}` (bright cyan — matches accent family)
- `colSoloActive`: `{200, 170, 40}` → `{80, 220, 255}` (bright cyan)
- `colSoloActiveBdr`: `{240, 210, 60}` → `{120, 235, 255}` (lighter cyan border)
- `SoloActiveStyle`: updated automatically via `colSoloActive` / `colSoloActiveBdr`
- `colWarningAmb`: `{240, 180, 40}` — audit all usages. If only used for solo/highlight, remove. If used elsewhere (notifications?), replace with cyan accent or keep as semantic warning color.

- `colWarningAmb` is only referenced in `theme.go` itself (for the highlight/solo comment). It's safe to remove or repurpose.
- `colFuzzyHighlight` in `uigrid.go:216`: `NRGBA{255, 220, 40, 60}` — used for fuzzy search match highlighting in the instrument menu. Change to cyan: `NRGBA{0, 200, 255, 60}` to match accent family.

**Note on `DropdownStyle` usage:** `DropdownStyle` is used beyond the context menu (instrument menu, subdiv menu, EQ channel menu, fuzzy search). The context menu change should use a *new* style (`ContextMenuItemStyle`) rather than modifying `DropdownStyle` itself — those other menus keep their current look.

---

## Files to Modify

| File | Changes |
|------|---------|
| `src/go/internal/ui/theme.go` | Color values, new group colors, new menu styles |
| `src/go/internal/ui/transport_zone.go` | Pill group rendering, container backgrounds |
| `src/go/internal/ui/drumview_context_menu.go` | Grouped layout, item styles, icon tinting, spacing |
| `src/go/internal/ui/drawing.go` | Possibly add `drawPillContainer` helper |
| `src/go/internal/ui/components.go` | Button rendering within pill groups (suppress borders) |

---

## Verification

### Visual verification
1. `make run` — check desktop drum view: transport bar groups, highlight color, instrument colors, context menu
2. `make wasm && make test-browser-list` — check WASM build compiles
3. Open mobile viewport (< 900px) — verify bottom sheet grouping, touch targets, transport scaling

### Test verification
```bash
# Fast Go tests (should all pass — no logic changes, only colors)
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/...

# Context menu tests specifically
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run "ContextMenu|contextMenu"

# Transport zone tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run "Transport|transport"

# Color-related tests
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -run "Color|color|Highlight"
```

### What could break
- Tests that assert exact pixel colors (e.g., highlight yellow `{255, 220, 40}`) will need updating:
  - `drumview_test.go:2633,3074,3088` — highlight color assertions
  - `narrow_cell_render_test.go:18,66,91,131,169,213` — uses `colHighlight` (will auto-update via variable)
- Tests that check `DropdownStyle` on context menu items will need updating:
  - `mute_solo_context_menu_test.go:65-66,99-100` — asserts `ButtonVisual(DropdownStyle)` for menu items
- Tests that assert solo button amber color will need updating
- `game_draw_grid_pane.go:576,721,922-927` — uses `colHighlight` for graph node highlights (will auto-update via variable)
- `timeline_zone.go:431` — uses `colHighlight` for timeline playhead highlight (will auto-update)
- `colFuzzyHighlight` in `uigrid.go:216` — hardcoded yellow, needs manual update
