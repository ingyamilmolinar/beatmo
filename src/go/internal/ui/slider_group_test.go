//go:build test

package ui

import (
	"image"
	"testing"
)

func makeTestSliders(n int) []*Slider {
	sliders := make([]*Slider, n)
	for i := range sliders {
		s := NewSlider(0.5)
		// Position sliders side by side: [0..100], [100..200], etc.
		s.SetRect(image.Rect(i*100, 0, (i+1)*100, 20))
		sliders[i] = s
	}
	return sliders
}

// TestSliderGroup_PressStartsCapture verifies that pressing inside a slider
// captures the group and fires onChange.
func TestSliderGroup_PressStartsCapture(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var cbIdx int
	var cbVal float64
	var cbCount int
	sliders := makeTestSliders(3)
	g := NewSliderGroup(sliders, func(idx int, val float64) {
		cbIdx = idx
		cbVal = val
		cbCount++
	})

	// Press inside slider 1 (x=150, which is in [100..200]).
	r := g.HandleInput(150, 10, true)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
	if !g.Capturing() {
		t.Fatal("group should be capturing")
	}
	if g.Active() != 1 {
		t.Fatalf("expected active=1, got %d", g.Active())
	}
	if cbCount != 1 {
		t.Fatalf("expected 1 onChange call, got %d", cbCount)
	}
	if cbIdx != 1 {
		t.Fatalf("expected onChange idx=1, got %d", cbIdx)
	}
	_ = cbVal // value depends on track rect computation

	// Release.
	r = g.HandleInput(150, 10, false)
	if r != InputConsumed {
		t.Fatalf("expected InputConsumed on release, got %d", r)
	}
	if g.Capturing() {
		t.Fatal("group should not be capturing after release")
	}
	if g.Active() != -1 {
		t.Fatalf("expected active=-1, got %d", g.Active())
	}
}

// TestSliderGroup_MutualExclusion verifies that once a slider captures,
// other sliders do not receive input.
func TestSliderGroup_MutualExclusion(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var activatedIdx int
	sliders := makeTestSliders(3)
	g := NewSliderGroup(sliders, func(idx int, val float64) {
		activatedIdx = idx
	})

	// Press slider 0.
	g.HandleInput(50, 10, true)
	if g.Active() != 0 {
		t.Fatalf("expected active=0, got %d", g.Active())
	}
	activatedIdx = -1

	// Now try pressing where slider 2 would be — should route to slider 0.
	g.HandleInput(250, 10, true)
	if g.Active() != 0 {
		t.Fatalf("should still be slider 0, got %d", g.Active())
	}
	if activatedIdx != 0 {
		t.Fatalf("onChange should fire for idx 0, got %d", activatedIdx)
	}

	// Release.
	g.HandleInput(250, 10, false)
	if g.Capturing() {
		t.Fatal("should not be capturing after release")
	}
}

// TestSliderGroup_MissReturnsIgnored verifies that pressing outside all
// sliders returns InputIgnored.
func TestSliderGroup_MissReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, nil)

	r := g.HandleInput(500, 500, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
	if g.Capturing() {
		t.Fatal("should not be capturing on miss")
	}
}

// TestSliderGroup_ReleaseWithoutCapture verifies that releasing without a
// prior capture returns InputIgnored.
func TestSliderGroup_ReleaseWithoutCapture(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, nil)

	r := g.HandleInput(50, 10, false)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// TestSliderGroup_DragUpdatesValue verifies that dragging within a captured
// slider updates the value and fires onChange each time.
func TestSliderGroup_DragUpdatesValue(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var values []float64
	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, func(idx int, val float64) {
		values = append(values, val)
	})

	track := sliders[0].TrackRect()

	// Press at left edge.
	g.HandleInput(track.Min.X, 10, true)
	// Drag to right edge.
	g.HandleInput(track.Max.X-1, 10, true)
	// Release.
	g.HandleInput(track.Max.X-1, 10, false)

	if len(values) != 2 {
		t.Fatalf("expected 2 onChange calls, got %d", len(values))
	}
	if values[0] != 0.0 {
		t.Fatalf("expected first value=0.0, got %f", values[0])
	}
	if values[1] != 1.0 {
		t.Fatalf("expected second value=1.0, got %f", values[1])
	}
}

