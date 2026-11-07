package ui

import "testing"

// Settings is reached only through the gear.
func TestOverflowMenuHasNoSettingsEntry(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1200, 800)
	dv := g.drum
	items := dv.overflowItems()
	for i := range items {
		if items[i].iconID == IconSettings {
			t.Fatal("overflow menu still has a Settings entry (IconSettings); it should be removed")
		}
	}
}
