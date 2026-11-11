package ui

// Tests for the 2026-07-15 menus redesign (see
// docs/superpowers/specs/2026-07-15-menus-uiux-redesign-design.md):
// shared search-field clear-×, template group headers + BPM trail captions +
// loaded-template active row, and instrument-category row metadata.

import (
	"regexp"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestTemplateMenu_GroupHeadersWhenUnfiltered pins the template page's group
// headers: with an empty query the canonical group labels appear as
// non-clickable header rows between templates; an active search flattens the
// list (score-sorted, no headers).
func TestTemplateMenu_GroupHeadersWhenUnfiltered(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.overflowPage = 1

	items := dv.overflowItems()
	var groups []string
	for i, it := range items {
		// Skip the pinned band (Templates title + search row).
		if i >= 2 && it.header {
			groups = append(groups, it.label)
		}
	}
	if len(groups) < 5 {
		t.Fatalf("expected >=5 template group headers, got %v", groups)
	}
	// The groove templates lead — they are what shows a new user the drums —
	// and the classical library follows in chronological order.
	if groups[0] != "Funk · Soul · Jazz" {
		t.Errorf("first group header = %q, want the groove templates first", groups[0])
	}
	idx := map[string]int{}
	for i, gl := range groups {
		idx[gl] = i
	}
	if idx["Baroque"] >= idx["Classical"] || idx["Classical"] >= idx["Romantic"] {
		t.Errorf("classical groups out of chronological order: %v", groups)
	}
	seen := map[string]bool{}
	for _, gl := range groups {
		if seen[gl] {
			t.Errorf("group header %q rendered twice", gl)
		}
		seen[gl] = true
	}

	// An active query flattens the results — no group headers.
	dv.setTemplateSearchQuery("bach")
	defer dv.setTemplateSearchQuery("")
	for i, it := range dv.overflowItems() {
		if i >= 2 && it.header {
			t.Fatalf("group header %q rendered while a search query is active", it.label)
		}
	}
}

// TestTemplateMenu_RowsCarryBPMTrail pins that every template row carries its
// tempo as a right-aligned "N BPM" trail caption (MenuRowSpec.TrailLabel).
func TestTemplateMenu_RowsCarryBPMTrail(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.overflowPage = 1

	bpm := regexp.MustCompile(`^\d+ BPM$`)
	n := 0
	for _, it := range dv.overflowItems() {
		if it.header || it.search || it.iconID != IconTemplate {
			continue
		}
		n++
		if !bpm.MatchString(it.trail) {
			t.Errorf("template row %q trail = %q, want 'N BPM'", it.label, it.trail)
		}
	}
	if n < 20 {
		t.Fatalf("expected >=20 template rows, got %d", n)
	}
}

// TestTemplateMenu_LoadedTemplateReadsActive pins the "currently loaded"
// indicator: selecting a template remembers its stem for the session, and the
// reopened list marks exactly that row active (stripe + tint via
// menuItemActive in drawOverflowMenu).
func TestTemplateMenu_LoadedTemplateReadsActive(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.overflowPage = 1

	var firstLabel string
	for _, it := range dv.overflowItems() {
		if it.header || it.search || it.iconID != IconTemplate {
			continue
		}
		firstLabel = it.label
		if it.active {
			t.Fatalf("row %q active before any template was loaded", it.label)
		}
		it.onClick()
		break
	}
	if firstLabel == "" {
		t.Fatal("no template rows found")
	}
	if dv.templateLoadedStem == "" {
		t.Fatal("selecting a template did not record templateLoadedStem")
	}

	dv.overflowPage = 1
	actives := 0
	for _, it := range dv.overflowItems() {
		if it.active {
			actives++
			if it.label != firstLabel {
				t.Errorf("active row = %q, want %q", it.label, firstLabel)
			}
		}
	}
	if actives != 1 {
		t.Errorf("expected exactly 1 active template row, got %d", actives)
	}
}

// TestInstMenu_SearchClearButton pins the shared search-field clear-×: with a
// query present, a tap on menuSearchClearRect wipes the query and restores the
// unfiltered list; with no query the rect is not treated as a clear action.
func TestInstMenu_SearchClearButton(t *testing.T) {
	props := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
		},
		[]audio.Category{audio.CatPercussion},
	)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.Open()
	comp.SetSearchText("ki")
	if got := len(comp.InstBtns()); got != 1 {
		t.Fatalf("filtered list should show 1 row, got %d", got)
	}

	cr := menuSearchClearRect(comp.SearchBox())
	if cr.Empty() {
		t.Fatal("clear rect empty while a query is present")
	}
	cx, cy := cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2
	comp.fireTapAt(cx, cy)
	if comp.state.searchText != "" {
		t.Errorf("clear-× tap left searchText = %q", comp.state.searchText)
	}
	if got := len(comp.InstBtns()); got != 2 {
		t.Errorf("cleared list should show 2 rows, got %d", got)
	}

	// With no query the same point is NOT a clear action (adornment hidden).
	if comp.searchClearHit(cr.Min.Add(cr.Max).Div(2)) {
		t.Error("searchClearHit reported a hit with an empty query")
	}
}

// TestInstMenu_CategoryRowsCarryCounts pins the categories-mode metadata: each
// category row carries its instrument count for the trailing caption.
func TestInstMenu_CategoryRowsCarryCounts(t *testing.T) {
	props := instMenuTestProps(
		[]InstrumentOption{
			{ID: roster.Kick, Label: "Kick", Category: audio.CatPercussion},
			{ID: roster.Snare, Label: "Snare", Category: audio.CatPercussion},
			{ID: "saw", Label: "Saw", Category: audio.CatLead},
		},
		[]audio.Category{audio.CatPercussion, audio.CatLead},
	)
	props.ForceCategories = true
	comp := NewInstrumentMenuComponent()
	comp.SetProps(props)
	comp.Open()

	btns := comp.CategoryBtns()
	if len(btns) != len(comp.categoryMeta) {
		t.Fatalf("categoryMeta len %d != categoryBtns len %d", len(comp.categoryMeta), len(btns))
	}
	counts := map[string]int{}
	for i, b := range btns {
		counts[b.Text] = comp.categoryMeta[i].count
	}
	if counts["Percussion"] != 2 || counts["Lead/Synth"] != 1 {
		t.Errorf("category counts = %v, want Percussion=2 Lead/Synth=1", counts)
	}
}
