package ui

import (
	"fmt"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// diffPredictorTimeline reports positions where timeline commits disagree with
// predictor truth within the provided window. It is meant for diagnostics and
// parity logging; it does not mutate timeline state.
func (g *Game) diffPredictorTimeline(row, offset, length int, freezeLimit int) []mismatchEntry {
	if g == nil || g.drum == nil || row < 0 || row >= len(g.drum.Rows) || length <= 0 {
		return nil
	}
	end := offset + length
	if end < 0 {
		return nil
	}

	// Past boundary for this specific row. Timeline playback/import commits are
	// allowed to diverge from the current predictor after edits, so we never
	// treat abs < pastExclusive as a mismatch.
	pastExclusive := g.rowPastExclusive(row)
	var pred *engine.Predictor
	if g.engine != nil {
		pred = g.engine.Predictor
	}
	if pred != nil {
		pred.Ensure(end)
	}

	predVal := func(abs int, bi model.BeatInfo) bool {
		if pred == nil {
			return false
		}
		switch bi.NodeType {
		case model.NodeTypeRegular:
			return pred.VisibleAt(row, abs)
		case model.NodeTypeMute:
			return pred.TriggeredAt(row, abs)
		default:
			return false
		}
	}

	var mismatches []mismatchEntry
	for abs := offset; abs < end; abs++ {
		// Past playback history can diverge from the current predictor after edits.
		// Ignore indices already traversed by playback for this row.
		if pastExclusive > 0 && abs < pastExclusive {
			continue
		}
		// When the playhead boundary is unknown (e.g., stopped), fall back to the
		// frozen ceiling so we don't flag stale immutable history as a mismatch.
		if pastExclusive <= 0 && freezeLimit >= 0 && abs <= freezeLimit {
			continue
		}
		tVal, tTyp, kind, hasCommit := g.timelineCommittedWithKind(row, abs)
		if !hasCommit {
			continue
		}
		// Mutable/speculative commits are expected to diverge; only surface
		// immutable playback/import mismatches.
		if kind != timeline.CommitKindPlayback && kind != timeline.CommitKindImport {
			continue
		}
		bi := g.beatInfoAtRow(row, abs)
		pVal := predVal(abs, bi)
		if pVal == tVal {
			continue
		}
		mismatches = append(mismatches, mismatchEntry{
			Row:      row,
			Abs:      abs,
			Kind:     "timeline_vs_pred",
			NodeType: bi.NodeType,
			Expected: pVal,
			Actual:   tVal,
			Source:   "timeline",
			Offset:   offset,
			Length:   length,
			InWindow: abs >= offset && abs < end,
			Detail:   fmt.Sprintf("commitKind=%v commitTyp=%v biTyp=%v pred=%v commit=%v", kind, tTyp, bi.NodeType, pVal, tVal),
		})
	}
	return mismatches
}

// reconcileMutableTimeline aligns non-immutable timeline commits (seeded / gap-pad)
// with the current predictor snapshot so live edits while paused do not leave
// stale masks that trigger parity mismatches once logging is enabled.
//
// Released commits are treated as bookkeeping and are intentionally not rewritten
// so demoted immutable leaks remain debuggable.
func (g *Game) reconcileMutableTimeline(row, offset, length int, predictAt func(int, model.BeatInfo) bool) {
	if g == nil || predictAt == nil || row < 0 || row >= len(g.beatInfosByRow) {
		return
	}
	end := offset + length
	for abs := offset; abs < end; abs++ {
		val, typ, kind, ok := g.timelineCommittedWithKind(row, abs)
		if !ok {
			continue
		}
		if kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport || kind == timeline.CommitKindReleased {
			continue
		}
		bi := g.beatInfoAtRow(row, abs)
		want := predictAt(abs, bi)
		// Avoid churn when both value and type already match the predictor view.
		if val == want && typ == bi.NodeType {
			continue
		}
		g.timelineReplaceCommit(row, abs, want, bi.NodeType, timeline.CommitKindReleased)
	}
}

// parityScan inspects view, predictor, audio events, and graph/timeline to
// detect mismatches in real time.
func (g *Game) parityScan(reason string) {
	if g == nil {
		return
	}
	g.parityScanCallsForTest++
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		g.parityScanReturnsForTest[0]++
		return
	}
	// Retention runs regardless of the comparator early-returns below: the
	// comparator can be silenced for legitimate UI-state reasons (origin
	// selection mask, import in progress, render not yet ready) without
	// stopping unbounded growth of paritySeqDecisions / parityAudio. Without
	// this, programmatic origin assignments (import, scene-build flows that
	// don't go through click-to-set-origin) leave pendingStartRow >= 0
	// indefinitely, the comparator skips, and the maps fill until WASM OOMs
	// (production crash stack: RowRackZone.drawRowControlsToCache → vector
	// path tessellation can no longer allocate at the 2 GB ceiling).
	defer g.parityPruneAroundPlayhead()
	if g.importDialog || g.importing || (g.drum != nil && (g.drum.importing || g.drum.lengthChanging)) {
		g.parityScanReturnsForTest[1]++
		return
	}
	// During origin selection the UI intentionally greys/overlays circuits; skip parity
	// to avoid flagging those transient masks.
	if g.pendingStartRow >= 0 {
		g.parityScanReturnsForTest[2]++
		return
	}
	if !g.Playing() && !g.state.Paused() {
		// If parity is effectively disabled, skip scans entirely when stopped.
		if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
			g.parityScanReturnsForTest[3]++
			return
		}
		// Allow scans in tests only when an explicit watcher is enabled.
		if g.parityWatch == parityWatchOff && runningUnderGoTest() {
			g.parityScanReturnsForTest[4]++
			return
		}
		// In runtime, skip when stopped.
		if !runningUnderGoTest() {
			g.parityScanReturnsForTest[5]++
			return
		}
	}
	if g.drum == nil || !g.renderReady {
		g.parityScanReturnsForTest[6]++
		return
	}
	// When not playing, skip highlight parity; only the playing path is relevant.
	if !g.Playing() && reason == "refresh" {
		g.parityScanReturnsForTest[7]++
		return
	}
	every, stride := g.parityScanSettings()
	phase := 0
	if reason == "refresh" {
		if !g.parityScanDue(every) {
			g.parityScanReturnsForTest[8]++
			return
		}
		if stride > 1 {
			phase = g.parityScanPhase % stride
			g.parityScanPhase = (phase + 1) % stride
		} else {
			stride = 1
		}
		g.parityScanLastFrame = g.frame
	} else {
		stride = 1
	}
	start := time.Now()
	defer func() { g.recordParityScan(time.Since(start)) }()
	viewOffset := g.renderOffset
	viewLength := g.renderLength
	if viewLength <= 0 {
		g.parityScanReturnsForTest[9]++
		return
	}
	look := g.grid.MaxDiv()
	if look <= 0 {
		look = 1
	}
	lookahead := look * 4 // 4 beats in subdivisions
	if g.parityWatch == parityWatchLog && !parityFatalEnabled.Load() && !runningUnderGoTest() {
		// Reduce scan width when logging only to keep parity overhead small.
		if lookahead > look {
			lookahead = look
		}
	}
	anySolo := false
	for _, r := range g.drum.Rows {
		if r.Solo {
			anySolo = true
			break
		}
	}

	// Scan view/predictor parity near the rendered window, but scan audio/seq
	// parity around the global playhead so panning the DrumView doesn't silence
	// audio mismatch detection.
	playhead := g.globalPastExclusive() - 1
	if playhead < 0 {
		playhead = 0
	}
	audioStart := playhead - lookahead
	if audioStart < 0 {
		audioStart = 0
	}
	audioEnd := playhead + lookahead + 1
	if audioEnd < 0 {
		audioEnd = 0
	}

	curParityGen := g.parityGen.Load()
	g.parityMu.Lock()
	audioEvents := append([]parityAudioEvent{}, g.parityAudio...)
	seqCopy := make(map[int]map[int]paritySeqDecision, len(g.paritySeqDecisions))
	for r, m := range g.paritySeqDecisions {
		rowCopy := make(map[int]paritySeqDecision, len(m))
		for k, v := range m {
			if k < audioStart || k >= audioEnd {
				continue
			}
			// Generation filter (SYMMETRIC with the audio side below): a
			// decision recorded before the latest structural mutation belongs
			// to a prior parity generation and must never be compared against
			// the post-mutation predictor — that comparison is the live-edit
			// false positive. Dropping it here (and the matching audio event
			// below) keeps both sides of every comparison in the same
			// generation. The sequencer re-records current/future beats under
			// the new generation, so detection resumes within a frame.
			if v.ParityGen != curParityGen {
				continue
			}
			rowCopy[k] = v
		}
		if len(rowCopy) > 0 {
			seqCopy[r] = rowCopy
		}
	}
	g.parityMu.Unlock()

	eventsByRow := make(map[int]map[int]parityAudioEvent)
	curGen := g.audioGen.Load()
	for _, ev := range audioEvents {
		// audioGen filter: drop in-flight notes from a prior audio run
		// (stop/replay). parityGen filter: drop events from a prior structural
		// generation (live edit), symmetric with the seq-decision filter above.
		// Both sides of every comparison are now guaranteed same-generation.
		if ev.Gen != curGen || ev.ParityGen != curParityGen {
			continue
		}
		if ev.Abs < audioStart || ev.Abs >= audioEnd {
			continue
		}
		if eventsByRow[ev.Row] == nil {
			eventsByRow[ev.Row] = make(map[int]parityAudioEvent)
		}
		eventsByRow[ev.Row][ev.Abs] = ev
	}

	var pred *engine.Predictor
	if g.engine != nil {
		pred = g.engine.Predictor
	}
	for row := range g.drum.Rows {
		// True past boundary is row-specific but must never exceed the global
		// wall-clock playhead; otherwise stale commits can mask future edits.
		pastExclusive := g.rowPastExclusive(row)
		rowEvents := eventsByRow[row]
		decRow := seqCopy[row]
		hasSeq := len(decRow) > 0
		hasAudioEvents := len(rowEvents) > 0
		rowMuted := g.drum.Rows[row].Muted
		soloGated := anySolo && !g.drum.Rows[row].Solo
		rowViewOffset := viewOffset
		rowViewLength := viewLength
		cacheOK := false
		cacheFresh := false
		if off, ln, ok := g.drum.cachedRowWindow(row); ok {
			rowViewOffset = off
			rowViewLength = ln
			cacheOK = true
			if row < len(g.drum.rowCacheSig) && row < len(g.rowRenderSig) {
				cacheFresh = g.drum.rowCacheSig[row] == g.rowRenderSig[row]
			}
		}
		if rowViewLength <= 0 {
			continue
		}
		rowViewStart := rowViewOffset - lookahead
		if rowViewStart < 0 {
			rowViewStart = 0
		}
		rowViewEnd := rowViewOffset + rowViewLength + lookahead
		if pred != nil {
			horizon := rowViewEnd
			if audioEnd > horizon {
				horizon = audioEnd
			}
			pred.Ensure(horizon)
		}
		cacheUse := cacheOK
		slateDetail := ""
		if cacheOK {
			if cacheFresh {
				slateDetail = "slate=cache"
			} else {
				slateDetail = "slate=cache-stale"
			}
		}
		slateAt := func(abs int) (bool, bool) {
			inWindow := abs >= rowViewOffset && abs < rowViewOffset+rowViewLength
			if !inWindow {
				return false, false
			}
			if cacheUse {
				if v, ok := g.drum.cachedRowState(row, abs); ok {
					return v, true
				}
			}
			rel := abs - rowViewOffset
			if rel >= 0 && rel < len(g.drum.Rows[row].Steps) {
				return g.drum.Rows[row].Steps[rel], true
			}
			return false, false
		}

		// View/predictor parity: only within the rendered window and nearby padding.
		viewStride := stride
		viewPhase := phase
		startAbs := rowViewStart
		if viewStride > 1 {
			rem := startAbs % viewStride
			if rem < 0 {
				rem += viewStride
			}
			if rem != viewPhase {
				delta := viewPhase - rem
				if delta < 0 {
					delta += viewStride
				}
				startAbs += delta
			}
		}
		for abs := startAbs; abs < rowViewEnd; abs += viewStride {
			inPast := abs < pastExclusive
			info := g.beatInfoAtRow(row, abs)
			typ := info.NodeType
			// Compute view truth (what DrumView renders) from the engine predictor.
			// This differs from audio truth for some policies (e.g., mute nodes),
			// so keep view parity separate from audio parity checks.
			viewExpected := false
			if pred != nil {
				if info.NodeType == model.NodeTypeMute {
					viewExpected = pred.TriggeredAt(row, abs)
				} else {
					viewExpected = pred.VisibleAt(row, abs)
				}
			}
			slate, inWindow := slateAt(abs)
			if inWindow && !inPast && slate != viewExpected {
				g.parityReport(mismatchEntry{
					Row:      row,
					Abs:      abs,
					Kind:     "view_vs_truth",
					Expected: viewExpected,
					Actual:   slate,
					NodeType: typ,
					Source:   reason,
					Offset:   rowViewOffset,
					Length:   rowViewLength,
					InWindow: true,
					Detail:   slateDetail,
				})
			}
			// Skip cell-type parity for now; view_vs_truth already covers functional mismatches.
			// Detect immutable timeline commits that disagree with predictor truth.
			if v, ctyp, kind, ok := g.timelineCommittedWithKind(row, abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
				// Past playback/import entries are allowed to diverge from the current
				// predictor after edits; only check for "leaked" immutable commits
				// at/after the current past boundary.
				if abs < pastExclusive {
					continue
				}
				if viewExpected != v {
					g.parityReport(mismatchEntry{
						Row:      row,
						Abs:      abs,
						Kind:     "timeline_vs_pred_immutable",
						Expected: viewExpected,
						Actual:   v,
						NodeType: ctyp,
						Source:   reason,
						Offset:   rowViewOffset,
						Length:   rowViewLength,
						InWindow: abs >= rowViewOffset && abs < rowViewOffset+rowViewLength,
						Detail:   fmt.Sprintf("kind=%v", kind),
					})
				}
			}
		}

		// Audio/seq/highlight parity: anchored near the playhead so it fires even
		// when DrumView is panned away.
		if !hasSeq && !hasAudioEvents {
			continue
		}
		missingFloor := pastExclusive - 2
		if missingFloor < 0 {
			missingFloor = 0
		}
		audioFloor := pastExclusive - 1
		if audioFloor < 0 {
			audioFloor = 0
		}
		for abs := audioStart; abs < audioEnd; abs++ {
			info := g.beatInfoAtRow(row, abs)
			typ := info.NodeType
			audioExpected := false
			if pred != nil {
				if info.NodeType == model.NodeTypeMute {
					audioExpected = pred.TriggeredAt(row, abs)
				} else {
					audioExpected = pred.AudibleAt(row, abs)
				}
			}
			slate, inWindow := slateAt(abs)
			// Audio/seq/highlight parity only makes sense near the current playhead.
			// For missing-audio checks we keep one beat of slack behind the current
			// beat so the 120ms grace window can't "age out" before we check.
			if abs < missingFloor {
				continue
			}
			hasAudio := false
			var ev parityAudioEvent
			if rowEvents != nil {
				ev, hasAudio = rowEvents[abs]
			}
			if dec, ok2 := decRow[abs]; ok2 {
				// dec.Enqueued gates this check: a decision whose audio the
				// scheduler already handed to the audio pipeline is NOT a
				// missing-audio violation even when no parityAudio event has
				// been recorded yet — the audio is simply still in-flight in
				// audioCh (audioLoop briefly behind), or was dropped by the
				// audio thread under backpressure / a transport transition.
				// audio_missing only flags the genuine scheduler bug: an
				// audible decision whose audio was never enqueued at all
				// (Enqueued=false). Production couples decision-record and
				// enqueue in one seqMu critical section, so Audible⟹Enqueued
				// and this check never false-positives on pipeline latency.
				if dec.Audible && !dec.Missing && !dec.Enqueued && !hasAudio && typ == model.NodeTypeRegular {
					// Avoid racey false-positives: if the sequencer just
					// recorded the decision but hasn't yet logged the audio
					// event into parityAudio, allow a small grace window.
					if time.Since(dec.RecordedAt) < 120*time.Millisecond {
						continue
					}
					g.parityReport(mismatchEntry{
						Row:      row,
						Abs:      abs,
						Kind:     "audio_missing",
						Expected: true,
						Actual:   false,
						NodeType: typ,
						Source:   reason,
						Offset:   rowViewOffset,
						Length:   rowViewLength,
						InWindow: inWindow,
					})
				}
			}
			// Skip the remaining audio/seq/highlight checks for beats strictly older
			// than the current beat (abs < pastExclusive-1). The current beat
			// (abs == pastExclusive-1) is where audio↔predictor/view mismatches must
			// be caught.
			if abs < audioFloor {
				continue
			}
			if inWindow {
				if dec, ok2 := decRow[abs]; ok2 {
					if !dec.Missing && !rowMuted && !soloGated {
						// Use dec.Visible (view truth) for seq_vs_view comparison,
						// not dec.Audible (audio truth). Mute gates make nodes
						// inaudible but still visible in the UI.
						//
						// The `abs == seqNextIdxs[row]-1` clause is the
						// just-scheduled-beat grace: that beat's slate will be
						// rendered on the next refresh, so a visible-decision /
						// not-yet-slated disagreement there is a one-frame
						// pipeline lag, not a desync. This grace now holds in
						// EVERY mode (including fatal) — previously a leading
						// `fatalNow ||` bypassed it exactly in the panic mode,
						// crashing on the benign lag.
						if !dec.Visible || slate || row < 0 || row >= len(g.seqNextIdxs) || abs != g.seqNextIdxs[row]-1 {
							if dec.Visible != slate {
								g.parityReport(mismatchEntry{
									Row:       row,
									Abs:       abs,
									Kind:      "seq_vs_view",
									NodeType:  dec.NodeType,
									Scheduled: dec.Visible,
									Slate:     slate,
									Expected:  dec.Visible,
									Actual:    slate,
									Source:    reason,
									Offset:    rowViewOffset,
									Length:    rowViewLength,
									InWindow:  true,
									RowMuted:  rowMuted,
									AnySolo:   anySolo,
									Missing:   dec.Missing,
									Detail:    slateDetail,
								})
							}
						}
					}
				}
			}
			if hasAudio && !audioExpected {
				g.parityReport(mismatchEntry{
					Row:      row,
					Abs:      abs,
					Kind:     "audio_unexpected",
					Expected: false,
					Actual:   true,
					NodeType: typ,
					Source:   reason,
					Offset:   rowViewOffset,
					Length:   rowViewLength,
					InWindow: inWindow,
					When:     ev.When,
				})
			}
			if hasAudio && inWindow && !slate {
				g.parityReport(mismatchEntry{
					Row:      row,
					Abs:      abs,
					Kind:     "audio_vs_view",
					Expected: true,
					Actual:   slate,
					NodeType: typ,
					Source:   reason,
					Offset:   rowViewOffset,
					Length:   rowViewLength,
					InWindow: true,
					Detail:   slateDetail,
					When:     ev.When,
				})
			}
			if hasAudio && inWindow {
				skip := false
				if !g.rowIsAudible(row) {
					skip = true
				}
				// Beats the UI playhead has already advanced past are not
				// highlight-parity violations — applySequencerHighlight (and
				// the syncUIToTime catch-up / seek paths) set
				// nextBeatIdxs[row] = abs+1 when they process beat abs, and
				// the highlight then expires by design (highlightedBeats
				// entries carry a frame deadline). Comparing a recorded audio
				// event against a naturally-expired highlight is a false
				// positive; this can happen even at abs == audioFloor (the
				// "current" beat per the global wall-clock clamp) when the
				// beat outlasts the highlight window. A genuine violation —
				// audio fired but the UI never highlighted the beat — leaves
				// nextBeatIdxs[row] <= abs and is still reported below.
				if !skip && row < len(g.nextBeatIdxs) && g.nextBeatIdxs[row] > abs {
					skip = true
				}
				// Freshly-recorded audio events get a grace window before the
				// highlight is required: the sequencer goroutine records the
				// event and emits the matching highlight to hlCh, but the UI
				// only applies it on the next Update drain. A scan landing in
				// that gap is a pipeline-latency artifact, not a violation —
				// mirrors the audio_missing RecordedAt grace above. The
				// When-now > 0.5 future skip below does not cover this:
				// tight-lead scheduling (fast-forward drives, small lookahead)
				// records When ≈ now.
				if !skip && time.Since(ev.RecordedAt) < 120*time.Millisecond {
					skip = true
				}
				now := audio.Now()
				if !skip && ev.When > 0 {
					if now > 0 && ev.When-now > 0.5 {
						// Scheduled sufficiently in the future; allow highlight to be applied closer to playback.
						skip = true
					}
					if now == 0 {
						// Audio clock not initialized yet; defer highlight parity until it is.
						skip = true
					}
				}
				// A highlight the sequencer dropped (hlCh full under load) is
				// never painted by the UI, so "audio fired but no highlight" is
				// an expected cosmetic loss, not an audio/UI desync. Exempt it.
				if !skip && g.highlightWasDropped(row, abs) {
					skip = true
				}
				if !skip {
					key := makeBeatKey(row, abs)
					g.highlightMu.RLock()
					hlUntil, okHL := g.highlightedBeats[key]
					g.highlightMu.RUnlock()
					on := okHL && hlUntil > g.frame
					if !on {
						g.parityReport(mismatchEntry{
							Row:      row,
							Abs:      abs,
							Kind:     "highlight_vs_audio",
							Expected: true,
							Actual:   false,
							NodeType: typ,
							Source:   reason,
							Offset:   rowViewOffset,
							Length:   rowViewLength,
							InWindow: true,
							When:     ev.When,
						})
					}
				}
			}
			if hasAudio {
				if dec, ok2 := decRow[abs]; !ok2 || !dec.Audible {
					g.parityReport(mismatchEntry{
						Row:      row,
						Abs:      abs,
						Kind:     "audio_vs_seq",
						Expected: true,
						Actual:   false,
						NodeType: typ,
						Source:   reason,
						Offset:   rowViewOffset,
						Length:   rowViewLength,
						InWindow: inWindow,
						When:     ev.When,
					})
				}
			}
		}
	}
	// Prune handled by deferred parityPruneAroundPlayhead at function entry.
}

// parityPruneAroundPlayhead computes the playhead-relative window used by
// parityScan and calls parityPrune to bound paritySeqDecisions / parityAudio.
// It is called as a deferred function from parityScan so retention runs even
// when the comparator early-returns (origin selection, import in progress,
// renderReady=false, etc).
func (g *Game) parityPruneAroundPlayhead() {
	if g == nil || g.grid == nil {
		return
	}
	look := g.grid.MaxDiv()
	if look <= 0 {
		look = 1
	}
	lookahead := look * 4
	playhead := g.globalPastExclusive() - 1
	if playhead < 0 {
		playhead = 0
	}
	audioStart := playhead - lookahead
	if audioStart < 0 {
		audioStart = 0
	}
	g.parityPrune(audioStart)
}
