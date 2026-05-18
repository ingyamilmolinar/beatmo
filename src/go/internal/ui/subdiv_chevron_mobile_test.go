//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSubdivChevron_AbsentOnMobile verifies that the small chevron-down
// hint over the subdivision button is NOT rendered on mobile. Regression
// for A5 in the screenshot critique: the chevron packed into the narrow
// "÷32" rect at small DPR reads as a typo on phone-class screens.
func TestSubdivChevron_AbsentOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 390, 96))

	dst := ebiten.NewImage(390, 96)
	z.subdivChevronDrawn = false
	z.drawSubdivChevronOffset(dst, 0, 0)
	if z.subdivChevronDrawn {
		t.Fatalf("subdiv chevron rendered on mobile (A5 regression)")
	}
}

// TestSubdivChevron_PresentOnDesktop verifies the chevron is preserved on
// desktop where the inline hint is useful and screen real estate isn't
// constrained.
func TestSubdivChevron_PresentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 1280, 96))

	dst := ebiten.NewImage(1280, 96)
	z.subdivChevronDrawn = false
	z.drawSubdivChevronOffset(dst, 0, 0)
	if !z.subdivChevronDrawn {
		t.Fatalf("subdiv chevron not rendered on desktop — regression: hint dropped where it's still useful")
	}
}
