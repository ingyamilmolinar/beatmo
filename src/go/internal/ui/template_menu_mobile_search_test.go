//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// feedTemplateNativeSearch injects a live native-input value for the template
// search field and drives real frames (portal updateFn → updateTemplateSearch).
func feedTemplateNativeSearch(t *testing.T, g *Game, v string) {
	t.Helper()
	testMobileInputValue[templateSearchMobileInputID] = v
	for i := 0; i < 5; i++ {
		_ = g.Update()
	}
}

func TestTemplateMenu_MobileNativeInputFilters(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	testMobileInputValue = map[string]string{}
	t.Cleanup(func() { testMobileInputValue = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()
	wantAll := len(assets.Templates())

	feedTemplateNativeSearch(t, g, "brandenburg")
	if dv.templateSearchQuery != "brandenburg" {
		t.Fatalf("native-input value never reached the query; got %q", dv.templateSearchQuery)
	}
	if !containsStr(templateRowLabels(dv), "Bach — Brandenburg Concerto No. 3") {
		t.Fatalf("mobile query 'hotel' dropped its own template: %v", templateRowLabels(dv))
	}

	// Non-matching query empties the template rows (proves per-frame re-read).
	feedTemplateNativeSearch(t, g, "zzqxjkvw")
	if n := len(templateRowLabels(dv)); n != 0 {
		t.Fatalf("non-matching mobile query should empty the list, got %d", n)
	}

	// Clearing restores all rows.
	feedTemplateNativeSearch(t, g, "")
	if n := len(templateRowLabels(dv)); n != wantAll {
		t.Fatalf("clearing mobile query should restore %d rows, got %d", wantAll, n)
	}
}
