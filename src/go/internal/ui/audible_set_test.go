//go:build test

package ui

import "testing"

func rowsFixture() []*DrumRow {
	return []*DrumRow{
		{Instrument: "kick"},
		{Instrument: "snare"},
		{Instrument: "hat"},
	}
}

func TestAudibleInstrumentIDs_NoSoloNoMute_All(t *testing.T) {
	got := audibleInstrumentIDs(rowsFixture())
	for _, id := range []string{"kick", "snare", "hat"} {
		if !got[id] {
			t.Fatalf("expected %q audible when nothing soloed/muted; got %v", id, got)
		}
	}
}

func TestAudibleInstrumentIDs_SingleSolo_OnlySoloed(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if !got["hat"] || got["kick"] || got["snare"] {
		t.Fatalf("single solo should yield only hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MultiSolo_SoloedSet(t *testing.T) {
	rows := rowsFixture()
	rows[1].Solo = true // snare
	rows[2].Solo = true // hat
	got := audibleInstrumentIDs(rows)
	if got["kick"] || !got["snare"] || !got["hat"] {
		t.Fatalf("multi-solo should yield snare+hat; got %v", got)
	}
}

func TestAudibleInstrumentIDs_MuteNoSolo_ExcludesMuted(t *testing.T) {
	rows := rowsFixture()
	rows[0].Muted = true // kick
	got := audibleInstrumentIDs(rows)
	if got["kick"] || !got["snare"] || !got["hat"] {
		t.Fatalf("mute should exclude kick; got %v", got)
	}
}

func TestAudibleInstrumentIDs_SoloAndMuted_Excluded(t *testing.T) {
	rows := rowsFixture()
	rows[2].Solo = true
	rows[2].Muted = true // muted overrides solo
	got := audibleInstrumentIDs(rows)
	if got["hat"] {
		t.Fatalf("a muted-and-soloed row must be inaudible; got %v", got)
	}
}

func TestAudibleInstrumentIDs_EmptyInstrumentIDIgnored(t *testing.T) {
	rows := []*DrumRow{{Instrument: ""}, {Instrument: "kick"}}
	got := audibleInstrumentIDs(rows)
	if _, ok := got[""]; ok {
		t.Fatalf("empty instrument id must not be a key; got %v", got)
	}
	if !got["kick"] {
		t.Fatalf("kick should be audible; got %v", got)
	}
}
