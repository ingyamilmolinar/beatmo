//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// These tests cover the "auto-select the sole audible instrument" QoL feature:
// when exactly one row is audible (soloed, or all-but-one muted), the audio-panel
// channel dropdown auto-switches to that instrument, and EVERY audio-panel tab
// follows the dropdown as the single source of truth.
//
// Design: docs/superpowers/specs/2026-05-27-auto-select-sole-audible-channel-design.md

// newAutoSelectGame builds a Game whose drum view has one row per supplied
// instrument id (distinct, since channel id == instrument id). The active
// channel starts at Master.
func newAutoSelectGame(t *testing.T, instruments ...instrument.ID) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	for len(g.drum.Rows) < len(instruments) {
		g.drum.AddRow()
	}
	for i, inst := range instruments {
		g.drum.Rows[i].Instrument = inst
	}
	return g
}

func firstNonEmptyInstrument(dv *DrumView) instrument.ID {
	for _, r := range dv.Rows {
		if r != nil && r.Instrument != "" {
			return r.Instrument
		}
	}
	return ""
}

// assertChannelAllTabs asserts that every audio-panel tab's instrument-selection
// observable resolves to want. The Synth tab falls back to the first non-empty
// row when Master is selected, by design.
func assertChannelAllTabs(t *testing.T, g *Game, want channel.ID) {
	t.Helper()
	dv := g.drum
	if got := dv.activeEQChannel(); got != want {
		t.Errorf("EQ tab: activeEQChannel() = %q, want %q", got, want)
	}
	if got := dv.eqPanelZone.ActiveChannel(); got != want {
		t.Errorf("Wave/Spectrum tabs: eqPanelZone.ActiveChannel() = %q, want %q", got, want)
	}
	wantSynth, ok := want.Instrument()
	if !ok {
		wantSynth = firstNonEmptyInstrument(dv)
	}
	if got := dv.synthTabActiveInstrument(); got != wantSynth {
		t.Errorf("Synth tab: synthTabActiveInstrument() = %q, want %q", got, wantSynth)
	}
	if z := dv.eqPanelZone.chainZone; z != nil {
		if got := z.instrumentID; got != want {
			t.Errorf("Chain tab: chainZone.instrumentID = %q, want %q", got, want)
		}
	} else {
		t.Fatalf("chainZone is nil; cannot verify Chain tab follows the dropdown")
	}
}

// assertChannelStaysMaster asserts the dropdown has not switched to an
// instrument (used for the negative / "stay put" cases).
func assertChannelStaysMaster(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	if got := dv.activeEQChannel(); got != channel.Master {
		t.Errorf("expected channel to stay Master, activeEQChannel() = %q", got)
	}
	if got := dv.eqPanelZone.ActiveChannel(); got != channel.Master {
		t.Errorf("expected channel to stay Master, eqPanelZone.ActiveChannel() = %q", got)
	}
}

// 1. Soloing a single instrument auto-selects it on every tab.
func TestSoloAutoSelectsInstrumentAllTabs(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleSolo(1)
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))
}

//  2. Muting all-but-one auto-selects the remaining instrument; muting only one
//     of three (two still audible) does not.
func TestMuteAllButOneAutoSelects(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)

	g.drum.toggleMute(0) // kick muted; snare+hat audible (2) -> no auto-select
	assertChannelStaysMaster(t, g)

	g.drum.toggleMute(2) // hat muted; only snare audible (1) -> auto-select snare
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))
}

// 3. Re-soloing a different instrument moves the selection (single-solo model).
func TestReSoloMovesSelection(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleSolo(1)
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))
	g.drum.toggleSolo(2)
	assertChannelAllTabs(t, g, channel.Of(roster.Hihat))
}

//  4. Leaving the single-audible state (un-solo) reverts the selection to
//     Master: with more than one instrument audible there is no single focus,
//     so the selector goes back to Master.
func TestUnSoloRevertsToMasterWhenMultipleAudible(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleSolo(1)
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.toggleSolo(1) // un-solo -> all 3 audible -> revert to Master
	assertChannelStaysMaster(t, g)
}

