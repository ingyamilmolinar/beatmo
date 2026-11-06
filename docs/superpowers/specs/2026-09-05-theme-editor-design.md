# Theme editor — runtime UI color schemes

**Date:** 2026-09-05
**Status:** approved design, not yet implemented
**Scope:** `internal/ui`, `internal/userprefs`, `internal/hooks`, `internal/eventlogger`, `internal/i18n`, `cmd/gen_design_tokens`, DESIGN.md prose

## Goal

Let the user recolor the whole UI at runtime — grid, menus, dividers, nodes,
accents, visualizers — from a pop-up opened by a new top-right corner button,
using the existing swatch-grid color picker. Colors are grouped into nine
categories so one pick recolors a coherent family. The chosen theme is part of
the user profile (persists across circuits); a circuit may carry its own theme
and the user may choose to override it. Themes are complete, config-only
documents: every UI color is expressible in a JSON theme with no code change.

Non-goals: light mode, theme presets/gallery, per-token editing UI, theming of
instrument identity colors (per-row colors stay the only multi-color signal),
theming of semantic reds/greens (play, stop, mute, delete, record, clip).

## 1. Theme model

### 1.1 Theme document

```json
{
  "categories": { "accent": "orchid-300", "background": "electric-blue-500" },
  "tokens":     { "focus-ring": "#FF2D9E" }
}
```

- `categories`: category id → **swatch name** from the Vice City grid
  (`SuggestedInstrumentSwatches`, e.g. `sunset-gold-300`). Names, never hex,
  so a palette regeneration cannot orphan a saved theme. Unknown names are
  ignored with a WARN.
- `tokens`: DESIGN.md color token name → `#RRGGBB`. Optional escape hatch for
  hand-authored config. Unknown names ignored with a WARN. Tokens in the fixed
  semantic set (§1.2) are rejected with a WARN.
- Both maps partial; both `omitempty`. An empty document is the shipped design.
- Resolution: defaults → category derivation (§1.3) → explicit `tokens`.
- The editor writes `categories` only. When it changes a category it deletes
  every `tokens` entry whose token belongs to that category, so a stale
  hand-authored override never sticks under a fresh pick.

Go type in a tiny new package `internal/theme` (no build tags, no Ebiten
import) so `userprefs` and `internal/ui` can both carry it without a
userprefs→ui import cycle:

```go
package theme
type Doc struct {
    Categories map[string]string `json:"categories,omitempty"`
    Tokens     map[string]string `json:"tokens,omitempty"`
}
func (d Doc) IsEmpty() bool
func (d Doc) Clone() Doc
```

### 1.2 Categories and membership

One table in `internal/ui/theme_categories.go` is the single source of truth
for the editor rows, the derivation engine, and the tests:

| id | label (i18n key) | anchor swatch | member tokens |
|---|---|---|---|
| `background` | Background | `orchid-500` | background, grid-horizon, transport-bar-bg, rack-bed, toolbar-bed, timeline-total-bg, viz-bg, viz-scope-bg, viz-meter-bg, drum-stripe-even, drum-stripe-odd, drum-cell-off, eq-readout-bg, sidebar-badge-bg, viz-pill-fill |
| `surfaces` | Surfaces & Menus | `orchid-500` | surface-1, surface-2, surface-3, surface-overlay, disabled-fill, fx-active-fill, sidebar-section-bg, sidebar-chip-fill, slider-track-fill, scope-label-dim |
| `text` | Text | `orchid-100` | on-surface, on-surface-muted, on-surface-disabled, popup-text-secondary, slider-thumb-fill, sidebar-swatch-fallback, row-rack-color-fallback, drum-cell-highlight, drum-mute-highlight, divider-base |
| `accent` | Accent | `sunset-gold-300` | primary, primary-bright, primary-dim, on-surface-accent, splitter-handle, drum-glow, slider-fill, solo-active-border, eq-filter-active-fill, eq-filter-active-border, viz-glow |
| `focus` | Focus ring | `hot-pink-300` | focus-ring |
| `grid` | Grid lines | `orchid-500` | grid-line, grid-half, grid-quarter, grid-eighth, grid-sixteenth, grid-thirty-second |
| `nodes` | Nodes & Edges | `orchid-500` | node-fill, node-border, edge-color, drum-mute-cell, timeline-beat |
| `dividers` | Dividers & Borders | `orchid-100` | border, divider-shadow, divider-highlight, sidebar-chip-border, viz-pill-border, eq-zero-line, wave-trace-dry |
| `viz` | Visualizers | `sunset-gold-300` | viz-bar, viz-bar-peak, viz-curve, viz-wave-a, viz-wave-b, viz-scope-trace-a, viz-scope-trace-b, viz-scope-trace-diff, viz-scope-trigger, viz-spectrum-peak-marker, viz-meter-low, viz-meter-mid, viz-bass, viz-mids, viz-treble, timeline-view, timeline-view-hi, timeline-cursor |

