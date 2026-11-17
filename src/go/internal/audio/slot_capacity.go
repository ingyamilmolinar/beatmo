package audio

import (
	"fmt"
	"math"
)

// sourceFamilyBlock maps a gen-slot source to the modularFields Block holding
// its family's per-slot fields. Sources 1 (wavetable oscillator) and 2 (noise
// tap) read only the generic gen-bank fields, which exist for every slot.
var sourceFamilyBlock = map[int]string{
	3: "ks", 4: "bass", 5: "kick", 6: "tom", 7: "snare", 8: "snare", 9: "cym", 10: "fm",
}

// familySlotCapacity is how many gen slots can host a voice of this source: the
// slot count of its family's fields (0 = no family block, every slot works).
func familySlotCapacity(source int) int {
	block, ok := sourceFamilyBlock[source]
	if !ok {
		return 0
	}
	for _, f := range modularFields {
		if f.Block == block {
			return f.Slots
		}
	}
	return 0
}

// validateSlotCapacity rejects params that put a family voice in a slot its
// family has no fields for (the engine would render that slot as silence).
func validateSlotCapacity(p RecipeParams) error {
	for k := 1; k <= modularGenSlots; k++ {
		name := fmt.Sprintf("gen%d_source", k)
		v, ok := p[name]
		if !ok {
			continue
		}
		src := int(math.RoundToEven(v)) // the engine picks the voice with lrint (half to even)
		if c := familySlotCapacity(src); c > 0 && k > c {
			return fmt.Errorf("%s = %d: a %s voice can only occupy slots 1..%d", name, src, sourceFamilyBlock[src], c)
		}
	}
	return nil
}
