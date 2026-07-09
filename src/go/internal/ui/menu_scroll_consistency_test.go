//go:build test

package ui

import (
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// configScrollBehavior sets up a raw ScrollBehavior the way the context-menu and
// EQ-channel dropdowns do (item-based, a list that overflows the viewport).
func configScrollBehavior(itemH, total, visible int) *ScrollBehavior {
	sb := NewScrollBehavior(DropdownScrollbarStyle, itemH)
	sb.VS.View = image.Rect(0, 0, 100, visible*itemH)
	sb.VS.Total = total
	sb.VS.Visible = visible
	sb.VS.Clamp()
	return sb
}

// TestTickMenuScrollCooldowns_ReleasesEveryMenu proves the single per-frame
// chokepoint (DrumView.tickMenuScrollCooldowns, called from DrumView.Update)
// advances the wheel cooldown for EVERY scrollable menu — so all of them are
// clicky the same way rather than one being permanently locked after its first
// notch. Each menu's scroll is put into a cooldown, then released only after
// controlGridScrollCooldownFrames ticks through the shared chokepoint.
func TestTickMenuScrollCooldowns_ReleasesEveryMenu(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

	// Populate every menu-scroll field with a scrollable list, exactly the two
	// shapes production uses: MenuScroll (overflow/subdiv/instrument) and raw
	// ScrollBehavior (context/EQ-dropdown). The comps are created by the ctor.
	dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, 20)
	dv.overflowMenuScroll.Configure(image.Rect(0, 0, 100, 100), 50, 5)
	dv.contextMenuScroll = configScrollBehavior(20, 50, 5)
	if dv.subdivMenuComp == nil {
		t.Fatal("subdivMenuComp nil (expected ctor-created)")
	}
	dv.subdivMenuComp.menuScroll = NewMenuScroll(DropdownScrollbarStyle, 20)
	dv.subdivMenuComp.menuScroll.Configure(image.Rect(0, 0, 100, 100), 50, 5)
	if dv.instMenuComp == nil {
		t.Fatal("instMenuComp nil (expected ctor-created)")
	}
	dv.instMenuComp.menuScroll = NewMenuScroll(DropdownScrollbarStyle, 20)
	dv.instMenuComp.menuScroll.Configure(image.Rect(0, 0, 100, 100), 50, 5)
	if dv.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil (expected ctor-created)")
	}
	dv.eqPanelZone.channelScroll = configScrollBehavior(20, 50, 5)

	// firstNotch drives one production-style wheel notch for each menu.
	menuNotch := func(ms *MenuScroll) bool { return ms.HandleWheel(-3) }
	sbNotch := func(sb *ScrollBehavior) bool { return sb.WheelStep(-3, controlGridScrollCooldownFrames) }

	// Put every menu into a cooldown with its first notch (must move one item).
	if !menuNotch(dv.overflowMenuScroll) || !sbNotch(dv.contextMenuScroll) ||
		!menuNotch(dv.subdivMenuComp.menuScroll) || !menuNotch(dv.instMenuComp.menuScroll) ||
		!sbNotch(dv.eqPanelZone.channelScroll) {
		t.Fatal("first notch failed to move one of the menus")
	}
	// While in cooldown, a second notch is locked out for all of them.
	if menuNotch(dv.overflowMenuScroll) || sbNotch(dv.contextMenuScroll) ||
		menuNotch(dv.subdivMenuComp.menuScroll) || menuNotch(dv.instMenuComp.menuScroll) ||
		sbNotch(dv.eqPanelZone.channelScroll) {
		t.Fatal("a menu accepted a second notch during cooldown (not clicky)")
	}
	// Advance frames through the REAL chokepoint.
	for i := 0; i < controlGridScrollCooldownFrames; i++ {
		dv.tickMenuScrollCooldowns()
	}
	// Now every menu accepts exactly one more notch — the chokepoint released all.
	for _, ok := range []bool{
		menuNotch(dv.overflowMenuScroll), sbNotch(dv.contextMenuScroll),
		menuNotch(dv.subdivMenuComp.menuScroll), menuNotch(dv.instMenuComp.menuScroll),
		sbNotch(dv.eqPanelZone.channelScroll),
	} {
		if !ok {
			t.Fatal("tickMenuScrollCooldowns did not release a menu's cooldown — it is not ticked by the shared chokepoint")
		}
	}
}

