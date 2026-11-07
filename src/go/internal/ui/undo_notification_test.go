//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The affected control may sit in a hidden menu or tab. Buttons and shortcuts both route through performUndo /
// performRedo, so the chokepoint covers every trigger.
func TestUndoRedoShowNotification(t *testing.T) {
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular) // records the "add node" action
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: expected an undoable action")
	}

	g.performUndo()
	requireNotified(t, g, "Undo: add node")

	g.performRedo()
	requireNotified(t, g, "Redo: add node")
}

func TestUndoRedoNoNotificationWhenStackEmpty(t *testing.T) {
	g := newTestGameForUndo(t)
	before := g.drum.notifStore.Len()
	g.performUndo() // nothing to undo
	g.performRedo() // nothing to redo
	if g.drum.notifStore.Len() != before {
		t.Fatalf("no-op undo/redo must not notify (len %d -> %d)", before, g.drum.notifStore.Len())
	}
}

// Through the production input path, not only the chokepoint.
func TestUndoKeyboardShowsNotification(t *testing.T) {
	g := newTestGameForUndo(t)
	warmCircuitForTest(t, g)
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
	// The snapshot diff resolves the single BPM scalar.
	requireNotified(t, g, "Undo: BPM 120 → 140")
}

// The action name too, not only the prefix.
func TestUndoRedoNotificationLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular) // "add node" / "añadir nodo"

	i18n.SetLocale(i18n.LocaleES)
	g.performUndo()
	requireNotified(t, g, "Deshacer: añadir nodo")
	g.performRedo()
	requireNotified(t, g, "Rehacer: añadir nodo")
}

func notifTextOf(n *notification) string {
	if n == nil {
		return "<nil>"
	}
	return n.display()
}

// A new undoable action must get a real translation rather than fall back to English.
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

// In the HISTORY, not Latest(): an undo re-imports, and the warm-up's "Circuit ready" toast may land on top of it,
// depending on suite load. Both orders are correct.
func requireNotified(t *testing.T, g *Game, want string) {
	t.Helper()
	var got []string
	for _, n := range g.drum.notifStore.History() {
		if n.display() == want {
			return
		}
		got = append(got, n.display())
	}
	t.Fatalf("expected notification %q, history (newest first) = %q", want, got)
}
