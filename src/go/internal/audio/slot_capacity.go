package audio

import (
	"fmt"
	"math"
)

// Sources 1 (wavetable) and 2 (noise tap) read only generic fields, which every slot has.
var sourceFamilyBlock = map[int]string{
	3: "ks", 4: "bass", 5: "kick", 6: "tom", 7: "snare", 8: "snare", 9: "cym", 10: "fm",
}

// 0 = no family block, every slot works.
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

// The engine would render that slot as silence.
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
