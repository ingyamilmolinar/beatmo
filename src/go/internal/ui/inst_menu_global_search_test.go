//go:build test

package ui

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// newGlobalSearchComp builds an instrument-menu component that opens in
// categories mode with a favorites store wired — the fixture for the global
// (categories-level) search bar.
func newGlobalSearchComp(t *testing.T) (*InstrumentMenuComponent, *InMemoryFavoritesStore) {
	t.Helper()
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 300, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Drums"},
			{ID: "snare", Label: "Snare", Category: "Drums"},
			{ID: "guitar", Label: "Guitar", Category: "Strings"},
			{ID: "violin", Label: "Violin", Category: "Strings"},
			{ID: "gong", Label: "Gong", Category: "Cymbals"},
		},
		Categories:            []string{"Drums", "Strings", "Cymbals"},
		ForceCategories:       true,
		ShowFavoritesCategory: true,
		Favorites:             store,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("fixture must open in categories mode, got %v", comp.Mode())
	}
	return comp, store
}

// compUpdateFrames drives comp.Update() n frames with idle input (the real
// per-frame polling path that syncs the search box into the filter state).
func compUpdateFrames(comp *InstrumentMenuComponent, n int) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	defer restore()
	for i := 0; i < n; i++ {
		comp.Update()
	}
}

// TestInstMenuGlobalSearch_CategoriesModeHasSearchBar: the categories view
// (below the "Categories" title strip) must host a search field, wired for
// both desktop (TextInput) and mobile (SearchRect drives the native-input
// registration in drumview_layout.go).
func TestInstMenuGlobalSearch_CategoriesModeHasSearchBar(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)
	if comp.SearchBox() == nil {
		t.Fatal("categories mode must build the global search box")
	}
	if comp.SearchRect().Empty() {
		t.Fatal("categories mode must expose a non-empty SearchRect so the " +
			"mobile native-input / soft-keyboard registration can find the field")
	}
	// The search row sits below the title strip and above the first row.
	if comp.SearchRect().Min.Y >= comp.CategoryBtns()[0].Rect().Min.Y {
		t.Fatalf("search row (%v) must sit above the first category row (%v)",
			comp.SearchRect(), comp.CategoryBtns()[0].Rect())
	}
}

// TestInstMenuGlobalSearch_RealTimeTypingFilters: typing into the categories
// search box must filter in real time through the same per-frame polling the
// instruments-mode search uses.
func TestInstMenuGlobalSearch_RealTimeTypingFilters(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)
	if comp.SearchBox() == nil {
		t.Fatal("no global search box in categories mode")
	}
	comp.SearchBox().SetText("gui")
	compUpdateFrames(comp, 3)
	if comp.state.searchText != "gui" {
		t.Fatalf("typed query %q never reached the menu state (got %q) — the "+
			"per-frame search poll must run in categories mode too", "gui", comp.state.searchText)
	}
	if !slices.Contains(comp.instIDs, "guitar") {
		t.Fatalf("query \"gui\" must surface instrument \"guitar\" in the global "+
			"results, got instrument rows %v", comp.instIDs)
	}
}

// TestInstMenuGlobalSearch_MixedCategoryAndInstrumentResults: the result list
// mixes matching categories (drill-in rows) and matching instruments (each
// labelled with the category it lives in).
func TestInstMenuGlobalSearch_MixedCategoryAndInstrumentResults(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)

	// "str" matches only the category "Strings" — no instrument label.
	comp.SetSearchText("str")
	catLabels := make([]string, 0, len(comp.CategoryBtns()))
	for _, b := range comp.CategoryBtns() {
		catLabels = append(catLabels, b.Text)
	}
	if len(catLabels) != 1 || catLabels[0] != "Strings" {
		t.Fatalf("query \"str\" should match exactly the category \"Strings\", got %v", catLabels)
	}
	if len(comp.instIDs) != 0 {
		t.Fatalf("query \"str\" matches no instrument, got instrument rows %v", comp.instIDs)
	}

	// "gui" matches only the instrument "Guitar" — shown with its category.
	comp.SetSearchText("gui")
	if got := comp.instIDs; len(got) != 1 || got[0] != "guitar" {
		t.Fatalf("query \"gui\" should match exactly instrument \"guitar\", got %v", got)
	}
	if len(comp.CategoryBtns()) != 0 {
		var labels []string
		for _, b := range comp.CategoryBtns() {
			labels = append(labels, b.Text)
		}
		t.Fatalf("query \"gui\" matches no category, got category rows %v", labels)
	}
	trails := comp.GlobalResultTrails()
	if len(trails) != 1 || trails[0] != "Strings" {
		t.Fatalf("the guitar result must carry its category \"Strings\" as the "+
			"row trail, got %v", trails)
	}
}

