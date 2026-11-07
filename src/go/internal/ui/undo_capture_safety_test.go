package ui

import "testing"

// undoCapture returns nil when exportBytes errors or panics on a half-built document; the manager then records no
// bogus step and keeps its baseline.
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

	// The first valid snapshot becomes the baseline without a step to undo "to nothing".
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
