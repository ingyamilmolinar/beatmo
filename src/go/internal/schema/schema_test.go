package schema

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestValidateStrictRule pins the accept / reject table every format applies:
// only the current version is accepted — an absent version is rejected like
// any other mismatch (no pre-versioning migration).
func TestValidateStrictRule(t *testing.T) {
	cases := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"current version accepted", Version, false},
		{"absent version rejected", "", true},
		{"older version rejected", "0.0.0", true},
		{"newer version rejected", "0.0.2", true},
		{"future major rejected", "1.0.0", true},
		{"garbage rejected", "not-a-version", true},
		{"legacy int form rejected", "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Envelope{SchemaVersion: tc.version}.Validate("project")
			if tc.wantErr && err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.version)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", tc.version, err)
			}
		})
	}
}

// TestVersionErrorNamesFormatWantGot ensures a rejection is diagnosable:
// the message must say which format, what was expected, and what was found.
func TestVersionErrorNamesFormatWantGot(t *testing.T) {
	err := Envelope{SchemaVersion: "9.9.9"}.Validate("instrument bundle")
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	for _, want := range []string{"instrument bundle", "9.9.9", Version, Field} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	var ve *VersionError
	if !errors.As(err, &ve) {
		t.Fatalf("error is %T, want *VersionError", err)
	}
	if ve.Got != "9.9.9" || ve.Want != Version || ve.Format != "instrument bundle" {
		t.Errorf("VersionError = %+v", ve)
	}
}

// The load-bearing assumption: an embedded Envelope yields a TOP-LEVEL key.
func TestEnvelopeFlattensIntoDocument(t *testing.T) {
	type doc struct {
		Envelope
		BPM int `json:"bpm"`
	}
	data, err := json.Marshal(doc{Envelope: Current(), BPM: 120})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := raw[Field]; got != Version {
		t.Fatalf("top-level %q = %v, want %q (json = %s)", Field, got, Version, data)
	}
	if _, nested := raw["Envelope"]; nested {
		t.Errorf("Envelope nested instead of flattened: %s", data)
	}

	// And it round-trips back through the embedded field.
	var back doc
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if back.SchemaVersion != Version || back.BPM != 120 {
		t.Errorf("round-trip = %+v", back)
	}
}

// TestVersionIsPinnedPreRelease guards the release checklist: the version
// stays 0.0.1 until we deliberately cut a release.
func TestVersionIsPinnedPreRelease(t *testing.T) {
	if Version != "0.0.1" {
		t.Fatalf("Version = %q; if this is an intentional release bump, update this test and every migration note in the 2026-07-27 schema-versioning spec", Version)
	}
}
