# Menu consistency: FX + Template list menus → instrument-menu anatomy

**Date:** 2026-07-22
**Status:** Design approved (pending spec review)
**Owner:** UI / internal/ui

## Problem

Beatmo's list menus are meant to share one visual anatomy (DESIGN.md
"Menu anatomy", five slots: Title band → Search → Group headers → Rows →
Empty state), rendered through shared helpers (`drawMenuRow`/`MenuRowSpec`,
`drawMenuSearchBox`, `drawMenuGroupHeader`, `drawMenuHeaderBandAccent`). Two
menus drift from the gold-standard **instrument selector**:

1. **FX add-effect picker** (`drumview_fx_panel.go`, `fxAddMenuOpen` state) is
   the outlier: centered `InstButtonStyle` **keycap** buttons, label-only, with
   **no search, no leading icons, no hover/active state, no category grouping,
   no trailing caption, no empty state**. It does not use `drawMenuRow` at all.
2. **Template menu** (`drumview_context_menu.go`, page-1 of the File/overflow
   menu) already routes rows through `drawMenuRow` and is ~90% consistent; it
   drifts only in small cosmetic/behavioral details and lacks a paginator.

Separately, the **logs pop-up** (`logs_component.go`) has a visible **footer
pager** ("‹ Page X / M ›") that FX + template should reuse for long lists,
instead of inventing a third pagination style.

The user's goals, in their words: make FX and template "comply to the
aesthetics and style and everything from the main instrument menu (it is
cleaner, nicer, simpler)"; "re-use infrastructure as much as possible to avoid
behavioral or stylistic drift"; and ensure "the shared UI components can adapt
to multiple menu widths, sizing, styles."

## Goals

- FX add-effect picker reaches **full anatomy parity** with the instrument
  menu (flat rows, search, category group headers, trailing caption, empty
  state, active/hover states as applicable).
- Template menu reaches **pixel parity** on rows/search/active-cues + gains the
  shared footer pager.
- **One** paginator implementation shared by logs + FX + template (extracted
  from logs), guaranteeing no style/behavior drift.
- All shared helpers are **geometry-driven** and adapt to every consuming
  menu's width, height, density, and platform (desktop / mobile bottom-sheet).
- Every visual change verified by rendered screenshots (desktop + `MOBILE=1`)
  per the mandatory UI loop.

## Non-goals

- **Do not** restructure navigation: templates stay reachable via
  File → Load template (page-1 of the overflow menu); the FX picker stays a
  sub-page of the FX panel. (User chose "light polish"/"keep as-is" over
  "standalone menu.")
- **Do not** modify the instrument menu's row rendering — it is the reference.
- **Do not** retrofit the instrument menu's own (derived, invisible) page model
  onto the shared footer pager in this change. Flagged as optional future
  unification only; called out in "Future work" so it is a conscious deferral,
  not an oversight.
- No new rendering primitives beyond the pager extraction; reuse existing
  shared helpers.

## Current state (verified by exploration + screenshots)

### Shared row/menu infra (already exists — reuse)
- `menu_chrome.go`: `drawMenuRow(dst, *Button, MenuRowSpec)` + `MenuRowSpec`
  (Accent, State rest/hover/active, Swatch, IconID, IconTint, Highlights,
  Label, LabelColor, LabelRole, CenterLabel, TrailLabel, TrailIconID,
  TrailTint); `drawMenuItemBackgroundAccent`, `drawMenuGroupHeader`,
  `drawMenuHeaderBandAccent`, `drawMenuTitle`, `menuItemState`.
- `menu_search_field.go`: `drawMenuSearchBox`, `ensureMenuSearchBox`,
  `MenuSearcher` (fuzzy).
- `menu_scroll.go`: `MenuScroll` (shared scroll/momentum/deferred-tap engine).
- `logs_model.go`: generic `pageCount(total, pageSize) int` and
  `pageSlice[T any](items, pageSize, page) []T` — **already shared**, not
  logs-specific.

### Instrument menu (gold standard) — `drumview_overlay_inst_comp.go`
`InstrumentMenuComponent`, portal-mounted with scrim. Rows via `drawMenuRow`
(swatch = instrument color; active = current instrument = tint + left stripe;
hover tint; fuzzy highlights; category as trailing caption in global-search).
Search box, breadcrumb strip, category drill-down (chevron + count), favorite
stars. Width floored 260px; mobile bottom-sheet. Has its own derived page model
(`Page`/`PageCount`/`PageSize`/`jumpToPage`) computed from scroll state; **no
visible footer pager**.

