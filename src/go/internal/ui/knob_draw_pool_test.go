//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestDrawArcResetsAndReusesScratch asserts that drawArc uses the
// package-level drawArcVS/drawArcIS scratch slices: each call must
// reset their length to 0 (so AppendVerticesAndIndicesForStroke
// appends from a clean state) and preserve their backing arrays
// (no fresh allocation per call). Under the -tags test stub the real
// vertex tessellator is a no-op, so this test verifies the pool
// invariants — reset semantics + cap preservation — that hold
// regardless of the renderer doing actual work. The production
// allocation reduction (~0.7 MB/s when the synth tab is visible)
// flows from these invariants once real Ebiten runs the tessellator.
func TestDrawArcResetsAndReusesScratch(t *testing.T) {
	// Pre-populate the package scratch with a sentinel length + cap.
	drawArcVS = make([]ebiten.Vertex, 10, 50)
	drawArcIS = make([]uint16, 10, 50)
	preVSData := unsafe.SliceData(drawArcVS)
	preISData := unsafe.SliceData(drawArcIS)
	preVSCap := cap(drawArcVS)
	preISCap := cap(drawArcIS)

	dst := ebiten.NewImage(64, 64)
	drawArc(dst, 32, 32, 20, 0, 1.5, 2, color.White)

	if len(drawArcVS) != 0 {
		t.Fatalf("expected drawArcVS reset to len=0 by drawArc, got %d (proof that the pool is mutated, not bypassed)", len(drawArcVS))
	}
	if len(drawArcIS) != 0 {
		t.Fatalf("expected drawArcIS reset to len=0 by drawArc, got %d", len(drawArcIS))
	}
	if got := unsafe.SliceData(drawArcVS); got != preVSData {
		t.Fatalf("expected drawArcVS backing array preserved across drawArc (no fresh allocation); was %p now %p", preVSData, got)
	}
	if got := unsafe.SliceData(drawArcIS); got != preISData {
		t.Fatalf("expected drawArcIS backing array preserved across drawArc; was %p now %p", preISData, got)
	}
	if cap(drawArcVS) != preVSCap {
		t.Fatalf("expected drawArcVS cap preserved; was %d now %d", preVSCap, cap(drawArcVS))
	}
	if cap(drawArcIS) != preISCap {
		t.Fatalf("expected drawArcIS cap preserved; was %d now %d", preISCap, cap(drawArcIS))
	}
}

// TestKnobDrawDoesNotPanicWithPool is a regression guard that the
// pool change doesn't break Knob.Draw's existing behaviour. It pairs
// with TestKnobDrawDoesNotPanicOnEmptyRect / TestKnobDrawAtTypicalSize
// (knob_test.go) — those tests still pass post-fix because the pool
// is invisible from the public API.
func TestKnobDrawDoesNotPanicWithPool(t *testing.T) {
	k := NewKnob(0.5)
	k.SetRect(image.Rect(0, 0, 60, 60))
	dst := ebiten.NewImage(64, 64)
	for i := 0; i < 5; i++ {
		k.Draw(dst)
	}
	// After 5 Draws + per-Knob 2 drawArc invocations, the pool slices
	// must still be reset (len=0) so the next draw starts clean.
	if len(drawArcVS) != 0 || len(drawArcIS) != 0 {
		t.Fatalf("expected pool reset after Knob.Draw cycle; got vs=%d is=%d", len(drawArcVS), len(drawArcIS))
	}
}
