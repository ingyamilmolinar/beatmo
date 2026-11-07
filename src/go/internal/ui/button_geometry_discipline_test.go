//go:build test

package ui

import (
	"image"
	"testing"
)

// Controls in one cluster share their vertical bounds (an inset sibling leaves a seam), and no rounded rect asks for
// a radius over a quarter of its shorter side.

// So a failure names the control.
type clusterMember struct {
	name string
	rect func() image.Rectangle
}

// Against the first non-empty member; a control hidden on this profile is skipped.
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

// Record shares play and stop's vertical extent exactly: an inset left bands of the chip visible above and below it,
// reading as a line. Its lower weight is carried by tone.
func TestTransportRecordMatchesPlayStopBounds(t *testing.T) {
	withSmallScreen(t, true)

	z, _ := newTestTransportZone()
	// The demote was gated on a height floor, so one width could pass by accident.
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

// Every control in the strip lines up, on both profiles.
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

			// The desktop BPM chevrons stack in one column, so the member is their union.
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
