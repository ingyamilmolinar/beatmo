//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// makeInstMenuWithFiltered constructs a configured component with N
// instruments in a single category so paginated tests have a deterministic
// flat list to drive.
func makeInstMenuWithFiltered(t *testing.T, n int) *InstrumentMenuComponent {
	t.Helper()
	comp := NewInstrumentMenuComponent()
	insts := make([]InstrumentOption, n)
	for i := 0; i < n; i++ {
		insts[i] = InstrumentOption{
			ID:       instrument.ID("id-" + itoaPad(i)),
			Label:    "Item " + itoaPad(i),
			Category: audio.CatPercussion,
		}
	}
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:      image.Rect(0, 0, 200, 24),
		VertBounds:      image.Rect(0, 0, 400, 800),
		RowHeight:       24,
		Categories:      []audio.Category{audio.CatPercussion},
		Instruments:     insts,
		ForceCategories: false,
	})
	comp.Open()
	return comp
}

func TestInstMenuComp_BreadcrumbStripClickPopsToCategories(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:      image.Rect(0, 0, 200, 24),
		VertBounds:      image.Rect(0, 0, 400, 800),
		RowHeight:       24,
		Categories:      []audio.Category{audio.CatPercussion, audio.CatLead},
		Instruments:     []InstrumentOption{{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion}},
		ForceCategories: true,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("precondition: ForceCategories should open in categories mode")
	}
	// Drill into Drums.
	comp.state.mode = InstMenuModeInstruments
	comp.state.activeCat = audio.CatPercussion
	comp.rebuildMenu()
	path := comp.BreadcrumbPath()
	if len(path) != 2 {
		t.Fatalf("expected breadcrumb depth 2, got %v", path)
	}
	if len(comp.breadcrumbRects) < 2 {
		t.Fatalf("expected ≥2 breadcrumb rects, got %d", len(comp.breadcrumbRects))
	}
	// Click on the first segment ("Categories"): popBreadcrumbTo(0)
	// should return to categories mode.
	root := comp.breadcrumbRects[0]
	cx := root.Min.X + root.Dx()/2
	cy := root.Min.Y + root.Dy()/2
	if d := comp.breadcrumbHitAt(cx, cy); d != 0 {
		t.Fatalf("breadcrumbHitAt(%d,%d) = %d, want 0", cx, cy, d)
	}
	comp.popBreadcrumbTo(0)
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("after popBreadcrumbTo(0), Mode = %v, want categories", comp.Mode())
	}
}

func TestInstMenuComp_LargeListScrollsWithoutPaginationStrip(t *testing.T) {
	// The pagination strip was removed (2026-07-04 design pass): it was a
	// second navigation affordance layered on a list that already scrolls,
	// and its bottom-band layout clipped off-panel/off-screen. A large list
	// must instead expose the scrollbar, and every laid-out row must sit
	// inside the panel bounds.
	comp := makeInstMenuWithFiltered(t, 100)
	if comp.PageCount() <= 1 {
		t.Fatalf("precondition: many instruments should produce >1 page; got %d", comp.PageCount())
	}
	if !comp.scroll.HasScroll() {
		t.Fatal("large list must be scrollable")
	}
	for i, b := range comp.InstBtns() {
		if !b.Rect().In(comp.Bounds()) {
			t.Fatalf("row %d rect %v outside panel bounds %v", i, b.Rect(), comp.Bounds())
		}
	}
}

func TestInstMenuComp_JumpToPageClampsAndAdvancesScroll(t *testing.T) {
	comp := makeInstMenuWithFiltered(t, 50)
	pc := comp.PageCount()
	if pc <= 1 {
		t.Fatalf("precondition: PageCount must be >1; got %d", pc)
	}
	comp.jumpToPage(pc)
	if got := comp.Page(); got != pc {
		t.Fatalf("jumpToPage(%d) → Page() = %d, want %d", pc, got, pc)
	}
	comp.jumpToPage(0)
	if got := comp.Page(); got != 1 {
		t.Fatalf("jumpToPage(0) clamped → Page() = %d, want 1", got)
	}
	comp.jumpToPage(99999)
	if got := comp.Page(); got != pc {
		t.Fatalf("jumpToPage(99999) clamped → Page() = %d, want %d", got, pc)
	}
}

// pressKey simulates one rising-edge keystroke on the component:
// press, Update (rising edge fires), release, Update (edge resets).
// Mirrors the keyEdge map's two-frame semantics.
func pressKey(comp *InstrumentMenuComponent, ks *keyState, k ebiten.Key) {
	ks.press(k)
	comp.Update()
	ks.release(k)
	comp.Update()
}

func TestInstMenuComp_KeyboardArrowAdvancesSelection(t *testing.T) {
	ks := installInputStub(t)
	comp := makeInstMenuWithFiltered(t, 10)
	// A freshly opened menu highlights NOTHING: lighting a row on open would
	// imply a default the user never chose, and Enter would then fire it.
	// (Before the shared-MenuKeyNav refactor this menu opened on row 0.)
	nav := comp.MenuKeyNavForTest()
	if nav.Selected() != MenuKeyNavNoSelection {
		t.Fatalf("initial selection = %d, want %d (nothing highlighted)",
			nav.Selected(), MenuKeyNavNoSelection)
	}
	pressKey(comp, ks, ebiten.KeyDown)
	if nav.Selected() != 0 {
		t.Fatalf("after Down: selection = %d, want 0", nav.Selected())
	}
	pressKey(comp, ks, ebiten.KeyDown)
	if nav.Selected() != 1 {
		t.Fatalf("after second Down: selection = %d, want 1", nav.Selected())
	}
	pressKey(comp, ks, ebiten.KeyUp)
	if nav.Selected() != 0 {
		t.Fatalf("after Up: selection = %d, want 0", nav.Selected())
	}
}

