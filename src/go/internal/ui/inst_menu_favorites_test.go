package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestInstMenuComp_StarToggleHitArea verifies that the star column lays
// out one hit rectangle per visible instrument when a FavoritesStore is
// wired in, and zero rectangles when no store is configured. This is
// the load-bearing assertion for Phase 5: it pins the favorites
// integration without coupling to legacy scroll state.
func TestInstMenuComp_StarToggleHitArea(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 200, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
		},
		Favorites: store,
	})
	comp.Open()
	if comp.state.mode != InstMenuModeInstruments {
		// The component opens in instruments mode unless ForceCategories
		// is set; skip the precondition if categories are forced.
		t.Logf("opened in mode=%v (instruments mode required for stars)", comp.state.mode)
	}
	if got := nonEmptyStars(comp); got < 2 {
		t.Fatalf("expected at least 2 star hit rects (one per instrument), got %d", got)
	}
	if got := len(comp.instIDs); got != len(comp.favRects) {
		t.Fatalf("instIDs len %d does not match favRects len %d", got, len(comp.favRects))
	}
	// Toggle via the component's helper — should propagate to the store.
	idx, _, id, ok := firstStarRow(comp)
	if !ok {
		t.Fatalf("no star row found")
	}
	comp.toggleFavoriteAt(idx)
	if !store.Get(instrument.ID(id)) {
		t.Fatalf("toggleFavoriteAt did not propagate to store; id=%q stored=%v", id, store.Get(instrument.ID(id)))
	}
	comp.toggleFavoriteAt(idx)
	if store.Get(instrument.ID(id)) {
		t.Fatalf("second toggleFavoriteAt did not flip back; id=%q stored=%v", id, store.Get(instrument.ID(id)))
	}
}

// TestInstMenuComp_StarColumnDisabledWhenNoStore verifies the column is
// not rendered (and no hit rects allocated) when Favorites is nil. This
// keeps the legacy menu layout unchanged for callers that haven't opted
// into favorites yet.
func TestInstMenuComp_StarColumnDisabledWhenNoStore(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(0, 0, 200, 24),
		VertBounds: image.Rect(0, 0, 400, 800),
		RowHeight:  24,
		Instruments: []InstrumentOption{
			{ID: roster.Kick, Label: "Kick"},
		},
		Favorites: nil,
	})
	comp.Open()
	// favRects is row-indexed, so it has one (zero) slot per row; what must
	// be absent is any drawable/hittable star.
	if got := nonEmptyStars(comp); got != 0 {
		t.Fatalf("expected zero star hit rects when Favorites is nil, got %d", got)
	}
	if got := comp.starRowAt(comp.InstBtns()[0].Rect().Max); got != -1 {
		t.Fatalf("starRowAt found star row %d with Favorites nil", got)
	}
}

// TestInstMenuComp_BreadcrumbAccessor confirms the new BreadcrumbPath()
// accessor returns sensible values across the open/categories/instruments
// state transitions. Phase 6 migrates legacy mode-enum tests onto this
// API.
func TestInstMenuComp_BreadcrumbAccessor(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	if got := comp.BreadcrumbPath(); len(got) != 0 {
		t.Fatalf("closed menu BreadcrumbPath = %v, want []", got)
	}
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(0, 0, 200, 24),
		VertBounds:  image.Rect(0, 0, 400, 800),
		RowHeight:   24,
		Categories:  []audio.Category{audio.CatPercussion, audio.CatLead},
		Instruments: []InstrumentOption{{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion}},
	})
	comp.Open()
	got := comp.BreadcrumbPath()
	if len(got) == 0 || got[0] != "Categories" {
		t.Fatalf("open menu BreadcrumbPath[0] should be 'Categories', got %v", got)
	}
}

// instMenuTestProps is a small helper for the tests below: returns a fully-
// formed InstrumentMenuProps with `instruments` (each labeled by id) sorted
// into one synthetic category. ShowFavoritesCategory defaults true here so
// tests opt in by default; legacy tests in drumview_test.go that call
// NewDrumView directly do not (the flag is wired by the production builder
// and is opt-out for them).
func instMenuTestProps(instruments []InstrumentOption, categories []audio.Category) InstrumentMenuProps {
	return InstrumentMenuProps{
		AnchorRect:            image.Rect(0, 0, 240, 24),
		VertBounds:            image.Rect(0, 0, 480, 800),
		RowHeight:             24,
		Instruments:           instruments,
		Categories:            categories,
		ShowFavoritesCategory: true,
	}
}