// TestSliderGroup_Release forces capture release without onChange.
func TestSliderGroup_ForceRelease(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	called := false
	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, func(idx int, val float64) {
		called = true
	})

	g.HandleInput(50, 10, true)
	if !g.Capturing() {
		t.Fatal("should be capturing")
	}
	called = false
	g.Release()
	if g.Capturing() {
		t.Fatal("should not be capturing after Release()")
	}
	if called {
		t.Fatal("onChange should not fire on forced release")
	}
}

// TestSliderGroup_NilSliderSkipped verifies that nil entries in the slider
// slice are safely skipped.
func TestSliderGroup_NilSliderSkipped(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	s := NewSlider(0.5)
	s.SetRect(image.Rect(100, 0, 200, 20))
	sliders := []*Slider{nil, s, nil}
	g := NewSliderGroup(sliders, nil)

	r := g.HandleInput(150, 10, true)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
	if g.Active() != 1 {
		t.Fatalf("expected active=1, got %d", g.Active())
	}

	g.HandleInput(150, 10, false)
}

// TestSliderGroup_SetSliders replaces sliders and resets capture.
func TestSliderGroup_SetSliders(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, nil)

	// Start capture.
	g.HandleInput(50, 10, true)
	if !g.Capturing() {
		t.Fatal("should be capturing")
	}

	// Replace sliders.
	newSliders := makeTestSliders(1)
	g.SetSliders(newSliders)
	if g.Capturing() {
		t.Fatal("should not be capturing after SetSliders")
	}
	if g.Active() != -1 {
		t.Fatalf("expected active=-1, got %d", g.Active())
	}
}

// TestSliderGroup_InputBounds verifies the union of all slider bounds.
func TestSliderGroup_InputBounds(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(3) // [0..100], [100..200], [200..300]
	g := NewSliderGroup(sliders, nil)

	bounds := g.InputBounds()
	want := image.Rect(0, 0, 300, 20)
	if bounds != want {
		t.Fatalf("expected bounds %v, got %v", want, bounds)
	}
}

// TestSliderGroup_Sliders verifies that Sliders() returns the current
// slider slice.
func TestSliderGroup_Sliders(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(3)
	g := NewSliderGroup(sliders, nil)

	got := g.Sliders()
	if len(got) != len(sliders) {
		t.Fatalf("expected %d sliders, got %d", len(sliders), len(got))
	}
	for i := range sliders {
		if got[i] != sliders[i] {
			t.Fatalf("slider %d: expected pointer %p, got %p", i, sliders[i], got[i])
		}
	}
}

// TestSliderGroup_ZIndex verifies that ZIndex() returns 0.
func TestSliderGroup_ZIndex(t *testing.T) {
	assertDefaultParityState(t)

	g := NewSliderGroup(nil, nil)
	if z := g.ZIndex(); z != 0 {
		t.Fatalf("expected ZIndex 0, got %d", z)
	}
}

// TestSliderGroup_HandleWheel verifies that HandleWheel returns InputIgnored.
func TestSliderGroup_HandleWheel(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, nil)

	r := g.HandleWheel(50, 10, 3)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// TestSliderGroup_ContainerBoundsRejectsMiss verifies that when container
// bounds are set, a press outside the container is rejected.
func TestSliderGroup_ContainerBoundsRejectsMiss(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2) // [0..100], [100..200], y: [0..20]
	g := NewSliderGroup(sliders, nil)
	// Set container to a small area that excludes slider 1.
	g.SetContainerBounds(image.Rect(0, 0, 50, 20))

	// Press inside slider 1 (x=150) but outside container bounds.
	r := g.HandleInput(150, 10, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored for press outside container, got %d", r)
	}
	if g.Capturing() {
		t.Fatal("should not be capturing outside container bounds")
	}
}

// TestSliderGroup_ContainerBoundsAllowsHit verifies that when container
// bounds are set, a press inside the container works normally.
func TestSliderGroup_ContainerBoundsAllowsHit(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2) // [0..100], [100..200], y: [0..20]
	g := NewSliderGroup(sliders, nil)
	// Set container to cover both sliders.
	g.SetContainerBounds(image.Rect(0, 0, 200, 20))

	// Press inside slider 0 (x=50), inside container.
	r := g.HandleInput(50, 10, true)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured for press inside container, got %d", r)
	}
	if !g.Capturing() {
		t.Fatal("should be capturing inside container bounds")
	}
	// Clean up capture.
	g.HandleInput(50, 10, false)
}
