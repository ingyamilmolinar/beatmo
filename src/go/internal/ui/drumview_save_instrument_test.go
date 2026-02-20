package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSaveInstrumentCopiesWavToSavedDir(t *testing.T) {
	assertDefaultParityState(t)
	root := t.TempDir()
	t.Setenv("BEATMO_ASSETS", root)

	src := writeTempWAV(t, "hit.wav")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	snareDir := filepath.Join(root, "snare")
	if err := os.MkdirAll(snareDir, 0o755); err != nil {
		t.Fatalf("mkdir snare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snareDir, "hit.wav"), data, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}

	audio.ResetInstruments()
	audio.ResetCatalogForTest(nil)
	if err := audio.InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog: %v", err)
	}
	if got := audio.AssetsRoot(); got != root {
		t.Fatalf("assets root mismatch: got=%q want=%q", got, root)
	}

	dv := newTestDrumView(t, 800, 600)
	id := "snare-hit"
	dv.Rows[0].Instrument = id
	dv.Rows[0].Name = "Hit"
	dv.rowLabels[0].Text = "Hit"
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	if _, ok := dv.instMeta[id]; !ok {
		t.Fatalf("missing catalog entry for %q", id)
	}

	dv.saveInstrument(0)

	savedDir := filepath.Join(root, "Saved")
	entries, err := os.ReadDir(savedDir)
	if err != nil {
		t.Fatalf("saved dir: %v", err)
	}
	wavCount := 0
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".wav") {
			wavCount++
		}
	}
	if wavCount != 1 {
		t.Fatalf("expected 1 saved wav, got %d", wavCount)
	}
	found := false
	for _, m := range audio.Catalog() {
		if strings.HasPrefix(strings.ToLower(m.RelPath), "saved/") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("saved instrument not found in catalog")
	}
}

func TestSaveInstrumentUsesSamplePathWhenMissingMeta(t *testing.T) {
	assertDefaultParityState(t)
	root := t.TempDir()
	t.Setenv("BEATMO_ASSETS", root)

	src := writeTempWAV(t, "custom.wav")

	audio.ResetInstruments()
	audio.ResetCatalogForTest(nil)

	dv := newTestDrumView(t, 800, 600)
	dv.samplePath = map[string]string{"custom": src}
	dv.Rows[0].Instrument = "custom"
	dv.Rows[0].Name = "Custom"
	dv.rowLabels[0].Text = "Custom"
	dv.instRefreshDirty = true
	dv.refreshInstruments()

	dv.saveInstrument(0)

	savedDir := filepath.Join(root, "Saved")
	entries, err := os.ReadDir(savedDir)
	if err != nil {
		t.Fatalf("saved dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected saved wav to be created")
	}
}

func TestSaveInstrumentRejectsSynth(t *testing.T) {
	assertDefaultParityState(t)
	root := t.TempDir()
	t.Setenv("BEATMO_ASSETS", root)
	withAudioCatalog(t, []audio.SoundMeta{{ID: "snare", Name: "Snare", Category: "Snares (Synth)", Source: "synth"}})

	dv := newTestDrumView(t, 800, 600)
	dv.Rows[0].Instrument = "snare"
	dv.Rows[0].Name = "Snare"
	dv.rowLabels[0].Text = "Snare"
	dv.instRefreshDirty = true
	dv.refreshInstruments()

	dv.saveInstrument(0)
	if len(dv.notifs) == 0 || !dv.notifs[len(dv.notifs)-1].isErr {
		t.Fatalf("expected error notification when saving synth")
	}
	if !strings.Contains(strings.ToLower(dv.notifs[len(dv.notifs)-1].msg), "wav") {
		t.Fatalf("unexpected error message: %q", dv.notifs[len(dv.notifs)-1].msg)
	}
}

func TestSaveInstrumentRejectsMissingPath(t *testing.T) {
	assertDefaultParityState(t)
	root := t.TempDir()
	t.Setenv("BEATMO_ASSETS", root)
	withAudioCatalog(t, []audio.SoundMeta{{ID: "kick-a", Name: "Kick A", Category: "Kick Drums (WAV)", Source: "wav"}})

	dv := newTestDrumView(t, 800, 600)
	dv.Rows[0].Instrument = "kick-a"
	dv.Rows[0].Name = "Kick A"
	dv.rowLabels[0].Text = "Kick A"
	dv.instRefreshDirty = true
	dv.refreshInstruments()

	dv.saveInstrument(0)
	if len(dv.notifs) == 0 || !dv.notifs[len(dv.notifs)-1].isErr {
		t.Fatalf("expected error notification when path is missing")
	}
}

func TestSaveInstrumentButtonWorksDuringPlayback(t *testing.T) {
	assertDefaultParityState(t)
	root := t.TempDir()
	t.Setenv("BEATMO_ASSETS", root)

	src := writeTempWAV(t, "hit.wav")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	snareDir := filepath.Join(root, "snare")
	if err := os.MkdirAll(snareDir, 0o755); err != nil {
		t.Fatalf("mkdir snare: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snareDir, "hit.wav"), data, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}

	audio.ResetInstruments()
	audio.ResetCatalogForTest(nil)
	if err := audio.InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog: %v", err)
	}

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.SetPlaying(true)

	dv := g.drum
	id := "snare-hit"
	dv.Rows[0].Instrument = id
	dv.Rows[0].Name = "Hit"
	dv.rowLabels[0].Text = "Hit"
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	dv.recalcButtons()
	dv.calcLayout()

	btn := dv.rowSaveBtns[0]
	if btn.OnClick == nil {
		t.Fatalf("save button missing OnClick handler")
	}
	r := btn.Rect()
	if r.Empty() {
		t.Fatalf("save button rect empty during playback")
	}
	clickDrumView(t, dv, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)

	if len(dv.notifs) == 0 {
		t.Fatalf("save click produced no notification")
	}
	last := dv.notifs[len(dv.notifs)-1]
	if last.isErr {
		t.Fatalf("save click error: %q", last.msg)
	}

	savedDir := filepath.Join(root, "Saved")
	entries, err := os.ReadDir(savedDir)
	if err != nil {
		t.Fatalf("saved dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected saved wav to be created during playback")
	}
	if len(dv.notifs) > 0 && dv.notifs[len(dv.notifs)-1].isErr {
		t.Fatalf("unexpected error notification during playback: %q", dv.notifs[len(dv.notifs)-1].msg)
	}
}
