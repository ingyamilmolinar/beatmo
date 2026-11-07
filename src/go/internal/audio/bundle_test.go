package audio

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func eqWithGain(db float64) *EQSettings {
	s := NewEQSettings()
	s.GainsDB[3] = db
	return s
}

func TestEffectiveEveryModeAndPart(t *testing.T) {
	edit := &SampleEdit{StartFrac: 0.1, EndFrac: 0.9}
	delay := EffectSlot{Type: EffectDelay, Enabled: true}
	reverb := EffectSlot{Type: EffectReverb, Enabled: true}
	inst := AudioBundle{SampleEdit: edit, EQ: eqWithGain(3), Effects: []EffectSlot{delay}}
	circEQOnly := AudioBundle{EQ: eqWithGain(-6)}
	circFX := AudioBundle{Effects: []EffectSlot{reverb}}

	cases := []struct {
		name string
		circ AudioBundle
		mode BundleMode
		want AudioBundle
	}{
		{"instrument ignores the circuit", circEQOnly, ModeInstrument, inst},
		{"circuit ignores the instrument", circEQOnly, ModeCircuit, circEQOnly},
		{"override: the circuit's parts win, the rest is the instrument's", circEQOnly, ModeOverride,
			AudioBundle{SampleEdit: edit, EQ: eqWithGain(-6), Effects: []EffectSlot{delay}}},
		{"merge: FX chained, instrument first", circFX, ModeMerge,
			AudioBundle{SampleEdit: edit, EQ: eqWithGain(3), Effects: []EffectSlot{delay, reverb}}},
		{"merge: EQ and Sampler edit as override", circEQOnly, ModeMerge,
			AudioBundle{SampleEdit: edit, EQ: eqWithGain(-6), Effects: []EffectSlot{delay}}},
	}
	for _, c := range cases {
		if got := Effective(inst, c.circ, c.mode); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

func TestBundleRoundTripThroughStores(t *testing.T) {
	id := roster.Snare
	t.Cleanup(func() { ApplyAudioBundle(id, AudioBundle{}) })
	want := AudioBundle{
		SampleEdit: &SampleEdit{StartFrac: 0.2, EndFrac: 0.8, GainDB: -3},
		EQ:         eqWithGain(4.5),
		Effects:    []EffectSlot{{Type: EffectDelay, Enabled: true, Params: map[string]float64{"time": 250}}},
	}
	ApplyAudioBundle(id, want)
	got := CurrentAudioBundle(id)
	if got.Digest() != want.Normalized().Digest() {
		t.Fatalf("round trip changed the bundle:\n got %+v\nwant %+v", got, want)
	}
	ApplyAudioBundle(id, AudioBundle{})
	if got := CurrentAudioBundle(id); !got.IsNeutral() {
		t.Fatalf("a neutral bundle left settings behind: %+v", got)
	}
}

func TestFlatEQIsAnAbsentPart(t *testing.T) {
	id := instrument.ID(roster.Kick)
	t.Cleanup(func() { ApplyAudioBundle(id, AudioBundle{}) })
	EQSettingsOf(id).GainsDB[0] = 0 // touched but flat
	if b := CurrentAudioBundle(id); b.EQ != nil {
		t.Fatalf("a flat EQ reads as a part: %+v", b.EQ)
	}
}

func TestBundleDigestIgnoresEffectInstances(t *testing.T) {
	id := roster.Snare
	t.Cleanup(func() { ApplyAudioBundle(id, AudioBundle{}) })
	slots := []EffectSlot{{Type: EffectDelay, Enabled: true, Params: map[string]float64{"time": 250}}}
	SetInsertEffects(id, slots)
	a := CurrentAudioBundle(id).Digest()
	SetInsertEffects(id, nil)
	SetInsertEffects(id, slots) // fresh instances, new uids
	if b := CurrentAudioBundle(id).Digest(); a != b {
		t.Fatalf("the same settings hash differently across effect instances: %s vs %s", a, b)
	}
}

func TestBundleModeDecodesStrictly(t *testing.T) {
	var m BundleMode
	if err := m.UnmarshalText([]byte("merge")); err != nil || m != ModeMerge {
		t.Fatalf("merge: %v %v", m, err)
	}
	if err := m.UnmarshalText([]byte("blend")); err == nil {
		t.Fatal("an unknown mode decoded")
	}
}

func TestUnsetFilterCutoffFiltersNothing(t *testing.T) {
	s := NewEQSettings()
	s.HPF, s.HPFHz = true, 0
	s.LPF, s.LPFHz = true, 0
	if !s.IsFlat() {
		t.Fatal("filters switched on with unset (0) cutoffs read as filtering")
	}
	for _, b := range s.Bands() {
		if b.Kind == EQHighpass || b.Kind == EQLowpass {
			t.Fatalf("an unset cutoff compiled a filter: %+v", b)
		}
	}
}

// A store reading flat over a channel that still filters would play an EQ nobody can see.
func TestResetEQSettingsLeavesNoHiddenEQ(t *testing.T) {
	id := roster.Kick
	t.Cleanup(func() { ApplyAudioBundle(id, AudioBundle{}) })
	EQSettingsOf(id).GainsDB[4] = 6
	ApplyEQSettings(id)
	if len(ChannelEQSnapshot(channel.Of(id)).Bands) == 0 {
		t.Fatal("premise: the EQ did not reach the channel")
	}
	ResetEQSettings()
	if b := ChannelEQSnapshot(channel.Of(id)).Bands; len(b) != 0 {
		t.Fatalf("after ResetEQSettings the channel still filters with %d bands", len(b))
	}
}
