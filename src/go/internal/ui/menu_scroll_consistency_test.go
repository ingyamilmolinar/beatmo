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

// flingAndSettle simulates a fast upward finger fling on a ScrollBehavior and
// then runs momentum to rest. Returns the offset at finger-release, the offset
// at rest, and how many frames of inertia elapsed. Content scrolls DOWN (First
// grows) as the finger moves up.
func flingAndSettle(sb *ScrollBehavior) (release, rest, frames int) {
	sb.HandleTouchBegin(50, 90)
	sb.HandleTouchMove(50, 60) // commits the vertical direction past the dead zone
	sb.HandleTouchMove(50, 20) // builds velocity
	release = sb.VS.First
	sb.HandleTouchEnd()
	for i := 0; i < 600 && sb.HasMomentum(); i++ {
		sb.UpdateMomentum()
		frames++
	}
	rest = sb.VS.First
	return release, rest, frames
}

// TestAllMenuScrolls_SmoothCadenceIdentical proves every scrollable list menu
// scrolls with the EXACT same smooth cadence — the instrument-menu feel — via the
// shared engine: the wheel RESPECTS magnitude (a 3-step notch moves three items
// in one event) and has NO per-notch cooldown lock (an immediately-following
// notch moves again on the same frame). Each case drives the menu's real
// production wheel call and its scroll type. (The row rack and synth control
// grids deliberately keep the clicky WheelStep path — they are not list menus.)
func TestAllMenuScrolls_SmoothCadenceIdentical(t *testing.T) {
	menuCase := func() (func() bool, func() int) {
		m := NewMenuScroll(DropdownScrollbarStyle, 20)
		m.Configure(image.Rect(0, 0, 100, 100), 50, 5)
		return func() bool { return m.HandleWheel(-3) },
			func() int { return m.ScrollBehavior().VS.First }
	}
	sbCase := func() (func() bool, func() int) {
		sb := configScrollBehavior(20, 50, 5)
		return func() bool { return sb.HandleWheel(-3) },
			func() int { return sb.VS.First }
	}

	cases := map[string]func() (func() bool, func() int){
		"overflow-template": menuCase, // MenuScroll.HandleWheel
		"subdivision":       menuCase, // MenuScroll.HandleWheel
		"instrument":        menuCase, // MenuScroll.HandleWheel
		"context":           sbCase,   // ScrollBehavior.HandleWheel
		"eq-dropdown":       sbCase,   // ScrollBehavior.HandleWheel
	}

	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			notch, first := mk()
			// Magnitude respected: a three-step notch moves three items at once.
			if !notch() || first() != 3 {
				t.Fatalf("%s: first notch moved to %d, want 3 (magnitude respected)", name, first())
			}
			// No cooldown lock: an immediately-following notch moves again.
			if !notch() || first() != 6 {
				t.Fatalf("%s: second notch moved to %d, want 6 (no cooldown lock)", name, first())
			}
		})
	}
}

// TestAllMenuScrolls_MomentumIdentical proves every scrollable list menu carries
// touch-fling inertia the same way: a fast flick keeps scrolling after the finger
// lifts (momentum advances the offset past its position at release, then decays to
// rest). Each case uses the menu's real scroll type.
func TestAllMenuScrolls_MomentumIdentical(t *testing.T) {
	menuSB := func() *ScrollBehavior {
		m := NewMenuScroll(DropdownScrollbarStyle, 20)
		m.Configure(image.Rect(0, 0, 100, 100), 200, 5)
		return m.ScrollBehavior()
	}
	rawSB := func() *ScrollBehavior { return configScrollBehavior(20, 200, 5) }

	cases := map[string]func() *ScrollBehavior{
		"overflow-template": menuSB,
		"subdivision":       menuSB,
		"instrument":        menuSB,
		"context":           rawSB,
		"eq-dropdown":       rawSB,
	}

	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			release, rest, frames := flingAndSettle(mk())
			if frames == 0 {
				t.Fatalf("%s: no momentum frames — a fling produced no inertia", name)
			}
			if rest <= release {
				t.Fatalf("%s: momentum did not carry past release (release=%d rest=%d)", name, release, rest)
			}
		})
	}
}

// TestMenuWheelDisciplineNoClickyPath guards the "every list menu scrolls the same
// smooth way" guarantee against regression. The two raw-ScrollBehavior list menus
// (context menu, EQ-channel dropdown) must drive the wheel via the smooth,
// magnitude-respecting ScrollBehavior.HandleWheel — NOT the clicky WheelStep that
// threw away wheel magnitude and cooldown-locked the list. The row rack and synth
// control grids keep WheelStep by design, so the ban is scoped to the two
// list-menu scroll fields. MenuScroll-based menus are covered by asserting
// MenuScroll.HandleWheel delegates to the smooth sb.HandleWheel.
func TestMenuWheelDisciplineNoClickyPath(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	banned := regexp.MustCompile(`\b(contextMenuScroll|channelScroll)\.WheelStep\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := banned.FindIndex(b); loc != nil {
			t.Errorf("%s: a raw-ScrollBehavior list menu uses the clicky WheelStep (offset %d) — "+
				"list menus must scroll smooth via ScrollBehavior.HandleWheel(steps)", f, loc[0])
		}
	}

	// MenuScroll.HandleWheel must delegate to the smooth sb.HandleWheel so
	// overflow/subdiv/inst/logs stay smooth. If someone reverts it to WheelStep,
	// those menus silently go clicky again.
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
	if !strings.Contains(fn, "sb.HandleWheel(") || strings.Contains(fn, "WheelStep(") {
		t.Errorf("MenuScroll.HandleWheel must delegate to sb.HandleWheel (smooth), not sb.WheelStep (clicky). Body:\n%s", fn)
	}
}
