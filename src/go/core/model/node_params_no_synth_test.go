package model

import (
	"reflect"
	"strings"
	"testing"
)

// TestNodeParamsCarryNoLegacySynthFields: the per-node 7-field synth knobs
// (SynthDecay … SynthBrightness) had no audio consumer once the 7-field
// synth_params ABI was deleted — they were dead state copied around and
// persisted into project files. Every synth param lives on the instrument's
// recipe now.
func TestNodeParamsCarryNoLegacySynthFields(t *testing.T) {
	ty := reflect.TypeOf(NodeParams{})
	for i := 0; i < ty.NumField(); i++ {
		if name := ty.Field(i).Name; strings.HasPrefix(name, "Synth") {
			t.Errorf("NodeParams.%s is a legacy per-node synth knob with no consumer", name)
		}
	}
}
