package ui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/objects"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// projectFile is the on-the-wire shape of a Beatmo project (beatmo.json).
//
// Export and import share ONE struct deliberately. They were previously two
// field-identical declarations (exportFile / importFile) that had to be
// edited in lockstep; when they drifted, a field written by the exporter was
// silently dropped by the importer — that is exactly how authored kits and
// node groups were lost before they were mirrored by hand.
type projectFile struct {
	schema.Envelope

	Subdiv int `json:"subdiv,omitempty"`
	BPM    int `json:"bpm"`
	// MasterVolume is a POINTER so a genuine 0 (silence) survives the
	// round-trip: with a plain float64+omitempty, master=0 was dropped on
	// export and restored as the legacy 1.0 default on import — a muted
	// project came back at FULL volume. nil = field absent (legacy files,
	// which imported at 1.0 long before the fresh-session default moved to
	// 0.5 — that fallback must stay for their sake). Export now always
	// writes the current value so new files are self-describing.
	MasterVolume *float64           `json:"master_volume,omitempty"` // 0-1; nil = legacy file (imports as 1.0)
	Instruments  []exportInstrument `json:"instruments"`
	Nodes        []exportNode       `json:"nodes"`
	EQ           *exportEQ          `json:"eq,omitempty"`
	// PinnedInstruments holds project-scope instrument pins (tier 0 in the
	// menu's PinSource). Distinct from per-user favorites which live in
	// userprefs and never touch this file.
	PinnedInstruments []instrument.ID `json:"pinned_instruments,omitempty"`
	// Kits carries every kit registered in the audio package at export
	// time.
	Kits []audio.Kit `json:"kits,omitempty"`
	// Groups carries node groups (batch pitch/volume/duration rules across a
	// set of nodes). Invisible nodes are never exported, so group members
	// are filtered to exported node ids; a group left empty after filtering
	// is dropped.
	Groups []exportGroup `json:"groups,omitempty"`
	// Theme is the circuit's own UI color scheme (see internal/theme and
	// theme_categories.go). Optional; the user's profile theme may override
	// it. Written only when the circuit sets something.
	Theme *theme.Doc `json:"theme,omitempty"`
	// InstrumentDocs carries the instrument document of every row's instrument
	// (spec 2026-10-04 §6), keyed by instrument id: everything its synth voice
	// depends on. The file is complete on its own — a clean machine, whose
	// library has never seen a Save or a Save As, plays it exactly. Import
	// replaces the session's copies with these (replace, not merge).
	//
	// A value is a full document or, in an EMBEDDED TEMPLATE only, a pack
	// reference {"ref":"sha256:…"} (a ContentHash of a document in the
	// instrument pack — shipped or pinned). Export never writes a ref; import
	// resolves every ref in validateProjectDocs, before anything resets.
	InstrumentDocs map[instrument.ID]projectDoc `json:"instrument_docs,omitempty"`
}

// circuitDigest is the circuit's version id (spec 2026-10-31 §2.2): the file
// with each instrument document replaced by that instrument's version digest,
// so the circuit points to the exact versions it plays. Rows carry their own
// EQ / FX / Sampler edit, which stay part of the circuit's body.
func circuitDigest(f projectFile) string {
	versions := make(map[instrument.ID]string, len(f.InstrumentDocs))
	for id, pd := range f.InstrumentDocs {
		versions[id] = audio.InstrumentVersionDigest(pd.Doc, audio.AudioBundle{})
	}
	f.InstrumentDocs = nil
	return objects.Digest(struct {
		Circuit     projectFile              `json:"circuit"`
		Instruments map[instrument.ID]string `json:"instruments"`
	}{f, versions})
}

// projectDoc is one instrument_docs value: a full document, or (Ref != "") a
// reference to the pack document with that content hash.
type projectDoc struct {
	instrument.Doc
	Ref string
}

// MarshalJSON writes {"ref":…} for a reference and the bare document otherwise.
func (p projectDoc) MarshalJSON() ([]byte, error) {
	if p.Ref != "" {
		return json.Marshal(struct {
			Ref string `json:"ref"`
		}{p.Ref})
	}
	return json.Marshal(p.Doc)
}

