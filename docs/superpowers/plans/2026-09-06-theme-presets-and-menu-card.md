# Theme Presets + MenuCard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users pick one of six first-party themes (Vice City default) with an optional collapsed "Adjust colors" section, and every floating menu shares one frame and one close behavior modeled on the swatch color picker.

**Architecture:** Part A adds a preset table + a `preset` field resolved before per-category picks. Part B adds `MenuCard`, a value-type frame composed ONLY from existing primitives (`drawPanel`, `drawMenuHeaderBandAccent`, `drawMenuTitle`, `newPopupCloseButton`, `closeButtonRect`, `buttonHitAdapter`, `MenuScroll`, `drawMenuRow`), plus `menuCardEntry` for portal flags; every menu keeps its own type/body/input and adopts the frame. A discipline test enumerates portal overlays.

**Tech Stack:** Go 1.26 via `.tools/go/bin/go`; tests `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <Name>`.

**Spec:** `docs/superpowers/specs/2026-09-06-theme-presets-and-menu-card-design.md`

## Global Constraints

- Never `git add`/`git commit` (owner rule). "Commit" steps = `git status --short` sanity check.
- Reuse primitives; never duplicate a style/behavior. No new `color.RGBA{}` literals in `internal/ui` (token discipline). No forbidden glyphs. Radius by role (`fitRadius` in shared helpers).
- Component-tree isolation: every overlay stays a portal entry publishing its own `HitArea`s (buttons first, catch-all LAST); GridTree layers (node sidebar, group menu) stay GridTree layers; the kit never reads Game/DrumView state.
- Close-X policy stays `desktopCloseVisible()` (X on desktop, none on mobile bottom sheets).
- Every UI change is verified with screenshots opened and inspected (desktop + `-mobile -win-w 390 -win-h 844`).
- Default look (Vice City preset unset / shipped tokens) stays byte-identical: drift/discipline/screenshot tests untouched.

---

### Task A1: Preset table + `Preset` field + resolution

**Files:** Create `src/go/internal/ui/theme_presets.go`, `theme_presets_test.go`; Modify `src/go/internal/theme/doc.go` (field + Clone/Equal/IsEmpty), `src/go/internal/ui/theme_derive.go` (`resolveThemeTokens`), `theme_state.go` (`effectiveTheme` merges Preset with the category rule; new `setUserThemePreset/setCircuitThemePreset`).

**Interfaces:** `type ThemePreset struct{ID string; NameKey, GlossKey i18n.Key; Categories map[string]string}`; `var themePresets []ThemePreset`; `func themePresetByID(id string) (ThemePreset, bool)`; `theme.Doc.Preset string`; `(g *Game) setUserThemePreset(id string)`, `(g *Game) setCircuitThemePreset(id string)`; `func effectivePresetID(user theme.UserDoc, circuit theme.Doc) string`.

- [ ] Test first (`theme_presets_test.go`): every preset has all 9 categories with valid swatch names and unique ids (`TestThemePresetsWellFormed`); `vice-city` resolves to the shipped defaults for every token except `focus-ring` (`TestVicePresetIsShippedPaletteButFocusRing`); resolution order preset < categories < tokens (`TestPresetResolutionOrder`); unknown preset → 1 warning; `effectiveTheme` merges Preset with the same rule as categories (extend `TestThemePrecedenceTable` with a preset column); `setUserThemePreset` clears the scope's categories+tokens and persists (`TestPresetPickClearsCustomizations`).
- [ ] Run → FAIL. Implement: presets table per spec A.1 (i18n keys `theme.preset.<id>` + `theme.preset.<id>.gloss`, en+es, `AllKeys`); `resolveThemeTokens`: after copying defaults, if `doc.Preset != ""` → `themePresetByID` (warn if unknown) and derive its categories, then the doc's categories, then tokens; `effectiveTheme` picks Preset (override&&user!="" → user; else circuit; else user); `dropTokensOfCategory` unchanged; `setUserThemePreset(id)`: `g.userTheme.Doc = theme.Doc{Preset: id}` + recompute + persist + `emitThemeChanged("user", "preset", id)`; circuit variant emits `emitCircuitThemeChanged("preset", id)` (undoable). `hooks.ThemePayload` reuses `Category:"preset", Swatch:id`; formatter prints `preset = ocean`.
- [ ] Run → PASS; run `TestTheme|TestApplyTheme|TestRecompute|TestProject` set.

### Task A2: Preset rows + "Adjust colors" expander in the Theme overlay

