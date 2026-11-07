package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Audio recorded and seqNextIdxs advanced, the highlight still in hlCh; the clock is pinned so When is due now.
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
	g.nextBeatIdxs = []int{abs}    // UI processed through abs-1; abs's highlight not applied yet
	g.seqNextIdxs = []int{abs + 1} // sequencer already scheduled abs

	const eventWhen = 5.0
	restoreNow := audio.SetNowForTest(func() float64 { return eventWhen })
	t.Cleanup(restoreNow)

	g.parityMu.Lock()
	g.parityAudio = append(g.parityAudio, parityAudioEvent{
		Row:        0,
		Abs:        abs,
		When:       eventWhen,
		Inst:       roster.Kick,
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

// The sequencer can record a beat's audio an instant before the UI drains its highlight; like audio_missing, an
// event recorded within the grace window gets time for its highlight to land.
func TestParityHighlightInFlightGraceFreshAudio(t *testing.T) {
	assertDefaultParityState(t)
	// Pinned, so the event always reads as fresh: a GC pause or a loaded host could otherwise outlast the window.
	fixedNow := time.Now()
	restoreNow := SetParityRecordNowForTest(func() time.Time { return fixedNow })
	defer restoreNow()

	g := buildInflightHighlightGame(t, fixedNow) // freshly recorded (pinned)
	g.parityWatch = parityWatchPanic

	// Without the grace, parityScan panics with highlight_vs_audio here.
	g.parityScan("test-inflight-highlight")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "highlight_vs_audio" {
			t.Fatalf("highlight_vs_audio fired for freshly-recorded audio whose highlight is still in flight: %+v", m)
		}
	}
}

// The grace must not gut the detector: a stale RecordedAt is a real "audio fired, never highlighted".
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
