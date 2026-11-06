# Template Menu Fuzzy Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fuzzy search to the template menu (page 1 of the drum-view overflow menu), reusing the instrument menu's fuzzy engine, styling, and behavior via shared components.

**Architecture:** Extract the instrument menu's search-box draw + sync into shared free helpers (`menu_search_field.go`) used by both menus. The `Templates` header + a search box form a fixed pinned chrome band; results (Back + template rows) scroll beneath. Filtering runs through the existing `MenuSearcher`, is memoized per-query, and highlights matches through the existing `MenuRowSpec.Highlights` → `drawMenuRow` path.

**Tech Stack:** Go 1.26, Ebiten (stubbed under `-tags test`), existing `internal/ui` menu infrastructure.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go` (from `src/go`). Never system `go`.
- Fast test command (run from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
- Reuse existing tokens only (`BPMBoxStyle`, `colFuzzyHighlight`, `colTextSecondary`, `SpaceSM`, `SpaceMD`, `SpaceXS`, `touchMinTargetPx`). Introduce NO new `color.RGBA`/`color.NRGBA` literals (keeps `TestTokenDiscipline`/`TestDesignMDDrift` green).
- The template menu is page 1 of the overflow menu; all its logic lives on `*DrumView`. `overflowItems()` (`drumview_context_menu.go`) is the single item-list choke point and is called several times per frame — never run fuzzy matching directly inside it (memoize).
- Placeholder text is the existing i18n string `i18n.KeyCapSearch` ("Search"). No new i18n keys.
- Search field id for the mobile native-input bridge: the const `templateSearchMobileInputID = "template-search"`.
- Telemetry surface string for `emitSearchChanged`: `"template-menu"`.
- Behavior parity target: the instrument menu's search (`drumview_overlay_inst_comp.go`). Its existing tests (`inst_menu_global_search_test.go`, `inst_menu_search_highlight_render_test.go`, `inst_menu_search_cursor_test.go`, `inst_menu_mobile_search_filter_test.go`) MUST stay green — that is the guard proving the shared-helper extraction is behavior-preserving.

---

### Task 1: Shared search-box helpers (extract from instrument menu)

**Files:**
- Create: `src/go/internal/ui/menu_search_field.go`
- Create: `src/go/internal/ui/menu_search_field_test.go`
- Modify: `src/go/internal/ui/drumview_overlay_inst_comp.go` (`ensureSearchBoxSynced` ~533-542, `drawSearchRow` ~2163-2171)

**Interfaces:**
- Produces:
  - `func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle) *TextInput` — lazily creates a `BPMBoxStyle` / `MaxLen 40` search `TextInput` and syncs its `Rect` to `insetRect(searchRect, SpaceSM)`; returns the box.
  - `func drawMenuSearchBox(dst *ebiten.Image, box *TextInput)` — draws the box plus the muted "Search" hint when empty & unfocused.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/menu_search_field_test.go`:

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestEnsureMenuSearchBox_CreatesStyledBoxAndSyncsRect(t *testing.T) {
	searchRect := image.Rect(10, 20, 210, 64)
	box := ensureMenuSearchBox(nil, searchRect)
	if box == nil {
		t.Fatal("ensureMenuSearchBox returned nil")
	}
	if box.MaxLen != 40 {
		t.Fatalf("MaxLen = %d, want 40 (parity with instrument search box)", box.MaxLen)
	}
	if got, want := box.Rect, insetRect(searchRect, SpaceSM); got != want {
		t.Fatalf("box.Rect = %v, want insetRect(searchRect, SpaceSM) = %v", got, want)
	}
	// Second call reuses the same box and re-syncs the rect.
	newRect := image.Rect(0, 0, 100, 44)
	box2 := ensureMenuSearchBox(box, newRect)
	if box2 != box {
		t.Fatal("ensureMenuSearchBox allocated a new box instead of reusing the existing one")
	}
	if got, want := box2.Rect, insetRect(newRect, SpaceSM); got != want {
		t.Fatalf("box.Rect not re-synced: got %v want %v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEnsureMenuSearchBox`
Expected: FAIL — `undefined: ensureMenuSearchBox`.

- [ ] **Step 3: Create the shared helpers**

Create `src/go/internal/ui/menu_search_field.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ensureMenuSearchBox lazily creates a fuzzy-search TextInput styled exactly
// like the instrument menu's search box (BPMBoxStyle, MaxLen 40) and keeps its
// Rect synced to searchRect (inset by SpaceSM). Returns the box so callers can
// store it. The caller owns the query text; this handles creation + geometry
// only. Shared by the instrument menu and the template/overflow menu so both
// search fields are the same widget with the same styling.
func ensureMenuSearchBox(box *TextInput, searchRect image.Rectangle) *TextInput {
	if box == nil {
		box = NewTextInput(image.Rectangle{}, BPMBoxStyle)
		box.MaxLen = 40
	}
	box.Rect = insetRect(searchRect, SpaceSM)
	return box
}

// drawMenuSearchBox draws the search TextInput plus a muted "Search" hint when
// the box is empty and unfocused. Extracted verbatim from the instrument menu's
// drawSearchRow so both menus render identically.
func drawMenuSearchBox(dst *ebiten.Image, box *TextInput) {
	if box == nil {
		return
	}
	box.Draw(dst)
	if box.Value() == "" && !box.Focused() {
		r := box.Rect
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(dst, i18n.T(i18n.KeyCapSearch), r.Min.X+SpaceXS, ty, RoleBody, colTextSecondary)
	}
}
```

- [ ] **Step 4: Run the new test — verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEnsureMenuSearchBox`
Expected: PASS.

- [ ] **Step 5: Refactor the instrument menu to use the shared helpers (behavior-preserving)**

In `src/go/internal/ui/drumview_overlay_inst_comp.go`, replace the body of `ensureSearchBoxSynced` (currently ~533-542):

```go
func (m *InstrumentMenuComponent) ensureSearchBoxSynced() {
	m.searchBox = ensureMenuSearchBox(m.searchBox, m.searchRect)
	if m.searchBox.Value() != m.state.searchText {
		m.searchBox.SetText(m.state.searchText)
	}
}
```

Replace the body of `drawSearchRow` (currently ~2163-2171):

```go
func (m *InstrumentMenuComponent) drawSearchRow(dst *ebiten.Image) {
	drawMenuSearchBox(dst, m.searchBox)
}
```

- [ ] **Step 6: Run the instrument-menu search tests — verify no regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'InstMenu|inst_menu|Search'`
Expected: PASS (all instrument-menu search + highlight + cursor + mobile tests green).

- [ ] **Step 7: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/menu_search_field.go src/go/internal/ui/menu_search_field_test.go src/go/internal/ui/drumview_overlay_inst_comp.go
git commit -m "refactor(ui): extract shared menu search-box helpers"
```

---

### Task 2: Memoized template filtering + highlighted rows

**Files:**
- Modify: `src/go/internal/ui/drumview.go` (add state fields near the overflow fields ~626-634)
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowItem` struct ~783-789; `overflowItems` page-1 branch ~796-817; add helpers; add const; `drawOverflowMenu` `drawMenuRow` call ~988-994)
- Create: `src/go/internal/ui/template_menu_search_test.go`

**Interfaces:**
- Consumes: `MenuSearcher`, `MenuSearchItem`, `MenuSearchResult` (`fuzzy_search.go`); `assets.Templates()` returns `[]assets.Template{Genre, Display, BPM, Bytes}`.
- Produces:
  - New `DrumView` fields: `templateSearchBox *TextInput`, `templateSearchRect image.Rectangle`, `templateSearchQuery string`, `templateSearchCache []MenuSearchResult`, `templateSearchCacheQuery string`, `templateSearchCacheValid bool`.
  - `const templateSearchMobileInputID = "template-search"`.
  - `func (dv *DrumView) templateSearchResults() []MenuSearchResult` — memoized fuzzy results for the current query (empty query → all templates in order).
  - `func (dv *DrumView) invalidateTemplateSearch()`.
  - `func (dv *DrumView) setTemplateSearchQuery(q string)`.
  - `overflowItem` gains fields `search bool` and `highlights []int`.

- [ ] **Step 1: Write the failing tests**

Create `src/go/internal/ui/template_menu_search_test.go`:

```go
//go:build test

package ui

import (
	"strings"
	"testing"
)

// openTemplatePageForSearch opens the overflow menu on the template page at a
// roomy size and returns the DrumView.
func openTemplatePageForSearch(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(900, 700)
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	return dv
}

// templateRowLabels returns the labels of the template rows currently in the
// overflow item list (excludes header, search, and Back rows).
func templateRowLabels(dv *DrumView) []string {
	var out []string
	for _, it := range dv.overflowItems() {
		if it.header || it.search || it.iconID != IconTemplate {
			continue
		}
		out = append(out, it.label)
	}
	return out
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestTemplateMenu_SearchFiltersRows(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("hotel")

	labels := templateRowLabels(dv)
	if !containsStr(labels, "Eagles — Hotel California") {
		t.Fatalf("query 'hotel' dropped its own template; got %v", labels)
	}
	if containsStr(labels, "Miles Davis — So What") {
		t.Fatalf("query 'hotel' should exclude unrelated templates; got %v", labels)
	}
}

func TestTemplateMenu_EmptyQueryShowsAllInOrder(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("")

	labels := templateRowLabels(dv)
	if len(labels) != 31 {
		t.Fatalf("empty query = %d template rows, want 31 (all)", len(labels))
	}
	if labels[0] != "Bach — Toccata & Fugue in D minor" {
		t.Fatalf("empty query first row = %q, want the first templateOrder entry", labels[0])
	}
}

func TestTemplateMenu_SearchSortsByScore(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("bach")

	labels := templateRowLabels(dv)
	if len(labels) == 0 {
		t.Fatal("query 'bach' matched nothing")
	}
	if !strings.Contains(labels[0], "Bach") {
		t.Fatalf("best 'bach' match should sort first; got first=%q (%v)", labels[0], labels)
	}
}

func TestTemplateMenu_SearchHighlightsMatches(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("hotel")

	var found bool
	for _, it := range dv.overflowItems() {
		if it.iconID != IconTemplate || it.label != "Eagles — Hotel California" {
			continue
		}
		found = true
		if len(it.highlights) == 0 {
			t.Fatalf("matched template has no highlights")
		}
		rs := []rune(it.label)
		for _, idx := range it.highlights {
			if idx < 0 || idx >= len(rs) {
				t.Fatalf("highlight index %d out of range for %q", idx, it.label)
			}
		}
	}
	if !found {
		t.Fatal("Eagles template row not present for query 'hotel'")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_Search`
Expected: FAIL — `dv.setTemplateSearchQuery undefined` and `it.search`/`it.highlights` undefined fields.

- [ ] **Step 3a: Add DrumView state fields**

In `src/go/internal/ui/drumview.go`, immediately after the `overflowPage int` field (~634), add:

```go
	// Template-menu (overflow page 1) fuzzy search. Mirrors the instrument
	// menu's search field; templateSearchCache memoizes MenuSearcher output so
	// overflowItems (called several times per frame) never re-fuzzes per frame.
	templateSearchBox        *TextInput
	templateSearchRect       image.Rectangle
	templateSearchQuery      string
	templateSearchCache      []MenuSearchResult
	templateSearchCacheQuery string
	templateSearchCacheValid bool
```

(If `image` is not already imported in `drumview.go`, it is — the file already uses `image.Rectangle`.)

- [ ] **Step 3b: Add the const, memoized filter, and query setter**

In `src/go/internal/ui/drumview_context_menu.go`, near the top (after the imports / with the other file-level decls), add:

```go
// templateSearchMobileInputID is the mobile native-input bridge id for the
// template-menu (overflow page 1) search field. Parallels instSearchMobileInputID.
const templateSearchMobileInputID = "template-search"
```

Then add these methods (anywhere in the file, e.g. just above `overflowItems`):

```go
// templateSearchResults returns MenuSearcher output for the current template
// query, recomputed only when the query changes. Empty query → all templates in
// templateOrder, unhighlighted. Memoized because overflowItems is called several
// times per frame from draw, input, and scroll config.
func (dv *DrumView) templateSearchResults() []MenuSearchResult {
	q := strings.TrimSpace(dv.templateSearchQuery)
	if dv.templateSearchCacheValid && dv.templateSearchCacheQuery == q {
		return dv.templateSearchCache
	}
	tps := assets.Templates()
	items := make([]MenuSearchItem, 0, len(tps))
	for _, tp := range tps {
		items = append(items, MenuSearchItem{Key: tp.Genre, Label: tp.Display})
	}
	var searcher MenuSearcher
	dv.templateSearchCache = searcher.Search(q, items)
	dv.templateSearchCacheQuery = q
	dv.templateSearchCacheValid = true
	return dv.templateSearchCache
}

// invalidateTemplateSearch drops the memoized filter cache after a query change.
func (dv *DrumView) invalidateTemplateSearch() { dv.templateSearchCacheValid = false }

// setTemplateSearchQuery sets the query programmatically (Back/select reset,
// mobile native input) and pushes it into the box. NOT used on the typing path,
// where the box already holds the text (see updateTemplateSearch) — calling
// SetText there would teleport the caret to the end.
func (dv *DrumView) setTemplateSearchQuery(q string) {
	if dv.templateSearchQuery == q {
		return
	}
	dv.templateSearchQuery = q
	dv.invalidateTemplateSearch()
	if dv.templateSearchBox != nil {
		dv.templateSearchBox.SetText(q)
	}
}
```

Confirm `strings` and `"github.com/ingyamilmolinar/beatmo/internal/assets"` are imported in this file (they are — `assets.Templates()` is already called at ~803).

- [ ] **Step 3c: Extend the `overflowItem` struct**

In `src/go/internal/ui/drumview_context_menu.go`, extend the struct (~783-789):

```go
type overflowItem struct {
	label      string
	iconID     IconID
	active     bool
	header     bool     // true → renders as a non-clickable subgroup header
	search     bool     // true → renders the pinned fuzzy-search box (no button)
	highlights []int    // fuzzy-match highlight rune indices for this row's label
	onClick    func()
}
```

- [ ] **Step 3d: Filter + highlight the template rows in `overflowItems`**

In `src/go/internal/ui/drumview_context_menu.go`, replace the `overflowPage == 1` branch's template loop. The branch (~796-817) becomes:

```go
	if dv.overflowPage == 1 {
		items := []overflowItem{
			{label: i18n.T(i18n.KeyMenuTemplates), header: true},
			{search: true},
			{label: i18n.T(i18n.KeyMenuBack), iconID: IconChevronLeft, onClick: func() {
				dv.setTemplateSearchQuery("")
				dv.overflowPage = 0
			}},
		}
		byGenre := make(map[string]assets.Template, len(assets.Templates()))
		for _, tp := range assets.Templates() {
			byGenre[tp.Genre] = tp
		}
		for _, r := range dv.templateSearchResults() {
			tp, ok := byGenre[r.Key]
			if !ok {
				continue
			}
			tp := tp
			hl := r.Highlights
			items = append(items, overflowItem{
				label:      tp.Display,
				iconID:     IconTemplate,
				highlights: hl,
				onClick: func() {
					dv.setTemplateSearchQuery("")
					dv.overflowPage = 0
					dv.closeOverflowMenu()
					if dv.onImport != nil {
						_ = dv.onImport(tp.Bytes, tp.Display)
					}
				},
			})
		}
		return items
	}
```

- [ ] **Step 3e: Thread highlights into the drawn rows**

In `src/go/internal/ui/drumview_context_menu.go`, in `drawOverflowMenu`'s `drawMenuRow` call (~988-994), add the `Highlights` field:

```go
			drawMenuRow(clip, btn, MenuRowSpec{
				Accent:     dv.contextMenuAccent(),
				State:      state,
				IconID:     item.iconID,
				IconTint:   iconTint,
				Highlights: item.highlights,
				Label:      btn.Text,
			})
```

(The search row is not yet drawn/pinned — Task 3 handles that. For now `{search: true}` is an item with no button; Step 3f prevents it from consuming a button slot so the existing draw/scroll code stays correct.)

- [ ] **Step 3f: Skip `search` rows in button accounting**

In `src/go/internal/ui/drumview_context_menu.go`, three loops currently treat every non-`header` item as a button row. Update each to also skip `search` rows so the un-pinned interim build stays correct:

In `rebuildOverflowBtns` (~874): change `if item.header {` to `if item.header || item.search {`.

In `overflowPopupBtns` (~901 count loop): change `if !item.header {` to `if !item.header && !item.search {`.

In `overflowPopupBtns` (~912 rect loop): change `if item.header {` to `if item.header || item.search {`.

In `drawOverflowMenu` (~967 header check): change `if item.header {` to handle search too — replace with:

```go
		if item.header {
			titleRect := image.Rect(rowR.Min.X+SpaceMD, rowR.Min.Y, rowR.Max.X-SpaceMD, rowR.Max.Y)
			drawMenuTitle(clip, titleRect, item.label, nil)
			continue
		}
		if item.search {
			continue // drawn as a pinned box in Task 3
		}
```

In `configureOverflowScroll` (~1018) the count uses `len(items)` — leave unchanged for now (the extra search row just reserves one blank scrolling slot until Task 3 pins it; harmless and Task-2 tests don't assert geometry).

- [ ] **Step 4: Run the Task-2 tests — verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_Search`
Expected: PASS.

- [ ] **Step 5: Run the existing template-menu tests — verify no regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestOverflowMenu|TestTemplateMenu'`
Expected: PASS (template list, select-queues-import, scroll, width tests green).

- [ ] **Step 6: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/drumview.go src/go/internal/ui/drumview_context_menu.go src/go/internal/ui/template_menu_search_test.go
git commit -m "feat(ui): fuzzy-filter + highlight template menu rows"
```

---

### Task 3: Pinned search-box chrome band + scroll restructure

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowItems` already emits `{search:true}`; add `overflowPinnedCount`; rework `configureOverflowScroll`, `overflowPopupBtns`, `drawOverflowMenu`)
- Modify: `src/go/internal/ui/template_menu_search_test.go` (add pinned-geometry + style tests)

**Interfaces:**
- Consumes: `ensureMenuSearchBox` / `drawMenuSearchBox` (Task 1); `overflowItem.search` (Task 2); `drawMenuHeaderBandAccent(dst, rect, accent)` and `dv.contextMenuAccent()` (existing).
- Produces:
  - `func overflowPinnedCount(items []overflowItem) int` — number of leading `header`/`search` rows (the fixed band).
  - `dv.templateSearchRect` populated by `overflowPopupBtns`; `dv.templateSearchBox` ensured there too.

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/template_menu_search_test.go`:

```go
import (
	"image" // add to the existing import block if not present
)

func TestTemplateMenu_SearchBoxStyledLikeInstMenu(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.overflowPopupBtns(dv.overflowPopupRect()) // populates rect + box

	if dv.templateSearchBox == nil {
		t.Fatal("template search box not created")
	}
	if dv.templateSearchBox.MaxLen != 40 {
		t.Fatalf("MaxLen = %d, want 40 (instrument-search parity)", dv.templateSearchBox.MaxLen)
	}
	if dv.templateSearchRect.Empty() {
		t.Fatal("templateSearchRect not set")
	}
	if got, want := dv.templateSearchBox.Rect, insetRect(dv.templateSearchRect, SpaceSM); got != want {
		t.Fatalf("box.Rect = %v, want insetRect(searchRect, SpaceSM) = %v (shared helper not used)", got, want)
	}
}

func TestTemplateMenu_HeaderAndSearchPinnedUnderScroll(t *testing.T) {
	// Short pane → the template list overflows so scrolling is meaningful.
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(420, 360)
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()

	popup := dv.overflowPopupRect()
	dv.overflowPopupBtns(popup)
	searchBefore := dv.templateSearchRect

	scroll := dv.OverflowMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("template list fits without scrolling at this size")
	}
	rowBefore, ok := lastTemplateBtn(dv)
	if !ok {
		t.Fatal("no last template button")
	}
	// Scroll down a few notches (clicky wheel needs cooldown ticks between).
	for i := 0; i < scroll.ScrollBehavior().VS.Total+4; i++ {
		for j := 0; j < controlGridScrollCooldownFrames; j++ {
			scroll.TickStep()
		}
		scroll.HandleWheel(-1)
	}
	dv.overflowPopupBtns(popup)
	searchAfter := dv.templateSearchRect
	rowAfter, _ := lastTemplateBtn(dv)

	if searchAfter != searchBefore {
		t.Fatalf("search box must stay pinned: before=%v after=%v", searchBefore, searchAfter)
	}
	if rowAfter.Min.Y >= rowBefore.Min.Y {
		t.Fatalf("template rows must scroll under the pinned band: beforeY=%d afterY=%d",
			rowBefore.Min.Y, rowAfter.Min.Y)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu_SearchBoxStyled|TestTemplateMenu_HeaderAndSearchPinned'`
Expected: FAIL — `templateSearchBox`/`templateSearchRect` never populated (box nil, rect empty).

- [ ] **Step 3a: Add the pinned-count helper**

In `src/go/internal/ui/drumview_context_menu.go`, add:

```go
// overflowPinnedCount returns the number of leading pinned rows — a contiguous
// run of header/search items at the top of the current page's item list. These
// render at a fixed Y (chrome band); every item after them scrolls.
func overflowPinnedCount(items []overflowItem) int {
	n := 0
	for _, it := range items {
		if it.header || it.search {
			n++
			continue
		}
		break
	}
	return n
}
```

- [ ] **Step 3b: Scroll only the region below the band**

Replace the body of `configureOverflowScroll` (~1014-1029):

```go
func (dv *DrumView) configureOverflowScroll() {
	if dv.overflowMenuScroll == nil {
		dv.overflowMenuScroll = NewMenuScroll(dropdownScrollbarStyle(), touchMinTargetPx)
	}
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	popupRect := dv.overflowPopupRect()
	bandH := overflowPinnedCount(items) * rowH
	scrollRegion := popupRect
	scrollRegion.Min.Y += bandH
	if scrollRegion.Min.Y > scrollRegion.Max.Y {
		scrollRegion.Min.Y = scrollRegion.Max.Y
	}
	scrollCount := len(items) - overflowPinnedCount(items)
	visible := scrollRegion.Dy() / rowH
	if visible > scrollCount {
		visible = scrollCount
	}
	if visible < 1 {
		visible = 1
	}
	dv.overflowMenuScroll.Configure(scrollRegion, scrollCount, visible)
}
```

- [ ] **Step 3c: Pin the band, offset scrolling rows, ensure the box**

Replace the rect-refresh loop in `overflowPopupBtns` (from `curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx()` through the close-button rect, ~910-927):

```go
	rowH := touchMinTargetPx
	pinned := overflowPinnedCount(items)
	bandH := pinned * rowH
	pinnedY := popupRect.Min.Y
	scrollY := popupRect.Min.Y + bandH - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	seen := 0
	for _, item := range items {
		if seen < pinned {
			r := image.Rect(popupRect.Min.X, pinnedY, popupRect.Max.X, pinnedY+rowH)
			pinnedY += rowH
			seen++
			if item.search {
				dv.templateSearchRect = r
				dv.templateSearchBox = ensureMenuSearchBox(dv.templateSearchBox, r)
				if dv.templateSearchBox.Value() != dv.templateSearchQuery {
					dv.templateSearchBox.SetText(dv.templateSearchQuery)
				}
			}
			continue
		}
		r := image.Rect(popupRect.Min.X, scrollY, popupRect.Max.X, scrollY+rowH)
		scrollY += rowH
		if btnIdx < len(dv.overflowBtns)-1 {
			dv.overflowBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
		}
		btnIdx++
	}
	// Close button rect (last element).
	if len(dv.overflowBtns) > 0 {
		dv.overflowBtns[len(dv.overflowBtns)-1].SetRect(closeButtonRect(popupRect, SpaceXS))
	}
	return dv.overflowBtns
}
```

Note: the `rowH := touchMinTargetPx` and `items := dv.overflowItems()` declarations already exist earlier in `overflowPopupBtns` (the count loop at ~897-905). Keep the existing `rowH`/`items`; remove the duplicate `rowH := touchMinTargetPx` here if the compiler flags a redeclaration — reuse the outer ones.

- [ ] **Step 3d: Draw the pinned band (header via title, search via shared box)**

Replace `drawOverflowMenu`'s draw loop (from `clip := dst.SubImage(popupRect)...` through the row loop, ~953-995) with a pinned-then-scrolling split:

```go
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	pinned := overflowPinnedCount(items)
	bandH := pinned * rowH
	// Refresh button rects + templateSearchRect/box for this frame.
	btns := dv.overflowPopupBtns(popupRect)

	// Scrolling rows are clipped to the region below the pinned band.
	scrollRegion := popupRect
	scrollRegion.Min.Y += bandH
	if scrollRegion.Min.Y > scrollRegion.Max.Y {
		scrollRegion.Min.Y = scrollRegion.Max.Y
	}
	clip := dst.SubImage(scrollRegion).(*ebiten.Image)

	// 1. Pinned band (fixed Y, unclipped chrome).
	pinnedY := popupRect.Min.Y
	for i := 0; i < pinned && i < len(items); i++ {
		item := items[i]
		rowR := image.Rect(popupRect.Min.X, pinnedY, popupRect.Max.X, pinnedY+rowH)
		pinnedY += rowH
		if item.header {
			titleRect := image.Rect(rowR.Min.X+SpaceMD, rowR.Min.Y, rowR.Max.X-SpaceMD, rowR.Max.Y)
			drawMenuTitle(dst, titleRect, item.label, nil)
			continue
		}
		if item.search {
			drawMenuHeaderBandAccent(dst, rowR, dv.contextMenuAccent())
			drawMenuSearchBox(dst, dv.templateSearchBox)
		}
	}

	// 2. Scrolling rows (Back + templates); btns already carry scroll-offset rects.
	btnIdx := 0
	nActionBtns := len(btns) - 1
	for i := pinned; i < len(items); i++ {
		item := items[i]
		if btnIdx >= nActionBtns {
			break
		}
		btn := btns[btnIdx]
		btnIdx++
		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		var iconTint color.Color = colMenuIcon
		if item.active {
			iconTint = colFollowActive
		}
		drawMenuRow(clip, btn, MenuRowSpec{
			Accent:     dv.contextMenuAccent(),
			State:      state,
			IconID:     item.iconID,
			IconTint:   iconTint,
			Highlights: item.highlights,
			Label:      btn.Text,
		})
	}

	// Close button + scrollbar (pinned chrome, unclipped).
	if len(btns) > 0 {
		btns[len(btns)-1].Draw(dst)
	}
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.Draw(dst)
	}
}
```

This removes the old flat `curY`-based loop entirely. Keep the panel/backdrop drawing (`drawBottomSheetPanel`/`drawPanel`) that precedes it unchanged.

- [ ] **Step 4: Run the Task-3 tests — verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu_SearchBoxStyled|TestTemplateMenu_HeaderAndSearchPinned'`
Expected: PASS.

- [ ] **Step 5: Run the full overflow/template + scroll suite — verify no regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestOverflowMenu|TestTemplateMenu'`
Expected: PASS (list, select-import, scroll-to-last, desktop-width tests green — the pinned band changes their geometry but not their assertions).

- [ ] **Step 6: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/drumview_context_menu.go src/go/internal/ui/template_menu_search_test.go
git commit -m "feat(ui): pin template menu header + search box above scroll"
```

---

### Task 4: Input, per-frame poll, escape, and reset

**Files:**
- Modify: `src/go/internal/ui/dv_overlay_portal.go` (add `escapeFn` field + `HandleEscape`)
- Modify: `src/go/internal/ui/drumview_portal_open.go` (`openOverflowMenuPortal` ~156-189: wire `updateFn` + `escapeFn`; `OnClose` reset)
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`handleOverflowMenuInput` ~586: tap-to-focus; `closeOverflowMenu` ~1036: reset; add `updateTemplateSearch` + `handleTemplateSearchEscape`)
- Modify: `src/go/internal/ui/template_menu_search_test.go` (add lifecycle tests)

**Interfaces:**
- Consumes: `portalEscapeHandler` interface (`game_input_shortcuts.go:330` — `HandleEscape() bool`); `dvOverlayPortal.updateFn` (existing); `emitSearchChanged` (`interaction_events.go`); `mobileInputGetValue` (`mobile_input_stub.go`); `Profile().IsMobile()`.
- Produces:
  - `dvOverlayPortal.escapeFn func() bool` + `func (o *dvOverlayPortal) HandleEscape() bool`.
  - `func (dv *DrumView) updateTemplateSearch()`.
  - `func (dv *DrumView) handleTemplateSearchEscape() bool`.

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/template_menu_search_test.go`:

```go
func TestTemplateMenu_EscapeClearsQueryKeepsOpen(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("bach")

	if !dv.handleTemplateSearchEscape() {
		t.Fatal("escape with a non-empty query should be consumed (keep menu open)")
	}
	if dv.templateSearchQuery != "" {
		t.Fatalf("escape did not clear query; got %q", dv.templateSearchQuery)
	}
	if !dv.IsOverflowMenuOpen() {
		t.Fatal("escape cleared the query but wrongly closed the menu")
	}
	// Empty query → not consumed, so the universal handler closes the menu.
	if dv.handleTemplateSearchEscape() {
		t.Fatal("escape with empty query should NOT be consumed")
	}
}

func TestTemplateMenu_CloseResetsQuery(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("hotel")
	dv.closeOverflowMenu()

	if dv.templateSearchQuery != "" {
		t.Fatalf("closing the menu did not reset the query; got %q", dv.templateSearchQuery)
	}
	// Reopen: fresh, unfiltered.
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	if got, want := len(templateRowLabels(dv)), len(assets.Templates()); got != want {
		t.Fatalf("reopened template menu should show all %d rows, got %d (stale query)", want, got)
	}
}

func TestTemplateMenu_TapFocusesSearchBox(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	popup := dv.overflowPopupRect()
	dv.overflowPopupBtns(popup) // populate rect + box

	c := dv.templateSearchRect
	cx, cy := (c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2
	if !dv.handleOverflowMenuInput(cx, cy, true) {
		t.Fatal("press inside the search box was not consumed")
	}
	if !dv.templateSearchBox.Focused() {
		t.Fatal("press inside the search box did not focus it")
	}
}

func TestTemplateMenu_TypingUpdatesQueryAndFilters(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.overflowPopupBtns(dv.overflowPopupRect()) // create the box
	// Simulate a committed edit in the box, then poll (mirrors a typed frame).
	dv.templateSearchBox.SetText("hotel")
	dv.updateTemplateSearch()

	if dv.templateSearchQuery != "hotel" {
		t.Fatalf("poll did not read the box value into the query; got %q", dv.templateSearchQuery)
	}
	if !containsStr(templateRowLabels(dv), "Eagles — Hotel California") {
		t.Fatalf("typed query did not filter the list")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu_Escape|TestTemplateMenu_CloseResets|TestTemplateMenu_TapFocuses|TestTemplateMenu_TypingUpdates'`
Expected: FAIL — `handleTemplateSearchEscape`/`updateTemplateSearch` undefined; tap not consumed.

- [ ] **Step 3a: Add `escapeFn` to `dvOverlayPortal`**

In `src/go/internal/ui/dv_overlay_portal.go`, add the field to the struct (~24-32):

```go
	escapeFn func() bool // optional Esc handler; true → keep portal open
```

And add the method (near the `Update` method ~35-39):

```go
// HandleEscape implements portalEscapeHandler: lets the wrapped overlay clear
// its own field (e.g. a search query) before the universal handler closes it.
func (o *dvOverlayPortal) HandleEscape() bool {
	if o.escapeFn != nil {
		return o.escapeFn()
	}
	return false
}
```

- [ ] **Step 3b: Add the poll + escape methods**

In `src/go/internal/ui/drumview_context_menu.go`, add:

```go
// updateTemplateSearch polls the pinned template search box each frame while the
// template page is open, syncing typed text into the query and refreshing the
// filtered results. Mirrors the instrument menu's per-frame search poll.
func (dv *DrumView) updateTemplateSearch() {
	if !dv.IsOverflowMenuOpen() || dv.overflowPage != 1 {
		return
	}
	dv.overflowPopupBtns(dv.overflowPopupRect()) // ensures box + rect exist
	if dv.templateSearchBox == nil {
		return
	}
	// Mobile: value comes from the native <input> overlay, not the ebiten kbd.
	if Profile().IsMobile() {
		if v, ok := mobileInputGetValue(templateSearchMobileInputID); ok && v != dv.templateSearchQuery {
			dv.setTemplateSearchQuery(v)
		}
	}
	dv.templateSearchBox.Update()
	if nv := dv.templateSearchBox.Value(); nv != dv.templateSearchQuery {
		dv.templateSearchQuery = nv
		dv.invalidateTemplateSearch()
		emitSearchChanged("template-menu", nv)
	}
}

// handleTemplateSearchEscape clears a non-empty template query (keeping the menu
// open) and returns true. Returns false when the query is empty so the universal
// Esc handler closes the menu. Mirrors InstrumentMenuComponent.HandleEscape.
func (dv *DrumView) handleTemplateSearchEscape() bool {
	if dv.overflowPage != 1 || strings.TrimSpace(dv.templateSearchQuery) == "" {
		return false
	}
	dv.setTemplateSearchQuery("")
	return true
}
```

- [ ] **Step 3c: Tap-to-focus in `handleOverflowMenuInput`**

In `src/go/internal/ui/drumview_context_menu.go`, in `handleOverflowMenuInput` (~586), after `btns := dv.overflowPopupBtns(popupRect)` and before `closeBtn := ...`, insert:

```go
	// Pinned search box: a press inside it focuses the box (not a row tap).
	if left && dv.overflowPage == 1 && dv.templateSearchBox != nil &&
		!dv.templateSearchRect.Empty() && image.Pt(mx, my).In(dv.templateSearchRect) {
		dv.templateSearchBox.SetFocus(true)
		return true
	}
```

- [ ] **Step 3d: Reset on close**

In `src/go/internal/ui/drumview_context_menu.go`, update `closeOverflowMenu` (~1036) to reset the query + defocus:

```go
func (dv *DrumView) closeOverflowMenu() {
	dv.overflowPage = 0
	dv.overflowBtns = nil // force rebuild on next open so stale animation state is cleared
	dv.setTemplateSearchQuery("")
	if dv.templateSearchBox != nil {
		dv.templateSearchBox.SetFocus(false)
	}
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
	dv.closeOverflowMenuPortal()
}
```

- [ ] **Step 3e: Wire the portal hooks + click-outside reset**

In `src/go/internal/ui/drumview_portal_open.go`, in `openOverflowMenuPortal`'s `dvOverlayPortal{...}` literal (~158-178), add two fields:

```go
				updateFn: func() { dv.updateTemplateSearch() },
				escapeFn: func() bool { return dv.handleTemplateSearchEscape() },
```

And extend the portal's `OnClose` (~181-188) so click-outside / CloseTop also resets (it deliberately does NOT call `closeOverflowMenu`):

```go
			OnClose: func() {
				if dv.overflowMenuScroll != nil {
					dv.overflowMenuScroll.DeferredTap().Cancel()
				}
				dv.overflowPage = 0
				dv.setTemplateSearchQuery("")
				if dv.templateSearchBox != nil {
					dv.templateSearchBox.SetFocus(false)
				}
			},
```

- [ ] **Step 4: Run the Task-4 tests — verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTemplateMenu_Escape|TestTemplateMenu_CloseResets|TestTemplateMenu_TapFocuses|TestTemplateMenu_TypingUpdates'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/dv_overlay_portal.go src/go/internal/ui/drumview_portal_open.go src/go/internal/ui/drumview_context_menu.go src/go/internal/ui/template_menu_search_test.go
git commit -m "feat(ui): template search input, per-frame poll, escape + reset"
```

---

### Task 5: Mobile native-input bridge

**Files:**
- Modify: `src/go/internal/ui/native_gesture.go` (`softKeyboardCandidates` ~191-216; `nativeInputCandidates` ~221-253)
- Create: `src/go/internal/ui/template_menu_mobile_search_test.go`

**Interfaces:**
- Consumes: `NativeRectCandidate`, `NativeIntent`, `NativeTextInput`, `NativeSoftKeyboard` (existing); `templateSearchMobileInputID` (Task 2); the poll `updateTemplateSearch` (Task 4) which already reads `mobileInputGetValue(templateSearchMobileInputID)`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/template_menu_mobile_search_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// feedTemplateNativeSearch injects a live native-input value for the template
// search field and drives real frames (portal updateFn → updateTemplateSearch).
func feedTemplateNativeSearch(t *testing.T, g *Game, v string) {
	t.Helper()
	testMobileInputValue[templateSearchMobileInputID] = v
	for i := 0; i < 5; i++ {
		_ = g.Update()
	}
}

func TestTemplateMenu_MobileNativeInputFilters(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputValue = map[string]string{}
	t.Cleanup(func() { testMobileInputValue = nil })

	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	wantAll := len(assets.Templates())

	feedTemplateNativeSearch(t, g, "hotel")
	if dv.templateSearchQuery != "hotel" {
		t.Fatalf("native-input value never reached the query; got %q", dv.templateSearchQuery)
	}
	if !containsStr(templateRowLabels(dv), "Eagles — Hotel California") {
		t.Fatalf("mobile query 'hotel' dropped its own template: %v", templateRowLabels(dv))
	}

	// Non-matching query empties the template rows (proves per-frame re-read).
	feedTemplateNativeSearch(t, g, "zzqxjkvw")
	if n := len(templateRowLabels(dv)); n != 0 {
		t.Fatalf("non-matching mobile query should empty the list, got %d", n)
	}

	// Clearing restores all rows.
	feedTemplateNativeSearch(t, g, "")
	if n := len(templateRowLabels(dv)); n != wantAll {
		t.Fatalf("clearing mobile query should restore %d rows, got %d", wantAll, n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_MobileNativeInputFilters`
Expected: FAIL — query stays empty (no native-input candidate registered, so the JS bridge id is never active in the real path).

- [ ] **Step 3a: Register the desktop-browser soft-keyboard candidate**

In `src/go/internal/ui/native_gesture.go`, in `softKeyboardCandidates` (after the inst-menu block ~206-208), add:

```go
	if dv.IsOverflowMenuOpen() && dv.overflowPage == 1 && dv.templateSearchBox != nil {
		add("overflow-menu", templateSearchMobileInputID, "text", dv.templateSearchRect)
	}
```

- [ ] **Step 3b: Register the mobile native-input candidate**

In `src/go/internal/ui/native_gesture.go`, in `nativeInputCandidates` (after the inst-menu block ~244-247), add:

```go
	if dv.IsOverflowMenuOpen() && dv.overflowPage == 1 && dv.templateSearchBox != nil && !dv.templateSearchRect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "overflow-menu", Rect: dv.templateSearchRect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: templateSearchMobileInputID, Text: dv.templateSearchBox.Text, MaxLen: 40, InputMode: "text"}})
	}
```

- [ ] **Step 4: Run the mobile test — verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplateMenu_MobileNativeInputFilters`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/native_gesture.go src/go/internal/ui/template_menu_mobile_search_test.go
git commit -m "feat(ui): mobile native-input bridge for template search"
```

---

### Task 6: Full-suite verification + gen/discipline guards

**Files:** none (verification only; fix-forward if a guard trips).

- [ ] **Step 1: Full `internal/ui` fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. This includes the token-discipline and design-drift guards; if `TestTokenDiscipline` trips, an inline `color.RGBA{}` literal slipped in — replace it with an existing token (the plan uses none, so this should be clean).

- [ ] **Step 2: Broader packages touched indirectly**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/assets/ ./internal/i18n/`
Expected: PASS.

- [ ] **Step 3: gofmt check**

Run: `cd src/go && ../../.tools/go/bin/gofmt -l internal/ui/menu_search_field.go internal/ui/drumview_context_menu.go internal/ui/drumview.go internal/ui/dv_overlay_portal.go internal/ui/drumview_portal_open.go internal/ui/native_gesture.go`
Expected: empty output (all formatted).

- [ ] **Step 4: Manual/verify pass (real app)**

Invoke the `verify` skill (or `make run`) and exercise: open the overflow menu → Load template → type in the search box → confirm rows filter + highlight, header/search stay pinned while results scroll, Esc clears then closes, reopening is fresh. Confirm desktop + a mobile viewport.

- [ ] **Step 5: Commit (if any fix-forward changes were needed)**

```bash
cd /home/ymolinar/Repos/beatmo
git add -A src/go/internal/ui
git commit -m "test(ui): template menu search full-suite verification"
```

---

## Self-Review

**Spec coverage:**
- §3 shared helpers → Task 1. ✓
- §4 pinned-chrome restructure → Task 3. ✓
- §5 memoized filtering → Task 2 (`templateSearchResults`). ✓
- §6 state fields → Task 2. ✓
- §7 input/lifecycle (poll, focus, escape, reset) → Task 4. ✓
- §8 mobile native input → Task 5. ✓
- §9 tests: filter/sort/highlight/empty (Task 2), style/pinned (Task 3), escape/close/focus/typing (Task 4), mobile (Task 5), inst-menu regression (Task 1 Step 6). The spec's "matches genre key" case is covered by `MenuSearcher`'s own `fuzzy_search_test.go` (it already searches `Key`) and by feeding `Key: tp.Genre` in Task 2; a dedicated brittle key-only UI test is intentionally omitted (most stems share words with their display titles, so a pure key-only match is not reliably constructible). ✓
- §10 guards (token discipline, scroll geometry) → Task 6. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every run step shows the exact command + expected result.

**Type consistency:** `ensureMenuSearchBox`/`drawMenuSearchBox` (Task 1) used identically in Tasks 3-4. `templateSearchResults`/`setTemplateSearchQuery`/`invalidateTemplateSearch` (Task 2) used in Tasks 3-4. `overflowPinnedCount` (Task 3) used in `configureOverflowScroll`/`overflowPopupBtns`/`drawOverflowMenu`. `templateSearchMobileInputID` (Task 2) used in Tasks 4-5. `escapeFn`/`HandleEscape` (Task 4) consumed by the existing `portalEscapeHandler` path. Field names (`templateSearchBox`, `templateSearchRect`, `templateSearchQuery`, `templateSearchCache*`) consistent across all tasks.

**Known interim state:** After Task 2 (before Task 3) the `{search:true}` row reserves a blank scrolling slot with no visible box — intentional and covered by keeping existing tests green; Task 3 converts it to the pinned box. Each task still ends green.
