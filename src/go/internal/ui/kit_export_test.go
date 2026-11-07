package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Kits and Role are omitempty, so a project with neither exports without them.
func TestExportSchema_Phase6FieldsAreAdditive(t *testing.T) {
	// Empty kit registry → top-level Kits omitted.
	t.Cleanup(audio.ResetKitsForTest)
	audio.ResetKitsForTest()

	file := projectFile{
		Envelope: schema.Current(),
		BPM:      120,
		Instruments: []exportInstrument{
			{Name: "Snare", ID: roster.Snare, Kind: kindBuiltin, Volume: 1, Color: "#FFFFFFFF"},
		},
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"kits"`) {
		t.Errorf("empty kit registry emitted 'kits' field: %s", raw)
	}
	if strings.Contains(string(raw), `"role"`) {
		t.Errorf("instrument with empty Role emitted 'role' field: %s", raw)
	}

	// Populate a kit + a role → both appear.
	audio.RegisterKit(audio.Kit{
		ID:          "kit.fixture",
		DisplayName: "Fixture",
		Members:     map[instrument.Role]instrument.ID{instrument.RoleSnare: roster.SnareLofi},
	})
	file.Kits = audio.KitsForExport()
	file.Instruments[0].Role = instrument.RoleSnare
	raw2, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal2: %v", err)
	}
	if !strings.Contains(string(raw2), `"kits"`) {
		t.Errorf("populated kits not emitted: %s", raw2)
	}
	if !strings.Contains(string(raw2), `"role":"snare"`) {
		t.Errorf("Role not emitted: %s", raw2)
	}

	// Round-trip decode must preserve the kit's id/name/members.
	var decoded projectFile
	if err := json.Unmarshal(raw2, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Kits) != 1 {
		t.Fatalf("decoded kit count = %d want 1", len(decoded.Kits))
	}
	gotKit := decoded.Kits[0]
	if gotKit.ID != "kit.fixture" || gotKit.DisplayName != "Fixture" {
		t.Errorf("decoded kit metadata = %+v", gotKit)
	}
	if gotKit.Members[instrument.RoleSnare] != roster.SnareLofi {
		t.Errorf("decoded kit members = %v", gotKit.Members)
	}
	if decoded.Instruments[0].Role != instrument.RoleSnare {
		t.Errorf("decoded Role = %q want snare", decoded.Instruments[0].Role)
	}
}
