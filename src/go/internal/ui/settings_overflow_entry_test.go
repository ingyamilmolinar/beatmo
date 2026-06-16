package ui

import "testing"

func TestOverflowMenuHasSettingsEntry(t *testing.T) {
	g := newTestGameForUndo(t)
	g.Layout(1200, 800)
	dv := g.drum
	items := dv.overflowItems()
	var found *overflowItem
	for i := range items {
		if items[i].iconID == IconSettings {
			it := items[i]
			found = &it
			break
		}
	}
	if found == nil {
		t.Fatal("overflow menu has no Settings entry (IconSettings)")
	}
	if found.onClick == nil {
		t.Fatal("Settings overflow item has no onClick")
	}
	found.onClick()
	p := dv.tree.Portal()
	if p == nil || !p.Has(settingsOverlayID) {
		t.Fatal("Settings overflow item did not open the settings overlay")
	}
}
