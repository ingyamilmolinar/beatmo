//go:build test

package ui

import (
	"image"
	"testing"
)

// TestTransportHasNoNavOrOverflowHitAreas asserts the bullseye (Navigate) and
// kebab (Overflow) transport buttons are gone from both platforms: navigation
// now opens from the beat counter, file options from the grid folder button.
func TestTransportHasNoNavOrOverflowHitAreas(t *testing.T) {
	run := func(t *testing.T, mobile bool, rect image.Rectangle) {
		forceSmallScreenForTest = mobile
		t.Cleanup(func() { forceSmallScreenForTest = false })
		z, _ := newTestTransportZone()
		z.Layout(rect)
		for _, ha := range z.HitAreas() {
			if ha.Tag == "transport-nav" || ha.Tag == "transport-overflow" {
				t.Fatalf("mobile=%v: %q hit area must be gone", mobile, ha.Tag)
			}
		}
	}
	t.Run("mobile", func(t *testing.T) { run(t, true, image.Rect(0, 0, 390, 96)) })
	t.Run("desktop", func(t *testing.T) { run(t, false, image.Rect(0, 0, 1280, 64)) })
}