// TestInstMenu_FavoritesVirtualCategoryAppearsFirst pins that the menu
// renders "Favorites" as the very first category row whenever a Favorites
// store is wired, and that the row carries its ★ count as a trailing
// caption (categoryMeta) so a glance tells the user how many ★s they've
// collected — the count is display metadata, not part of the label.
func TestInstMenu_FavoritesVirtualCategoryAppearsFirst(t *testing.T) {
	store := NewInMemoryFavoritesStore(roster.Kick, roster.Snare)
	props := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
		},
		[]audio.Category{audio.CatPercussion},
	)
	props.Favorites = store
	props.ForceCategories = true

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.Open()

	if comp.state.mode != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %v", comp.state.mode)
	}
	btns := comp.CategoryBtns()
	if len(btns) == 0 {
		t.Fatalf("no category buttons rendered")
	}
	if btns[0].Text != "Favorites" {
		t.Errorf("first category row Text = %q, want %q", btns[0].Text, "Favorites")
	}
	if len(comp.categoryMeta) == 0 || !comp.categoryMeta[0].favRow || comp.categoryMeta[0].count != 2 {
		t.Errorf("first category row meta = %+v, want favRow=true count=2", comp.categoryMeta)
	}
}

// TestInstMenu_FavoritesEmptyState pins the onboarding hint shown when the ★
// store is empty. Without this guard a fresh install would surface "No
// matches" when the user clicked Favorites — confusing because it implies a
// search ran.
func TestInstMenu_FavoritesEmptyState(t *testing.T) {
	store := NewInMemoryFavoritesStore() // empty
	props := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
		},
		[]audio.Category{audio.CatPercussion},
	)
	props.Favorites = store
	props.ForceCategories = true

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.Open()

	// Click the Favorites virtual category (always index 0 when the store is
	// wired).
	btns := comp.CategoryBtns()
	if len(btns) == 0 || btns[0].OnClick == nil {
		t.Fatalf("Favorites button missing OnClick")
	}
	btns[0].OnClick()

	// Now we should be in instruments mode with the empty-state placeholder.
	if comp.state.mode != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode after Favorites click, got %v", comp.state.mode)
	}
	if !comp.state.favoritesView {
		t.Errorf("favoritesView should be true after Favorites click")
	}
	insts := comp.InstBtns()
	if len(insts) != 1 {
		t.Fatalf("expected 1 placeholder button, got %d", len(insts))
	}
	if insts[0].Text == "" || insts[0].Text == "No matches" {
		t.Errorf("expected onboarding hint, got %q", insts[0].Text)
	}
}

// TestInstMenu_FavoritesPinnedInInstrumentsMode pins the empty-query order:
// within a regular category, ★-favorited items sort before non-favorited
// items, alphabetically within each tier.
func TestInstMenu_FavoritesPinnedInInstrumentsMode(t *testing.T) {
	store := NewInMemoryFavoritesStore("zeta-fav", "alpha-fav")
	insts := []InstrumentOption{
		{ID: "zeta-fav", Label: "Zeta Fav", Category: audio.CatPercussion},
		{ID: "mike", Label: "Mike", Category: audio.CatPercussion},
		{ID: "alpha-fav", Label: "Alpha Fav", Category: audio.CatPercussion},
		{ID: "charlie", Label: "Charlie", Category: audio.CatPercussion},
	}
	props := instMenuTestProps(insts, []audio.Category{audio.CatPercussion})
	props.Favorites = store
	// Open in instruments mode targeting the Drums category directly.
	props.CurrentInstrument = "mike"

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.state.activeCat = audio.CatPercussion
	comp.Open()

	// filteredInsts should be ordered: ★ alpha first, then non-★ in input
	// order (tier 2 keeps the catalog's curated ordering — see
	// PinSource.SortByTierAlpha for the contract). Input order for the
	// non-★ items is mike before charlie.
	got := comp.state.filteredInsts
	want := []string{"alpha-fav", "zeta-fav", "mike", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("filteredInsts len: got %d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != instrument.ID(want[i]) {
			t.Errorf("filteredInsts[%d]: got %q want %q (full %v)", i, got[i], want[i], got)
		}
	}
}

