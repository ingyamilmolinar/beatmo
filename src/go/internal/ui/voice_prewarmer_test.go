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

// newTestGameWithOrganAtPitch builds a Game whose row 0 plays the given melodic
// instrument, with a single regular node at the given semitone pitch. Mirrors
// buildTestGame's node-placement recipe (subdiv_safety_test.go).
func newTestGameWithOrganAtPitch(t *testing.T, inst instrument.ID, pitch int) *Game {
	t.Helper()
	g := buildTestGame(t)
	// Neutralize the prewarmer BEFORE any updateBeatInfos below can fire it.
	// Under -tags test PrewarmVoice is a no-op and VoiceCached is always false,
	// but under the real build (make test-real) the default warmFn renders the
	// melodic-instrument neighborhood into the process-global globalVoiceCache.
	// That render would make VoiceCached true, so warmPitch would correctly skip
	// the already-warm pitch and the tests below would observe zero warms. Tests
	// install their own recorder via recordWarms after this; here we only want a
	// clean cache so warmFromBeatInfos's enumeration decisions are observable.
	g.voicePrewarmer.warmFn = func(instrument.ID, float64) {}
	// Neutralizing OUR warmFn is not enough: globalVoiceCache is process-global
	// and every earlier test in the package renders into it (the param-edit
	// playback bench plays the startup demo's organ-church row for 240 steps ×
	// 4 scenarios). Whatever it left warm makes warmPitch correctly skip, so the
	// recorder below sees an arbitrary subset of the enumeration — that is what
	// made these tests pass alone and fail under `make test-real`. Clear it here
	// so the premise above ("a clean cache") is actually established. No-op
	// under -tags test, where VoiceCached is always false anyway.
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

// TestWarmFromBeatInfosEnumeratesNodePitches drives the real enumeration over a
// Game whose row plays a node at pitch -2 (the crashing organ pitch) and asserts
// the prewarmer warmed exactly that (instrument, pitch). Under -tags test
// audio.VoiceCached is always false, so every distinct pitch is submitted.
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

// TestPitchNeighborhoodWarmedForMelodic verifies the predictive ±N warm: for a
// melodic instrument with ticks=1, a node at pitch -2 warms the 0.5st neighbors
// (-2.5, -2.0, -1.5) so an immediate one-tick pitch nudge is a cache hit.
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

// TestPitchNeighborhoodSkippedForNonMelodic verifies non-melodic instruments get
// ONLY the exact pitch (their pitch is a post-render resample — already a hit).
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

// TestWarmFromBeatInfosWithinPassDedup verifies a single enumeration pass
// collapses the many repeated beats of a looping melody to one warm per distinct
// (instrument, pitch) pair — so a graph edit can't burst-flood the pool.
func TestWarmFromBeatInfosWithinPassDedup(t *testing.T) {
	g := newTestGameWithOrganAtPitch(t, roster.OrganChurch, -2)
	// Isolate dedup from pitch-neighborhood expansion: ticks defaults to the
	// runtime profile's PredictiveWarmTicks (0 under -tags test, 1 on real
	// desktop), which would fan the single pitch out to its ±0.5st neighbors and
	// make the "one distinct pitch -> one warm" assertion profile-dependent.
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

// TestReWarmInstrumentReplaysEnumeratedPitches verifies the proactive re-warm
// path: after enumeration records an instrument's pitches, reWarmInstrument
// (fired by a cache invalidation) re-submits them so a param edit does not leave
// the next beat to cold-render.
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

// TestScheduleSoundBackstopWarmsExactPitch verifies the schedule-time backstop
// warms the exact post-effect pitch handed to scheduleSound — the coverage for
// group-transposed/dynamic pitches beyond the enumerated window.
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
