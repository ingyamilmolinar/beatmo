//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Render with the generated spec matches ButtonStyle.Draw pixel for pixel. Stub only: real Ebiten cannot
// ReadPixels before the game loop starts.
func TestRenderProducesNoVisualRegression(t *testing.T) {
	r := image.Rect(0, 0, 64, 32)

	imgRender := ebiten.NewImage(64, 32)
	Render(imgRender, r, Spec(ComponentButtonSecondary), ComponentState{})

	imgLegacy := ebiten.NewImage(64, 32)
	TransportPlayStyle.Draw(imgLegacy, r, false, false)

	rr1, gg1, bb1, aa1 := imgRender.At(20, 20).RGBA()
	rr2, gg2, bb2, aa2 := imgLegacy.At(20, 20).RGBA()
	if rr1 != rr2 || gg1 != gg2 || bb1 != bb2 || aa1 != aa2 {
		t.Errorf("interior pixel mismatch: Render=%v vs ButtonStyle.Draw=%v",
			[4]uint32{rr1, gg1, bb1, aa1}, [4]uint32{rr2, gg2, bb2, aa2})
	}
}

// Pinned exactly, not "small": 5 allocs/call still looks small but triples GC pressure at hundreds of surfaces per
// frame; raise one only deliberately. Stub only: real Ebiten's pipeline allocations would drown the signal.
func TestRenderAllocationCeiling(t *testing.T) {
	dst := ebiten.NewImage(64, 32)
	r := image.Rect(0, 0, 64, 32)
	specBtn := Spec(ComponentButtonSecondary)
	specInput := Spec(ComponentInputField)

	cases := []struct {
		name string
		max  uint64
		fn   func()
	}{
		{
			name: "fast-path (no interaction delta)",
			max:  4,
			fn:   func() { Render(dst, r, specBtn, ComponentState{}) },
		},
		{
			name: "hover-path (delta + adjustColor x2)",
			max:  6,
			fn:   func() { Render(dst, r, specBtn, ComponentState{Hovered: true}) },
		},
		{
			name: "focus-path (delta + accent ring)",
			max:  7,
			fn:   func() { Render(dst, r, specInput, ComponentState{Focused: true}) },
		},
	}

	for _, tc := range cases {
		got := uint64(testing.AllocsPerRun(200, tc.fn))
		if got > tc.max {
			t.Errorf("%s: %d allocs/op exceeds ceiling %d — investigate before raising the ceiling", tc.name, got, tc.max)
		}
	}
}
