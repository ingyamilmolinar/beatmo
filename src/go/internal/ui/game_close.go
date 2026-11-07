package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The loops return within ~1 ms; a longer wait would only mask a real hang.
const closeJoinTimeout = 250 * time.Millisecond

// Idempotent. Signals close first so receivers unblock, then the loops are joined; closing audio
// first would race audioLoop's dispatch.
func (g *Game) Close() {
	if g == nil || g.closed {
		return
	}
	// Before any channel goes away, so audioCh drains and the audioQuit select wins.
	g.closing.Store(true)
	// Never leave committed lane voices playing after the Game is gone.
	if g.audioLaneOn() {
		g.resetAudioLane()
		g.flushLaneOps()
	}
	if g.themeCancel != nil {
		g.themeCancel()
		g.themeCancel = nil
	}
	if g.recordStopUnsub != nil {
		g.recordStopUnsub()
		g.recordStopUnsub = nil
	}
	// The locale registry is process-global; without this a closed Game stays reachable.
	if g.i18nCancel != nil {
		g.i18nCancel()
		g.i18nCancel = nil
	}
	unregisterGameForTest(g)
	// The palette is package state: tests build hundreds of Games per process.
	if !g.circuitTheme.IsEmpty() || !g.userTheme.Doc.IsEmpty() || !ActiveTheme().IsEmpty() {
		ApplyTheme(theme.Doc{})
	}
	g.StopBackgroundPredictorForTest()
	g.StopSequencerForTest()
	if g.audioScheduler != nil {
		_ = g.audioScheduler.Close()
	}
	if g.voicePrewarmer != nil {
		g.voicePrewarmer.release()
	}
	if g.engine != nil {
		g.engine.Close()
	}
	safeClose := func(ch interface{}) {
		defer func() { _ = recover() }()
		switch c := ch.(type) {
		case chan int:
			close(c)
		case chan struct{}:
			close(c)
		}
	}
	// audioCh stays open: audioLoop still sends into it. It exits on audioQuit.
	if g.audioQuit != nil {
		safeClose(g.audioQuit)
	}
	if g.bpmCh != nil {
		safeClose(g.bpmCh)
	}
	// A bounded wait turns a goleak failure at TestMain into a local, bisectable timeout.
	done := make(chan struct{})
	go func() {
		g.bgWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeJoinTimeout):
		if g.logger != nil {
			g.logger.Errorf("[game] Close() join timed out after %s — background loops still parked", closeJoinTimeout)
		}
	}
	audio.Close()
	g.closed = true
}