### Template menu — `drumview_context_menu.go`
Page-1 of the overflow/File menu (`overflowPage == 1`). Already uses
`drawMenuRow` (leading `IconTemplate`, fuzzy highlights, `TrailLabel` = BPM,
active = currently-loaded template), `MenuScroll`, `drawMenuSearchBox`,
`drawMenuGroupHeader` (genre groups, suppressed when searching), a "Back" row
(chevron-left) to page-0. Title via `drawCardTitleIn` (panel-title 21px, it is
a File *card*). Width baseline 160px, grows to fit widest label, capped to pane.
No footer pager.

### FX add-effect picker — `drumview_fx_panel.go`
`fxAddMenuOpen` renders the effect-type list inline in the FX panel (not a
separate portal). Rows are bare `InstButtonStyle` keycap buttons created in
`buildFXPanel` (~lines 658-696), text-only, centered, drawn via `btn.Draw`. A
`Cancel` keycap row returns to the chain view. Header title `FX: <row>` (colon)
already carries an instrument swatch dot + close ×. Scrolls via `MenuScroll`
(pixel mode). Effects come from `audio.EffectRegistrations()` /
`audio.EffectTypeOrder()`; each `EffectRegistration` has `DisplayName` and
`Category` ∈ {creative, modulation, dynamics, utility}. 18 effects total.

### Logs pager (to extract) — `logs_component.go`
`drawFooter` paints `i18n.Tf(KeyLogsPage, page, pageCount)` centered, with
`prevBtn`/`nextBtn` (`NewSpecButton("", ComponentButtonSecondary, …)` +
`IconChevronLeft`/`IconChevronRight`) at `prevRect`/`nextRect`. `logsFooterH()`
= `TextHeight() + SpaceMD` reserves the band. `pagePrev`/`pageNext` change page
+ `scrollToTop`. Fixed `logsPageSize = 50`, scroll-within-page via `MenuScroll`.

## Design

### A. Shared footer pager (extract from logs)

Add a new `menu_pager.go` with a **width/geometry-driven** pager the three
menus share:

```go
// menuPagerFooterH returns the reserved footer band height (density-aware).
func menuPagerFooterH() int  // == StyledTextHeight(RoleBody)+SpaceMD equivalent, via tokens

// drawMenuPagerFooter draws "‹ Page p / pc ›" centered inside footerRect and
// positions/draws the caller-owned prev/next buttons, recording their rects.
// footerRect drives ALL geometry (center x, button placement) so the pager
// adapts to any menu width. Buttons are ComponentButtonSecondary chevrons.
func drawMenuPagerFooter(dst *ebiten.Image, footerRect image.Rectangle,
    page, pageCount int, prev, next *Button) (prevRect, nextRect image.Rectangle)
```

- **Adaptability requirement (explicit):** every dimension is derived from
  `footerRect` (passed by the caller from its own panel width) and design
  tokens (`SpaceMD/LG/XS`, `RoleBody`, density button size), never a hardcoded
  logs width. The center label centers on `footerRect`; buttons flank it and
  are clamped inside `footerRect`. Verified at ≥3 widths (FX ~340, template
  ~160–pane, logs wide) + mobile bottom-sheet + all densities.
- Button size uses the same density-aware source logs uses (`logsBtnSize()` →
  generalize to a shared `menuPagerBtnSize()` reading the same token) so touch
  targets stay ≥44px on mobile.
