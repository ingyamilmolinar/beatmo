package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// ParityMismatchSnapshot returns a copy of recent scheduler-vs-UI mismatches.
func (g *Game) ParityMismatchSnapshot() []mismatchEntry {
	return g.parityRing.snapshot()
}

// ClearParityMismatches resets the mismatch ring (primarily for tests).
func (g *Game) ClearParityMismatches() {
	g.parityRing.clear()
}

// clearParityState drops parity buffers (audio events, seq decisions, highlights)
// so a stop/restart cycle does not compare fresh playback against stale events.
func (g *Game) clearParityState() {
	g.parityMu.Lock()
	g.parityAudio = nil
	g.parityAudioMaxIdx = nil
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.parityMu.Unlock()
	g.highlightMu.Lock()
	g.highlightedBeats = make(map[int]int64)
	g.highlightMu.Unlock()
	g.ClearParityMismatches()
}

// startImportDialog temporarily disables parity fatals/watch while the file
// picker is open, avoiding scheduler/UI parity panics during the blocking
// dialog. It is idempotent and safe to call multiple times.
func (g *Game) startImportDialog() {
	g.importDialog = true
	g.importPrevParityFatal = parityFatalEnabled.Load()
	g.importPrevParityWatch = g.parityWatch
	parityFatalEnabled.Store(false)
	g.parityWatch = parityWatchOff
}

// endImportDialog restores parity settings after the import dialog closes.
func (g *Game) endImportDialog() {
	if !g.importDialog {
		return
	}
	parityFatalEnabled.Store(g.importPrevParityFatal)
	g.parityWatch = g.importPrevParityWatch
	g.importDialog = false
}

func (g *Game) recordParityAudio(row, abs int, when float64, inst string, vol, pitch, dur float64, gen uint64) {
	if g == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	if row < 0 {
		return
	}
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	if len(g.parityAudioMaxIdx) < len(g.drum.Rows) {
		orig := g.parityAudioMaxIdx
		g.parityAudioMaxIdx = make([]int, len(g.drum.Rows))
		for i := range g.parityAudioMaxIdx {
			g.parityAudioMaxIdx[i] = -1
			if i < len(orig) {
				g.parityAudioMaxIdx[i] = orig[i]
			}
		}
	}
	if row < len(g.parityAudioMaxIdx) && abs > g.parityAudioMaxIdx[row] {
		g.parityAudioMaxIdx[row] = abs
	}
	g.parityAudio = append(g.parityAudio, parityAudioEvent{
		Row:        row,
		Abs:        abs,
		When:       when,
		Inst:       inst,
		Vol:        vol,
		Pitch:      pitch,
		Dur:        dur,
		Gen:        gen,
		RecordedAt: time.Now(),
	})
	const parityAudioMax = 1024
	if len(g.parityAudio) > parityAudioMax {
		// Drop oldest to bound memory without copying the slice.
		g.parityAudio = g.parityAudio[len(g.parityAudio)-parityAudioMax:]
	}
}

func (g *Game) recordSeqDecision(row, abs int, audible bool, typ model.NodeType, missing bool) {
	if g == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	// Record view truth (VisibleAt) separately from audio truth (AudibleAt).
	// The seq_vs_view parity check compares scheduled view state against the
	// DrumView slate, not audio state, to avoid false positives when mute
	// gates make nodes inaudible but still visible.
	visible := false
	if g.engine != nil && g.engine.Predictor != nil {
		if typ == model.NodeTypeMute {
			visible = g.engine.Predictor.TriggeredAt(row, abs)
		} else {
			visible = g.engine.Predictor.VisibleAt(row, abs)
		}
	}
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	if g.paritySeqDecisions[row] == nil {
		g.paritySeqDecisions[row] = make(map[int]paritySeqDecision)
	}
	g.paritySeqDecisions[row][abs] = paritySeqDecision{
		Row:        row,
		Abs:        abs,
		Audible:    audible,
		Visible:    visible,
		NodeType:   typ,
		Missing:    missing,
		RecordedAt: time.Now(),
	}
}