**Files:** Modify `theme_overlay.go`, `theme_overlay_test.go`, `theme_picker_e2e_test.go`; i18n keys `theme.adjust` ("Adjust colors"/"Ajustar colores"), `theme.customized` ("customized"/"personalizado").

**Interfaces:** `(*ThemeOverlay).PresetRowRect(id string) image.Rectangle`, `(*ThemeOverlay).AdjustHeaderRect() image.Rectangle`, `(*ThemeOverlay).AdjustExpanded() bool`, `presetSwatchStrip(dst, r image.Rectangle, p ThemePreset)` (5 rounded squares: background, surfaces, accent, viz, text — drawn with `drawRoundedRect` + `fitRadius(RadiusControl…)`, colors from `swatchRGBA`).

- [ ] Tests first: layout has one row per preset inside the panel (scrolling if short), the effective preset row is `menuItemActive`; real click on the Ocean row → `g.UserTheme().Preset=="ocean"` + `genColorPrimary` = tangerine-300 + categories cleared; "Adjust colors" header click toggles the chip rows (chips empty rects when collapsed, present when expanded; hit areas absent when collapsed); a chip pick after a preset keeps `Preset` and the row's trail reads "customized"; mobile: rows ≥ touch min.
- [ ] Implement: rows as `*Button`s laid out by the existing `layoutRows` pattern; draw via `drawMenuRow(dst, btn, MenuRowSpec{State, Label, TrailLabel})` after painting the swatch strip in the leading slot (pass `IconID:""`, offset the label by the strip width — extend `MenuRowSpec` with an optional `Leading func(dst, r image.Rectangle) int` ONLY if `drawMenuRow` has no slot for custom leading art; otherwise draw the strip and set `Label` with a left pad). Expander header via `drawMenuGroupHeader` + `drawMenuChevron`. One `MenuScroll` over presets+chips as a single virtual list (row stride = rowH). Update `assertThemeRowsReachable` for the new row set.
- [ ] Screenshots: `theme_editor` (desktop + mobile), open the expander in a new scene `theme_editor_adjust`; inspect.

### Task B1: `MenuCard` kit + `menuCardEntry` + reference parity on the swatch picker

**Files:** Create `menu_card.go`, `menu_card_test.go`; Modify `drumview_overlay_color_comp.go` (use the card for its frame), `drumview_portal_open.go` (`openColorWheelPortal` uses `menuCardEntry`).