// TestInstMenuComp_KeyboardEnterPreviewsThenSecondEnterCloses pins the
// preview/confirm rule at the component level.
//
// This test used to assert that ONE Enter both selected and closed, which made
// the keyboard mean something different from a click (a click previews and
// leaves the menu open). Enter now previews like a click, and the second Enter
// on the same row is the confirm.
func TestInstMenuComp_KeyboardEnterPreviewsThenSecondEnterCloses(t *testing.T) {
	ks := installInputStub(t)
	called := 0
	got := ""
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(0, 0, 200, 24),
		VertBounds:  image.Rect(0, 0, 400, 800),
		RowHeight:   24,
		Instruments: []InstrumentOption{{ID: roster.Kick, Label: "Kick"}, {ID: roster.Snare, Label: "Snare"}},
		OnSelect:    func(id instrument.ID) { called++; got = string(id) },
	})
	comp.Open()
	comp.MenuKeyNavForTest().BindHost(comp)
	comp.MenuKeyNavForTest().SetSelected(1)

	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	comp.Update()

	if called != 1 {
		t.Fatalf("Enter must invoke OnSelect; called=%d", called)
	}
	if got != "snare" {
		t.Fatalf("Enter selected id=%q, want snare", got)
	}
	if !comp.IsOpen() {
		t.Fatalf("the FIRST Enter closed the menu; it must preview and stay open")
	}

	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	comp.Update()

	if comp.IsOpen() {
		t.Fatalf("the SECOND Enter on the same row must confirm and close")
	}
	if called != 1 {
		t.Fatalf("the confirming Enter re-fired OnSelect (called=%d, want 1); "+
			"confirming an already-applied instrument must not re-apply it", called)
	}
}

func TestInstMenuComp_KeyboardPageDownAdvancesScrollByVisible(t *testing.T) {
	ks := installInputStub(t)
	comp := makeInstMenuWithFiltered(t, 50)
	first := comp.scroll.VS.First
	vis := comp.scroll.VS.Visible
	ks.press(ebiten.KeyPageDown)
	comp.Update()
	ks.release(ebiten.KeyPageDown)
	if got := comp.scroll.VS.First; got != first+vis {
		t.Fatalf("PgDn: scroll.First = %d, want %d (first + visible)", got, first+vis)
	}
}

func TestInstMenuComp_KeyboardHomeAndEndJumps(t *testing.T) {
	ks := installInputStub(t)
	comp := makeInstMenuWithFiltered(t, 50)
	// Move first away from 0, then Home returns it.
	comp.scroll.VS.First = 20
	ks.press(ebiten.KeyHome)
	comp.Update()
	ks.release(ebiten.KeyHome)
	if comp.scroll.VS.First != 0 {
		t.Fatalf("Home should reset scroll.First to 0; got %d", comp.scroll.VS.First)
	}
	ks.press(ebiten.KeyEnd)
	comp.Update()
	ks.release(ebiten.KeyEnd)
	wantFirst := comp.scroll.VS.Total - comp.scroll.VS.Visible
	if wantFirst < 0 {
		wantFirst = 0
	}
	if got := comp.scroll.VS.First; got != wantFirst {
		t.Fatalf("End should set scroll.First to last page (%d); got %d", wantFirst, got)
	}
}

func TestInstMenuComp_SlashFocusesSearch(t *testing.T) {
	ks := installInputStub(t)
	comp := makeInstMenuWithFiltered(t, 5)
	if comp.searchBox != nil && comp.searchBox.Focused() {
		t.Fatalf("precondition: search box must start unfocused")
	}
	ks.press(ebiten.KeySlash)
	comp.Update()
	ks.release(ebiten.KeySlash)
	if comp.searchBox == nil || !comp.searchBox.Focused() {
		t.Fatalf("'/' did not focus search box")
	}
}

func TestInstMenuComp_KeyboardNavSkippedOnMobile(t *testing.T) {
	// When the layout profile reports mobile (small screen), keyboard
	// handlers early-return so a paired Bluetooth keyboard doesn't move
	// the selection unexpectedly.
	old := forceSmallScreenForTest
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() {
		forceSmallScreenForTest = old
		UpdateProfile()
	})
	if !Profile().IsMobile() {
		t.Fatalf("precondition: forceSmallScreenForTest should produce mobile profile")
	}
	ks := installInputStub(t)
	comp := makeInstMenuWithFiltered(t, 5)
	before := comp.MenuKeyNavForTest().Selected()
	ks.press(ebiten.KeyDown)
	comp.Update()
	ks.release(ebiten.KeyDown)
	if got := comp.MenuKeyNavForTest().Selected(); got != before {
		t.Fatalf("mobile path: keyboard should not move selection; before=%d after=%d", before, got)
	}
}