func (g *Game) parityPrune(minAbs int) {
	if g == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	if minAbs > 0 && len(g.parityAudio) > 0 {
		dst := g.parityAudio[:0]
		for _, ev := range g.parityAudio {
			if ev.Abs >= minAbs {
				dst = append(dst, ev)
			}
		}
		g.parityAudio = dst
	}
	for row, m := range g.paritySeqDecisions {
		for abs := range m {
			if abs < minAbs {
				delete(m, abs)
			}
		}
		if len(m) == 0 {
			delete(g.paritySeqDecisions, row)
		}
	}
}

// parityReport records and optionally panics/logs a mismatch according to the watcher mode.
func (g *Game) parityReport(entry mismatchEntry) {
	if g == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	g.parityRing.add(entry)
	switch g.parityWatch {
	case parityWatchOff:
		if !parityFatalEnabled.Load() {
			return
		}
		entry.Force = true
		g.emitParityFatal(entry)
	case parityWatchLog:
		if g.logger != nil {
			g.logger.Warnf("[PARITY][%s] row=%d abs=%d expected=%v actual=%v src=%s detail=%s",
				entry.Kind, entry.Row, entry.Abs, entry.Expected, entry.Actual, entry.Source, entry.Detail)
		}
	case parityWatchPanic:
		entry.Force = true
		g.emitParityFatal(entry)
	}
}

// emitParityFatal logs full debug context and panics to terminate immediately.
func (g *Game) emitParityFatal(e mismatchEntry) {
	if !parityFatalEnabled.Load() && !e.Force {
		if g.logger != nil {
			g.logger.Warnf("[PARITY][nonfatal] row=%d abs=%d src=%s sched=%v slate=%v inWindow=%v", e.Row, e.Abs, e.Source, e.Scheduled, e.Slate, e.InWindow)
		}
		return
	}
	builder := &strings.Builder{}
	builder.WriteString("[PARITY] scheduler vs DrumView mismatch\n")
	builder.WriteString(fmt.Sprintf(" row=%d abs=%d source=%s kind=%s scheduled=%v slate=%v expected=%v actual=%v nodeType=%v missingInst=%v rowMuted=%v anySolo=%v when=%.6f detail=%s\n",
		e.Row, e.Abs, e.Source, e.Kind, e.Scheduled, e.Slate, e.Expected, e.Actual, e.NodeType, e.Missing, e.RowMuted, e.AnySolo, e.When, e.Detail))
	builder.WriteString(fmt.Sprintf(" inWindow=%v offset=%d length=%d\n", e.InWindow, e.Offset, e.Length))
	// Transport snapshot
	ts := g.transportSnapshot()
	builder.WriteString(fmt.Sprintf(" transport: playing=%v paused=%v bpm=%d beatBase=%.3f lastBeat=%.3f lastStep=%d pausedBeats=%d seekFreeze=%d\n",
		ts.Playing, ts.Paused, ts.AppliedBPM, ts.BeatBase, ts.LastBeat, ts.LastStep, ts.PausedBeats, ts.SeekFreezeFrames))
	// Scheduler indices
	builder.WriteString(fmt.Sprintf(" elapsedBeats=%d offset=%d length=%d seqNextIdxs=%v nextBeatIdxs=%v\n",
		g.elapsedBeats, g.drum.Offset, g.drum.Length, g.seqNextIdxs, g.nextBeatIdxs))
	// Row window snapshot
	if e.Row >= 0 && e.Row < len(g.drum.Rows) {
		r := g.drum.Rows[e.Row]
		builder.WriteString(fmt.Sprintf(" row steps=%v\n", boolSliceDebug(r.Steps)))
		builder.WriteString(fmt.Sprintf(" row types=%v\n", r.CellTypes))
	}
	// Predictor values at abs
	vis, trig := false, false
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.Ensure(e.Abs + 1)
		vis = g.engine.Predictor.VisibleAt(e.Row, e.Abs)
		trig = g.engine.Predictor.TriggeredAt(e.Row, e.Abs)
	}
	builder.WriteString(fmt.Sprintf(" predictor visible=%v triggered=%v\n", vis, trig))
	// Timeline snapshot
	seg := g.TimelineSegments(e.Row)
	builder.WriteString(fmt.Sprintf(" timeline offset=%d past=%v pastMask=%v present=%v future=%v\n",
		seg.Offset, boolSliceDebug(seg.Past), boolSliceDebug(seg.PastMask), boolSliceDebug(seg.Present), boolSliceDebug(seg.Future)))
	// Graph summary
	builder.WriteString(fmt.Sprintf(" graph start=%d nodes=%d edges=%d\n", g.graph.StartNodeID, len(g.graph.Nodes), len(g.graph.Edges)))
	// Dump recent mismatches ring
	builder.WriteString(" recent mismatches:\n")
	for _, m := range g.parityRing.snapshot() {
		builder.WriteString(fmt.Sprintf("  row=%d abs=%d src=%s sched=%v slate=%v nodeType=%v\n", m.Row, m.Abs, m.Source, m.Scheduled, m.Slate, m.NodeType))
	}
	// Highlight state at abs
	if e.Row >= 0 {
		key := makeBeatKey(e.Row, e.Abs)
		g.highlightMu.RLock()
		until, hl := g.highlightedBeats[key]
		g.highlightMu.RUnlock()
		builder.WriteString(fmt.Sprintf(" highlight key=%d present=%v until=%d frame=%d\n", key, hl, until, g.frame))
	}
	msg := builder.String()
	if g.logger != nil {
		g.logger.Errorf(msg)
	}
	if os.Getenv("PARITY_DUMP_STDERR") == "1" {
		fmt.Fprint(os.Stderr, msg)
	}
	panic("scheduler/drumview parity mismatch")
}

