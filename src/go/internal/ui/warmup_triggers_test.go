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

// TestImportTripsTheGate pins the primary trigger: loading a circuit must leave
// playback gated until the working set is warm.
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

// TestImportProducesExactlyOneWarmupSession: Import() restores f.BPM via
// g.drum.SetBPM(f.BPM) before its own unconditional g.beginWarmup("import")
// call. A tempo change used to trip a SEPARATE "bpm" warm-up session (the
// voice cache key carried the tempo); voices no longer depend on it, so only
// the "import" trigger fires.
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
	// wireWarmupNotifications already set onBegin; wrap it rather than
	// clobbering the toast/hooks wiring, so this test also proves the
	// notification path itself only fires once.
	prevOnBegin := g.warmup.onBegin
	g.warmup.onBegin = func(total int, src hooks.Source) {
		begins++
		lastReason = src.File
		if prevOnBegin != nil {
			prevOnBegin(total, src)
		}
	}

	// The fixture template's own tempo differs from the default Game's
	// starting BPM (120), so f.BPM's restore inside Import is a genuine
	// change — exactly the case that used to start a second "bpm" session.
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
	// A few frames, in case a tempo change still started a session late —
	// it must NOT produce a second one.
	for i := 0; i < 12; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if begins != 1 {
		t.Fatalf("import produced %d warm-up sessions (last reason source file %q), want exactly 1", begins, lastReason)
	}
}

// TestTempoChangeNeverGatesPlayback: a voice is render.seconds long at every
// tempo, so no BPM change — programmatic (DrumView.SetBPM), interactive
// (TransportZone.SetBPM, what the +/- keycaps and the BPM wheel reach) or a
// held repeat changing it every frame — starts a warm-up session or closes
// the readiness gate. It used to re-render every voice behind the gate, and
// Play was ignored until that finished.
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

// TestSampleRateChangeTripsTheGate pins the browser-critical case: a circuit
// imported before the AudioContext exists warms at the assumed rate, and every
// cache key rotates when the real rate arrives.
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
	// No Begin() has been called yet, so there is nothing pending to race —
	// this pins the starting state before the sample-rate change below.
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
