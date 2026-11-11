//go:build test

package ui

import (
	"testing"
)

// A press over the open overflow menu resolves to the menu's overlay owner, and
// a press over a base-zone control resolves to that zone — proving TopmostOwnerAt
// mirrors real dispatch precedence (blocking portal wins).
func TestTopmostOwnerAt_BlockingPortalOwnsPoint(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	dv.OpenOverflowMenu()
	g.Update() // publish portal hit areas into the overlay subtree index

	r := dv.overflowPopupRect()
	if r.Empty() {
		t.Fatal("overflow popup rect empty after open")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if got := dv.rootTree.TopmostOwnerAt(cx, cy); got != "overflow-menu" {
		t.Fatalf("TopmostOwnerAt over open menu = %q, want overflow-menu", got)
	}
}
