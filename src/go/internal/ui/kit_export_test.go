package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestExportSchema_Phase6FieldsAreAdditive — verifies the two new
// Phase-6 export fields (top-level Kits, per-instrument Role) are
// omitempty so projects with no kits / no roles round-trip to byte
// shapes pre-Phase-6 loaders accept.
func TestExportSchema_Phase6FieldsAreAdditive(t *testing.T) {
	// Empty kit registry → top-level Kits omitted.
	t.Cleanup(audio.ResetKitsForTest)
	audio.ResetKitsForTest()

	file := exportFile{
		Version: 1,
		BPM:     120,
		Instruments: []exportInstrument{
			{Name: "Snare", ID: "snare", Kind: "builtin", Volume: 1, Color: "#FFFFFFFF"},
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
		Members:     map[string]string{"snare": "snare-2"},
	})
	file.Kits = audio.KitsForExport()
	file.Instruments[0].Role = "snare"
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
	var decoded exportFile
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
	if gotKit.Members["snare"] != "snare-2" {
		t.Errorf("decoded kit members = %v", gotKit.Members)
	}
	if decoded.Instruments[0].Role != "snare" {
		t.Errorf("decoded Role = %q want snare", decoded.Instruments[0].Role)
	}
}
