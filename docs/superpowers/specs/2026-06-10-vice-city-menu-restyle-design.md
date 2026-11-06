# Vice City Menu Restyle — Design

> Status: approved design, pre-implementation.
> Date: 2026-06-10. Branch: `add-core-node-types`.
> Scope: every menu/popup surface in `internal/ui/`, headline = node property sidebar.

## Problem

The menus and popups look "rough and primitive" relative to the rest of the
app. The grid pane carries the Vice City mood (sunset/violet horizon gradient,
neon nodes, cushioned buttons); the menus are flat dark boxes with chunky text
that read as a different, older app. Concretely, audited against DESIGN.md and
the live rendering (`screenshots/all/desktop/{node_sidebar_open,context_menu_open,instrument_menu_open,color_wheel_open,subdiv_menu_open}.png`):

1. **Typography is soft/chunky, with no hierarchy.** All chrome text is rendered
   as a *single* 14px Inter-SemiBold sprite (`fontcache.go` →
   `renderTextSprite(s, FontSizeBody=14, bold=true)`), and menus get "larger"
   text by `GeoM`-upscaling that one sprite (`DrawTextAtScale(..., sidebarLabelScale()=1.2)`).
   The blurry upscale — not a wrong font — is the primitive look. There is no
   crisp panel-title / section-header / body / caption distinction.
2. **Inert chrome.** Menus are flat `surface-overlay` rounded rects; no sunset
   echo, no neon accent, no cushion depth on rows. They don't feel part of the
   same world as the grid pane.
3. **Hand-drawn triangle chevrons** in the sidebar (`drawFilledTriangleRight/Down`
   in `game_node_sidebar.go`) instead of the icon-system chevrons — a glyph-rule
   deviation and visually inconsistent.
4. **Cyan in chrome.** The long-press popup's "Connect" hover text uses `colStep`
   (cyan), violating the single-azure-chrome-accent invariant
   (`game_longpress_popup.go:168`).
5. **Hardcoded dimensions.** The long-press popup uses literal `80/44/8/16`
   instead of spacing/density tokens (`game_longpress_popup.go:28-32`).
6. **Weak active/selected state** — ad-hoc highlights, no left accent stripe,
   no value-pill / stepper polish.

## Goal

Every menu adopts a cohesive **"Neon Horizon"** Vice City treatment plus crisp
Inter typographic hierarchy, staying within DESIGN.md's expressive range. Same
information architecture as today — this is visual character + typography, not a
re-layout. Direction "A · Neon Horizon" was chosen from three mockups
(see `.superpowers/brainstorm/`).

## Non-goals

- No change to any menu's information architecture, item set, or behavior.
- No new audition/preview paths.
- Not migrating every non-menu panel header to the new typography API in this
  pass (the API is added globally; only menu call-sites switch now — controlled
  blast radius). Other panels are a future sweep.

## Design

### Unit 1 — Crisp sized typography API (`textcache.go` / `fontcache.go`)

The single biggest visual win and a precondition for hierarchy.

- **What it does:** renders and caches a text sprite at the *true target pixel
  size and weight*, instead of rendering one 14px sprite and GeoM-scaling it.
- **Public surface:** a role-based draw helper, e.g.
  `DrawTextStyled(dst, s, x, y, role TextRole, col color.Color)` with roles:
  - `RolePanelTitle`  — 21px Inter SemiBold (menu/panel headers, e.g. "Kick-1")
  - `RoleSectionHeader` — 16px Inter SemiBold (section labels: Volume/Pitch/…)
  - `RoleBody` — 14px Inter Regular (menu item labels, values)
  - `RoleCaption` — 12px Inter Regular (sub-labels: "Vol", "Pct")
  - Provide matching `StyledTextWidth(s, role)` / `StyledTextHeight(role)` for layout.
- **Cache:** key the sprite cache by `(string, sizeKey, bold)` (today it is keyed
  by string alone). `renderTextSprite(s, size, bold)` already accepts size+weight;
  the cache and public API are what hardcode `FontSizeBody, true`.
- **Dependencies:** Inter is already embedded (`assets.GoRegularTTF` =
  Inter-Regular, `assets.GoBoldTTF` = Inter-SemiBold; legacy names).
  Test builds keep the debug-font fallback path (no TTF under `-tags test`):
  the role API must degrade to the existing fallback metrics so fast-path tests
  still run.
- **Additive:** existing `DrawTextAt` / `DrawTextColorAt` / `DrawTextAtScale`
  callers are untouched. Only menu call-sites migrate to the role API.

DESIGN.md `typography:` block is corrected to reflect reality: `fontFamily`
becomes Inter (was inaccurately "Ebiten Debug") and the four menu roles pin the
px sizes above. The drift/lint guards are updated to match.

### Unit 2 — Shared "Neon Horizon" menu chrome helpers (new, e.g. `menu_chrome.go`)

One small helper set so every menu is visually identical. These wrap the existing
`drawPanel` / `drawRoundedRect` / icon / cushion primitives — no parallel
rendering stack.

