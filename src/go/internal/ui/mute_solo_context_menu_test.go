//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newDrumViewForContextMenuTest(t *testing.T) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.recalcButtons()
	dv.calcLayout()
	return dv
}

// findContextMenuItem finds a context menu item by label prefix.
func findContextMenuItem(items []contextMenuItem, label string) (contextMenuItem, bool) {
	for _, item := range items {
		if item.label == label {
			return item, true
		}
	}
	return contextMenuItem{}, false
}

func TestContextMenuMuteActiveState(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForContextMenuTest(t)
	dv.Rows[0].Muted = true

	items := dv.ContextMenuItemsForTest(0)
	item, ok := findContextMenuItem(items, "Muted")
	if !ok {
		t.Fatal("expected 'Muted' label when row is muted, got items:", itemLabels(items))
	}
	if item.style != ButtonVisual(MuteActiveStyle) {
		t.Fatalf("expected MuteActiveStyle for muted row, got %v", item.style)
	}
}

func TestContextMenuMuteInactiveState(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForContextMenuTest(t)
	// Row 0 not muted (default)

	items := dv.ContextMenuItemsForTest(0)
	item, ok := findContextMenuItem(items, "Mute")
	if !ok {
		t.Fatal("expected 'Mute' label when row is not muted, got items:", itemLabels(items))
	}
	if item.style != ButtonVisual(ContextMenuItemStyle) {
		t.Fatalf("expected ContextMenuItemStyle for non-muted row, got %v", item.style)
	}
}

func TestContextMenuSoloActiveState(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForContextMenuTest(t)
	dv.Rows[0].Solo = true

	items := dv.ContextMenuItemsForTest(0)
	item, ok := findContextMenuItem(items, "Soloed")
	if !ok {
		t.Fatal("expected 'Soloed' label when row is soloed, got items:", itemLabels(items))
	}
	if item.style != ButtonVisual(SoloActiveStyle) {
		t.Fatalf("expected SoloActiveStyle for soloed row, got %v", item.style)
	}
}

func TestContextMenuSoloInactiveState(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForContextMenuTest(t)
	// Row 0 not soloed (default)

	items := dv.ContextMenuItemsForTest(0)
	item, ok := findContextMenuItem(items, "Solo")
	if !ok {
		t.Fatal("expected 'Solo' label when row is not soloed, got items:", itemLabels(items))
	}
	if item.style != ButtonVisual(ContextMenuItemStyle) {
		t.Fatalf("expected ContextMenuItemStyle for non-soloed row, got %v", item.style)
	}
}

func TestContextMenuToggleMuteSoloUpdatesStyle(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := newDrumViewForContextMenuTest(t)

	// Initially neither muted nor soloed
	items := dv.ContextMenuItemsForTest(0)
	if _, ok := findContextMenuItem(items, "Mute"); !ok {
		t.Fatal("expected 'Mute' label initially")
	}
	if _, ok := findContextMenuItem(items, "Solo"); !ok {
		t.Fatal("expected 'Solo' label initially")
	}

	// Toggle mute on
	dv.Rows[0].Muted = true
	items = dv.ContextMenuItemsForTest(0)
	if _, ok := findContextMenuItem(items, "Muted"); !ok {
		t.Fatal("expected 'Muted' after toggling mute on")
	}

	// Toggle mute off
	dv.Rows[0].Muted = false
	items = dv.ContextMenuItemsForTest(0)
	if _, ok := findContextMenuItem(items, "Mute"); !ok {
		t.Fatal("expected 'Mute' after toggling mute off")
	}

	// Toggle solo on
	dv.Rows[0].Solo = true
	items = dv.ContextMenuItemsForTest(0)
	if _, ok := findContextMenuItem(items, "Soloed"); !ok {
		t.Fatal("expected 'Soloed' after toggling solo on")
	}

	// Toggle solo off
	dv.Rows[0].Solo = false
	items = dv.ContextMenuItemsForTest(0)
	if _, ok := findContextMenuItem(items, "Solo"); !ok {
		t.Fatal("expected 'Solo' after toggling solo off")
	}
}

// itemLabels extracts labels from context menu items for error messages.
func itemLabels(items []contextMenuItem) []string {
	var labels []string
	for _, item := range items {
		if !item.divider {
			labels = append(labels, item.label)
		}
	}
	return labels
}
