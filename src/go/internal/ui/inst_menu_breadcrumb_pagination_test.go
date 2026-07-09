//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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
			ID:       "id-" + itoaPad(i),
			Label:    "Item " + itoaPad(i),
			Category: "Drums",
		}
	}
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:      image.Rect(0, 0, 200, 24),
		VertBounds:      image.Rect(0, 0, 400, 800),
		RowHeight:       24,
		Categories:      []string{"Drums"},
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
		Categories:      []string{"Drums", "Synths"},
		Instruments:     []InstrumentOption{{ID: "kick", Label: "Kick", Category: "Drums"}},
		ForceCategories: true,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeCategories {
		t.Fatalf("precondition: ForceCategories should open in categories mode")
	}
	// Drill into Drums.
	comp.state.mode = InstMenuModeInstruments
	comp.state.activeCat = "Drums"
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
	if comp.selectedIdx != 0 {
		t.Fatalf("initial selectedIdx = %d, want 0", comp.selectedIdx)
	}
	pressKey(comp, ks, ebiten.KeyDown)
	if comp.selectedIdx != 1 {
		t.Fatalf("after Down: selectedIdx = %d, want 1", comp.selectedIdx)
	}
	pressKey(comp, ks, ebiten.KeyDown)
	if comp.selectedIdx != 2 {
		t.Fatalf("after second Down: selectedIdx = %d, want 2", comp.selectedIdx)
	}
	pressKey(comp, ks, ebiten.KeyUp)
	if comp.selectedIdx != 1 {
		t.Fatalf("after Up: selectedIdx = %d, want 1", comp.selectedIdx)
	}
}

func TestInstMenuComp_KeyboardEnterSelectsAndCloses(t *testing.T) {
	ks := installInputStub(t)
	called := 0
	got := ""
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(0, 0, 200, 24),
		VertBounds:  image.Rect(0, 0, 400, 800),
		RowHeight:   24,
		Instruments: []InstrumentOption{{ID: "kick", Label: "Kick"}, {ID: "snare", Label: "Snare"}},
		OnSelect:    func(id string) { called++; got = id },
	})
	comp.Open()
	comp.selectedIdx = 1
	ks.press(ebiten.KeyEnter)
	comp.Update()
	ks.release(ebiten.KeyEnter)
	if called != 1 {
		t.Fatalf("Enter must invoke OnSelect; called=%d", called)
	}
	if got != "snare" {
		t.Fatalf("Enter selected id=%q, want snare", got)
	}
	if comp.IsOpen() {
		t.Fatalf("menu should close after Enter")
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
	before := comp.selectedIdx
	ks.press(ebiten.KeyDown)
	comp.Update()
	ks.release(ebiten.KeyDown)
	if comp.selectedIdx != before {
		t.Fatalf("mobile path: keyboard should not move selection; before=%d after=%d", before, comp.selectedIdx)
	}
}