//  5. No instrument is auto-selected while two or more remain audible; if one
//     was previously selected the selector reverts to Master.
func TestNoAutoSelectWhenMultipleAudible(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleMute(0) // two still audible
	assertChannelStaysMaster(t, g)
}

// 5b. Un-muting back into a multi-audible state reverts a previously
//
//	auto-selected instrument to Master.
func TestUnMuteBackToMultipleRevertsToMaster(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleMute(0) // kick muted; snare+hat audible (2) -> stays Master
	g.drum.toggleMute(2) // hat muted; snare sole-audible -> auto-select snare
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.toggleMute(0) // un-mute kick; kick+snare audible (2) -> revert to Master
	assertChannelStaysMaster(t, g)
}

// 5c. A MANUAL channel selection also reverts to Master once a mute/solo
//
//	change leaves more than one instrument audible.
func TestManualSelectionRevertsToMasterWhenMultipleAudible(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.eqPanelZone.callbacks.OnChannelChange(channel.Of(roster.Kick)) // manually focus kick
	assertChannelAllTabs(t, g, channel.Of(roster.Kick))

	g.drum.toggleMute(2) // mute hat; kick+snare audible (2) -> revert to Master
	assertChannelStaysMaster(t, g)
}

// 6. A single-row project never auto-switches away from Master.
func TestSingleRowNoAutoSelect(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick)
	g.drum.toggleSolo(0)
	assertChannelStaysMaster(t, g)
	g.drum.toggleMute(0)
	assertChannelStaysMaster(t, g)
}

// 7. A sole-audible row with an empty instrument id is not auto-selected.
func TestEmptyInstrumentNotAutoSelected(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare)
	g.drum.Rows[1].Instrument = "" // sole-audible target has no channel id
	g.drum.toggleSolo(1)
	assertChannelStaysMaster(t, g)
}

//  8. The dropdown's OnChannelChange callback is the single source of truth:
//     selecting a channel through it updates EVERY tab's observable, and
//     resetting to Master resets them (Synth falls back to first row).
func TestOnChannelChangeDrivesAllTabs(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)

	g.drum.eqPanelZone.callbacks.OnChannelChange(channel.Of(roster.Snare))
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.eqPanelZone.callbacks.OnChannelChange(channel.Master)
	assertChannelAllTabs(t, g, channel.Master)
}

//  9. Consolidation regression: the channel-cycle button and row-instrument
//     rename keep eqPanelZone.activeChannel in lockstep with eqActiveChannel.
//     Previously only the dropdown click synced z.activeChannel, so Wave/
//     Spectrum/Synth tabs ignored these entry points.
func TestCycleAndRenameSyncPanelChannel(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)

	g.drum.cycleEQChannel() // main -> first instrument (kick)
	if got := g.drum.activeEQChannel(); got != channel.Of(roster.Kick) {
		t.Fatalf("cycleEQChannel: activeEQChannel() = %q, want kick", got)
	}
	if got := g.drum.eqPanelZone.ActiveChannel(); got != channel.Of(roster.Kick) {
		t.Errorf("cycleEQChannel: eqPanelZone.ActiveChannel() = %q, want kick (must follow)", got)
	}

	// Rename the active instrument; the panel channel must follow the new id.
	g.drum.Rows[0].Instrument = roster.KickDeep
	g.drum.onRowInstrumentChanged(0, roster.Kick, roster.KickDeep)
	if got := g.drum.activeEQChannel(); got != channel.Of(roster.KickDeep) {
		t.Fatalf("rename: activeEQChannel() = %q, want kick2", got)
	}
	if got := g.drum.eqPanelZone.ActiveChannel(); got != channel.Of(roster.KickDeep) {
		t.Errorf("rename: eqPanelZone.ActiveChannel() = %q, want kick2 (must follow)", got)
	}
}
