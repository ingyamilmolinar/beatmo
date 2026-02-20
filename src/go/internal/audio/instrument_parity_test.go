package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestInstrumentParityAfterReset verifies that ResetInstruments() populates the
// instrument list with all built-in synth instruments in the canonical order
// matching BuiltinInstrumentIDs. This is a contract test that prevents drift
// between stub/WASM/desktop instrument lists.
func TestInstrumentParityAfterReset(t *testing.T) {
	ResetInstruments()

	got := Instruments()
	if len(got) != len(BuiltinInstrumentIDs) {
		t.Fatalf("instrument count mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(BuiltinInstrumentIDs), got, BuiltinInstrumentIDs)
	}

	for i, id := range BuiltinInstrumentIDs {
		if got[i] != id {
			t.Errorf("instrument[%d] = %q, want %q", i, got[i], id)
		}
	}
}

// TestInstrumentListContainsStartupDemoIDs verifies that the instrument list
// contains the specific instruments used by the startup demo (kick-tight, ride,
// etc.) which are the ones that triggered the original bug.
func TestInstrumentListContainsStartupDemoIDs(t *testing.T) {
	ResetInstruments()

	// These are the instrument IDs referenced in startup_demo.json.
	startupIDs := []string{"kick-tight", "snare", "hihat", "clap", "tom", "ride"}

	got := Instruments()
	set := make(map[string]bool, len(got))
	for _, id := range got {
		set[id] = true
	}

	for _, id := range startupIDs {
		if !set[id] {
			t.Errorf("startup demo instrument %q missing from Instruments() after ResetInstruments()", id)
		}
	}
}

// TestBuiltinInstrumentIDsMatchStartupDemo reads startup_demo.json from disk
// and verifies every instrument ID in the demo is present in
// BuiltinInstrumentIDs. This catches stale codegen (startup demo edited but
// go generate not re-run).
func TestBuiltinInstrumentIDsMatchStartupDemo(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	demoPath := filepath.Join(filepath.Dir(thisFile), "..", "assets", "startup_demo.json")

	data, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatalf("reading startup_demo.json: %v", err)
	}

	var demo struct {
		Instruments []struct {
			ID string `json:"id"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(data, &demo); err != nil {
		t.Fatalf("parsing startup_demo.json: %v", err)
	}

	set := make(map[string]bool, len(BuiltinInstrumentIDs))
	for _, id := range BuiltinInstrumentIDs {
		set[id] = true
	}

	for _, inst := range demo.Instruments {
		if !set[inst.ID] {
			t.Errorf("startup_demo.json instrument %q is not in BuiltinInstrumentIDs — run 'go generate ./internal/audio/...'", inst.ID)
		}
	}
}
