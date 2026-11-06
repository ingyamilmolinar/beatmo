//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
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

// containsStr is defined in bottom_nav_relabel_test.go (same package).

func TestTemplateMenu_SearchFiltersRows(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("brandenburg")

	labels := templateRowLabels(dv)
	if !containsStr(labels, "Bach — Brandenburg Concerto No. 3") {
		t.Fatalf("query 'brandenburg' dropped its own template; got %v", labels)
	}
	if containsStr(labels, "Miles Davis — So What") {
		t.Fatalf("query 'brandenburg' should exclude unrelated templates; got %v", labels)
	}
}

func TestTemplateMenu_EmptyQueryShowsAllInOrder(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("")

	labels := templateRowLabels(dv)
	if want := len(assets.Templates()); len(labels) != want {
		t.Fatalf("empty query = %d template rows, want %d (all)", len(labels), want)
	}
	if want := assets.Templates()[0].Display; labels[0] != want {
		t.Fatalf("empty query first row = %q, want the first templateOrder entry %q", labels[0], want)
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
	if searchBefore.Empty() {
		t.Fatal("search rect not laid out before scroll")
	}

	scroll := dv.OverflowMenuScrollForTest()
	if scroll == nil || !scroll.HasScroll() {
		t.Skip("template list fits without scrolling at this size")
	}
	rowBefore, ok := lastTemplateBtn(dv)
	if !ok {
		t.Fatal("no last template button")
	}
	// Scroll down a few notches (smooth wheel: no cooldown between notches).
	for i := 0; i < scroll.ScrollBehavior().VS.Total+4; i++ {
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

func TestTemplateMenu_SearchHighlightsMatches(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("brandenburg")

	var found bool
	for _, it := range dv.overflowItems() {
		if it.iconID != IconTemplate || it.label != "Bach — Brandenburg Concerto No. 3" {
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
		t.Fatal("Brandenburg template row not present for query 'brandenburg'")
	}
}

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
	dv.setTemplateSearchQuery("brandenburg")
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
	dv.templateSearchBox.SetText("brandenburg")
	dv.updateTemplateSearch()

	if dv.templateSearchQuery != "brandenburg" {
		t.Fatalf("poll did not read the box value into the query; got %q", dv.templateSearchQuery)
	}
	if !containsStr(templateRowLabels(dv), "Bach — Brandenburg Concerto No. 3") {
		t.Fatalf("typed query did not filter the list")
	}
}

func TestTemplateMenu_SearchResultsMemoizedOnRepeatCall(t *testing.T) {
	dv := openTemplatePageForSearch(t)
	dv.setTemplateSearchQuery("bach")

	r1 := dv.templateSearchResults()
	r2 := dv.templateSearchResults()
	if len(r1) == 0 {
		t.Fatal("no results for query 'bach'")
	}
	// A repeat call with an unchanged query must return the SAME backing slice
	// (memo cache hit) — i.e. MenuSearcher.Search was not re-invoked. This guards
	// the documented template-menu-hang regression class (per-frame refuzz).
	if &r1[0] != &r2[0] {
		t.Fatal("templateSearchResults recomputed on unchanged query — memo cache not hit")
	}
	// Changing the query must invalidate the cache and produce a fresh slice.
	dv.setTemplateSearchQuery("miles")
	r3 := dv.templateSearchResults()
	if len(r3) > 0 && &r3[0] == &r1[0] {
		t.Fatal("cache not invalidated after query change")
	}
}
