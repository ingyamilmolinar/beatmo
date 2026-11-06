# Template Menu Fuzzy Search — Design

**Date:** 2026-07-14
**Status:** Approved (design); implementation pending
**Author:** pairing session

## Goal

Add fuzzy search to the **template menu** (page 1 of the drum view's overflow
menu, `drumview_context_menu.go`), reusing the **instrument menu's** fuzzy
search engine, styling, and behavior. The user requirement is explicit:

> Re-use the fuzzy search from the instrument menu, style it the same way,
> handle search the same way, and re-use as much style/behavior code as possible
> (shared components).

The template menu today is page 1 of the overflow menu: a flat, uniformly
scrolling list of `assets.Templates()` (31 rows) preceded by a `Templates`
header and a `Back` row. There is no search of any kind today.

## Non-goals

- No change to the instrument menu's search **behavior** (a pure, behavior-
  preserving extraction of its search-box helpers is in scope; see §3).
- No change to the template data model (`assets.Template`).
- No search on page 0 (File actions). Page 0 only gains pinned-header layout as
  a side effect of the generic chrome-band change (§4), which is a strict,
  low-risk improvement.

## Decisions (from brainstorming)

1. **Pinned search box** (not scrolling): the `Templates` header + search box
   form a fixed top "chrome band"; only the results (Back + template rows)
   scroll beneath — exact parity with the instrument menu.
2. **Search matches both** the display name (`Label`) and the genre key
   (`Key`), same as the instrument menu (`MenuSearcher` already does this;
   label matches produce highlights, key matches are a scoring fallback).
3. **Maximize shared-component reuse**: extract the instrument menu's search-box
   draw + sync into shared free helpers used by *both* menus.

## Reused primitives (already shared — no new logic)

| Primitive | Location | Reuse |
|---|---|---|
| `MenuSearcher.Search(query, []MenuSearchItem) []MenuSearchResult` | `fuzzy_search.go` | Fuzzy filter + score-sort + highlight positions. Fed `{Key: genre, Label: display}`. |
| `MenuRowSpec.Highlights []int` → `drawMenuRow` → `drawTextHighlights` | `menu_chrome.go`, `uigrid.go` | Font-metric-aligned match highlighting. Field already exists on `MenuRowSpec`; the template page simply never populated it. |
| `TextInput` + `BPMBoxStyle` + `MaxLen 40` | `textinput.go`, `theme.go` | Identical search box widget. |
| `i18n.KeyCapSearch` ("Search") placeholder, `RoleBody`, `colTextSecondary` | i18n + theme | Identical muted placeholder hint. |
| `drawMenuHeaderBandAccent` | `menu_chrome.go` | Accent band behind the search row (as the instrument menu does). |
| `emitSearchChanged(surface, query)` | `interaction_events.go` | Layer-B `search` telemetry, surface = `"template-menu"`. |
| Mobile native-input bridge (`mobileInputGetValue`, `NativeRectCandidate`) | `native_gesture*.go` | Same pattern as `inst-search`. |

## §3. Shared search-box helpers (new — extracted from instrument menu)

To honor "re-use as much style/behavior code as possible," extract the two
pieces of the instrument menu's search box that are menu-agnostic into shared
free functions in a new small file `menu_search_field.go`:

```go
// ensureMenuSearchBox lazily creates the shared search TextInput (BPMBoxStyle,
// MaxLen 40) and keeps its Rect synced to the given search-row rect. Mirrors
// InstrumentMenuComponent.ensureSearchBoxSynced (minus the component-state
// text sync, which the caller owns).
func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle) *TextInput

// drawMenuSearchBox draws the search TextInput plus the muted "Search" hint
// when empty & unfocused. Extracted verbatim from
// InstrumentMenuComponent.drawSearchRow so both menus render identically.
func drawMenuSearchBox(dst *ebiten.Image, box *TextInput)
```

