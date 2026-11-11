# Theme presets + MenuCard menu unification

**Date:** 2026-09-06
**Status:** approved design (brainstorm), plan to follow
**Builds on:** `2026-09-05-theme-editor-design.md` (runtime themes) and the
DESIGN.md "Menu anatomy" / "Panel & overlay components" sections.

Two sub-projects, built in order. Both obey two standing rules from the
owner: **reuse the existing primitives** (never a second copy of a style or
a behavior) and **preserve component-tree isolation** (every overlay stays a
portal entry that publishes its own `HitArea`s; no zone reaches into another
zone's fields).

---

## Part A — First-party theme presets

### A.1 Model

- `theme.Doc` gains `Preset string \`json:"preset,omitempty"\`` (preset id).
- Resolution order: shipped defaults → the preset's category swatches →
  the document's own `categories` → `tokens`. Unknown preset id → WARN, ignored.
- `theme_presets.go` (ui) is the single table: `{ID, NameKey, GlossKey, Categories map[string]string}`.
  Guard: every preset names all nine categories with valid swatch names;
  `vice-city` resolves to the shipped defaults (identity) except the deliberate
  refinement below; ids are unique; `TestThemePresetsResolve`.
- Presets (nine categories each: background, surfaces, text, accent, focus, grid, nodes, dividers, viz):

| id | name | picks |
|---|---|---|
| `vice-city` | Vice City | orchid-500, orchid-500, orchid-100, sunset-gold-300, **hot-pink-400**, orchid-500, orchid-500, orchid-100, sunset-gold-300 |
| `ocean` | Ocean | electric-blue-500, electric-blue-400, cyan-100, tangerine-300, hot-pink-300, electric-blue-300, cyan-400, cyan-200, cyan-200 |
| `midnight` | Midnight | electric-blue-500, electric-blue-500, cyan-100, cyan-300, hot-pink-300, electric-blue-400, cyan-300, cyan-300, cyan-200 |
| `ember` | Ember | tangerine-500, tangerine-500, sunset-gold-100, tangerine-300, hot-pink-300, tangerine-400, sunset-gold-400, tangerine-200, sunset-gold-300 |
| `neon-mint` | Neon Mint | mint-500, mint-500, mint-100, mint-300, hot-pink-300, mint-400, cyan-300, mint-200, cyan-200 |
| `orchid` | Orchid | orchid-500, orchid-400, orchid-100, orchid-300, hot-pink-300, orchid-400, cyan-300, orchid-200, cyan-200 |

The only Vice City change versus the shipped palette is the focus ring:
hot-pink-400 (#EE00DD, the palette spec's "Vice City signature") instead of
hot-pink-300. DESIGN.md's `focus-ring` token is NOT edited — the preset applies
it — so drift/discipline tests keep comparing against the YAML.

Every preset is verified by the screenshot sweep (Part C); a preset whose
text/background contrast reads poorly is retuned before it ships.

### A.2 Precedence and persistence

Unchanged from the theme-editor spec: user doc vs circuit doc per category,
override toggle. `Preset` is merged with the same rule as a category (user's
preset wins when override is on, else circuit's, else user's). Persisted in
prefs and in `beatmo.json` through the existing `theme` blocks (additive field).

### A.3 Editor

The Theme overlay keeps its scope pills and override toggle, then shows:

1. **Presets list** — one `drawMenuRow` per preset: leading element a
   five-swatch strip (background, surfaces, accent, viz, text) drawn as small
   rounded squares, label = preset name, `TrailLabel` = gloss, `State` active
   for the effective preset of the active scope (stripe + tint, the shared row
   convention). Picking a row sets `Preset` on the active scope and clears
   that scope's categories/tokens. Rows use the shared `MenuScroll` when the
   panel is short.
2. **"Adjust colors" expander** — a `drawMenuChevron` section header
   (collapsed by default) revealing the existing nine chip rows unchanged.
   When any chip is set the active preset row's `TrailLabel` reads
   "customized" (i18n).
3. Footer: Reset all (two-tap destructive) as today.

The overlay itself becomes a `MenuCard` (Part B) so its frame is shared.

---

## Part B — MenuCard: one frame, one behavior for every floating menu

### B.1 Reference aesthetic

The swatch color picker (`drumview_overlay_color_comp.go`): `drawPanel`
surface, a title band = optional leading chip + title at `menuTitleRole` over
`drawMenuHeaderBandAccent` (accent hairline underline), close X keycap at the
top-right, a muted caption band under the title (its "Light … Deep" hint),
then the content. This is the frame every menu gets.

### B.2 The kit (`menu_card.go`)

`MenuCard` is a small value type composed ONLY from existing primitives —
it introduces no new drawing code paths:

```go
type MenuCardSpec struct {
    TitleKey  i18n.Key        // or Title string (already-localized, e.g. an instrument name)
    Title     string
    Swatch    color.Color     // leading chip for identity menus (instrument/group); nil → none
    IconID    IconID          // leading icon for global menus; "" → none
    Accent    color.Color     // hairline + row accent; nil → colAccent
    CaptionKey i18n.Key       // optional one-line hint band; "" → none
    Footer    []*Button       // optional keycap actions, laid out right-to-left
    Scrollable bool           // body scrolls via MenuScroll (rowH from the body)
}
type MenuCard struct { /* rects computed by Layout: panel, title, caption, body, footer */ }
func (c *MenuCard) Layout(panel image.Rectangle, spec MenuCardSpec)   // computes bands from shared paddings
func (c *MenuCard) Draw(dst *ebiten.Image, spec MenuCardSpec)         // drawPanel + drawMenuHeaderBandAccent + drawMenuTitle (chip) / DrawIcon + caption + footer buttons
func (c *MenuCard) HitAreas(z int, tag string, panel HitHandler) []HitArea // close X + footer buttons via buttonHitAdapter, then the catch-all LAST
func (c *MenuCard) CloseBtn() *Button                                 // CloseButton() from ui_style.go, OnClick wired by the owner
func (c *MenuCard) Body() image.Rectangle
```

- Paddings/roles: `cardContentPadX`, `SpaceMD/SM/XS`, `menuTitleRole`,
  `RoleCaption`, `RadiusContainer` via `drawPanel` — the existing tokens.
- Mobile: the same card; `Profile().UseBottomSheet` menus keep their
  bottom-sheet rect (drag handle via `drawSheetDragHandle`) but the title
  band, X and rows are the card's.
- The kit never reads Game/DrumView state; owners pass everything in.

### B.3 Behavior contract (`menuCardEntry`)

One helper builds the portal entry so open/close semantics cannot drift:

```go
func menuCardEntry(id string, ov PortalOverlay, anchor image.Rectangle, opts menuCardOpts) PortalEntry
// Modal: false, Scrim: true for menus; Modal: true only for opts.MustAnswer
// (confirm dialogs, value-wheel popups). OnClose forwarded.
```

- Confirm dialogs stay non-modal (an outside tap cancels, as today); `mustAnswer` is used for a picker stacked on an open menu and for value-wheel popups.
- Esc closes the top overlay (universal handler, unchanged).
- Close X closes its own overlay (`portal.Close(id)`).
- Outside tap closes only the top overlay (tree `CloseTop`, unchanged).
- Lists: `MenuScroll` for overflow, `MenuKeyNav` for arrow/Enter/Home/End.
- Discipline: `TestEveryPortalMenuUsesMenuCard` enumerates every overlay
  type registered through `portal.Open` in production code (AST scan of
  `PortalEntry{ID:` sites + the overlay types) and asserts each embeds
  `MenuCard` or is on the explicit exemption list (tooltips, the FX docked
  panel body, the value-wheel popups).

### B.4 Migration (each keeps its own type, body and input handlers)

| Menu | Today | Change |
|---|---|---|
| Swatch picker | reference | adopt `MenuCard` (no visual change; proves the kit reproduces it) |
| Theme overlay | card title, no X, modal | MenuCard, icon IconPalette, X, non-modal+scrim |
| Settings | card title, no X, modal | MenuCard, icon IconSettings, X, non-modal+scrim |
| Logs | card title, no X, modal | MenuCard, icon IconLogs, X, non-modal+scrim; keeps search/pills/pager in body |
| File (overflow) | title, no X | MenuCard, icon IconFolder, X |
| Kebab context menu | band + chip + X | MenuCard (chip = row color) |
| Nav (beat/time) menu | icon title + X | MenuCard, icon IconTarget |
| Group menu (GridTree layer) | band + chip + × + Save | unchanged: already the same band/title/× primitives; stays a GridTree layer |
| Node sidebar (GridTree layer) | band + chip + grip + × | unchanged: already the same band/title/× primitives; stays a GridTree layer |
| Instrument menu | band + X + search | EXEMPT: its breadcrumb strip IS the title band (same hairline/× primitives); a plain title would lose category navigation |
| Subdiv menu / action menus | bare panel rows | EXEMPT: dropdown option lists anchored to their control (selects, not menus) |
| Confirm dialogs | drawMenuTitle + X | MenuCard, `MustAnswer` (modal), footer = Cancel/Confirm keycaps |
| FX panel | band + chip + X | MenuCard header (chip = row color); body unchanged |
| Node long-press quick-action bar | toolbar | unchanged (not a menu) |

Placement helpers (`gridAnchoredPopupRect`, `anchoredPopupRect`) are unchanged.

---

## Part C — Verification

- Unit: preset table guards; resolution order; editor pick clears
  categories/tokens; `MenuCard.Layout` band geometry (title/caption/body/
  footer never overlap, X inside the title band, body inside the panel);
  `menuCardEntry` flag table.
- Real input (through `Game.Update`) for EVERY migrated menu: open from its
  trigger → X closes; open → Esc closes; open → outside tap closes only the
  top; a list menu's arrows move the key cursor. One table-driven test with a
  per-menu opener/locator so adding a menu is one row.
- Existing per-menu tests stay green (they pin behaviors the kit must keep).
- Screenshot sweep (inspected): every menu on desktop and mobile under
  `vice-city` and `ocean`, plus the six preset rows in the Theme overlay.
- Docs: DESIGN.md "Menu anatomy" updated to name `MenuCard` as the frame;
  CLAUDE.md gets the discipline test and the preset table pointer.

## Visual audit (2026-09-06)

Every preset was selected through the editor (real row press, `theme_pick_<id>`
scenes) and compared pixel-for-pixel against booting with the same preset
(`TestRuntimeThemePickMatchesStartup`, 6 presets × 15 desktop + 4 mobile
scenes). Fixed: grid subdivision line styles captured their token at
construction (`gridLineColorForDiv` + `Grid.RefreshLineColors`), the add-row
strip sprite cache carried no color key (`invalidateAddRowBtnCache`), and every
`ScrollbarStyle` copy kept alpha colors `liveCaptured` cannot map (the style now
carries a `live` palette tag). Palette tweaks from the audit: the timeline
ruler window and playhead tokens moved from Visualizers to Accent (Ocean showed
a mint ruler beside tangerine controls); Ember's canvases use sunset-gold-300
instead of the lemon -200; Ocean's canvases use cyan-200 so each preset carries
an accent hue and one data hue, never three.
