//go:build test

package ui

import (
	"sync/atomic"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestPredictorBackgroundThrottle(t *testing.T) {
	assertDefaultPredictorThrottle(t)
	assertDefaultParityState(t)
	prev := predictorPerfThrottleEnabled
	predictorPerfThrottleEnabled = true
	defer func() { predictorPerfThrottleEnabled = prev }()

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	target := g.engine.Predictor.BackgroundTargetForTest()
	if target == nil {
		t.Fatalf("expected predictor background target function")
	}

	setDrawAvg := func(ms float64) {
		g.perf.reset()
		atomic.StoreInt64(&g.perf.frames, 1)
		atomic.StoreInt64(&g.perf.drawSumNS, int64(ms*1e6))
	}
	computeExpected := func(look int) int {
		need := g.drum.Offset + g.drum.Length
		if len(g.nextBeatIdxs) > 0 {
			for _, v := range g.nextBeatIdxs {
				if v+look > need {
					need = v + look
				}
			}
		}
		if need < g.drum.Length {
			need = g.drum.Length
		}
		return need + look
	}

	g.drum.Offset = 16
	g.drum.SetLength(64)
	g.nextBeatIdxs = []int{40}

	baseLook := g.grid.MaxDiv() * 16
	setDrawAvg(8.0)
	if got, want := target(), computeExpected(baseLook); got != want {
		t.Fatalf("baseline lookahead mismatch: got %d want %d", got, want)
	}

	setDrawAvg(12.0)
	if got, want := target(), computeExpected(baseLook/2); got != want {
		t.Fatalf("halved lookahead mismatch: got %d want %d", got, want)
	}

	setDrawAvg(16.0)
	if got, want := target(), computeExpected(baseLook/4); got != want {
		t.Fatalf("quarter lookahead mismatch: got %d want %d", got, want)
	}
}