The instrument menu's `ensureSearchBoxSynced` and `drawSearchRow` are refactored
to call these helpers (behavior-preserving — the extracted body is identical).
This is the concrete "shared component" the user asked for: one code path draws
both search boxes.

> Rationale for not extracting a full `MenuSearchField` struct: the instrument
> menu's search is entangled with two menu modes (categories/global) and
> favorites, so a full struct rewrite is high-risk in a heavily-tested
> component. Extracting the two pure helpers captures the shared style/behavior
> with near-zero regression surface. Guarded by the instrument menu's existing
> search tests staying green.

## §4. Pinned-chrome scroll restructure

The overflow menu currently draws every item at `curY = popupRect.Min.Y -
scrollOffset` (header included). Introduce a **leading pinned band**:

- A contiguous run of `header` + `search` items at the *top* of `overflowItems()`
  is the pinned band, drawn at fixed Y (no scroll offset), stacked from
  `popupRect.Min.Y`.
- All items after the band scroll: drawn from `popupRect.Min.Y + pinnedH -
  scrollOffset`, clipped to `[popupRect.Min.Y + pinnedH, popupRect.Max.Y]`.
- Scroll range (`configureOverflowScroll`) counts only the scrolling items;
  `ItemView` / `PopupRect` for `MenuScroll` shrink to the region below the band.

This holds for both pages: page 0's `File` header pins (strict improvement,
rarely scrolls); page 1's `Templates` header + search box pin.

Touched functions (all in `drumview_context_menu.go`):

- `overflowItems()` — page 1 emits, in order: `{header: Templates}`,
  `{search: true}`, `{Back}`, then filtered template rows (each carrying
  `highlights`). Reads the memoized filter cache (§5).
- `drawOverflowMenu()` — split into pinned-band draw (header via
  `drawMenuTitle`; search via `drawMenuSearchBox` + `drawMenuHeaderBandAccent`)
  and scrolling-rows draw (`drawMenuRow` with `Highlights`).
- `overflowPopupBtns()` / `rebuildOverflowBtns()` — `search` rows produce no
  button (like `header`); button rects for scrolling rows offset by the band
  height + scroll.
- `configureOverflowScroll()` — visible/count/itemView derived from the
  below-band region and scrolling-item count.
- A `pinnedBandRows()` / `pinnedBandHeight()` helper centralizes the split so
  draw, input, and scroll config agree on geometry (single source of truth,
  matching the existing "compute once" discipline in this file).

## §5. Filtering (memoized)

`overflowItems()` is called several times per frame (draw, input, scroll config,
native-gesture projection). Running `MenuSearcher` inside it each call would
re-fuzz 31 rows many times/frame — the exact per-frame-recompute class of bug
called out in the template-menu-hang memory. Memoize:

```go
// templateSearchResults returns MenuSearcher output for the current query,
// recomputed only when the query changes. Empty query → all templates,
// unhighlighted, in templateOrder.
func (dv *DrumView) templateSearchResults() []MenuSearchResult
```

Cache fields: `templateSearchCache []MenuSearchResult`, `templateSearchCacheQuery
string` (plus a validity flag or a sentinel). Invalidated on query change.

## §6. New state on `DrumView`

```go
templateSearchBox        *TextInput         // BPMBoxStyle, MaxLen 40
templateSearchRect       image.Rectangle    // pinned search-row rect (set during layout)
templateSearchQuery      string             // authoritative query mirror
templateSearchCache      []MenuSearchResult // memoized filter output (§5)
templateSearchCacheQuery string
```

```go
const templateSearchMobileInputID = "template-search"
```

## §7. Input & lifecycle

- **Per-frame poll:** in `dv.Update()`, when `IsOverflowMenuOpen() &&
  overflowPage == 1`, call a new `dv.updateTemplateSearch()`:
  - `ensureMenuSearchBox`, then `templateSearchBox.Update()`.
  - Mobile: read `mobileInputGetValue(templateSearchMobileInputID)` first.
  - On value change: set `templateSearchQuery`, invalidate cache,
    `emitSearchChanged("template-menu", q)`.
  Mirrors the instrument menu's poll block (`drumview_overlay_inst_comp.go`
  ~1601-1621).
