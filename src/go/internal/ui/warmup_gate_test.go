//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Through the real transport path, not poked state: a component test can pass while the feature is dead.
func TestPlayBlockedWhileWarming(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.warmup.Begin()
	if g.playbackReady() {
		t.Fatal("playbackReady() true while the warm set is outstanding")
	}

	g.drum.TriggerPlayPause()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.Playing() {
		t.Fatal("playback started while the voice cache was cold")
	}

	// The feeder is async; wait for the whole set before completing it.
	_, total := g.warmup.Progress()
	waitForAskedCount(t, be, total)
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("playbackReady() false after the warm set completed")
	}
	g.drum.TriggerPlayPause()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.Playing() {
		t.Fatal("playback did not start after the gate opened")
	}
	stopPlaybackForTest(g)
}

// TestPlayButtonDisabledWhileWarming pins the visible affordance.
func TestPlayButtonDisabledWhileWarming(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.warmup.Begin()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !g.drum.transportZone.playBtn.Disabled {
		t.Fatal("Play button not disabled while warming")
	}
	// The feeder is async; completing a partially fed backend would leave the gate closed.
	_, total := g.warmup.Progress()
	waitForAskedCount(t, be, total)
	be.completeAll()
	g.warmup.Tick(1)
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if g.drum.transportZone.playBtn.Disabled {
		t.Fatal("Play button still disabled after the gate opened")
	}
	stopPlaybackForTest(g)
}

// Only the USER entry point is gated; tests, scenes, parity harnesses and benches drive SetPlaying directly.
func TestSetPlayingBypassesTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	g.warmup = newWarmupController(g, newFakeWarmBackend())
	g.warmup.Begin()
	g.SetPlaying(true)
	if !g.Playing() {
		t.Fatal("SetPlaying(true) was blocked by the gate; it must not be")
	}
	stopPlaybackForTest(g)
}
