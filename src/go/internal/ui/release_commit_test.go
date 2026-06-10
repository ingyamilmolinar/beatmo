package ui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// waitForRelease polls cond up to ~500ms (the hooks bus delivers off the
// publisher goroutine). Fails the test if cond never holds.
func waitForRelease(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestReleaseCommit_EQBandFiresOnceOnRelease(t *testing.T) {
	var count int32
	unsub := hooks.Subscribe(hooks.EventEQBandChange, func(e hooks.Event) { atomic.AddInt32(&count, 1) })
	t.Cleanup(unsub)

	g := newTestGameForUndo(t)
	// Simulate 5 per-frame OnGainChange stashes during a drag.
	for i := 0; i < 5; i++ {
		g.drum.eqPendingChannel, g.drum.eqPendingBand, g.drum.eqPendingGainDB, g.drum.eqPendingDirty = "main", 3, float64(i), true
	}
	g.drum.commitEQBand()
	waitForRelease(t, func() bool { return atomic.LoadInt32(&count) == 1 })
	// Clean (dirty cleared) → no-op, no second emit.
	g.drum.commitEQBand()
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 1 {
		t.Fatalf("commitEQBand fired %d times, want 1", c)
	}
}

func TestReleaseCommit_MainVolumeFiresOnceOnRelease(t *testing.T) {
	var count int32
	unsub := hooks.Subscribe(hooks.EventMasterVolumeChange, func(e hooks.Event) { atomic.AddInt32(&count, 1) })
	t.Cleanup(unsub)

	g := newTestGameForUndo(t)
	for i := 0; i < 5; i++ {
		g.drum.mainVolPending, g.drum.mainVolPendingDirty = 0.1*float64(i), true
	}
	g.drum.commitMainVolume()
	waitForRelease(t, func() bool { return atomic.LoadInt32(&count) == 1 })
	g.drum.commitMainVolume() // clean → no-op
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 1 {
		t.Fatalf("commitMainVolume fired %d times, want 1", c)
	}
}

func TestReleaseCommit_RowVolumeFiresOnceOnRelease(t *testing.T) {
	var count int32
	unsub := hooks.Subscribe(hooks.EventRowVolume, func(e hooks.Event) { atomic.AddInt32(&count, 1) })
	t.Cleanup(unsub)

	g := newTestGameForUndo(t)
	g.drum.Rows[0].Volume = 0.42
	g.drum.commitRowVolume(0)
	waitForRelease(t, func() bool { return atomic.LoadInt32(&count) == 1 })

	// Out-of-range row → no emit.
	g.drum.commitRowVolume(999)
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 1 {
		t.Fatalf("commitRowVolume fired %d times, want 1", c)
	}
}

func TestReleaseCommit_FXSliderFiresOnceOnRelease(t *testing.T) {
	var count int32
	unsub := hooks.Subscribe(hooks.EventInsertEffectParam, func(e hooks.Event) { atomic.AddInt32(&count, 1) })
	t.Cleanup(unsub)

	g := newTestGameForUndo(t)
	instID := g.drum.Rows[0].Instrument
	order := audio.EffectTypeOrder()
	if len(order) == 0 {
		t.Skip("no insert-effect types registered")
	}
	audio.AddInsertEffect(instID, order[0], nil)
	t.Cleanup(func() { audio.RemoveInsertEffect(instID, 0) })

	// Open the FX panel and build bindings (mobile-agnostic: enabled effect
	// always shows params on desktop; force the expanded slot so the param
	// sliders are built regardless of profile).
	g.drum.OpenFXPanel(0)
	g.drum.fxExpandedSlots = map[int]bool{0: true}
	g.drum.buildFXPanel()
	if len(g.drum.fxSliderBindings) == 0 {
		t.Skip("no FX param sliders built under this profile")
	}

	before := g.undoManager.CanUndo()
	g.drum.commitFXSlider(0)
	waitForRelease(t, func() bool { return atomic.LoadInt32(&count) == 1 })
	if !g.undoManager.CanUndo() && before == g.undoManager.CanUndo() {
		// commitFXSlider must record at least one undo step.
		t.Fatal("commitFXSlider should record an undo step")
	}

	// Out-of-range index → no emit.
	g.drum.commitFXSlider(999)
	time.Sleep(40 * time.Millisecond)
	if c := atomic.LoadInt32(&count); c != 1 {
		t.Fatalf("commitFXSlider fired %d times, want 1", c)
	}
}

func TestReleaseCommit_RecordsOneUndoStep(t *testing.T) {
	g := newTestGameForUndo(t)
	before := g.undoManager.CanUndo()
	if before {
		t.Fatal("expected fresh undo stack")
	}
	g.drum.Rows[0].Volume = 0.33
	g.drum.commitRowVolume(0)
	if !g.undoManager.CanUndo() {
		t.Fatal("commitRowVolume should record one undo step")
	}
}
