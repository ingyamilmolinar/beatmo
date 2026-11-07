package audio

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Shipped documents put the drum/FM/bass/KS voices in slot 1 and the cymbal plate in 1-3; generic fields keep all 12.
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
	// Table cells are capped separately; this budget is the knob fields.
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

// Stored, it renders silence and every later undo/redo and re-import (which validate the whole map) is rejected.
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

// C picks the voice with lrint (half to even), so 8.5 is a CLAP to the engine, not math.Round's cymbal.
func TestSlotCapacityRoundsLikeTheEngine(t *testing.T) {
	if err := validateSlotCapacity(RecipeParams{"gen2_source": 8.5}); err == nil {
		t.Fatal("gen2_source=8.5 renders the clap voice (lrint) in slot 2, past its capacity, but was accepted")
	}
}
