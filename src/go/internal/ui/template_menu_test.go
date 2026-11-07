package ui

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

func TestOverflowMenu_TemplatePageListsGenres(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

	// Page 0 contains a "Load template" action.
	var loadTemplate *overflowItem
	for i := range dv.overflowItems() {
		if dv.overflowItems()[i].label == "Load template" {
			it := dv.overflowItems()[i]
			loadTemplate = &it
		}
	}
	if loadTemplate == nil {
		t.Fatalf("page 0 missing 'Load template' entry")
	}
	// Activating it switches to the template page.
	loadTemplate.onClick()
	if dv.overflowPage != 1 {
		t.Fatalf("overflowPage=%d want 1 after Load template", dv.overflowPage)
	}
	// Page 1 lists all seven genres by display name + a Back row.
	labels := map[string]bool{}
	var back bool
	for _, it := range dv.overflowItems() {
		labels[it.label] = true
		if it.label == "Back" {
			back = true
		}
	}
	if !back {
		t.Fatalf("template page missing Back row")
	}
	// One per menu group, so a group silently dropped from templateGroups fails.
	for _, want := range []string{
		"Bach — Brandenburg Concerto No. 3", // Baroque
		"Beethoven — Symphony No. 5",        // Classical
		"Dvořák — American Quartet",         // Romantic
		"Schubert — Erlkönig",               // Lied · Song
		"Satie — Air du rat",                // French · Americas
	} {
		if !labels[want] {
			t.Fatalf("template page missing %q (have %v)", want, labels)
		}
	}
}

func TestOverflowMenu_SelectTemplateQueuesImport(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.overflowPage = 1

	// Click the first template entry.
	const bach = "Bach — Brandenburg Concerto No. 3"
	var tpl *overflowItem
	for i := range dv.overflowItems() {
		if dv.overflowItems()[i].label == bach {
			it := dv.overflowItems()[i]
			tpl = &it
		}
	}
	if tpl == nil {
		t.Fatalf("no %q entry on template page", bach)
	}
	tpl.onClick()

	if len(g.pendingImportData) == 0 {
		t.Fatalf("selecting a template did not queue import data")
	}
	var doc struct {
		SchemaVersion string `json:"schema_version"`
		Instruments   []struct {
			ID string `json:"id"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(g.pendingImportData, &doc); err != nil {
		t.Fatalf("queued data not JSON: %v", err)
	}
	if doc.SchemaVersion != schema.Version || len(doc.Instruments) == 0 {
		t.Fatalf("queued data not a template: %+v", doc)
	}
	// Selecting resets the page so reopening starts at File.
	if dv.overflowPage != 0 {
		t.Fatalf("overflowPage=%d want 0 after selection", dv.overflowPage)
	}
}

// The same *Button instances survive across frames, so hover/press edges and deferred taps stay coherent; rows are
// flat, so persistence is asserted on the pointer.
func TestOverflowButtonsPersistAcrossFrames(t *testing.T) {
	// Use the same harness as openTemplatePageClamped.
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(420, 360)
	dv := g.drum
	dv.overflowPage = 1 // Templates page (header + Back + template rows)
	dv.OpenOverflowMenu()

	// Explicitly rebuild so dv.overflowBtns is populated before we capture a pointer.
	dv.rebuildOverflowBtns()
	if len(dv.overflowBtns) == 0 {
		t.Fatal("no overflow buttons built")
	}
	// btns[0] = Back row, btns[1] = first template row (header is excluded).
	firstPtr := dv.overflowBtns[1]

	dst := ebiten.NewImage(420, 360)
	for i := 0; i < 5; i++ {
		dv.drawOverflowMenu(dst)
	}
	if dv.overflowBtns[1] != firstPtr {
		t.Fatal("overflow buttons were recreated across frames — input state lost")
	}
}
