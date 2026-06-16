package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMobileBPMInputRegisteredPerFrame reproduces the regression where tapping
// the BPM box on mobile no longer brought up a native input.
//
// The JS native-input system creates the real HTML <input> synchronously inside
// the touchend gesture (onCanvasTouchEnd), which only fires when "bpm" is already
// present in the registrations map AT TOUCHEND TIME. The shared numeric editor
// registers "bpm" only on open — 1-2 frames AFTER Go processes the tap, by which
// point the gesture is gone and the next layout's mobileInputClear() has wiped it.
//
// Therefore the "bpm" direct rect MUST be (re)registered every layout pass on
// mobile, exactly like inst-search / wav-name, so the gesture handler can find it.
func TestMobileBPMInputRegisteredPerFrame(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	testMobileInputRegistered = make(map[string]bool)
	t.Cleanup(func() { testMobileInputRegistered = nil })

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	restore()

	if dv.bpmBox() == nil || dv.bpmBox().Rect.Empty() {
		t.Fatal("precondition: bpm box must be laid out with a non-empty rect on mobile")
	}

	// The native-input gesture handler can only create the input if "bpm" is
	// registered during the layout pass (not deferred to editor-open).
	if !testMobileInputRegistered["bpm"] {
		t.Fatal("bpm native input rect must be registered every layout pass on mobile")
	}
}
