package ui

import (
	"strings"
	"testing"
)

// No Chain and "1 - 6" in production, all seven in a developer build.
func TestShortcutHelpListsTheReachableTabs(t *testing.T) {
	row := func() shortcutRow {
		for _, r := range localizedShortcutRows() {
			if strings.HasPrefix(r.keys, "1 - ") {
				return r
			}
		}
		t.Fatal("no tab-switch row in the shortcut help")
		return shortcutRow{}
	}
	chain := PanelTabLabel(TabScope)
	if devToolsBuild {
		dev := row()
		if dev.keys != "1 - 7" || !strings.Contains(dev.action(), chain) {
			t.Fatalf("developer build row = %q / %q, want 1 - 7 with %s", dev.keys, dev.action(), chain)
		}
	}
	forceProductionUIForTest(t)
	prod := row()
	if prod.keys != "1 - 6" || strings.Contains(prod.action(), chain) {
		t.Fatalf("production row = %q / %q, want 1 - 6 without %s", prod.keys, prod.action(), chain)
	}
}
