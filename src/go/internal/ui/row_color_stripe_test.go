//go:build test

package ui

import (
	"testing"
)

// TestRowAccentStripeWidthMatchesProfile asserts the profile exposes a
// non-zero accent-stripe width on mobile, which the row draw path uses as
// the column of instrument-colored pixels at rowRect.Min.X. The stripe gives
// each row a distinct visual identity — it's how the eye groups
// "this row plays kick" without reading the row name.
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
