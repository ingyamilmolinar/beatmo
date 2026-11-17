//go:build test

package ui

import (
	"image"
	"testing"
)

// The aggregate column's "headline" row (the big Headroom number) must use
// the panel-title typography role — the styled type-scale step above body —
// not an ad-hoc scale multiplier. Its rendered height must sit strictly
// above the body-value rows so the visual hierarchy is real.
func TestLevelsAggregate_HeadlineUsesTypeScale(t *testing.T) {
	rect := image.Rect(0, 0, 200, 400)
	rows := levelsAggregateColumnRows(rect)
	if got := rows[aggRowHeadroomValue].Role; got != RolePanelTitle {
		t.Fatalf("headline role: got %v, want RolePanelTitle", got)
	}
	if got := rows[aggRowClipsValue].Role; got != RoleBody {
		t.Fatalf("body value role: got %v, want RoleBody", got)
	}
	if StyledTextHeight(RolePanelTitle) <= StyledTextHeight(RoleBody) {
		t.Fatalf("headline height (%d) must exceed body height (%d)",
			StyledTextHeight(RolePanelTitle), StyledTextHeight(RoleBody))
	}
}
