//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMenusUseWidthClass is the ratchet for DESIGN.md "Menu sizing & layout":
// a floating menu's desktop width comes from a ROLE class (menuWAction /
// menuWList / menuWEditor / menuWWide), never a hand-picked pixel count.
//
// Every file that builds a menu card (NewMenuCard / menuCardEntry) must either
// size it through a menuW* class or be listed in menuWidthClassPending. The
// pending list may only SHRINK: a listed file that has adopted a class fails
// until it is removed from the list, and a new menu file that is neither fails
// immediately.
var menuWidthClassPending = map[string]string{
	"confirm_dialog.go":              "confirm dialog: fixed card sized to its message",
	"drumview_context_menu.go":       "row context / overflow menu: hand-picked 160 / pad widths",
	"drumview_fx_panel.go":           "FX panel: 250 / 340 hand-picked",
	"drumview_logs_panel.go":         "logs panel host (sizing in logs_component.go)",
	"drumview_overlay_color_comp.go": "colour picker: swatch-grid geometry",
	"game_input_shortcuts.go":        "opens shortcut-reachable menus",
	"logs_component.go":              "logs panel: 3/4 window, not yet menuWWide",
	"theme_picker.go":                "theme swatch picker: swatch-grid geometry",
}

var menuCardCall = regexp.MustCompile(`\b(NewMenuCard|menuCardEntry)\(`)
var menuWidthClassCall = regexp.MustCompile(`\bmenuW(Action|List|Editor|Wide)\(`)

func TestMenusUseWidthClass(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "menu_card.go" || f == "menu_sizing.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !menuCardCall.Match(src) {
			continue
		}
		seen[f] = true
		adopted := menuWidthClassCall.Match(src)
		_, pending := menuWidthClassPending[f]
		switch {
		case adopted && pending:
			t.Errorf("%s now sizes its menu with a width class — remove it from menuWidthClassPending (the list only shrinks)", f)
		case !adopted && !pending:
			t.Errorf("%s builds a menu card but sizes it without a menuW* width class (DESIGN.md \"Menu sizing & layout\") — use menuWAction / menuWList / menuWEditor / menuWWide", f)
		}
	}
	for f := range menuWidthClassPending {
		if !seen[f] {
			t.Errorf("menuWidthClassPending lists %s, which no longer builds a menu card — remove it", f)
		}
	}
}

func TestMenuWidthClassValues(t *testing.T) {
	if menuWAction(10) != Profile().MenuWActionMin || menuWAction(10000) != Profile().MenuWActionMax {
		t.Errorf("menuWAction must clamp to [%d, %d]", Profile().MenuWActionMin, Profile().MenuWActionMax)
	}
	if got := menuWAction(200); got != 200 {
		t.Errorf("menuWAction(200) = %d, want the content width inside the class", got)
	}
	if menuWEditor() != 360 || menuWList() != 300 {
		t.Errorf("editor/list widths = %d/%d, want 360/300 (DESIGN.md)", menuWEditor(), menuWList())
	}
	if menuWWide(1000) != 750 || menuWWide(4000) != Profile().MenuWWideMax {
		t.Errorf("menuWWide must be 3/4 of the window, capped at %d", Profile().MenuWWideMax)
	}
}
