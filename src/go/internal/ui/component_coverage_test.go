// A per-file budget of hand-coded *Style constructions that only goes down, toward an empty map and the legacy types
// deleted.

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Per file; any file not listed must have none. Lower an entry when its constructions move to Spec(...); never raise
// one without review.
var allowedStyleBudget = map[string]int{
	// DrumCellUI and ContextMenuItemStyle have no ComponentSpec yet.
	"theme.go": 2,
	// The FromSpec bridges must build the legacy types.
	"components.go": 2,
	// SetActive's transient highlight style needs an active-spec path.
	"ui_style.go": 1,
	// ColorSwatchStyle binds a runtime colour closure.
	"row_rack_zone.go": 1,
	"button_styles.go": 0,
	// The active fills are per-instrument shades computed at draw time, which no static spec can hold.
	"row_instrument_shades.go": 1,
}

// Anchored after a non-identifier byte, so prefixes like addRowButtonStyle do not count.
var styleConstructionRE = regexp.MustCompile(`(^|[^A-Za-z0-9_])(ButtonStyle|TextInputStyle|ColorSwatchStyle|DrumCellStyle|DrumRowStyle)\{`)

// Every non-test, non-generated file under internal/ui/.
func TestComponentCoverage(t *testing.T) {
	root := findUIDir(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read internal/ui: %v", err)
	}

	// theme.go is scanned on purpose: it most needs the forcing function.
	excludedSuffix := []string{"_test.go", ".gen.go"}

	overBudget := map[string]int{}
	underBudget := map[string]int{}
	unexpected := []string{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		skip := false
		for _, sfx := range excludedSuffix {
			if strings.HasSuffix(name, sfx) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		count := len(styleConstructionRE.FindAllIndex(body, -1))
		budget, allowed := allowedStyleBudget[name]
		if count == 0 && !allowed {
			continue
		}
		if !allowed && count > 0 {
			unexpected = append(unexpected, name)
			continue
		}
		if count > budget {
			overBudget[name] = count
		}
		if count < budget {
			underBudget[name] = count
		}
	}

	if len(unexpected) > 0 {
		sort.Strings(unexpected)
		t.Errorf("hand-coded *Style construction in files not on the allowlist:\n  %s\n"+
			"Use NewSpecButton(text, ComponentXxx, onClick) instead, or (with review) "+
			"add the file to allowedStyleBudget in component_coverage_test.go.",
			strings.Join(unexpected, "\n  "))
	}

	if len(overBudget) > 0 {
		var msgs []string
		for name, count := range overBudget {
			msgs = append(msgs, "  "+name+": "+itoa(count)+" *Style constructions (budget "+itoa(allowedStyleBudget[name])+")")
		}
		sort.Strings(msgs)
		t.Errorf("*Style construction budget exceeded:\n%s\n"+
			"Replace the new constructions with Spec(ID) + NewSpecButton, or justify "+
			"and bump the budget in component_coverage_test.go.",
			strings.Join(msgs, "\n"))
	}

	if len(underBudget) > 0 {
		var msgs []string
		for name, count := range underBudget {
			msgs = append(msgs, "  "+name+": "+itoa(count)+" *Style constructions (budget "+itoa(allowedStyleBudget[name])+")")
		}
		sort.Strings(msgs)
		t.Errorf("*Style construction budget can be tightened:\n%s\n"+
			"Decrement allowedStyleBudget so future regressions are caught.",
			strings.Join(msgs, "\n"))
	}
}
