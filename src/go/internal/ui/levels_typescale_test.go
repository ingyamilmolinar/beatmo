//go:build test

package ui

import (
	"image"
	"testing"
)

// The panel-title role, strictly taller than the body rows, not an ad-hoc scale.
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
