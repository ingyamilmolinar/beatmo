package userprefs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// fullSnapshot exercises every section at once, so a round-trip test
// actually proves the single-pass marshaller covers all of them (the old
// V3..V7 chain hid a dropped section behind an early return).
func fullSnapshot() prefsSnapshot {
	return prefsSnapshot{
		favorites:     map[instrument.ID]bool{roster.Kick: true, roster.Snare: true},
		audioPanel:    AudioPanelStateDoc{SynthGraphDetails: true},
		sampleEdits:   map[instrument.ID]map[string]float64{"user.smp": {"gain": 1.5, "reverse": 1}},
		notifications: []NotificationRecord{{Text: "hello", UnixMs: 42}},
		knobSteps:     map[string]float64{"filter_cutoff": 100},
		language:      "es",
	}
}

// TestPrefsWritesSchemaVersion pins the stamp on every written document.
func TestPrefsWritesSchemaVersion(t *testing.T) {
	b, err := marshalPrefs(fullSnapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := raw[schema.Field]; got != schema.Version {
		t.Fatalf("%s = %v, want %q\n%s", schema.Field, got, schema.Version, b)
	}
}

// TestPrefsRoundTripsEverySection is the regression guard for the chain
// collapse: every section must survive write → read unchanged.
func TestPrefsRoundTripsEverySection(t *testing.T) {
	want := fullSnapshot()
	b, err := marshalPrefs(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := parsePrefs(b, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got.favorites) != 2 || !got.favorites[roster.Kick] || !got.favorites[roster.Snare] {
		t.Errorf("favorites = %+v", got.favorites)
	}
	if got.audioPanel != want.audioPanel {
		t.Errorf("audioPanel = %+v, want %+v", got.audioPanel, want.audioPanel)
	}
	if got.sampleEdits["user.smp"]["gain"] != 1.5 || got.sampleEdits["user.smp"]["reverse"] != 1 {
		t.Errorf("sampleEdits = %+v", got.sampleEdits)
	}
	if len(got.notifications) != 1 || got.notifications[0].Text != "hello" {
		t.Errorf("notifications = %+v", got.notifications)
	}
	if got.knobSteps["filter_cutoff"] != 100 {
		t.Errorf("knobSteps = %+v", got.knobSteps)
	}
	if got.language != "es" {
		t.Errorf("language = %q", got.language)
	}
}

// TestPrefsMarshalIsSinglePass guards the property the V-chain violated:
// the document is serialized exactly once. A re-marshal of the parsed
// result must be byte-identical, which only holds if no intermediate
// re-encoding reorders or re-indents anything.
func TestPrefsMarshalIsSinglePass(t *testing.T) {
	a, err := marshalPrefs(fullSnapshot())
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	snap, err := parsePrefs(a, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	b, err := marshalPrefs(snap)
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("write→read→write not stable:\nA=%s\nB=%s", a, b)
	}
}

// TestPrefsSchemaLessDocumentStartsEmpty: a prefs document with NO
// schema_version is rejected like any other unreadable version — the store
// starts from empty and logs it (never a startup failure).
func TestPrefsSchemaLessDocumentStartsEmpty(t *testing.T) {
	schemaLess := []byte(`{
	  "favorites": ["kick"],
	  "knob_steps": {"filter_cutoff": 100},
	  "language": "en"
	}`)
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, format) }

	snap, err := parsePrefs(schemaLess, logf)
	if err == nil {
		t.Fatal("schema-less prefs accepted")
	}
	if snap.favorites[roster.Kick] || snap.knobSteps["filter_cutoff"] != 0 || snap.language != "" {
		t.Errorf("schema-less prefs leaked data into the snapshot: %+v", snap)
	}
	if len(logged) == 0 {
		t.Error("the rejection must be logged")
	}
}

// TestPrefsRejectsUnknownVersion verifies an unreadable version degrades to
// empty-but-usable state with a logged reason, rather than failing startup.
func TestPrefsRejectsUnknownVersion(t *testing.T) {
	doc := []byte(`{"schema_version":"99.0.0","favorites":["kick"],"language":"es"}`)
	var logged []string
	logf := func(format string, args ...any) {
		logged = append(logged, format)
	}

	snap, err := parsePrefs(doc, logf)
	if err == nil {
		t.Fatal("unknown version must report an error to the caller")
	}
	var ve *schema.VersionError
	if !asVersionError(err, &ve) {
		t.Fatalf("err = %T, want *schema.VersionError", err)
	}
	if len(snap.favorites) != 0 || snap.language != "" {
		t.Errorf("rejected document must not leak data: %+v", snap)
	}
	if snap.favorites == nil {
		t.Error("rejected document must still yield usable non-nil maps")
	}
	if len(logged) == 0 || !strings.Contains(strings.Join(logged, " "), "%v") {
		t.Errorf("rejection should be logged, got %v", logged)
	}
}

// TestPrefsCorruptJSONIsNotAVersionError keeps the two failure modes
// distinct: malformed bytes are tolerated silently (nil error), an
// unreadable version is reported.
func TestPrefsCorruptJSONIsNotAVersionError(t *testing.T) {
	snap, err := parsePrefs([]byte(`{not json`), nil)
	if err != nil {
		t.Fatalf("corrupt JSON should degrade silently, got %v", err)
	}
	if len(snap.favorites) != 0 {
		t.Errorf("corrupt document must not leak data: %+v", snap)
	}
}

// sameJSON compares two JSON payloads structurally, ignoring formatting.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal a (%s): %v", a, err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal b (%s): %v", b, err)
	}
	ab, _ := json.Marshal(av)
	bb, _ := json.Marshal(bv)
	return string(ab) == string(bb)
}

func asVersionError(err error, target **schema.VersionError) bool {
	ve, ok := err.(*schema.VersionError)
	if ok {
		*target = ve
	}
	return ok
}
