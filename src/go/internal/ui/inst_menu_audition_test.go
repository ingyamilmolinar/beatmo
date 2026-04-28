//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the audition flow: the instrument menu must stay open after
// each selection so users can rapidly try multiple instruments without
// reopening the menu. The menu only closes via explicit dismissal: the X
// close button, Escape, click-outside, or opening another row's menu.

// auditionDV builds a minimal DrumView with one row and the menu component
// ready to be opened. Mirrors the setup in inst_menu_open_close_test.go and
// inst_menu_second_select_test.go.
func auditionDV(t *testing.T) *DrumView {
	t.Helper()
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
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
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false
	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not created after calcLayout")
	}
	return dv
}

// openInstMenuViaLabel exercises the real entry point: click the row label,
// which routes through openInstMenuForRow → SetProps + Open + portal.
func openInstMenuViaLabel(t *testing.T, dv *DrumView, row int) {
	t.Helper()
	if row < 0 || row >= len(dv.rowLabels()) {
		t.Fatalf("row %d out of range (have %d)", row, len(dv.rowLabels()))
	}
	lbl := dv.rowLabels()[row]
	if lbl.OnClick == nil {
		t.Fatalf("row label %d has no OnClick handler", row)
	}
	lbl.OnClick()
	suppressClicksUntilRelease = false
	if !dv.IsInstMenuOpen() {
		t.Fatalf("inst menu did not open after clicking row %d label", row)
	}
}

// clickInstButton finds the menu button labeled buttonLabel and simulates
// a press → release cycle on the component, which fires the button's
// OnClick (and therefore the menu's OnSelect callback).
func clickInstButton(t *testing.T, dv *DrumView, buttonLabel string) {
	t.Helper()
	if dv.instMenuComp == nil || !dv.instMenuComp.IsOpen() {
		t.Fatalf("expected inst menu to be open before clicking %q", buttonLabel)
	}
	var btn *Button
	for _, b := range dv.instMenuComp.InstBtns() {
		if b.Text == buttonLabel {
			btn = b
			break
		}
	}
	if btn == nil {
		t.Fatalf("could not find button %q in menu", buttonLabel)
	}
	r := btn.Rect()
	cx := r.Min.X + 5
	cy := r.Min.Y + 5
	dv.instMenuComp.HandleInput(cx, cy, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(cx, cy, false)
	// Settle layout updates from SetInstrument.
	if dv.bgDirty {
		dv.calcLayout()
		dv.bgDirty = false
	}
}

// TestInstrumentMenuStaysOpenAfterSelection — the core contract: clicking an
// instrument must NOT close the menu.
func TestInstrumentMenuStaysOpenAfterSelection(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)

	clickInstButton(t, dv, "Snare")

	if dv.Rows[0].Instrument != "snare" {
		t.Fatalf("row instrument: got %q, want %q", dv.Rows[0].Instrument, "snare")
	}
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu must STAY OPEN after instrument selection (audition contract)")
	}
	if !dv.instMenuComp.IsOpen() {
		t.Fatal("menu component must STAY OPEN after instrument selection")
	}

	// Idle frames: menu must remain open with no input.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	for i := 0; i < 5; i++ {
		dv.Update()
		if !dv.IsInstMenuOpen() {
			t.Fatalf("menu closed unexpectedly on idle frame %d after selection", i)
		}
	}
	restore()
}

// TestInstrumentMenuMultipleSelectionsInOneOpenSession — user can click
// through several instruments without reopening.
func TestInstrumentMenuMultipleSelectionsInOneOpenSession(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)

	for _, pick := range []struct{ label, id string }{
		{"Snare", "snare"},
		{"Hihat", "hihat"},
		{"Tom", "tom"},
		{"Clap", "clap"},
	} {
		clickInstButton(t, dv, pick.label)
		if dv.Rows[0].Instrument != pick.id {
			t.Fatalf("after picking %q: row instrument got %q, want %q", pick.label, dv.Rows[0].Instrument, pick.id)
		}
		if !dv.IsInstMenuOpen() {
			t.Fatalf("menu closed during audition after picking %q", pick.label)
		}
	}

	if dv.Rows[0].Instrument != "clap" {
		t.Fatalf("final row instrument: got %q, want %q", dv.Rows[0].Instrument, "clap")
	}
}

