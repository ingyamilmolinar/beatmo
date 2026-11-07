//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Hiding a tab releases its textures at once and returning re-renders lazily; otherwise each hidden tab parks GPU
// texture until a GC finalizer runs, and the WASM atlas drives long-session OOM.
func TestTabSwitchReleasesTraceSprites(t *testing.T) {
	g := driveScene(t, "crop_eq_tab_eq")
	expandEQPanelForTest(t, g)
	z := g.drum.eqPanelZone
	screen := ebiten.NewImage(1280, 720)

	g.Draw(screen)
	if !z.curveSprite.resident() || !z.eqBackdropSprite.resident() || !z.eqLabelsSprite.resident() {
		t.Fatalf("EQ tab draw did not allocate its sprites (curve=%v backdrop=%v labels=%v)",
			z.curveSprite.resident(), z.eqBackdropSprite.resident(), z.eqLabelsSprite.resident())
	}

	// Leave the EQ tab: its sprites must be released immediately.
	z.SetActiveTab(TabWave)
	if z.curveSprite.resident() || z.eqBackdropSprite.resident() || z.eqLabelsSprite.resident() {
		t.Fatalf("leaving TabEQ must release its sprites (curve=%v backdrop=%v labels=%v)",
			z.curveSprite.resident(), z.eqBackdropSprite.resident(), z.eqLabelsSprite.resident())
	}

	// Return: lazy re-allocation + re-render on the next draw.
	z.SetActiveTab(TabEQ)
	g.Draw(screen)
	if !z.curveSprite.resident() {
		t.Fatal("returning to TabEQ must re-allocate the curve sprite on draw")
	}
}
