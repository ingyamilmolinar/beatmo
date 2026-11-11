package audio

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestFamilyFieldsHaveTheirFamilyCapacity: a family's per-slot fields exist only
// for the slots that family can occupy (Phase C task C3). Every shipped seed
// puts a kick/snare/tom/FM/bass/Karplus-Strong voice in slot 1 and a cymbal
// plate in slots 1-3; the generic gen-bank fields keep all 12 slots.
func TestFamilyFieldsHaveTheirFamilyCapacity(t *testing.T) {
	want := map[string]int{"kick_h2": 1, "kick_hold": 1, "kick_mode_gain": 1, "kick_reverb": 1,
		"snare_tone2": 1, "tom_sweep": 1, "fm_r1": 1, "ks_sustain": 1, "harm_mix": 1,
		"cym_tune": 3, "cym_rattle": 3, "source": 12, "filt_freq": 12}
	seen := 0
	for _, f := range modularFields {
		if n, ok := want[f.Name]; ok {
			seen++
			if f.Slots != n {
				t.Errorf("%s: %d slots, want %d", f.Name, f.Slots, n)
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("found %d of %d probed fields — table names changed?", seen, len(want))
	}
	// Stage TABLE cells are a deliberate, separately capped block
	// (TestTablesAreModelParametersOnly); the slot-scoping budget is about the
	// knob fields.
	n := 0
	for _, f := range ModularParamSchema() {
		if !IsTableField(f) {
			n++
		}
	}
	if n > 470 {
		t.Errorf("param block has %d non-table doubles; slot-scoped families should leave ~430", n)
	}
}

// TestEverySeedRespectsSlotCapacity: no shipped seed places a family voice in a
// slot its family has no fields for.
func TestEverySeedRespectsSlotCapacity(t *testing.T) {
	for _, id := range roster.All {
		if err := validateSlotCapacity(MergedInstrumentParamsRO(id)); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

func TestValidateSlotCapacityNamesTheSlot(t *testing.T) {
	err := validateSlotCapacity(RecipeParams{"gen2_source": 5})
	if err == nil || !strings.Contains(err.Error(), "gen2_source") {
		t.Fatalf("kick voice in slot 2: err = %v, want an error naming gen2_source", err)
	}
	if err := validateSlotCapacity(RecipeParams{"gen3_source": 9, "gen12_source": 1}); err != nil {
		t.Fatalf("cymbal in slot 3 / oscillator in slot 12 rejected: %v", err)
	}
}

// TestSetInstrumentParamRefusesAFamilyVoiceBeyondCapacity: a live write that
// would put a family voice in a slot it cannot occupy is refused, like an
// unknown name. Stored, it renders silence and every later undo/redo and
// re-import of the session (both validate the whole map) is rejected.
func TestSetInstrumentParamRefusesAFamilyVoiceBeyondCapacity(t *testing.T) {
	const id = roster.Kick
	t.Cleanup(func() { ResetInstrumentParams(id) })
	SetInstrumentParam(id, "gen2_source", 5) // a second kick voice: capacity 1
	if v, ok := GetInstrumentParams(id)["gen2_source"]; ok && v == 5 {
		t.Fatal("gen2_source=5 (kick) was stored past the kick block's 1-slot capacity")
	}
	SetInstrumentParam(id, "gen2_source", 9) // cymbal: capacity 3, allowed
	if v := GetInstrumentParams(id)["gen2_source"]; v != 9 {
		t.Fatalf("gen2_source=9 (cymbal, capacity 3) was refused: got %v", v)
	}
}

// TestSlotCapacityRoundsLikeTheEngine: C picks the voice with lrint (round half
// to even), so 8.5 is a CLAP (8, capacity 1) to the engine — Go must judge the
// same voice, not math.Round's cymbal (9, capacity 3).
func TestSlotCapacityRoundsLikeTheEngine(t *testing.T) {
	if err := validateSlotCapacity(RecipeParams{"gen2_source": 8.5}); err == nil {
		t.Fatal("gen2_source=8.5 renders the clap voice (lrint) in slot 2, past its capacity, but was accepted")
	}
}
