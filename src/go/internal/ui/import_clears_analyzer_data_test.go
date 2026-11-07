package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// No samples flow while stopped, so a channel analyzer reused across imports keeps showing the previous
// circuit's last frame. "kick" is in both circuits and B has no EQ, so nothing else rebuilds that analyzer.
func TestImportClearsStaleAnalyzerData(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)

	// Load an initial circuit that uses "kick".
	circuitA := projectFile{
		Envelope: schema.Current(), Subdiv: 32, BPM: 120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: roster.Kick, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	dataA, _ := json.Marshal(circuitA)
	if err := g.Import(dataA); err != nil {
		t.Fatalf("import A: %v", err)
	}

	// A full window of non-zero samples: the state a just-stopped circuit leaves behind.
	an := audio.EnableChannelAnalyzer(channel.Of(roster.Kick), 512)
	if an == nil {
		t.Fatal("expected a kick channel analyzer")
	}
	samples := make([]float32, 512)
	for i := range samples {
		samples[i] = 0.5
	}
	an.ProcessBlock(samples, len(samples))

	// Checked on the raw snapshot and on the state builder the tabs use on WASM.
	pre := audio.ChannelAnalyzerSnapshot(channel.Of(roster.Kick))
	if pre.RMS == 0 && pre.Peak == 0 {
		t.Fatalf("precondition failed: expected non-flat analyzer data, got RMS=%v Peak=%v", pre.RMS, pre.Peak)
	}
	if st := buildAnalyzerStateFromSnapshotsImpl(channel.Of(roster.Kick), g.drum.Rows, audio.SampleRate(), audio.ChannelAnalyzerSnapshot, nil); st == nil || st.Detail == nil || !st.Detail.Active {
		t.Fatalf("precondition failed: expected active kick detail state before import")
	}

	circuitB := projectFile{
		Envelope: schema.Current(), Subdiv: 32, BPM: 140,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: roster.Kick, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	dataB, _ := json.Marshal(circuitB)
	if err := g.Import(dataB); err != nil {
		t.Fatalf("import B: %v", err)
	}

	post := audio.ChannelAnalyzerSnapshot(channel.Of(roster.Kick))
	if post.RMS != 0 || post.Peak != 0 {
		t.Errorf("stale analyzer scalars after import: RMS=%v Peak=%v (want 0, 0)", post.RMS, post.Peak)
	}
	for i, v := range post.Waveform {
		if v != 0 {
			t.Errorf("stale waveform sample[%d]=%v after import (want all zero)", i, v)
			break
		}
	}
	for i, v := range post.Spectrum {
		if v != 0 {
			t.Errorf("stale spectrum bin[%d]=%v after import (want all zero)", i, v)
			break
		}
	}

	if st := buildAnalyzerStateFromSnapshotsImpl(channel.Of(roster.Kick), g.drum.Rows, audio.SampleRate(), audio.ChannelAnalyzerSnapshot, nil); st != nil && st.Detail != nil && st.Detail.Active {
		t.Errorf("kick detail state still active after import (expected flat until played)")
	}
}