// TestInstrumentMenuRepeatedSameInstrumentClicks — clicking the same row
// repeatedly is a no-op for the row state but must not close the menu.
func TestInstrumentMenuRepeatedSameInstrumentClicks(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)

	for i := 0; i < 3; i++ {
		clickInstButton(t, dv, "Snare")
		if dv.Rows[0].Instrument != "snare" {
			t.Fatalf("iteration %d: row instrument got %q, want %q", i, dv.Rows[0].Instrument, "snare")
		}
		if !dv.IsInstMenuOpen() {
			t.Fatalf("menu closed after repeated click %d on Snare", i)
		}
	}
}

// TestInstrumentMenuSelectionDuringPlaybackKeepsPlayingAndOpen —
// the headline use case: while playing, the user clicks through instruments
// and the next scheduled beat naturally fires the latest pick. We verify the
// row state updates and playback is uninterrupted.
func TestInstrumentMenuSelectionDuringPlaybackKeepsPlayingAndOpen(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.SetPlayFunc(func(string, float64, ...float64) {}) // swallow audio side-effects
	dv := g.drum

	dv.instOptions = []string{"kick", "snare", "hihat", "tom", "clap"}
	dv.instCategories = nil
	dv.instCatByID = nil
	if len(dv.Rows) == 0 {
		t.Fatal("expected default row")
	}
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = "kick"
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false

	g.SetPlaying(true)
	advanceFrames(g, 5)
	if !g.Playing() {
		t.Fatal("expected playing after SetPlaying(true)")
	}

	openInstMenuViaLabel(t, dv, 0)
	clickInstButton(t, dv, "Snare")

	if dv.Rows[0].Instrument != "snare" {
		t.Fatalf("row instrument during playback: got %q, want %q", dv.Rows[0].Instrument, "snare")
	}
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu must stay open during playback selection")
	}
	if !g.Playing() {
		t.Fatal("playback must be uninterrupted by menu selection")
	}

	// Pick again to confirm second pick during playback also stays open.
	clickInstButton(t, dv, "Hihat")
	if dv.Rows[0].Instrument != "hihat" {
		t.Fatalf("row instrument after second pick during playback: got %q, want %q", dv.Rows[0].Instrument, "hihat")
	}
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu must stay open after second selection during playback")
	}
	if !g.Playing() {
		t.Fatal("playback must remain uninterrupted across multiple selections")
	}

	stopPlaybackForTest(g)
}

// TestInstrumentMenuCloseButtonClosesAfterSelection — explicit X dismissal
// must still work after the user has picked an instrument.
func TestInstrumentMenuCloseButtonClosesAfterSelection(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)
	clickInstButton(t, dv, "Snare")

	if !dv.IsInstMenuOpen() {
		t.Fatal("precondition: menu must still be open after selection")
	}

	closeBtn := dv.instMenuComp.CloseBtn()
	if closeBtn == nil {
		t.Fatal("close button missing")
	}
	r := closeBtn.Rect()
	if r.Empty() {
		t.Fatal("close button rect empty")
	}
	dv.instMenuComp.HandleInput(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, false)

	if dv.instMenuComp.IsOpen() {
		t.Fatal("menu component must close after X button click")
	}
}

// TestInstrumentMenuEscapeClosesAfterSelection — Escape must still close
// after the user has picked an instrument.
func TestInstrumentMenuEscapeClosesAfterSelection(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)
	clickInstButton(t, dv, "Snare")

	if !dv.IsInstMenuOpen() {
		t.Fatal("precondition: menu must still be open after selection")
	}

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	if dv.instMenuComp.IsOpen() {
		t.Fatal("menu component must close after Escape press")
	}
}

