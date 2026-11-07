//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A node's audio is fixed at trigger time: an edit renders into a NEW buffer under a NEW key, cached buffers are never
// written, eviction only drops the lookup. The best-available fallback hands out a different buffer, but must never
// mutate one being played.

// I1 — once a voice is scheduled, nothing can alter its samples.
func TestI1_InFlightVoiceIsImmutableAcrossParamEdit(t *testing.T) {
	const id = roster.OrganChurch
	const pitch = 2.0
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	v := newRecipeAwareVoicePitched(id, SampleRate(), pitch)
	cv, ok := v.(*cVoice)
	if !ok {
		t.Fatalf("expected *cVoice, got %T", v)
	}
	if len(cv.buf) == 0 {
		t.Fatal("voice buffer is empty")
	}
	before := append([]float32(nil), cv.buf...) // snapshot

	// Storm the params while the voice is "in flight".
	for i := 0; i < 50; i++ {
		SetInstrumentParam(id, churnableParam(t, string(id)), float64(i)/50.0)
	}

	if len(cv.buf) != len(before) {
		t.Fatalf("in-flight voice buffer was resized: %d -> %d", len(before), len(cv.buf))
	}
	for i := range before {
		if cv.buf[i] != before[i] {
			t.Fatalf("param edit mutated an already-triggered voice at sample %d: %v -> %v",
				i, before[i], cv.buf[i])
		}
	}
}

func TestI2_EditAppliesToNextTriggerAndComposes(t *testing.T) {
	const id = roster.OrganChurch
	const pitch = 2.0
	globalVoiceCache.Clear()
	ResetInstrumentParams(id)
	t.Cleanup(func() { ResetInstrumentParams(id) })

	p := churnableParam(t, string(id))

	k0, ok := pitchedVoiceKey(id, SampleRate(), pitch)
	if !ok {
		t.Fatal("pitchedVoiceKey failed for a melodic instrument")
	}

	SetInstrumentParam(id, p, 0.25)
	k1, _ := pitchedVoiceKey(id, SampleRate(), pitch)
	if k1 == k0 {
		t.Fatal("a param edit did not rotate the binding — the next trigger would read stale audio")
	}

	// Returning to the original value returns to the original binding.
	SetInstrumentParam(id, p, 0.75)
	k2, _ := pitchedVoiceKey(id, SampleRate(), pitch)
	if k2 == k1 {
		t.Fatal("second edit did not rotate the binding")
	}
	SetInstrumentParam(id, p, 0.25)
	k3, _ := pitchedVoiceKey(id, SampleRate(), pitch)
	if k3 != k1 {
		t.Fatal("binding is not a pure function of the composed param state " +
			"(same value must yield the same binding)")
	}
}

// I4 — an edit to instrument A cannot change instrument B's binding.
func TestI4_EditIsolatedToItsInstrument(t *testing.T) {
	const a = roster.OrganChurch
	const b = roster.Snare
	ResetInstrumentParams(a)
	ResetInstrumentParams(b)
	t.Cleanup(func() { ResetInstrumentParams(a); ResetInstrumentParams(b) })

	bBefore := docHashForTest(string(b))
	aBefore := docHashForTest(string(a))

	// Must vary per iteration: same-value writes are coalesced, so a constant would make every iteration a no-op.
	for i := 0; i < 20; i++ {
		SetInstrumentParam(a, churnableParam(t, string(a)), float64(i)/20.0)
	}

	// Vacuity guard: B's document is a separate entry, so "B unchanged" would pass even if SetInstrumentParam did nothing.
	aAfter := docHashForTest(string(a))
	if aAfter == aBefore {
		t.Fatal("A's binding did not change — the edits never landed, so the " +
			"'B unchanged' assertion below is vacuous")
	}

	bAfter := docHashForTest(string(b))
	if bAfter != bBefore {
		t.Fatalf("editing %q changed %q's binding (%x -> %x)", a, b, bBefore, bAfter)
	}
}

// Alphabetically first, excluding "pitch".
func churnableParam(t *testing.T, id string) string {
	t.Helper()
	owned := GetInstrumentParams(instrument.ID(id))
	best := ""
	for _, d := range InstrumentParamDefs(instrument.ID(id)) {
		if d.Name == "pitch" || len(d.Enum) > 0 || d.Max <= d.Min || d.Group == SynthHiddenGroup {
			continue
		}
		if _, ok := owned[d.Name]; !ok {
			continue
		}
		if best == "" || d.Name < best {
			best = d.Name
		}
	}
	if best == "" {
		t.Fatalf("instrument %q has no editable knob", id)
	}
	return best
}

// docHashForTest identifies id's current sound: its document's block hash.
func docHashForTest(id string) uint64 {
	_, h, _, _ := docBlockForDispatch(instrument.ID(id))
	return h
}
