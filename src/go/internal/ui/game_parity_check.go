package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// parityExpected returns the scheduler's predicted truth at (row, idx).
func (g *Game) parityExpected(row, idx int, info model.BeatInfo) bool {
	if row < 0 || g.engine == nil || g.engine.Predictor == nil {
		return false
	}
	switch info.NodeType {
	case model.NodeTypeRegular, model.NodeTypeChord:
		return g.engine.Predictor.AudibleAt(row, idx) // audio truth
	case model.NodeTypeMute:
		return g.engine.Predictor.TriggeredAt(row, idx)
	default:
		return false
	}
}

// parityCheck logs a mismatch between scheduler truth and DrumView slate.
func (g *Game) parityCheck(row, idx int, info model.BeatInfo, scheduled bool, source string, missing bool) {
	if g == nil || g.drum == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	if g.importDialog || g.importing || (g.drum != nil && g.drum.importing) {
		return
	}
	if g.state.JustResumed() {
		return
	}
	// After a mutation the audio thread may still fire pre-mutation notes, so comparisons are
	// meaningless until grace expires.
	if g.parityInGrace() {
		return
	}
	if missing {
		// A row without an instrument intentionally plays nothing.
		return
	}
	if !g.renderReady {
		return
	}
	if row < 0 || row >= len(g.drum.Rows) {
		return
	}
	if g.drum.Length <= 0 {
		return
	}
	if info.NodeType == model.NodeTypeMute {
		return
	}
	// Only the current and future window: the past shows immutable commits.
	playhead := g.playheadFloor() - 1
	if playhead < 0 {
		playhead = 0
	}
	if idx < playhead {
		return
	}
	// The slate shows predictor truth, which ignores mute/solo.
	anySolo := false
	for _, r := range g.drum.Rows {
		if r.Solo {
			anySolo = true
			break
		}
	}
	if g.drum.Rows[row].Muted || (anySolo && !g.drum.Rows[row].Solo) {
		return
	}
	slate, inWindow, offset, length, slateSource := g.parityViewState(row, idx)
	if !inWindow {
		return
	}
	// The just-scheduled beat is a one-frame pipeline lag in every mode, fatal included.
	if scheduled && !slate && row >= 0 && row < len(g.seqNextIdxs) && idx == g.seqNextIdxs[row]-1 {
		return
	}
	// Do not early-return; we also want to check highlight parity.
	if scheduled == slate {
		return
	}
	detail := ""
	if slateSource == "cache" {
		detail = "slate=cache"
	}
	entry := mismatchEntry{
		Row:       row,
		Abs:       idx,
		Kind:      "scheduler_vs_view",
		NodeType:  info.NodeType,
		Scheduled: scheduled,
		Slate:     slate,
		Expected:  scheduled,
		Actual:    slate,
		Source:    source,
		Offset:    offset,
		Length:    length,
		InWindow:  inWindow,
		RowMuted:  g.drum.Rows[row].Muted,
		AnySolo:   anySolo,
		Missing:   missing,
		Detail:    detail,
	}
	g.parityReport(entry)
}

// parityHighlightCheck disabled; highlight parity handled in parityScan with timing guards.
