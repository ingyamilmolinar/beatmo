package model

import "testing"

func TestNormalizeNodeParamsLogicKindRewrites(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"none", ""},
		{" None ", ""},
		{"NONE", ""},
		{"prev_fired", "trigger_if_prev_triggered"},
		{" PREV_FIRED ", "trigger_if_prev_triggered"},
		{"every_n_loops", "every_n_triggers"},
		{"skip_if_prev_skipped", "trigger_if_prev_triggered"},
		{"skip_if_prev_triggered", "trigger_if_prev_skipped"},
		{"probability", "probability"},
		{"every_n_triggers", "every_n_triggers"},
		{"unknown_kind", "unknown_kind"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			got := normalizeNodeParams(NodeParams{LogicKind: tc.in})
			if got.LogicKind != tc.want {
				t.Fatalf("normalize %q: got %q want %q", tc.in, got.LogicKind, tc.want)
			}
		})
	}
}

func TestNormalizeNodeParamsAliasMapping(t *testing.T) {
	t.Run("probability_kind_is_preserved", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{LogicKind: "probability", LogicP: 0.25})
		if got.LogicKind != "probability" || got.LogicP != 0.25 {
			t.Fatalf("unexpected: %+v", got)
		}
	})
}
