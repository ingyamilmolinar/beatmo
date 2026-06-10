package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// buildInflightHighlightGame constructs the scheduler→UI in-flight gap state:
// the sequencer has just scheduled beat abs (audio event recorded, seqNextIdxs
// advanced) but the matching highlight event is still in hlCh — the UI has not
// applied it yet (nextBeatIdxs[0] == abs, highlightedBeats empty). The audio
// clock is pinned so When is "due now" (no future-lead skip, clock
// initialized).
func buildInflightHighlightGame(t *testing.T, recordedAt time.Time) *Game {
	t.Helper()
	g := buildTestGame(t)
	t.Cleanup(g.CloseForTest)

	// Single-row playable circuit (same shape as the expiry-past test).
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.updateBeatInfos()

	g.drum.SetLength(128)
	g.drum.Offset = 64
	g.refreshDrumRow()
	g.ClearParityMismatches()

	const abs = 139
	g.SetPlaying(true)
	g.elapsedBeats = abs
	g.nextBeatIdxs = []int{abs}     // UI processed through abs-1; abs's highlight not applied yet
	g.seqNextIdxs = []int{abs + 1}  // sequencer already scheduled abs

	const eventWhen = 5.0
	restoreNow := audio.SetNowForTest(func() float64 { return eventWhen })
	t.Cleanup(restoreNow)

	g.parityMu.Lock()
	g.parityAudio = append(g.parityAudio, parityAudioEvent{
		Row:        0,
		Abs:        abs,
		When:       eventWhen,
		Inst:       "kick",
		Vol:        1.0,
		Dur:        1.0,
		Gen:        g.audioGen.Load(),
		ParityGen:  g.parityGen.Load(),
		RecordedAt: recordedAt,
	})
	if g.paritySeqDecisions[0] == nil {
		g.paritySeqDecisions[0] = make(map[int]paritySeqDecision)
	}
	g.paritySeqDecisions[0][abs] = paritySeqDecision{
		Row:        0,
		Abs:        abs,
		Audible:    true,
		Visible:    true,
		NodeType:   model.NodeTypeRegular,
		ParityGen:  g.parityGen.Load(),
		RecordedAt: recordedAt,
	}
	g.parityMu.Unlock()
	return g
}

// TestParityHighlightInFlightGraceFreshAudio reproduces the order-dependent
// TestTrackBeatConsumesCanonicalPlayhead crash from `make test-real`: the
// sequencer goroutine records the audio event for the current beat an instant
// before the UI drains the matching highlight from hlCh. A parityScan landing
// in that gap saw hasAudio && highlight-off and panicked with
// highlight_vs_audio, even though the highlight was simply still in flight.
//
// The fix mirrors the audio_missing check's RecordedAt grace: an audio event
// recorded <120ms ago is given time for its highlight to be applied before
// the mismatch fires.
func TestParityHighlightInFlightGraceFreshAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := buildInflightHighlightGame(t, time.Now()) // freshly recorded
	g.parityWatch = parityWatchPanic

	// Without the grace, parityScan panics with highlight_vs_audio here.
	g.parityScan("test-inflight-highlight")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			t.Fatalf("highlight_vs_audio fired for freshly-recorded audio whose highlight is still in flight: %+v", m)
		}
	}
}

// TestParityHighlightInFlightGraceExpired proves the grace does not gut the
// detector: the same state with a stale RecordedAt (well past the 120ms
// grace) is a genuine "audio fired but the UI never highlighted the beat"
// violation and must still be reported.
func TestParityHighlightInFlightGraceExpired(t *testing.T) {
	assertDefaultParityState(t)
	g := buildInflightHighlightGame(t, time.Now().Add(-500*time.Millisecond))
	g.parityWatch = parityWatchLog

	g.parityScan("test-stale-highlight")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			return // detector intact
		}
	}
	t.Fatalf("expected highlight_vs_audio for stale audio event with no highlight; got %+v", g.ParityMismatchSnapshot())
}
