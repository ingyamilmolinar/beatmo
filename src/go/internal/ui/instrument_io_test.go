//go:build test

package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInstrumentMenuHasExportImport(t *testing.T) {
	g := newInstrumentTestGame(t)
	items := g.drum.instrumentActionMenuItems(func() {})
	labels := map[string]bool{}
	for _, it := range items {
		labels[it.Label] = true
	}
	if !labels[i18n.T(i18n.KeyMenuExport)] || !labels[i18n.T(i18n.KeyMenuImport)] {
		t.Errorf("⋯ menu missing Export/Import items: %v", labels)
	}
	if !labels[i18n.T(i18n.KeySaveAs)] || labels[i18n.T(i18n.KeyReset)] {
		t.Errorf("⋯ menu must offer Save As and no Reset: %v", labels)
	}
}

func TestInstrumentFileNameSanitizes(t *testing.T) {
	restore := SetDownloadClockForTest(func() time.Time {
		return time.Date(2026, 9, 13, 14, 15, 30, 0, time.UTC)
	})
	defer restore()
	if got := instrumentFileName("Warm 808 !!"); got != "warm-808-2026-09-13_141530.beatmo-inst.json" {
		t.Errorf("filename = %q", got)
	}
	if got := instrumentFileName(""); got != "instrument-2026-09-13_141530.beatmo-inst.json" {
		t.Errorf("empty name → %q", got)
	}
}

// Otherwise the browser renames the second file, spelling the uniquifier differently per engine.
func TestInstrumentExportNameNeverCollides(t *testing.T) {
	now := time.Date(2026, 9, 13, 14, 15, 30, 0, time.UTC)
	restore := SetDownloadClockForTest(func() time.Time { return now })
	defer restore()
	first := instrumentFileName("Snare")
	now = now.Add(time.Second)
	second := instrumentFileName("Snare")
	if first == second {
		t.Errorf("two exports of the same instrument both asked for %q", first)
	}
	for _, n := range []string{first, second} {
		assertCollisionFreeName(t, n)
	}
}

func TestExportActiveInstrumentDownloadsBundle(t *testing.T) {
	g := newInstrumentTestGame(t)
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
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
	g := newDocsGame(t, roster.Snare)
	dv := g.drum
	dv.selRow = 0

	// Build a valid bundle to feed the picker.
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.4)
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
	if _, ok := audio.UserLibraryDoc(after); !ok || !strings.HasPrefix(string(after), "user.") {
		t.Errorf("imported instrument %q is not a user library instrument", after)
	}
	if !audio.HasInstrumentDoc(after) {
		t.Errorf("imported instrument %q has no project document", after)
	}
}
