package ui

import (
	"math"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func newTestGameWithOrganAtPitch(t *testing.T, inst instrument.ID, pitch int) *Game {
	t.Helper()
	g := buildTestGame(t)
	// Before any updateBeatInfos: in the real build the default warmFn renders into the process-global cache, and a
	// cached pitch is skipped, so the tests would observe no warms.
	g.voicePrewarmer.warmFn = func(instrument.ID, float64) {}
	// The cache is process-global and earlier tests leave pitches warm, so the recorder would see an arbitrary subset;
	// clearing it makes the "clean cache" premise true. A no-op under -tags test.
	audio.ClearVoiceCacheForTest()
	g.drum.Rows[0].Instrument = inst
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.pendingStartRow = -1
	g.graph.SetNodeParams(n0.ID, model.NodeParams{Volume: 1, Pitch: float64(pitch), Duration: 1})
	g.graph.SetNodeParams(n1.ID, model.NodeParams{Volume: 1, Pitch: float64(pitch), Duration: 1})
	g.updateBeatInfos()
	return g
}

// recordWarms swaps in a recording warmFn and returns the recorded set + a mutex.
func recordWarms(g *Game) (*sync.Mutex, map[warmKey]struct{}) {
	var mu sync.Mutex
	warmed := map[warmKey]struct{}{}
	g.voicePrewarmer.warmFn = func(id instrument.ID, pitch float64) {
		mu.Lock()
		warmed[warmKey{id: id, pitch: pitch}] = struct{}{}
		mu.Unlock()
	}
	return &mu, warmed
}

// Under -tags test VoiceCached is always false, so every distinct pitch is submitted.
func TestWarmFromBeatInfosEnumeratesNodePitches(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)
	mu, warmed := recordWarms(g)

	g.updateBeatInfos() // builds beatInfosByRow, then warmFromBeatInfos()

	mu.Lock()
	_, ok := warmed[warmKey{id: roster.OrganChurch, pitch: -2.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("expected warm for (organ-church, -2.0); got %v", warmed)
	}
}

// A one-tick pitch nudge then hits the cache.
func TestPitchNeighborhoodWarmedForMelodic(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)
	g.voicePrewarmer.isMelodic = func(instrument.ID) bool { return true }
	g.voicePrewarmer.ticks = 1
	mu, warmed := recordWarms(g)

	g.voicePrewarmer.warmFromBeatInfos()

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []float64{-2.5, -2.0, -1.5} {
		if _, ok := warmed[warmKey{id: roster.OrganChurch, pitch: want}]; !ok {
			t.Errorf("melodic pitch neighbor %.1f not warmed; got %v", want, warmed)
		}
	}
}

// Their pitch is a post-render resample, already a hit.
func TestPitchNeighborhoodSkippedForNonMelodic(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.Kick, 0)
	g.voicePrewarmer.isMelodic = func(instrument.ID) bool { return false }
	g.voicePrewarmer.ticks = 1
	mu, warmed := recordWarms(g)

	g.voicePrewarmer.warmFromBeatInfos()

	mu.Lock()
	defer mu.Unlock()
	if _, ok := warmed[warmKey{id: roster.Kick, pitch: 0.5}]; ok {
		t.Errorf("non-melodic instrument should not warm pitch neighbors; got %v", warmed)
	}
	if _, ok := warmed[warmKey{id: roster.Kick, pitch: 0.0}]; !ok {
		t.Errorf("non-melodic instrument should still warm its exact pitch; got %v", warmed)
	}
}

// One warm per distinct (instrument, pitch), so a graph edit cannot flood the pool.
func TestWarmFromBeatInfosWithinPassDedup(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)
	// ticks depends on the runtime profile and would fan the pitch out to its neighbours.
	g.voicePrewarmer.ticks = 0

	var mu sync.Mutex
	var calls int
	g.voicePrewarmer.warmFn = func(id instrument.ID, pitch float64) {
		mu.Lock()
		calls++
		mu.Unlock()
	}

	g.voicePrewarmer.warmFromBeatInfos()

	mu.Lock()
	got := calls
	mu.Unlock()
	// Row 0 fires one distinct pitch (-2); both nodes share it. One warm.
	if got != 1 {
		t.Fatalf("within-pass dedup: expected 1 warm for the single distinct pitch, got %d", got)
	}
}

// A cache invalidation re-submits the recorded pitches, so a param edit does not leave the next beat cold.
func TestReWarmInstrumentReplaysEnumeratedPitches(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)
	g.voicePrewarmer.warmFromBeatInfos() // record pitchesByInst

	mu, warmed := recordWarms(g)
	g.voicePrewarmer.reWarmInstrument(roster.OrganChurch)

	mu.Lock()
	_, ok := warmed[warmKey{id: roster.OrganChurch, pitch: -2.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("reWarmInstrument should replay (organ-church, -2.0); got %v", warmed)
	}
}

// Covers group-transposed and dynamic pitches beyond the enumerated window.
func TestScheduleSoundBackstopWarmsExactPitch(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, 0)
	mu, warmed := recordWarms(g)

	info := model.BeatInfo{NodeType: model.NodeTypeRegular}
	// A pitch the enumeration would not have produced (simulated group transpose).
	g.scheduleSound(0, 0, info, roster.OrganChurch, 0.8, 7.0, 1.0, math.NaN(), true)

	mu.Lock()
	_, ok := warmed[warmKey{id: roster.OrganChurch, pitch: 7.0}]
	mu.Unlock()
	if !ok {
		t.Fatalf("backstop should warm (organ-church, 7.0); got %v", warmed)
	}
}