- i18n: **Decision (revised during planning): reuse `KeyLogsPage`** ("Page %d /
  %d", already in en + es) verbatim for all three menus — zero new keys, lowest
  i18n drift, aligned with the "reuse infrastructure" directive. The pager
  helper is key-agnostic (caller passes the pre-formatted label), so the
  logs-flavored key name is an internal detail, not a coupling. A later rename
  to a neutral `KeyMenuPage` is a trivial follow-up if desired.
- **Logs refactor:** `logs_component.go` `drawFooter` becomes a thin call into
  `drawMenuPagerFooter` (owning its own `prevBtn`/`nextBtn`), so logs stays
  byte-identical (guarded by the existing `logs_panel_page2` scene +
  `logs_panel_cleared` "1/1").

### B. Page-size model (shared behavior)

Reuse `pageCount`/`pageSlice` verbatim. Page size is **viewport-fit** for the
anchored dropdown menus (FX, template): `pageSize = max(1, floor(listViewportH
/ rowH))`, recomputed on (re)build. Rationale: the footer pager engages exactly
when content overflows the panel and never shows an awkward "1 / 1" on a short
list; a page == one screenful, so there is no confusing scroll-within-page in a
small dropdown. `MenuScroll` stays wired (mobile momentum, partial last page),
but a full page fills the viewport. Logs keeps its fixed `logsPageSize = 50`
(its own constant) — the shared pager renders both models identically because
it is driven purely by `(page, pageCount)`.

Footer visibility: the footer band is reserved and drawn whenever
`pageCount > 1`; when `pageCount == 1` the menu omits the band (no wasted
chrome on short lists). Logs is unaffected (it always paginates its 50-row
pages and already shows "1 / 1"; keep logs' current always-shown behavior to
avoid changing it).

### C. FX add-effect picker — full parity

In `buildFXPanel` (add-menu branch) + `drawFXPanel`:

1. **Rows** → build `*Button`s laid out for `drawMenuRow` (like
   `drawInstRow`), not centered keycaps. `MenuRowSpec`:
   - `IconID: IconFx`, `IconTint: colMenuIcon` (semantic icon per anatomy — no
     per-effect icons exist; category conveyed by group header).
   - `State`: rest / hover only. The picker **appends** an effect (no "current
     selection"), so there is no `menuItemActive` — consistent with the
     anatomy's "lists that have a notion of current."
   - `Label`: `DisplayName`.
   - `TrailLabel`: **category name only while a search query is active**
     (group headers hidden then), mirroring the instrument menu's global-search
     category caption. Idle → no category trail (the group header carries it).
   - Improvement **A**: when idle and the row's chain already contains that
     effect type, `TrailLabel = "×N"` (count of existing instances in the
     row's chain). Idle-only so it never collides with the search-time
     category caption.
2. **Search field**: `ensureMenuSearchBox` + `drawMenuSearchBox`, fuzzy via
   `MenuSearcher` over effect `DisplayName`. Improvement **C**: auto-focus the
   box when the add-menu opens.
3. **Group headers**: `drawMenuGroupHeader` per category (uppercase
   CREATIVE/MODULATION/DYNAMICS/UTILITY) when idle; suppressed + flat
   score-sorted when searching. Category order fixed
   (creative, modulation, dynamics, utility) for determinism.
4. **Footer pager** (§A/§B) over the effect list.
5. **Empty state**: centered `body` + muted `caption` ("No effects match")
   when a search yields nothing.
6. **Title**: `FX · <row>` (middot, replacing colon), keep the swatch dot +
   close ×.
7. **Back affordance**: replace the bespoke `Cancel` keycap with the shared
   "Back" row pattern the template menu uses (chevron-left leading icon,
   `drawMenuRow` with `IconChevronLeft`), returning to the chain view. Title ×
   still closes the whole FX panel.
8. Improvement **B** (Esc): Esc clears a non-empty query first; if the query is
   empty, Esc returns to the chain view (equivalent to Back). Panel-level close
   unchanged.
9. Behavior preserved: selecting a row calls
   `audio.AddInsertEffect(instID, et, nil)` then returns to the chain view
   (`fxAddMenuOpen = false`, rebuild).

### D. Template menu — light polish + pager

In `drumview_context_menu.go` (overflow page-1):

1. Audit + tighten row insets, search-field, group-header, and active-stripe
   geometry to pixel-match the instrument menu's use of the shared helpers.
   (Rows already go through `drawMenuRow`; this is verification + minor
   token/inset alignment, not a rewrite.)
2. Adopt the shared **footer pager** (§A/§B) so long template lists page
   consistently with logs/FX. Reuse `pageSlice` over the template item list;
   pinned band + search stay above the paged list; group headers page with the
   list.
3. Improvement **B** (Esc): confirm Esc clears a non-empty query first, then
   backs out one level (to page-0), then closes — unify with FX + logs.
4. Title role: keep panel-title (21px) — it is legitimately a File *card* per
   the documented hierarchy (grid-button cards use `cardTitleRole`). No change.

## Component boundaries

- **`menu_pager.go` (new)** — pure geometry + draw; depends only on `Button`,
  design tokens, i18n. Inputs: `footerRect`, `(page, pageCount)`, two buttons.
  No knowledge of logs/FX/template. Testable in isolation at multiple widths.
- **`logs_component.go`** — owns its page state + buttons; delegates footer draw
  to `menu_pager.go`. Unchanged externally.
- **`drumview_fx_panel.go`** — owns FX add-menu state (query, page, buttons);
  builds `MenuRowSpec`s + calls shared row/search/group/pager helpers.
- **`drumview_context_menu.go`** — owns template page state; adds pager.
- **`logs_model.go`** — keeps generic `pageCount`/`pageSlice`.

Each unit answers: *what does it do* (render one adaptable footer pager / one
menu's rows), *how do you use it* (pass geometry + state), *what it depends on*
(tokens + Button; not each other).

## Error / edge handling

- `pageCount == 0` / empty list → clamp to 1 page, show empty state, hide/neutralize
  pager (prev/next disabled).
- Filter/search change → snap to page 1 + scroll top (logs' `reapply`
  semantics) so results are always visible from the top.
- Narrow width → footer label + buttons must not overlap; clamp button rects
  inside `footerRect`, and if the label + buttons cannot fit, the label
  truncates before the buttons are pushed out (guard at min FX/template width).
- Mobile bottom-sheet → footer band height + button size use touch-min density;
  pager sits above the sheet's safe-area inset.
- Adding an effect while paginated → return to chain view (page state reset on
  next open), no stale page index.

## Testing

### Go fast-path (`-tags test`)
- **New** `menu_pager_test.go`: `pageCount`/`pageSlice` edge cases (already
  partly covered via logs) + `drawMenuPagerFooter` geometry at ≥3 widths
  (FX/template/logs) asserting prev/next rects stay inside `footerRect` and do
  not overlap the centered label; density variants (compact/comfortable/
  spacious) for button size + band height.
- FX: extend `drumview_fx_panel_test.go` — add-menu builds `drawMenuRow` rows
  (not keycaps), search filters, group headers present idle / absent when
  searching, category trail only when searching, `×N` instance count idle,
  empty state, page slicing, Back row returns to chain, select appends effect.
- Template: extend `drumview_context_menu_test.go` — pager present when
  `pageCount>1`, absent at 1 page, Esc semantics, active-cue parity.
- Logs: existing tests + `logs_panel_page2` must stay green (byte-identical
  after extraction).
- Discipline: `TestDrawMenuRowIsFlatNoKeycap` still holds for the FX rows;
  token discipline (`TestTokenDiscipline`) — no new color literals.

### Screenshot verification (mandatory UI loop)
Scenes (desktop + `MOBILE=1`):
- `fx_add_effect_menu` (already added) — restyled: flat rows, search, group
  headers, footer pager.
- New `fx_add_effect_menu_search` — query active: flat score-sorted, category
  trail, headers hidden, empty-state variant.
- New `fx_add_effect_menu_page2` (or reuse a long-list state) — footer pager
  engaged.
- `template_menu_open` — verify polish + pager (extend list to force >1 page
  or add `template_menu_page2`).
- `logs_panel_page2` / `logs_panel_cleared` — unchanged (regression).
Open each PNG and confirm: text legible, no blank controls, no overlap, correct
active/hover states, pager centered and inside the panel at every width.

## Design-system compliance (DESIGN.md)

- Follows "Menu anatomy" five slots for FX (title/search/groups/rows/empty).
- FX title accent hairline uses the row's instrument color (identity menu);
  template + logs use `colAccent` (global). Never borrow another surface's
  accent.
- Leading slot: swatch when the row IS an instrument, else semantic icon
  (`IconFx`) — FX rows use the icon.
- Icons via `IconID` (`IconFx`, `IconChevronLeft/Right`) — no raw glyphs.
- Colors via `Token*()`/`colXxx` only; no `color.RGBA{}` literals in
  `internal/ui/` outside the exempt files.
- Footer pager buttons = `ComponentButtonSecondary` (same as logs).
- Density/width/runtime kept orthogonal: sizing from `Profile().DensityValues`,
  layout from geometry passed in — no new `runtime.GOOS=="js"` checks.

## Rollout / sequencing (for the plan)

1. Extract `menu_pager.go`; refactor logs to use it; add `KeyMenuPage`; green
   logs tests + screenshot.
2. FX add-menu: rows→`drawMenuRow`, search, groups, empty state, title, Back;
   screenshots.
3. FX improvements A/B/C + footer pager; screenshots (search + page2).
4. Template: polish + footer pager + Esc; screenshots.
5. Full fast-path + browser bridge smoke (no new JS exports expected; if an FX
   page-state export is added, register in `wasm_bridge_smoke` catalogue per
   the JS charter).

## Future work (explicit deferrals)

- Optionally unify the instrument menu's derived page model onto the shared
  footer pager for total consistency. Deferred to avoid risk to the gold
  standard in this change.
