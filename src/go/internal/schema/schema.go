// Package schema is the single source of truth for the on-disk schema
// version carried by every Beatmo JSON format.
//
// It has no dependencies beyond the standard library so that ui, audio,
// userprefs and uistate can all import it without introducing a cycle.
//
// # The version rule
//
// Every format applies the same rule when reading a document:
//
//	schema_version == Version      → accept
//	anything else, absent included → reject (Validate returns an error)
//
// There is no migration arm: documents written before versioning are not
// read.
//
// # Recovery is the caller's choice
//
// Validate reports a mismatch identically for every format. What a caller
// does about it differs by format class:
//
//   - Documents the user opens explicitly (project JSON, instrument
//     bundle) surface the error. The user picked the file; tell them.
//   - Stores loaded automatically at boot (prefs, instruments, uistate)
//     log a warning and start from empty, matching how they already treat
//     a corrupt file. A fatal error there would fail startup on a file the
//     app itself wrote.
package schema

import "fmt"

// Version is the on-disk schema version for every Beatmo JSON format.
// Pinned at 0.0.1 until first release; bump once, here, at release time.
const Version = "0.0.1"

// Field is the JSON key holding the version. Exported so tests and
// migration tooling can reference it without hardcoding the string.
const Field = "schema_version"

// Envelope carries the schema version. Embed it anonymously in a document
// struct — encoding/json flattens anonymous structs, so it contributes a
// top-level "schema_version" key rather than a nested object:
//
//	type projectFile struct {
//	        schema.Envelope
//	        BPM int `json:"bpm"`
//	}
//
// The field is deliberately NOT omitempty: every document this codebase
// writes must be self-describing, and an absent key is rejected on read.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
}

// Current returns an Envelope stamped with the current version. Use it
// when constructing a document for writing.
func Current() Envelope { return Envelope{SchemaVersion: Version} }

// Validate checks the envelope against the current version: only an exact
// match is accepted, an absent version included in the rejections.
//
// format names the document kind for the error message, e.g. "project" or
// "instrument bundle".
func (e Envelope) Validate(format string) error {
	if e.SchemaVersion == Version {
		return nil
	}
	return &VersionError{Format: format, Want: Version, Got: e.SchemaVersion}
}

// VersionError reports a schema version this build cannot read. It is a
// distinct type so callers can branch on it (e.g. to offer a migration
// hint) instead of string-matching the message.
type VersionError struct {
	Format string
	Want   string
	Got    string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("%s: unsupported %s %q (this build reads %q)", e.Format, Field, e.Got, e.Want)
}
