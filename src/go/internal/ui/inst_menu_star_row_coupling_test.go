package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The star column is a per-row adornment: exactly one star belongs to each
// instrument row currently laid out by the menu, and nothing else. These
// tests pin that coupling structurally so a build path that produces zero
// rows can never leave a previous build's stars behind — the "stars float
// down the screen over an empty Favorites menu" bug.

// starCouplingProps builds an instruments-with-categories menu wired to a
// favorites store, sized so several rows are visible at once.
func starCouplingProps(store FavoritesStore) InstrumentMenuProps {
	p := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
			{ID: roster.Hihat, Label: "Hat", Category: audio.CatPercussion},
			{ID: roster.Clap, Label: "Clap", Category: audio.CatPercussion},
		},
		[]audio.Category{audio.CatPercussion},
	)
	p.Favorites = store
	p.ForceCategories = true
	return p
}

// firstStarRow returns the index, rect and instrument id of the first row
// that carries a drawable star. ok is false when the menu has no stars.
// Tests use this instead of indexing favRects[0] directly: favRects is
// row-indexed, so index 0 is only a star when row 0 happens to have one.
func firstStarRow(m *InstrumentMenuComponent) (idx int, rect image.Rectangle, id string, ok bool) {
	for i, r := range m.favRects {
		if r.Empty() || i >= len(m.instIDs) || m.instIDs[i] == "" {
			continue
		}
		return i, r, string(m.instIDs[i]), true
	}
	return -1, image.Rectangle{}, "", false
}

// nonEmptyStars counts the star rects that would actually be drawn.
func nonEmptyStars(m *InstrumentMenuComponent) int {
	n := 0
	for _, r := range m.favRects {
		if !r.Empty() {
			n++
		}
	}
	return n
}

// TestInstMenu_EmptyFavoritesViewDrawsNoStars reproduces the reported bug:
// browse a populated category (which lays out one star per row), navigate
// back to the category list, then open the empty Favorites view. The menu
// shows only the "no favorites yet" placeholder, so there must be zero star
// rects — previously the category listing's stars survived the empty-list
// build and were drawn (and hit-tested) far outside the shrunken menu.
func TestInstMenu_EmptyFavoritesViewDrawsNoStars(t *testing.T) {
	store := NewInMemoryFavoritesStore() // no ★s at all
	comp := NewInstrumentMenuComponent()
	comp.SetProps(starCouplingProps(store))
	comp.Open()

	// Drill into "Drums" (index 0 is the virtual Favorites row).
	cats := comp.CategoryBtns()
	if len(cats) < 2 {
		t.Fatalf("expected Favorites + Drums category rows, got %d", len(cats))
	}
	cats[1].OnClick()
	if nonEmptyStars(comp) == 0 {
		t.Fatalf("precondition: category listing should lay out star rects, got 0")
	}

	// Back to categories, then into the empty Favorites view.
	comp.BackBtn().OnClick()
	comp.CategoryBtns()[0].OnClick()

	if !comp.state.favoritesView || len(comp.state.filteredInsts) != 0 {
		t.Fatalf("precondition: expected empty favorites view, favoritesView=%v filtered=%v",
			comp.state.favoritesView, comp.state.filteredInsts)
	}
	if got := nonEmptyStars(comp); got != 0 {
		t.Fatalf("empty favorites view left %d stale star rect(s): %v", got, comp.favRects)
	}
}

// TestInstMenu_CategoriesModeDrawsNoStars pins that popping back to the
// category list drops the instrument rows' stars. Categories mode renders no
// instrument rows, and its global-search tap path hit-tests favRects — stale
// rects there would toggle a favorite for an instrument that is not on screen.
func TestInstMenu_CategoriesModeDrawsNoStars(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(starCouplingProps(store))
	comp.Open()

	comp.CategoryBtns()[1].OnClick() // into Drums
	if nonEmptyStars(comp) == 0 {
		t.Fatalf("precondition: category listing should lay out star rects, got 0")
	}
	comp.BackBtn().OnClick() // back to categories

	if comp.state.mode != InstMenuModeCategories {
		t.Fatalf("expected categories mode, got %v", comp.state.mode)
	}
	if got := nonEmptyStars(comp); got != 0 {
		t.Fatalf("categories mode left %d stale star rect(s): %v", got, comp.favRects)
	}
}

