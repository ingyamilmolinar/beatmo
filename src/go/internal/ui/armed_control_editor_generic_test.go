//go:build test

package ui

import "testing"

// Over an int counter, with no DrumView.
type fakeBarrelBinding struct {
	val  int
	min  int
	max  int
	name string
}

func (b *fakeBarrelBinding) labelAt(offset int) string {
	v := b.val + offset
	if v < b.min || v > b.max {
		return ""
	}
	return b.name
}

func (b *fakeBarrelBinding) stepBy(detents int) bool {
	next := b.val + detents
	if next < b.min {
		next = b.min
	}
	if next > b.max {
		next = b.max
	}
	if next == b.val {
		return false
	}
	b.val = next
	return true
}

func (b *fakeBarrelBinding) canStep(dir int) bool {
	if dir > 0 {
		return b.val < b.max
	}
	return b.val > b.min
}

// Built from bindings and an onEdit closure with no DrumView, the seam the Sampler uses; keyboard steps move the value
// and fire onEdit.
func TestArmedControlEditorIsPanelAgnostic(t *testing.T) {
	val := &fakeBarrelBinding{val: 5, min: 0, max: 10, name: "v"}
	res := &fakeBarrelBinding{val: 2, min: 0, max: 4, name: "r"}
	edits := 0

	e := newArmedControlEditor("inst-x", 3, val, res, func() { edits++ })

	if e.instID != "inst-x" || e.idx != 3 {
		t.Fatalf("identity not carried: instID=%q idx=%d", e.instID, e.idx)
	}
	if e.res() == nil {
		t.Fatal("resolution lane should exist when a non-nil res binding is supplied")
	}
	if !e.StepValue(+1) {
		t.Fatal("StepValue(+1) should move the value binding")
	}
	if val.val != 6 {
		t.Fatalf("value binding = %d, want 6", val.val)
	}
	if !e.StepResolution(+1) {
		t.Fatal("StepResolution(+1) should move the resolution binding")
	}
	if res.val != 3 {
		t.Fatalf("resolution binding = %d, want 3", res.val)
	}
	if edits != 2 {
		t.Fatalf("onEdit fired %d times, want 2 (one per successful step)", edits)
	}
}

// No resolution ladder: a single value lane.
func TestArmedControlEditorNilResBindingHasNoResLane(t *testing.T) {
	val := &fakeBarrelBinding{val: 1, min: 0, max: 3, name: "v"}
	e := newArmedControlEditor("", 0, val, nil, func() {})
	if e.res() != nil {
		t.Fatal("nil res binding must leave the resolution wheel nil")
	}
	if e.StepResolution(+1) {
		t.Fatal("StepResolution must be a no-op with no resolution lane")
	}
}
