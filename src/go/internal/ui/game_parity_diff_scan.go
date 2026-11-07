package ui

import (
	"fmt"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

func (g *Game) diffPredictorTimeline(row, offset, length int, freezeLimit int) []mismatchEntry {
	if g == nil || g.drum == nil || row < 0 || row >= len(g.drum.Rows) || length <= 0 {
		return nil
	}
	end := offset + length
	if end < 0 {
		return nil
	}

	// Commits before this boundary may legitimately diverge after an edit.
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
		case model.NodeTypeRegular, model.NodeTypeChord:
			return pred.VisibleAt(row, abs)
		case model.NodeTypeMute:
			return pred.TriggeredAt(row, abs)
		default:
			return false
		}
	}

	var mismatches []mismatchEntry
	for abs := offset; abs < end; abs++ {
		if pastExclusive > 0 && abs < pastExclusive {
			continue
		}
		// With no playhead (stopped), the frozen ceiling keeps stale history from being flagged.
		if pastExclusive <= 0 && freezeLimit >= 0 && abs <= freezeLimit {
			continue
		}
		tVal, tTyp, kind, hasCommit := g.timelineCommittedWithKind(row, abs)
		if !hasCommit {
			continue
		}
		// Only immutable playback/import commits are compared; speculative ones are expected to diverge.
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

// So live edits while paused leave no stale masks. Released commits are left alone so demoted
// immutable leaks stay debuggable.
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

func (g *Game) parityScan(reason string) {
	if g == nil {
		return
	}
	g.parityScanCallsForTest++
	if g.parityWatch == parityWatchOff && !parityFatalEnabled.Load() {
		g.parityScanReturnsForTest[0]++
		return
	}
	// Retention runs even when the comparator returns early: the maps would otherwise grow without
	// bound (an import that leaves pendingStartRow set ran WASM out of memory).
	defer g.parityPruneAroundPlayhead()
	if g.importDialog || g.importing || (g.drum != nil && (g.drum.importing || g.drum.lengthChanging)) {
		g.parityScanReturnsForTest[1]++
		return
	}
	// Origin selection masks circuits on purpose.
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
		// Skipped when stopped under go test too, so a test checks exactly what production checks.
		if !g.parityScanStoppedForTest {
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

	// Audio/seq parity centres on the playhead, so panning the drum view never silences it.
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
			// A decision from an earlier generation is never compared with the post-mutation predictor (the
			// live-edit false positive); the audio side filters the same way, so both sides always match.
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
		// audioGen drops a previous run's notes, parityGen a previous structural generation.
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
		// Never past the global playhead, or stale commits mask future edits.
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
			// View truth differs from audio truth for some policies (mute nodes), so they are checked apart.
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
			// Cell types are not compared: view_vs_truth covers functional mismatches.
			if v, ctyp, kind, ok := g.timelineCommittedWithKind(row, abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
				// Only commits at or after the past boundary can be leaks.
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
			// One beat of slack behind the current beat, so the 120 ms grace cannot age out first.
			if abs < missingFloor {
				continue
			}
			hasAudio := false
			var ev parityAudioEvent
			if rowEvents != nil {
				ev, hasAudio = rowEvents[abs]
			}
			if dec, ok2 := decRow[abs]; ok2 {
				// A decision already handed to the pipeline is not missing audio, only in flight or dropped
				// downstream. Record and enqueue share one seqMu section, so this never fires on latency.
				if dec.Audible && !dec.Missing && !dec.Enqueued && !hasAudio && typ.Fires() {
					// The event may not be logged yet.
					if parityRecordNow().Sub(dec.RecordedAt) < 120*time.Millisecond {
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
			// The current beat (pastExclusive-1) is where audio mismatches must be caught.
			if abs < audioFloor {
				continue
			}
			g.parityCheckSeqVsView(row, abs, decRow, slate, inWindow, rowMuted, soloGated, anySolo, reason, slateDetail, rowViewOffset, rowViewLength)
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
			g.parityCheckHighlightVsAudio(row, abs, typ, ev, hasAudio, inWindow, reason, rowViewOffset, rowViewLength)
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

// Deferred from parityScan, so retention runs even when the comparator returns early.
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

func (g *Game) parityCheckSeqVsView(row, abs int, decRow map[int]paritySeqDecision, slate, inWindow, rowMuted, soloGated, anySolo bool, reason, slateDetail string, rowViewOffset, rowViewLength int) {
	if !inWindow {
		return
	}
	dec, ok := decRow[abs]
	if !ok {
		return
	}
	// dec.Visible, not dec.Audible: mute gates make nodes silent but still visible.
	if dec.Missing || rowMuted || soloGated {
		return
	}
	// The just-scheduled beat is a one-frame lag in every mode, fatal included.
	if dec.Visible && !slate && row >= 0 && row < len(g.seqNextIdxs) && abs == g.seqNextIdxs[row]-1 {
		return
	}
	if dec.Visible == slate {
		return
	}
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

func (g *Game) parityCheckHighlightVsAudio(row, abs int, typ model.NodeType, ev parityAudioEvent, hasAudio, inWindow bool, reason string, rowViewOffset, rowViewLength int) {
	if !hasAudio || !inWindow {
		return
	}
	if !g.rowIsAudible(row) {
		return
	}
	// A beat the UI playhead has passed had its highlight expire by design; a real violation leaves
	// nextBeatIdxs[row] <= abs.
	if row < len(g.nextBeatIdxs) && g.nextBeatIdxs[row] > abs {
		return
	}
	// The UI applies the highlight on the next Update drain, so a scan in that gap is latency.
	// Tight-lead scheduling records When ≈ now, which the future skip below does not cover.
	if parityRecordNow().Sub(ev.RecordedAt) < 120*time.Millisecond {
		return
	}
	now := audio.Now()
	if ev.When > 0 {
		if now > 0 && ev.When-now > 0.5 {
			return
		}
		// Audio clock not initialized yet; defer highlight parity until it is.
		if now == 0 {
			return
		}
	}
	// A highlight dropped because hlCh was full is an expected cosmetic loss.
	if g.highlightWasDropped(row, abs) {
		return
	}
	key := makeBeatKey(row, abs)
	g.highlightMu.RLock()
	hlUntil, okHL := g.highlightedBeats[key]
	g.highlightMu.RUnlock()
	if okHL && hlUntil > g.frame {
		return
	}
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
