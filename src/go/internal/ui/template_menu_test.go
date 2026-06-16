package ui

import (
	"encoding/json"
	"testing"
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
	for _, want := range []string{"Rock", "Hip-Hop", "Pop", "Funk", "Salsa", "House", "Techno"} {
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

	// Click the "Rock" entry.
	var rock *overflowItem
	for i := range dv.overflowItems() {
		if dv.overflowItems()[i].label == "Rock" {
			it := dv.overflowItems()[i]
			rock = &it
		}
	}
	if rock == nil {
		t.Fatalf("no Rock entry on template page")
	}
	rock.onClick()

	if len(g.pendingImportData) == 0 {
		t.Fatalf("selecting a template did not queue import data")
	}
	var doc struct {
		Version     int `json:"version"`
		Instruments []struct {
			ID string `json:"id"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(g.pendingImportData, &doc); err != nil {
		t.Fatalf("queued data not JSON: %v", err)
	}
	if doc.Version != 1 || len(doc.Instruments) == 0 {
		t.Fatalf("queued data not a template: %+v", doc)
	}
	// Selecting resets the page so reopening starts at File.
	if dv.overflowPage != 0 {
		t.Fatalf("overflowPage=%d want 0 after selection", dv.overflowPage)
	}
}
