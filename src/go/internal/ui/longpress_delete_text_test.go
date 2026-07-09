//go:build test

package ui

import "testing"

// The long-press popup's Delete keycap is a filled neon (destructive ember)
// control, so its label follows the DESIGN.md dark-on-neon contrast rule
// ("Filled controls = neon fill + #120A1C label") and the
// `button-destructive.textColor: {colors.background}` component token.
// Pre-fix the label was on-surface near-white, which read as a glossy
// highlight on the saturated fill and broke the filled-control pattern.
func TestLongPressDeleteLabelIsDarkOnNeon(t *testing.T) {
	got := longPressDeleteTextColor()
	if got != genColorBackground {
		t.Fatalf("longPressDeleteTextColor() = %v, want background %v", got, genColorBackground)
	}
}
