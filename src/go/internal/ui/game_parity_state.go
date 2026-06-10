package ui

// ─── Parity Verification Defaults ───────────────────────────────────────────
//
// Parity checks ensure the UI slate matches predictor/scheduler state.
//
// Platform defaults:
//   WASM:    parity stays log-only unless PARITY_WASM_FATAL=1|true|panic
//   Desktop: honors PARITY_WATCH (log|panic) and PARITY_FATAL (0|false to disable)
//
// During imports: g.importing disables parity scans and clears buffers to
// prevent false positives while the graph is being rebuilt.
//
// Grace periods prevent false positives during tight loops where the scheduler
// has committed a beat but the UI hasn't refreshed yet.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
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

// clearParityStateForGenBump drops only the parity comparator buffers whose
// entries do NOT carry a generation stamp via natural production code paths
// (highlightedBeats — a UI-rendering aid, not a parity input). It deliberately
// leaves parityAudio, paritySeqDecisions, and highlightedBeats untouched:
//
//   - parityAudio entries carry ParityGen; the scan filters by current gen.
//   - paritySeqDecisions entries carry ParityGen; same filter.
//   - highlightedBeats is genuine past UI state (one expiration frame per
//     beat key) — clearing it while the audio thread still has fresh events
//     in flight creates spurious highlight_vs_audio mismatches.
//
// Called from bumpParityGen after the parity generation advances. The function
// is currently a no-op for that reason; we keep it as a named seam so future
// pruning policies have an obvious place to land.
func (g *Game) clearParityStateForGenBump() {
	// Intentionally empty. See doc comment above.
}

// parityInGrace reports whether the post-mutation grace window is active.
// During grace, parityScan/parityCheck downgrade mismatches to log-only so
// the predictor / timeline / scheduler / audio buffers can reach coherence
// after a structural mutation without tripping the watchdog.
//
// The grace window protects against real-time races between the audio thread
// and the UI thread on a live system. Fast-path Go tests serialize all state
// synchronously and rely on strict immediate parity, so grace is bypassed
// under `go test`. Tests that explicitly want to verify grace behavior can
// inspect g.parityGraceUntilNS directly.
func (g *Game) parityInGrace() bool {
	if g == nil {
		return false
	}
	if runningUnderGoTest() {
		return false
	}
	until := g.parityGraceUntilNS.Load()
	if until <= 0 {
		return false
	}
	return time.Now().UnixNano() < until
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
		ParityGen:  g.parityGen.Load(),
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
	m := g.paritySeqDecisions[row]
	if m == nil {
		m = make(map[int]paritySeqDecision)
		g.paritySeqDecisions[row] = m
	}
	m[abs] = paritySeqDecision{
		Row:        row,
		Abs:        abs,
		Audible:    audible,
		Visible:    visible,
		NodeType:   typ,
		Missing:    missing,
		ParityGen:  g.parityGen.Load(),
		RecordedAt: time.Now(),
	}
	// Defense-in-depth: parityPruneAroundPlayhead runs deferred inside
	// parityScan, but the sequencer goroutine writes here independently. If
	// the UI thread stalls (slow draw) or parityScan stops being called, the
	// map could grow unbounded between prunes. Mirror parityAudioMax (line 150)
	// with a per-row sliding window keyed by abs.
	if len(m) > paritySeqDecisionsPerRowMax {
		minKeep := abs - paritySeqDecisionsPerRowMax + 1
		for k := range m {
			if k < minKeep {
				delete(m, k)
			}
		}
	}
}

// paritySeqDecisionsPerRowMax mirrors parityAudioMax (1024) at the parity-
// seq-decisions side. Both maps are keyed by absolute beat index and grow
// monotonically during playback; this constant is the per-row retention
// ceiling enforced inline in recordSeqDecision.
const paritySeqDecisionsPerRowMax = 1024

