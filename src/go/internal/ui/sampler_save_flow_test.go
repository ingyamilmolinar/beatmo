//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// From the Go store, with no synth render, so imports and saved chops load the same everywhere.
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

// A document makes an instrument a synth source; an id with none is a raw WAV.
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

// No re-render that would clobber in-flight edits.
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

// The dropdown auto-load replaced it.
func TestSamplerFromSynthButtonRemoved(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	layoutSamplerTab(t, g)
	if b := g.drum.samplerButtonByTag("sampler-from-synth"); b != nil {
		t.Error("From Synth button must be removed (dropdown selection auto-loads instead)")
	}
}

// A name prompt, as on the Synth tab.
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

// The master bus cannot be sampled or synthesised; other tabs keep it.
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
