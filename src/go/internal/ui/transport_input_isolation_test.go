//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests guard transport-bar input isolation: a tap on the visible rect
// of one control must activate ONLY that control. The reported bug was that
// tapping the BPM text box on mobile toggled recording — because the BPM box's
// hit adapter returned InputIgnored, letting the dispatcher fall through to the
// Record button's touch-EXPANDED hit rect (MinTarget=44 on mobile expands the
// ~21px Record cell across the BPM-dec button into the BPM box).
//
// Root cause chain (see transport_zone.go + hit_index.go + drumview_tree.go):
//   1. Simple transport buttons register Touch:true hit areas, expanded by
//      TouchMinTarget() (44 on mobile) on every side in HitIndex.At.
//   2. The BPM box registers an exact (non-Touch) hit at z+1; it sorts first
//      (exact-before-expanded), but textInputHitAdapter.OnPress returned
//      InputIgnored, so the dispatcher fell through to the next hit.
//   3. The next hit is the Record button's expanded rect; buttonHitAdapter
//      fires OnClick unconditionally -> recording toggles.

// newMobileTransportDV builds a laid-out mobile DrumView for input tests.
func newMobileTransportDV(t *testing.T) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, 390, 800), nil, game_log.New(nil, game_log.LevelError))
	// Two warm-up frames settle layout + hit-area publication.
	dv.Update()
	dv.Update()
	return dv
}

// tapCenter injects a single left-button press at (cx,cy) and advances one
// frame so the tree dispatches the press. It does NOT release — the one-frame
// pressed flags (playPressed/stopPressed/recordPressed) are read immediately
// after, before any consumer clears them.
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

// TestMobileBPMBoxTapDoesNotToggleRecord is the precise regression for the
// reported bug: tapping the BPM box must focus it, never fire Record.
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
	if !z.paramEditor.Active() {
		t.Errorf("tapping the BPM box did not open the shared editor (paramEditor inactive)")
	}
}

// TestMobileTransportControlsTapIsolation sweeps each transport control and
// asserts a center tap activates that control and no foreign transport button.
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
