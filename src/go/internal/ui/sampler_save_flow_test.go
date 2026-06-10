//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSamplerSaveOnSynthIsNonDestructive replaces the retired
// TestSamplerSaveConvertsSynthInstrument: Save over a synth-backed instrument
// no longer converts it to a sample. The edit is stored as a non-destructive
// descriptor and the recipe binding is KEPT, so the chop is audible (the
// dispatcher applies the descriptor to the fresh recipe render at trigger
// time) AND later synth edits still take effect. Full coverage lives in
// sampler_edit_descriptor_flow_test.go.
func TestSamplerSaveOnSynthIsNonDestructive(t *testing.T) {
	assertDefaultParityState(t)
	orig := audio.RecipeForInstrument("kick")
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe("kick", orig)
		audio.ClearSampleEdit("kick")
	})
	audio.BindInstrumentToRecipe("kick", "drum-kick")
	audio.ClearSampleEdit("kick")

	s := &samplerState{}
	s.reset()
	s.captureFromSynth("kick")
	s.startFrac = 0.25
	s.save()

	if got := audio.RecipeForInstrument("kick"); got != "drum-kick" {
		t.Errorf("after Save, RecipeForInstrument(kick)=%q, want drum-kick (binding kept)", got)
	}
	if !audio.HasSampleEdit("kick") {
		t.Error("after Save, kick has no sample-edit descriptor")
	}
	if _, ok := audio.UserSamplePCM("kick"); ok {
		t.Error("after Save, kick must NOT have baked user-sample PCM")
	}
}

// TestSamplerEnsureLoadedFromUserSample verifies the dropdown auto-load path
// pulls a user sample's PCM straight from the canonical Go store (no synth
// render), so WAV imports / saved chops load identically on every platform.
func TestSamplerEnsureLoadedFromUserSample(t *testing.T) {
	g := newSamplerTabGame(t)
	pcm := make([]float32, 1000)
	for i := range pcm {
		pcm[i] = 0.25
	}
	audio.PutUserSample("user.sample.loadtest", pcm, 48000)

	g.drum.ensureSamplerLoaded("user.sample.loadtest")
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after loading user sample")
	}
	if s.captureID != "user.sample.loadtest" {
		t.Errorf("captureID=%q, want user.sample.loadtest", s.captureID)
	}
	if s.source != samplerSourceWAV {
		t.Errorf("source=%v, want WAV (loaded from PCM store)", s.source)
	}
	if len(s.raw) != len(pcm) {
		t.Errorf("raw len=%d, want %d", len(s.raw), len(pcm))
	}
	if s.rawSampleRate != 48000 {
		t.Errorf("rawSampleRate=%d, want 48000", s.rawSampleRate)
	}
}

// TestSamplerEnsureLoadedRendersSynth verifies a non-sample instrument
// auto-loads via a synth one-shot render when selected.
func TestSamplerEnsureLoadedRendersSynth(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "ensureload.synth.test" // never saved as a user sample
	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after synth auto-load")
	}
	if s.captureID != id {
		t.Errorf("captureID=%q, want %q", s.captureID, id)
	}
	if s.source != samplerSourceSynth {
		t.Errorf("source=%v, want synth", s.source)
	}
}

// TestSamplerEnsureLoadedIdempotent verifies re-selecting the loaded instrument
// is a no-op (does not re-render / clobber in-flight edits).
func TestSamplerEnsureLoadedIdempotent(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.ensureSamplerLoaded("kick")
	s := &g.drum.sampler
	s.startFrac = 0.3 // simulate an edit
	g.drum.ensureSamplerLoaded("kick")
	if s.startFrac != 0.3 {
		t.Errorf("re-loading the same instrument reset edits (startFrac=%v, want 0.3)", s.startFrac)
	}
}

// TestSamplerFromSynthButtonRemoved guards the removal of the "From Synth"
// button (the dropdown auto-load replaces it).
func TestSamplerFromSynthButtonRemoved(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	if b := g.drum.samplerButtonByTag("sampler-from-synth"); b != nil {
		t.Error("From Synth button must be removed (dropdown selection auto-loads instead)")
	}
}

// TestSamplerSaveAsOpensDialog verifies the Save As button opens a name-prompt
// dialog (consistent with the Synth tab) rather than saving immediately with a
// hardcoded name.
func TestSamplerSaveAsOpensDialog(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	g.drum.samplerSaveAs()
	if g.drum.saveAsDialog == nil {
		t.Fatal("Save As did not open a name-prompt dialog")
	}
	if g.drum.saveAsDialog.Value() == "" {
		t.Error("Save As dialog should pre-fill a suggested name")
	}
}

// TestSamplerSaveAsConfirmCreatesAndRebinds verifies the full Save As flow:
// confirming a typed name bakes a new sample instrument, points the row that
// played the source instrument at it, renames the row, and closes the dialog —
// mirroring the Synth tab's Save As (which rebinds the active row to the clone).
func TestSamplerSaveAsConfirmCreatesAndRebinds(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("test game has no rows")
	}
	base := dv.Rows[0].Instrument
	dv.selectAudioChannel(base)
	layoutSamplerTab(t, g) // auto-loads base into the sampler buffer
	if !dv.sampler.hasBuffer() {
		t.Fatalf("sampler did not auto-load %q", base)
	}

	dv.openSamplerSaveAsDialog()
	if dv.saveAsDialog == nil {
		t.Fatal("Save As dialog did not open")
	}
	dv.saveAsDialog.SetValue("My Chop")
	dv.ConfirmSaveAsDialog()
	if dv.saveAsDialog != nil {
		t.Error("dialog should close after confirm")
	}

	wantID := samplerUserID("My Chop", base)
	found := false
	for _, x := range audio.Instruments() {
		if x == wantID {
			found = true
		}
	}
	if !found {
		t.Errorf("new sample %q not registered after Save As", wantID)
	}
	if dv.Rows[0].Instrument != wantID {
		t.Errorf("row 0 instrument=%q, want %q (rebind to new sample)", dv.Rows[0].Instrument, wantID)
	}
	if dv.Rows[0].Name != "My Chop" {
		t.Errorf("row 0 name=%q, want \"My Chop\"", dv.Rows[0].Name)
	}
}

// TestChannelDropdownHidesMasterOnSynthSampler verifies Master is excluded from
// the channel dropdown on the Synth and Sampler tabs (you can't sample/synth
// the master bus), but kept on the other tabs.
func TestChannelDropdownHidesMasterOnSynthSampler(t *testing.T) {
	g := newSamplerTabGame(t)
	z := g.drum.eqPanelZone
	o := &eqChannelDropdownOverlay{zone: z}

	z.SetActiveTab(TabEQ)
	o.buildCallbacks()
	if len(o.allLabels) == 0 || o.allLabels[0] != "Master" {
		t.Errorf("EQ tab: first dropdown entry=%v, want Master first", o.allLabels)
	}

	for _, tab := range []PanelTab{TabSynth, TabSampler} {
		z.SetActiveTab(tab)
		o.buildCallbacks()
		for _, l := range o.allLabels {
			if l == "Master" {
				t.Errorf("tab %v: dropdown must not list Master, got %v", tab, o.allLabels)
			}
		}
		if len(o.allOnClicks) != len(o.allLabels) {
			t.Errorf("tab %v: %d onClicks != %d labels (must stay aligned)", tab, len(o.allOnClicks), len(o.allLabels))
		}
	}
}
