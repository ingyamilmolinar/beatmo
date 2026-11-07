//go:build test

package ui

import (
	"testing"
)

// The stripe lets the eye group "this row plays kick" without reading its name.
func TestRowAccentStripeWidthMatchesProfile(t *testing.T) {
	assertDefaultParityState(t)

	desktopWidth := genDesktopProfile.AccentStripeWidth
	mobileWidth := genMobileProfile.AccentStripeWidth
	if desktopWidth <= 0 {
		t.Errorf("desktop AccentStripeWidth should be >0 to render the stripe, got %d", desktopWidth)
	}
	if mobileWidth <= 0 {
		t.Errorf("mobile AccentStripeWidth should be >0 to render the stripe, got %d", mobileWidth)
	}
	if mobileWidth < desktopWidth {
		t.Errorf("mobile AccentStripeWidth (%d) should not be smaller than desktop (%d) — touch UIs need the stronger anchor",
			mobileWidth, desktopWidth)
	}
}
