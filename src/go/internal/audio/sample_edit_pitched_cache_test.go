//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A melodic voice looked up the UNFOLDED key first, so once rendered unedited it kept serving that entry and the
// Sampler edit was inaudible, while an export/import identity test passed because both sides ignored it.
func TestPitchedVoiceHonoursASampleEditAfterItWasCachedUnedited(t *testing.T) {
	const id = roster.OrganChurch
	if !IsMelodicInstrument(id) {
		t.Fatalf("precondition: %s must render through the pitched path", id)
	}
	t.Cleanup(func() { ClearSampleEdit(id) })
	ClearSampleEdit(id)

	render := func() []float32 {
		t.Helper()
		v, ok := newRecipeAwareVoicePitchedOrigin(id, SampleRate(), 2, originPrewarm).(*cVoice)
		if !ok || v == nil || len(v.buf) == 0 {
			t.Fatalf("no pitched voice for %s", id)
		}
		return append([]float32(nil), v.buf...)
	}
	same := func(a, b []float32) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	plain := render() // caches the UNEDITED voice
	SetSampleEdit(id, SampleEditFromFields(map[string]float64{
		"start_frac": 0.1, "end_frac": 0.7, "gain_db": -6, "reverse": 1,
	}))
	edited := render()
	if same(plain, edited) {
		t.Fatalf("the sample edit is inaudible on the pitched path: the cached unedited voice was served")
	}
	ClearSampleEdit(id)
	if !same(plain, render()) {
		t.Errorf("clearing the edit did not restore the unedited voice")
	}
}
