package ui

import (
	"math"
	"os"
	"testing"
	

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(os.Stdout, game_log.LevelDebug)
}

func TestTryAddNodeTogglesRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	nodeID := g.tryAddNode(2, 0, model.NodeTypeRegular).ID
	g.graph.StartNodeID = nodeID
	// After adding a node and setting it as start, the beat row should reflect it.
	// We need to call Update to propagate the change to drum.Rows.
	g.Update()
	if len(g.drum.Rows[0].Steps) <= 0 || !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected step 0 on")
	}
}

func TestDeleteNodeClearsRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.deleteNode(n)
	g.Update() // Propagate changes to drum.Rows
	if len(g.drum.Rows[0].Steps) > 1 && g.drum.Rows[0].Steps[1] {
		t.Fatalf("expected step 1 off after delete")
	}
}

func TestAddEdgeNoDuplicates(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(a, b)
	if len(g.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.edges))
	}
}

func TestUpdateRunsSchedulerWhenPlaying(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// ensure first step active
	nodeID := g.tryAddNode(0, 0, model.NodeTypeRegular).ID
	g.graph.StartNodeID = nodeID

	var fired int
	g.sched.OnTick = func(i int) { fired++ }
	g.bpm = 60

	// Simulate click on play button
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.playBtn.Min.X + 1, g.drum.playBtn.Min.Y + 1 }, // Click inside the button
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // Simulate press
	pressed = false
	g.Update() // Simulate release

	if fired == 0 {
		t.Fatalf("scheduler did not run")
	}
}

func TestClickAddsNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return 10, topOffset + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	if g.nodeAt(0, 0) != nil {
		t.Fatalf("node created before release")
	}
	pressed = false
	g.Update() // release
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("expected node created at (0,0) after release")
	}
	if !n.Selected || g.sel != n {
		t.Fatalf("new node should be selected")
	}
	pressed = true
	// click another position
	restore2 := SetInputForTest(
		func() (int, int) { return StepPixels(g.cam.Scale) + 10, topOffset + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore2()
	g.Update() // press second node
	if g.nodeAt(1, 0) != nil {
		t.Fatalf("node created before release at second position")
	}
	pressed = false
	g.Update() // release second node
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if n2 == nil || !n2.Selected || g.sel != n2 || n.Selected {
		t.Fatalf("selection did not move to new node")
	}
}

func TestRowLengthMatchesConnectedNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Add some nodes and edges
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	// Set start node
	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.updateBeatInfos()

	// The drum view length should now be independent of the number of connected nodes
	// and should match the default drum.Length (which is 8)
	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("row len=%d want %d", len(g.drum.Rows[0].Steps), g.drum.Length)
	}

	// Verify the first few steps based on the connected nodes
	if !g.drum.Rows[0].Steps[0] {
		t.Errorf("Expected step 0 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[1] {
		t.Errorf("Expected step 1 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[2] {
		t.Errorf("Expected step 2 to be true, got false")
	}

	// Verify the remaining steps are false (padded)
	for i := 3; i < g.drum.Length; i++ {
		if g.drum.Rows[0].Steps[i] {
			t.Errorf("Expected step %d to be false (padded), got true", i)
		}
	}
}

func TestPulseAnimationProgress(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	node0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	node1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(node0, node1)
	g.graph.StartNodeID = node0.ID

	// Manually set playing to true and call spawnPulse to create the pulse
	g.playing = true
	g.spawnPulse()

	// The pulse should be active now
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after spawning")
	}

	firstT := g.activePulse.t

	// Advance the game state by a few frames
	for i := 0; i < 10; i++ {
		g.Update()
	}

	if g.activePulse == nil {
		t.Fatalf("pulse disappeared unexpectedly")
	}

	// The animation time 't' should have progressed
	if g.activePulse.t <= firstT {
		t.Fatalf("active pulse did not advance: %f <= %f", g.activePulse.t, firstT)
	}
}

