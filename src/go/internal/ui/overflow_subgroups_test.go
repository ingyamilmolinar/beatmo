//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestOverflowSubgroups_HasFileHeader verifies the overflow menu groups
// File-group items under a labelled "File" header. The "View" subgroup
// was removed when its only entries (Window length +/−) were retired in
// favour of the inline timeline controls.
func TestOverflowSubgroups_HasFileHeader(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum

	items := dv.overflowItems()
	var headers []string
	for _, it := range items {
		if it.header {
			headers = append(headers, it.label)
		}
	}
	hasFile := false
	for _, h := range headers {
		if h == "File" {
			hasFile = true
		}
		if h == "View" {
			t.Errorf("overflowItems should no longer contain 'View' header; got headers=%v", headers)
		}
	}
	if !hasFile {
		t.Errorf("overflowItems missing 'File' header; got headers=%v", headers)
	}

	// File header precedes Upload/Import/Export.
	idxFile, idxUpload := -1, -1
	for i, it := range items {
		if it.header && it.label == "File" {
			idxFile = i
		}
		if !it.header && it.label == "Upload" {
			idxUpload = i
		}
	}
	if idxFile == -1 || idxUpload == -1 {
		t.Fatalf("missing File header (%d) or Upload entry (%d)", idxFile, idxUpload)
	}
	if idxFile >= idxUpload {
		t.Errorf("File header at %d should precede Upload at %d", idxFile, idxUpload)
	}
}

// TestOverflowSubgroups_HeadersNotClickable verifies header rows have
// no onClick handler and don't fire actions when tapped.
func TestOverflowSubgroups_HeadersNotClickable(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	for _, it := range g.drum.overflowItems() {
		if it.header && it.onClick != nil {
			t.Errorf("header %q should have nil onClick, got non-nil", it.label)
		}
	}
}

// TestOverflowSubgroups_HeaderRowsNotInButtonList verifies the popup
// button list (overflowPopupBtns) skips header rows so taps on them
// don't fire actions even if they sit at a hit-area position.
func TestOverflowSubgroups_HeaderRowsNotInButtonList(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.drum.openOverflowMenuPortal()
	g.Layout(390, 844)
	g.Update()

	popupRect := g.drum.overflowPopupRect()
	btns := g.drum.overflowPopupBtns(popupRect)
	for _, b := range btns {
		// Header text is "File" or "View"; if either appears in the button
		// list, that's a regression.
		if b.Text == "File" || b.Text == "View" {
			t.Errorf("button list contains header %q (should be excluded)", b.Text)
		}
	}
}
