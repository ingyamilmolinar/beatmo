package model

import (
	"testing"
)

// TestCalculateBeatRow_ReverseVertical verifies that an edge from a high-J node
// to a low-J node produces intermediate invisible entries in descending J order.
func TestCalculateBeatRow_ReverseVertical(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 3, NodeTypeRegular)
	n1 := g.AddNode(0, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	// Expected: n0 at (0,3), intermediates at J=2 and J=1, then n1 at (0,0).
	if len(beatInfos) < 5 {
		t.Fatalf("expected at least 5 entries (2 nodes + 2 intermediates + padding), got %d", len(beatInfos))
	}
	// Entry 0: start node at (0,3)
	if beatInfos[0].NodeID != n0 || beatInfos[0].I != 0 || beatInfos[0].J != 3 {
		t.Fatalf("expected start node at (0,3), got %+v", beatInfos[0])
	}
	// Entry 1: intermediate at (0,2)
	if beatInfos[1].NodeID != InvalidNodeID || beatInfos[1].NodeType != NodeTypeInvisible || beatInfos[1].I != 0 || beatInfos[1].J != 2 {
		t.Fatalf("expected invisible intermediate at (0,2), got %+v", beatInfos[1])
	}
	// Entry 2: intermediate at (0,1)
	if beatInfos[2].NodeID != InvalidNodeID || beatInfos[2].NodeType != NodeTypeInvisible || beatInfos[2].I != 0 || beatInfos[2].J != 1 {
		t.Fatalf("expected invisible intermediate at (0,1), got %+v", beatInfos[2])
	}
	// Entry 3: end node at (0,0)
	if beatInfos[3].NodeID != n1 || beatInfos[3].I != 0 || beatInfos[3].J != 0 {
		t.Fatalf("expected end node at (0,0), got %+v", beatInfos[3])
	}
}

// TestCalculateBeatRow_ReverseHorizontal verifies that an edge from a high-I
// node to a low-I node produces intermediate invisible entries in descending I order.
func TestCalculateBeatRow_ReverseHorizontal(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(3, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	// Expected: n0 at (3,0), intermediates at I=2 and I=1, then n1 at (0,0).
	if len(beatInfos) < 5 {
		t.Fatalf("expected at least 5 entries, got %d", len(beatInfos))
	}
	// Entry 0: start node at (3,0)
	if beatInfos[0].NodeID != n0 || beatInfos[0].I != 3 || beatInfos[0].J != 0 {
		t.Fatalf("expected start node at (3,0), got %+v", beatInfos[0])
	}
	// Entry 1: intermediate at (2,0)
	if beatInfos[1].NodeID != InvalidNodeID || beatInfos[1].NodeType != NodeTypeInvisible || beatInfos[1].I != 2 || beatInfos[1].J != 0 {
		t.Fatalf("expected invisible intermediate at (2,0), got %+v", beatInfos[1])
	}
	// Entry 2: intermediate at (1,0)
	if beatInfos[2].NodeID != InvalidNodeID || beatInfos[2].NodeType != NodeTypeInvisible || beatInfos[2].I != 1 || beatInfos[2].J != 0 {
		t.Fatalf("expected invisible intermediate at (1,0), got %+v", beatInfos[2])
	}
	// Entry 3: end node at (0,0)
	if beatInfos[3].NodeID != n1 || beatInfos[3].I != 0 || beatInfos[3].J != 0 {
		t.Fatalf("expected end node at (0,0), got %+v", beatInfos[3])
	}
}

// TestCalculateBeatRow_HorizontalIntermediateRegularIsInvisible verifies that a
// regular node sitting at an intermediate horizontal position between two
// connected nodes gets NodeID=InvalidNodeID (treated as invisible pass-through).
func TestCalculateBeatRow_HorizontalIntermediateRegularIsInvisible(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	nMid := g.AddNode(1, 0, NodeTypeRegular) // Regular node at intermediate position
	n1 := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	if len(beatInfos) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(beatInfos))
	}
	// Entry 0: start node
	if beatInfos[0].NodeID != n0 {
		t.Fatalf("expected start node %d, got %+v", n0, beatInfos[0])
	}
	// Entry 1: intermediate regular node should be invisible placeholder
	mid := beatInfos[1]
	if mid.NodeID != InvalidNodeID {
		t.Fatalf("expected intermediate regular node to have InvalidNodeID, got NodeID=%d (nMid=%d)", mid.NodeID, nMid)
	}
	if mid.NodeType != NodeTypeInvisible {
		t.Fatalf("expected NodeTypeInvisible for intermediate regular, got %d", mid.NodeType)
	}
	if mid.I != 1 || mid.J != 0 {
		t.Fatalf("expected intermediate at (1,0), got (%d,%d)", mid.I, mid.J)
	}
	// Entry 2: end node
	if beatInfos[2].NodeID != n1 {
		t.Fatalf("expected end node %d, got %+v", n1, beatInfos[2])
	}
}

