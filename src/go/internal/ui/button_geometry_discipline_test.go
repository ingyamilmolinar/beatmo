//go:build test

package ui

import (
	"image"
	"testing"
)

// button_geometry_discipline_test.go — the two geometry invariants from
// docs/superpowers/specs/2026-07-27-unified-button-geometry-design.md:
//
//   1. Cluster alignment — controls that sit in the same cluster share their
//      vertical bounds. A control that insets itself relative to its siblings
//      leaves a visible seam in the strip it lives in.
//   2. The anti-pill invariant — no rounded rect may request a corner radius
//      greater than a quarter of its shorter side (see
//      TestNoPillShapedControls in rounded_literal_discipline_test.go).

// clusterMember pairs a control's display name with a live accessor for its
// rect, so a failure names the offending control rather than an index.
type clusterMember struct {
	name string
	rect func() image.Rectangle
}

// assertSharedVerticalBounds fails when the members of a cluster disagree on
// Min.Y, Max.Y or Dy. The first non-empty member is the reference; empty rects
// are skipped (a control hidden on this profile is not part of the cluster).
func assertSharedVerticalBounds(t *testing.T, cluster string, members []clusterMember) {
	t.Helper()
	var refName string
	var ref image.Rectangle
	for _, m := range members {
		r := m.rect()
		if r.Empty() {
			continue
		}
		if ref.Empty() {
			ref, refName = r, m.name
			continue
		}
		if r.Min.Y != ref.Min.Y || r.Max.Y != ref.Max.Y {
			t.Errorf("%s: %s spans y=%d..%d but sibling %s spans y=%d..%d — "+
				"cluster members must share vertical bounds (the gap reads as a seam)",
				cluster, m.name, r.Min.Y, r.Max.Y, refName, ref.Min.Y, ref.Max.Y)
		}
	}
}

// TestTransportRecordMatchesPlayStopBounds is the regression test for the
// reported bug: on mobile the record button was shrunk by
// recordDemoteInsetMobile (8 px top AND bottom), leaving two ~6 px bands of the
// surrounding chip visible above and below it. Those bands render as a
// horizontal line cutting across the top of the control's slot.
//
// Record must occupy exactly the same vertical extent as play and stop. Its
// lower visual weight is carried by tone (colRecordIdle vs. the saturated play
// green), not by geometry.
func TestTransportRecordMatchesPlayStopBounds(t *testing.T) {
	withSmallScreen(t, true)

	z, _ := newTestTransportZone()
	// Sweep realistic phone widths — the demote was gated on a height floor, so
	// a single width could pass by accident.
	for _, w := range []int{320, 360, 390, 414, 480} {
		z.Layout(image.Rect(0, 0, w, 56))
		play := z.playBtn.Rect()
		rec := z.recordBtn.Rect()
		if play.Empty() || rec.Empty() {
			t.Fatalf("width=%d: play=%v record=%v — both must be laid out", w, play, rec)
		}
		if rec.Dy() != play.Dy() {
			t.Errorf("width=%d: record height %d != play height %d — record is demoted by geometry",
				w, rec.Dy(), play.Dy())
		}
		if rec.Min.Y != play.Min.Y || rec.Max.Y != play.Max.Y {
			t.Errorf("width=%d: record spans y=%d..%d but play spans y=%d..%d — "+
				"the offset bands are the reported seam",
				w, rec.Min.Y, rec.Max.Y, play.Min.Y, play.Max.Y)
		}
	}
}

// TestTransportClusterSharesBounds generalises the record regression to the
// whole transport strip, on both profiles. Every control the user reads as
// belonging to one horizontal run must line up.
func TestTransportClusterSharesBounds(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name, func(t *testing.T) {
			withSmallScreen(t, mobile)

			z, _ := newTestTransportZone()
			w, h := 1280, 40
			if mobile {
				w, h = 390, 56
			}
			z.Layout(image.Rect(0, 0, w, h))

			// The BPM chevrons STACK inside one column on desktop, so each half
			// legitimately spans less than the row. The cluster member is their
			// union — the column they share — which must match the row.
			bpmStepper := func() image.Rectangle {
				return z.bpmDecBtn.Rect().Union(z.bpmIncBtn.Rect())
			}
			assertSharedVerticalBounds(t, "transport row", []clusterMember{
				{"play", z.playBtn.Rect},
				{"stop", z.stopBtn.Rect},
				{"record", z.recordBtn.Rect},
				{"subdiv", z.subdivBtn.Rect},
				{"bpmStepper", bpmStepper},
			})
		})
	}
}