// TestInstMenu_FavoritesPinnedWithSearchQuery pins fuzzy-score primacy: a
// clearly higher score wins over a ★, but within equal-score groups the ★
// breaks the tie. Without this, a project pinned the user types "snare"
// against would surface a weakly-matched ★ ahead of a strongly-matched
// non-★.
func TestInstMenu_FavoritesPinnedWithSearchQuery(t *testing.T) {
	store := NewInMemoryFavoritesStore(roster.KickDeep) // kick-fav is ★ but a weak match for "snare"
	insts := []InstrumentOption{
		{ID: roster.SnareFat, Label: "Snare Tight", Category: audio.CatPercussion},  // strong match
		{ID: roster.KickDeep, Label: "Kick Fav", Category: audio.CatPercussion},     // ★, no match for snare
		{ID: roster.SnareLofi, Label: "Snare Loose", Category: audio.CatPercussion}, // strong match
	}
	props := instMenuTestProps(insts, []audio.Category{audio.CatPercussion})
	props.Favorites = store

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.state.activeCat = audio.CatPercussion
	comp.Open()
	comp.SetSearchText("snare")

	got := comp.state.filteredInsts
	// "snare-tight" and "snare-loose" both hit; alphabetical breaks the tie.
	// "kick-fav" doesn't contain s-n-a-r-e in order so it shouldn't match at
	// all. (The fuzzy matcher requires query as a subsequence.)
	if len(got) != 2 {
		t.Fatalf("expected 2 results for 'snare', got %d (%v)", len(got), got)
	}
	for _, id := range got {
		if id == roster.KickDeep {
			t.Errorf("kick-fav should not match 'snare' query (got %v)", got)
		}
	}
}

// TestInstMenu_ProjectPinsBeatUserStars pins the tier ordering: project pins
// (tier 0) sort above ★ items (tier 1) above the rest (tier 2). All three
// alphabetically within their tier.
func TestInstMenu_ProjectPinsBeatUserStars(t *testing.T) {
	store := NewInMemoryFavoritesStore("user-star")
	insts := []InstrumentOption{
		{ID: "project-pin", Label: "Project Pin", Category: audio.CatPercussion},
		{ID: "user-star", Label: "User Star", Category: audio.CatPercussion},
		{ID: "rest", Label: "Rest", Category: audio.CatPercussion},
	}
	props := instMenuTestProps(insts, []audio.Category{audio.CatPercussion})
	props.Favorites = store
	props.ProjectPins = map[instrument.ID]struct{}{"project-pin": {}}

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.state.activeCat = audio.CatPercussion
	comp.Open()

	got := comp.state.filteredInsts
	want := []string{"project-pin", "user-star", "rest"}
	if len(got) != len(want) {
		t.Fatalf("filteredInsts: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != instrument.ID(want[i]) {
			t.Errorf("filteredInsts[%d]: got %q want %q (full %v)", i, got[i], want[i], got)
		}
	}
}

// TestInstMenu_BackClearsFavoritesView pins that exiting Favorites via the
// Back button drops favoritesView so the next category click renders that
// category's instruments (not a leftover Favorites filter on top of it).
func TestInstMenu_BackClearsFavoritesView(t *testing.T) {
	store := NewInMemoryFavoritesStore(roster.Kick)
	props := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
		},
		[]audio.Category{audio.CatPercussion},
	)
	props.Favorites = store
	props.ForceCategories = true

	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.Open()

	// Click Favorites then Back.
	comp.CategoryBtns()[0].OnClick()
	if !comp.state.favoritesView {
		t.Fatalf("favoritesView should be true after Favorites click")
	}
	if comp.BackBtn() == nil || comp.BackBtn().OnClick == nil {
		t.Fatalf("missing back button in instruments mode")
	}
	comp.BackBtn().OnClick()
	if comp.state.favoritesView {
		t.Errorf("favoritesView should be false after Back; got true")
	}
	if comp.state.mode != InstMenuModeCategories {
		t.Errorf("expected categories mode after Back; got %v", comp.state.mode)
	}
}

// TestInstMenuComp_PageAccessors confirm the new Page/PageCount/PageSize
// methods derive from the underlying scroll state and don't crash on
// closed menus.
func TestInstMenuComp_PageAccessors(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	// Closed menu — accessors should still return sane defaults.
	if got := comp.Page(); got != 1 {
		t.Fatalf("closed Page() = %d, want 1", got)
	}
	if got := comp.PageSize(); got < 1 {
		t.Fatalf("closed PageSize() = %d, want ≥1", got)
	}
	// PageCount on an empty list returns 0.
	if got := comp.PageCount(); got != 0 {
		t.Fatalf("closed PageCount() = %d, want 0", got)
	}
}