// TestCalculateBeatRow_HorizontalIntermediateInvisibleKeepsID verifies that an
// invisible node at an intermediate horizontal position keeps its actual NodeID
// (not InvalidNodeID) in the beat row.
func TestCalculateBeatRow_HorizontalIntermediateInvisibleKeepsID(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	nMid := g.AddNode(1, 0, NodeTypeInvisible) // Invisible node at intermediate position
	n1 := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	if len(beatInfos) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(beatInfos))
	}
	// Entry 0: start node
	if beatInfos[0].NodeID != n0 {
		t.Fatalf("expected start node %d, got %+v", n0, beatInfos[0])
	}
	// Entry 1: intermediate invisible node should keep its actual ID
	mid := beatInfos[1]
	if mid.NodeID != nMid {
		t.Fatalf("expected intermediate invisible node to keep ID %d, got %d", nMid, mid.NodeID)
	}
	if mid.NodeType != NodeTypeInvisible {
		t.Fatalf("expected NodeTypeInvisible, got %d", mid.NodeType)
	}
	if mid.I != 1 || mid.J != 0 {
		t.Fatalf("expected intermediate at (1,0), got (%d,%d)", mid.I, mid.J)
	}
	// Entry 2: end node
	if beatInfos[2].NodeID != n1 {
		t.Fatalf("expected end node %d, got %+v", n1, beatInfos[2])
	}
}

// TestCalculateBeatRow_MissingNodeStopsTraversal verifies that traversal stops
// gracefully when a node referenced by an edge is missing from the Nodes map.
func TestCalculateBeatRow_MissingNodeStopsTraversal(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}

	// Delete nB from the Nodes map directly (not via RemoveNode, to keep the edge).
	delete(g.Nodes, nB)

	g.SetBeatLength(8)
	beatInfos, isLoop, loopStart := g.CalculateBeatRow()

	// Should not be a loop since traversal stopped at the missing node.
	if isLoop {
		t.Fatalf("expected no loop when node is missing, got isLoop=true")
	}
	if loopStart != -1 {
		t.Fatalf("expected loopStart=-1, got %d", loopStart)
	}
	// The beat row should have nA followed by padding.
	if len(beatInfos) != 8 {
		t.Fatalf("expected beat row length 8, got %d", len(beatInfos))
	}
	if beatInfos[0].NodeID != nA {
		t.Fatalf("expected first entry to be node %d, got %+v", nA, beatInfos[0])
	}
	// Remaining entries should be invisible padding.
	for i := 1; i < 8; i++ {
		if beatInfos[i].NodeID != InvalidNodeID || beatInfos[i].NodeType != NodeTypeInvisible {
			t.Fatalf("expected invisible padding at index %d, got %+v", i, beatInfos[i])
		}
	}
}

