package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The grid keyboard gate must be the single KeyboardClaimed() predicate. This
// guards against a denylist creeping back into handleGlobalShortcuts (the
// original bug: an inline `focusedZone != "" || valueEditorActive()` check that
// had to enumerate every text input and missed several).
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

// After Phase 3, handleGlobalShortcuts must contain NO inline shortcut-key
// handling — every gated key is owned by a component node dispatched via the
// router. This guards against the monolithic grab-bag creeping back.
func TestGlobalShortcutsHasNoInlineKeyHandling(t *testing.T) {
	src, err := os.ReadFile("game_input_shortcuts.go")
	if err != nil {
		t.Fatalf("read game_input_shortcuts.go: %v", err)
	}
	body := extractFuncBody(t, string(src), "handleGlobalShortcuts")

	if !strings.Contains(body, "dispatchGated()") {
		t.Fatal("handleGlobalShortcuts must dispatch gated shortcuts via g.keyboardRouter.dispatchGated()")
	}
	// No inline shortcut-key constants — they belong in the nodes now. (KeyEscape
	// is NOT checked: Esc routing via handleEscape is Phase 2's concern and the
	// call site, not a constant, lives here.)
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

// extractFuncBody returns the source text of the named top-level method/func
// body (between its first '{' and the matching '}').
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
