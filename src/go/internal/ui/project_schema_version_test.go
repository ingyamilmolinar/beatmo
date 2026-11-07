package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// The version header spliced in verbatim, so each case goes through the real Import.
func minimalProject(versionField string) []byte {
	return []byte(`{
		` + versionField + `
		"subdiv": 32,
		"bpm": 120,
		"instruments": [{"name":"Snare","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF"}],
		"nodes": [{"id":0,"i":0,"j":0,"type":"regular"}]
	}`)
}

// Only the current schema_version is accepted; anything else, a pre-versioning file included, is rejected.
func TestImportProjectVersionRule(t *testing.T) {
	cases := []struct {
		name         string
		versionField string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "current schema_version accepted",
			versionField: `"schema_version": "` + schema.Version + `",`,
		},
		{
			name:         "legacy int version without schema_version rejected",
			versionField: `"version": 1,`,
			wantErr:      true,
			errContains:  "schema_version",
		},
		{
			name:         "unknown schema_version rejected",
			versionField: `"schema_version": "99.0.0",`,
			wantErr:      true,
			errContains:  "99.0.0",
		},
		{
			name:         "schema_version wins over legacy marker",
			versionField: `"schema_version": "99.0.0", "version": 1,`,
			wantErr:      true,
			errContains:  "99.0.0",
		},
		{
			name:         "no version at all rejected",
			versionField: ``,
			wantErr:      true,
			errContains:  "not a beatmo project file",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(640, 480)

			err := g.Import(minimalProject(tc.versionField))
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not mention %q", err, tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Import: %v", err)
			}
		})
	}
}

// Import is replace-not-merge, so a foreign document must be rejected rather than empty the project.
func TestImportRejectsForeignJSONWipe(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	if err := g.Import(minimalProject(`"schema_version": "` + schema.Version + `",`)); err != nil {
		t.Fatalf("seed import: %v", err)
	}
	before := len(g.graph.Nodes)
	if before == 0 {
		t.Fatal("seed project produced no nodes")
	}

	for _, foreign := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"name":"some other app","data":[1,2,3]}`),
		[]byte(`{"schema_version":"0.0.1-not-ours"}`),
	} {
		if err := g.Import(foreign); err == nil {
			t.Errorf("foreign document accepted: %s", foreign)
		}
		if got := len(g.graph.Nodes); got != before {
			t.Fatalf("rejected import mutated the project: nodes %d -> %d (doc %s)", before, got, foreign)
		}
	}
}

func TestExportImportRoundTripCarriesVersion(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(minimalProject(`"schema_version": "` + schema.Version + `",`)); err != nil {
		t.Fatalf("seed import: %v", err)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := doc["version"]; present {
		t.Errorf("export still writes the retired \"version\" key")
	}
	var sv string
	if err := json.Unmarshal(doc[schema.Field], &sv); err != nil {
		t.Fatalf("export has no readable %s: %v", schema.Field, err)
	}
	if sv != schema.Version {
		t.Errorf("%s = %q, want %q", schema.Field, sv, schema.Version)
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("re-import of own export failed: %v", err)
	}
}

func TestImportRejectsSchemaLessProjectBeforeMutating(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	before, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export before: %v", err)
	}
	if err := g.Import([]byte(`{"version":1,"bpm":90,"instruments":[],"nodes":[]}`)); err == nil {
		t.Fatal("schema-less project accepted")
	}
	after, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export after: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("rejected import mutated the circuit")
	}
}

// The sequencer path snapshot stays intact (a nil one silences playback until the next edit) and the undo history
// is kept: a failed load is not a document swap.
func TestRejectedImportHasNoSideEffects(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.tryAddNode(3, 3, model.NodeTypeRegular)
	recordUndo(hooks.EventNodeAdded)
	if !g.undoManager.CanUndo() {
		t.Fatal("precondition: no undo history to protect")
	}
	if g.seqPathSnap.Load() == nil {
		t.Fatal("precondition: no sequencer path snapshot")
	}
	for name, doc := range map[string]string{
		"schema-less":    `{"version":1,"bpm":90,"instruments":[],"nodes":[]}`,
		"recipe-era row": `{"schema_version":"` + schema.Version + `","bpm":90,"instruments":[{"name":"S","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF","recipe":"drum-snare"}],"nodes":[]}`,
		"bad document":   `{"schema_version":"` + schema.Version + `","bpm":90,"instruments":[{"name":"S","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF"}],"instrument_docs":{"snare":{"id":"snare","stages":{}}},"nodes":[]}`,
	} {
		if err := g.Import([]byte(doc)); err == nil {
			t.Fatalf("%s: import accepted", name)
		}
		if g.seqPathSnap.Load() == nil {
			t.Errorf("%s: rejected import cleared the sequencer path snapshot (playback would go silent)", name)
		}
		if !g.undoManager.CanUndo() {
			t.Errorf("%s: rejected import wiped the undo history", name)
		}
	}
}