- **Focus:** tap inside `templateSearchRect` focuses the box. Intercepted at the
  top of `handleOverflowMenuInput` (before the scroll handler) so it is not
  treated as a row tap. The search rect is pinned, so the hit test is a plain
  `pt.In(rect)`.
- **Escape:** a new page-1 branch clears a non-empty query (keeps menu open);
  empty query falls through to the normal close. Mirrors
  `InstrumentMenuComponent.HandleEscape`.
- **Reset:** `closeOverflowMenu()` and page switches (Back / Load template)
  clear `templateSearchQuery`, the box text, and the cache, so reopening page 1
  starts fresh (matches the existing `overflowPage = 0` reset discipline).

## §8. Mobile native input

Add `template-search` candidates gated on `IsOverflowMenuOpen() && overflowPage
== 1 && templateSearchBox != nil && !templateSearchRect.Empty()`:

- `softKeyboardCandidates()` (desktop browser) — `add("overflow-menu",
  templateSearchMobileInputID, "text", dv.templateSearchRect)`.
- `nativeInputCandidates()` (mobile) — `NativeTextInput` intent, `Text:
  templateSearchBox.Text`, `MaxLen: 40`.

Both mirror the existing `inst-search` entries in `native_gesture.go`.

## §9. Testing (TDD — write failing first)

New `template_menu_search_test.go` (+ a mobile variant), mirroring
`inst_menu_*search*_test.go`:

1. `TestTemplateMenu_SearchFiltersRows` — query "hotel" → only "Eagles — Hotel
   California" among template rows; header/search/Back preserved.
2. `TestTemplateMenu_SearchSortsByScore` — results ordered best-score first.
3. `TestTemplateMenu_SearchHighlightsMatches` — matched row's `highlights` is
   non-empty and indexes the matched runes in the display label.
4. `TestTemplateMenu_EmptyQueryShowsAllInOrder` — empty query → all templates in
   `templateOrder`, no highlights.
5. `TestTemplateMenu_MatchesGenreKey` — query "jazz" matches via key even when
   the display name lacks the substring.
6. `TestTemplateMenu_EscapeClearsQueryKeepsOpen` — Escape with query clears it;
   menu stays open. Escape with empty query closes.
7. `TestTemplateMenu_CloseResetsQuery` — closing (and page switch) clears query +
   box + cache.
8. `TestTemplateMenu_SearchBoxStyledLikeInstMenu` — box uses `BPMBoxStyle`,
   `MaxLen == 40` (same as the instrument search box).
9. `TestTemplateMenu_HeaderAndSearchPinnedUnderScroll` — after scrolling the
   results, the header + search rects stay fixed while template row rects move.
10. `TestTemplateMenu_MobileNativeInputFilters` — feeding
    `template-search` via the mobile bridge filters rows (mirror
    `inst_menu_mobile_search_filter_test.go`).
11. Regression: existing instrument-menu search tests stay green (proves the §3
    extraction is behavior-preserving).

Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.

## §10. Risk & guards

- **Scroll geometry regression** is the main risk (template-menu-hang memory
  warns about this area). Mitigated by the single `pinnedBandHeight()` source of
  truth and test #9. Existing `template_menu_scroll_test.go` /
  `template_menu_width_test.go` must stay green.
- **Per-frame fuzzy recompute** avoided via §5 memoization.
- **Instrument-menu regression** from §3 extraction bounded by keeping the
  existing inst-menu search tests green.
- **Token/design discipline:** reuse existing tokens (`colFuzzyHighlight`,
  `BPMBoxStyle`, `colTextSecondary`); no new color literals, so
  `TestTokenDiscipline` / `TestDesignMDDrift` are untouched.

## Out-of-scope follow-ups

- Full `MenuSearchField` struct unifying every menu search (instrument,
  template, and future menus) — a larger refactor deferred until a third menu
  needs search.
