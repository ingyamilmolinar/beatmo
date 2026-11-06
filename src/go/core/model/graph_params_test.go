package model

import (
	"encoding/json"
	"testing"
)

// TestNodeKindsHaveOneSpelling: each logic rule, groove and node type has
// exactly one spelling. The retired aliases ("none", "prev_fired",
// "every_n_loops", "skip_if_prev_*"), other cases and padding are unknown
// spellings — a decode error, never silently mapped onto a rule.
func TestNodeKindsHaveOneSpelling(t *testing.T) {
	for _, k := range LogicKinds() {
		got, err := ParseLogicKind(k.String())
		if err != nil || got != k {
			t.Errorf("logic %d does not round-trip through %q: %v", k, k.String(), err)
		}
	}
	for _, bad := range []string{"none", " None ", "prev_fired", "every_n_loops", "skip_if_prev_skipped", "skip_if_prev_triggered", "Probability", "unknown_kind"} {
		if _, err := ParseLogicKind(bad); err == nil {
			t.Errorf("ParseLogicKind(%q) accepted", bad)
		}
	}
	for _, bad := range []string{"none", "Delay", "swing"} {
		if _, err := ParseGrooveKind(bad); err == nil {
			t.Errorf("ParseGrooveKind(%q) accepted", bad)
		}
	}
	var p struct {
		L LogicKind  `json:"l"`
		G GrooveKind `json:"g"`
		T NodeType   `json:"t"`
	}
	if err := json.Unmarshal([]byte(`{"l":"probability","g":"rush","t":"mute"}`), &p); err != nil || p.L != LogicProbability || p.G != GrooveRush || p.T != NodeTypeMute {
		t.Fatalf("decode = %+v, %v", p, err)
	}
	for _, bad := range []string{`{"l":"prev_fired"}`, `{"g":"swing"}`, `{"t":"Regular"}`} {
		if err := json.Unmarshal([]byte(bad), &p); err == nil {
			t.Errorf("%s decoded", bad)
		}
	}
}
