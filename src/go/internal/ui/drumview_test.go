package ui

import (
	"image"
	"image/color"
	"io"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestNewDrumView(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	if drumView == nil {
		t.Fatal("NewDrumView returned nil")
	}
	if drumView.Length != 8 {
		t.Errorf("Expected initial drum view length to be 8, got %d", drumView.Length)
	}
	if len(drumView.Rows) != 1 {
		t.Fatalf("Expected 1 drum row, got %d", len(drumView.Rows))
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
	for i, step := range drumView.Rows[0].Steps {
		if step {
			t.Errorf("Expected step %d to be false (empty), got true", i)
		}
	}
}

func TestDrumViewLengthIncrease(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Simulate button press
	drumView.lenIncPressed = true
	drumView.Update()

	if drumView.Length != 9 {
		t.Errorf("Expected drum view length to increase to 9, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 9 {
		t.Errorf("Expected drum row steps length to be 9, got %d", len(drumView.Rows[0].Steps))
	}
}

func TestDrumViewLengthDecrease(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Increase length first to ensure we can decrease
	drumView.lenIncPressed = true
	drumView.Update() // Length is now 9

	// Simulate button press
	drumView.lenDecPressed = true
	drumView.Update()

	if drumView.Length != 8 {
		t.Errorf("Expected drum view length to decrease to 8, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
}

func TestDrumViewWheelAdjustsLength(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)

	wheelVal := 1.0
	cursor := func() (int, int) { return dv.Bounds.Min.X + dv.labelW + 500, dv.Bounds.Min.Y + timelineHeight + 5 }
	restore := SetInputForTest(cursor,
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return 800, 600 },
	)
	dv.Update() // wheel up -> length++
	restore()
	if dv.Length != 9 {
		t.Fatalf("expected length 9 got %d", dv.Length)
	}
}

func TestDrumViewLengthMinMax(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Test min length (should not go below 1)
	drumView.Length = 1
	drumView.lenDecPressed = true
	drumView.Update()
	if drumView.Length != 1 {
		t.Errorf("Expected drum view length to stay at 1, got %d", drumView.Length)
	}

	// Test max length (should not go above 64)
	drumView.Length = 64
	drumView.lenIncPressed = true
	drumView.Update()
	if drumView.Length != 64 {
		t.Errorf("Expected drum view length to stay at 64, got %d", drumView.Length)
	}
}

func TestTimelineInfo(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.bpm = 120
	info := dv.timelineInfo(4)
	expected := "00:02.000/00:04.000 | Beats 1-8/8"
	if info != expected {
		t.Fatalf("expected %q got %q", expected, info)
	}
}

func TestTimelineLayout(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	textY := dv.Bounds.Min.Y + 5
	if textY >= dv.timelineRect.Min.Y {
		t.Fatalf("info text overlaps timeline bar")
	}
	rowStart := dv.Bounds.Min.Y + timelineHeight
	if dv.timelineRect.Max.Y >= rowStart {
		t.Fatalf("timeline bar overlaps drum rows")
	}
}

func TestDrumViewUpdatesGraphBeatLength(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Initial check
	if graph.BeatLength() != 8 {
		t.Errorf("Expected initial graph beat length to be 8, got %d", graph.BeatLength())
	}

	// Increase length and check graph
	drumView.lenIncPressed = true
	drumView.Update()
	if graph.BeatLength() != 9 {
		t.Errorf("Expected graph beat length to be 9 after increase, got %d", graph.BeatLength())
	}

	// Decrease length and check graph
	drumView.lenDecPressed = true
	drumView.Update()
	if graph.BeatLength() != 8 {
		t.Errorf("Expected graph beat length to be 8 after decrease, got %d", graph.BeatLength())
	}
}

func TestDrumViewLooping(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)

	// Create a looping graph: O > X > X > (loop start) X > X > (loop end)
	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node1 := graph.AddNode(1, 0, model.NodeTypeRegular)
	node2 := graph.AddNode(2, 0, model.NodeTypeRegular)
	node3 := graph.AddNode(3, 0, model.NodeTypeRegular)
	node4 := graph.AddNode(4, 0, model.NodeTypeRegular)

	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node2, node3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node3, node4}] = struct{}{}
	graph.Edges[[2]model.NodeID{node4, node2}] = struct{}{}

	drumView := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)
	drumView.Length = 10
	drumView.SetBeatLength(10)

	// Manually call updateBeatInfos to populate the drum view
	game := &Game{graph: graph, drum: drumView, logger: logger}
	game.updateBeatInfos()

	expectedSteps := []bool{true, true, true, true, true, true, true, true, true, true}
	t.Logf("Generated drum row: %v", drumView.Rows[0].Steps)
	if len(drumView.Rows[0].Steps) != len(expectedSteps) {
		t.Fatalf("Expected %d steps, but got %d", len(expectedSteps), len(drumView.Rows[0].Steps))
	}

	for i, step := range drumView.Rows[0].Steps {
		if step != expectedSteps[i] {
			t.Errorf("Step %d: expected %v, got %v", i, expectedSteps[i], step)
		}
	}
}

