package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// TestImportResetsAudioChannelToMaster reproduces the reported bug: when an
// instrument channel is selected in an audio-panel tab (e.g. Wave) and the user
// loads a different circuit, the channel selector must snap back to "Master".
// Before the fix, Import only re-synced the EQ when the active channel already
// was "main", so a per-instrument selection survived the document swap and the
// panel kept showing a channel from the previous (now gone) circuit.
func TestImportResetsAudioChannelToMaster(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)

	// Load an initial circuit containing a "kick" instrument.
	circuitA := projectFile{
		Envelope: schema.Current(),
		Subdiv:   32,
		BPM:      120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: roster.Kick, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	dataA, _ := json.Marshal(circuitA)
	if err := g.Import(dataA); err != nil {
		t.Fatalf("import A: %v", err)
	}

	// User selects the "kick" instrument channel (as they would in the Wave tab).
	g.drum.selectAudioChannel(channel.Of(roster.Kick))
	if g.drum.activeEQChannel() != channel.Of(roster.Kick) {
		t.Fatalf("precondition: expected kick channel active, got %q", g.drum.activeEQChannel())
	}

	// Load a different circuit (a "snare" instrument, no "kick" at all).
	circuitB := projectFile{
		Envelope: schema.Current(),
		Subdiv:   32,
		BPM:      120,
		Instruments: []exportInstrument{{
			Name: "Snare", ID: roster.Snare, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	dataB, _ := json.Marshal(circuitB)
	if err := g.Import(dataB); err != nil {
		t.Fatalf("import B: %v", err)
	}

	// The channel selector must have reset back to Master.
	if got := g.drum.activeEQChannel(); got != channel.Master {
		t.Errorf("after loading a new circuit, expected audio channel reset to Master (\"main\"), got %q", got)
	}
	if dv := g.drum; dv.eqPanelZone != nil {
		if got := dv.eqPanelZone.ActiveChannel(); got != channel.Master {
			t.Errorf("expected eqPanelZone active channel reset to \"main\", got %q", got)
		}
	}
}
