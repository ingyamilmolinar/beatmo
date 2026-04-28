// Code review follow-up (P2.2): per-file budget for hand-coded *Style
// constructions in internal/ui/, mirroring the inline-literal ratchet in
// token_discipline_test.go. The original Phase 3 plan demanded a coverage
// test that "enumerates every ButtonStyle/TextInputStyle/ColorSwatchStyle
// instantiation in zone files and confirms each is sourced via Spec(ID)."
// Until the legacy *Style types are fully retired, the equivalent forcing
// function is a per-file count that monotonically tightens — adding a new
// hand-coded ButtonStyle in a file at budget 0 fails the test, and the
// budgets only ever go down.
//
// The eventual end-state is allowedStyleBudget = {} (empty map) and the
// *Style types deleted; this test ratchets toward that.

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// allowedStyleBudget pins the current count of `XxxStyle{` literal
// constructions per file, where Xxx is one of ButtonStyle, TextInputStyle,
// ColorSwatchStyle, DrumCellStyle, DrumRowStyle. Files not in this map are
// required to contain zero such constructions.
//
// Decrement (or remove) an entry when its constructions are replaced with
// `Spec(ComponentXxx)` + `NewSpecButton`. Never increment without code
// review — adding a new *Style instantiation undermines the design-system-
// driven UI promise.
//
// Audit baseline (2026-04-26 / 2026-04-27 post-M2):
//
// theme.go's 32-construction count was reduced to 2 by routing all
// migratable vars through ButtonStyleFromSpec / TextInputStyleFromSpec.
// The 2 remaining constructions are:
//   - DrumCellUI = DrumCellStyle{...} (struct fields don't match
//     ComponentSpec; needs a separate spec-from-cell helper)
//   - ContextMenuItemStyle = ButtonStyle{0, 0, 0, 0} (transparent
//     fill — no DESIGN.md component represents this; future PR could
//     add a `transparent-button` spec to take this to zero).
//
// components.go contains the helper struct returns
// (ButtonStyle{Fill, Border} inside ButtonStyleFromSpec; same shape
// inside TextInputStyleFromSpec). These are infrastructure: they're
// the bridge code by which all theme.go vars become spec-derived.
// Both helpers MUST construct the legacy struct type — that's the
// whole point. Allowlisting them is correct.
var allowedStyleBudget = map[string]int{
	// theme.go: 2 transitional hand-coded vars left (DrumCellUI,
	// ContextMenuItemStyle). Both have no equivalent ComponentSpec yet.
	"theme.go": 2,
	// components.go: 2 helper-internal constructions (Button/TextInput
	// FromSpec). Required to bridge from generated specs to legacy types.
	"components.go": 2,
	// NewButton.SetActive constructs a transient highlight style. Migration
	// requires giving Button a "active spec ID" path; trivial but separate.
	"ui_style.go": 1,
	// ColorSwatchStyle binds a runtime color closure. Only the dynamic-
	// anchor registry (review priority P4) lets this go through Spec(ID),
	// so this stays at 1 until the registry lands.
	"row_rack_zone.go": 1,
}

// styleConstructionRE matches `XxxStyle{` literal construction sites for
// the five legacy *Style types. The pattern is anchored to the type name
// (after a non-identifier byte) so identifier prefixes like
// `addRowButtonStyle` are not counted.
var styleConstructionRE = regexp.MustCompile(`(^|[^A-Za-z0-9_])(ButtonStyle|TextInputStyle|ColorSwatchStyle|DrumCellStyle|DrumRowStyle)\{`)

// TestComponentCoverage walks every non-test, non-generated source file
// under internal/ui/ and asserts that hand-coded *Style construction sites
// are covered by allowedStyleBudget. Any new construction in a file at
// budget 0 fails the test; counts must monotonically tighten.
func TestComponentCoverage(t *testing.T) {
	root := findUIDir(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read internal/ui: %v", err)
	}

	// We exclude generated files and tests; we *do* scan theme.go (which
	// is the primary site of hand-coded styles and the one most in need of
	// a forcing function).
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
