package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The tracked channel follows the row to its new instrument, and the button shows the new name.
func TestEQActiveChannelFollowsInstrumentChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"

	dv.selRow = 0
	dv.setEQActiveChannel(channel.Of(roster.Kick))
	if dv.eqActiveChannel != channel.Of(roster.Kick) {
		t.Fatalf("setup: eqActiveChannel = %q, want %q", dv.eqActiveChannel, "kick")
	}

	dv.SetInstrument(roster.Hihat)

	if dv.eqActiveChannel != channel.Of(roster.Hihat) {
		t.Fatalf("eqActiveChannel = %q, want %q (EQ should follow row's new instrument)", dv.eqActiveChannel, "hihat")
	}
	if dv.eqChannelBtn() == nil {
		t.Fatalf("eqChannelBtn is nil")
	}
	// Against the row name, which depends on catalogue state, so the contract holds whatever ran before.
	if got, want := dv.eqChannelBtn().Text, dv.Rows[dv.selRow].Name; got != want {
		t.Fatalf("eqChannelBtn.Text = %q, want %q (the new row name)", got, want)
	}
}

// Another row's change leaves the channel alone.
func TestEQActiveChannelUnchangedWhenNotTracking(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = roster.Snare
	dv.Rows[1].Name = "Snare"

	dv.setEQActiveChannel(channel.Of(roster.Snare))
	if dv.eqActiveChannel != channel.Of(roster.Snare) {
		t.Fatalf("setup: eqActiveChannel = %q, want %q", dv.eqActiveChannel, "snare")
	}

	dv.selRow = 0
	dv.SetInstrument(roster.Hihat)

	if dv.eqActiveChannel != channel.Of(roster.Snare) {
		t.Fatalf("eqActiveChannel = %q, want %q (EQ was not tracking the changed row)", dv.eqActiveChannel, "snare")
	}
}

// So the menu never shows stale entries.
func TestEQDropdownClosedWhenTrackedInstrumentChanges(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	dv := g.drum
	dv.recalcButtons()

	dv.AddRow()
	dv.Rows[0].Instrument = roster.Kick
	dv.Rows[0].Name = "Kick"
	dv.Rows[1].Instrument = roster.Snare
	dv.Rows[1].Name = "Snare"

	dv.selRow = 0
	dv.setEQActiveChannel(channel.Of(roster.Kick))

	// Through real input.
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
	dv.SetInstrument(roster.Hihat)

	if dv.eqPanelZone.ChannelDropdownOpen() {
		t.Fatalf("ChannelDropdownOpen() = true after instrument change, want false")
	}
	if dv.eqActiveChannel != channel.Of(roster.Hihat) {
		t.Fatalf("eqActiveChannel = %q, want %q", dv.eqActiveChannel, "hihat")
	}
}
