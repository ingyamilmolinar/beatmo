package model

import (
	"reflect"
	"strings"
	"testing"
)

// Per-node synth knobs had no audio consumer: every synth param lives on the instrument's document.
func TestNodeParamsCarryNoLegacySynthFields(t *testing.T) {
	ty := reflect.TypeOf(NodeParams{})
	for i := 0; i < ty.NumField(); i++ {
		if name := ty.Field(i).Name; strings.HasPrefix(name, "Synth") {
			t.Errorf("NodeParams.%s is a legacy per-node synth knob with no consumer", name)
		}
	}
}
