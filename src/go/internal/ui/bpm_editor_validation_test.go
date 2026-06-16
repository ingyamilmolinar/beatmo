package ui

import (
	"testing"
)

// Typing non-digits should not change BPM on commit and should trigger the
// editor's error highlight.
func TestBPMEditorRejectsNonDigits(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	_ = g.Update()

	// Open the shared editor, type a mix of digits and letters, then commit.
	g.drum.transportZone.openBPMEditor()
	ed := g.drum.transportZone.paramEditor
	ed.ti.SetText("1a9X")
	ed.commit()

	if g.drum.transportZone.BPM() != 120 {
		t.Fatalf("BPM changed unexpectedly: %d", g.drum.transportZone.BPM())
	}
	if ed.errorAnim == 0 {
		t.Fatalf("expected editor error highlight on invalid input")
	}
}

// Empty BPM input should be rejected (invalid parse) and leave the current BPM
// untouched, flashing the editor error like any other invalid input.
func TestBPMEditorRejectsEmpty(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	_ = g.Update()

	g.drum.transportZone.openBPMEditor()
	ed := g.drum.transportZone.paramEditor
	ed.ti.SetText("")
	ed.commit()

	if g.drum.transportZone.BPM() != 120 {
		t.Fatalf("BPM changed unexpectedly: %d", g.drum.transportZone.BPM())
	}
	if ed.errorAnim == 0 {
		t.Fatalf("expected editor error highlight on empty (invalid) input")
	}
}
