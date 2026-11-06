package ui

import "testing"

// fakeBinding is a test barrelBinding over an int counter with hard clamps.
type fakeBinding struct {
	v, min, max int
}

func (f *fakeBinding) labelAt(off int) string { return "" }
func (f *fakeBinding) stepBy(detents int) bool {
	old := f.v
	f.v += detents
	if f.v < f.min {
		f.v = f.min
	}
	if f.v > f.max {
		f.v = f.max
	}
	return f.v != old
}
func (f *fakeBinding) canStep(dir int) bool {
	if dir > 0 {
		return f.v < f.max
	}
	return f.v > f.min
}

func TestBarrelWheel_SingleSlowNotchMovesOneDetent(t *testing.T) {
	b := &fakeBinding{v: 5, min: 0, max: 100}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	w.ImpulseNotch(+1)
	// One impulse then settle should land exactly one detent up.
	for i := 0; i < 240 && w.Tick(); i++ {
	}
	if b.v != 6 {
		t.Fatalf("one notch = one detent: got %d want 6", b.v)
	}
}

func TestBarrelWheel_FlingDecaysAndSnaps(t *testing.T) {
	b := &fakeBinding{v: 0, min: -1000, max: 1000}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	// Simulate a fast downward drag: y increases 40px/frame for 5 frames.
	w.BeginDrag(0)
	for i := 1; i <= 5; i++ {
		w.DragTo(i * 40)
	}
	w.EndDrag()
	moved := 0
	for i := 0; i < 600 && w.Tick(); i++ {
		moved++
	}
	if w.Animating() {
		t.Fatal("fling must come to rest")
	}
	if b.v <= 10 {
		t.Fatalf("downward fling should coast well past the drag distance, got %d", b.v)
	}
	if w.OffsetPx() != 0 {
		t.Fatalf("must snap to a detent (offset 0), got %v", w.OffsetPx())
	}
}

func TestBarrelWheel_ClampKillsVelocity(t *testing.T) {
	b := &fakeBinding{v: 99, min: 0, max: 100}
	w := NewBarrelWheel()
	w.Bind(b, 20)
	w.BeginDrag(0)
	for i := 1; i <= 10; i++ {
		w.DragTo(i * 60) // huge downward fling toward max
	}
	w.EndDrag()
	for i := 0; i < 600 && w.Tick(); i++ {
	}
	if b.v != 100 {
		t.Fatalf("clamp at max: got %d want 100", b.v)
	}
	if w.OffsetPx() != 0 || w.Animating() {
		t.Fatalf("clamp must kill velocity and settle, offset=%v animating=%v", w.OffsetPx(), w.Animating())
	}
}
