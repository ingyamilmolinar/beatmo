package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

type rowSnapshot struct {
	Offset   int
	Steps    map[int]bool
	State    RowStateDump
	Timeline TimelineSegments
	Commits  map[int]commitSnapshot
}

type commitSnapshot struct {
	Val  bool
	Typ  model.NodeType
	Kind timeline.CommitKind
	OK   bool
}

type scenarioContext struct {
	row        int
	fromAbs    int
	toAbs      int
	fromInfo   model.BeatInfo
	toInfo     model.BeatInfo
	mid1Coord  struct{ I, J int }
	mid2Coord  struct{ I, J int }
	successors []model.NodeID
}

func captureRowSnapshot(t *testing.T, g *Game, row int) rowSnapshot {
	g.refreshDrumRow()
	state := g.rowStateSnapshot(row)
	steps := make(map[int]bool)
	commits := make(map[int]commitSnapshot)
	offset := g.drum.Offset
	if row >= 0 && row < len(g.drum.Rows) {
		for idx, on := range g.drum.Rows[row].Steps {
			steps[offset+idx] = on
		}
		for i := 0; i < len(g.drum.Rows[row].Steps); i++ {
			abs := offset + i
			v, typ, k, ok := g.timelineCommittedWithKind(row, abs)
			if ok {
				commits[abs] = commitSnapshot{Val: v, Typ: typ, Kind: k, OK: ok}
			}
		}
	}
	return rowSnapshot{Offset: offset, Steps: steps, State: state, Timeline: g.TimelineSegments(row), Commits: commits}
}

func compareSnapshots(t *testing.T, g *Game, label string, before, after rowSnapshot, row int) {
	freeze := before.State.FrozenUpTo
	if after.State.FrozenUpTo < freeze {
		freeze = after.State.FrozenUpTo
	}
	if before.Timeline.Offset != after.Timeline.Offset {
		t.Fatalf("%s: row %d timeline offset changed (%d -> %d)", label, row, before.Timeline.Offset, after.Timeline.Offset)
	}
	for i := range before.Timeline.Past {
		abs := before.Timeline.Offset + i
		if abs <= freeze {
			if i < len(before.Timeline.PastMask) && i < len(after.Timeline.PastMask) {
				if before.Timeline.PastMask[i] && after.Timeline.PastMask[i] && before.Timeline.Past[i] != after.Timeline.Past[i] {
					t.Logf("timeline before offset=%d mask=%v past=%v", before.Timeline.Offset, before.Timeline.PastMask, before.Timeline.Past)
					t.Logf("timeline after offset=%d mask=%v past=%v", after.Timeline.Offset, after.Timeline.PastMask, after.Timeline.Past)
					bCommit := before.Commits[abs]
					aCommit := after.Commits[abs]
					t.Fatalf("%s: row %d past timeline mismatch at abs=%d before=%v after=%v freeze=%d commitBefore={%v %v %v %v} commitAfter={%v %v %v %v}",
						label, row, abs, before.Timeline.Past[i], after.Timeline.Past[i], freeze,
						bCommit.Val, bCommit.Typ, bCommit.Kind, bCommit.OK,
						aCommit.Val, aCommit.Typ, aCommit.Kind, aCommit.OK,
					)
				}
			}
		}
	}
	next := after.State.NextBeatIdx
	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("%s: engine predictor is nil", label)
	}
	g.engine.Predictor.Ensure(next + g.grid.MaxDiv()*2)
	for abs := next; abs < next+g.grid.MaxDiv(); abs++ {
		pred := g.engine.Predictor.VisibleAt(row, abs)
		cur, ok := after.Steps[abs]
		if !ok {
			continue
		}
		if cur != pred {
			t.Fatalf("%s: row %d future mismatch at abs=%d (window=%v pred=%v)", label, row, abs, cur, pred)
		}
		idx := abs - after.Timeline.Offset
		if idx >= 0 {
			if idx < len(after.Timeline.Present) {
				if cur != after.Timeline.Present[idx] {
					t.Fatalf("%s: row %d present timeline mismatch at abs=%d seg=%v val=%v", label, row, abs, after.Timeline.Present[idx], cur)
				}
				continue
			}
			if idx < len(after.Timeline.Future) {
				if cur != after.Timeline.Future[idx] {
					t.Fatalf("%s: row %d future timeline mismatch at abs=%d seg=%v val=%v", label, row, abs, after.Timeline.Future[idx], cur)
				}
			}
		}
	}
}

