//go:build test

package ui

import "testing"

// Pop-up close "X" buttons use the muted on-surface tint, per the DESIGN.md
// IconColor mapping ("Close (any panel) → on-surface-muted"). The error red
// is reserved for stop/error text and destructive actions (three-reds
// invariant); a neutral dismiss glyph must not read as destructive.
// This also unifies the two historical conventions (ui_style.go CloseButton
// already used colTextSecondary while closeIconColor was red).
func TestCloseButtonGlyphIsMuted(t *testing.T) {
	got := closeIconColor()
	if got != colTextSecondary {
		t.Fatalf("closeIconColor() = %v, want on-surface-muted colTextSecondary %v", got, colTextSecondary)
	}
}
