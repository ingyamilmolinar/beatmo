package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestImportClearsStaleAnalyzerData reproduces the reported bug: after loading a
// new circuit while playback is STOPPED, the audio-panel tabs (Wave, Spectrum,
// Levels) still show "garbage" — the last analyzer frame from the PREVIOUS
// circuit. No new samples flow when stopped, so the per-channel Analyzer keeps
// returning its retained waveform/spectrum/RMS/peak, and the channel-analyzer
// wiring reuses the same *Analyzer instance across imports.
//
// The reproduction uses an instrument id ("kick") present in BOTH circuits: the
// new circuit carries no per-instrument EQ, so nothing rebuilds that channel's
// analyzer during import, and its stale buffer survives. Loading a circuit must
// clear the retained frame so the channel reads flat until played.
func TestImportClearsStaleAnalyzerData(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)

	// Load an initial circuit that uses "kick".
	circuitA := exportFile{
		Version: 1, Subdiv: 32, BPM: 120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	dataA, _ := json.Marshal(circuitA)
	if err := g.Import(dataA); err != nil {
		t.Fatalf("import A: %v", err)
	}

	// Simulate audio having flowed through the kick channel analyzer: feed a full
	// window of non-zero samples so the analyzer computes RMS/peak and retains
	// the waveform (exactly the state a just-stopped circuit leaves behind).
	an := audio.EnableChannelAnalyzer("kick", 512)
	if an == nil {
		t.Fatal("expected a kick channel analyzer")
	}
	samples := make([]float32, 512)
	for i := range samples {
		samples[i] = 0.5
	}
	an.ProcessBlock(samples, len(samples))

	// Precondition: the analyzer now reports non-flat data. Assert both on the
	// raw snapshot and on the pure state-builder the Wave/Spectrum tabs use on
	// WASM (buildAnalyzerStateFromSnapshotsImpl reads the same snapshots).
	pre := audio.ChannelAnalyzerSnapshot("kick")
	if pre.RMS == 0 && pre.Peak == 0 {
		t.Fatalf("precondition failed: expected non-flat analyzer data, got RMS=%v Peak=%v", pre.RMS, pre.Peak)
	}
	if st := buildAnalyzerStateFromSnapshotsImpl("kick", g.drum.Rows, audio.SampleRate(), audio.ChannelAnalyzerSnapshot); st == nil || st.Detail == nil || !st.Detail.Active {
		t.Fatalf("precondition failed: expected active kick detail state before import")
	}

	// Load a different circuit that ALSO uses "kick" (no per-instrument EQ, so
	// nothing rebuilds the kick channel). Playback is stopped throughout.
	circuitB := exportFile{
		Version: 1, Subdiv: 32, BPM: 140,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	dataB, _ := json.Marshal(circuitB)
	if err := g.Import(dataB); err != nil {
		t.Fatalf("import B: %v", err)
	}

	// The retained analyzer frame must be cleared: the channel should read flat
	// until the new circuit is actually played.
	post := audio.ChannelAnalyzerSnapshot("kick")
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

	// The State the Wave/Spectrum tabs render on WASM (built from the same
	// per-channel snapshots) must report an inactive channel.
	if st := buildAnalyzerStateFromSnapshotsImpl("kick", g.drum.Rows, audio.SampleRate(), audio.ChannelAnalyzerSnapshot); st != nil && st.Detail != nil && st.Detail.Active {
		t.Errorf("kick detail state still active after import (expected flat until played)")
	}
}
