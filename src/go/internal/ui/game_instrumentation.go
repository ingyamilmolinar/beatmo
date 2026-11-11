package ui

import "github.com/ingyamilmolinar/beatmo/internal/gamestate"

// RowStateDump captures per-row cached state for instrumentation.
type RowStateDump struct {
	NextBeatIdx        int
	FrozenUpTo         int
	Timeline           TimelineSegments
	Window             RowWindow
	PredictorVisible   []bool
	PredictorAudible   []bool
	PredictorTriggered []bool
	Transport          gamestate.Snapshot
}

func (g *Game) rowStateSnapshot(rowIdx int) RowStateDump {
	dump := RowStateDump{
		NextBeatIdx: -1,
		FrozenUpTo:  -1,
	}
	if g == nil || g.drum == nil {
		return dump
	}
	if rowIdx < 0 || rowIdx >= len(g.drum.Rows) {
		return dump
	}
	if rowIdx < len(g.nextBeatIdxs) {
		dump.NextBeatIdx = g.nextBeatIdxs[rowIdx]
	}
	if rowIdx < len(g.frozenUpToByRow) {
		dump.FrozenUpTo = g.frozenUpToByRow[rowIdx]
	}

	dump.Timeline = g.timelineService().Snapshot(rowIdx)
	dump.Window = cloneRowWindow(g.rowWindow(rowIdx))
	dump.Transport = g.transportSnapshot()

	horizon := g.drum.Offset + g.drum.Length
	if rowIdx < len(g.beatInfosByRow) && len(g.beatInfosByRow[rowIdx]) > horizon {
		horizon = len(g.beatInfosByRow[rowIdx])
	}
	if horizon < 0 {
		horizon = 0
	}

	if g.engine == nil || g.engine.Predictor == nil {
		return dump
	}
	g.engine.Predictor.Ensure(horizon)
	dump.PredictorVisible = make([]bool, horizon)
	dump.PredictorAudible = make([]bool, horizon)
	dump.PredictorTriggered = make([]bool, horizon)
	for i := 0; i < horizon; i++ {
		dump.PredictorVisible[i] = g.engine.Predictor.VisibleAt(rowIdx, i)
		dump.PredictorAudible[i] = g.engine.Predictor.AudibleAt(rowIdx, i)
		dump.PredictorTriggered[i] = g.engine.Predictor.TriggeredAt(rowIdx, i)
	}
	return dump
}

func (g *Game) dumpRowState(rowIdx int) RowStateDump {
	return g.rowStateSnapshot(rowIdx)
}

func (g *Game) transportSnapshot() gamestate.Snapshot {
	if g == nil || g.state == nil {
		return gamestate.Snapshot{}
	}
	return g.state.Snapshot()
}
