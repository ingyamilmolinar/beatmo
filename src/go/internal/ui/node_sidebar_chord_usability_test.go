//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Usability of the chord builder the way a USER meets it: the card opens with
// every section collapsed, nothing is scrolled for the test, and a control can
// only be pressed where the drawn card shows it. The older chord tests opened
// every section (ExpandAllSections) and scrolled each target into view before
// clicking, so all three of these defects passed them:
//   - the Chord section header did not toggle (it was missing from the
//     sidebar's hard-coded header lists), so a chord node reopened later could
//     never show its builder;
//   - the type cycle pushed its own Audible button below the card on
//     Regular → Chord (the auto-expanded Chord section grew the content above
//     it), so the second tap of a "rotate" landed on nothing;
//   - the chord matrix opened only from the Pitch chip, inside a collapsed
//     section the Chord section gave no hint of.

// chordUser drives the sidebar like a person: tap = real input through
// Game.Update, then one drawn frame (the sidebar lays out in Draw).
type chordUser struct {
	t      *testing.T
	g      *Game
	node   *uiNode
	screen *ebiten.Image
	tapFn  func(pt image.Point)
}

func (u *chordUser) frame() { u.g.Draw(u.screen) }

// visibleRect is the drawn rect of a control, failing when the card does not
// show it (absent, or scrolled out of the card's viewport).
func (u *chordUser) visibleRect(id sbID) image.Rectangle {
	u.t.Helper()
	r := u.g.sidebar.rects[id]
	if r.Empty() {
		u.t.Fatalf("control %q is not on the card", id)
	}
	if id.isPickerCell() { // a picker cell, not a card control
		if p := u.g.sidebar.openPicker(); p == nil || !r.In(p.InputBounds()) {
			u.t.Fatalf("picker cell %q at %v is not inside the open picker", id, r)
		}
		return r
	}
	if !u.g.sidebar.inViewport(r) || !r.Overlaps(u.g.sidebar.rects[sbPanel]) {
		u.t.Fatalf("control %q at %v is outside the visible card %v", id, r, u.g.sidebar.rects[sbPanel])
	}
	return r
}

func (u *chordUser) tapPoint(pt image.Point) {
	u.tapFn(pt)
	u.frame()
}

func (u *chordUser) tap(id sbID) image.Point {
	u.t.Helper()
	r := u.visibleRect(id)
	pt := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	u.tapPoint(pt)
	return pt
}

func (u *chordUser) nodeType() model.NodeType {
	n, _ := u.g.graph.GetNodeByID(u.node.ID)
	return n.Type
}

func newChordUser(t *testing.T, mobile bool) *chordUser {
	t.Helper()
	var g *Game
	if mobile {
		g = newSettledMobileGame(t)
	} else {
		g = newTestGameForUndo(t)
		g.Layout(1280, 720)
	}
	g.drum.Rows[0].Instrument = roster.OrganChurch
	node := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if node == nil || g.start == nil {
		t.Fatal("fixture: no start node / test node")
	}
	g.addEdge(g.start, node)
	g.addEdge(node, g.start)
	g.updateBeatInfos()
	u := &chordUser{t: t, g: g, node: node, screen: ebiten.NewImage(g.winW, g.winH)}
	if mobile {
		u.tapFn = func(pt image.Point) { realTouchPath(t, g, pt, pt) }
	} else {
		u.tapFn = func(pt image.Point) { nmClick(g, pt.X, pt.Y) }
	}
	g.sidebar.Open(node) // every section collapsed, as a user first sees it
	u.frame()
	return u
}

func forEachPointer(t *testing.T, fn func(t *testing.T, u *chordUser)) {
	t.Run("desktop-mouse", func(t *testing.T) { fn(t, newChordUser(t, false)) })
	t.Run("mobile-touch", func(t *testing.T) { fn(t, newChordUser(t, true)) })
}

// TestChordTypeRotationByRepeatedTaps: open Audible and keep tapping the type
// button. Every tap must land on it (it stays on the card) and advance the
// cycle Regular → Chord → Silent → Mute → Regular → Chord.
func TestChordTypeRotationByRepeatedTaps(t *testing.T) {
	forEachPointer(t, func(t *testing.T, u *chordUser) {
		u.tap(sbSectionHeader(secAud))
		want := []model.NodeType{model.NodeTypeChord, model.NodeTypeSilent, model.NodeTypeMute,
			model.NodeTypeRegular, model.NodeTypeChord}
		for i, w := range want {
			u.tap(sbAudible)
			if got := u.nodeType(); got != w {
				t.Fatalf("tap %d: type = %s, want %s", i+1, got.String(), w.String())
			}
			if !u.g.sidebar.IsOpen() {
				t.Fatalf("tap %d closed the card", i+1)
			}
		}
	})
}