// TestCalculateBeatRow_LoopWithPrefix verifies that a circuit with a prefix
// before the loop (A->B->C->B) correctly produces [A, B, C, B, C, B, C] when
// beat length is 7.
func TestCalculateBeatRow_LoopWithPrefix(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}
	g.Edges[[2]NodeID{nC, nB}] = struct{}{} // Loop back to B

	g.SetBeatLength(7)
	beatInfos, isLoop, loopStart := g.CalculateBeatRow()

	if !isLoop {
		t.Fatalf("expected loop to be detected")
	}
	// Prefix is [A], loop segment is [B, C], so loopStart should be 1.
	if loopStart != 1 {
		t.Fatalf("expected loopStart=1, got %d", loopStart)
	}
	if len(beatInfos) != 7 {
		t.Fatalf("expected beat row length 7, got %d", len(beatInfos))
	}

	// Expected: [A, B, C, B, C, B, C]
	expectedIDs := []NodeID{nA, nB, nC, nB, nC, nB, nC}
	for i, want := range expectedIDs {
		if beatInfos[i].NodeID != want {
			t.Fatalf("at index %d: expected NodeID %d, got %d", i, want, beatInfos[i].NodeID)
		}
		if beatInfos[i].NodeType != NodeTypeRegular {
			t.Fatalf("at index %d: expected NodeTypeRegular, got %d", i, beatInfos[i].NodeType)
		}
	}
}

// TestIsLoop_NoEdges verifies that IsLoop returns false for a graph with nodes
// but no edges.
func TestIsLoop_NoEdges(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	g.AddNode(1, 0, NodeTypeRegular)
	g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = n0

	if g.IsLoop() {
		t.Fatal("expected IsLoop to return false when there are no edges")
	}
}

// TestCalculateBeatRowUnbounded_ReturnsRawWithoutPadding verifies that the
// unbounded variant returns exactly the raw traversal entries without padding
// to beat length or expanding loops.
func TestCalculateBeatRowUnbounded_ReturnsRawWithoutPadding(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}

	g.SetBeatLength(16)

	row, isLoop, loopStart := g.CalculateBeatRowUnbounded()

	if isLoop {
		t.Fatalf("expected no loop, got isLoop=true")
	}
	if loopStart != -1 {
		t.Fatalf("expected loopStart=-1, got %d", loopStart)
	}
	// A->B->C is a straight line, each adjacent by 1 in I, same J.
	// No intermediates since nodes are adjacent (I differs by 1).
	if len(row) != 3 {
		t.Fatalf("expected exactly 3 entries (no padding to beat length 16), got %d", len(row))
	}
	expectedIDs := []NodeID{nA, nB, nC}
	for i, want := range expectedIDs {
		if row[i].NodeID != want {
			t.Fatalf("at index %d: expected NodeID %d, got %d", i, want, row[i].NodeID)
		}
	}
}