**Interfaces (exact):**
```go
type MenuCardSpec struct {
	Title      string; Swatch color.Color; IconID IconID; Accent color.Color
	Caption    string        // "" → no caption band
	Footer     []*Button     // right-aligned keycaps, last = rightmost
	BottomSheet bool         // mobile sheet: drag handle, no X (desktopCloseVisible)
}
type MenuCard struct { spec MenuCardSpec; panel, title, caption, body, footer image.Rectangle; closeBtn *Button }
func NewMenuCard(onClose func()) *MenuCard                    // closeBtn = newPopupCloseButton(onClose) (nil on mobile)
func (c *MenuCard) Layout(panel image.Rectangle, spec MenuCardSpec)
func (c *MenuCard) Draw(dst *ebiten.Image)                     // drawPanel → band+title/icon → caption (RoleCaption, colTextSecondary) → footer buttons → closeBtn
func (c *MenuCard) HitAreas(z int, tag string, catchAll HitHandler) []HitArea // closeBtn + footer via buttonHitAdapter, then {panel, catchAll} LAST
func (c *MenuCard) Body() image.Rectangle; func (c *MenuCard) CloseBtn() *Button; func (c *MenuCard) Panel() image.Rectangle
func menuCardEntry(id string, ov PortalOverlay, anchor image.Rectangle, mustAnswer bool, onClose func()) PortalEntry // Modal: mustAnswer, Scrim: true
```
Geometry: title band height = `StyledTextHeight(menuTitleRole)+2*SpaceSM` (mirror the picker's `colorSwatchHeader`), caption band = `colorShadeHintH`-equivalent (`StyledTextHeight(RoleCaption)+SpaceXS`), content inset `cardContentPadX` horizontally / `SpaceSM` vertically, footer height `themeResetAllH` (32) + `SpaceMD`, close X at `closeButtonRect(panel, SpaceXS)` and the title text clipped before it.
- [ ] Tests: bands never overlap and lie inside the panel; X inside the title band (desktop) and nil on mobile; hit areas order (buttons first, catch-all last); `menuCardEntry` flag table; **parity**: drawing the swatch picker before/after adopting the card yields the same set of `drawRoundedRect`/`drawRect` calls for the frame (capture with the interceptors; compare rects+colors ignoring order).
- [ ] Implement; migrate `ColorWheelComponent.rebuildWheel/Draw/HandleInput/closeBtn` onto the card (its own cells stay). Run `TestColor*|TestSwatch*|TestNoPillShapedControls`.

### Task B2: Card-family menus — Theme, Settings, Logs, File

**Files:** `theme_overlay.go`, `settings_overlay.go`, `logs_component.go`, the overflow/File menu (`drumview_context_menu.go` overflow items: drop the `header:true` title row; title moves to the card), `game_input_shortcuts.go` (`openSettingsOverlay` → `menuCardEntry`), `drumview_logs_panel.go`; tests `menu_card_close_behavior_test.go`.

- [ ] Table-driven real-input test `TestEveryMenuClosesTheSameWay`: rows `{name, open func(g), locate func(g) (panel image.Rectangle, closeBtn *Button)}` for theme, settings, logs, file; assert: opened → X click closes (desktop); reopen → Esc (stubKeys) closes; reopen → tap outside the panel (inside the drum pane, on empty rack bed) closes only that menu; the grid button still toggles.
- [ ] Migrate each: `MenuCard` frame (IconPalette/IconSettings/IconLogs/IconFolder), body laid out from `card.Body()`, hit areas = `card.HitAreas(...)` + body areas, portal entry via `menuCardEntry(id, ov, anchor, false, …)`. `desktopCloseVisible` doc comment updated (the three toggle menus now carry the X on desktop too). Keep `gridButtonMenuIDs` mutual exclusion.
- [ ] Screenshots of all four, desktop + mobile; inspect.

### Task B3: Trigger menus — kebab context, nav, instrument, subdiv, action menus, confirm dialogs, FX header

**Files:** `drumview_context_menu.go`, `drumview_overlay_nav_comp.go`, `drumview_overlay_inst_comp.go`, `drumview_overlay_subdiv_comp.go`, `action_menu_comp.go`, `confirm_dialog.go`, `drumview_fx_panel.go`, `drumview_portal_open.go`, `action_menu_open.go`.

- [ ] Extend `TestEveryMenuClosesTheSameWay` rows for each (confirm dialogs: X = cancel, Esc = cancel, outside tap does NOT close — `mustAnswer`).
- [ ] Migrate frames: context menu (Swatch = row color, Accent = row color), nav (IconTarget), instrument menu (Swatch = current instrument, search field stays first in body), subdiv/action menus (Title from trigger label, no X on mobile per policy), confirm (Footer = Cancel/Confirm keycaps, `mustAnswer`), FX panel header (Swatch = row color; body unchanged). Portal entries through `menuCardEntry`.
- [ ] Screenshots: `context_menu_open`, `nav_menu_open`, `instrument_menu_open`, `playback_subdiv_menu_open`, `paste_confirm_dialog`, `fx_panel_with_3_effects` desktop+mobile; inspect.

### Task B4: GridTree layers — node sidebar + group menu (chrome only)

**Files:** `game_node_sidebar.go`, `game_group_menu.go` + their tests.

- [ ] Tests: header band/title/X geometry comes from a `MenuCard` (sidebar keeps grip + coords line inside `card.Body()`), group menu Save is a footer keycap (`Button` with `ComponentButtonSecondary`, toggled/disabled per dirty state), both still register through GridTree/inputDispatcher (no portal) — assert no `portal.Has` for them and existing placement tests green.
- [ ] Implement; screenshots `node_sidebar_open`, `group_menu_open` desktop+mobile.

### Task B5: Discipline test, docs, sweep

- [ ] `TestEveryPortalMenuUsesMenuCard` (`menu_card_discipline_test.go`): AST-scan non-test files for `PortalEntry{` literals → collect overlay type names via the `Overlay:` field expression (`&Type{`, `NewType(`, `compPortalOverlay{comp: dv.X}` → X's type); each type must have a `card *MenuCard`/embedded `MenuCard` field or be in `menuCardExempt` (tooltips `chain-tt`/`levels-agg-tt`, wheel popups, `volume-popup`, `naming`, `rename`, FX panel body). Failing list names the type and its open site.
- [ ] DESIGN.md "Menu anatomy": slot 0 = `MenuCard` frame; CLAUDE.md: kit + discipline test + presets pointer; memory.
- [ ] Sweep: capture every menu scene under `vice-city` and `ocean` (BEATMO_DEMO_CONFIG with `"theme":{"preset":"ocean"}`), desktop + mobile; inspect; then full `internal/ui` suite, builds (desktop + wasm), `make gen-design-tokens` idempotence, bridge smoke test.
