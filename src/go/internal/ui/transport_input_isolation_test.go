//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A tap on one control's visible rect activates only it. The BPM box once returned InputIgnored, so the press
// fell through to Record's touch-expanded rect (44 px on mobile reaches across BPM-dec) and toggled recording.

// newMobileTransportDV builds a laid-out mobile DrumView for input tests.
func newMobileTransportDV(t *testing.T) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, 390, 800), nil, game_log.New(nil, game_log.LevelError))
	// Two warm-up frames settle layout + hit-area publication.
	dv.Update()
	dv.Update()
	return dv
}

// No release: the one-frame pressed flags are read before a consumer clears them.
func tapCenter(dv *DrumView, cx, cy int) {
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 800 },
	)
	dv.Update()
	restore()
}

func centerOf(r image.Rectangle) (int, int) {
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}

// Opens its value wheel, never Record.
func TestMobileBPMBoxTapDoesNotToggleRecord(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	t.Cleanup(restore)
	if !Profile().IsMobile() {
		t.Fatal("precondition: expected mobile profile")
	}

	dv := newMobileTransportDV(t)
	z := dv.transportZone

	cx, cy := centerOf(z.bpmBox.Rect)
	tapCenter(dv, cx, cy)

	if z.recordPressed {
		t.Errorf("tapping the BPM box fired Record (recordPressed=true) — input bleed via touch-expanded hit area")
	}
	if z.playPressed {
		t.Errorf("tapping the BPM box fired Play")
	}
	if z.stopPressed {
		t.Errorf("tapping the BPM box fired Stop")
	}
	if dv.valueWheelPopup == nil || !dv.valueWheelPopup.IsOpen() {
		t.Errorf("tapping the BPM box did not open the value wheel")
	}
}

// A centre tap activates that control and no other.
func TestMobileTransportControlsTapIsolation(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	t.Cleanup(restore)
	if !Profile().IsMobile() {
		t.Fatal("precondition: expected mobile profile")
	}

	type fired struct{ play, stop, record bool }

	cases := []struct {
		name string
		rect func(z *TransportZone) image.Rectangle
		want fired
	}{
		{"play", func(z *TransportZone) image.Rectangle { return z.playBtn.Rect() }, fired{play: true}},
		{"stop", func(z *TransportZone) image.Rectangle { return z.stopBtn.Rect() }, fired{stop: true}},
		{"record", func(z *TransportZone) image.Rectangle { return z.recordBtn.Rect() }, fired{record: true}},
		{"bpm-box", func(z *TransportZone) image.Rectangle { return z.bpmBox.Rect }, fired{}},
		{"bpm-dec", func(z *TransportZone) image.Rectangle { return z.bpmDecBtn.Rect() }, fired{}},
		{"bpm-inc", func(z *TransportZone) image.Rectangle { return z.bpmIncBtn.Rect() }, fired{}},
		{"subdiv", func(z *TransportZone) image.Rectangle { return z.subdivBtn.Rect() }, fired{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dv := newMobileTransportDV(t)
			z := dv.transportZone
			r := tc.rect(z)
			if r.Empty() {
				t.Skipf("%s has empty rect on this layout", tc.name)
			}
			cx, cy := centerOf(r)
			tapCenter(dv, cx, cy)

			got := fired{play: z.playPressed, stop: z.stopPressed, record: z.recordPressed}
			if got != tc.want {
				t.Errorf("tap on %s center %v: transport one-shot flags = %+v, want %+v "+
					"(a foreign control fired — touch-expanded hit-area bleed)", tc.name, image.Pt(cx, cy), got, tc.want)
			}
		})
	}
}
