package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// One KeyboardClaimed() predicate, never a denylist: the inline check it replaced missed several text inputs.
func TestGridKeyboardGateUsesSinglePredicate(t *testing.T) {
	src, err := os.ReadFile("game_input_shortcuts.go")
	if err != nil {
		t.Fatalf("read game_input_shortcuts.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "handleGlobalShortcuts")

	if !strings.Contains(body, "KeyboardClaimed()") {
		t.Fatal("handleGlobalShortcuts must gate grid shortcuts via g.drum.KeyboardClaimed()")
	}
	if strings.Contains(body, "focusedZone") {
		t.Fatal("handleGlobalShortcuts must NOT inspect focusedZone directly — route ownership through KeyboardClaimed()")
	}
	if strings.Contains(body, "valueEditorActive()") {
		t.Fatal("handleGlobalShortcuts must NOT inspect valueEditorActive() directly — it belongs inside KeyboardClaimed()")
	}
}

// No inline shortcut-key handling: every gated key belongs to a router node.
func TestGlobalShortcutsHasNoInlineKeyHandling(t *testing.T) {
	src, err := os.ReadFile("game_input_shortcuts.go")
	if err != nil {
		t.Fatalf("read game_input_shortcuts.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "handleGlobalShortcuts")

	if !strings.Contains(body, "dispatchGated(") {
		t.Fatal("handleGlobalShortcuts must dispatch gated shortcuts via g.keyboardRouter.dispatchGated()")
	}
	// The per-key claim seam is how a popup control gets precedence; an inline "popup open?" check is a denylist.
	if !strings.Contains(body, "dispatchClaimedKeys()") {
		t.Fatal("handleGlobalShortcuts must let partial claimants act via g.dispatchClaimedKeys() before the gated router")
	}
	// KeyEscape is not checked: its call site, handleEscape, lives here.
	for _, forbidden := range []string{
		"KeyArrowLeft", "KeyArrowRight", "KeyArrowUp", "KeyArrowDown",
		"KeySpace", "KeyBracketLeft", "KeyBracketRight", "Key0",
		"KeyEqual", "KeyMinus", "KeyNumpadAdd", "KeyNumpadSubtract",
		"Key1", "KeySlash",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("handleGlobalShortcuts must not handle %s inline — move it to a keyboard_shortcuts.go node", forbidden)
		}
	}
}

func extractFuncBody(t *testing.T, src, name string) string {
	t.Helper()
	re := regexp.MustCompile(`func (\([^)]*\) )?` + regexp.QuoteMeta(name) + `\(`)
	loc := re.FindStringIndex(src)
	if loc == nil {
		t.Fatalf("func %s not found", name)
	}
	open := strings.IndexByte(src[loc[1]:], '{')
	if open < 0 {
		t.Fatalf("no opening brace for %s", name)
	}
	start := loc[1] + open
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatalf("unbalanced braces for %s", name)
	return ""
}
