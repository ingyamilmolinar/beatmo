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
	expected := "00:02.000/00:04.000 | View 00:00.000-00:04.000 | Beats 1-8/8"
	if info != expected {
		t.Fatalf("expected %q got %q", expected, info)
	}
}

func TestTimelineViewRect(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.SetBeatLength(16)
	dv.Offset = 4
	dv.recalcButtons()

	var got image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineView {
				got = r
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(800, 200), nil, 0, nil, 0)

	totalBeats := dv.timelineBeats
	start := dv.timelineRect.Min.X + int(float64(dv.Offset)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
	width := int(float64(dv.Length) / float64(totalBeats) * float64(dv.timelineRect.Dx()))
	want := image.Rect(start, dv.timelineRect.Min.Y, start+width, dv.timelineRect.Max.Y)
	if got != want {
		t.Fatalf("view rect = %v want %v", got, want)
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

func TestTimelineExpandsAndViewShrinks(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	img := ebiten.NewImage(800, 200)

	var rect image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineView {
				rect = r
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(img, nil, 0, nil, 0)
	baseWidth := rect.Dx()

	dv.Draw(img, nil, 0, nil, 20)
	expandedWidth := rect.Dx()

	if dv.timelineBeats != 28 {
		t.Fatalf("timelineBeats = %d want 28", dv.timelineBeats)
	}
	if expandedWidth >= baseWidth {
		t.Fatalf("view width did not shrink: base %d expanded %d", baseWidth, expandedWidth)
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
			if _, ok := game.highlightedBeats[makeBeatKey(0, j)]; ok {
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
	if count != 10 {
		t.Fatalf("expected 10 buttons drawn, got %d", count)
	}
}

func TestDrumViewHighlightsMultipleRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.Length = 4
	dv.SetBeatLength(4)
	dv.AddRow()

	highlights := map[int]int64{
		makeBeatKey(0, 1): 1,
		makeBeatKey(1, 2): 1,
	}

	dst := ebiten.NewImage(800, 200)
	orig := drawRect
	var hits [][2]int
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && r.Min.Y >= dv.Bounds.Min.Y+timelineHeight {
			row := (r.Min.Y - (dv.Bounds.Min.Y + timelineHeight)) / dv.rowHeight()
			col := (r.Min.X - (dv.Bounds.Min.X + dv.labelW + dv.controlsW)) / dv.cell
			if color.RGBAModel.Convert(c).(color.RGBA) == colHighlight {
				hits = append(hits, [2]int{row, col})
			}
		}
		orig(dst, r, c, filled)
	}
	dv.Draw(dst, highlights, 0, make([]model.BeatInfo, dv.Length), 0)
	drawRect = orig

	want := map[[2]int]bool{{0, 1}: true, {1, 2}: true}
	if len(hits) != 2 || !want[hits[0]] || !want[hits[1]] {
		t.Fatalf("unexpected highlight cells %v", hits)
	}
}

func TestDrumViewAddAndDeleteRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.calcLayout()

	pressed := true
	cx, cy := dv.addRowBtn.Min.X+1, dv.addRowBtn.Min.Y+1
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if len(dv.Rows) != 2 {
		t.Fatalf("expected 2 rows got %d", len(dv.Rows))
	}

	// recalc layout to get delete button for new row
	dv.Update()
	pressed = true
	cx, cy = dv.rowDeleteBtns[1].Min.X+1, dv.rowDeleteBtns[1].Min.Y+1
	restore = SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if len(dv.Rows) != 1 {
		t.Fatalf("expected 1 row after deletion got %d", len(dv.Rows))
	}
}

func TestDrumViewChangeInstrumentPerRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.instOptions = []string{"snare", "kick"}
	dv.AddRow()
	dv.Update()
	dv.selRow = 1
	dv.recalcButtons()

	pressed := true
	cx, cy := dv.instBtn.Min.X+1, dv.instBtn.Min.Y+1
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	if dv.Rows[1].Instrument != "kick" {
		t.Fatalf("expected instrument 'kick', got %s", dv.Rows[1].Instrument)
	}
}

func TestDrumViewDeleteRowRecordsOrigin(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	dv.Rows[1].Origin = 42
	dv.DeleteRow(1)
	rows := dv.ConsumeDeletedRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(rows))
	}
	if rows[0].index != 1 || rows[0].origin != 42 {
		t.Fatalf("unexpected deleted row info: %+v", rows[0])
	}
}

func TestDrumViewConsumeAddedRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	rows := dv.ConsumeAddedRows()
	if len(rows) != 1 || rows[0] != 1 {
		t.Fatalf("expected added row index 1, got %v", rows)
	}
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("expected added rows cleared after consume")
	}
}

func TestDrumViewLayoutStacksRows(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, testLogger)
	dv.AddRow()
	dv.calcLayout()
	if len(dv.rowLabelRects) != 2 {
		t.Fatalf("expected 2 row labels, got %d", len(dv.rowLabelRects))
	}
	if dv.rowLabelRects[1].Min.Y <= dv.rowLabelRects[0].Min.Y {
		t.Fatalf("row labels not stacked vertically: %v vs %v", dv.rowLabelRects[0], dv.rowLabelRects[1])
	}
	if dv.addRowBtn.Min.Y <= dv.rowLabelRects[1].Min.Y {
		t.Fatalf("add button not below rows: %v vs %v", dv.addRowBtn, dv.rowLabelRects[1])
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

	type call struct {
		c color.Color
		r image.Rectangle
	}
	calls := []call{}
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, call{c: c, r: r})
	}
	defer func() { drawRect = orig }()

	highlighted := map[int]int64{1: 1}
	dv.Draw(ebiten.NewImage(300, 50), highlighted, 0, game.beatInfos, 0)

	var highlightCount int
	for _, call := range calls {
		if clr, ok := call.c.(color.RGBA); ok && clr == colHighlight && call.r.Min.Y >= timelineHeight {
			highlightCount++
		}
	}
	if highlightCount != 1 {
		t.Fatalf("expected 1 highlight draw, got %d", highlightCount)
	}
}

func TestDrumViewSetBPMClamp(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(maxBPM + 10)
	if dv.bpm != maxBPM {
		t.Fatalf("expected BPM %d, got %d", maxBPM, dv.bpm)
	}
	if dv.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on high bpm")
	}
	dv.bpmErrorAnim = 0
	dv.SetBPM(0)
	if dv.bpm != 1 {
		t.Fatalf("expected BPM 1, got %d", dv.bpm)
	}
	if dv.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on low bpm")
	}
}

func TestDrumViewBPMTextInput(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()

	cx, cy := dv.bpmBox.Min.X+1, dv.bpmBox.Min.Y+1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update() // click to focus
	pressed = false

	chars = []rune{'5'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()

	// click outside to commit
	pressed = true
	cx, cy = 0, 0
	dv.Update()

	if dv.BPM() != 500 {
		t.Fatalf("expected BPM 500 got %d", dv.BPM())
	}
}
