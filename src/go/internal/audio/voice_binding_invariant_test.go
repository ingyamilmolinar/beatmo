//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// voice_binding_invariant_test.go pins the Voice Binding contract from
// docs/superpowers/specs/2026-07-15-realtime-param-edit-audio-model-design.md:
// a node's audio is fully determined at trigger time and immutable afterward.
//
// These invariants hold structurally today (a param edit renders into a NEW
// buffer under a NEW key; cached buffers are never written; eviction only drops
// the map lookup and Go's GC keeps the slice alive for a live reader). They are
// pinned here because Phase 2 adds a best-available fallback that deliberately
// hands out a different buffer than the exact binding requested — the one thing
// it must never do is mutate a buffer someone is already playing.

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

// I2 — an edit is visible to the NEXT trigger, and N edits compose latest-wins
// into ONE binding.
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

	// Latest-wins: setting the same param again lands on ONE binding, and
	// returning to the original value returns to the original binding.
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

	// NOTE: the churn value MUST vary per iteration. SetInstrumentParam coalesces
	// same-value writes (SetInstrumentParam's no-op-on-identical-value skip) and
	// rejects non-finite values, so a constant here would silently make every
	// iteration a no-op and re-vacuum this test.
	for i := 0; i < 20; i++ {
		SetInstrumentParam(a, churnableParam(t, string(a)), float64(i)/20.0)
	}

	// Vacuity guard: prove A's edits actually landed. B's document is a
	// separate entry of the project store, so "B unchanged" is guaranteed by
	// the data structure and would pass even if
	// SetInstrumentParam were a complete no-op. Without this, the test proves nothing.
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

// churnableParam returns a deterministic continuous knob of id's document
// (the alphabetically first one it owns), excluding the special-cased "pitch".
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