// TestChordTypeTapKeepsTheButtonUnderThePointer: Regular → Chord grows the
// card above the Audible button; the button must still be under the point
// that was just tapped, so a second tap without moving rotates again.
func TestChordTypeTapKeepsTheButtonUnderThePointer(t *testing.T) {
	forEachPointer(t, func(t *testing.T, u *chordUser) {
		u.tap(sbSectionHeader(secAud))
		pt := u.tap(sbAudible) // Regular → Chord
		if u.nodeType() != model.NodeTypeChord {
			t.Fatalf("type = %s, want chord", u.nodeType().String())
		}
		if r := u.visibleRect(sbAudible); !pt.In(r) {
			t.Fatalf("Audible button moved from under the pointer: tapped %v, button now %v", pt, r)
		}
		u.tapPoint(pt) // same spot, no re-aim
		if u.nodeType() != model.NodeTypeSilent {
			t.Fatalf("second tap at the same spot: type = %s, want silent", u.nodeType().String())
		}
	})
}

// TestChordSectionHeaderToggles: a chord node opened fresh shows its Chord
// section collapsed; tapping the header must expand it (the builder appears)
// and tapping again must collapse it.
func TestChordSectionHeaderToggles(t *testing.T) {
	forEachPointer(t, func(t *testing.T, u *chordUser) {
		u.g.setNodeType(u.node.ID, model.NodeTypeChord)
		u.g.sidebar.Close()
		u.g.sidebar.Open(u.node)
		u.frame()
		if u.g.sidebar.sectionOpen[secChord] {
			t.Fatal("fixture: chord section should start collapsed on a fresh open")
		}
		u.tap(sbSectionHeader(secChord))
		if !u.g.sidebar.sectionOpen[secChord] {
			t.Fatal("tapping the Chord header did not expand the section")
		}
		u.visibleRect(sbPlus(fieldChordInv))
		u.tap(sbSectionHeader(secChord))
		if u.g.sidebar.sectionOpen[secChord] {
			t.Fatal("tapping the Chord header again did not collapse the section")
		}
	})
}

// TestEverySidebarSectionHeaderToggles: every section header the card draws
// toggles its section — so a future section cannot ship with a dead header.
func TestEverySidebarSectionHeaderToggles(t *testing.T) {
	u := newChordUser(t, false)
	u.g.setNodeType(u.node.ID, model.NodeTypeChord)
	u.g.sidebar.Close()
	u.g.sidebar.Open(u.node)
	u.frame()
	for _, sec := range nodeSidebarSections {
		id := sbSectionHeader(sec)
		if u.g.sidebar.rects[id].Empty() {
			continue
		}
		before := u.g.sidebar.sectionOpen[sec]
		u.tap(id)
		if u.g.sidebar.sectionOpen[sec] == before {
			t.Errorf("header %q did not toggle its section", id)
		}
		u.tap(id) // restore so the card stays short enough to reach the rest
	}
}

// TestChordSectionChipOpensTheChordMatrix: the Chord section leads with a chip
// naming the chord; tapping it opens the chord matrix (no detour through the
// Pitch section), a cell applies, and the chip closes it again.
func TestChordSectionChipOpensTheChordMatrix(t *testing.T) {
	forEachPointer(t, func(t *testing.T, u *chordUser) {
		u.tap(sbSectionHeader(secAud))
		u.tap(sbAudible) // Regular → Chord: the Chord section opens with it
		if !u.g.sidebar.sectionOpen[secChord] {
			t.Fatal("entering Chord should open the Chord section")
		}
		u.tap(sbChordName)
		if !u.g.sidebar.chordMatrixIsOpen() {
			t.Fatal("tapping the chord chip did not open the chord matrix")
		}
		ids := u.g.sidebar.chordMatrixCellIDs()
		if len(ids) == 0 {
			t.Fatal("chord matrix shows no cells")
		}
		pick := cellOutsideCard(t, u, ids)
		before := chordOf(t, u.g, u.node.ID)
		pitchBefore, _ := u.g.graph.GetNodeByID(u.node.ID)
		u.tap(pick)
		after := chordOf(t, u.g, u.node.ID)
		pitchAfter, _ := u.g.graph.GetNodeByID(u.node.ID)
		if after == before && pitchAfter.Params.Pitch == pitchBefore.Params.Pitch {
			t.Fatalf("picking %s changed nothing (chord %+v)", pick, after)
		}
		if !u.g.sidebar.chordMatrixIsOpen() {
			t.Fatal("a pick must keep the matrix open (select applies, re-select confirms)")
		}
		u.tap(pick) // re-selecting the current chord confirms and closes
		if u.g.sidebar.chordMatrixIsOpen() {
			t.Fatal("re-selecting the current chord did not close the matrix")
		}
		if !u.g.sidebar.IsOpen() {
			t.Fatal("the confirming tap closed the node card too (treated as a tap outside it)")
		}
		// Where the chip is not under the picker (desktop), it toggles too.
		u.tap(sbChordName)
		if !u.g.sidebar.chordMatrixIsOpen() {
			t.Fatal("the chord chip did not reopen the matrix")
		}
		if p := u.g.sidebar.openPicker(); !u.g.sidebar.rects[sbChordName].Overlaps(p.InputBounds()) {
			u.tap(sbChordName)
			if u.g.sidebar.chordMatrixIsOpen() {
				t.Fatal("tapping the chord chip again did not close the matrix")
			}
		}
	})
}

