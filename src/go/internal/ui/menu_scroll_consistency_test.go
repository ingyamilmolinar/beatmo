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

// Item-based, overflowing the viewport, as the context menu and EQ-channel dropdowns use it.
func configScrollBehavior(itemH, total, visible int) *ScrollBehavior {
	sb := NewScrollBehavior(DropdownScrollbarStyle, itemH)
	sb.VS.View = image.Rect(0, 0, 100, visible*itemH)
	sb.VS.Total = total
	sb.VS.Visible = visible
	sb.VS.Clamp()
	return sb
}

// Content scrolls down (First grows) as the finger moves up.
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

// Every list menu scrolls the same smooth way: wheel magnitude respected, no per-notch cooldown. The rack and
// synth grids deliberately keep the clicky WheelStep.
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

// A fast flick keeps scrolling past the release position, then decays to rest.
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

// The two raw-ScrollBehavior list menus must use the smooth HandleWheel, not WheelStep; MenuScroll menus are
// covered by its delegation. The rack and synth grids keep WheelStep by design.
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

	// Reverted to WheelStep, the overflow, subdiv, inst and logs menus silently go clicky.
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
