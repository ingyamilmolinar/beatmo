package ui

import (
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/testwav"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// The catalog is staged with tiny synthetic WAVs: the runtime never depends on embedded or repo-tracked samples.
func TestImportRegistersSampleInstruments(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	entries := testwav.Stage(t, []testwav.Spec{
		{ID: "kick-test", Name: "Kick Test", Group: "Kick Drums (WAV)"},
		{ID: "snare-test", Name: "Snare Test", Group: "Snares (WAV)"},
	})
	withAudioCatalog(t, entries)

	f := projectFile{
		Envelope: schema.Current(),
		Subdiv:   8,
		BPM:      120,
		Instruments: []exportInstrument{
			{Name: "Kick", ID: "kick-test", Kind: kindSample, Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
			{Name: "Snare", ID: "snare-test", Kind: kindSample, Volume: 1, Origin: 1, Color: "#FFFFFFFF"},
		},
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: model.NodeTypeRegular},
			{ID: 1, I: 4, J: 0, Type: model.NodeTypeRegular},
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

func TestImportRespectsExplicitPath(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, nil)
	entries := testwav.Stage(t, []testwav.Spec{
		{ID: "external-clap", Name: "External Clap", Group: "Percussion (WAV)"},
	})
	if len(entries) != 1 {
		t.Fatalf("stage returned %d entries", len(entries))
	}
	path := entries[0].Path

	f := projectFile{
		Envelope: schema.Current(),
		Subdiv:   8,
		BPM:      120,
		Instruments: []exportInstrument{
			{Name: "Clap", ID: "external-clap", Kind: kindSample, Path: path, Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
		},
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: model.NodeTypeRegular},
		},
	}
	data, _ := json.Marshal(f)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 260)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if !g.drum.IsInstrumentAvailable("external-clap") {
		t.Fatalf("instrument with explicit path not available")
	}
	if !slices.Contains(audio.Instruments(), "external-clap") {
		t.Fatalf("explicit path import did not register with audio engine; insts=%v", audio.Instruments())
	}
}
