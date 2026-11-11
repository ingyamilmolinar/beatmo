//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestPlayBlockedWhileWarming drives the REAL transport path (playPressed →
// Game.Update) rather than poking state, per the project's end-to-end testing
// rule: a component test can pass while the feature is dead.
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

	// Begin() feeds the backend off the caller's goroutine (see
	// warmup_controller.go), so the asked/pending slices populate
	// asynchronously. Wait for the full set to be fed before completing it —
	// mirrors the existing TestWarmup* convention (waitForAskedCount) so this
	// test isn't racy under load.
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
	// See TestPlayBlockedWhileWarming: wait for the async feeder to finish
	// submitting the full warm set before completing it, or completeAll can
	// race a partially-fed backend and leave the gate closed.
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

// TestSetPlayingBypassesTheGate pins that the gate lives at the USER entry
// point only. Tests, scenes, parity harnesses and the bench drive SetPlaying
// directly and must keep working.
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