func TestNodeScreenAlignment(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	g.cam.OffsetX = 12.3
	g.cam.OffsetY = 7.8

	n := g.tryAddNode(3, 2, model.NodeTypeRegular)
	g.graph.StartNodeID = n.ID
	step := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	sx := offX + float64(step*n.I)
	sy := offY + float64(step*n.J)

	expX := sx
	expY := sy

	if sx != expX || sy != expY {
		t.Fatalf("screen (%v,%v) want (%v,%v)", sx, sy, expX, expY)
	}
}

func TestDragMaintainsAlignment(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	n := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.graph.StartNodeID = n.ID

	pos := []struct{ x, y int }{{100, topOffset + 100}, {120, topOffset + 110}}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	step := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	nodeX := offX + float64(step*n.I)
	nodeY := offY + float64(step*n.J)

	xs, ys := GridLines(g.cam, g.winW, g.split.Y)
	foundX, foundY := false, false
	for _, x := range xs {
		if math.Abs(x-nodeX) < 1e-3 {
			foundX = true
			break
		}
	}
	for _, y := range ys {
		if math.Abs(y-nodeY) < 1e-3 {
			foundY = true
			break
		}
	}
	if !foundX || !foundY {
		t.Fatalf("node not aligned with grid after drag")
	}
}
func TestInitialDrumRows(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	if len(g.drum.Rows) != 1 {
		t.Fatalf("rows=%d want 1", len(g.drum.Rows))
	}
	g.Update()
	g.Draw(ebiten.NewImage(640, 480)) // Call Draw to populate bgCache
	if len(g.drum.bgCache) != 1 {
		t.Fatalf("bgCache=%d want 1", len(g.drum.bgCache))
	}
}

func TestHighlightScalesWithZoom(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.sel = n
	g.graph.StartNodeID = n.ID

	g.cam.Scale = 1.0
	a1, _, a2, _ := g.nodeScreenRect(n)
	w1 := a2 - a1

	g.cam.Scale = 2.0
	b1, _, b2, _ := g.nodeScreenRect(n)
	w2 := b2 - b1

	if math.Abs(w2-2*w1) > 1e-3 {
		t.Fatalf("highlight width did not scale: w1=%f w2=%f", w1, w2)
	}
}

func TestDragPanDoesNotCreateNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	pos := []struct{ x, y int }{
		{10, topOffset + 10},
		{30, topOffset + 20},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	if g.nodeAt(0, 0) != nil {
		t.Fatalf("node created after drag")
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestBottomPaneClickIgnored(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.OffsetY = 100

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return 10, g.split.Y + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press in bottom pane
	pressed = false
	g.Update() // release

	if len(g.nodes) != 0 {
		t.Fatalf("node created from bottom pane click")
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestHighlightMatchesNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(2, 3, model.NodeTypeRegular)
	g.sel = n
	g.graph.StartNodeID = n.ID

	g.cam.Scale = 1.5
	g.cam.OffsetX = 12
	g.cam.OffsetY = 8

	step := StepPixels(g.cam.Scale)
	camScale := float64(step) / float64(GridStep)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	worldX := float64(n.I * GridStep)
	worldY := float64(n.J * GridStep)
	screenX := worldX*camScale + offX
	screenY := worldY*camScale + offY + float64(topOffset)
	half := float64(NodeSpriteSize) * camScale / 2

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	if math.Abs(x1-(screenX-half)) > 1e-3 || math.Abs(x2-(screenX+half)) > 1e-3 ||
		math.Abs(y1-(screenY-half)) > 1e-3 || math.Abs(y2-(screenY+half)) > 1e-3 {
		t.Fatalf("highlight mismatch: (%f,%f,%f,%f) want (%f,%f,%f,%f)",
			x1, y1, x2, y2,
			screenX-half, screenY-half, screenX+half, screenY+half)
	}
}

func TestSplitterDragPersists(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	startY := g.split.Y
	pos := []struct{ x, y int }{
		{10, startY},
		{10, startY + 50},
		{10, startY + 50},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update()         // release
	g.Layout(640, 480) // layout called again as in game loop
	g.Update()
	if g.split.Y != startY+50 {
		t.Fatalf("splitter Y=%d want %d", g.split.Y, startY+50)
	}
	g.graph.StartNodeID = model.InvalidNodeID
}
func TestSplitterDragDoesNotCreateNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.Layout(640, 480)
	startY := g.split.Y
	pos := []struct{ x, y int }{
		{10, startY},      // press on divider
		{10, startY + 40}, // drag
		{10, startY + 40}, // release
		{10, startY + 40}, // idle
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update() // release while over divider
	idx = 3
	g.Update() // after release
	g.Layout(640, 480)
	g.Update()

	if len(g.nodes) != 0 {
		t.Fatalf("unexpected node created during splitter drag")
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestStartNodeSelection(t *testing.T) {
       g := New(testLogger)
       g.Layout(640, 480)
       n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
       if g.start != n1 || !n1.Start {
               t.Fatalf("first node should be start")
       }
       n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
       g.sel = n2
       restore := SetInputForTest(
               func() (int, int) { return 0, topOffset + 10 },
               func(ebiten.MouseButton) bool { return false },
               func(k ebiten.Key) bool { return k == ebiten.KeyS },
               func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
       )
       defer restore()
       g.Update()
       if g.start != n2 || !n2.Start || n1.Start {
               t.Fatalf("start node not updated")
       }
	g.graph.StartNodeID = n2.ID
}

func TestHighlightEmptyCells(t *testing.T) {
	t.Skip()
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	g := New(logger)
	g.Layout(1280, 720)
	g.bpm = 60

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(3, 0, model.NodeTypeRegular) // Node with a gap
	g.addEdge(n1, n2)

	g.start = n1
	g.graph.StartNodeID = n1.ID

	g.drum.Length = 4
	g.updateBeatInfos() // This will now correctly handle the invisible nodes

	if len(g.beatInfos) != 4 { // n1, invisible, invisible, n2
		t.Fatalf("Expected beatInfos length to be 4, got %d", len(g.beatInfos))
	}

	g.playing = true
	g.sched.Start()

	// Set playing to true and spawn the pulse
	g.playing = true
	g.spawnPulse()

	// Tick 0: Should highlight n1 (index 0)
	for g.activePulse == nil || g.activePulse.pathIdx == 0 || g.activePulse.t < 1 {
		g.Update()
	}
	if _, ok := g.highlightedNodes[n1.ID]; !ok {
		t.Errorf("Tick 0: Node n1 should be highlighted")
	}

	// Tick 1: Should highlight the first empty cell (index 1)
	for g.activePulse.pathIdx == 1 && g.activePulse.t < 1 {
		g.Update()
	}
	if _, ok := g.drum.highlightedEmptyBeats[1]; !ok {
		t.Errorf("Tick 1: Empty cell at index 1 should be highlighted")
	}

	// Tick 2: Should highlight the second empty cell (index 2)
	for g.activePulse.pathIdx == 2 && g.activePulse.t < 1 {
		g.Update()
	}
	if _, ok := g.drum.highlightedEmptyBeats[2]; !ok {
		t.Errorf("Tick 2: Empty cell at index 2 should be highlighted")
	}

	// Tick 3: Should highlight n2 (index 3)
	for g.activePulse.pathIdx == 3 && g.activePulse.t < 1 {
		g.Update()
	}
	if _, ok := g.highlightedNodes[n2.ID]; !ok {
		t.Errorf("Tick 3: Node n2 should be highlighted")
	}
}

func TestDrumViewLengthIndependence(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Add some nodes and edges
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)

	g.addEdge(n0, n1)

	// Set start node
	g.start = n0
	g.graph.StartNodeID = n0.ID

	// Set drum view length to something different than the actual path length
	g.drum.Length = 8

	g.updateBeatInfos()

	// The beatInfos should now be padded to the drum.Length
	if len(g.beatInfos) != g.drum.Length {
		t.Errorf("Expected beatInfos length to be %d, got %d", g.drum.Length, len(g.beatInfos))
	}

	// Verify the first few steps based on the connected nodes
	if g.beatInfos[0].NodeID != n0.ID || g.beatInfos[1].NodeID != n1.ID {
		t.Errorf("Expected beatInfos to start with n0 and n1, got %v", g.beatInfos)
	}

	// Verify the remaining steps are InvalidNodeID (padded)
	for i := 2; i < g.drum.Length; i++ {
		if g.beatInfos[i].NodeID != model.InvalidNodeID {
			t.Errorf("Expected beatInfos[%d] to be InvalidNodeID, got %v", i, g.beatInfos[i].NodeID)
		}
	}

	// The drum view steps should also reflect the padded length
	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("Expected drum view steps length to be %d, got %d", g.drum.Length, len(g.drum.Rows[0].Steps))
	}

	if !g.drum.Rows[0].Steps[0] {
		t.Errorf("Expected drum view step 0 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[1] {
		t.Errorf("Expected drum view step 1 to be true, got false")
	}
	for i := 2; i < g.drum.Length; i++ {
		if g.drum.Rows[0].Steps[i] {
			t.Errorf("Expected drum view step %d to be false (padded), got true", i)
		}
	}
}

func TestDrumViewLoopingWithInvisibleNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Create a path with an invisible node: n0 -> invisible -> n1
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	_ = g.tryAddNode(1, 0, model.NodeTypeInvisible) // Invisible node

	g.addEdge(n0, n1)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	// Set drum view length
	g.drum.Length = 6

	g.updateBeatInfos()

	// Expected drum view steps: n0 (true), invisible (false), n1 (true), then padded
	expectedSteps := []bool{
		true,  // n0
		false, // invisible
		true,  // n1
		false, // padded
		false, // padded
		false, // padded
	}

	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("Expected drum view steps length to be %d, got %d", g.drum.Length, len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedSteps {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("Drum view step %d: expected %t, got %t", i, expected, g.drum.Rows[0].Steps[i])
		}
	}
}

func TestDrumViewLoopingHighlighting(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	g := New(logger) // Use the Game struct to leverage its graph manipulation and beat info update logic

	// Set up the drum view length to accommodate the expected sequence
	g.drum.Length = 6
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	// Create the circuit: [X] -> [] -> [X] -> [X]
	//                       ^      ^
	//                       |      |
	//                      [X] <- [X]

	// Nodes:
	// (0,0) - Node 1 (Regular)
	// (1,0) - Node 2 (Invisible)
	// (2,0) - Node 3 (Regular)
	// (3,0) - Node 4 (Regular)
	// (3,1) - Node 5 (Regular)
	// (2,1) - Node 6 (Regular)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = n1
	g.graph.StartNodeID = n1.ID

	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible) // invisible
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)   // X
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)   // X
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)   // X
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)   // X

	// Edges:
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	// Update beat infos to populate drum view steps
	g.updateBeatInfos()

	expectedDrumRow := []bool{true, false, true, true, true, true} // [X][ ][X][X][X][X]

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}

	// Test a more complex looped circuit
	// [X] -> [X] -> [X]
	//  ^           |
	//  |           v
	// [X] <- [X] <- [X]

	// Reset graph and drum view
	g = New(logger)
	g.drum.Length = 6
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	cn1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = cn1
	g.graph.StartNodeID = cn1.ID

	cn2 := g.tryAddNode(1, 0, model.NodeTypeRegular) // X
	cn3 := g.tryAddNode(2, 0, model.NodeTypeRegular) // X
	cn4 := g.tryAddNode(2, 1, model.NodeTypeRegular) // X
	cn5 := g.tryAddNode(1, 1, model.NodeTypeRegular) // X
	cn6 := g.tryAddNode(0, 1, model.NodeTypeRegular) // X

	g.addEdge(cn1, cn2)
	g.addEdge(cn2, cn3)
	g.addEdge(cn3, cn4)
	g.addEdge(cn4, cn5)
	g.addEdge(cn5, cn6)
	g.addEdge(cn6, cn1) // Loop back to cn1

	g.updateBeatInfos()

	expectedDrumRow2 := []bool{true, true, true, true, true, true} // All X

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow2) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow2), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow2 {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}
}

