//go:build test

package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

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

// The native <input> value reaches Go only through mobileInputGetValue("inst-search"); the menu must poll it
// every frame, or the phone keyboard types into a list that never filters.
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
	baseline := append([]instrument.ID(nil), comp.state.filteredInsts...)
	if len(baseline) == 0 {
		t.Fatal("no instruments available in instruments mode")
	}

	// A real label from the fixture, so the fuzzy filter keeps at least that one whatever the catalogue holds.
	target := baseline[0]
	label := comp.state.displayLabelByID[target]
	if label == "" {
		label = string(target)
	}
	query := strings.ToLower(label)

	// The stub returns the injected value for the registered id, as the bridge returns the live <input>.value.
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

	// A query matching nothing empties the list: the value is re-read every frame, not once.
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

func feedNativeSearch(t *testing.T, g *Game, v string) {
	t.Helper()
	testMobileInputValue["inst-search"] = v
	for i := 0; i < 5; i++ {
		_ = g.Update()
	}
}
