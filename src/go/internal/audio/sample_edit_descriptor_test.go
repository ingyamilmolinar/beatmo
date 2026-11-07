package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// resetSampleEditsForTest clears the descriptor registry between tests.
func resetSampleEditsForTest(t *testing.T) {
	t.Helper()
	sampleEditsMu.Lock()
	sampleEdits = map[instrument.ID]SampleEdit{}
	sampleEditsMu.Unlock()
}

func nonTrivialEdit() SampleEdit {
	return SampleEdit{StartFrac: 0.25, EndFrac: 0.75, GainDB: -6, Reverse: true}
}

func TestSampleEditSetGetClear(t *testing.T) {
	resetSampleEditsForTest(t)
	const id = roster.Kick
	if HasSampleEdit(id) {
		t.Fatal("fresh registry: HasSampleEdit must be false")
	}
	want := nonTrivialEdit()
	SetSampleEdit(id, want)
	if !HasSampleEdit(id) {
		t.Fatal("after Set: HasSampleEdit must be true")
	}
	got, ok := SampleEditFor(id)
	if !ok || got != want {
		t.Fatalf("SampleEditFor = %+v, %v; want %+v, true", got, ok, want)
	}
	ClearSampleEdit(id)
	if HasSampleEdit(id) {
		t.Fatal("after Clear: HasSampleEdit must be false")
	}
	if _, ok := SampleEditFor(id); ok {
		t.Fatal("after Clear: SampleEditFor must report absent")
	}
}

func TestSampleEditIdentitySetClears(t *testing.T) {
	resetSampleEditsForTest(t)
	const id = roster.Snare
	SetSampleEdit(id, nonTrivialEdit())
	SetSampleEdit(id, SampleEdit{StartFrac: 0, EndFrac: 1})
	if HasSampleEdit(id) {
		t.Fatal("identity Set must clear the descriptor")
	}
}

func TestSampleEditIsIdentity(t *testing.T) {
	cases := []struct {
		name string
		e    SampleEdit
		want bool
	}{
		{"full-range no-op", SampleEdit{StartFrac: 0, EndFrac: 1}, true},
		{"trim", SampleEdit{StartFrac: 0.1, EndFrac: 1}, false},
		{"gain", SampleEdit{EndFrac: 1, GainDB: -3}, false},
		{"reverse", SampleEdit{EndFrac: 1, Reverse: true}, false},
		{"normalize", SampleEdit{EndFrac: 1, Normalize: true}, false},
		{"pitch", SampleEdit{EndFrac: 1, TransposeSemis: 1}, false},
		{"detune", SampleEdit{EndFrac: 1, DetuneCents: 5}, false},
		{"fade", SampleEdit{EndFrac: 1, Fade: true}, false},
	}
	for _, c := range cases {
		if got := c.e.isIdentity(); got != c.want {
			t.Errorf("%s: isIdentity = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHashSampleEditDeterministicAndDiscriminating(t *testing.T) {
	a, b := nonTrivialEdit(), nonTrivialEdit()
	if hashSampleEdit(a) != hashSampleEdit(b) {
		t.Fatal("equal edits must hash equal")
	}
	b.GainDB = -3
	if hashSampleEdit(a) == hashSampleEdit(b) {
		t.Fatal("differing edits must hash differently")
	}
	c := a
	c.Reverse = false
	if hashSampleEdit(a) == hashSampleEdit(c) {
		t.Fatal("Reverse flag must contribute to the hash")
	}
}

func TestApplySampleEditToBufferDelegatesToBakeSample(t *testing.T) {
	src := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	e := nonTrivialEdit()
	got := ApplySampleEditToBuffer(append([]float32(nil), src...), 48000, e)
	want := BakeSample(append([]float32(nil), src...), 48000, e)
	if len(got) != len(want) {
		t.Fatalf("len %d != %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sample %d: %v != %v (must be the SAME transform as BakeSample)", i, got[i], want[i])
		}
	}
}

func TestSampleEditFieldsRoundTrip(t *testing.T) {
	e := SampleEdit{StartFrac: 0.2, EndFrac: 0.8, TransposeSemis: 3, DetuneCents: -15, GainDB: -6, Fade: true, Reverse: true, Normalize: true}
	got := SampleEditFromFields(e.Fields())
	if got != e {
		t.Fatalf("Fields round-trip lost data: %+v != %+v", got, e)
	}
	// Zero-value-with-full-range round-trips too.
	id := SampleEdit{EndFrac: 1}
	if got := SampleEditFromFields(id.Fields()); got != id {
		t.Fatalf("identity round-trip: %+v != %+v", got, id)
	}
}

type stubSampleEditSource map[instrument.ID]map[string]float64

func (s stubSampleEditSource) LoadSampleEdits() (map[instrument.ID]map[string]float64, error) {
	return s, nil
}

func TestSampleEditPlatformHookFiresOnSetAndClear(t *testing.T) {
	resetSampleEditsForTest(t)
	type call struct {
		id instrument.ID
		e  SampleEdit
	}
	var calls []call
	prev := SwapPlatformSampleEditChangedForTest(func(id instrument.ID, e SampleEdit) {
		calls = append(calls, call{id, e})
	})
	t.Cleanup(func() { SwapPlatformSampleEditChangedForTest(prev) })

	want := nonTrivialEdit()
	SetSampleEdit(roster.Hihat, want)
	if len(calls) != 1 || calls[0].id != roster.Hihat || calls[0].e != want {
		t.Fatalf("after Set: calls = %+v", calls)
	}
	ClearSampleEdit(roster.Hihat)
	if len(calls) != 2 || calls[1].id != roster.Hihat || !calls[1].e.isIdentity() {
		t.Fatalf("after Clear: calls = %+v (clear must push an identity edit)", calls)
	}
	// Clearing an absent id must not fire the bridge again.
	ClearSampleEdit(roster.Hihat)
	if len(calls) != 2 {
		t.Fatalf("clear of absent id fired the bridge: %+v", calls)
	}
}
