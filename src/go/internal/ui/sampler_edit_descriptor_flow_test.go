//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// sampler_edit_descriptor_flow_test.go — the Sampler-tab Save on a SYNTH
// source is non-destructive: it stores the edit as a per-instrument
// descriptor (audio.SetSampleEdit) and KEEPS the synth document, so the synth
// stays the source of truth and later synth changes always take effect.
// Only WAV sources bake PCM.

// sampleEditApproxEqual compares two SampleEdits with a tolerance on the float
// fields (trim fractions, pitch, gain, fades) and exact on the bool flags. Used
// where the reverse trim-mirror introduces sub-ulp float drift on round-trip.
func sampleEditApproxEqual(a, b audio.SampleEdit, eps float64) bool {
	close := func(x, y float64) bool {
		d := x - y
		if d < 0 {
			d = -d
		}
		return d <= eps
	}
	return close(a.StartFrac, b.StartFrac) && close(a.EndFrac, b.EndFrac) &&
		close(a.TransposeSemis, b.TransposeSemis) && close(a.DetuneCents, b.DetuneCents) &&
		close(float64(a.GainDB), float64(b.GainDB)) &&
		a.Fade == b.Fade && a.Reverse == b.Reverse && a.Normalize == b.Normalize
}

// cleanupSamplerEditState snapshots and restores the global audio state the
// sampler Save flow mutates (TEST GOTCHA: save() writes GLOBAL maps).
func cleanupSamplerEditState(t *testing.T, instID instrument.ID) {
	t.Helper()
	keepInstrumentDoc(t, instID)
	origEdit, hadEdit := audio.SampleEditFor(instID)
	t.Cleanup(func() {
		if hadEdit {
			audio.SetSampleEdit(instID, origEdit)
		} else {
			audio.ClearSampleEdit(instID)
		}
	})
}

func TestSamplerEnsureLoadedRestoresDescriptorIntoEditor(t *testing.T) {
	g := newSamplerTabGame(t)
	const id = "descload.synth.test"
	cleanupSamplerEditState(t, id)
	_ = audio.InstallInstrumentDocForTest(id, roster.Kick)
	want := audio.SampleEdit{StartFrac: 0.2, EndFrac: 0.8, TransposeSemis: 3, DetuneCents: -10, GainDB: -4.5, Normalize: true, Reverse: true, Fade: true}
	audio.SetSampleEdit(id, want)

	g.drum.ensureSamplerLoaded(id)
	s := &g.drum.sampler
	if !s.hasBuffer() {
		t.Fatal("no buffer after descriptor-bearing synth load")
	}
	if s.source != samplerSourceSynth {
		t.Fatalf("source=%v, want synth (descriptor keeps the synth source of truth)", s.source)
	}
	// Tolerance compare: a reversed descriptor's trim is mirrored to the display
	// frame on load and back to the source frame on save (editDescriptor), and the
	// 1-x mirror is not exactly self-inverse in float64 (1-(1-0.2) ≈ 0.2 to ~1e-16).
	// The round-trip is faithful well below a single sample, so compare with eps.
	if got := s.editDescriptor(); !sampleEditApproxEqual(got, want, 1e-9) {
		t.Errorf("editor state after load = %+v, want the saved descriptor %+v", got, want)
	}
	if !s.fadeOn || !s.normalize {
		t.Errorf("fadeOn=%v normalize=%v, want both true", s.fadeOn, s.normalize)
	}
}

// stubSampleEditSink records SaveSampleEdit / DeleteSampleEdit calls.
type stubSampleEditSink struct {
	saved   map[instrument.ID]map[string]float64
	deleted []instrument.ID
}

func (s *stubSampleEditSink) SaveSampleEdit(instID instrument.ID, fields map[string]float64) error {
	if s.saved == nil {
		s.saved = map[instrument.ID]map[string]float64{}
	}
	s.saved[instID] = fields
	return nil
}

func (s *stubSampleEditSink) DeleteSampleEdit(instID instrument.ID) error {
	s.deleted = append(s.deleted, instID)
	return nil
}

func TestSamplerSourceSignatureFoldsDescriptor(t *testing.T) {
	assertDefaultParityState(t)
	const id = "descsig.synth.test"
	cleanupSamplerEditState(t, id)
	audio.ClearSampleEdit(id)

	before := samplerSourceSignatureFn(id)
	audio.SetSampleEdit(id, audio.SampleEdit{StartFrac: 0.1, EndFrac: 1})
	after := samplerSourceSignatureFn(id)
	if before == after {
		t.Error("source signature must change when the sample-edit descriptor changes")
	}
}
