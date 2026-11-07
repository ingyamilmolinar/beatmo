package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The zero channel is no bus, never a spelling of master.
func TestActiveEQChannelStartsOnMasterAndRefusesNoChannel(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	if got := dv.activeEQChannel(); got != channel.Master {
		t.Errorf("activeEQChannel() of a new view = %q, want %q", got, channel.Master)
	}
	dv.selectAudioChannel(channel.Of(roster.Kick))
	dv.selectAudioChannel(channel.ID{})
	if got := dv.activeEQChannel(); got != channel.Of(roster.Kick) {
		t.Errorf("a switch to no channel moved the EQ view to %q", got)
	}
	dv.eqActiveChannel = channel.Of(roster.Kick)
	if got := dv.activeEQChannel(); got != channel.Of(roster.Kick) {
		t.Errorf("activeEQChannel() = %q, want %q", got, "kick")
	}
}

// An unset cutoff (≤ 0) reads as 20, the band-floor sentinel.
func TestActiveHPFCutoffHzFallback(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := dv.Rows[0]
	row.Instrument = roster.Kick
	rowEQ(row).HPFHz = 0
	dv.eqActiveChannel = channel.Of(roster.Kick)
	if got := dv.activeHPFCutoffHz(); got != 20 {
		t.Errorf("activeHPFCutoffHz() with zero cutoff = %v, want 20", got)
	}
	rowEQ(row).HPFHz = 240
	if got := dv.activeHPFCutoffHz(); got != 240 {
		t.Errorf("activeHPFCutoffHz() = %v, want 240", got)
	}

	// Switch to main — must read dv.hpfCutoffHz, not row state.
	dv.eqActiveChannel = channel.Master
	dv.hpfCutoffHz = 75
	if got := dv.activeHPFCutoffHz(); got != 75 {
		t.Errorf("activeHPFCutoffHz(main) = %v, want 75", got)
	}
}

// An unset cutoff reads as 20000.
func TestActiveLPFCutoffHzFallback(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := dv.Rows[0]
	row.Instrument = roster.Snare
	rowEQ(row).LPFHz = 0
	dv.eqActiveChannel = channel.Of(roster.Snare)
	if got := dv.activeLPFCutoffHz(); got != 20000 {
		t.Errorf("activeLPFCutoffHz() with zero cutoff = %v, want 20000", got)
	}
	rowEQ(row).LPFHz = 8000
	if got := dv.activeLPFCutoffHz(); got != 8000 {
		t.Errorf("activeLPFCutoffHz() = %v, want 8000", got)
	}
}

// The panel's channel must move with the rename, or it targets an orphan channel.
func TestOnRowInstrumentChangedFollowsRename(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := 0
	dv.Rows[row].Instrument = roster.Kick
	dv.eqActiveChannel = channel.Of(roster.Kick)

	dv.onRowInstrumentChanged(row, roster.Kick, roster.KickDeep)
	if dv.eqActiveChannel != channel.Of(roster.KickDeep) {
		t.Errorf("after rename kick→kick2, eqActiveChannel=%q, want %q", dv.eqActiveChannel, "kick2")
	}

	// Renaming a non-active row leaves the active channel alone.
	dv.eqActiveChannel = channel.Master
	dv.onRowInstrumentChanged(row, roster.KickDeep, roster.KickLofi)
	if dv.eqActiveChannel != channel.Master {
		t.Errorf("rename of inactive row mutated active channel: %q", dv.eqActiveChannel)
	}

	// No-op rename (oldID == newID) must be a fast return.
	dv.eqActiveChannel = channel.Of(roster.Snare)
	dv.onRowInstrumentChanged(row, roster.Snare, roster.Snare)
	if dv.eqActiveChannel != channel.Of(roster.Snare) {
		t.Errorf("no-op rename mutated channel: %q", dv.eqActiveChannel)
	}
}
