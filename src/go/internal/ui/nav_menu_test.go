//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestNavAndFileMenusAreSeparatePortals asserts the split: the nav menu
// (OpenNavigation) is its OWN portal component (navMenuComp), NOT the overflow
// file portal; the file menu (OpenFileMenu) opens the overflow portal whose
// items carry the FILE header and no nav content.
func TestNavAndFileMenusAreSeparatePortals(t *testing.T) {
	g := newGameWithStart(t)

	// Nav menu: its own portal component, overflow (file) menu NOT open.
	g.drum.OpenNavigation()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("OpenNavigation must open the nav-menu portal component")
	}
	if g.drum.IsOverflowMenuOpen() {
		t.Fatal("OpenNavigation must NOT open the overflow (file) menu")
	}
	g.drum.closeNavMenuPortal()

	// File menu: overflow portal with the FILE header; nav menu NOT open.
	g.drum.OpenFileMenu()
	if !g.drum.IsOverflowMenuOpen() {
		t.Fatal("OpenFileMenu must open the overflow portal")
	}
	if g.drum.IsNavMenuOpen() {
		t.Fatal("OpenFileMenu must NOT open the nav menu")
	}
	fileHdr := false
	for _, it := range g.drum.overflowItems() {
		if it.header && it.label == i18n.T(i18n.KeyMenuFile) {
			fileHdr = true
		}
	}
	if !fileHdr {
		t.Fatal("file menu must contain the FILE section header")
	}
}

// TestNavPrevNextJumpByAmountAndUnit drives the NavigationPanel's callbacks
// directly (bypassing gesture simulation) to prove OnNext/OnPrev seek by the
// live jumpBeats amount, honoring jumpUnitBeats: true -> whole beats
// (RequestSeekDeltaBeats), false -> raw subdivisions (RequestSeekDeltaSteps).
func TestNavPrevNextJumpByAmountAndUnit(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(20*div, seekOriginJS)
	g.drum.jumpBeats = 4
	g.drum.jumpUnitBeats = true
	g.drum.navPanel.cb.OnNext() // +4 beats
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 24*div {
		t.Fatalf("next(beats)=%d want %d", got, 24*div)
	}
	g.drum.jumpUnitBeats = false
	g.drum.navPanel.cb.OnPrev() // -4 subdivisions
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 24*div-4 {
		t.Fatalf("prev(subdiv)=%d want %d", got, 24*div-4)
	}
}

// TestNavAmountPickerStacksOverNavMenu guards the portal-stacking: opening the
// ×N amount dropdown from inside the nav menu must stack the picker OVER the nav
// menu (both portal entries alive, picker on top), not tear the nav menu down.
func TestNavAmountPickerStacksOverNavMenu(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must be open before exercising the nested picker")
	}
	g.drum.navPanel.cb.OnPickAmount()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must remain open while the amount picker is stacked over it")
	}
	if top := g.drum.portal().TopID(); top != "jump-menu" {
		t.Fatalf("topmost portal entry = %q, want %q (picker must stack ABOVE the nav menu)", top, "jump-menu")
	}
	if n := g.drum.portal().StackLen(); n < 2 {
		t.Fatalf("portal stack len=%d, want >=2 (nav-menu + jump-menu both present)", n)
	}
}

// TestNavGotoBeatEditorOpensOverNavMenu: the go-to-beat editor (owned by the nav
// menu component) must activate and stay open while the nav menu remains open.
func TestNavGotoBeatEditorOpensOverNavMenu(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	g.drum.navPanel.cb.OnGotoBeat()
	ed := g.drum.navMenuComp.Editor()
	if ed == nil || !ed.Active() {
		t.Fatal("go-to-beat editor must be active after OnGotoBeat")
	}
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must remain open while the go-to-beat editor is showing")
	}
	if !g.drum.navMenuComp.ClaimsKeyboard() {
		t.Fatal("nav menu must claim the keyboard while its go-to editor is active")
	}
}

// TestNavAmountPickerSelectionMutatesJumpBeats closes the coverage gap left by
// deleting transport_jump_test.go's TestJumpSizeConfigurable (Task-5 review
// M1): opening the NAVIGATION panel's ×N amount menu and picking an option
// must actually update dv.jumpBeats — the value NavigationPanel reads for its
// prev/next jump distance. Drives the real OnPickAmount → OpenJumpMenu path and
// fires the option button's OnSelect through the component (press edge), rather
// than poking the field directly.
func TestNavAmountPickerSelectionMutatesJumpBeats(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	g.drum.jumpBeats = 4 // known default before the pick
	g.drum.navPanel.cb.OnPickAmount()
	comp := g.drum.jumpMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("amount picker did not open")
	}
	// Options are {1,2,4,8,16}; index 3 is value 8 (different from the default).
	if len(comp.buttons) < 5 {
		t.Fatalf("expected 5 jump options {1,2,4,8,16}, got %d", len(comp.buttons))
	}
	suppressClicksUntilRelease = false
	r := comp.buttons[3].Rect()
	comp.HandleInput(r.Min.X+2, r.Min.Y+2, true) // press fires OnSelect(8)
	if g.drum.jumpBeats != 8 {
		t.Fatalf("picking ×8 must set jumpBeats=8, got %d", g.drum.jumpBeats)
	}
}
