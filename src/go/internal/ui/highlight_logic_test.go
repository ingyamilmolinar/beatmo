package ui

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func stubGameForHighlight(nodeType model.NodeType, triggered bool) *Game {
	logger := game_log.New(io.Discard, game_log.LevelError)
	const size = 64
	g := &Game{
		highlightedBeats:   make(map[int]int64),
		lastTriggeredByRow: make(map[int]map[model.NodeID]bool),
		nodeAnim:           make(map[model.NodeID]float64),
		logger:             logger,
		graph:              model.NewGraph(logger),
	}
	g.drum = &DrumView{
		Rows:   []*DrumRow{{Steps: make([]bool, size), CellTypes: make([]model.NodeType, size)}},
		Length: size,
	}
	g.drum.Rows[0].CellTypes[0] = nodeType
	g.predTriggeredByRow = [][]bool{make([]bool, size)}
	g.predTriggeredByRow[0][0] = triggered
	g.predVisibleByRow = [][]bool{make([]bool, size)}
	g.predAudibleByRow = [][]bool{make([]bool, size)}
	g.predHorizon = size
	g.lastTriggeredByRow[0] = make(map[model.NodeID]bool)
	g.lastTriggeredByRow[0][1] = triggered
	return g
}

func TestHighlightBeatSkipsUntriggeredRegular(t *testing.T) {
	g := stubGameForHighlight(model.NodeTypeRegular, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("highlight present for skipped regular node")
	}
	if g.lastTriggeredByRow[0][info.NodeID] {
		t.Fatalf("lastTriggered recorded true for skipped regular node")
	}
}

func TestHighlightBeatSkipsUntriggeredMute(t *testing.T) {
	g := stubGameForHighlight(model.NodeTypeMute, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeMute}
	key := makeBeatKey(0, 0)

	g.highlightBeat(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("highlight present for skipped mute node")
	}
	if g.lastTriggeredByRow[0][info.NodeID] {
		t.Fatalf("lastTriggered recorded true for skipped mute node")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("node animation set for skipped mute node")
	}
}

func TestHighlightVisualSkipsUntriggeredRegular(t *testing.T) {
	g := stubGameForHighlight(model.NodeTypeRegular, false)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	key := makeBeatKey(0, 0)

	g.highlightVisual(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("visual highlight present for skipped regular node")
	}
}

func TestHighlightVisualSkipsUntriggeredMute(t *testing.T) {
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
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.instMu.Lock()
	g.drum.instAvail = map[string]bool{"snare": true}
	g.drum.instMu.Unlock()
	g.drum.Rows[0].Instrument = "snare"
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
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.instMu.Lock()
	g.drum.instAvail = map[string]bool{"snare": true}
	g.drum.instMu.Unlock()
	g.drum.Rows[0].Instrument = "snare"
	g.drum.Rows[0].Muted = true
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
	g := stubGameForHighlight(model.NodeTypeRegular, true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.drum.instMu.Lock()
	g.drum.instAvail = map[string]bool{"snare": true}
	g.drum.instMu.Unlock()
	g.drum.Rows[0].Instrument = "snare"
	g.drum.Rows[0].Muted = true
	key := makeBeatKey(0, 0)

	g.highlightVisual(0, 0, info, 10)
	if _, ok := g.highlightedBeats[key]; ok {
		t.Fatalf("muted row should not get visual highlight")
	}
	if g.nodeAnim[info.NodeID] != 0 {
		t.Fatalf("muted row should not animate during visual highlight")
	}
}
