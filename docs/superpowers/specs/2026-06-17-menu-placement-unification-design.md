# Menu placement + minimalist unification — Design

**Date:** 2026-06-17
**Branch:** add-core-node-types
**Status:** Approved (brainstorming), pending implementation plan

## Problem

Two related problems with the drum-row / graph menus:

1. **Placement.** When a row's kebab ("⋯") context menu or per-row color picker opens,
   it anchors off the row label and grows downward, frequently **clipping off the bottom
   of the screen** and landing far from the control the user actually tapped (the kebab is
   at the row's far right; the menu anchors at the label on the left). The instrument-label
   menu already lands better. The user wants every row-opened menu to pop at a consistent,
   fully-visible spot near the controls.

2. **Style.** Menus currently use the heavier "Neon Horizon" chrome (sunset-gradient header
   bands, per-row keycap bevels, accent stripes). The user wants every menu unified to a
   cleaner, more minimal, modern look — closer to the instrument-picker menu — with better
   use of space.

## Goals

1. Row-opened menus open **pinned to the top edge of the bottom (audio) panel**, growing
   downward, fully on-screen, left-aligned to the row control column.
2. **Every** menu adopts a **minimalist-flat** style with a **per-instrument color accent**
   (neutral azure for menus that aren't scoped to one instrument).
3. `DESIGN.md` updated to describe the new menu style and tokens (normative source of truth).

## Non-goals

- No change to menu **contents** (items, ordering, behavior) — placement + style only.
- No gesture/input-routing rework. Menus keep their existing open/close/scroll machinery
  (`MenuScroll`, overlay portals, `dvOverlayPortal`).
- Mobile bottom-sheet behavior is preserved (already finger-optimal); it only gets the restyle.

## Decisions (from brainstorming)

| Question | Decision |
|---|---|
| Where do row menus open? | **Top of the bottom/audio panel**, grow down, never clipped. |
| Menu width | **Left-aligned narrow** (under the row control column), as mocked — not full width. |
| Graph node pop-up placement | **Stays anchored near the node** (restyle only). |
| Global menus (subdiv ÷8, overflow) placement | **Keep transport-trigger anchor** (restyle only). |
| Mobile | **Keep bottom sheet**; apply restyle only. |
| Style direction | **Minimalist flat** (mock option B). |
| Accent color | **Per-instrument color** where the menu is scoped to an instrument; **azure** otherwise. |
| Destructive (Delete) color | **Stays red** — fixed semantic role, never instrument-tinted. |
| Restyle scope | **All menus**: row context, instrument picker + submenus, graph node pop-up, subdiv, overflow, color picker. |

## Design

### A. Placement

Add a shared anchor helper alongside `AnchorPopupRect` in `overlay_anchor.go`:

```
AnchorPopupPanelTop(bounds, panelTopY, leftX, w, h) image.Rectangle
```

- Vertically pins the popup's top to `panelTopY` (the bottom-panel top edge), grows down.
- Horizontally places at `leftX` (the row control column left), clamped fully inside `bounds`
  via the existing `ClampRectInto`.
- The bottom-panel top edge is read from `dv.eqPanelZone.rect.Min.Y` (existing field;
  add a small accessor like `(*DrumView).bottomPanelTopY()` rather than reaching into the
  zone from call sites).

Switch these row-opened menus to the new helper:

- `openContextMenu` (`drumview_context_menu.go`) — currently `AnchorPopupRect(..., PopupRight)`.
- `openInstMenuForRow` / `buildInstrumentsMode` (`drumview_instrument_helpers.go`,
  `drumview_overlay_inst_comp.go`) — replace the bespoke above/below space logic with the
  shared panel-top anchor (desktop path only; mobile keeps `UseBottomSheet`).
- Per-row color picker (`drumview_color_menu.go` / `openColorPickerForRow`).

Leave unchanged: graph node pop-up anchoring, subdiv (`drumview_overlay_subdiv_comp.go`),
overflow (`overflowPopupRect`), and all mobile bottom-sheet branches.

**Edge cases:** menu taller than the available panel height → clamp + rely on the existing
`MenuScroll` to scroll within the clamped rect (it already handles offset + scrollbar). Menu
must never exceed `bounds`.

### B. Visual style — flat (concentrated in `menu_chrome.go`)

The three shared helpers already accept an `accent color.Color`; the call sites already thread
it. The change is to make their *rendering* flat:

- **`drawMenuHeaderBandAccent`** → drop the vertical sunset gradient + the bevel feel. Render a
  **slim title row**: a small instrument-color dot + the name drawn in the accent color
  (`TextRole` caption/section), optional 1px hairline under it. New small helper
  `drawMenuTitle(dst, r, name, accent)` may host this.
- **`drawMenuItemBackgroundAccent`**:
  - `rest` → **flat** (remove `drawRoundedRect(colSurface2)` + `drawKeycapBevel`).
  - `hover` → flat `WithAlphaFromColor(accent, AlphaSubtle)` fill (drop the `colSurface3`
    cushion lift/glow).
  - `active` → `WithAlphaFromColor(accent, menu-active-tint)` fill + thin accent left stripe +
    accent-tinted icon.
- Hairline group separator helper (`drawMenuSeparator`) for the rule before destructive items.
- Destructive items: text in `colError` (red) regardless of accent.

The menu container surface stays a single calm dark surface (no per-row pads).

### C. Accent color rule

- Instrument-scoped menus resolve accent from `DrumRow.Color` via the existing
  `row_instrument_shades.go` helpers (this per-instrument menu coloring already partly exists).
- Global menus (subdiv, overflow) pass `colAccent`.
- Graph node pop-up: instrument color when the node maps to a row/instrument; else `colAccent`.

### D. DESIGN.md updates (normative)

- **Menu chrome narrative**: replace the "Neon Horizon" gradient-header/keycap-bevel menu
  description with the minimalist-flat spec (flat rows, slim title, per-instrument accent,
  hairline separators, red destructive).
- **Tokens**: reconcile the menu alpha tokens with the flat design.
  - `menu-active-tint` — retained (active row fill).
  - `menu-header-accent` — the gradient header underline alpha; **retire** if the gradient
    header is removed, or repurpose for the slim title hairline. Whichever: update DESIGN.md
    YAML, the Go constant, **and** the want-map in `design_md_drift_test.go` in the same change.
  - If a new hover/separator alpha bucket is needed, prefer an existing `Alpha*` bucket
    (`AlphaFaint|AlphaSubtle|…`) rather than introducing a new opacity value.
- **Semantic invariants**: add/confirm "menus are tinted by the owning instrument's color;
  global menus use `primary`/azure; destructive stays `destructive` red."

## Affected files (initial)

- `overlay_anchor.go` — new `AnchorPopupPanelTop`.
- `menu_chrome.go` — flat helpers + title/separator helpers.
- `drumview_context_menu.go`, `drumview_instrument_helpers.go`,
  `drumview_overlay_inst_comp.go`, `drumview_color_menu.go` — adopt panel-top anchor.
- `drumview_overlay_subdiv_comp.go`, overflow, node pop-up, color picker render paths — adopt
  flat helpers + correct accent.
- `DESIGN.md` + `design_md_drift_test.go` (+ `theme.go`/`theme_tokens.go` constants) for token
  changes; mind `token_discipline_test.go`.
- A `bottomPanelTopY()` accessor on `DrumView`.

## Testing

- **Go fast-path** (`-tags test`):
  - Placement: for each row menu, assert the menu rect top == bottom-panel top, the rect is
    fully inside screen bounds (not clipped), and left-aligned to the control column.
  - Accent: per-instrument menu render uses the instrument color (intercept draw calls / assert
    accent threaded); global menus use azure; Delete uses red.
  - Flat-style guard: assert the menu render no longer invokes the gradient/keycap-bevel paths
    (interceptor or absence test), mirroring existing menu render tests.
  - Token guards: `TestDesignMDDrift`, `TestTokenDiscipline`, `TestDesignMDLintSnapshot`.
- **Screenshots**: re-capture `instrument_menu_open`, `context_menu_open`, `subdiv_menu_open`,
  `color_wheel_open`, and the node pop-up scene (desktop + mobile); run `scene_crop_test.go`.
- Follow CLAUDE.md "After Modifying" fast-path for `./internal/ui`.

## Risks / notes

- The branch carries large uncommitted WIP; the `internal/ui` suite has known pre-existing
  failures unrelated to menus — verify via `-run` filters, not the whole-suite pass/fail.
- Per repo convention, **do not commit** — leave all changes (including this spec) in the
  working tree; the user owns committing.
