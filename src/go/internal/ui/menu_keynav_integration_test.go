//go:build test

// Real Game.Update with injected keys, so a menu that never pumps its nav, never claims the keyboard or
// activates the wrong row fails; each case asserts the menu's OWN action happened, not only the highlight.

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/hajimehoshi/ebiten/v2"
)

// Edge-detected: each press is one frame down, then one up.
type keyDriver struct {
	g      *Game
	held   ebiten.Key
	screen *ebiten.Image
}

func newKeyDriver(t *testing.T) *keyDriver {
	t.Helper()
	assertDefaultParityState(t)
	d := &keyDriver{held: -1, screen: ebiten.NewImage(1280, 720)}
	restore := SetInputForTest(
		func() (int, int) { return -100, -100 }, // pointer parked off every menu
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == d.held },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	t.Cleanup(restore)
	d.g = New(testLogger)
	t.Cleanup(d.g.CloseForTest)
	d.g.Layout(1280, 720)
	return d
}

func (d *keyDriver) frame() {
	_ = d.g.Update()
	d.g.Draw(d.screen)
}

func (d *keyDriver) settle(n int) {
	for i := 0; i < n; i++ {
		d.frame()
	}
}

// press taps a key: one frame held (the rising edge), one frame released.
func (d *keyDriver) press(k ebiten.Key) {
	d.held = k
	d.frame()
	d.held = -1
	d.frame()
}

// menuNavCase describes one menu's keyboard contract.
type menuNavCase struct {
	name string
	// open puts the menu on screen.
	open func(t *testing.T, g *Game)
	// nav returns the menu's live MenuKeyNav (nil until wired).
	nav func(g *Game) *MenuKeyNav
	// effect snapshots whatever Enter is supposed to change.
	effect func(g *Game) string
	// isOpen reports whether the menu is still on screen — the Esc contract.
	isOpen func(g *Game) bool
	// To a row whose activation is observable: some first rows are inert here (Templates opens on "Back", File on
	// "Upload", which raises a dialog), so those go to the end.
	reach func(d *keyDriver)
}

// downOnce highlights the first selectable row.
func downOnce(d *keyDriver) { d.press(ebiten.KeyArrowDown) }

// toLastRow jumps to the last selectable row via End.
func toLastRow(d *keyDriver) { d.press(ebiten.KeyEnd) }

// The real items raise file dialogs and reset recipes, so the case wires inert ones.
var actionMenuNavPick string

