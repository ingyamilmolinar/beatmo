//go:build test

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestImportTripsTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()

	data, err := os.ReadFile(filepath.Join("..", "assets", "templates", "beethoven-ode-to-joy.json"))
	if err != nil {
		t.Skipf("template unavailable: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if g.playbackReady() {
		t.Fatal("import left playback ungated — the voice cache is cold")
	}
	_, total := g.warmup.Progress()
	waitForAskedCount(t, be, total)
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("gate did not open after the warm set completed")
	}
}

// Import restores the BPM before its own beginWarmup; voices no longer depend on tempo, so only the "import"
// trigger fires.
func TestImportProducesExactlyOneWarmupSession(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()
	begins := 0
	var lastReason string
	// Wrapped, not clobbered, so the notification path itself is shown to fire once.
	prevOnBegin := g.warmup.onBegin
	g.warmup.onBegin = func(total int, src hooks.Source) {
		begins++
		lastReason = src.File
		if prevOnBegin != nil {
			prevOnBegin(total, src)
		}
	}

	// The template's tempo differs from 120, so Import's BPM restore is a real change.
	beforeBPM := g.drum.BPM()
	data, err := os.ReadFile(filepath.Join("..", "assets", "templates", "beethoven-ode-to-joy.json"))
	if err != nil {
		t.Skipf("template unavailable: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if g.drum.BPM() == beforeBPM {
		t.Skip("fixture template's BPM matches the default Game's starting BPM — this test needs a genuine delta to be meaningful")
	}
	// A late session from the tempo change would show here.
	for i := 0; i < 12; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if begins != 1 {
		t.Fatalf("import produced %d warm-up sessions (last reason source file %q), want exactly 1", begins, lastReason)
	}
}

// A voice is render.seconds long at every tempo, so no BPM change (programmatic, interactive, or a held repeat
// every frame) starts a warm-up or closes the gate.
func TestTempoChangeNeverGatesPlayback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	if g.drum.transportZone == nil {
		t.Fatal("test requires a real TransportZone — the interactive BPM path doesn't exist without one")
	}

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	begins := 0
	g.warmup.onBegin = func(total int, _ hooks.Source) { begins++ }
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("precondition: a fresh controller is ready")
	}

	update := func() {
		t.Helper()
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	g.drum.SetBPM(g.drum.BPM() + 40)
	update()
	g.drum.transportZone.SetBPM(g.drum.BPM() + 13)
	update()
	base := g.drum.BPM()
	for i := 1; i <= 20; i++ {
		g.drum.transportZone.SetBPM(base + i)
		update()
	}
	for i := 0; i < 12; i++ {
		update()
	}
	if begins != 0 || !g.playbackReady() {
		t.Fatalf("tempo changes started %d warm-up session(s), ready=%v; want none and ready", begins, g.playbackReady())
	}
}

// A circuit imported before the AudioContext exists warms at the assumed rate; every cache key rotates when the real
// rate arrives.
func TestSampleRateChangeTripsTheGate(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()

	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.wireWarmupNotifications()
	g.warmupSampleRate = 44100
	// No Begin() yet, so nothing is pending to race.
	be.completeAll()
	g.warmup.Tick(1)
	if !g.playbackReady() {
		t.Fatal("gate shut before the rate change")
	}

	restore := audio.SetSampleRateForTest(48000)
	t.Cleanup(restore)
	g.checkWarmupSampleRate()
	if g.playbackReady() {
		t.Fatal("sample-rate change left playback ungated despite a re-keyed voice cache")
	}
}
