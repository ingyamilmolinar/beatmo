//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

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

// Beats seek whole beats, subdiv seeks raw steps.
func TestNavPrevNextJumpByAmountAndUnit(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(20*div, hooks.SeekJS)
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

// Both portal entries stay alive; the picker must not tear the nav menu down.
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

// Seated on the playhead the moment the menu opens.
func TestNavGotoWheelIsInlineInTheNavMenu(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.OpenNavigation()
	g.Draw(ebiten.NewImage(640, 480))
	p := g.drum.navPanel
	wr := p.GotoWheelRect()
	if wr.Empty() || !wr.In(g.drum.navMenuComp.card) {
		t.Fatalf("go-to wheel %v must sit inside the nav card %v", wr, g.drum.navMenuComp.card)
	}
	if p.PendingSteps() != g.playheadAbsSubdiv() {
		t.Fatalf("wheel seated at %d, playhead %d", p.PendingSteps(), g.playheadAbsSubdiv())
	}
}

// Through the real OnPickAmount → OpenJumpMenu path and the option's press edge, never by poking the field.
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
