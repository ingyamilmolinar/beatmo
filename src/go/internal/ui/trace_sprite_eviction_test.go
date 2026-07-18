//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTabSwitchReleasesTraceSprites pins the eviction rule: hiding a tab
// releases its trace-sprite textures immediately (ebiten Deallocate via
// releaseHiddenTabSprites at the tab-switch chokepoints), and returning to
// the tab lazily re-allocates and re-renders. Without this, each hidden tab
// parks ~2 MB of GPU texture until a Go GC finalizer cycle — measured 20+ s
// apart in real sessions, and the WASM atlas is the long-session OOM driver.
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
