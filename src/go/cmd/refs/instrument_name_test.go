//go:build !js

package main

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The drop-in directory is self-describing: the filename names the
// instrument, resolved against the GAME's registry (roster.All),
// so dropping snare-lofi.wav in and running a scan is the whole workflow.
func TestInstrumentFromFilename_Resolves(t *testing.T) {
	cases := []struct {
		file string
		want instrument.ID
		why  string
	}{
		{"snare.wav", roster.Snare, "the plain base id"},
		{"snare-lofi.wav", roster.SnareLofi, "longest match wins; must NOT collapse to snare"},
		{"kick-lofi.wav", roster.KickLofi, "the variants the catalogue never mentions"},
		{"snare-lofi_garage_take3.wav", roster.SnareLofi, "a take suffix after the id"},
		{"Kick_Deep.wav", roster.KickDeep, "case and underscores normalise"},
		{"kick deep.wav", roster.KickDeep, "spaces normalise too"},
		{"cello-warm.aiff", roster.CelloWarm, "a character variant, any audio extension"},
		{"fm-bell-soft.wav", roster.FmBellSoft, "a hyphenated id whose own suffix is a digit"},
	}
	for _, c := range cases {
		got, err := instrumentFromFilename(c.file)
		if err != nil {
			t.Errorf("instrumentFromFilename(%q): %v (%s)", c.file, err, c.why)
			continue
		}
		if got != c.want {
			t.Errorf("instrumentFromFilename(%q) = %q, want %q (%s)", c.file, got, c.want, c.why)
		}
	}
}

// At a SEGMENT boundary: a bare substring would make "mycrash.wav" a crash reference.
func TestInstrumentFromFilename_RejectsANameThatMerelyContainsAnID(t *testing.T) {
	for _, name := range []string{"mycrash.wav", "backing-track.wav", "take3.wav"} {
		if got, err := instrumentFromFilename(name); err == nil {
			t.Errorf("instrumentFromFilename(%q) = %q, want an error", name, got)
		}
	}
}

// An unresolvable name must say what to do about it, not just refuse.
func TestInstrumentFromFilename_SuggestsNearMisses(t *testing.T) {
	_, err := instrumentFromFilename("snare2.wav")
	if err == nil {
		t.Fatal("instrumentFromFilename accepted snare2.wav")
	}
	if !strings.Contains(err.Error(), "snare") {
		t.Errorf("error suggests no near miss for snare2: %v", err)
	}
}

// The registry is the authority, so an id that is not a registered
// instrument cannot be adopted by filename even if it looks plausible.
func TestInstrumentFromFilename_RejectsAnUnregisteredID(t *testing.T) {
	if got, err := instrumentFromFilename("didgeridoo.wav"); err == nil {
		t.Errorf("instrumentFromFilename(didgeridoo.wav) = %q, want an error", got)
	}
}

// A scan reports every file it could not claim, so the message has to be
// readable in a list: the basename, not the absolute path it was handed.
func TestInstrumentFromFilename_ErrorNamesTheFileNotItsFullPath(t *testing.T) {
	_, err := instrumentFromFilename("/home/someone/.beatmo/wav-refs/sources/backing-track.wav")
	if err == nil {
		t.Fatal("instrumentFromFilename accepted backing-track.wav")
	}
	if strings.Contains(err.Error(), "/home/someone") {
		t.Errorf("error carries the full path instead of the filename: %v", err)
	}
	if !strings.Contains(err.Error(), "backing-track.wav") {
		t.Errorf("error does not name the file: %v", err)
	}
}

// A two-character prefix offers irrelevant ids; better to state the rule.
func TestInstrumentFromFilename_DoesNotSuggestOnATwoLetterCoincidence(t *testing.T) {
	_, err := instrumentFromFilename("backing-track.wav")
	if err == nil {
		t.Fatal("instrumentFromFilename accepted backing-track.wav")
	}
	if strings.Contains(err.Error(), "banjo") {
		t.Errorf("error offers a two-letter coincidence as a near miss: %v", err)
	}
}
