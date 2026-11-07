//go:build test

package ui

import "testing"

// Off in production: on, they drew 1 px ticks across the top of the drum pane. BEATMO_DEBUG_LAYOUT
// ("1", "true", "TRUE", "yes", "on") enables them.
func TestLayoutGuidesDefaultOff(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if desktopProfile().ShowLayoutGuides {
		t.Errorf("desktop ShowLayoutGuides default is true; want false (set BEATMO_DEBUG_LAYOUT=1 to enable)")
	}
}

// The developer hatch; without it the splitter handles are invisible on desktop.
func TestLayoutGuidesDebugOverrideEnables(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "1")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if !desktopProfile().ShowLayoutGuides {
		t.Errorf("BEATMO_DEBUG_LAYOUT=1 did not enable ShowLayoutGuides on desktop")
	}
}

// Mobile has its own divider chrome; the guides are a desktop debug overlay.
func TestLayoutGuidesMobileAlwaysOff(t *testing.T) {
	t.Setenv("BEATMO_DEBUG_LAYOUT", "1")
	UpdateProfile()
	t.Cleanup(UpdateProfile)
	if mobileProfile().ShowLayoutGuides {
		t.Errorf("mobile profile honored BEATMO_DEBUG_LAYOUT — should always be off on mobile")
	}
}
