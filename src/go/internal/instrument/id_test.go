package instrument

import (
	"errors"
	"testing"
)

// TestBase covers the suffix range and, more importantly, the ids that must
// NOT be read as instances. Chord-lane splitting in gen-showcase produces a
// third and fourth row on one instrument, so the strip has to reach past
// "--2" without swallowing a real id like "bass-808".
func TestBase(t *testing.T) {
	cases := map[ID]ID{
		"organ--2":        "organ",
		"piano-grand--3":  "piano-grand",
		"guitar-nylon--4": "guitar-nylon",
		"voice-ahh--9":    "voice-ahh",
		"violin--1":       "violin",
		"x--1":            "x",

		// Not instances.
		"bass-808": "bass-808",
		"organ":    "organ",
		"organ-0":  "organ-0",
		"organ-2":  "organ-2", // a single hyphen + digit is just an id
		"organ-10": "organ-10",
		"organ--0": "organ--0",
		"organ--x": "organ--x",
		"--1":      "--1",
		"":         "",
	}
	for in, want := range cases {
		if got := in.Base(); got != want {
			t.Errorf("%q.Base() = %q, want %q", in, got, want)
		}
		if got := in.IsInstance(); got != (want != in) {
			t.Errorf("%q.IsInstance() = %v", in, got)
		}
	}
}

func TestInstanceRoundTripsThroughBase(t *testing.T) {
	for n := 2; n <= 9; n++ {
		id, err := Instance("organ", n)
		if err != nil {
			t.Fatal(err)
		}
		if id.Base() != "organ" || !id.IsInstance() {
			t.Fatalf("Instance(organ, %d) = %q does not strip back to organ", n, id)
		}
	}
	for _, n := range []int{-1, 0, 1, 10} {
		if _, err := Instance("organ", n); err == nil {
			t.Errorf("Instance(organ, %d) accepted a number Base cannot read back", n)
		}
	}
	if _, err := Instance("", 2); err == nil {
		t.Error("Instance of no instrument accepted")
	}
}

func TestParseID(t *testing.T) {
	for _, ok := range []string{"oboe", "kick-808", "user.my-kick.a1b2c3", "user.sample.0000abcd", "organ--2", "kick-acoustic-foo_bar"} {
		if id, err := ParseID(ok); err != nil || string(id) != ok {
			t.Errorf("ParseID(%q) = %q, %v", ok, id, err)
		}
	}
	if _, err := ParseID(""); !errors.Is(err, ErrEmptyID) {
		t.Errorf("ParseID(\"\") = %v, want ErrEmptyID", err)
	}
	for _, bad := range []string{ReservedMasterName, "my kick", "kick\n", "\tx"} {
		if _, err := ParseID(bad); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ParseID(%q) = %v, want ErrInvalidID", bad, err)
		}
	}
	if _, err := ParseIDs([]string{"oboe", ""}); err == nil {
		t.Error("ParseIDs accepted an empty id")
	}
}

func TestMintedUserIDs(t *testing.T) {
	if got := UserID("My Kick!", "a1b2c3"); got != "user.my-kick.a1b2c3" || !got.IsUser() {
		t.Errorf("UserID = %q", got)
	}
	if got := UserSampleID(0xabcd); got != "user.sample.0000abcd" || !got.IsUser() {
		t.Errorf("UserSampleID = %q", got)
	}
	if ID("oboe").IsUser() {
		t.Error("a roster id read as a user id")
	}
	for _, id := range []ID{UserID("x", "1"), UserSampleID(1)} {
		if _, err := ParseID(string(id)); err != nil {
			t.Errorf("a minted id does not parse: %v", err)
		}
	}
}

func TestSampleID(t *testing.T) {
	cases := map[string]ID{
		"Snare/Hit 01":  "snare-hit-01",
		"kick_heavy":    "kick-heavy",
		"path/to/file":  "path-to-file",
		"--leading--":   "leading",
		"UPPER CASE":    "upper-case",
		"a--2":          "a-2", // never an instance
		"Kick (Synth)!": "kick-synth",
		"main":          "sample-main",
		"":              "sample",
	}
	for in, want := range cases {
		got := SampleID(in)
		if got != want {
			t.Errorf("SampleID(%q) = %q, want %q", in, got, want)
		}
		if got.IsInstance() {
			t.Errorf("SampleID(%q) = %q reads as an instance", in, got)
		}
		if _, err := ParseID(string(got)); err != nil {
			t.Errorf("SampleID(%q) = %q does not parse: %v", in, got, err)
		}
	}
}
