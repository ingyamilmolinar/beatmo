package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
	"github.com/ingyamilmolinar/beatmo/internal/ui/preview"
)

// buildRowWindow merges predictor state, timeline commits, freezes, and past preservation
// into a fresh Steps/CellTypes window.
func (g *Game) buildRowWindow(rowIdx int, cfg rowWindowConfig) ([]bool, []model.NodeType) {
	windowLen := g.drum.Length
	offset := g.drum.Offset

	nextIdx := g.rowPastExclusive(rowIdx)

	wantAt := cfg.predictAt

	beatInfoAt := func(abs int) model.BeatInfo { return g.beatInfoAtRow(rowIdx, abs) }
	commitAt := func(abs int) (bool, model.NodeType, timeline.CommitKind, bool) {
		return g.timelineCommittedWithKind(rowIdx, abs)
	}

	return preview.BuildRowWindow(preview.Config{
		Offset:            offset,
		Length:            windowLen,
		NextIdx:           nextIdx,
		ElapsedBeats:      g.elapsedBeats,
		FreezeLimit:       cfg.freezeLimit,
		PreservePastSteps: g.Playing(),
		PrevStepsOffset:   cfg.prevStepsOffset,
		PrevSteps:         cfg.prevSteps,
		PrevTypesOffset:   cfg.prevTypesOffset,
		PrevTypes:         cfg.prevTypes,
		FastPath:          cfg.fastPath,
	}, beatInfoAt, wantAt, commitAt, cfg.reuseSteps, cfg.reuseTypes)
}

func rowRenderSignature(steps []bool, types []model.NodeType) uint64 {
	var h uint64 = 1469598103934665603 // FNV offset basis
	const prime uint64 = 1099511628211
	n := len(steps)
	if len(types) < n {
		n = len(types)
	}
	for i := 0; i < n; i++ {
		var v uint64
		if steps[i] {
			v = 1
		}
		v |= uint64(uint32(types[i])) << 1
		h = (h ^ v) * prime
	}
	// Include any length mismatch defensively (should not happen in steady state).
	for i := n; i < len(steps); i++ {
		var v uint64
		if steps[i] {
			v = 1
		}
		h = (h ^ v) * prime
	}
	for i := n; i < len(types); i++ {
		v := uint64(uint32(types[i])) << 1
		h = (h ^ v) * prime
	}
	h = (h ^ uint64(len(steps))) * prime
	h = (h ^ uint64(len(types))) * prime
	return h
}