// UnmarshalJSON reads {"ref":…} (and nothing else) as a reference, any other
// object as a document.
func (p *projectDoc) UnmarshalJSON(b []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	if raw, ok := keys["ref"]; ok {
		if len(keys) != 1 {
			return fmt.Errorf("an instrument_docs reference carries only \"ref\"")
		}
		*p = projectDoc{}
		return json.Unmarshal(raw, &p.Ref)
	}
	*p = projectDoc{}
	return json.Unmarshal(b, &p.Doc)
}

// fullDocs returns the file's documents as plain documents (refs resolved by
// validateProjectDocs beforehand).
func (f projectFile) fullDocs() map[instrument.ID]instrument.Doc {
	if f.InstrumentDocs == nil {
		return nil
	}
	out := make(map[instrument.ID]instrument.Doc, len(f.InstrumentDocs))
	for id, pd := range f.InstrumentDocs {
		out[id] = pd.Doc
	}
	return out
}

// newProjectFile returns a project document stamped with the current schema
// version. Every writer must go through this rather than building a
// projectFile literal, so no path can emit an unstamped document.
func newProjectFile() projectFile {
	return projectFile{Envelope: schema.Current()}
}

// validateProjectVersion applies the strict version rule: the current
// schema_version or a rejection. It doubles as the foreign-JSON sniffer — an
// empty {} or another app's file carries no schema_version, so it is rejected
// instead of importing as an all-zero project (import is replace-not-merge).
func (f projectFile) validateProjectVersion() error {
	if f.SchemaVersion == "" {
		return fmt.Errorf("not a beatmo project file: no %q", schema.Field)
	}
	return f.Validate("project")
}

// legacyInstrumentFields are the recipe-era synth fields of an instruments[]
// row; a file carrying them predates instrument documents.
type legacyInstrumentFields struct {
	ID          string             `json:"id"`
	Recipe      string             `json:"recipe"`
	SynthParams map[string]float64 `json:"synth_params"`
}

// validateProjectDocs is the read boundary for the file's instrument
// documents, run BEFORE anything is reset: each key must match its document's
// id (an empty id takes the key), and each document is migrated and validated
// in place. A file from before instrument documents (rows with recipe /
// synth_params) is rejected — there is no migration before the first release.
func (f *projectFile) validateProjectDocs(raw []byte) error {
	var legacy struct {
		Instruments []legacyInstrumentFields `json:"instruments"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil {
		for _, inst := range legacy.Instruments {
			if inst.Recipe != "" || inst.SynthParams != nil {
				return fmt.Errorf("instrument row %q predates instrument documents (recipe/synth_params); re-save it from an older build is not supported", inst.ID)
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(f.InstrumentDocs)) {
		d := f.InstrumentDocs[key].Doc
		if ref := f.InstrumentDocs[key].Ref; ref != "" {
			pd, ok := audio.PackDoc(ref)
			if !ok {
				return fmt.Errorf("instrument_docs.%s: reference %s is not in the instrument pack", key, ref)
			}
			pd.ID, pd.Origin = key, key.Base()
			d = pd
		}
		if d.ID == "" {
			d.ID = key
		}
		if d.ID != key {
			return fmt.Errorf("instrument_docs.%s: document id %q does not match its key", key, d.ID)
		}
		m, err := audio.MigrateAndValidateInstrumentDoc(d)
		if err != nil {
			return fmt.Errorf("instrument_docs.%s: %w", key, err)
		}
		f.InstrumentDocs[key] = projectDoc{Doc: m}
	}
	return nil
}

// validateNodeChords rejects a chord block that names an unknown quality or
// an out-of-range inversion / spread (an empty quality with any other field
// set included). Runs before Import resets anything.
func (f *projectFile) validateNodeChords() error {
	for _, n := range f.Nodes {
		if n.Chord == nil {
			continue
		}
		c := n.Chord.spec()
		if c.Quality == "" || !model.ChordSpecValid(c) {
			return fmt.Errorf("node %d: invalid chord {quality:%q inversion:%d spread:%d}", n.ID, c.Quality, c.Inversion, c.Spread)
		}
	}
	return nil
}