// TestInstrumentMenuClickOutsideClosesAfterSelection — clicking outside the
// menu's bounds (and outside the row label that anchors it) must still close
// after the user has picked an instrument.
func TestInstrumentMenuClickOutsideClosesAfterSelection(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)
	clickInstButton(t, dv, "Snare")

	if !dv.IsInstMenuOpen() {
		t.Fatal("precondition: menu must still be open after selection")
	}

	// Pick a coordinate far outside both the menu rect and the anchor rect.
	full := dv.instMenuComp.fullRect
	anchor := dv.instMenuComp.Props().AnchorRect
	outsideX := 1
	outsideY := 1
	if image.Pt(outsideX, outsideY).In(full) || image.Pt(outsideX, outsideY).In(anchor) {
		// Far corner instead.
		outsideX = 799
		outsideY = 599
	}

	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(outsideX, outsideY, true)

	if dv.instMenuComp.IsOpen() {
		t.Fatalf("menu component must close after click-outside at (%d,%d) (fullRect=%v anchor=%v)",
			outsideX, outsideY, full, anchor)
	}
}

// TestInstrumentMenuFavoriteStarDoesNotSelectOrClose — the star hit-rect is
// processed before the row button, so a tap on the star toggles the favorite
// without selecting the instrument or closing the menu.
func TestInstrumentMenuFavoriteStarDoesNotSelectOrClose(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	// Install a fresh in-memory favorites store so we can observe the toggle
	// without polluting the global state across tests.
	originalStore := Favorites()
	store := NewInMemoryFavoritesStore()
	SetFavoritesStore(store)
	t.Cleanup(func() { SetFavoritesStore(originalStore) })

	dv := auditionDV(t)
	openInstMenuViaLabel(t, dv, 0)

	if len(dv.instMenuComp.favRects) == 0 {
		t.Fatal("expected at least one star hit rect with a Favorites store wired in")
	}
	starRect := dv.instMenuComp.favRects[0]
	starID := dv.instMenuComp.favIDs[0]
	originalInstrument := dv.Rows[0].Instrument

	if store.Get(starID) {
		t.Fatalf("precondition: %q should not be favorited yet", starID)
	}

	// Star toggles fire on the press edge (drumview_overlay_inst_comp.go:1138).
	dv.instMenuComp.HandleInput(starRect.Min.X+starRect.Dx()/2, starRect.Min.Y+starRect.Dy()/2, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(starRect.Min.X+starRect.Dx()/2, starRect.Min.Y+starRect.Dy()/2, false)

	if !store.Get(starID) {
		t.Fatalf("expected %q to be favorited after star tap", starID)
	}
	if dv.Rows[0].Instrument != originalInstrument {
		t.Fatalf("star tap must NOT change row instrument: got %q, was %q", dv.Rows[0].Instrument, originalInstrument)
	}
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu must stay open after star toggle")
	}
}

// TestInstrumentMenuSwitchingRowsClosesPriorMenu — opening a different row's
// menu must close the previous one (CloseAllPopups exclusivity contract).
func TestInstrumentMenuSwitchingRowsClosesPriorMenu(t *testing.T) {
	assertDefaultParityState(t)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.instOptions = []string{"kick", "snare", "hihat"}
	dv.instCategories = nil
	dv.instCatByID = nil
	dv.Rows = []*DrumRow{
		{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0},
		{Name: "Snare", Instrument: "snare", Steps: make([]bool, 8), Volume: 1.0},
	}
	dv.Length = 8
	dv.bgDirty = true
	dv.calcLayout()
	dv.bgDirty = false
	if len(dv.rowLabels()) < 2 {
		t.Fatalf("expected at least 2 row labels, got %d", len(dv.rowLabels()))
	}

	// Open row 0's menu and pick something to confirm stay-open behavior.
	openInstMenuViaLabel(t, dv, 0)
	if dv.instMenuRow != 0 {
		t.Fatalf("instMenuRow=%d, want 0", dv.instMenuRow)
	}

	// Click row 1's label — openInstMenuForRow → CloseAllPopups → opens row 1.
	dv.rowLabels()[1].OnClick()
	suppressClicksUntilRelease = false

	if !dv.IsInstMenuOpen() {
		t.Fatal("row 1 menu should be open after clicking row 1 label")
	}
	if dv.instMenuRow != 1 {
		t.Fatalf("instMenuRow=%d, want 1", dv.instMenuRow)
	}
}
