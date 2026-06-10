package ui

import (
	"testing"
)

// TestActiveEQChannelDefaultsToMain pins the documented fallback: an
// unset eqActiveChannel resolves to "main" so audio.SetChannelEQ targets
// the master channel instead of an empty-string id.
func TestActiveEQChannelDefaultsToMain(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.eqActiveChannel = ""
	if got := dv.activeEQChannel(); got != "main" {
		t.Errorf("activeEQChannel() with empty eqActiveChannel = %q, want %q", got, "main")
	}
	dv.eqActiveChannel = "kick"
	if got := dv.activeEQChannel(); got != "kick" {
		t.Errorf("activeEQChannel() = %q, want %q", got, "kick")
	}
}

// TestEnsureRowEQLazyInit verifies that ensureRowEQ allocates a band
// gain slice with the correct length on first call, and that a second
// call leaves it untouched (no realloc, no zeroing of user values).
func TestEnsureRowEQLazyInit(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one default row")
	}
	row := 0
	dv.Rows[row].EQGainsDB = nil
	dv.ensureRowEQ(row)
	if got, want := len(dv.Rows[row].EQGainsDB), len(eqBandDefs); got != want {
		t.Errorf("first ensureRowEQ: len(EQGainsDB)=%d, want %d", got, want)
	}
	// Mark a known value, ensure idempotent call doesn't clobber it.
	dv.Rows[row].EQGainsDB[3] = 4.2
	dv.ensureRowEQ(row)
	if got := dv.Rows[row].EQGainsDB[3]; got != 4.2 {
		t.Errorf("idempotent ensureRowEQ clobbered user value: got %v, want 4.2", got)
	}
	// Out-of-range row must be a no-op (not panic).
	dv.ensureRowEQ(-1)
	dv.ensureRowEQ(len(dv.Rows) + 10)
}

// TestEnsureRowEQMutedLazyInit mirrors TestEnsureRowEQLazyInit for the
// per-band mute slice. The bridge that gates rendering (EQBandMuted) is
// loaded by EQ Draw, so a nil/short slice would silently mute nothing.
func TestEnsureRowEQMutedLazyInit(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := 0
	dv.Rows[row].EQBandMuted = nil
	dv.ensureRowEQMuted(row)
	if got, want := len(dv.Rows[row].EQBandMuted), len(eqBandDefs); got != want {
		t.Errorf("first ensureRowEQMuted: len(EQBandMuted)=%d, want %d", got, want)
	}
	dv.Rows[row].EQBandMuted[2] = true
	dv.ensureRowEQMuted(row)
	if !dv.Rows[row].EQBandMuted[2] {
		t.Errorf("idempotent ensureRowEQMuted clobbered user value")
	}
}

// TestActiveHPFCutoffHzFallback covers the per-row cutoff defaulting:
// when the row's HPFCutoffHz is unset (≤0), activeHPFCutoffHz must
// return 20 (the band-floor sentinel) instead of zero / negative.
func TestActiveHPFCutoffHzFallback(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := dv.Rows[0]
	row.Instrument = "kick"
	row.HPFCutoffHz = 0
	dv.eqActiveChannel = "kick"
	if got := dv.activeHPFCutoffHz(); got != 20 {
		t.Errorf("activeHPFCutoffHz() with zero cutoff = %v, want 20", got)
	}
	row.HPFCutoffHz = 240
	if got := dv.activeHPFCutoffHz(); got != 240 {
		t.Errorf("activeHPFCutoffHz() = %v, want 240", got)
	}

	// Switch to main — must read dv.hpfCutoffHz, not row state.
	dv.eqActiveChannel = "main"
	dv.hpfCutoffHz = 75
	if got := dv.activeHPFCutoffHz(); got != 75 {
		t.Errorf("activeHPFCutoffHz(main) = %v, want 75", got)
	}
}

// TestActiveLPFCutoffHzFallback is the LPF counterpart. Default sentinel
// is 20000 Hz (Nyquist-adjacent).
func TestActiveLPFCutoffHzFallback(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := dv.Rows[0]
	row.Instrument = "snare"
	row.LPFCutoffHz = 0
	dv.eqActiveChannel = "snare"
	if got := dv.activeLPFCutoffHz(); got != 20000 {
		t.Errorf("activeLPFCutoffHz() with zero cutoff = %v, want 20000", got)
	}
	row.LPFCutoffHz = 8000
	if got := dv.activeLPFCutoffHz(); got != 8000 {
		t.Errorf("activeLPFCutoffHz() = %v, want 8000", got)
	}
}

// TestOnRowInstrumentChangedFollowsRename is the invariant guard from
// CLAUDE.md § Event Notification: when the row's instrument id changes
// and the EQ panel was viewing the old id, the panel's active channel
// must move to the new id atomically. The previous bug let the EQ panel
// silently target a stale (now-orphan) channel after a rename.
func TestOnRowInstrumentChangedFollowsRename(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	row := 0
	dv.Rows[row].Instrument = "kick"
	dv.eqActiveChannel = "kick"

	dv.onRowInstrumentChanged(row, "kick", "kick2")
	if dv.eqActiveChannel != "kick2" {
		t.Errorf("after rename kick→kick2, eqActiveChannel=%q, want %q", dv.eqActiveChannel, "kick2")
	}

	// Renaming a non-active row leaves the active channel alone.
	dv.eqActiveChannel = "main"
	dv.onRowInstrumentChanged(row, "kick2", "kick3")
	if dv.eqActiveChannel != "main" {
		t.Errorf("rename of inactive row mutated active channel: %q", dv.eqActiveChannel)
	}

	// No-op rename (oldID == newID) must be a fast return.
	dv.eqActiveChannel = "drums"
	dv.onRowInstrumentChanged(row, "drums", "drums")
	if dv.eqActiveChannel != "drums" {
		t.Errorf("no-op rename mutated channel: %q", dv.eqActiveChannel)
	}
}