func menuNavCases() []menuNavCase {
	return []menuNavCase{
		{
			// The shared "⋯" overflow and the synth "+ Add stage" list.
			name:   "action_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has("test-action-menu") },
			open: func(t *testing.T, g *Game) {
				actionMenuNavPick = ""
				t.Cleanup(func() { actionMenuNavPick = "" })
				g.drum.openActionMenu("test-action-menu", image.Rect(40, 40, 160, 64),
					// Six rows, so the cursor has not clamped at the end before the first repeat is due.
					[]actionMenuItem{
						{Label: "Alpha", OnSelect: func() { actionMenuNavPick = "alpha" }},
						{Label: "Beta", OnSelect: func() { actionMenuNavPick = "beta" }},
						{Label: "Gamma", OnSelect: func() { actionMenuNavPick = "gamma" }},
						{Label: "Delta", OnSelect: func() { actionMenuNavPick = "delta" }},
						{Label: "Epsilon", OnSelect: func() { actionMenuNavPick = "epsilon" }},
						{Label: "Zeta", OnSelect: func() { actionMenuNavPick = "zeta" }},
					})
			},
			nav:    func(g *Game) *MenuKeyNav { return g.drum.ActionMenuKeyNavForTest() },
			effect: func(g *Game) string { return "actionPick=" + actionMenuNavPick },
			reach:  downOnce,
		},
		{
			name:   "subdiv_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has("subdiv-menu") },
			open:   func(t *testing.T, g *Game) { g.drum.OpenSubdivMenu() },
			nav:    func(g *Game) *MenuKeyNav { return g.drum.subdivMenuComp.MenuKeyNavForTest() },
			effect: func(g *Game) string {
				return "subdiv=" + navItoa(g.grid.MaxDiv())
			},
			reach: downOnce,
		},
		{
			// A second instance: the wiring belongs to the component, not one call site.
			name:   "jump_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has("jump-menu") },
			open:   func(t *testing.T, g *Game) { g.drum.OpenJumpMenu() },
			nav:    func(g *Game) *MenuKeyNav { return g.drum.jumpMenuComp.MenuKeyNavForTest() },
			effect: func(g *Game) string {
				return "jumpBeats=" + navItoa(g.drum.jumpBeats)
			},
			reach: downOnce,
		},
		{
			name:   "template_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has(overflowMenuID) },
			open: func(t *testing.T, g *Game) {
				g.drum.overflowPage = 1
				g.drum.OpenOverflowMenu()
			},
			nav: func(g *Game) *MenuKeyNav { return g.drum.OverflowKeyNavForTest() },
			effect: func(g *Game) string {
				return "loadedTemplate=" + g.drum.templateLoadedStem
			},
			// Row 0 of the Templates page is "Back"; End lands on a real template.
			reach: toLastRow,
		},
		{
			name:   "file_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has(overflowMenuID) },
			open: func(t *testing.T, g *Game) {
				g.drum.overflowPage = 0
				g.drum.OpenOverflowMenu()
			},
			nav: func(g *Game) *MenuKeyNav { return g.drum.OverflowKeyNavForTest() },
			effect: func(g *Game) string {
				return "overflowPage=" + navItoa(g.drum.overflowPage)
			},
			// The last row is "Load template"; the first, Upload, raises a native dialog.
			reach: toLastRow,
		},
		{
			name:   "context_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has("context-menu") },
			open:   func(t *testing.T, g *Game) { g.drum.OpenContextMenu(0) },
			nav:    func(g *Game) *MenuKeyNav { return g.drum.ContextKeyNavForTest() },
			effect: func(g *Game) string {
				return "contextOpen=" + navBtoa(g.drum.IsContextMenuOpen())
			},
			reach: downOnce,
		},
		{
			name:   "theme_menu",
			isOpen: func(g *Game) bool { return g.drum.portal().Has(themeOverlayID) },
			open: func(t *testing.T, g *Game) {
				t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
				g.toggleThemeOverlay()
			},
			nav: func(g *Game) *MenuKeyNav {
				ov, _ := g.drum.portal().TopOverlay().(*ThemeOverlay)
				if ov == nil {
					return nil
				}
				return ov.MenuKeyNavForTest()
			},
			effect: func(g *Game) string {
				return "preset=" + effectivePresetID(g.userTheme, g.circuitTheme)
			},
			// Row 0 is the default preset, already in effect.
			reach: func(d *keyDriver) { d.press(ebiten.KeyArrowDown); d.press(ebiten.KeyArrowDown) },
		},
		{
			name:   "instrument_menu",
			isOpen: func(g *Game) bool { return g.drum.IsInstMenuOpen() },
			open:   func(t *testing.T, g *Game) { g.drum.OpenInstrumentMenu(0) },
			nav:    func(g *Game) *MenuKeyNav { return g.drum.instMenuComp.MenuKeyNavForTest() },
			effect: func(g *Game) string {
				if len(g.drum.Rows) == 0 {
					return "norows"
				}
				return "inst=" + string(g.drum.Rows[0].Instrument)
			},
			// Row 0's instrument is already selected, so End picks one that visibly differs.
			reach: toLastRow,
		},
	}
}

func navItoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func navBtoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func TestMenuKeyNav_ArrowsMoveHighlight(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			nav := tc.nav(d.g)
			if nav == nil {
				t.Fatalf("%s has no MenuKeyNav — the menu is not wired for keyboard navigation", tc.name)
			}
			if got := nav.Selected(); got != MenuKeyNavNoSelection {
				t.Fatalf("menu opened with row %d already highlighted; want nothing "+
					"highlighted until the user acts", got)
			}

			d.press(ebiten.KeyArrowDown)
			first := nav.Selected()
			if first == MenuKeyNavNoSelection {
				t.Fatalf("ArrowDown did not highlight anything (Selected=%d)", first)
			}

			d.press(ebiten.KeyArrowDown)
			second := nav.Selected()
			if second <= first {
				t.Errorf("second ArrowDown moved the highlight from %d to %d; it must advance",
					first, second)
			}

			d.press(ebiten.KeyArrowUp)
			if got := nav.Selected(); got != first {
				t.Errorf("ArrowUp returned to %d, want %d", got, first)
			}
		})
	}
}

