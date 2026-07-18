package ui

import (
	"image"
	"testing"
)

// A button at rest or steadily-on must do ZERO per-frame state work and
// allocate nothing — the single WASM audio thread cannot afford chrome churn.
func TestButtonSettlesToSilence(t *testing.T) {
	b := NewButton("Play", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 60, 30))
	b.HandleInputResult(10, 10, true)
	b.HandleInputResult(10, 10, false)
	b.engageAnim = 1 // simulate a one-shot engage flash that must decay to 0
	settled := -1
	for i := 0; i < 240; i++ {
		b.AdvancePressAnim()
		if b.pressDepth == b.pressTarget && b.engageAnim == 0 {
			settled = i
			break
		}
	}
	if settled < 0 {
		t.Fatalf("button never settled (depth/engage) within 240 frames")
	}
	// Once settled, AdvancePressAnim must be a pure no-op.
	before := b.pressDepth
	for i := 0; i < 100; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth != before || b.engageAnim != 0 {
		t.Fatalf("settled button mutated state on tick (depth %v→%v, engage %v)", before, b.pressDepth, b.engageAnim)
	}
}

func TestButtonAnimationAllocFree(t *testing.T) {
	b := NewButton("Rec", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 60, 30))
	b.HandleInputResult(10, 10, true)
	b.HandleInputResult(10, 10, false)
	b.engageAnim = 1 // exercise the engage-decay branch too
	allocs := testing.AllocsPerRun(500, func() {
		b.AdvancePressAnim()
	})
	if allocs != 0 {
		t.Fatalf("AdvancePressAnim allocates %v/run, want 0", allocs)
	}
}
