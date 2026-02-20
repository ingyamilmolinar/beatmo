package engine

import (
	"sync/atomic"
	"testing"
	"time"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestEngineCloseStopsPredictorBackground(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	e := New(logger)
	if e.Predictor == nil {
		e.Predictor = NewPredictor(e.Graph, nil)
	}

	var calls int64
	e.Predictor.StartBackground(func() int {
		atomic.AddInt64(&calls, 1)
		// Keep the target modest; value doesn't matter for the test.
		return 512
	})

	// Allow a few ticks to accrue.
	time.Sleep(30 * time.Millisecond)
	before := atomic.LoadInt64(&calls)
	if before == 0 {
		t.Fatalf("expected background predictor to be active")
	}

	e.Close()
	// Give time for the goroutine to stop; further increments should cease.
	time.Sleep(30 * time.Millisecond)
	after := atomic.LoadInt64(&calls)
	if after != before {
		t.Fatalf("expected background predictor to stop after Close; got %d -> %d", before, after)
	}
}
