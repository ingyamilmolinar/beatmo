package model

import (
	"os"
	"testing"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(os.Stdout, game_log.LevelDebug)
}

func TestAddNodeTogglesRow(t *testing.T) {
	g := NewGraph(testLogger)
	nodeID := g.AddNode(2, 0)
	g.StartNodeID = nodeID
	row, _ := g.CalculateBeatRow()
	if len(row) <= 0 || !row[0] {
		t.Fatalf("expected step 0 on after adding node at (2,0) and setting as start node, got %v", row)
	}
}

func TestDeleteNodeClearsRow(t *testing.T) {
	g := NewGraph(testLogger)
	nodeID := g.AddNode(1, 0)
	g.StartNodeID = nodeID
	row, _ := g.CalculateBeatRow()
	if len(row) <= 0 || !row[0] {
		t.Fatalf("expected step 0 on before delete, got %v", row)
	}
	g.RemoveNode(nodeID)
	row, _ = g.CalculateBeatRow()
	if len(row) > 0 && row[0] {
		t.Fatalf("expected step 0 off after delete, got %v", row)
	}
}

func TestCalculateBeatRow(t *testing.T) {
	var n0, n1, n2, n3 NodeID
	g := NewGraph(testLogger)

	// Test with no nodes
	row, _ := g.CalculateBeatRow()
	if len(row) != g.BeatLength() || row[0] || row[1] || row[2] || row[3] {
		t.Fatalf("expected empty row of length %d, got %v", g.BeatLength(), row)
	}

	// Test with a single node at (0,0) as start node
	n1 = g.AddNode(0, 0)
	g.StartNodeID = n1
	row, _ = g.CalculateBeatRow()
	if len(row) != g.BeatLength() || !row[0] || row[1] || row[2] || row[3] {
		t.Fatalf("expected [T F F F], got %v", row)
	}

	// Test with a connected graph, start node at (1,0)
	g = NewGraph(testLogger) // Reset graph for new test case
	n0 = g.AddNode(1, 0)
	n1 = g.AddNode(2, 0)
	n2 = g.AddNode(3, 0)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	if len(row) != g.BeatLength() || !row[0] || !row[1] || !row[2] || row[3] || row[4] || row[5] || row[6] || row[7] || row[8] || row[9] || row[10] || row[11] || row[12] || row[13] || row[14] || row[15] {
		t.Fatalf("expected [T T T F F F F F F F F F F F F F] for start node at (1,0), got %v", row)
	}

	// Test with a connected graph, start node at (0,0)
	g = NewGraph(testLogger) // Reset graph for new test case
	n0 = g.AddNode(0, 0)
	n1 = g.AddNode(1, 0)
	n2 = g.AddNode(2, 0)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	if len(row) != g.BeatLength() || !row[0] || !row[1] || !row[2] || row[3] {
		t.Fatalf("expected [T T T F], got %v", row)
	}

	// Test with a connected graph, start node at (1,0)
	g = NewGraph(testLogger) // Reset graph for new test case
	n0 = g.AddNode(1, 0)
	n1 = g.AddNode(2, 0)
	n2 = g.AddNode(3, 0)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	if len(row) != g.BeatLength() || !row[0] || !row[1] || !row[2] || row[3] || row[4] || row[5] || row[6] || row[7] || row[8] || row[9] || row[10] || row[11] || row[12] || row[13] || row[14] || row[15] {
		t.Fatalf("expected [T T T F F F F F F F F F F F F F] for start node at (1,0), got %v", row)
	}

	// Test with a disconnected node
	// Test with a disconnected node
	g = NewGraph(testLogger) // Reset graph for new test case
	n0 = g.AddNode(0, 0)
	_ = g.AddNode(5, 0) // Disconnected node
	g.StartNodeID = n0
	row, _ = g.CalculateBeatRow()
	if len(row) != g.BeatLength() || !row[0] || row[1] || row[2] || row[3] {
		t.Fatalf("expected [T F F F] with disconnected node, got %v", row)
	}

	// Test with a longer beat length
	g = NewGraph(testLogger) // Reset graph for new test case
	g.beatLengthValue = 8
	n0 = g.AddNode(0, 0)
	n1 = g.AddNode(1, 0)
	n2 = g.AddNode(2, 0)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	if len(row) != 8 || !row[0] || !row[1] || !row[2] || row[3] || row[4] || row[5] || row[6] || row[7] {
		t.Fatalf("expected [T T T F F F F F] for longer beat length, got %v", row)
	}

	// Test with nodes at different grid distances
	g = NewGraph(testLogger) // Reset graph for new test case
	g.beatLengthValue = 8
	n0 = g.AddNode(0, 0) // Start
	n1 = g.AddNode(0, 2) // 2 beats away
	n2 = g.AddNode(1, 2) // 1 beat away from n1 (total 3 from n0)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	// Expected: T (n0 at 0), F (1), T (n1 at 2), T (n2 at 3)
	if len(row) != 8 || !row[0] || row[1] || !row[2] || !row[3] || row[4] || row[5] || row[6] || row[7] {
		t.Fatalf("expected [T F T T F F F F] for varied distances, got %v", row)
	}

	// Test with a more complex path
	g = NewGraph(testLogger) // Reset graph for new test case
	g.beatLengthValue = 10
	n0 = g.AddNode(0, 0) // Start
	n1 = g.AddNode(2, 0) // 2 beats
	n2 = g.AddNode(2, 2) // 2 beats from n1 (total 4)
	n3 = g.AddNode(0, 2) // 2 beats from n2 (total 6)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	g.Edges[[2]NodeID{n2, n3}] = struct{}{}
	row, _ = g.CalculateBeatRow()
	// Expected: T (n0 at 0), F, T (n1 at 2), F, T (n2 at 4), F, T (n3 at 6), F, F, F
	if len(row) != 10 || !row[0] || row[1] || !row[2] || row[3] || !row[4] || row[5] || !row[6] || row[7] || row[8] || row[9] {
		t.Fatalf("expected [T F T F T F T F F F] for complex path, got %v", row)
	}
}