// TestInstMenu_NoMatchesDrawsNoStars covers the other empty-list path: a
// search query inside a category that matches nothing. Same early return,
// same stale-star exposure.
func TestInstMenu_NoMatchesDrawsNoStars(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	comp.SetProps(starCouplingProps(store))
	comp.Open()

	comp.CategoryBtns()[1].OnClick()
	if nonEmptyStars(comp) == 0 {
		t.Fatalf("precondition: category listing should lay out star rects, got 0")
	}
	comp.SetSearchText("zzzzzzz")

	if len(comp.state.filteredInsts) != 0 {
		t.Fatalf("precondition: expected no matches, got %v", comp.state.filteredInsts)
	}
	if got := nonEmptyStars(comp); got != 0 {
		t.Fatalf("no-matches view left %d stale star rect(s): %v", got, comp.favRects)
	}
}

// TestInstMenu_StarRectsPairWithVisibleRows is the structural invariant that
// makes the three cases above impossible by construction: favRects is
// parallel to instBtns, and every non-empty star sits on its row's Y band
// inside the menu's own rect. A star that outlives its row breaks this.
func TestInstMenu_StarRectsPairWithVisibleRows(t *testing.T) {
	store := NewInMemoryFavoritesStore(roster.Kick)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(starCouplingProps(store))
	comp.Open()
	comp.CategoryBtns()[1].OnClick()
	assertStarRowParallel(t, comp, "populated category")

	full := comp.fullRect
	for i, star := range comp.favRects {
		if star.Empty() {
			continue
		}
		if !star.In(full) {
			t.Errorf("star[%d] %v escapes menu rect %v", i, star, full)
		}
		row := comp.instBtns[i].Rect()
		if star.Min.Y >= row.Max.Y || star.Max.Y <= row.Min.Y {
			t.Errorf("star[%d] %v does not share a Y band with its row %v", i, star, row)
		}
		if star.Min.X < row.Max.X {
			t.Errorf("star[%d] %v overlaps its row %v horizontally", i, star, row)
		}
	}

	// The invariant must hold in the empty-list state too — that is the
	// state the bug shipped in. The placeholder row is a row: it gets an
	// (empty) star slot and an (empty) id slot like every other.
	store.Set(roster.Kick, false)
	comp.BackBtn().OnClick()
	comp.CategoryBtns()[0].OnClick()
	assertStarRowParallel(t, comp, "empty favorites view")
}

// assertStarRowParallel checks the per-row slices stay index-aligned so the
// star for row i is always instIDs[i]'s star and never a leftover.
func assertStarRowParallel(t *testing.T, m *InstrumentMenuComponent, when string) {
	t.Helper()
	if len(m.favRects) != len(m.instBtns) {
		t.Fatalf("%s: favRects len %d != instBtns len %d — star column is not row-parallel",
			when, len(m.favRects), len(m.instBtns))
	}
	if len(m.instIDs) != len(m.instBtns) {
		t.Fatalf("%s: instIDs len %d != instBtns len %d — star ids are not row-derived",
			when, len(m.instIDs), len(m.instBtns))
	}
}

// TestInstMenu_EmptyAnchorDropsStars pins the last early return: a menu whose
// anchor collapses (row removed, panel closed) lays out nothing, so it must
// carry no stars either.
func TestInstMenu_EmptyAnchorDropsStars(t *testing.T) {
	store := NewInMemoryFavoritesStore()
	comp := NewInstrumentMenuComponent()
	props := starCouplingProps(store)
	comp.SetProps(props)
	comp.Open()
	comp.CategoryBtns()[1].OnClick()
	if nonEmptyStars(comp) == 0 {
		t.Fatalf("precondition: category listing should lay out star rects, got 0")
	}

	props.AnchorRect = image.Rectangle{}
	comp.SetProps(props)
	comp.rebuildMenu()

	if got := nonEmptyStars(comp); got != 0 {
		t.Fatalf("collapsed anchor left %d stale star rect(s): %v", got, comp.favRects)
	}
}
