package ui

import (
	"encoding/json"
	"io"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Regression: importing a project with sample-backed instruments (IDs prefixed
// with "sample-") must auto-register those WAVs from assets/wav so playback is
// not muted due to missing instruments.
func TestImportRegistersSampleInstruments(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, nil)

	// Minimal import payload mirroring beatmo.json sample ids.
	f := importFile{
		Version: 1,
		Subdiv:  8,
		BPM:     120,
		Instruments: []exportInstrument{
			{Name: "Kick", ID: "sample-kick-9-wonder-lop-k1", Kind: "sample", Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
			{Name: "Snare", ID: "sample-snare", Kind: "sample", Volume: 1, Origin: 1, Color: "#FFFFFFFF"},
		},
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular"},
			{ID: 1, I: 4, J: 0, Type: "regular"},
		},
	}
	data, _ := json.Marshal(f)

	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 260)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if len(g.drum.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(g.drum.Rows))
	}
	for i, row := range g.drum.Rows {
		if !g.drum.IsInstrumentAvailable(row.Instrument) {
			t.Fatalf("row %d instrument %q not available after import", i, row.Instrument)
		}
	}
}

// Ensure resolveSamplePath finds asset WAVs when running tests from src/go.
func TestResolveSamplePathFindsAssets(t *testing.T) {
	path := resolveSamplePath("sample-kick-9-wonder-lop-k1")
	if path == "" {
		t.Fatalf("resolveSamplePath returned empty path")
	}
}