func findFuturePair(t *testing.T, g *Game, row, start int) (model.BeatInfo, model.BeatInfo, int, int) {
	var prev model.BeatInfo
	prevAbs := -1
	for abs := start; abs < start+512; abs++ {
		bi := g.beatInfoAtRow(row, abs)
		if bi.NodeID == model.InvalidNodeID || bi.NodeType != model.NodeTypeRegular {
			continue
		}
		if prev.NodeID != 0 && bi.NodeID != prev.NodeID {
			return prev, bi, prevAbs, abs
		}
		prev = bi
		prevAbs = abs
	}
	t.Fatalf("no future pair found after %d", start)
	return model.BeatInfo{}, model.BeatInfo{}, -1, -1
}

func addDetourNodes(t *testing.T, g *Game, row int, ctx *scenarioContext) {
	start := g.rowStateSnapshot(row).NextBeatIdx + g.grid.MaxDiv()
	fromInfo, toInfo, fromAbs, toAbs := findFuturePair(t, g, row, start)
	dir := 1
	if toInfo.I < fromInfo.I {
		dir = -1
	}
	mid1I := fromInfo.I + dir
	mid1J := fromInfo.J
	for g.nodeAt(mid1I, mid1J) != nil {
		mid1I += dir
	}
	mid2I := mid1I
	mid2J := fromInfo.J + 1
	for g.nodeAt(mid2I, mid2J) != nil {
		mid2J++
	}
	n1 := g.tryAddNode(mid1I, mid1J, model.NodeTypeRegular)
	if n1 == nil {
		t.Fatalf("failed to add detour node mid1")
	}
	n2 := g.tryAddNode(mid2I, mid2J, model.NodeTypeRegular)
	if n2 == nil {
		t.Fatalf("failed to add detour node mid2")
	}
	fromNode := g.nodeByID(fromInfo.NodeID)
	toNode := g.nodeByID(toInfo.NodeID)
	if fromNode == nil || toNode == nil {
		t.Fatalf("missing detour endpoints")
	}
	g.deleteEdge(fromNode, toNode)
	g.addEdge(fromNode, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, toNode)
	ctx.row = row
	ctx.fromAbs = fromAbs
	ctx.toAbs = toAbs
	ctx.fromInfo = fromInfo
	ctx.toInfo = toInfo
	ctx.successors = collectSuccessors(g, toInfo.NodeID)
	ctx.mid1Coord = struct{ I, J int }{I: mid1I, J: mid1J}
	ctx.mid2Coord = struct{ I, J int }{I: mid2I, J: mid2J}
}

func restoreDetour(t *testing.T, g *Game, ctx *scenarioContext) {
	if ctx.row < 0 {
		return
	}
	fromNode := g.nodeByID(ctx.fromInfo.NodeID)
	toNode := g.nodeByID(ctx.toInfo.NodeID)
	if fromNode == nil || toNode == nil {
		t.Fatalf("restore: missing endpoints")
	}
	m1 := g.nodeAt(ctx.mid1Coord.I, ctx.mid1Coord.J)
	m2 := g.nodeAt(ctx.mid2Coord.I, ctx.mid2Coord.J)
	if m1 != nil {
		g.deleteEdge(fromNode, m1)
		if m2 != nil {
			g.deleteEdge(m1, m2)
		}
		g.deleteNode(m1)
	}
	if m2 != nil {
		g.deleteEdge(m2, toNode)
		g.deleteNode(m2)
	}
	g.addEdge(fromNode, toNode)
}

func setNodeLogic(t *testing.T, g *Game, info model.BeatInfo, kind string, n int, p float64) {
	node, ok := g.graph.GetNodeByID(info.NodeID)
	if !ok {
		t.Fatalf("logic change: node %d not found", info.NodeID)
	}
	params := node.Params
	params.LogicKind = kind
	params.LogicN = n
	params.LogicP = p
	g.graph.SetNodeParams(info.NodeID, params)
	g.notifyPredictorNode(info.NodeID)
}

func advanceGame(g *Game, frames int) {
	if frames <= 0 {
		return
	}
	if !g.Playing() {
		for i := 0; i < frames; i++ {
			_ = g.Update()
		}
		return
	}
	g.elapsedBeats += frames
	if g.elapsedBeats < 0 {
		g.elapsedBeats = 0
	}
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	for row := range g.drum.Rows {
		g.nextBeatIdxs[row] = g.elapsedBeats + 1
		g.seqNextIdxs[row] = g.elapsedBeats + 1
	}
	g.refreshDrumRow()
}

func refreshContext(g *Game, ctx *scenarioContext) {
	if ctx.row < 0 {
		return
	}
	ctx.fromInfo = g.beatInfoAtRow(ctx.row, ctx.fromAbs)
	ctx.toInfo = g.beatInfoAtRow(ctx.row, ctx.toAbs)
	ctx.successors = collectSuccessors(g, ctx.toInfo.NodeID)
}

