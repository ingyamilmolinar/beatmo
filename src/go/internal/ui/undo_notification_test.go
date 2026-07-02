//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestUndoRedoShowNotification proves that undoing/redoing surfaces a short,
// clear notification naming the action — so the user sees what changed even when
// the affected control lives in a now-hidden menu/tab. Both the transport buttons
// and the keyboard shortcuts route through performUndo/performRedo, so testing
// that chokepoint covers every trigger.
func TestUndoRedoShowNotification(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular) // records the "add node" action
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected an undoable action")
	}

	g.performUndo()
	if last := g.drum.notifStore.Latest(); last == nil || last.display() != "Undo: add node" {
		t.Fatalf("undo should notify 'Undo: add node', got %q", notifTextOf(g.drum.notifStore.Latest()))
	}

	g.performRedo()
	if last := g.drum.notifStore.Latest(); last == nil || last.display() != "Redo: add node" {
		t.Fatalf("redo should notify 'Redo: add node', got %q", notifTextOf(g.drum.notifStore.Latest()))
	}
}

// TestUndoRedoNoNotificationWhenStackEmpty proves a no-op undo/redo (empty stack)
// does not raise a spurious notification.
func TestUndoRedoNoNotificationWhenStackEmpty(t *testing.T) {
	g := newTestGameForUndo(t)
	before := g.drum.notifStore.Len()
	g.performUndo() // nothing to undo
	g.performRedo() // nothing to redo
	if g.drum.notifStore.Len() != before {
		t.Fatalf("no-op undo/redo must not notify (len %d -> %d)", before, g.drum.notifStore.Len())
	}
}

// TestUndoKeyboardShowsNotification drives a real Ctrl+Z through the production
// input path (g.Update → undoShortcut → performUndo) and asserts the notification
// fires — proving the keyboard trigger (not just the chokepoint) surfaces it.
func TestUndoKeyboardShowsNotification(t *testing.T) {
	g := newTestGameForUndo(t)
	g.drum.SetBPM(140)
	g.undoManager.record("change BPM")
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected an undo step")
	}

	restore := stubAllInput(400, 300,
		map[ebiten.Key]bool{ebiten.KeyControlLeft: true},
		map[ebiten.Key]bool{ebiten.KeyZ: true},
		false)
	defer restore()

	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// The keyboard path surfaces the concrete from → to detail (the snapshot diff
	// resolves the single BPM scalar), not the generic "change BPM" label.
	if last := g.drum.notifStore.Latest(); last == nil || last.display() != "Undo: BPM 120 → 140" {
		t.Fatalf("Ctrl+Z should notify 'Undo: BPM 120 → 140', got %q", notifTextOf(g.drum.notifStore.Latest()))
	}
}

// TestUndoRedoNotificationLocalized proves undo/redo notifications are fully
// localized (prefix + action label) — not just the undo/redo word but the
// action name too. Spanish locale must yield "Deshacer: añadir nodo".
func TestUndoRedoNotificationLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular) // "add node" / "añadir nodo"

	i18n.SetLocale(i18n.LocaleES)
	g.performUndo()
	if last := g.drum.notifStore.Latest(); last == nil || last.display() != "Deshacer: añadir nodo" {
		t.Fatalf("Spanish undo notification = %q, want %q", notifTextOf(g.drum.notifStore.Latest()), "Deshacer: añadir nodo")
	}
	g.performRedo()
	if last := g.drum.notifStore.Latest(); last == nil || last.display() != "Rehacer: añadir nodo" {
		t.Fatalf("Spanish redo notification = %q, want %q", notifTextOf(g.drum.notifStore.Latest()), "Rehacer: añadir nodo")
	}
}

func notifTextOf(n *notification) string {
	if n == nil {
		return "<nil>"
	}
	return n.display()
}

// TestEveryUndoableActionHasLocalizedLabel guards that every undoable
// hooks.Kind has a localized-label mapping, so a newly-added undoable action
// gets a real translation (catalog_test enforces es completeness for the key)
// rather than silently falling back to English in the notification.
func TestEveryUndoableActionHasLocalizedLabel(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	for _, a := range hooks.AllActions() {
		if !hooks.Undoable(a.Kind) {
			continue
		}
		key, ok := undoActionLabelKeys[a.Label]
		if !ok {
			t.Errorf("undoable action %q (label %q) has no entry in undoActionLabelKeys — add a localized label", a.Kind, a.Label)
			continue
		}
		if got := i18n.T(key); got != a.Label {
			t.Errorf("EN i18n label for %q = %q, want the registry label %q", a.Kind, got, a.Label)
		}
	}
}