func TestDrumViewLoopHighlighting(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	logger.SetLevel(game_log.LevelDebug) // Enable debug logging for this test

	graph := model.NewGraph(logger)

	// Circuit: [O] > [] > [X] > [X]
	//                    ^     v
	//                   [X] < [X]
	// This translates to:
	// node0 (0,0) -> node_inv1 (1,0) -> node1 (2,0)
	// node1 (2,0) -> node_inv2 (2,1) -> node2 (2,2)
	// node2 (2,2) -> node_inv3 (1,2) -> node3 (0,2)
	// node3 (0,2) -> node_inv4 (0,1) -> node1 (2,0) (loop back to node1)

	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node_inv1 := graph.AddNode(1, 0, model.NodeTypeInvisible)
	node1 := graph.AddNode(2, 0, model.NodeTypeRegular)
	node_inv2 := graph.AddNode(2, 1, model.NodeTypeInvisible)
	node2 := graph.AddNode(2, 2, model.NodeTypeRegular)
	node_inv3 := graph.AddNode(1, 2, model.NodeTypeInvisible)
	node3 := graph.AddNode(0, 2, model.NodeTypeRegular)
	node_inv4 := graph.AddNode(0, 1, model.NodeTypeInvisible)

	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node_inv1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv1, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node_inv2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv2, node2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node2, node_inv3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv3, node3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node3, node_inv4}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv4, node1}] = struct{}{} // Loop back to node1

	drumView := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)
	drumView.Length = 10 // Set a reasonable length for the drum view
	drumView.SetBeatLength(drumView.Length)

	game := New(logger)
	game.graph = graph
	game.drum = drumView
	game.bpm = 120        // Set a BPM for consistent beat duration
	game.Layout(800, 720) // Set layout to initialize drum view bounds

	// Simulate starting playback
	game.playing = true
	game.updateBeatInfos() // Call updateBeatInfos after drum is set
	game.spawnPulseFrom(0)

	// Run for a few cycles to test loop highlighting
	for i := 0; i < 20; i++ { // Simulate 20 frames
		game.Update()
		t.Logf("Frame %d: highlightedBeats: %v", game.frame, game.highlightedBeats)

		// Determine the expected highlighted index based on the current beat and loop
		expectedHighlightedIndex := -1
		if game.activePulse != nil {
			// The pulse has just arrived at this beat, so it's pathIdx-1
			currentBeatIndex := game.activePulse.pathIdx - 1
			if currentBeatIndex >= 0 && currentBeatIndex < len(game.beatInfos) {
				expectedHighlightedIndex = currentBeatIndex
			}
		} else if game.playing && len(game.beatInfos) > 0 {
			// If no active pulse, but playing, it means the first beat is highlighted
			expectedHighlightedIndex = 0
		}

		// Verify highlighting
		for j := 0; j < drumView.Length; j++ {
			isHighlighted := false
			if _, ok := game.highlightedBeats[j]; ok {
				isHighlighted = true
			}

			if j == expectedHighlightedIndex {
				if !isHighlighted {
					t.Errorf("Frame %d, Beat %d: Expected to be highlighted, but was not.", game.frame, j)
				}
			} else {
				if isHighlighted {
					t.Errorf("Frame %d, Beat %d: Expected NOT to be highlighted, but was.", game.frame, j)
				}
			}
		}

		// Advance time for the next frame
		time.Sleep(time.Millisecond * 16) // Simulate 60 TPS
	}
}

func TestDrumViewButtonsDrawn(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelInfo)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 100), graph, logger)

	count := 0
	orig := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		count++
	}
	defer func() { drawButton = orig }()

	dv.Draw(ebiten.NewImage(400, 100), map[int]int64{}, 0, nil, 0)
	if count != 9 {
		t.Fatalf("expected 9 buttons drawn, got %d", count)
	}
}

func TestDrumViewDrawHighlightsInvisibleCells(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)

	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node1 := graph.AddNode(1, 0, model.NodeTypeInvisible)
	node2 := graph.AddNode(2, 0, model.NodeTypeRegular)
	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node2}] = struct{}{}

	dv := NewDrumView(image.Rect(0, 0, 300, 50), graph, logger)
	dv.Length = 3
	dv.SetBeatLength(3)

	game := &Game{graph: graph, drum: dv, logger: logger}
	game.updateBeatInfos()

	type call struct{ c color.Color }
	calls := []call{}
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, call{c: c})
	}
	defer func() { drawRect = orig }()

	highlighted := map[int]int64{1: 1}
	dv.Draw(ebiten.NewImage(300, 50), highlighted, 0, game.beatInfos, 0)

	var highlightCount int
	for _, call := range calls {
		if clr, ok := call.c.(color.RGBA); ok && clr == colHighlight {
			highlightCount++
		}
	}
	if highlightCount != 1 {
		t.Fatalf("expected 1 highlight draw, got %d", highlightCount)
	}
}
