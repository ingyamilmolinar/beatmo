package ui

import (
	"encoding/json"
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestStartupDemoInstrumentsAvailable verifies that every instrument ID
// referenced in the embedded startup_demo.json is present in
// audio.Instruments() after audio.ResetInstruments(). This catches the bug
// where WASM only registered 6-18 instruments while the demo used kick-tight
// and ride.
func TestStartupDemoInstrumentsAvailable(t *testing.T) {
	assertDefaultParityState(t)
	audio.ResetInstruments()

	data := assets_pkg.StartupDemoJSON
	if len(data) == 0 {
		t.Fatalf("embedded startup demo JSON missing")
	}

	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal startup demo: %v", err)
	}

	available := make(map[string]bool)
	for _, id := range audio.Instruments() {
		available[id] = true
	}

	for i, inst := range f.Instruments {
		if !available[inst.ID] {
			t.Errorf("startup demo instrument[%d] %q not in audio.Instruments()", i, inst.ID)
		}
	}
}
