//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRenderProducesNoVisualRegression sanity-checks that Render with the
// generated ComponentButtonSecondary spec produces the same on-screen
// pixels as the legacy TransportPlayStyle.Draw. Drift between the two
// would mean either the spec is wrong (caught by TestComponentSpecsDrift)
// or Render's logic differs from ButtonStyle.Draw's logic (caught here).
//
// Stub-only: relies on Image.At() reading back rendered pixels. Real Ebiten
// panics with "ReadPixels cannot be called before the game starts" because
// no game loop is running under `go test`.
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

// TestRenderAllocationCeiling asserts (rather than merely reports) the
// per-call heap allocation count of Render across its three hot paths.
// Each ceiling matches the value the benchmarks observed at the time of
// Phase 4 PR1; raising one requires a deliberate update here so the bump
// is visible in code review.
//
// Numbers are intentionally pinned, not "<= small". A regression to e.g.
// 5 allocs/call would still look "small" in an isolated benchmark but
// triple the GC pressure at realistic chrome-surface counts (~hundreds
// per 60 Hz frame). Pin and review.
//
// Stub-only: real Ebiten's image pipeline allocates per-call buffers
// (~22-37 allocs/call observed) that have nothing to do with Render's
// own design and would drown out the signal these ceilings exist to
// catch. Stub mode strips that pipeline, leaving only the allocations
// Render itself produces.
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
