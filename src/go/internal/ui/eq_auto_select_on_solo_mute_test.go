//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// With exactly one audible row the channel dropdown switches to it, and every tab follows the dropdown.

// Distinct ids (channel id == instrument id); the channel starts at Master.
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

// On Master the Synth tab falls back to the first non-empty row, by design.
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

// Muting all but one selects it; muting one of three does not.
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

// With more than one audible there is no single focus.
func TestUnSoloRevertsToMasterWhenMultipleAudible(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleSolo(1)
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.toggleSolo(1) // un-solo -> all 3 audible -> revert to Master
	assertChannelStaysMaster(t, g)
}

func TestNoAutoSelectWhenMultipleAudible(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleMute(0) // two still audible
	assertChannelStaysMaster(t, g)
}

func TestUnMuteBackToMultipleRevertsToMaster(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)
	g.drum.toggleMute(0) // kick muted; snare+hat audible (2) -> stays Master
	g.drum.toggleMute(2) // hat muted; snare sole-audible -> auto-select snare
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.toggleMute(0) // un-mute kick; kick+snare audible (2) -> revert to Master
	assertChannelStaysMaster(t, g)
}

// A manual selection reverts too once more than one is audible.
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

// Selecting through it moves every tab; Master resets them (Synth falls back to the first row).
func TestOnChannelChangeDrivesAllTabs(t *testing.T) {
	g := newAutoSelectGame(t, roster.Kick, roster.Snare, roster.Hihat)

	g.drum.eqPanelZone.callbacks.OnChannelChange(channel.Of(roster.Snare))
	assertChannelAllTabs(t, g, channel.Of(roster.Snare))

	g.drum.eqPanelZone.callbacks.OnChannelChange(channel.Master)
	assertChannelAllTabs(t, g, channel.Master)
}

// The cycle button and rename keep eqPanelZone.activeChannel in step with eqActiveChannel.
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