// TestCalculateBeatRow_VerticalIntermediateRegularIsInvisible verifies that a
// regular node at an intermediate vertical position between two connected nodes
// gets NodeID=InvalidNodeID (treated as invisible pass-through).
func TestCalculateBeatRow_VerticalIntermediateRegularIsInvisible(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	nMid := g.AddNode(0, 1, NodeTypeRegular) // Regular node at vertical intermediate
	n1 := g.AddNode(0, 3, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	if len(beatInfos) < 4 {
		t.Fatalf("expected at least 4 entries (start + 2 intermediates + end), got %d", len(beatInfos))
	}
	// Entry 0: start node at (0,0)
	if beatInfos[0].NodeID != n0 {
		t.Fatalf("expected start node %d, got %+v", n0, beatInfos[0])
	}
	// Entry 1: intermediate at (0,1) — regular node treated as invisible
	mid := beatInfos[1]
	if mid.NodeID != InvalidNodeID {
		t.Fatalf("expected vertical intermediate regular node to have InvalidNodeID, got NodeID=%d (nMid=%d)", mid.NodeID, nMid)
	}
	if mid.NodeType != NodeTypeInvisible {
		t.Fatalf("expected NodeTypeInvisible for intermediate regular, got %d", mid.NodeType)
	}
	if mid.I != 0 || mid.J != 1 {
		t.Fatalf("expected intermediate at (0,1), got (%d,%d)", mid.I, mid.J)
	}
	// Entry 2: intermediate at (0,2) — no node present, synthesized invisible
	mid2 := beatInfos[2]
	if mid2.NodeID != InvalidNodeID {
		t.Fatalf("expected synthesized intermediate at (0,2) to have InvalidNodeID, got %d", mid2.NodeID)
	}
	if mid2.NodeType != NodeTypeInvisible {
		t.Fatalf("expected NodeTypeInvisible at (0,2), got %d", mid2.NodeType)
	}
	// Entry 3: end node at (0,3)
	if beatInfos[3].NodeID != n1 {
		t.Fatalf("expected end node %d, got %+v", n1, beatInfos[3])
	}
}

// TestCalculateBeatRow_VerticalIntermediateInvisibleKeepsID verifies that an
// invisible node at a vertical intermediate position keeps its actual NodeID.
func TestCalculateBeatRow_VerticalIntermediateInvisibleKeepsID(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	nMid := g.AddNode(0, 1, NodeTypeInvisible) // Invisible node at intermediate
	n1 := g.AddNode(0, 3, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	beatInfos, _, _ := g.CalculateBeatRow()

	if len(beatInfos) < 4 {
		t.Fatalf("expected at least 4 entries, got %d", len(beatInfos))
	}
	// Entry 0: start node
	if beatInfos[0].NodeID != n0 {
		t.Fatalf("expected start node %d, got %+v", n0, beatInfos[0])
	}
	// Entry 1: intermediate invisible node should keep its actual ID
	mid := beatInfos[1]
	if mid.NodeID != nMid {
		t.Fatalf("expected intermediate invisible node to keep ID %d, got %d", nMid, mid.NodeID)
	}
	if mid.NodeType != NodeTypeInvisible {
		t.Fatalf("expected NodeTypeInvisible, got %d", mid.NodeType)
	}
	if mid.I != 0 || mid.J != 1 {
		t.Fatalf("expected intermediate at (0,1), got (%d,%d)", mid.I, mid.J)
	}
	// Entry 3: end node
	if beatInfos[3].NodeID != n1 {
		t.Fatalf("expected end node %d, got %+v", n1, beatInfos[3])
	}
}

// TestCalculateBeatRow_SelfLoop verifies that a node with an edge to itself
// produces a loop that repeats the single node to fill beat length.
func TestCalculateBeatRow_SelfLoop(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n0}] = struct{}{} // Self-loop
	g.SetBeatLength(8)

	beatInfos, isLoop, loopStart := g.CalculateBeatRow()

	if !isLoop {
		t.Fatal("expected self-loop to be detected as loop")
	}
	if loopStart != 0 {
		t.Fatalf("expected loopStart=0 for self-loop, got %d", loopStart)
	}
	if len(beatInfos) != 8 {
		t.Fatalf("expected beat row length 8, got %d", len(beatInfos))
	}
	// Every entry should be n0 (self-loop repeats the single node)
	for i := 0; i < 8; i++ {
		if beatInfos[i].NodeID != n0 {
			t.Fatalf("at index %d: expected NodeID %d, got %d", i, n0, beatInfos[i].NodeID)
		}
		if beatInfos[i].NodeType != NodeTypeRegular {
			t.Fatalf("at index %d: expected NodeTypeRegular, got %d", i, beatInfos[i].NodeType)
		}
	}
}

