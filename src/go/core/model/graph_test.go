package model

import (
	"os"
	"reflect"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
		{NodeID: InvalidNodeID, NodeType: NodeTypeInvisible, I: 0, J: 1},
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

func TestCalculateBeatRow_NoStartReturnsInvisible(t *testing.T) {
	g := NewGraph(testLogger)
	beatInfos, isLoop, loopStart := g.CalculateBeatRow()
	if isLoop || loopStart != -1 {
		t.Fatalf("expected no loop; isLoop=%v loopStart=%d", isLoop, loopStart)
	}
	if len(beatInfos) != g.BeatLength() {
		t.Fatalf("unexpected beat row length: %d", len(beatInfos))
	}
	for i, bi := range beatInfos {
		if bi.NodeID != InvalidNodeID || bi.NodeType != NodeTypeInvisible || bi.I != -1 || bi.J != -1 {
			t.Fatalf("expected invisible entry at %d, got %+v", i, bi)
		}
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

func TestCalculateBeatRowFromDoesNotMutateStart(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 1, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n1, n0}] = struct{}{}

	row, _, _ := g.CalculateBeatRowFrom(n1)
	if g.StartNodeID != n0 {
		t.Fatalf("expected StartNodeID preserved, got %d", g.StartNodeID)
	}
	if len(row) == 0 || row[0].NodeID != n1 {
		t.Fatalf("expected row to start at node %d, got %+v", n1, row)
	}
}

func TestCalculateBeatRowUnboundedNoPadding(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 1, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	g.SetBeatLength(16)

	row, _, _ := g.CalculateBeatRowUnbounded()
	if len(row) != 2 {
		t.Fatalf("expected unbounded length 2, got %d", len(row))
	}
}

func TestIntermediateRegularNodesAreInvisible(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	nMid := g.AddNode(0, 1, NodeTypeRegular)
	n1 := g.AddNode(0, 2, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}

	beatInfos, _, _ := g.CalculateBeatRow()
	if len(beatInfos) < 3 {
		t.Fatalf("unexpected beat row length: %d", len(beatInfos))
	}
	mid := beatInfos[1]
	if mid.NodeID != InvalidNodeID || mid.NodeType != NodeTypeInvisible || mid.I != 0 || mid.J != 1 {
		t.Fatalf("expected intermediate regular node to be invisible placeholder, got %+v (node=%d)", mid, nMid)
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

func TestAddNodeDefaultsAndRemoveClearsEdges(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(0, 1, NodeTypeRegular)
	g.Edges[[2]NodeID{n0, n1}] = struct{}{}
	node, ok := g.GetNodeByID(n0)
	if !ok {
		t.Fatalf("expected node present")
	}
	if node.Params.Volume != 1 || node.Params.Duration != 1 {
		t.Fatalf("expected default params volume/duration=1, got %+v", node.Params)
	}
	g.RemoveNode(n0)
	if _, ok := g.Edges[[2]NodeID{n0, n1}]; ok {
		t.Fatalf("expected edge removed after node deletion")
	}
}

func TestSetNodeParamsNormalizesLogic(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(1, 0, NodeTypeRegular)

	// SkipEveryN with no explicit logic kind should normalize to skip_every_n.
	g.SetNodeParams(n0, NodeParams{SkipEveryN: 3})
	n, _ := g.GetNodeByID(n0)
	if n.Params.LogicKind != "skip_every_n" {
		t.Fatalf("expected skip_every_n, got %q", n.Params.LogicKind)
	}
	if n.Params.LogicN != 3 {
		t.Fatalf("expected logic N=3 from skipEveryN, got %d", n.Params.LogicN)
	}
	if n.Params.SkipEveryN != 0 {
		t.Fatalf("expected SkipEveryN cleared, got %d", n.Params.SkipEveryN)
	}

	// Aliases should normalize without applying SkipEveryN.
	g.SetNodeParams(n1, NodeParams{LogicKind: " Prev_Fired ", SkipEveryN: 2})
	n, _ = g.GetNodeByID(n1)
	if n.Params.LogicKind != "trigger_if_prev_triggered" {
		t.Fatalf("expected normalized logic kind, got %q", n.Params.LogicKind)
	}
	if n.Params.LogicN != 0 {
		t.Fatalf("expected logic N unchanged, got %d", n.Params.LogicN)
	}
	if n.Params.SkipEveryN != 0 {
		t.Fatalf("expected SkipEveryN cleared, got %d", n.Params.SkipEveryN)
	}
}

func TestNodeChangedHookFiresOnMutations(t *testing.T) {
	g := NewGraph(testLogger)
	var hits []NodeID
	g.SetNodeChangedHook(func(id NodeID) { hits = append(hits, id) })
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	g.SetNodeParams(n0, NodeParams{Volume: 0.5})
	g.SetNodeLogic(n0, func(NodeContext) NodeDecision { return NodeDecision{} })
	g.RemoveNode(n0)

	if len(hits) < 4 {
		t.Fatalf("expected hook on add/params/logic/remove, got %v", hits)
	}
	if hits[0] != n0 {
		t.Fatalf("expected hook to receive node id %d, got %v", n0, hits)
	}
}

func TestMoveNode(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	n1 := g.AddNode(4, 0, NodeTypeRegular)

	// Set some params to verify they're preserved
	g.SetNodeParams(n0, NodeParams{Volume: 0.7, Pitch: 0.3, Duration: 0.5})

	// Move node to new position
	ok := g.MoveNode(n0, 3, 5)
	if !ok {
		t.Fatal("MoveNode returned false for valid node")
	}

	// Verify position changed
	moved, exists := g.GetNodeByID(n0)
	if !exists {
		t.Fatal("node not found after move")
	}
	if moved.I != 3 || moved.J != 5 {
		t.Fatalf("expected (3,5), got (%d,%d)", moved.I, moved.J)
	}

	// Verify params preserved
	if moved.Params.Volume != 0.7 || moved.Params.Pitch != 0.3 || moved.Params.Duration != 0.5 {
		t.Fatalf("params not preserved: %+v", moved.Params)
	}

	// Verify other node unaffected
	other, exists := g.GetNodeByID(n1)
	if !exists {
		t.Fatal("other node missing")
	}
	if other.I != 4 || other.J != 0 {
		t.Fatalf("other node moved: (%d,%d)", other.I, other.J)
	}

	// Move nonexistent node should return false
	if g.MoveNode(999, 1, 1) {
		t.Fatal("MoveNode should return false for nonexistent node")
	}
}

func TestSetNodeParamsCopiesSynthAndOverrides(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)

	overrides := []EffectOverride{
		{Type: "distortion", Param: "drive", Value: 0.8},
		{Type: "delay", Param: "time", Value: 0.5},
	}
	p := NodeParams{
		Volume:          0.7,
		Pitch:           2,
		Duration:        1.5,
		SynthDecay:      0.5,
		SynthTone:       -0.3,
		SynthAttack:     0.1,
		SynthDrive:      0.6,
		SynthBody:       0.4,
		SynthColor:      -0.5,
		SynthBrightness: 0.9,
		EffectOverrides: overrides,
	}
	g.SetNodeParams(n0, p)

	n, ok := g.GetNodeByID(n0)
	if !ok {
		t.Fatal("node not found")
	}
	if n.Params.SynthDecay != 0.5 {
		t.Errorf("SynthDecay: got %f want 0.5", n.Params.SynthDecay)
	}
	if n.Params.SynthTone != -0.3 {
		t.Errorf("SynthTone: got %f want -0.3", n.Params.SynthTone)
	}
	if n.Params.SynthAttack != 0.1 {
		t.Errorf("SynthAttack: got %f want 0.1", n.Params.SynthAttack)
	}
	if n.Params.SynthDrive != 0.6 {
		t.Errorf("SynthDrive: got %f want 0.6", n.Params.SynthDrive)
	}
	if n.Params.SynthBody != 0.4 {
		t.Errorf("SynthBody: got %f want 0.4", n.Params.SynthBody)
	}
	if n.Params.SynthColor != -0.5 {
		t.Errorf("SynthColor: got %f want -0.5", n.Params.SynthColor)
	}
	if n.Params.SynthBrightness != 0.9 {
		t.Errorf("SynthBrightness: got %f want 0.9", n.Params.SynthBrightness)
	}
	if len(n.Params.EffectOverrides) != 2 {
		t.Fatalf("EffectOverrides: got %d want 2", len(n.Params.EffectOverrides))
	}
	if n.Params.EffectOverrides[0].Type != "distortion" || n.Params.EffectOverrides[0].Value != 0.8 {
		t.Errorf("EffectOverrides[0]: %+v", n.Params.EffectOverrides[0])
	}
	if n.Params.EffectOverrides[1].Type != "delay" || n.Params.EffectOverrides[1].Value != 0.5 {
		t.Errorf("EffectOverrides[1]: %+v", n.Params.EffectOverrides[1])
	}
}

func TestCalculateBeatRow_SelfLoopDoesNotHang(t *testing.T) {
	g := NewGraph(testLogger)
	n0 := g.AddNode(0, 0, NodeTypeRegular)
	g.StartNodeID = n0
	g.Edges[[2]NodeID{n0, n0}] = struct{}{}

	g.SetBeatLength(8)

	beatInfos, isLoop, loopStart := g.CalculateBeatRow()
	if !isLoop || loopStart != 0 {
		t.Fatalf("expected self-loop detected; isLoop=%v loopStart=%d", isLoop, loopStart)
	}
	if len(beatInfos) != g.BeatLength() {
		t.Fatalf("unexpected beat row length: got=%d want=%d", len(beatInfos), g.BeatLength())
	}
	for i := 0; i < g.BeatLength(); i++ {
		if beatInfos[i].NodeID != n0 || beatInfos[i].NodeType != NodeTypeRegular || beatInfos[i].I != 0 || beatInfos[i].J != 0 {
			t.Fatalf("unexpected beat info at %d: %+v", i, beatInfos[i])
		}
	}

	raw, rawLoop, rawStart := g.CalculateBeatRowUnbounded()
	if !rawLoop || rawStart != 0 {
		t.Fatalf("expected self-loop detected in unbounded; loop=%v start=%d", rawLoop, rawStart)
	}
	if len(raw) != 1 {
		t.Fatalf("unexpected unbounded length: got=%d want=1", len(raw))
	}
	if raw[0].NodeID != n0 || raw[0].NodeType != NodeTypeRegular || raw[0].I != 0 || raw[0].J != 0 {
		t.Fatalf("unexpected unbounded beat info: %+v", raw[0])
	}
}
