//go:build test

package ui

import "testing"

// A filled neon control takes the dark label (DESIGN.md: neon fill + #120A1C label); a near-white label reads
// as a glossy highlight on the saturated fill.
func TestLongPressDeleteLabelIsDarkOnNeon(t *testing.T) {
	got := longPressDeleteTextColor()
	if got != genColorBackground {
		t.Fatalf("longPressDeleteTextColor() = %v, want background %v", got, genColorBackground)
	}
}
