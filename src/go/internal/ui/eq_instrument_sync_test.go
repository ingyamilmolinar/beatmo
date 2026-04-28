package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEQActiveChannelFollowsInstrumentChange verifies the regression: when
// a row's instrument is changed via SetInstrument and the EQ panel is
// currently tracking that row's old instrument, the EQ active channel
// must follow to the new instrument id and the channel button label
// must reflect the new row name.
func TestEQActiveChannelFollowsInstrumentChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = "kick"
	dv.Rows[0].Name = "Kick"

	dv.selRow = 0
	dv.setEQActiveChannel("kick")
	if dv.eqActiveChannel != "kick" {
		t.Fatalf("setup: eqActiveChannel = %q, want %q", dv.eqActiveChannel, "kick")
	}

	dv.SetInstrument("hihat")

	if dv.eqActiveChannel != "hihat" {
		t.Fatalf("eqActiveChannel = %q, want %q (EQ should follow row's new instrument)", dv.eqActiveChannel, "hihat")
	}
	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}
	if got := dv.eqChannelBtn().Text; got != "Hihat" {
		t.Fatalf("eqChannelBtn.Text = %q, want %q", got, "Hihat")
	}
}

// TestEQActiveChannelUnchangedWhenNotTracking verifies the guard
// condition: when the EQ panel is tracking instrument X but a different
// row's instrument is changed, the EQ active channel must not move.
func TestEQActiveChannelUnchangedWhenNotTracking(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = "kick"
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = "snare"
	dv.Rows[1].Name = "Snare"

	dv.setEQActiveChannel("snare")
	if dv.eqActiveChannel != "snare" {
		t.Fatalf("setup: eqActiveChannel = %q, want %q", dv.eqActiveChannel, "snare")
	}

	dv.selRow = 0
	dv.SetInstrument("hihat")

	if dv.eqActiveChannel != "snare" {
		t.Fatalf("eqActiveChannel = %q, want %q (EQ was not tracking the changed row)", dv.eqActiveChannel, "snare")
	}
}

// TestEQDropdownClosedWhenTrackedInstrumentChanges verifies that an open
// channel dropdown is closed when the instrument it points at changes,
// so the menu does not display stale entries.
func TestEQDropdownClosedWhenTrackedInstrumentChanges(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = "kick"
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = "snare"
	dv.Rows[1].Name = "Snare"

	dv.selRow = 0
	dv.setEQActiveChannel("kick")

	// Open the dropdown via real input simulation, mirroring
	// TestEQChannelDropdownOpensAndSelects.
	btnRect := dv.eqChannelBtn().Rect()
	cx, cy := (btnRect.Min.X+btnRect.Max.X)/2, (btnRect.Min.Y+btnRect.Max.Y)/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	// Release frame.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	dv.Update()
	restore()

	if dv.eqPanelZone == nil || !dv.eqPanelZone.ChannelDropdownOpen() {
		t.Fatalf("dropdown failed to open in setup; cannot verify auto-close")
	}

	// Mutating the tracked instrument must close the dropdown.
	dv.SetInstrument("hihat")

	if dv.eqPanelZone.ChannelDropdownOpen() {
		t.Fatalf("ChannelDropdownOpen() = true after instrument change, want false")
	}
	if dv.eqActiveChannel != "hihat" {
		t.Fatalf("eqActiveChannel = %q, want %q", dv.eqActiveChannel, "hihat")
	}
}
