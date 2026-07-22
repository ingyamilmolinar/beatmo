//go:build test

package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestInstrumentMenuHasExportImport(t *testing.T) {
	g := newInstrumentTestGame(t)
	items := g.drum.instrumentActionMenuItems(func() {}, func() {})
	labels := map[string]bool{}
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels[i18n.T(i18n.KeyMenuExport)] || !labels[i18n.T(i18n.KeyMenuImport)] {
		t.Errorf("⋯ menu missing Export/Import items: %v", labels)
	}
	if !labels[i18n.T(i18n.KeySaveAs)] || !labels[i18n.T(i18n.KeyReset)] {
		t.Errorf("⋯ menu lost Save As / Reset: %v", labels)
	}
}

func TestInstrumentFileNameSanitizes(t *testing.T) {
	if got := instrumentFileName("Warm 808 !!"); got != "warm-808.beatmo-inst.json" {
		t.Errorf("filename = %q", got)
	}
	if got := instrumentFileName(""); got != "instrument.beatmo-inst.json" {
		t.Errorf("empty name → %q", got)
	}
}

func TestExportActiveInstrumentDownloadsBundle(t *testing.T) {
	g := newInstrumentTestGame(t)
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	g.drum.selRow = 0

	var gotName string
	var gotData []byte
	restore := SetSaveJSONForTest(func(name string, data []byte) error {
		gotName, gotData = name, data
		return nil
	})
	defer restore()

	g.drum.exportActiveInstrument()

	if !strings.HasSuffix(gotName, ".beatmo-inst.json") {
		t.Errorf("download name = %q, want *.beatmo-inst.json", gotName)
	}
	var b audio.InstrumentBundle
	if err := json.Unmarshal(gotData, &b); err != nil {
		t.Fatalf("exported file not valid JSON: %v", err)
	}
	if err := audio.ValidateBundleHeader(&b); err != nil {
		t.Errorf("exported bundle invalid: %v", err)
	}
	if len(b.Instruments) != 1 {
		t.Errorf("want 1 instrument, got %d", len(b.Instruments))
	}
}

func TestImportInstrumentFromFileApplies(t *testing.T) {
	g := newInstrumentTestGame(t)
	dv := g.drum
	dv.selRow = 0

	// Build a valid bundle to feed the picker.
	dv.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	bundle, err := dv.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, _ := json.Marshal(bundle)

	// Stub the file picker to feed our bytes.
	prev := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { cb(raw, "shared.beatmo-inst.json", nil) }
	t.Cleanup(func() { selectJSONAsyncFn = prev })

	before := dv.Rows[0].Instrument
	dv.importInstrumentFromFile() // picker stub sends to channel
	dv.drainInstrumentImport()    // applies on the UI goroutine

	after := dv.Rows[0].Instrument
	if after == before {
		t.Errorf("row instrument unchanged after import (%q)", after)
	}
	if !audio.RecipeExists(after) || audio.RecipeIsBuiltin(after) {
		t.Errorf("imported instrument %q not a registered user recipe", after)
	}
	t.Cleanup(func() { audio.UnregisterRecipeForTest(after) })
}
