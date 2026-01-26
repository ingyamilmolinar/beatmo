package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/timeline"
)

// parityExpected returns the scheduler's predicted truth at (row, idx).
func (g *Game) parityExpected(row, idx int, info model.BeatInfo) bool {
	if row < 0 || g.engine == nil || g.engine.Predictor == nil {
		return false
	}
	switch info.NodeType {
	case model.NodeTypeRegular:
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
	if missing {
		// If the row lacks an instrument/bus, the scheduler intentionally skips
		// audio; don't flag parity for the visible slate in that case.
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
	// Ignore beats that are already behind the playhead; DrumView may still
	// show immutable playback commits there, but scheduler parity should only
	// consider the current/future window.
	playhead := g.playheadFloor() - 1
	if playhead < 0 {
		playhead = 0
	}
	if idx < playhead {
		return
	}
	// Skip parity for muted/solo-gated rows: scheduler passes false but slate
	// shows predictor truth which doesn't account for UI mute/solo state.
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
	fatalNow := (g.parityWatch == parityWatchPanic) || (g.parityWatch == parityWatchOff && parityFatalEnabled.Load())
	// Allow a single-beat grace during nonfatal/log parity runs: sequencer may
	// have just scheduled idx while DrumView will reflect it on the next refresh.
	if !fatalNow && scheduled && !slate && row >= 0 && row < len(g.seqNextIdxs) && idx == g.seqNextIdxs[row]-1 {
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

// parityTruth returns the expected audible/visible truth at (row, abs) using
// predictor + timeline without mutating timeline state. ok=false when the row
// is out of range.
func (g *Game) parityTruth(row, abs int, freezeLimit int) (val bool, typ model.NodeType, ok bool) {
	if g == nil || g.drum == nil || row < 0 || row >= len(g.drum.Rows) || abs < 0 {
		return false, model.NodeTypeInvisible, false
	}
	bi := g.beatInfoAtRow(row, abs)
	val = false
	typ = bi.NodeType
	v, ctyp, kind, hasCommit := g.timelineCommittedWithKind(row, abs)
	pastExclusive := g.rowPastExclusive(row)
	inPast := abs < pastExclusive
	if hasCommit && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) && inPast {
		return v, ctyp, true
	}
	if g.engine == nil || g.engine.Predictor == nil {
		return false, typ, true
	}
	g.engine.Predictor.Ensure(abs + 1)
	switch bi.NodeType {
	case model.NodeTypeRegular:
		val = g.engine.Predictor.AudibleAt(row, abs)
	case model.NodeTypeMute:
		val = g.engine.Predictor.TriggeredAt(row, abs)
	}
	if hasCommit && freezeLimit >= 0 && abs <= freezeLimit {
		// Playback/import already handled; for soft commits just prefer predictor truth.
		if kind == timeline.CommitKindSeeded || kind == timeline.CommitKindGapPad || kind == timeline.CommitKindReleased {
			return val, ctyp, true
		}
	}
	if hasCommit && ctyp != model.NodeTypeInvisible {
		typ = ctyp
	}
	return val, typ, true
}
