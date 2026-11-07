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

// ONE struct for export and import: two parallel declarations drifted and the importer silently dropped fields
// the exporter wrote.
type projectFile struct {
	schema.Envelope

	Subdiv int `json:"subdiv,omitempty"`
	BPM    int `json:"bpm"`
	// A POINTER so a genuine 0 survives (omitempty dropped it and a muted project came back at full volume). nil means
	// absent: a foreign file imports at 1.0. Export always writes it.
	MasterVolume *float64           `json:"master_volume,omitempty"` // 0-1; nil = legacy file (imports as 1.0)
	Instruments  []exportInstrument `json:"instruments"`
	Nodes        []exportNode       `json:"nodes"`
	EQ           *exportEQ          `json:"eq,omitempty"`
	// Project-scope pins (tier 0); per-user favorites live in userprefs and never touch this file.
	PinnedInstruments []instrument.ID `json:"pinned_instruments,omitempty"`
	Kits              []audio.Kit     `json:"kits,omitempty"`
	// Invisible nodes are not exported, so members are filtered; an emptied group is dropped.
	Groups []exportGroup `json:"groups,omitempty"`
	// The user's profile theme may override it; written only when set.
	Theme *theme.Doc `json:"theme,omitempty"`
	// Every row's document, so the file is complete on a clean machine. An EMBEDDED TEMPLATE may hold a pack
	// reference {"ref":"sha256:…"} instead; export never writes one and import resolves them before any reset.
	InstrumentDocs map[instrument.ID]projectDoc `json:"instrument_docs,omitempty"`
}

// The file with each document replaced by its version digest, so the circuit names the exact versions it plays;
// rows' EQ/FX/Sampler edit stay in the body.
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

// {"ref":…} and nothing else is a reference; any other object is a document.
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

// Refs were resolved by validateProjectDocs.
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

// Every writer goes through this, so no path emits an unstamped document.
func newProjectFile() projectFile {
	return projectFile{Envelope: schema.Current()}
}

// Also the foreign-JSON sniffer: a {} or another app's file has no schema_version and would import as an empty
// project (import is replace-not-merge).
func (f projectFile) validateProjectVersion() error {
	if f.SchemaVersion == "" {
		return fmt.Errorf("not a beatmo project file: no %q", schema.Field)
	}
	return f.Validate("project")
}

type legacyInstrumentFields struct {
	ID          string             `json:"id"`
	Recipe      string             `json:"recipe"`
	SynthParams map[string]float64 `json:"synth_params"`
}

// Runs BEFORE any reset. Each key must match its document's id (an empty id takes the key); a pre-document file
// (recipe / synth_params rows) is rejected, with no migration before the first release.
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

// Runs before Import resets anything.
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
