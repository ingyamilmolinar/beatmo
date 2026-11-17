//go:build test

package ui

import "testing"

// TestLayoutGuidesDefaultOff asserts that the desktop profile's layout
// guides (1-px column/row dividers + splitter pills) are OFF by default
// in production. They were previously enabled-by-default on desktop,
// causing visible 1-px ticks across the top edge of the drum pane in
// the gap above the transport surface (see screenshot.png in the
// commit that introduced this test).
//
// The override hatch is the BEATMO_DEBUG_LAYOUT env var: "1" / "true" /
// "TRUE" / "yes" / "on" all enable. Anything else is off.
func TestLayoutGuidesDefaultOff(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if desktopProfile().ShowLayoutGuides {
		t.Errorf("desktop ShowLayoutGuides default is true; want false (set BEATMO_DEBUG_LAYOUT=1 to enable)")
	}
}

// TestLayoutGuidesDebugOverrideEnables asserts that BEATMO_DEBUG_LAYOUT=1
// flips desktop ShowLayoutGuides to true. This is the developer escape
// hatch — without it, splitter handles are invisible on desktop.
func TestLayoutGuidesDebugOverrideEnables(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "1")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if !desktopProfile().ShowLayoutGuides {
		t.Errorf("BEATMO_DEBUG_LAYOUT=1 did not enable ShowLayoutGuides on desktop")
	}
}

// TestLayoutGuidesMobileAlwaysOff asserts mobile is unaffected by the
// debug env var — it never gets layout guides, regardless. (Mobile
// has its own divider chrome via DrawToolbarSep and the bottom-sheet
// surface; layout guides are a desktop-only debug overlay.)
func TestLayoutGuidesMobileAlwaysOff(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "1")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if mobileProfile().ShowLayoutGuides {
		t.Errorf("mobile profile honored BEATMO_DEBUG_LAYOUT — should always be off on mobile")
	}
}
