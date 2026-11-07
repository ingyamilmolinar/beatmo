//go:build test

// The compiler cannot see a menu that renders rows but never calls MenuKeyNav.Update, so every drawMenuRow file
// is either navigable or exempt with a reason.

package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// navigableMenuFiles render rows AND drive a MenuKeyNav.
var navigableMenuFiles = map[string]string{
	"drumview_overlay_subdiv_comp.go": "subdiv + jump pickers (same component, two instances)",
	"drumview_overlay_inst_comp.go":   "instrument picker",
	"drumview_context_menu.go":        "row context menu + File/Templates overflow menu",
	"theme_overlay.go":                "theme editor: preset rows + Adjust header + category chips",
	"action_menu_comp.go":             "shared \u22ef overflow (Synth/Sampler Save As/Export/Import/Reset, synth + Add stage)",
}

// Not pick-one lists; each says why arrows/Enter do not apply.
var exemptRowRenderers = map[string]string{
	// Jog buttons and go-to editors that already own Enter: no rows to arrow through.
	"navigation_panel.go": "control panel (jog buttons + goto editors), no row list",
	// Per-row controls; Enter has no single meaning on a row.
	"drumview_fx_panel.go": "per-row controls, not a pick-one list",
	// A persistent pane with no open/close lifecycle to scope a cursor to.
	"game_node_sidebar.go": "always-visible inspector pane, not a menu",
	// Marquee/group actions drawn as rows inside the grid pane.
	"game_group_menu.go": "grid-pane group actions, opened and dismissed by gesture",
}

func TestEveryMenuRowRendererIsNavigableOrExempt(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	var unclassified []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(src), "drawMenuRow(") {
			continue
		}
		if _, ok := navigableMenuFiles[name]; ok {
			continue
		}
		if _, ok := exemptRowRenderers[name]; ok {
			continue
		}
		unclassified = append(unclassified, name)
	}
	sort.Strings(unclassified)
	for _, name := range unclassified {
		t.Errorf("%s renders menu rows (drawMenuRow) but is neither wired for keyboard "+
			"navigation nor exempted.\n"+
			"Either implement MenuKeyNavHost and drive a MenuKeyNav from the menu's "+
			"per-frame update (see menu_keynav.go), or add it to exemptRowRenderers "+
			"with a reason why arrows/Enter do not apply to it.", name)
	}
}

// So the registry cannot go stale into a false "yes".
func TestNavigableMenusDriveMenuKeyNav(t *testing.T) {
	for name, why := range navigableMenuFiles {
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(src)
		hasNav := strings.Contains(body, "MenuKeyNav") || strings.Contains(body, "KeyNav()")
		if !hasNav {
			t.Errorf("%s is registered as navigable (%s) but references no MenuKeyNav; "+
				"either wire it or move it to exemptRowRenderers", name, why)
		}
	}
}

// A blank reason is how a temporary skip becomes permanent.
func TestExemptRowRenderersCarryAReason(t *testing.T) {
	for name, why := range exemptRowRenderers {
		if strings.TrimSpace(why) == "" {
			t.Errorf("%s is exempt from menu keyboard navigation with no stated reason", name)
		}
		if _, err := os.Stat(name); err != nil {
			t.Errorf("exemptRowRenderers lists %s, which no longer exists — remove the "+
				"stale entry so the guard keeps covering real files", name)
		}
	}
}
