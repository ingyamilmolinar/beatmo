//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests pin the cross-platform parity contract for the per-row
// instrument controls:
//
//   - Tapping the instrument-name label opens the instrument picker on EVERY
//     platform (previously mobile opened the row context menu instead).
//   - The ellipsis (⋯) overflow menu carries only Rename / Origin / Delete on
//     every platform — "Instrument" (mobile-only) and "Color" (desktop-only)
//     are both gone.
//
// They drive the real input loop (Game.Update via DrumView.Update) and assert
// the user-visible overlay state, per the project's functional-test convention.

// instButtonTestDV builds a one-row DrumView with a known instrument list and
// runs a warm-up frame so layout (including the mobile-specific pass) settles.
func instButtonTestDV(t *testing.T, w, h int) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, w, h), nil, game_log.New(nil, game_log.LevelError))
	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
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

// pressReleaseLabel taps the row-0 label via the real input loop: one press
// frame at the label center followed by one release frame.
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

// TestMobileLabelTapOpensInstMenu is the headline behavior change: on mobile,
// tapping the instrument-name label opens the instrument picker directly (like
// desktop), NOT the row context menu.
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

// TestDesktopLabelTapOpensInstMenu is the desktop regression guard: the label
// continues to open the instrument picker (unchanged behavior).
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

// wantEllipsisMenu is the platform-uniform ellipsis menu contract.
var wantEllipsisMenu = []string{"Rename", "Origin", "Delete"}

// TestEllipsisMenuMobileItems pins the mobile ellipsis menu to exactly
// Rename / Origin / Delete — "Instrument" and "Color" are absent.
func TestEllipsisMenuMobileItems(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	dv := NewDrumView(image.Rect(0, 0, 390, 844), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	labels := nonDividerLabels(dv.ContextMenuItemsForTest(0))
	if !equalStringSlices(labels, wantEllipsisMenu) {
		t.Fatalf("mobile ellipsis labels=%v want=%v", labels, wantEllipsisMenu)
	}
	for _, l := range labels {
		if l == "Instrument" || l == "Color" {
			t.Errorf("mobile ellipsis menu must not contain %q", l)
		}
	}
}

// TestEllipsisMenuDesktopItems pins the desktop ellipsis menu to exactly
// Rename / Origin / Delete — "Color" (formerly desktop-only) is gone.
func TestEllipsisMenuDesktopItems(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 1280, 800), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8

	labels := nonDividerLabels(dv.ContextMenuItemsForTest(0))
	if !equalStringSlices(labels, wantEllipsisMenu) {
		t.Fatalf("desktop ellipsis labels=%v want=%v", labels, wantEllipsisMenu)
	}
	for _, l := range labels {
		if l == "Color" || l == "Instrument" {
			t.Errorf("desktop ellipsis menu must not contain %q", l)
		}
	}
}