// TestCalculateBeatRowFrom_UsesTemporaryStart verifies that CalculateBeatRowFrom
// starts traversal at the given node and restores StartNodeID after the call.
func TestCalculateBeatRowFrom_UsesTemporaryStart(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}

	origStart := g.StartNodeID
	row, _, _ := g.CalculateBeatRowFrom(nB)

	// StartNodeID should be restored
	if g.StartNodeID != origStart {
		t.Fatalf("expected StartNodeID to be restored to %d, got %d", origStart, g.StartNodeID)
	}
	// Row should start at nB, not nA
	if len(row) < 2 {
		t.Fatalf("expected at least 2 entries starting from B, got %d", len(row))
	}
	if row[0].NodeID != nB {
		t.Fatalf("expected first entry to be node %d (B), got %d", nB, row[0].NodeID)
	}
	if row[1].NodeID != nC {
		t.Fatalf("expected second entry to be node %d (C), got %d", nC, row[1].NodeID)
	}
}

// TestCalculateBeatRowUnbounded_WithLoop verifies that the unbounded variant
// returns the raw path with loop metadata but without expanding the loop.
// The C→A edge spans from (2,0) to (0,0), so an intermediate at (1,0) is
// synthesized (B is regular there, so it gets InvalidNodeID).
func TestCalculateBeatRowUnbounded_WithLoop(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}
	g.Edges[[2]NodeID{nC, nA}] = struct{}{} // Loop back to A

	g.SetBeatLength(32)

	row, isLoop, loopStart := g.CalculateBeatRowUnbounded()

	if !isLoop {
		t.Fatal("expected loop to be detected")
	}
	if loopStart != 0 {
		t.Fatalf("expected loopStart=0 (full cycle back to A), got %d", loopStart)
	}
	// Raw path: [A, B, C, intermediate_at_(1,0)] — C→A spans 2 cells so
	// an intermediate is generated at B's position (regular → InvalidNodeID).
	if len(row) != 4 {
		t.Fatalf("expected 4 entries (3 nodes + 1 intermediate), got %d", len(row))
	}
	if row[0].NodeID != nA {
		t.Fatalf("entry 0: expected A (%d), got %d", nA, row[0].NodeID)
	}
	if row[1].NodeID != nB {
		t.Fatalf("entry 1: expected B (%d), got %d", nB, row[1].NodeID)
	}
	if row[2].NodeID != nC {
		t.Fatalf("entry 2: expected C (%d), got %d", nC, row[2].NodeID)
	}
	// Entry 3: intermediate between C(2,0) and A(0,0) at (1,0) where B sits
	if row[3].NodeID != InvalidNodeID {
		t.Fatalf("entry 3: expected InvalidNodeID for intermediate, got %d", row[3].NodeID)
	}
	if row[3].NodeType != NodeTypeInvisible {
		t.Fatalf("entry 3: expected NodeTypeInvisible, got %d", row[3].NodeType)
	}
}

// TestIsLoop_WithCycle verifies that IsLoop returns true for a graph with a cycle.
func TestIsLoop_WithCycle(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}
	g.Edges[[2]NodeID{nC, nA}] = struct{}{} // Back edge forming cycle

	if !g.IsLoop() {
		t.Fatal("expected IsLoop to return true for A→B→C→A cycle")
	}
}

// TestIsLoop_LinearChain verifies that IsLoop returns false for a linear chain
// with no back edges.
func TestIsLoop_LinearChain(t *testing.T) {
	g := NewGraph(testLogger)
	nA := g.AddNode(0, 0, NodeTypeRegular)
	nB := g.AddNode(1, 0, NodeTypeRegular)
	nC := g.AddNode(2, 0, NodeTypeRegular)
	g.StartNodeID = nA
	g.Edges[[2]NodeID{nA, nB}] = struct{}{}
	g.Edges[[2]NodeID{nB, nC}] = struct{}{}

	if g.IsLoop() {
		t.Fatal("expected IsLoop to return false for linear chain A→B→C")
	}
}