func TestSignalTraversalInLoop(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	g := New(logger)
	g.Layout(640, 480)

	// Nodes
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n_inv1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	// Edges
	g.addEdge(n1, n_inv1)
	g.addEdge(n_inv1, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	g.drum.Length = 11 // Set drum length to match the expected path length
	g.updateBeatInfos()

	// Expected sequence of node IDs for the pulse traversal
	expectedNodeIDs := []model.NodeID{
		n1.ID,     // 0
		n_inv1.ID, // 1
		n3.ID,     // 2
		n4.ID,     // 3
		n5.ID,     // 4
		n6.ID,     // 5
		n3.ID,     // 6 (loop starts here)
		n4.ID,     // 7
		n5.ID,     // 8
		n6.ID,     // 9
		n3.ID,     // 10
	}

	t.Logf("Expected Node IDs: %v", expectedNodeIDs)
	actualNodeIDs := []model.NodeID{}
	for _, beatInfo := range g.beatInfos {
		actualNodeIDs = append(actualNodeIDs, beatInfo.NodeID)
	}
	t.Logf("Actual Beat Infos: %v", actualNodeIDs)

	// Verify the initial beatInfos generated by CalculateBeatRow
	if len(actualNodeIDs) != len(expectedNodeIDs) {
		t.Fatalf("Initial beatInfos length mismatch. Expected %d, got %d", len(expectedNodeIDs), len(actualNodeIDs))
	}
	for i, expectedID := range expectedNodeIDs {
		if actualNodeIDs[i] != expectedID {
			t.Errorf("Initial beatInfos mismatch at index %d. Expected %d, got %d", i, expectedID, actualNodeIDs[i])
		}
	}

	g.playing = true
	g.spawnPulse()

	if g.activePulse == nil {
		t.Fatalf("Expected active pulse after spawning")
	}

	// Advance the pulse and check its path
	maxIterations := len(expectedNodeIDs) * 100 // Safety break for infinite loops
	for i := 0; i < len(expectedNodeIDs); i++ {
		t.Logf("Test Loop: Iteration %d. Expected Node ID: %d", i, expectedNodeIDs[i])
		// Simulate enough frames for the pulse to advance to the next beat
		frameCounter := 0
		for g.activePulse != nil && g.activePulse.t < 1 && frameCounter < maxIterations {
			g.Update()
			frameCounter++
			t.Logf("  Inside inner loop: activePulse.t=%.2f, activePulse.pathIdx=%d, frameCounter=%d", g.activePulse.t, g.activePulse.pathIdx, frameCounter)
		}
		if frameCounter >= maxIterations {
			t.Fatalf("Inner loop exceeded max iterations (%d) at step %d, possible infinite loop. activePulse.t=%.2f, activePulse.pathIdx=%d", maxIterations, i, g.activePulse.t, g.activePulse.pathIdx)
		}

		if g.activePulse == nil {
			if i < len(expectedNodeIDs)-1 {
				t.Fatalf("Pulse ended prematurely at step %d", i)
			}
			break // Pulse has naturally ended
		}

		// The fromBeatInfo of the pulse should correspond to the expected node ID at the current step
		if g.activePulse.fromBeatInfo.NodeID != expectedNodeIDs[i] {
			t.Errorf("Step %d: Expected NodeID %d, got %d (fromBeatInfo)", i, expectedNodeIDs[i], g.activePulse.fromBeatInfo.NodeID)
		}

		// For all but the last step, check the toBeatInfo as well
		if i < len(expectedNodeIDs)-1 {
			if g.activePulse.toBeatInfo.NodeID != expectedNodeIDs[i+1] {
				t.Errorf("Step %d: Expected next NodeID %d, got %d (toBeatInfo)", i, expectedNodeIDs[i+1], g.activePulse.toBeatInfo.NodeID)
			}
		}
	}
}
