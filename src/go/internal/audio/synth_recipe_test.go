package audio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParamDefStepFieldRoundTrips(t *testing.T) {
	def := ParamDef{Name: "osc_octave", Min: -2, Max: 2, Default: 0, Step: 1}
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"step":1`) {
		t.Fatalf("expected step in JSON, got %s", b)
	}
	z := ParamDef{Name: "cutoff", Min: 20, Max: 20000}
	bz, _ := json.Marshal(z)
	if strings.Contains(string(bz), "step") {
		t.Fatalf("zero step must be omitted, got %s", bz)
	}
	var back ParamDef
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Step != 1 {
		t.Fatalf("step round-trip = %v, want 1", back.Step)
	}
}