- `drawMenuHeaderBand(dst, rect, opts)` — header band with a faint vertical
  sunset gradient (`grid-horizon` → `surface-overlay`, via the existing
  `fillVerticalGradient`) and a 1px azure bottom-edge accent
  (`WithAlpha(primary, menu-header-accent)`). Optional identity swatch (the
  instrument color) drawn with a soft glow.
- `drawMenuItemBackground(dst, rect, state)` — resting = transparent (panel
  surface shows through); hover = cushion lift (`surface-2`→`surface-3`) + faint
  azure glow; active/selected = left azure stripe (`accentStripeWidth` token) +
  `WithAlpha(primary, menu-active-tint)` fill.
- `drawMenuChevron(dst, rect, expanded, col)` — icon-system `chevron Right`
  (collapsed) / `chevron Down` (expanded) via `DrawIcon`.
- Value pills + steppers reuse the `button-stepper` component styling
  (navy fill + steel border) with `RoleCaption` labels.

### Unit 3 — Tokens (additive, minimal)

- Gradient + accent reuse existing tokens (`grid-horizon`, `surface-overlay`,
  `primary`). No new colors.
- Two new alpha buckets in DESIGN.md `alpha:`:
  - `menu-active-tint` — active menu-row fill alpha (~`subtle`, 60).
  - `menu-header-accent` — header underline alpha (~`accent-tint`, 50).
- Each new token: documented inline in DESIGN.md, added to the
  `design_md_drift_test.go` want-map, and to the lint expected set if flagged.
- No new inline `color.RGBA{}` / `color.NRGBA{}` literals — use `Token*()`
  accessors; update `token_discipline_test.go` budgets (ratchet) as literals are
  removed.

### Unit 4 — Per-menu application

| Menu | File(s) | Changes |
|---|---|---|
| **Node sidebar** (headline) | `game_node_sidebar.go` | Gradient header band + glowing identity swatch; `RolePanelTitle` title; `RoleSectionHeader` section labels; icon chevrons (drop `drawFilledTriangle*`); active-section azure stripe + glow; `RoleCaption` sub-labels; tokenized value pills + steppers. Replace `sidebarLabelScale()` upscaling with role API. |
| **Drum-row context menu** | `drumview_context_menu.go` | Shared header band (row color dot + name); item rows via shared bg helper + `RoleBody`; `Delete` keeps red *text* (`context-menu-item-destructive`); icon column preserved. |
| **Overflow / file menu** | `drumview_context_menu.go` | Shared chrome; subgroup labels `RoleCaption` muted; actions `RoleBody`. |
| **Instrument menu** | `drumview_overlay_inst_comp.go` | Shared chrome; keep swatch+label "gold standard"; selection azure stripe; search box + breadcrumb typography via roles. |
| **Subdiv menu** | `drumview_overlay_subdiv_comp.go` / `drumview_subdiv_menu.go` | Compact shared card; active value = azure stripe (not just border). |
| **Color picker** | `drumview_overlay_color_comp.go` | Header band; swatch grid unchanged (instrument identity); azure selection ring. |
| **Slider popups** | `slider_popup.go`, `drumview_volume_popup.go` | Shared panel; title via `RolePanelTitle`; track/thumb already tokenized. |
| **Long-press popup** | `game_longpress_popup.go` | Shared rounded buttons; **Connect hover azure not cyan**; replace `80/44/8/16` with spacing/density tokens; `button-label` typography. |
| **Tooltip** | `tooltip_overlay.go` | `RoleCaption`. |

### Unit 5 — Compliance fixes (folded into the above)

Cyan-in-chrome removed; triangles → icons; hardcoded dims → tokens; token
discipline kept green.

## Testing (all automated — no manual steps)

- **Typography:** a test asserting `RolePanelTitle` / `RoleSectionHeader` /
  `RoleBody` produce *distinct* `StyledTextHeight` values (proves sized
  rendering, not upscale). Extend `font_size_test.go`.
- **Tokens:** `design_md_drift_test.go` want-map gains the two alpha buckets;
  `design_md_lint_test.go` expected set updated if flagged;
  `token_discipline_test.go` budgets ratcheted for removed literals.
- **Glyphs:** `forbidden_glyphs_test.go` stays green; sidebar no longer hand-draws
  triangles.
- **Single accent:** assert no `colStep`/cyan token used in `game_longpress_popup.go`
  chrome (cyan stays `viz-*` only).
- **Pixel/scene:** extend the existing `scene_crop_test.go` / render-pixel
  pattern for the restyled menus; regenerate `screenshots/all` for visual review.
- **Fast path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
  must pass; `TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot`
  must pass; `make gen-design-tokens` re-run for any DESIGN.md change.

## Risks / notes

- Sized-text cache growth: keyed by `(string,size,bold)`; the role set is small
  (4) so growth is bounded. Verify no per-frame distinct-string churn on menus.
- `-tags test` has no TTF → role API must fall back cleanly to debug-font metrics
  (existing pattern) or fast tests break.
- `make gen-design-tokens` must be run after every DESIGN.md edit (pre-commit
  hook rejects stale `*.gen.go`).
- Mobile bottom-sheet variants of each menu must get the same treatment
  (`drawBottomSheetPanel` path), not just the desktop popup path.