Fixed (never themed): the 35 chromatic ramp tokens and 8 night neutrals (raw
palette, not roles), success, error, error-text, mute, mute-active-border,
destructive, destructive-border, destructive-confirm-*, record-idle,
record-active, play-desktop-*, stop-desktop-*, eq-mute-active-fill,
menu-delete-tint, viz-meter-high, viz-meter-clip, viz-debug-*, dim-black.

Guard `TestThemeCategoryTableComplete`: every `colors:` token in DESIGN.md is
either in exactly one category or in the fixed list; no token in both; no
unknown names. Adding a token to DESIGN.md without classifying it fails.

### 1.3 Derivation

At init, for every category the engine records each member token's offset
from the category's **anchor swatch** in HSL: `Δh` (degrees), `Δs`, `Δl`
(fractions). Applying a pick `S` sets each member to
`hsl(h(S)+Δh, clamp(s(S)+Δs), clamp(l(S)+Δl))`.

Dark categories (`background`, `surfaces`, `grid`, `nodes`, `dividers`) use a
lightness-guarded variant: hue comes from `S`; saturation from `S` scaled by
the shade column (100 → 0.35, 200 → 0.55, 300 → 0.75, 400 → 0.9, 500 → 1.0)
so the column reads as "tint strength"; the token's **absolute** lightness is
kept from the default (Δl applied against the anchor's default lightness, not
`S`'s). This guarantees a light swatch never yields a light background and the
surface ladder (`background` < `surface-1` < `surface-2` < `surface-3`) keeps
its ordering, which `TestThemeDarkCategoriesPreserveLadder` asserts.

Achromatic member tokens (s ≈ 0, e.g. `border`=#FFFFFF, `divider-shadow`=
#000000) are left exactly as designed: a hue is meaningless for them and
keeping them untouched is what makes the anchor pick an exact identity.

Identity property: applying every category's anchor reproduces the DESIGN.md
hex for every token exactly (`TestThemeDefaultIsIdentity`, byte compare).

## 2. Runtime apply

### 2.1 Engine (`internal/ui/theme_runtime.go`)

```go
func ApplyTheme(doc theme.Doc)           // resolve + write + rebuild + notify
func ActiveTheme() theme.Doc              // last applied (resolved precedence, §3)
func SetThemeForTest(doc theme.Doc) (restore func())
```

`ApplyTheme`:

1. Resolve `doc` → `map[token]color.RGBA` (defaults + derivation + tokens).
2. Write the overridden `genColor*` variables. The generator emits a
   `genColorByName map[string]*color.RGBA` so writes are name-driven; the
   default values are kept in a parallel immutable copy taken at init.
3. `rebuildThemeAliases()` — the `var (...)` block of `theme.go` moves into
   this function (called once from `init()` and again here). The per-file
   top-level derived colors in `render_chain.go`, `render_meters.go`,
   `render_waveform.go`, `synth_concept_viz.go` move into the same function.
   Guard: `TestNoTopLevelDerivedColorsOutsideTheme` (AST scan for package-
   level `color.*`/`WithAlpha(genColor…)` initialisers outside the rebuild).
4. `rebuildComponentSpecs()` — `cmd/gen_design_tokens` emits the
   `componentSpecs` table as a builder function referencing `genColor*`
   symbols instead of literal RGBA (the YAML already names tokens, e.g.
   `{colors.primary}`). `Spec(id)` keeps reading the table; buttons resolve
   by `SpecID` at draw so all spec buttons re-theme with no per-site change.
   `Button.SetSpec` currently copies `IconColor` at construction; change it to
   resolve at draw when `hasSpecID` (one-line in the draw path) so icon tints
   follow the theme.
5. `themeGeneration++` and `theme.OnChange` listeners fire (mirrors
   `i18n.OnChange`). `Game` registers `OnThemeChanged`.

### 2.2 Cache invalidation (`Game.OnThemeChanged`, modeled on `OnLocaleChanged`)

- `dv.invalidateRowCaches()` (row sprites bake stripe/cell colors).
- `dv.invalidateLabelCaches()`.
- Grid pane: `gridTileSubSig = 0`, drop `gridTile`, node layer, node-sprite
  map (keyed by color, so stale keys are just garbage), edge cache
  (`edgesDirty = true`).
- `eqPanelZone.Invalidate()`, chain zone, row rack zone, `synthPanel.sprites.releaseAll()`.
- Text and icon sprites are white and tinted at draw: nothing to do.
- Timeline zone and transport bar cached backgrounds: `Invalidate()`.

Functional guard: `TestThemeApplyRepaintsEverySurface` applies an accent and a
background theme and asserts, via the `drawRect` interceptor (under
`envRenderSafe`, as the audio-panel pixel tests do), that a transport button
fill, a grid-line, a drum-cell-off fill, and a panel background are painted
in the derived colors on the next frame.

### 2.3 Performance

- Zero per-frame cost: draw sites keep reading the same package variables.
- A pick = one pass over ~140 tokens + one cache rebuild burst identical to a
  window resize. No allocation added on any Draw path; the per-tab alloc
  budgets in `eq_panel_draw_alloc_discipline_test.go` are unchanged.
- The editor overlay itself draws through the shared `Button`/`MenuScroll`
  infra; its swatch chips are `drawRect` calls, no sprites.

## 3. Precedence and persistence

### 3.1 Sources and rule

Three sources: shipped defaults, user profile theme `U` with flag
`override`, circuit theme `C`. Effective value for category (and likewise
for an explicit token) `k`:

```
if override && U has k → U[k]
else if C has k        → C[k]
else if U has k        → U[k]
else                   → default
```

Guard: `TestThemePrecedenceTable` enumerates all 8 (override × U-has × C-has)
combinations.

`recomputeEffectiveTheme()` runs on: startup, project import, new/clear
project, any editor pick, override toggle. It calls `ApplyTheme` only when the
resolved document differs from the active one (avoid a needless rebuild burst
on import of an unthemed circuit).

### 3.2 User profile

- `userprefs.prefsDoc` gains `Theme *ThemeDoc \`json:"theme,omitempty"\`` where
  `ThemeDoc` = `theme.Doc` + `OverrideCircuits bool \`json:"override_circuits,omitempty"\``.
- New `userprefs.ThemeStore` interface (`LoadTheme() (ThemeDoc, error)`,
  `SaveTheme(ThemeDoc) error`), implemented by both backends; JSON and
  localStorage round-trip tests.
- `internal/ui/theme_sink.go` mirrors `language_sink.go`: `ThemeSaveSink`,
  `SetThemeSink`, `ApplyStoredTheme()` (called from `cmd/beatmo.go` and
  `js_bootstrap_wasm.go` right after the language sink, before the first
  Layout), `persistUserTheme()`.
- Corrupt/unknown data → WARN + empty theme (boot-store policy).

### 3.3 Circuit

- `projectFile` gains `Theme *theme.Doc \`json:"theme,omitempty"\``.
- Export writes the circuit's own theme document verbatim (never the user's
  effective theme). Import replaces it (replace-not-merge). Schema stays
  `0.0.1` (additive, `omitempty`).
- `Game.circuitTheme` holds it; circuit-scope picks set it and record an undo
  step (the snapshot journal captures the export bytes, which now include the
  theme block, and restore re-imports them). There is no project dirty flag in
  this codebase; the undo journal is the record. `undo_change_detail.go` gets
  a "Theme" detail line.
- Templates ship without a theme block (`TestTemplatesCarryNoTheme`).

## 4. Editor UI

### 4.1 Entry point

- Fourth top-right grid-pane button, one `SpaceSM` left of Logs, same size,
  `NewSpecButton("", ComponentButtonSecondary, g.toggleThemeOverlay)` with a
  new `IconPalette` glyph (24-unit canvas, a palette disc with three dots —
  no forbidden text glyphs). `gridThemeButtonRect` chains off
  `gridLogsButtonRect`; `gridTopButtonsReservedRect` and
  `inTopRightButtonZone` include it; `gridButtonMenuIDs` gains
  `themeOverlayID` so the four menus stay mutually exclusive.
- Width check: 4 × 30 + 3 × SpaceSM + insets fits a 390 px phone.

### 4.2 `ThemeOverlay` (`internal/ui/theme_overlay.go`)

Portal entry, modal like Settings, anchored under the palette button via
`gridAnchoredPopupRect`, width `settingsOverlayW`, height clamped to the pane
with a `MenuScroll` for the rows. Layout top→bottom:

1. Header: title "Theme".
2. Scope switch: two `Button` pills "My theme" / "This circuit" (latched
   like the language pills). Default scope: "My theme".
3. Toggle pill "Always use my theme" (latched when `override` is on). Visible
   in both scopes.
4. Nine rows (`themeCategories` order): label (RoleBody), one-line gloss via
   `PlainEnglish` (RoleCaption), current swatch chip (square, `RadiusControl`,
   shows the effective color for this scope with a ring when the scope
   explicitly sets it, dim when inherited), and a small reset button
   (`IconUndo`) that clears the category in the active scope.
5. Footer: "Reset all" (clears every category and every token in the active
   scope; destructive-confirm two-tap like Delete).

Tapping a chip opens the existing `ColorWheelComponent` (swatch grid) with
`AnchorRect` = chip, `CurrentColor` = the chip's swatch, `OnColorPick` writing
`categories[id] = swatchName` for the active scope, then
`recomputeEffectiveTheme()` + persist + `emitThemeChanged`. The overlay stays
open after a pick so the user keeps tweaking and sees the result live behind
the panel. Esc / gear / outside-click close as today.

Mobile: same panel, pane-clamped; picker cells use the touch minimum as they
do for row colors.

### 4.3 Text

New i18n keys (en + es): `theme.title`, `theme.scope.mine`,
`theme.scope.circuit`, `theme.override`, `theme.reset_all`, `theme.cat.<id>`
(9), `theme.gloss.<id>` (9). Added to `AllKeys()`.

## 5. Events

A registry entry carries exactly one scope per Kind, and the two scopes have
different undo semantics, so there are two kinds:

- `hooks.EventThemeChanged` (`"theme.changed"`, tag `theme`, `ScopeSession`):
  a profile-scope pick, reset, or override toggle. Not undoable, like
  language. Payload: `Category string`, `Swatch string`, `Override *bool`.
- `hooks.EventCircuitThemeChanged` (`"theme.circuit.changed"`, tag `theme`,
  `ScopeDocument`): a circuit-scope pick or reset. Mutates the exported
  document and is undoable through the snapshot journal like other project
  edits. Same payload shape.
- `eventlogger` formatters: `change theme: accent → orchid-300 (my theme)` /
  `… (this circuit)`; both tags lowercase.
- `NumNonVerbose` 74 → 76; `KindAll`, registry, formatter coverage, and the
  user-action registry (undo label for the circuit kind) all updated.
- Layer B: the chip tap already yields `[ui] tap`; no extra kind.

## 6. Testing

Unit (`-tags test`):
- `TestThemeDefaultIsIdentity`, `TestThemeCategoryTableComplete`,
  `TestThemeDarkCategoriesPreserveLadder`, `TestThemeAchromaticStaysHairline`,
  `TestThemeTokensOverrideAfterCategories`, `TestThemeRejectsFixedTokens`,
  `TestThemeUnknownNamesIgnored`, `TestThemeEditorPickClearsCategoryTokens`.
- `TestThemePrecedenceTable`; `TestRecomputeSkipsApplyWhenUnchanged`.
- `userprefs`: theme round-trip both backends, corrupt-data policy.
- Project: export/import round-trip with and without theme; legacy file
  without `theme` imports clean; templates carry no theme.
- Generator: `componentSpecs` builder references tokens (snapshot test);
  `TestComponentSpecsDrift` unchanged and green.
- `TestNoTopLevelDerivedColorsOutsideTheme`.
- hooks/eventlogger: registry, tag, formatter, non-verbose count.
- Design discipline: drift, token discipline, lint snapshot, anti-pill, glyph
  scan all unchanged and green (default theme is identity; the HSL math lives
  in `theme_derive.go`, added to the discipline exemption list with rationale).

Functional (real input through `Game.Update`, per [[feedback_verify_end_to_end_not_just_units]]):
- Click the palette button → overlay open; other corner menus closed.
- Tap Accent chip → picker open anchored to chip; pick `orchid-300` on release
  → transport-button fill pixel and splitter-handle pixel equal the derived
  colors; prefs sink received the doc; `[theme]` line logged.
- Switch scope to "This circuit", pick Background → project dirty, undo
  restores previous background pixel, redo re-applies.
- Override toggle flips precedence with a circuit theme loaded.
- `ApplyStoredTheme` before first frame renders themed on frame 1.
- Zone input isolation discipline covers `ThemeOverlay` (catch-all present).
- Alloc discipline: the existing per-tab budgets stay green (the overlay adds no per-frame allocation on any audio-panel tab).

Screenshots (mandatory, inspected by eye, desktop + `MOBILE`):
`theme_editor`, `theme_editor_picker`, `theme_applied_orchid` (full UI with
Accent=orchid-300, Background=electric-blue-500, Viz=mint-300, Synth tab open),
`crop_theme_editor` subject crop. Kept in the catalog as regression surfaces.

## 7. Files touched (summary)

New: `internal/theme/doc.go`, `internal/ui/theme_categories.go`,
`theme_derive.go`, `theme_runtime.go`, `theme_sink.go`, `theme_overlay.go`,
`theme_overlay_layout.go`, icon `IconPalette`, tests as above, scenes.
Modified: `theme.go` (var block → `rebuildThemeAliases`), `render_chain.go`,
`render_meters.go`, `render_waveform.go`, `synth_concept_viz.go`,
`cmd/gen_design_tokens/components.go` + `design_components.gen.go` +
`design_tokens.gen.go` (name map), `uigrid.go` (icon color at draw),
`game_new.go`, `game_layout.go`, `grid_pane_draw.go`, `grid_reserved_zone.go`,
`game_input_shortcuts.go`, `game_draw_grid_pane.go` / cache invalidation,
`drumview_cache_dirty.go`, `project_file.go`, `export.go`, `import.go`,
`undo_change_detail.go`, `userprefs/store*.go`, `hooks/events.go`,
`hooks/actions.go`, `eventlogger/format.go`, `i18n/keys.go`, catalogs,
`cmd/beatmo.go`, `js_bootstrap_wasm.go`, `scene_catalog.go`, DESIGN.md prose
("Runtime themes" subsection next to the chrome-accent invariants: the
invariants describe the *default* theme and the category structure that every
theme preserves), CLAUDE.md (theme section + standing tables).