func collectSuccessors(g *Game, nodeID model.NodeID) []model.NodeID {
	if nodeID == model.InvalidNodeID {
		return nil
	}
	succ := []model.NodeID{}
	for edge := range g.graph.Edges {
		if edge[0] == nodeID {
			succ = append(succ, edge[1])
		}
	}
	return succ
}

func maxLookahead(g *Game) int {
	if g.grid != nil {
		return g.grid.MaxDiv() * 4
	}
	return 128
}

func findRegularForwardFrom(t *testing.T, g *Game, row, startAbs int) (model.BeatInfo, int) {
	limit := maxLookahead(g)
	for delta := 0; delta < limit; delta++ {
		abs := startAbs + delta
		info := g.beatInfoAtRow(row, abs)
		if info.NodeID == model.InvalidNodeID {
			continue
		}
		if info.NodeType != model.NodeTypeRegular {
			continue
		}
		return info, abs
	}
	t.Fatalf("row %d forward regular beat not found from abs=%d", row, startAbs)
	return model.BeatInfo{}, -1
}

func findRegularBackwardFrom(t *testing.T, g *Game, row, startAbs int) (model.BeatInfo, int) {
	limit := maxLookahead(g)
	for delta := 0; delta < limit; delta++ {
		abs := startAbs - delta
		if abs < 0 {
			break
		}
		info := g.beatInfoAtRow(row, abs)
		if info.NodeID == model.InvalidNodeID {
			continue
		}
		if info.NodeType != model.NodeTypeRegular {
			continue
		}
		return info, abs
	}
	t.Fatalf("row %d backward regular beat not found from abs=%d", row, startAbs)
	return model.BeatInfo{}, -1
}

func findImmediateBeatInfo(t *testing.T, g *Game, row int) (model.BeatInfo, int) {
	if row >= len(g.nextBeatIdxs) {
		t.Fatalf("row %d missing nextBeatIdx", row)
	}
	abs := g.nextBeatIdxs[row]
	if abs < 0 {
		t.Fatalf("row %d nextBeatIdx negative", row)
	}
	return findRegularForwardFrom(t, g, row, abs)
}

func findPastBeatInfo(t *testing.T, g *Game, row int) (model.BeatInfo, int) {
	freeze := -1
	if row < len(g.frozenUpToByRow) {
		freeze = g.frozenUpToByRow[row]
	}
	if freeze < 0 {
		if row < len(g.nextBeatIdxs) {
			freeze = g.nextBeatIdxs[row] - 1
		}
	}
	if freeze < 0 {
		t.Fatalf("row %d has no past beats", row)
	}
	return findRegularBackwardFrom(t, g, row, freeze)
}

