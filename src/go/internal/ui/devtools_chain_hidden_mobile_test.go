//go:build test

package ui

import "testing"

// The phone's bottom nav in the production UI has no Chain segment, and every
// remaining segment still maps to the view it names (devtools.go).
func TestProductionMobileNavHasNoChainSegment(t *testing.T) {
	forceProductionUIForTest(t)
	g := newMobileGame(t)
	sc := g.drum.viewSwitchSegmented
	for i, l := range sc.Labels() {
		if l == "Chn" {
			t.Fatalf("mobile bottom-nav segment %d reads %q in the production UI", i, l)
		}
	}
	// Every segment still maps to the view it names.
	for i, m := range bottomNavModes() {
		if got := segmentIndexForViewMode(m); got != i {
			t.Fatalf("segmentIndexForViewMode(%v) = %d, want %d", m, got, i)
		}
	}
}
