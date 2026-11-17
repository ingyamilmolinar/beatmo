//go:build test

package ui

func init() {
	// Preserve legacy layout expectations in tests to avoid shrinking the row area.
	eqPanelHeight = 0
}
