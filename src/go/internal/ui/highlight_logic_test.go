package ui

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/gamestate"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func stubGameForHighlight(nodeType model.NodeType, triggered bool) *Game {
	logger := game_log.New(io.Discard, game_log.LevelError)
	const size = 64
	g := &Game{
		state:            gamestate.New(120),
		highlightedBeats: make(map[int]int64),
		nodeAnim:         make(map[model.NodeID]float64),
		logger:           logger,
		graph:            model.NewGraph(logger),
		// Prevent highlightBeat from early-returning due to missing instruments in stubbed DrumView.
		playFn: func(string, float64, ...float64) {},
	}
	g.drum = &DrumView{
		Rows:   []*DrumRow{{}},
		Graph:  g.graph,
		logger: logger,
	}
	g.drum.SetLength(size)
	g.drum.Rows[0].CellTypes[0] = nodeType
	g.state.SetPlaying(true)
	// Ensure nodeTriggeredState sees the node as present in the graph.
	g.graph.AddNode(0, 0, nodeType) // id=0
	g.graph.AddNode(0, 0, nodeType) // id=1
	g.setLastTriggeredForTest(0, 1, triggered)
	return g
}

func TestHighlightBeatSkipsUntriggeredRegular(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeRegular, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("highlight present for skipped regular node")
	}
	if v, _ := g.lastTriggeredForTest(0, info.NodeID); v {
		t.Fatalf("lastTriggered recorded true for skipped regular node")
	}
}

func TestHighlightBeatSkipsUntriggeredMute(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeMute, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeMute}
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("highlight present for skipped mute node")
	}
	if v, _ := g.lastTriggeredForTest(0, info.NodeID); v {
		t.Fatalf("lastTriggered recorded true for skipped mute node")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("node animation set for skipped mute node")
	}
}

func TestHighlightVisualSkipsUntriggeredRegular(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeRegular, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	key := makeBeatKey(0, 0)

	g.highlightVisual(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("visual highlight present for skipped regular node")
	}
}

func TestHighlightVisualSkipsUntriggeredMute(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeMute, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeMute}
	key := makeBeatKey(0, 0)

	g.highlightVisual(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("visual highlight present for skipped mute node")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("node animation set for skipped mute node")
	}
}

func TestHighlightBeatRequiresAudibleRow(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; !ok {
		t.Fatalf("expected highlight when row is audible")
	}
	if g.nodeAnim[info.NodeID] == 0 {
		t.Fatalf("expected node animation when row is audible")
	}
}

func TestHighlightBeatSkipsMutedRow(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	setRowMuted(t, g.drum, 0, true)
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("muted row should not highlight")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("muted row should not animate")
	}
}

func TestHighlightVisualSkipsMutedRow(t *testing.T) {
	assertDefaultParityState(t)
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	setRowMuted(t, g.drum, 0, true)
	key := makeBeatKey(0, 0)

	g.highlightVisual(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("muted row should not get visual highlight")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("muted row should not animate during visual highlight")
	}
}
