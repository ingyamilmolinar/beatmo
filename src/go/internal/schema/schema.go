// Package schema holds the one on-disk version of every Beatmo JSON format (stdlib only, so anything may import it).
// Exactly the current version is accepted, an absent one rejected (no migration). Opened documents surface the error;
// stores loaded at boot warn and start empty, since a fatal error would fail startup on a file the app wrote.
package schema

import "fmt"

// Version is the on-disk schema version for every Beatmo JSON format.
// Pinned at 0.0.1 until first release; bump once, here, at release time.
const Version = "0.0.1"

// Field is the JSON key holding the version. Exported so tests and
// migration tooling can reference it without hardcoding the string.
const Field = "schema_version"

// Embedded anonymously, it flattens to a top-level "schema_version". Deliberately NOT omitempty: every document written
// is self-describing, and an absent key is rejected on read.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
}

// Current returns an Envelope stamped with the current version. Use it
// when constructing a document for writing.
func Current() Envelope { return Envelope{SchemaVersion: Version} }

// Exactly the current version; format names the document kind for the error.
func (e Envelope) Validate(format string) error {
	if e.SchemaVersion == Version {
		return nil
	}
	return &VersionError{Format: format, Want: Version, Got: e.SchemaVersion}
}

// A distinct type, so callers branch on it instead of matching the message.
type VersionError struct {
	Format string
	Want   string
	Got    string
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("%s: unsupported %s %q (this build reads %q)", e.Format, Field, e.Got, e.Want)
}
