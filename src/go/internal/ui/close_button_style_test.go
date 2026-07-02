//go:build test

package ui

import "testing"

// Pop-up close "X" buttons use a red glyph (reusing the existing error/stop red
// — no 4th red) on an otherwise-unchanged body. Chrome unification 2026-06-15.
func TestCloseButtonGlyphIsRed(t *testing.T) {
	got := closeIconColor()
	if got != colError {
		t.Fatalf("closeIconColor() = %v, want red colError %v", got, colError)
	}
}
