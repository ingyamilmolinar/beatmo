package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// Close performs a full teardown of the game: stops background goroutines,
// closes channels, and releases the audio device. It is called on graceful
// shutdown (OS signals, window close) and is safe to call multiple times.
func (g *Game) Close() {
	if g == nil || g.closed {
		return
	}
	g.StopBackgroundPredictorForTest()
	g.StopSequencerForTest()
	if g.audioScheduler != nil {
		_ = g.audioScheduler.Close()
	}
	if g.engine != nil {
		g.engine.Close()
	}
	audio.Close()
	safeClose := func(ch interface{}) {
		defer func() { _ = recover() }()
		switch c := ch.(type) {
		case chan soundReq:
			close(c)
		case chan int:
			close(c)
		case chan struct{}:
			close(c)
		}
	}
	if g.audioCh != nil {
		safeClose(g.audioCh)
	}
	if g.bpmCh != nil {
		safeClose(g.bpmCh)
	}
	g.closed = true
}
