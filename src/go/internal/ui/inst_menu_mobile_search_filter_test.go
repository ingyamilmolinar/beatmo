//go:build test

package ui

import (
	"slices"
	"strings"
	"testing"
)

// navigateToInstrumentsMode opens the instrument menu on a mobile-sized game and
// drills into instruments mode (where the search box lives). Returns the live
// component. Mirrors the navigation in inst_menu_mobile_search_test.go.
func navigateToInstrumentsMode(t *testing.T, g *Game) *InstrumentMenuComponent {
	t.Helper()
	dv := g.drum
	dv.openInstMenuForRow(0)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	if comp.Mode() == InstMenuModeCategories {
		cats := comp.CategoryBtns()
		if len(cats) > 0 && cats[0].OnClick != nil {
			cats[0].OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode (search box present), got %v", comp.Mode())
	}
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	return comp
}

// TestInstMenuMobileSearchFiltersFromNativeInput reproduces the reported bug:
// on real mobile, the instrument-menu search bar opens the native keyboard and
// accepts typed text, but the menu list never filters/highlights because no Go
// code reads the live native-input value back into the menu's search state.
//
// The native <input> value is exposed to Go via mobileInputGetValue("inst-search")
// (live, non-consuming). The InstrumentMenuComponent.Update() loop only polls the
// ebiten-keyboard-backed searchBox (desktop path) and never consults the mobile
// native input, so state.searchText stays empty and the list never rebuilds.
//
// This test injects a live native-input value and drives the real Game.Update()
// loop (which runs the menu's portal updateFn → component Update). It asserts the
// search query propagated into the menu's filter state.
func TestInstMenuMobileSearchFiltersFromNativeInput(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputValue = map[string]string{}
	t.Cleanup(func() { testMobileInputValue = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	comp := navigateToInstrumentsMode(t, g)

	// Baseline: empty query shows the full category list.
	baseline := append([]string(nil), comp.state.filteredInsts...)
	if len(baseline) == 0 {
		t.Fatal("no instruments available in instruments mode")
	}

	// Type the full label of a real instrument so the fuzzy filter is guaranteed
	// to keep at least that one — derived from the fixture so the test does not
	// depend on a specific instrument catalog.
	target := baseline[0]
	label := comp.state.displayLabelByID[target]
	if label == "" {
		label = target
	}
	query := strings.ToLower(label)

	// Simulate the user typing into the native keyboard. The JS bridge
	// (_mobileInputGetValue) returns the live <input>.value; the stub returns the
	// injected value for the registered id.
	feedNativeSearch(t, g, query)

	if comp.state.searchText != query {
		t.Fatalf("SEARCH FILTER BUG: typed native-input %q never reached the menu's "+
			"search state (state.searchText=%q). The instrument list cannot filter or "+
			"highlight because the mobile native-input value is never read back into "+
			"the component.", query, comp.state.searchText)
	}

	// The typed-for instrument must survive the filter.
	if !slices.Contains(comp.state.filteredInsts, target) {
		t.Fatalf("query %q dropped its own instrument %q; filtered=%v",
			query, target, comp.state.filteredInsts)
	}

	// Highlights must be populated so the matched characters render highlighted.
	if len(comp.state.searchHighlights) == 0 {
		t.Fatalf("expected fuzzy-search highlights for query %q, got none", query)
	}

	// Real-time narrowing: a query that matches nothing empties the list as the
	// user keeps typing (proves the value is re-read every frame, not once).
	feedNativeSearch(t, g, "zzqxjkvw")
	if n := len(comp.state.filteredInsts); n != 0 {
		t.Fatalf("expected empty list for non-matching query, got %d: %v",
			n, comp.state.filteredInsts)
	}

	// Real-time clearing: deleting the query restores the full list live.
	feedNativeSearch(t, g, "")
	if n := len(comp.state.filteredInsts); n != len(baseline) {
		t.Fatalf("clearing the query should restore the full list (%d), got %d",
			len(baseline), n)
	}
}

// feedNativeSearch injects v as the live native-input value for the
// instrument-menu search field and drives a few update frames so the component
// picks it up.
func feedNativeSearch(t *testing.T, g *Game, v string) {
	t.Helper()
	testMobileInputValue["inst-search"] = v
	for i := 0; i < 5; i++ {
		_ = g.Update()
	}
}
