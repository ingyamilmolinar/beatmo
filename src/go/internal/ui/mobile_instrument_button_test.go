//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// On every platform the label opens the instrument picker, and ⋯ carries the same items. Driven through the
// real input loop.

// A warm-up frame settles the layout, mobile pass included.
func instButtonTestDV(t *testing.T, w, h int) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, w, h), nil, game_log.New(nil, game_log.LevelError))
	dv.instOptions = []instrument.ID{roster.Kick, roster.Snare, roster.Hihat, roster.Tom, roster.Clap}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	warmUp()
	return dv
}

func pressReleaseLabel(t *testing.T, dv *DrumView, w, h int) {
	t.Helper()
	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created")
	}
	lbl := dv.rowLabels()[0].Rect()
	if lbl.Empty() {
		t.Fatal("row label rect is empty")
	}
	cx := lbl.Min.X + lbl.Dx()/2
	cy := lbl.Min.Y + lbl.Dy()/2

	press := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	press()

	release := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	dv.Update()
	release()
}

// The picker directly, as on desktop, not the row context menu.
func TestMobileLabelTapOpensInstMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	const W, H = 390, 844
	dv := instButtonTestDV(t, W, H)

	if dv.IsInstMenuOpen() || dv.IsContextMenuOpen() {
		t.Fatal("no menu should be open before the tap")
	}

	pressReleaseLabel(t, dv, W, H)

	if !dv.IsInstMenuOpen() {
		t.Fatalf("mobile label tap should open the instrument picker (instMenuOpen=%v, contextMenuOpen=%v)",
			dv.IsInstMenuOpen(), dv.IsContextMenuOpen())
	}
	if dv.IsContextMenuOpen() {
		t.Error("mobile label tap must NOT open the context menu anymore")
	}

	// The picker stays open across idle frames (no flicker).
	idle := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	for i := 0; i < 5; i++ {
		dv.Update()
		if !dv.IsInstMenuOpen() {
			t.Fatalf("instrument picker closed unexpectedly on idle frame %d", i)
		}
	}
	idle()
}

func TestDesktopLabelTapOpensInstMenu(t *testing.T) {
	assertDefaultParityState(t)

	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	const W, H = 1280, 800
	dv := instButtonTestDV(t, W, H)

	pressReleaseLabel(t, dv, W, H)

	if !dv.IsInstMenuOpen() {
		t.Fatalf("desktop label tap should open the instrument picker (instMenuOpen=%v, contextMenuOpen=%v)",
			dv.IsInstMenuOpen(), dv.IsContextMenuOpen())
	}
	if dv.IsContextMenuOpen() {
		t.Error("desktop label tap must not open the context menu")
	}
}

// The same on every platform; "Instrument" is absent because the label opens the picker.
var wantEllipsisMenu = []string{"Rename", "Color", "Origin", "Delete"}

func TestEllipsisMenuMobileItems(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	labels := nonDividerLabels(dv.ContextMenuItemsForTest(0))
	if !equalStringSlices(labels, wantEllipsisMenu) {
		t.Fatalf("mobile ellipsis labels=%v want=%v", labels, wantEllipsisMenu)
	}
	for _, l := range labels {
		if l == "Instrument" {
			t.Errorf("mobile ellipsis menu must not contain %q", l)
		}
	}
}

func TestEllipsisMenuDesktopItems(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 1280, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: roster.Snare, Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	labels := nonDividerLabels(dv.ContextMenuItemsForTest(0))
	if !equalStringSlices(labels, wantEllipsisMenu) {
		t.Fatalf("desktop ellipsis labels=%v want=%v", labels, wantEllipsisMenu)
	}
	for _, l := range labels {
		if l == "Instrument" {
			t.Errorf("desktop ellipsis menu must not contain %q", l)
		}
	}
}
