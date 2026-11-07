//go:build !test && !js

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// waitPrewarmIdle blocks until the prewarm pool has no queued or running job.
func waitPrewarmIdle(t *testing.T, g *Game) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s := g.voicePrewarmer.pool.Stats()
		if s.Queued == 0 && s.Inflight == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("prewarm pool never went idle: %+v", g.voicePrewarmer.pool.Stats())
}

// A dial turned on a playing melodic row reaches the next trigger of every note it plays, through the real prewarm
// pool and dispatch; once settled, no note plays an older sound.
func TestSynthDialTurnReachesEveryPlayedNote(t *testing.T) {
	g, inst := openMelodicSynthRoot(t)
	dv := g.drum
	card, d := multiTermDial(t, dv, inst)
	g.voicePrewarmer.mu.Lock()
	pitches := append([]float64(nil), g.voicePrewarmer.pitchesByInst[instrument.ID(inst)]...)
	g.voicePrewarmer.mu.Unlock()
	if len(pitches) < 3 {
		t.Fatalf("the prewarmer enumerated only %d pitches for %q", len(pitches), inst)
	}
	waitPrewarmIdle(t, g)
	for _, p := range pitches { // the row is playing: its current voices are cached
		audio.RenderAsPlayed(instrument.ID(inst), p, 1, 0.05)
	}
	waitPrewarmIdle(t, g)
	before := g.voicePrewarmer.pool.Stats().Dropped

	h := &synthDialHitAdapter{dv: dv, instID: instrument.ID(inst), card: card, dial: d.Dial.ID}
	const y0 = 400
	h.OnPress(100, y0)
	for y := y0 - 12; y >= y0-synthDialDragSpanPx/2; y -= 4 {
		h.OnDrag(100, y)
		time.Sleep(16 * time.Millisecond) // one detent per 60 Hz frame
	}
	h.OnRelease(100, y0-synthDialDragSpanPx/2)
	waitPrewarmIdle(t, g)

	audio.ResetRenderLatencyMetrics()
	for _, p := range pitches {
		audio.RenderAsPlayed(instrument.ID(inst), p, 1, 0.05)
	}
	stale := audio.GetRenderLatencyMetrics().StalePlays
	dropped := g.voicePrewarmer.pool.Stats().Dropped - before
	t.Logf("%s/%s on %q: %d pitches, %d prewarm jobs dropped during the turn", card, d.Dial.ID, inst, len(pitches), dropped)
	if stale > 0 {
		t.Errorf("%d of %d notes still played a STALE voice after the turn settled — the final sound was never rendered", stale, len(pitches))
	}
}
