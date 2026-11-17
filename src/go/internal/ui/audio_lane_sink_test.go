//go:build test

package ui

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// recordEngine captures the engine calls flushLaneOps makes, in order.
func recordEngine(t *testing.T, refuse map[uint64]bool) *[]string {
	t.Helper()
	var mu sync.Mutex
	log := &[]string{}
	add := func(s string) { mu.Lock(); *log = append(*log, s); mu.Unlock() }
	t.Cleanup(audio.SetPlayBatchHookForTest(func(b []audio.BatchParam) {
		var vids []string
		for _, p := range b {
			vids = append(vids, fmt.Sprint(p.VoiceID))
		}
		add("play[" + strings.Join(vids, ",") + "]")
	}))
	t.Cleanup(audio.SetLaneEngineHooksForTest(audio.LaneEngineHooks{
		Cancel: func(ids []uint64) (started []uint64) {
			add(fmt.Sprintf("cancel%v", ids))
			for _, id := range ids {
				if refuse[id] {
					started = append(started, id)
				}
			}
			return started
		},
		StopAt:    func(inst instrument.ID, tt float64) { add("stopAt " + string(inst)) },
		CancelAll: func() { add("cancelAll") },
	}))
	return log
}

// The production sink path (outboxSink → flushLaneOps → engine): ops apply in
// order, consecutive schedules batch into one PlayBatch, a run of
// replacements cancels its old voices in ONE call BEFORE scheduling, and a
// replacement whose old voice the engine refused (already started) is
// dropped — never the old and the new voice together.
func TestFlushLaneOpsAppliesOpsInOrderAndReplacesAtomically(t *testing.T) {
	g := newLaneCircuitGame(t)
	log := recordEngine(t, map[uint64]bool{3: true})
	n := laneNote{Row: 0, Abs: 1, Kind: laneSound, Inst: roster.Kick, Vol: 1, Dur: 1, Time: 2}
	s := g.laneSink
	s.Schedule(1, n)
	s.Schedule(5, n)
	s.Replace(1, 1, 2, n)
	s.Replace(3, 1, 4, n) // refused: 3 already started
	s.StopAt(roster.Kick, 2.5)
	s.Cancel([]uint64{5})
	s.CancelAllFuture()
	g.flushLaneOps()
	got := strings.Join(*log, " | ")
	want := "play[1,5] | cancel[1 3] | play[2] | stopAt kick | cancel[5] | cancelAll"
	if got != want {
		t.Fatalf("engine calls:\n got  %s\n want %s", got, want)
	}
}

// audioLoop and Close both flush; two concurrent flushes must not interleave
// (they share laneBatchScratch, and a taken schedule batch could land after
// Close's cancel-all).
func TestFlushLaneOpsIsSerialised(t *testing.T) {
	g := newLaneCircuitGame(t)
	recordEngine(t, nil)
	n := laneNote{Row: 0, Abs: 1, Kind: laneSound, Inst: roster.Kick, Vol: 1, Dur: 1, Time: 2}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				g.laneSink.Schedule(uint64(w*1000+i+1), n)
				g.flushLaneOps()
			}
		}(w)
	}
	wg.Wait()
}