func (g *Game) parityPrune(minAbs int) {
	if g == nil {
		return
	}
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		return
	}
	g.parityPruneCallsForTest++
	if minAbs > g.parityPruneMaxMinAbsForTest {
		g.parityPruneMaxMinAbsForTest = minAbs
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
	if entry.GenAtScan == 0 {
		entry.GenAtScan = g.parityGen.Load()
	}
	// Within the post-mutation grace window, demote everything to log-only.
	// Real bugs that survive across the window (typically 80ms) will keep
	// firing after grace expires.
	if g.parityInGrace() {
		g.parityRing.add(entry)
		if g.logger != nil {
			g.logger.Debugf("[PARITY][grace][%s] row=%d abs=%d expected=%v actual=%v src=%s detail=%s",
				entry.Kind, entry.Row, entry.Abs, entry.Expected, entry.Actual, entry.Source, entry.Detail)
		}
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
	fmt.Fprintf(builder, " row=%d abs=%d source=%s kind=%s scheduled=%v slate=%v expected=%v actual=%v nodeType=%v missingInst=%v rowMuted=%v anySolo=%v when=%.6f detail=%s\n",
		e.Row, e.Abs, e.Source, e.Kind, e.Scheduled, e.Slate, e.Expected, e.Actual, e.NodeType, e.Missing, e.RowMuted, e.AnySolo, e.When, e.Detail)
	fmt.Fprintf(builder, " inWindow=%v offset=%d length=%d\n", e.InWindow, e.Offset, e.Length)
	// Transport snapshot
	ts := g.transportSnapshot()
	fmt.Fprintf(builder, " transport: playing=%v paused=%v bpm=%d beatBase=%.3f lastBeat=%.3f lastStep=%d pausedBeats=%d seekFreeze=%d\n",
		ts.Playing, ts.Paused, ts.AppliedBPM, ts.BeatBase, ts.LastBeat, ts.LastStep, ts.PausedBeats, ts.SeekFreezeFrames)
	// Scheduler indices
	fmt.Fprintf(builder, " elapsedBeats=%d offset=%d length=%d seqNextIdxs=%v nextBeatIdxs=%v\n",
		g.elapsedBeats, g.drum.Offset, g.drum.Length, g.seqNextIdxs, g.nextBeatIdxs)
	// Row window snapshot
	if e.Row >= 0 && e.Row < len(g.drum.Rows) {
		r := g.drum.Rows[e.Row]
		fmt.Fprintf(builder, " row steps=%v\n", boolSliceDebug(r.Steps))
		fmt.Fprintf(builder, " row types=%v\n", r.CellTypes)
	}
	// Predictor values at abs
	vis, trig := false, false
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.Ensure(e.Abs + 1)
		vis = g.engine.Predictor.VisibleAt(e.Row, e.Abs)
		trig = g.engine.Predictor.TriggeredAt(e.Row, e.Abs)
	}
	fmt.Fprintf(builder, " predictor visible=%v triggered=%v\n", vis, trig)
	// Timeline snapshot
	seg := g.TimelineSegments(e.Row)
	fmt.Fprintf(builder, " timeline offset=%d past=%v pastMask=%v present=%v future=%v\n",
		seg.Offset, boolSliceDebug(seg.Past), boolSliceDebug(seg.PastMask), boolSliceDebug(seg.Present), boolSliceDebug(seg.Future))
	// Graph summary
	fmt.Fprintf(builder, " graph start=%d nodes=%d edges=%d\n", g.graph.StartNodeID, len(g.graph.Nodes), len(g.graph.Edges))
	// Dump recent mismatches ring
	builder.WriteString(" recent mismatches:\n")
	for _, m := range g.parityRing.snapshot() {
		fmt.Fprintf(builder, "  row=%d abs=%d src=%s sched=%v slate=%v nodeType=%v\n", m.Row, m.Abs, m.Source, m.Scheduled, m.Slate, m.NodeType)
	}
	// Highlight state at abs
	if e.Row >= 0 {
		key := makeBeatKey(e.Row, e.Abs)
		g.highlightMu.RLock()
		until, hl := g.highlightedBeats[key]
		g.highlightMu.RUnlock()
		fmt.Fprintf(builder, " highlight key=%d present=%v until=%d frame=%d\n", key, hl, until, g.frame)
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
	if min := RuntimeProf().ParityScanMinPeriod; every < min {
		every = min
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