// TestInstMenuGlobalSearch_FavoritesOnTop: a ★-favorited instrument that
// matches the query must be the FIRST result row — above matching categories
// and above better-scoring non-favorite instruments.
func TestInstMenuGlobalSearch_FavoritesOnTop(t *testing.T) {
	comp, store := newGlobalSearchComp(t)
	store.Set("gong", true)

	// "g" matches instruments Guitar + Gong and the category Strings (g).
	comp.SetSearchText("g")
	if len(comp.instIDs) < 2 {
		t.Fatalf("query \"g\" should match Guitar and Gong, got %v", comp.instIDs)
	}
	if comp.instIDs[0] != "gong" {
		t.Fatalf("favorited \"gong\" must be the first instrument row, got %v", comp.instIDs)
	}
	// The favorite's row must sit above every other result row on screen.
	favTop := comp.InstBtns()[0].Rect().Min.Y
	for _, b := range comp.CategoryBtns() {
		if b.Rect().Min.Y <= favTop {
			t.Fatalf("favorited instrument row (y=%d) must render above category "+
				"row %q (y=%d)", favTop, b.Text, b.Rect().Min.Y)
		}
	}
	for i, b := range comp.InstBtns()[1:] {
		if b.Rect().Min.Y <= favTop {
			t.Fatalf("favorited instrument row (y=%d) must render above instrument "+
				"row %q (y=%d)", favTop, comp.instIDs[i+1], b.Rect().Min.Y)
		}
	}
}

// TestInstMenuGlobalSearch_CategoryResultDrillsInAndClearsQuery: tapping a
// category row in the global results enters that category (instruments mode)
// with a fresh, unfiltered list.
func TestInstMenuGlobalSearch_CategoryResultDrillsInAndClearsQuery(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)
	comp.SetSearchText("str")
	if len(comp.CategoryBtns()) != 1 {
		t.Fatalf("query \"str\" should yield the Strings category row, got %d rows",
			len(comp.CategoryBtns()))
	}
	comp.CategoryBtns()[0].OnClick()
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("clicking a category result must enter instruments mode, got %v", comp.Mode())
	}
	if comp.ActiveCategory() != "Strings" {
		t.Fatalf("clicking the Strings result must activate that category, got %q",
			comp.ActiveCategory())
	}
	if comp.state.searchText != "" {
		t.Fatalf("entering a category from global results must clear the query, got %q",
			comp.state.searchText)
	}
	want := []string{"guitar", "violin"}
	got := append([]string(nil), comp.state.filteredInsts...)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("Strings category should list %v, got %v", want, got)
	}
}

// TestInstMenuGlobalSearch_InstrumentResultSelects: tapping an instrument row
// in the global results selects it (audition), exactly like instruments mode.
func TestInstMenuGlobalSearch_InstrumentResultSelects(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)
	var selected string
	comp.props.OnSelect = func(id string) { selected = id }
	comp.SetSearchText("gui")
	if len(comp.InstBtns()) != 1 {
		t.Fatalf("query \"gui\" should yield one instrument row, got %d", len(comp.InstBtns()))
	}
	comp.InstBtns()[0].OnClick()
	if selected != "guitar" {
		t.Fatalf("clicking the guitar result must select it, got %q", selected)
	}
	if comp.ActiveInstrumentID() != "guitar" {
		t.Fatalf("live selection must follow the global-result pick, got %q",
			comp.ActiveInstrumentID())
	}
}

// TestInstMenuSearch_FavoritesFirstInInstrumentsMode: the instruments-mode
// search must also rank ★-favorited matches above better-scoring
// non-favorites ("all searches always show Favorites on top").
func TestInstMenuSearch_FavoritesFirstInInstrumentsMode(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 300, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: "snare", Label: "Snare", Category: "Drums"},
			{ID: "bigsnare", Label: "Big Snare Drum", Category: "Drums"},
		},
		Favorites: store,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("fixture must open in instruments mode, got %v", comp.Mode())
	}

	// Sanity: without a favorite, the exact-prefix match ranks first.
	comp.SetSearchText("snare")
	if got := comp.state.filteredInsts; len(got) != 2 || got[0] != "snare" {
		t.Fatalf("without favorites, \"Snare\" should outrank \"Big Snare Drum\": %v", got)
	}

	// Favoriting the lower-scoring match must lift it to the top.
	store.Set("bigsnare", true)
	comp.SetSearchText("") // force a state change so the next set rebuilds
	comp.SetSearchText("snare")
	if got := comp.state.filteredInsts; len(got) != 2 || got[0] != "bigsnare" {
		t.Fatalf("FAVORITES-ON-TOP: favorited \"Big Snare Drum\" must rank above the "+
			"higher-scoring \"Snare\" in search results, got %v", got)
	}
}

// TestInstMenuGlobalSearch_MobileNativeInputRealtime: on mobile the categories
// search bar reads the native <input> value live, exactly like the
// instruments-mode search (same "inst-search" bridge id).
func TestInstMenuGlobalSearch_MobileNativeInputRealtime(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputValue = map[string]string{}
	t.Cleanup(func() { testMobileInputValue = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	g.drum.openInstMenuForRow(0)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := g.drum.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	// Navigate to the categories view via the production breadcrumb-root path
	// when the menu opened directly in instruments mode.
	if comp.Mode() != InstMenuModeCategories {
		comp.popBreadcrumbTo(0)
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeCategories {
		t.Fatal("could not reach categories mode")
	}

	// Pick a real instrument label to search for.
	insts := comp.props.Instruments
	if len(insts) == 0 {
		t.Fatal("no instruments in the live catalog")
	}
	target := insts[0].ID
	label := comp.state.displayLabelByID[target]
	if label == "" {
		label = target
	}
	query := strings.ToLower(label)

	feedNativeSearch(t, g, query)
	if comp.state.searchText != query {
		t.Fatalf("MOBILE GLOBAL SEARCH: native-input value %q never reached the "+
			"categories-mode search state (got %q)", query, comp.state.searchText)
	}
	if !slices.Contains(comp.instIDs, target) {
		t.Fatalf("query %q should surface instrument %q in the global results, got %v",
			query, target, comp.instIDs)
	}
}
