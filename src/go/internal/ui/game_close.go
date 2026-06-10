package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// closeJoinTimeout caps how long Close() waits for bpmLoop/audioLoop to drain
// after their channels are closed. The loops typically return within ~1ms
// (their blocking points are the channel receives we just closed). A longer
// guard would only mask a genuine hang.
const closeJoinTimeout = 250 * time.Millisecond

// Close performs a full teardown of the game: stops background goroutines,
// closes channels, joins the goroutines, and releases the audio device. It
// is called on graceful shutdown (OS signals, window close, t.Cleanup) and
// is safe to call multiple times.
//
// Ordering: cancel/close upstream signals FIRST so receivers unblock, then
// Wait on the WaitGroup. Closing audio.* before the receivers exit would
// race the audioLoop's dispatch path.
func (g *Game) Close() {
	if g == nil || g.closed {
		return
	}
	// Stop audioLoop's opportunistic seqScheduleTime refills before any
	// channels go away so audioCh drains and the audioQuit select wins.
	g.closing.Store(true)
	g.StopBackgroundPredictorForTest()
	g.StopSequencerForTest()
	if g.audioScheduler != nil {
		_ = g.audioScheduler.Close()
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
	// audioCh is intentionally NOT closed: audioLoop writes back into it via
	// the opportunistic seqScheduleTime drive, and closing a channel that is
	// still being sent to panics. audioLoop exits on audioQuit instead.
	if g.audioQuit != nil {
		safeClose(g.audioQuit)
	}
	if g.bpmCh != nil {
		safeClose(g.bpmCh)
	}
	// Wait for audioLoop + bpmLoop to drain and return. A bounded wait
	// converts a "goleak fires at TestMain teardown" failure mode into a
	// loud, local "Close() timed out" log, which is far easier to bisect.
	done := make(chan struct{})
	go func() {
		g.bgWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeJoinTimeout):
		if g.logger != nil {
			g.logger.Errorf("[GAME] Close() join timed out after %s — background loops still parked", closeJoinTimeout)
		}
	}
	audio.Close()
	g.closed = true
}