func (g *Game) parityScanSettings() (every, stride int) {
	if g != nil {
		every = g.parityScanEvery
		stride = g.parityScanStride
	}
	if every < 1 {
		every = parityScanEveryFrames
	}
	if stride < 1 {
		stride = parityScanStride
	}
	if stride < 1 {
		stride = 1
	}
	if runningUnderGoTest() {
		return every, stride
	}
	// Reduce scan frequency in runtime logging mode to lower CPU use.
	if g != nil && g.parityWatch == parityWatchLog && !parityFatalEnabled.Load() {
		if every < 6 {
			every = 6
		}
	}
	// WASM targets get a more conservative default to reduce main-thread load.
	if runtime.GOOS == "js" && every < 8 {
		every = 8
	}
	return every, stride
}

func (g *Game) parityScanDue(every int) bool {
	if g == nil {
		return false
	}
	if every <= 1 {
		return true
	}
	return g.frame-g.parityScanLastFrame >= int64(every)
}

func (g *Game) recordParityScan(d time.Duration) {
	if g == nil {
		return
	}
	ns := int64(d)
	g.parityScanSumNS += ns
	g.parityScanCount++
	if ns > g.parityScanMaxNS {
		g.parityScanMaxNS = ns
	}
}

func (g *Game) parityScanStats() (avgMS, maxMS float64, count int64) {
	if g == nil {
		return 0, 0, 0
	}
	count = g.parityScanCount
	if count > 0 {
		avgMS = (float64(g.parityScanSumNS) / float64(count)) / 1e6
	}
	maxMS = float64(g.parityScanMaxNS) / 1e6
	return avgMS, maxMS, count
}

func (g *Game) resetParityPerf() {
	if g == nil {
		return
	}
	g.parityScanSumNS = 0
	g.parityScanMaxNS = 0
	g.parityScanCount = 0
}

// LastImmutableAbs exposes the newest playback/import commit for diagnostics
// and tests. Returns -1 when none exist.
func (g *Game) LastImmutableAbs(row int) int {
	return g.lastImmutableCommit(row)
}

// lastImmutableCommit returns the newest playback/import commit for the row.
// Seeded/gap/released entries are ignored to keep immutable history intact.
func (g *Game) lastImmutableCommit(row int) int {
	if g == nil {
		return -1
	}
	return g.timelineService().LastImmutableAbs(row)
}
