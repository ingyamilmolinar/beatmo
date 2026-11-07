//go:build test

package ui

import "testing"

// The muted tint (DESIGN.md "Close (any panel) → on-surface-muted"): red is for stop/error and destructive actions,
// and a neutral dismiss must not read as destructive.
func TestCloseButtonGlyphIsMuted(t *testing.T) {
	got := closeIconColor()
	if got != colTextSecondary {
		t.Fatalf("closeIconColor() = %v, want on-surface-muted colTextSecondary %v", got, colTextSecondary)
	}
}