func TestDrumLiveEditScenario(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(filename), "../../../../tunkul.json")
	data, err := os.ReadFile(root)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("fixture not valid JSON")
	}

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1280, 720)
	g.drum.SetFollow(false)

	if err := g.Import(data); err != nil {
		t.Fatalf("import fixture: %v", err)
	}

	g.drum.SetBPM(120)
	g.refreshDrumRow()
	g.drum.Draw(ebiten.NewImage(1280, 240), map[int]int64{}, 0, nil, 0)

	advanceGame(g, 60)
	g.SetPlaying(true)
	advanceGame(g, 120)

	scenarios := []struct {
		label  string
		mutate func(*testing.T, *Game, *scenarioContext)
	}{
		{"detour-add", func(t *testing.T, game *Game, ctx *scenarioContext) {
			addDetourNodes(t, game, 0, ctx)
		}},
		{"detour-restore", func(t *testing.T, game *Game, ctx *scenarioContext) {
			restoreDetour(t, game, ctx)
		}},
		{"logic-probability", func(t *testing.T, game *Game, ctx *scenarioContext) {
			if ctx.toInfo.NodeID == 0 {
				addDetourNodes(t, game, 0, ctx)
			}
			setNodeLogic(t, game, ctx.toInfo, "probability", 0, 0.35)
		}},
		{"logic-none", func(t *testing.T, game *Game, ctx *scenarioContext) {
			setNodeLogic(t, game, ctx.toInfo, "none", 0, 0)
		}},
		{"node-remove-readd", func(t *testing.T, game *Game, ctx *scenarioContext) {
			if ctx.toInfo.NodeID == 0 {
				addDetourNodes(t, game, 0, ctx)
			}
			toNode := game.nodeByID(ctx.toInfo.NodeID)
			fromNode := game.nodeByID(ctx.fromInfo.NodeID)
			if toNode == nil || fromNode == nil {
				t.Fatalf("remove-readd: endpoints missing")
			}
			succIDs := ctx.successors
			if len(succIDs) == 0 {
				succIDs = collectSuccessors(game, ctx.toInfo.NodeID)
			}
			game.deleteEdge(fromNode, toNode)
			for _, sid := range succIDs {
				if sn := game.nodeByID(sid); sn != nil {
					game.deleteEdge(toNode, sn)
				}
			}
			game.deleteNode(toNode)
			newNode := game.tryAddNode(ctx.toInfo.I, ctx.toInfo.J, model.NodeTypeRegular)
			if newNode == nil {
				t.Fatalf("failed to re-add node")
			}
			game.addEdge(fromNode, newNode)
			for _, sid := range succIDs {
				if sn := game.nodeByID(sid); sn != nil {
					game.addEdge(newNode, sn)
				}
			}
			ctx.toInfo = model.BeatInfo{NodeID: newNode.ID, NodeType: model.NodeTypeRegular, I: newNode.I, J: newNode.J}
			ctx.successors = succIDs
		}},
		{"logic-skip", func(t *testing.T, game *Game, ctx *scenarioContext) {
			setNodeLogic(t, game, ctx.toInfo, "skip_every_n", 3, 0)
		}},
		{"logic-reset", func(t *testing.T, game *Game, ctx *scenarioContext) {
			setNodeLogic(t, game, ctx.toInfo, "none", 0, 0)
		}},
		{"logic-immediate", func(t *testing.T, game *Game, ctx *scenarioContext) {
			info, _ := findImmediateBeatInfo(t, game, 0)
			setNodeLogic(t, game, info, "skip_every_n", 2, 0)
		}},
		{"logic-immediate-reset", func(t *testing.T, game *Game, ctx *scenarioContext) {
			info, _ := findImmediateBeatInfo(t, game, 0)
			setNodeLogic(t, game, info, "none", 0, 0)
		}},
		{"immediate-remove-readd", func(t *testing.T, game *Game, ctx *scenarioContext) {
			targetInfo, targetAbs := findImmediateBeatInfo(t, game, 0)
			prevInfo, _ := findRegularBackwardFrom(t, game, 0, targetAbs-1)
			prevNode := game.nodeByID(prevInfo.NodeID)
			targetNode := game.nodeByID(targetInfo.NodeID)
			if prevNode == nil || targetNode == nil {
				t.Fatalf("immediate-remove: missing endpoints prev=%v target=%v", prevNode, targetNode)
			}
			succIDs := collectSuccessors(game, targetInfo.NodeID)
			game.deleteEdge(prevNode, targetNode)
			for _, sid := range succIDs {
				if sn := game.nodeByID(sid); sn != nil {
					game.deleteEdge(targetNode, sn)
				}
			}
			game.deleteNode(targetNode)
			newNode := game.tryAddNode(targetInfo.I, targetInfo.J, model.NodeTypeRegular)
			if newNode == nil {
				t.Fatalf("immediate-remove: failed to re-add node")
			}
			game.addEdge(prevNode, newNode)
			for _, sid := range succIDs {
				if sn := game.nodeByID(sid); sn != nil {
					game.addEdge(newNode, sn)
				}
			}
		}},
		{"logic-past", func(t *testing.T, game *Game, ctx *scenarioContext) {
			info, _ := findPastBeatInfo(t, game, 0)
			setNodeLogic(t, game, info, "probability", 0, 0.5)
		}},
		{"logic-past-reset", func(t *testing.T, game *Game, ctx *scenarioContext) {
			info, _ := findPastBeatInfo(t, game, 0)
			setNodeLogic(t, game, info, "none", 0, 0)
		}},
	}

	ctx := &scenarioContext{row: -1}

	rows := len(g.drum.Rows)
	if rows == 0 {
		t.Fatalf("no drum rows loaded")
	}

	for _, sc := range scenarios {
		before := make([]rowSnapshot, rows)
		for r := 0; r < rows; r++ {
			before[r] = captureRowSnapshot(t, g, r)
		}
		sc.mutate(t, g, ctx)
		g.updateBeatInfos()
		g.refreshDrumRow()
		advanceGame(g, 160)
		after := make([]rowSnapshot, rows)
		for r := 0; r < rows; r++ {
			after[r] = captureRowSnapshot(t, g, r)
		}
		for r := 0; r < rows; r++ {
			label := sc.label
			if rows > 1 {
				label = fmt.Sprintf("%s (row%d)", sc.label, r)
			}
			compareSnapshots(t, g, label, before[r], after[r], r)
		}
	}
}
