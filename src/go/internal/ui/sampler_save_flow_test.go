//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

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

// TestSamplerEnsureLoadedRendersSynth verifies a synth-document instrument
// auto-loads via a synth one-shot render when selected. A synth document is
// what makes an instrument a synth source (the descriptor path that Save uses
// is only applied to synth voices); an id with no document is a raw WAV.
func TestSamplerEnsureLoadedRendersSynth(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "ensureload.synth.test" // synth document, never saved as a user sample
	_ = audio.InstallInstrumentDocForTest(id, roster.Kick)
	t.Cleanup(func() { audio.RemoveInstrumentDocForTest(id) })
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
	g.drum.ensureSamplerLoaded(roster.Kick)
	s := &g.drum.sampler
	s.startFrac = 0.3 // simulate an edit
	g.drum.ensureSamplerLoaded(roster.Kick)
	if s.startFrac != 0.3 {
		t.Errorf("re-loading the same instrument reset edits (startFrac=%v, want 0.3)", s.startFrac)
	}
}

// TestSamplerFromSynthButtonRemoved guards the removal of the "From Synth"
// button (the dropdown auto-load replaces it).
func TestSamplerFromSynthButtonRemoved(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
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
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.samplerSaveAs()
	if g.drum.saveAsDialog == nil {
		t.Fatal("Save As did not open a name-prompt dialog")
	}
	if g.drum.saveAsDialog.Value() == "" {
		t.Error("Save As dialog should pre-fill a suggested name")
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
