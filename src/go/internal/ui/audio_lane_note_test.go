//go:build test

package ui

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// laneClocks maps a lane test game to the fake audio clock installed for it.
var laneClocks sync.Map // *Game → *float64

// laneClockMu guards writes to a Game lane test clock against the engine
// goroutines (audioLoop, sequencerLoop) reading it through audio.Now.
var laneClockMu sync.RWMutex

// advanceLaneClock moves a Game lane test clock forward by d seconds.
func advanceLaneClock(c *float64, d float64) {
	laneClockMu.Lock()
	*c += d
	laneClockMu.Unlock()
}

// laneTestNow returns the fake audio clock driving g (seconds).
func laneTestNow(g *Game) *float64 {
	v, _ := laneClocks.Load(g)
	return v.(*float64)
}

// newLaneCircuitGame builds one looping row of 8 regular nodes, playing:
//
//	n2: pitch +3, volume 0.5     n3: MUTE node
//	n5: probability 0.5 (deterministic roll)
//
// with a fake audio clock (laneTestNow) starting at 1.0 s.
func newLaneCircuitGame(t *testing.T) *Game {
	t.Helper()
	g := buildTestGame(t)
	g.pendingStartRow = -1
	var nodes []*uiNode
	for i := 0; i < 8; i++ {
		typ := model.NodeTypeRegular
		if i == 3 {
			typ = model.NodeTypeMute
		}
		n := ensureNode(g, i, 0, typ)
		if n == nil {
			t.Fatalf("node %d not created", i)
		}
		nodes = append(nodes, n)
	}
	for i := 0; i < 8; i++ {
		g.addEdge(nodes[i], nodes[(i+1)%8])
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.graph.SetNodeParams(nodes[2].ID, model.NodeParams{Volume: 0.5, Pitch: 3, Duration: 1})
	g.graph.SetNodeParams(nodes[5].ID, model.NodeParams{Volume: 1, Duration: 1, LogicKind: model.LogicProbability, LogicP: 0.5})
	g.updateBeatInfos()
	clock := new(float64)
	*clock = 1.0
	laneClocks.Store(g, clock)
	t.Cleanup(func() { laneClocks.Delete(g) })
	restore := audio.SetNowForTest(func() float64 {
		laneClockMu.RLock() // audioLoop / sequencer goroutines read it too
		defer laneClockMu.RUnlock()
		return *clock
	})
	t.Cleanup(restore)
	g.SetPlayingForTest(true)
	g.state.StartClock(time.Now(), *clock) // audio-clock anchor: the lane schedules on it
	return g
}

// captureLegacyDispatch steps the LEGACY sequencer beat by beat through the
// real scheduleSound → audioCh → audioLoop → audio.PlayBatch path and
// reports every sequencer note it dispatched with its full parameters.
func captureLegacyDispatch(t *testing.T, g *Game, steps int, fn func(row, abs int, inst string, vol, pitch float64)) {
	t.Helper()
	var mu sync.Mutex
	var got []audio.BatchParam
	restore := audio.SetPlayBatchHookForTest(func(b []audio.BatchParam) {
		mu.Lock()
		got = append(got, b...)
		mu.Unlock()
	})
	defer restore()
	prevHook := g.scheduleHook
	g.scheduleHook = func(int, int) {} // route through scheduleSound/audioCh, not the direct-play test branch
	defer func() { g.scheduleHook = prevHook }()
	for abs := 0; abs < steps; abs++ {
		scheduleAbsForTest(g, abs)
	}
	// audioLoop drains asynchronously: wait until the capture stops growing.
	last, stable := -1, time.Now()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n != last {
			last, stable = n, time.Now()
		} else if time.Since(stable) > 150*time.Millisecond {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, b := range got {
		if b.Row >= 0 {
			fn(b.Row, b.Abs, string(b.ID), b.Vol, b.Pitch)
		}
	}
}

// laneNoteAt (the audio lane's PURE note function) must agree with what the
// legacy stateful sequencer actually dispatches: same cells sound, same
// instrument, volume and pitch — including a mute node, a probability node
// and per-node params.
func TestLaneNoteAtMatchesLegacyDispatch(t *testing.T) {
	g := newLaneCircuitGame(t)
	type got struct {
		inst       string
		vol, pitch float64
	}
	legacy := map[[2]int]got{}
	captureLegacyDispatch(t, g, 64, func(row, abs int, inst string, vol, pitch float64) {
		legacy[[2]int{row, abs}] = got{inst, vol, pitch}
	})
	if len(legacy) < 20 {
		t.Fatalf("legacy oracle captured only %d notes — the capture is broken", len(legacy))
	}
	c, ok := g.playbackSlate.laneContext()
	if !ok {
		t.Fatal("no lane context while playing")
	}
	sounds := 0
	for abs := 0; abs < 64; abs++ {
		n, ok := g.playbackSlate.laneNoteAt(&c, 0, abs)
		isSound := ok && n.Kind == laneSound
		want, played := legacy[[2]int{0, abs}]
		if played != isSound {
			t.Fatalf("(0,%d): legacy played=%v lane sound=%v (%+v)", abs, played, isSound, n)
		}
		if !played {
			continue
		}
		sounds++
		if n.Inst != instrument.ID(want.inst) || math.Abs(n.Vol-want.vol) > 1e-12 || math.Abs(n.Pitch-want.pitch) > 1e-12 {
			t.Fatalf("(0,%d): lane %+v vs legacy %+v", abs, n, want)
		}
	}
	if sounds == 0 {
		t.Fatal("no sounding cells compared")
	}
	// The mute node's cell is a laneStop for the row's instrument.
	if n, ok := g.playbackSlate.laneNoteAt(&c, 0, 3); !ok || n.Kind != laneStop || n.Inst != g.drum.Rows[0].Instrument {
		t.Fatalf("mute cell: %+v ok=%v", n, ok)
	}
	// Same inputs → same Hash (purity); a changed input → a different Hash.
	a, _ := g.playbackSlate.laneNoteAt(&c, 0, 2)
	b, _ := g.playbackSlate.laneNoteAt(&c, 0, 2)
	if a.Hash != b.Hash {
		t.Fatal("laneNoteAt is not pure: same cell, different Hash")
	}
	g.drum.Rows[0].Volume = 0.3
	if d, _ := g.playbackSlate.laneNoteAt(&c, 0, 2); d.Hash == a.Hash {
		t.Fatal("a row-volume change must change the note Hash")
	}
	// Muting the row silences every cell.
	g.drum.Rows[0].Muted = true
	c, _ = g.playbackSlate.laneContext()
	for abs := 0; abs < 64; abs++ {
		if n, ok := g.playbackSlate.laneNoteAt(&c, 0, abs); ok && n.Kind == laneSound {
			t.Fatalf("muted row produced %+v", n)
		}
	}
}