// Enter does what clicking the highlighted row would.
func TestMenuKeyNav_EnterPerformsTheRowAction(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			nav := tc.nav(d.g)
			if nav == nil {
				t.Fatalf("%s has no MenuKeyNav", tc.name)
			}
			before := tc.effect(d.g)

			tc.reach(d)
			if nav.Selected() == MenuKeyNavNoSelection {
				t.Fatalf("nothing highlighted after navigating")
			}
			d.press(ebiten.KeyEnter)
			d.settle(5)

			if after := tc.effect(d.g); after == before {
				t.Errorf("Enter on the highlighted row changed nothing (still %q). "+
					"Arrow selection without a working Enter is a dead end.", before)
			}
		})
	}
}

// An open menu owns the arrows, or every keystroke also pans the grid behind it.
func TestMenuKeyNav_ArrowsDoNotPanGridWhileMenuOpen(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			beforeX, beforeY := d.g.cam.OffsetX, d.g.cam.OffsetY
			d.press(ebiten.KeyArrowDown)
			d.press(ebiten.KeyArrowUp)

			if d.g.cam.OffsetX != beforeX || d.g.cam.OffsetY != beforeY {
				t.Errorf("arrows panned the grid camera (%.1f,%.1f -> %.1f,%.1f) while %s was open; "+
					"a nav-enabled menu must claim the keyboard",
					beforeX, beforeY, d.g.cam.OffsetX, d.g.cam.OffsetY, tc.name)
			}
		})
	}
}

// Up/Down walk the filtered results and Enter picks while the search field keeps Left/Right and typing: that
// is exactly where a keyboard user needs the arrows.
func TestMenuKeyNav_ArrowsWorkWhileSearchFocused(t *testing.T) {
	d := newKeyDriver(t)
	d.g.drum.OpenInstrumentMenu(0)
	d.settle(5)

	comp := d.g.drum.instMenuComp
	if comp == nil {
		t.Fatal("instrument menu component missing")
	}
	box := comp.SearchBox()
	if box == nil {
		t.Skip("instrument menu has no search box in this configuration")
	}
	box.SetFocus(true)
	d.settle(2)
	if !box.Focused() {
		t.Fatal("harness: search box did not take focus")
	}

	nav := comp.MenuKeyNavForTest()
	d.press(ebiten.KeyArrowDown)
	if nav.Selected() == MenuKeyNavNoSelection {
		t.Fatalf("ArrowDown did nothing while the search box was focused — "+
			"typing a filter must not kill list navigation (Selected=%d)", nav.Selected())
	}
	first := nav.Selected()

	d.press(ebiten.KeyArrowDown)
	if nav.Selected() <= first {
		t.Errorf("second ArrowDown did not advance the highlight (%d -> %d) while "+
			"the search box was focused", first, nav.Selected())
	}
}

// TextInput consumes Enter for its own submit, so it must reach the list instead.
func TestMenuKeyNav_EnterPicksHighlightedWhileSearchFocused(t *testing.T) {
	d := newKeyDriver(t)
	d.g.drum.OpenInstrumentMenu(0)
	d.settle(5)

	comp := d.g.drum.instMenuComp
	box := comp.SearchBox()
	if box == nil {
		t.Skip("instrument menu has no search box in this configuration")
	}
	box.SetFocus(true)
	d.settle(2)

	before := d.g.drum.Rows[0].Instrument
	d.press(ebiten.KeyEnd) // highlight the last filtered instrument
	nav := comp.MenuKeyNavForTest()
	if nav.Selected() == MenuKeyNavNoSelection {
		t.Fatal("nothing highlighted after End while search focused")
	}
	d.press(ebiten.KeyEnter)
	d.settle(5)

	if d.g.drum.Rows[0].Instrument == before {
		t.Errorf("Enter while the search box was focused did not pick the highlighted "+
			"instrument (still %q) — the text input swallowed it", before)
	}
}

// The list signature must start as a real value: an initial "" made the first frame look like a change and
// wipe the selection.
func TestMenuKeyNav_SelectionSurvivesIdleFrames(t *testing.T) {
	d := newKeyDriver(t)
	d.g.drum.overflowPage = 1
	d.g.drum.OpenOverflowMenu()
	d.settle(5)

	nav := d.g.drum.OverflowKeyNavForTest()
	d.press(ebiten.KeyArrowDown)
	want := nav.Selected()
	if want == MenuKeyNavNoSelection {
		t.Fatal("nothing highlighted after ArrowDown")
	}

	d.settle(30) // menu just sits there
	if got := nav.Selected(); got != want {
		t.Errorf("highlight was %d before 30 idle frames and %d after; it must persist "+
			"while the menu is open and its list unchanged", want, got)
	}
}

