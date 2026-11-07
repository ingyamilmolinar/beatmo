//go:build test

package ui

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func newGlobalSearchComp(t *testing.T) (*InstrumentMenuComponent, *InMemoryFavoritesStore) {
	t.Helper()
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 300, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
			{ID: roster.GuitarNylon, Label: "Guitar", Category: audio.CatStrings},
			{ID: roster.Violin, Label: "Violin", Category: audio.CatStrings},
			{ID: roster.SingingBowl, Label: "Gong", Category: audio.CatCymbal},
		},
		Categories:            []audio.Category{audio.CatPercussion, audio.CatStrings, audio.CatCymbal},
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

// The real per-frame polling that syncs the search box into the filter state.
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

// Desktop types into the TextInput; on mobile SearchRect drives the native-input registration.
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

// Through the same per-frame polling as the instruments-mode search.
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
	if !slices.Contains(comp.instIDs, roster.GuitarNylon) {
		t.Fatalf("query \"gui\" must surface instrument \"guitar\" in the global "+
			"results, got instrument rows %v", comp.instIDs)
	}
}

// Category rows drill in; instrument rows are labelled with their category.
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
	if got := comp.instIDs; len(got) != 1 || got[0] != roster.GuitarNylon {
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

// Every search shows Favorites on top: above matching categories and better-scoring non-favourites.
func TestInstMenuGlobalSearch_FavoritesOnTop(t *testing.T) {
	comp, store := newGlobalSearchComp(t)
	store.Set(roster.SingingBowl, true)

	// "g" matches instruments Guitar + Gong and the category Strings (g).
	comp.SetSearchText("g")
	if len(comp.instIDs) < 2 {
		t.Fatalf("query \"g\" should match Guitar and Gong, got %v", comp.instIDs)
	}
	if comp.instIDs[0] != roster.SingingBowl {
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
	if comp.ActiveCategory() != audio.CatStrings {
		t.Fatalf("clicking the Strings result must activate that category, got %q",
			comp.ActiveCategory())
	}
	if comp.state.searchText != "" {
		t.Fatalf("entering a category from global results must clear the query, got %q",
			comp.state.searchText)
	}
	want := []instrument.ID{roster.GuitarNylon, roster.Violin}
	got := append([]instrument.ID(nil), comp.state.filteredInsts...)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("Strings category should list %v, got %v", want, got)
	}
}

func TestInstMenuGlobalSearch_InstrumentResultSelects(t *testing.T) {
	comp, _ := newGlobalSearchComp(t)
	var selected instrument.ID
	comp.props.OnSelect = func(id instrument.ID) { selected = id }
	comp.SetSearchText("gui")
	if len(comp.InstBtns()) != 1 {
		t.Fatalf("query \"gui\" should yield one instrument row, got %d", len(comp.InstBtns()))
	}
	comp.InstBtns()[0].OnClick()
	if selected != roster.GuitarNylon {
		t.Fatalf("clicking the guitar result must select it, got %q", selected)
	}
	if comp.ActiveInstrumentID() != roster.GuitarNylon {
		t.Fatalf("live selection must follow the global-result pick, got %q",
			comp.ActiveInstrumentID())
	}
}

// Every search shows Favorites on top, instruments mode included.
func TestInstMenuSearch_FavoritesFirstInInstrumentsMode(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 300, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
			{ID: roster.SnareFat, Label: "Big Snare Drum", Category: audio.CatPercussion},
		},
		Favorites: store,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("fixture must open in instruments mode, got %v", comp.Mode())
	}

	// Sanity: without a favorite, the exact-prefix match ranks first.
	comp.SetSearchText("snare")
	if got := comp.state.filteredInsts; len(got) != 2 || got[0] != roster.Snare {
		t.Fatalf("without favorites, \"Snare\" should outrank \"Big Snare Drum\": %v", got)
	}

	// Favoriting the lower-scoring match must lift it to the top.
	store.Set(roster.SnareFat, true)
	comp.SetSearchText("") // force a state change so the next set rebuilds
	comp.SetSearchText("snare")
	if got := comp.state.filteredInsts; len(got) != 2 || got[0] != roster.SnareFat {
		t.Fatalf("FAVORITES-ON-TOP: favorited \"Big Snare Drum\" must rank above the "+
			"higher-scoring \"Snare\" in search results, got %v", got)
	}
}

// The same "inst-search" bridge id as the instruments-mode search.
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
	// Through the production breadcrumb-root path when the menu opened in instruments mode.
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
		label = string(target)
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
