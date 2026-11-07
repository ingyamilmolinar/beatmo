//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// One ParamCard per visible knob carrying name and value together, with the Advanced tier behind an always-visible
// divider row.
func TestSynthCardDraw_NameValueAndDivider(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum

	// Select a stage with advanced knobs so the divider row is exercised too.
	for i := range dv.instEditorSections {
		if _, adv := dv.synthSectionTierSplit(dv.instEditorSections[i].knobIdxs); len(adv) > 0 {
			dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), dv.instEditorSections[i].id)
			break
		}
	}
	layoutSynthTab(t, g)

	sel := dv.synthSelectedSection()
	if sel == nil || len(sel.knobIdxs) == 0 {
		t.Fatal("no knobs in selected section")
	}
	def := dv.instEditorBindings[sel.knobIdxs[0]].def
	wantName := strings.ToLower(synthParamDisplayName(def.Name))

	draw := func() map[string]bool {
		return collectDrawnTexts(t, func() {
			scratch := newTrackedImage("test.synthcarddraw", 1280, 720)
			defer releaseImage(scratch)
			g.drum.eqPanelZone.Draw(scratch)
		})
	}

	texts := draw()
	joined := strings.ToLower(strings.Join(mapKeys(texts), "\n"))
	if !strings.Contains(joined, wantName) {
		t.Fatalf("card name %q not drawn; texts:\n%s", wantName, joined)
	}

	// The pane is too short for essentials, divider and advanced at once.
	ess, adv := dv.synthSectionTierSplit(sel.knobIdxs)
	if len(adv) == 0 {
		t.Fatal("selected stage has no advanced knobs")
	}
	grid := dv.sectionGrid(sel.id)
	cols := grid.Cols()
	if cols < 1 {
		cols = 1
	}
	essRows := (len(ess) + cols - 1) / cols
	grid.ScrollToIndex(essRows * cols)
	layoutSynthTab(t, g)
	if dv.synthAdvDividerRect.Empty() {
		t.Fatal("expected a non-empty advanced divider rect once its row is scrolled into view")
	}

	texts = draw()
	joined = strings.ToLower(strings.Join(mapKeys(texts), "\n"))
	wantDivider := strings.ToLower(i18n.T(i18n.KeySynthMore))
	if !strings.Contains(joined, wantDivider) {
		t.Fatalf("More divider label %q not drawn; texts:\n%s", wantDivider, joined)
	}
}

// mapKeys returns the keys of a collectDrawnTexts-style set as a slice.
func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