// cellOutsideCard is a visible picker cell that does NOT overlap the node card
// — the confirming tap there used to read as a tap OUTSIDE the card once the
// picker had closed under it, and closed the card too.
func cellOutsideCard(t *testing.T, u *chordUser, ids []sbID) sbID {
	t.Helper()
	panel := u.g.sidebar.rects[sbPanel]
	for _, id := range ids {
		if r := u.g.sidebar.rects[id]; !r.Empty() && !r.Overlaps(panel) {
			return id
		}
	}
	t.Fatalf("fixture: every picker cell overlaps the card %v", panel)
	return sbID{}
}

// TestNoteMatrixConfirmOutsideCardKeepsTheCard: the same confirm-closes-the-
// card defect on the note matrix (shared picker host).
func TestNoteMatrixConfirmOutsideCardKeepsTheCard(t *testing.T) {
	forEachPointer(t, func(t *testing.T, u *chordUser) {
		u.tap(sbSectionHeader(secPit))
		u.tap(sbPitchNote)
		if !u.g.sidebar.noteMatrixIsOpen() {
			t.Fatal("the note chip did not open the note matrix")
		}
		pick := cellOutsideCard(t, u, u.g.sidebar.noteMatrixCellIDs())
		u.tap(pick)
		if !u.g.sidebar.noteMatrixIsOpen() {
			t.Fatal("a pick must keep the note matrix open")
		}
		nodes := len(u.g.graph.Nodes)
		u.tap(pick)
		if u.g.sidebar.noteMatrixIsOpen() {
			t.Fatal("re-selecting the current note did not close the matrix")
		}
		if !u.g.sidebar.IsOpen() {
			t.Fatal("the confirming tap closed the node card too")
		}
		if len(u.g.graph.Nodes) != nodes {
			t.Fatal("the confirming tap created a node beneath the picker")
		}
	})
}

// TestMobilePickerLeavesTheTopBarAndAnOutsideToTap: on a phone the grid
// pickers (note and chord matrix) used to fill the whole grid pane — under
// the top-right buttons, which hid their header, and with no "outside" left
// to tap, while mobile has no close × by design. They now start below the
// top-right button row, so the strip beside those buttons is outside the
// picker and an outside tap there closes it (and the card) like any menu.
func TestMobilePickerLeavesTheTopBarAndAnOutsideToTap(t *testing.T) {
	for _, which := range []string{"chord", "note"} {
		t.Run(which, func(t *testing.T) {
			u := newChordUser(t, true)
			if which == "chord" {
				u.tap(sbSectionHeader(secAud))
				u.tap(sbAudible)
				u.tap(sbChordName)
			} else {
				u.tap(sbSectionHeader(secPit))
				u.tap(sbPitchNote)
			}
			p := u.g.sidebar.openPicker()
			if p == nil {
				t.Fatal("picker did not open")
			}
			reserved := u.g.gridTopButtonsReservedRect()
			if pb := p.InputBounds(); pb.Overlaps(reserved) {
				t.Fatalf("picker %v sits under the top-right buttons %v", pb, reserved)
			}
			// Some grid point must be outside every open surface.
			grid := u.g.split.GridRect(u.g.winW, u.g.winH)
			var outside image.Point
			found := false
			for y := grid.Min.Y + 8; y < grid.Max.Y && !found; y += 8 {
				for x := grid.Min.X + 8; x < grid.Max.X && !found; x += 8 {
					pt := image.Pt(x, y)
					box := image.Rect(x-22, y-22, x+22, y+22) // a fingertip
					if !u.g.sidebar.Hit(x, y) && box.In(grid) && !box.Overlaps(reserved) &&
						!pt.In(u.g.split.InputBounds()) &&
						!box.Overlaps(u.g.sidebar.rects[sbPanel]) && !box.Overlaps(p.InputBounds()) {
						outside, found = pt, true
					}
				}
			}
			if !found {
				t.Fatalf("no touch-sized point of the grid pane %v is outside the card %v and picker %v",
					grid, u.g.sidebar.rects[sbPanel], p.InputBounds())
			}
			nodes := len(u.g.graph.Nodes)
			u.tapPoint(outside)
			if u.g.sidebar.openPicker() != nil || u.g.sidebar.IsOpen() {
				t.Fatalf("an outside tap at %v did not close the picker and card", outside)
			}
			if len(u.g.graph.Nodes) != nodes {
				t.Fatal("the outside tap created a node")
			}
		})
	}
}