// TestAllMenuScrolls_ClickyCadenceIdentical proves every scrollable menu scrolls
// with the EXACT same clicky cadence: one item per notch regardless of wheel
// magnitude, then locked until controlGridScrollCooldownFrames frames elapse.
// Each case mirrors the menu's real production wheel call and its scroll type.
func TestAllMenuScrolls_ClickyCadenceIdentical(t *testing.T) {
	newMenu := func() *MenuScroll {
		m := NewMenuScroll(DropdownScrollbarStyle, 20)
		m.Configure(image.Rect(0, 0, 100, 100), 50, 5)
		return m
	}
	menuCase := func() (func() bool, func(), func() int) {
		m := newMenu()
		return func() bool { return m.HandleWheel(-3) },
			m.TickStep,
			func() int { return m.ScrollBehavior().VS.First }
	}
	sbCase := func() (func() bool, func(), func() int) {
		sb := configScrollBehavior(20, 50, 5)
		return func() bool { return sb.WheelStep(-3, controlGridScrollCooldownFrames) },
			sb.TickStep,
			func() int { return sb.VS.First }
	}

	cases := map[string]func() (func() bool, func(), func() int){
		"overflow-template": menuCase, // MenuScroll.HandleWheel
		"subdivision":       menuCase, // MenuScroll.HandleWheel
		"instrument":        menuCase, // MenuScroll.HandleWheel
		"context":           sbCase,   // ScrollBehavior.WheelStep
		"eq-dropdown":       sbCase,   // ScrollBehavior.WheelStep
	}

	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			notch, tick, first := mk()
			// One big-magnitude notch → exactly one item.
			if !notch() || first() != 1 {
				t.Fatalf("%s: first notch moved to %d, want 1 (one item per notch)", name, first())
			}
			// Locked during cooldown.
			if notch() || first() != 1 {
				t.Fatalf("%s: accepted a notch during cooldown (First=%d) — not clicky", name, first())
			}
			// Release after exactly controlGridScrollCooldownFrames ticks.
			for i := 0; i < controlGridScrollCooldownFrames-1; i++ {
				tick()
			}
			if notch() || first() != 1 {
				t.Fatalf("%s: released one tick too early (First=%d)", name, first())
			}
			tick() // the final tick that clears the cooldown
			if !notch() || first() != 2 {
				t.Fatalf("%s: did not release after %d ticks (First=%d)", name, controlGridScrollCooldownFrames, first())
			}
		})
	}
}

// TestMenuWheelDisciplineNoFastPath guards the "every menu scrolls the same
// clicky way" guarantee against regression. The two raw-ScrollBehavior menus
// (context menu, EQ-channel dropdown) must drive the wheel via WheelStep (clicky
// + cooldown), NOT the magnitude-scaled ScrollBehavior.HandleWheel that let a
// trackpad flick fly through the list. MenuScroll-based menus are safe because
// MenuScroll.HandleWheel itself delegates to WheelStep (asserted below).
func TestMenuWheelDisciplineNoFastPath(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	banned := regexp.MustCompile(`\b(contextMenuScroll|channelScroll)\.HandleWheel\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := banned.FindIndex(b); loc != nil {
			t.Errorf("%s: a raw-ScrollBehavior menu uses the fast magnitude HandleWheel (offset %d) — "+
				"menus must scroll clicky via WheelStep(steps, controlGridScrollCooldownFrames)", f, loc[0])
		}
	}

	// MenuScroll.HandleWheel must delegate to WheelStep so overflow/subdiv/inst
	// stay clicky. If someone reverts it to sb.HandleWheel, those menus silently
	// go fast again.
	src, err := os.ReadFile("menu_scroll.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	i := strings.Index(body, "func (m *MenuScroll) HandleWheel(")
	if i < 0 {
		t.Fatal("MenuScroll.HandleWheel not found")
	}
	end := strings.Index(body[i:], "\n}")
	if end < 0 {
		t.Fatal("could not bound MenuScroll.HandleWheel body")
	}
	fn := body[i : i+end]
	if !strings.Contains(fn, "WheelStep(") || strings.Contains(fn, "sb.HandleWheel(") {
		t.Errorf("MenuScroll.HandleWheel must delegate to sb.WheelStep (clicky), not sb.HandleWheel (fast). Body:\n%s", fn)
	}
}
