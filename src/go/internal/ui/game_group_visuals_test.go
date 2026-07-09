package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestSidebarGroupChipOpensGroupMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, _ := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	g.sidebar.Open(g.nodesByID[aID])
	advanceFrames(g, 1)
	// Fire the chip's action directly (button plumbing is covered by the
	// sidebar's own tests); the chip id is "grpchip:<gid>".
	g.sidebar.fireGroupChip(gid)
	advanceFrames(g, 2)
	if g.sidebar.IsOpen() {
		t.Fatalf("sidebar must close when a group chip opens the group menu")
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("group menu must open for chip's group")
	}
}

func TestGroupMembershipQueryForRendering(t *testing.T) {
	g, aID, _ := buildGroupLoopCircuit(t)
	if g.groupIndexSnapshot().Member(aID) {
		t.Fatalf("no groups yet: Member must be false")
	}
	_, _ = g.graph.CreateGroup("", []model.NodeID{aID})
	if !g.groupIndexSnapshot().Member(aID) {
		t.Fatalf("Member(a) must be true after grouping (index refreshed via hook)")
	}
}
