//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func rowsFixture() []*DrumRow {
	return []*DrumRow{
		{Instrument: roster.Kick},
		{Instrument: roster.Snare},
		{Instrument: roster.Hihat},
	}
}

func TestAudibleInstrumentIDs_NoSoloNoMute_All(t *testing.T) {
	got := audibleInstrumentIDs(rowsFixture())
	for _, id := range []instrument.ID{roster.Kick, roster.Snare, roster.Hihat} {
		if !got[channel.Of(id)] {
			t.Fatalf("expected %q audible when nothing soloed/muted; got %v", id, got)
		}
	}
}

func TestAudibleInstrumentIDs_SingleSolo_OnlySoloed(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if !got[channel.Of(roster.Hihat)] || got[channel.Of(roster.Kick)] || got[channel.Of(roster.Snare)] {
		t.Fatalf("single solo should yield only hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MultiSolo_SoloedSet(t *testing.T) {
	rows := rowsFixture()
	rows[1].Solo = true // snare
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if got[channel.Of(roster.Kick)] || !got[channel.Of(roster.Snare)] || !got[channel.Of(roster.Hihat)] {
		t.Fatalf("multi-solo should yield snare+hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MuteNoSolo_ExcludesMuted(t *testing.T) {
	rows := rowsFixture()
	rows[0].Muted = true // kick
	got := audibleInstrumentIDs(rows)
	if got[channel.Of(roster.Kick)] || !got[channel.Of(roster.Snare)] || !got[channel.Of(roster.Hihat)] {
		t.Fatalf("mute should exclude kick; got %v", got)
	}
}

func TestAudibleInstrumentIDs_SoloAndMuted_Excluded(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true
	rows[2].Muted = true // muted overrides solo
	got := audibleInstrumentIDs(rows)
	if got[channel.Of(roster.Hihat)] {
		t.Fatalf("a muted-and-soloed row must be inaudible; got %v", got)
	}
}

func TestAudibleInstrumentIDs_EmptyInstrumentIDIgnored(t *testing.T) {
	rows := []*DrumRow{{Instrument: ""}, {Instrument: roster.Kick}}
	got := audibleInstrumentIDs(rows)
	if _, ok := got[channel.ID{}]; ok {
		t.Fatalf("empty instrument id must not be a key; got %v", got)
	}
	if !got[channel.Of(roster.Kick)] {
		t.Fatalf("kick should be audible; got %v", got)
	}
}
