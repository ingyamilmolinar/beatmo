package model

import (
	"os"
	"reflect"
	"testing"

	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(os.Stdout, game_log.LevelDebug)
}

func TestCalculateBeatRow_SimplePath(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 2, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}

	beatInfos, _, _ := g.CalculateBeatRow()

	initialPath := []BeatInfo{
		{NodeID: n0, NodeType: NodeTypeRegular, I: 0, J: 0},
		{NodeID: n1, NodeType: NodeTypeRegular, I: 0, J: 2},
	}

	expected := make([]BeatInfo, g.BeatLength())
	copy(expected, initialPath)
	for i := len(initialPath); i < g.BeatLength(); i++ {
		expected[i] = BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: -1, J: -1}
	}

	if !reflect.DeepEqual(beatInfos, expected) {
		t.Fatalf("Expected beatInfos %v, got %v", expected, beatInfos)
	}
}

func TestCalculateBeatRow_Loop(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 1, NodeTypeRegular)
	n2 := g.AddNode(1, 1, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	g.Edges[[2]NodeID{n2, n0}] = struct{}{}

	// Set a specific beat length for the test
	g.SetBeatLength(6)

	beatInfos, _, _ := g.CalculateBeatRow()

	// Expected single cycle
	singleCycle := []BeatInfo{
		{NodeID: n0, NodeType: NodeTypeRegular, I: 0, J: 0},
		{NodeID: n1, NodeType: NodeTypeRegular, I: 0, J: 1},
		{NodeID: n2, NodeType: NodeTypeRegular, I: 1, J: 1},
	}

	// Expected extended beatInfos (single cycle repeated twice)
	expected := make([]BeatInfo, 0, g.BeatLength())
	for i := 0; i < g.BeatLength()/len(singleCycle); i++ {
		expected = append(expected, singleCycle...)
	}

	if !reflect.DeepEqual(beatInfos, expected) {
		t.Fatalf("Expected beatInfos %v, got %v", expected, beatInfos)
	}
}

func TestCalculateBeatRow_Disconnected(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	_ = g.AddNode(5, 5, NodeTypeRegular) // Disconnected node
	g.StartNodeID = n0

	beatInfos, _, _ := g.CalculateBeatRow()

	initialPath := []BeatInfo{
		{NodeID: n0, NodeType: NodeTypeRegular, I: 0, J: 0},
	}

	expected := make([]BeatInfo, g.BeatLength())
	copy(expected, initialPath)
	for i := len(initialPath); i < g.BeatLength(); i++ {
		expected[i] = BeatInfo{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: -1, J: -1}
	}

	if !reflect.DeepEqual(beatInfos, expected) {
		t.Fatalf("Expected beatInfos %v, got %v", expected, beatInfos)
	}
}

func TestIsLoop(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 1, NodeTypeRegular)
	n2 := g.AddNode(1, 1, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}

	if g.IsLoop() {
		t.Fatal("Expected IsLoop to be false for a non-looping graph")
	}

	g.Edges[[2]NodeID{n2, n0}] = struct{}{}

	if !g.IsLoop() {
		t.Fatal("Expected IsLoop to be true for a looping graph")
	}
}

func TestCalculateBeatRow_ComplexLoopWithInvisibleNodes(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n_inv1 := g.AddNode(1, 0, NodeTypeInvisible)
	n1 := g.AddNode(2, 0, NodeTypeRegular)
	n_inv2 := g.AddNode(2, 1, NodeTypeInvisible)
	n2 := g.AddNode(2, 2, NodeTypeRegular)

	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.Edges[[2]NodeID{n1, n2}] = struct{}{}
	g.Edges[[2]NodeID{n2, n0}] = struct{}{}

	g.SetBeatLength(10) // Set a length that will cause multiple repetitions

	beatInfos, isLoop, _ := g.CalculateBeatRow()

	if !isLoop {
		t.Fatalf("Expected a loop to be detected, but isLoop is false")
	}

	expectedCycle := []BeatInfo{
		{NodeID: n0, NodeType: NodeTypeRegular, I: 0, J: 0},
		{NodeID: n_inv1, NodeType: NodeTypeInvisible, I: 1, J: 0},
		{NodeID: n1, NodeType: NodeTypeRegular, I: 2, J: 0},
		{NodeID: n_inv2, NodeType: NodeTypeInvisible, I: 2, J: 1},
		{NodeID: n2, NodeType: NodeTypeRegular, I: 2, J: 2},
	}

	// The expected beatInfos should be the cycle repeated and then trimmed/padded
	expected := make([]BeatInfo, g.BeatLength())
	for i := 0; i < g.BeatLength(); i++ {
		expected[i] = expectedCycle[i%len(expectedCycle)]
	}

	if !reflect.DeepEqual(beatInfos, expected) {
		t.Fatalf("Expected beatInfos %v, got %v", expected, beatInfos)
	}
}
