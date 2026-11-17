package ui

import "testing"

// Settings is reached exclusively via the grid pane's top-right gear button on
// every platform. The overflow ("ellipsis") menu must NOT carry a duplicate
// Settings entry.
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
