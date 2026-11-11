package ui

import "testing"

// TestUndoManager_CaptureFailureSkips proves the manager never records a bogus
// step or corrupts its baseline when capture() returns nil — which undoCapture
// does whenever exportBytes errors or panics (e.g. a transient/partially-built
// document reached through a tapped emit helper). Regression for the
// SetLength → emitLengthChange → recordUndo → undoCapture → exportBytes panic.
func TestUndoManager_CaptureFailureSkips(t *testing.T) {
	state := "A"
	failing := true
	m := NewUndoManager(
		func() []byte {
			if failing {
				return nil
			}
			return []byte(state)
		},
		func(b []byte) error { state = string(b); return nil },
	)

	// Initial capture failed → committed is empty.
	state = "B"
	m.record("while-failing")
	if m.CanUndo() {
		t.Fatal("must not record a step while capture fails")
	}

	// Capture starts working → adopt the first valid snapshot as baseline,
	// without recording a step to undo "to nothing".
	failing = false
	m.record("adopt-baseline")
	if m.CanUndo() {
		t.Fatal("adopting the first valid baseline must not create an undo step")
	}

	// A real change now records normally and undoes to the adopted baseline.
	state = "C"
	m.record("real-change")
	if !m.CanUndo() {
		t.Fatal("expected an undo step after a real change")
	}
	m.Undo()
	if state != "B" {
		t.Fatalf("undo restored %q, want B (the adopted baseline)", state)
	}
}
