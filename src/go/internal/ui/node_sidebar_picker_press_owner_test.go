//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A press begun on the open picker owns its gesture to the release: confirming closes the picker on the press
// frame, and the held finger must not then fire the control beneath (on a phone, Audible, which turned the
// chord back into a note).
func TestChordConfirmHeldPressDoesNotFireTheControlBeneath(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	g.Layout(390, 844)
	// The tour's state: only the Pitch and Audible sections open.
	for k := range g.sidebar.sectionOpen {
		g.sidebar.sectionOpen[k] = false
	}
	g.sidebar.sectionOpen[secPit], g.sidebar.sectionOpen[secAud] = true, true
	g.sidebar.layout()
	sidebarClick(t, g, sbAudible) // → chord
	if nodeTypeOf(t, g, node.ID) != model.NodeTypeChord {
		t.Fatal("fixture: the node did not become a chord")
	}
	sidebarClick(t, g, sbPitchNote)
	if !g.sidebar.chordMatrixIsOpen() {
		t.Fatal("fixture: the chord matrix did not open")
	}

	// A visible cell lying over a sidebar control: the case the latch exists for.
	var cell image.Rectangle
	var under sbID
	for id, r := range g.sidebar.rects {
		if id.el != elChordCell {
			continue
		}
		for cid, cr := range g.sidebar.rects {
			if cid == sbAudible && r.Overlaps(cr) {
				cell, under = r, cid
			}
		}
	}
	if cell.Empty() {
		t.Fatal("no chord cell lies over the Audible selector at 390x844 — the regression case is not reproduced")
	}

	heldTap := func(p image.Point) {
		for i := 0; i < 3; i++ {
			frameAt(g, p.X, p.Y, true)
		}
		frameAt(g, p.X, p.Y, false)
		idleFrame(g)
	}
	c := image.Pt(cell.Min.X+cell.Dx()/2, cell.Min.Y+cell.Dy()/2)
	heldTap(c) // pick
	heldTap(c) // re-tap = confirm (closes the picker on the press frame)

	if g.sidebar.chordMatrixIsOpen() {
		t.Error("re-tapping the picked cell did not confirm and close the chord matrix")
	}
	if got := nodeTypeOf(t, g, node.ID); got != model.NodeTypeChord {
		t.Fatalf("confirming the chord fired the %q control under the cell: node type %v, want chord", under, got)
	}
}