// A cursor placed before the first update frame survives it.
func TestMenuKeyNav_PreselectedRowSurvivesFirstFrame(t *testing.T) {
	d := newKeyDriver(t)
	d.g.drum.overflowPage = 1
	d.g.drum.OpenOverflowMenu()

	nav := d.g.drum.OverflowKeyNavForTest()
	nav.BindHost(overflowNavHost{dv: d.g.drum})

	// Pick the first real template row, before any frame has run.
	want := MenuKeyNavNoSelection
	items := d.g.drum.overflowItems()
	for i := range items {
		if !items[i].header && !items[i].search && items[i].onClick != nil && items[i].iconID == IconTemplate {
			want = i
			break
		}
	}
	if want == MenuKeyNavNoSelection {
		t.Fatal("harness: no template row found")
	}
	nav.SetSelected(want)

	d.settle(10)
	if got := nav.Selected(); got != want {
		t.Errorf("a row highlighted before the first frame was cleared by it "+
			"(want %d, got %d)", want, got)
	}
}

// Every list menu closes on Esc, so a keyboard user is never trapped.
func TestMenuKeyNav_EscapeClosesEveryMenu(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			if tc.isOpen == nil {
				t.Fatalf("%s has no isOpen probe", tc.name)
			}
			if !tc.isOpen(d.g) {
				t.Fatalf("%s did not open", tc.name)
			}

			d.press(ebiten.KeyEscape)
			d.settle(3)

			if tc.isOpen(d.g) {
				t.Errorf("Esc did not close %s", tc.name)
			}
		})
	}
}

// After Enter the menu is closed, or open ON PURPOSE with the selection applied (the audition menus), never
// open having done nothing.
func TestMenuKeyNav_EnterIsNeverSwallowed(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			before := tc.effect(d.g)
			wasOpen := tc.isOpen(d.g)

			tc.reach(d)
			d.press(ebiten.KeyEnter)
			d.settle(5)

			if tc.effect(d.g) == before && tc.isOpen(d.g) == wasOpen {
				t.Errorf("Enter on %s changed nothing and left the menu open — the "+
					"keystroke was swallowed", tc.name)
			}
		})
	}
}

// --- Hold-to-repeat, end to end -------------------------------------------

func (d *keyDriver) holdFrames(nav *MenuKeyNav, k ebiten.Key, n int) []int {
	var fired []int
	prev := nav.Selected()
	d.held = k
	for f := 1; f <= n; f++ {
		d.frame()
		if cur := nav.Selected(); cur != prev {
			fired = append(fired, f)
			prev = cur
		}
	}
	d.held = -1
	d.frame()
	return fired
}

// Through Game.Update, so each menu pumps its nav exactly once per frame: twice would repeat at double the
// curve's rate, invisibly to every unit test.
func TestMenuKeyNavRepeat_WorksThroughTheRealGameLoop(t *testing.T) {
	for _, tc := range menuNavCases() {
		t.Run(tc.name, func(t *testing.T) {
			d := newKeyDriver(t)
			tc.open(t, d.g)
			d.settle(5)

			nav := tc.nav(d.g)
			if nav == nil {
				t.Fatalf("%s has no MenuKeyNav", tc.name)
			}
			if nav.MenuItemCountForTest() < 4 {
				t.Skipf("%s has too few rows (%d) to observe a repeat",
					tc.name, nav.MenuItemCountForTest())
			}

			fired := d.holdFrames(nav, ebiten.KeyArrowDown, menuRepeat.Delay+menuRepeat.Interval+2)
			if len(fired) < 2 {
				t.Fatalf("%s: a held ArrowDown moved %d times (%v) in %d frames; the "+
					"repeat never reached this menu through the real loop",
					tc.name, len(fired), fired, menuRepeat.Delay+menuRepeat.Interval+2)
			}
			if got, want := fired[0], 1; got != want {
				t.Errorf("%s: first move on frame %d, want the press edge (%d)",
					tc.name, got, want)
			}
			if got, want := fired[1], menuRepeat.Delay+menuRepeat.Interval; got != want {
				t.Errorf("%s: first REPEAT on frame %d, want %d. Landing early means "+
					"the nav is pumped more than once per frame (hold counter "+
					"advancing at a multiple of the real rate).", tc.name, got, want)
			}
		})
	}
}
