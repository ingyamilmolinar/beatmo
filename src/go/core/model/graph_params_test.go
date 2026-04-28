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

func TestNormalizeNodeParamsSkipEveryNMigration(t *testing.T) {
	t.Run("legacy_field_with_no_kind_promotes_to_skip_every_n", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{SkipEveryN: 3})
		if got.LogicKind != "skip_every_n" {
			t.Fatalf("kind=%q want skip_every_n", got.LogicKind)
		}
		if got.LogicN != 3 {
			t.Fatalf("LogicN=%d want 3", got.LogicN)
		}
		if got.SkipEveryN != 0 {
			t.Fatalf("SkipEveryN should be cleared, got %d", got.SkipEveryN)
		}
	})

	t.Run("legacy_field_with_skip_every_n_kind_fills_logic_n", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{SkipEveryN: 4, LogicKind: "skip_every_n", LogicN: 0})
		if got.LogicN != 4 {
			t.Fatalf("LogicN=%d want 4", got.LogicN)
		}
		if got.SkipEveryN != 0 {
			t.Fatalf("SkipEveryN should be cleared, got %d", got.SkipEveryN)
		}
	})

	t.Run("legacy_field_does_not_override_explicit_logic_n", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{SkipEveryN: 4, LogicKind: "skip_every_n", LogicN: 7})
		if got.LogicN != 7 {
			t.Fatalf("LogicN=%d want 7 (explicit value preserved)", got.LogicN)
		}
		if got.SkipEveryN != 0 {
			t.Fatalf("SkipEveryN should be cleared, got %d", got.SkipEveryN)
		}
	})

	t.Run("legacy_field_with_unrelated_kind_is_just_cleared", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{SkipEveryN: 5, LogicKind: "probability", LogicP: 0.5})
		if got.LogicKind != "probability" {
			t.Fatalf("kind=%q want probability (unchanged)", got.LogicKind)
		}
		if got.LogicN != 0 {
			t.Fatalf("LogicN should remain 0, got %d", got.LogicN)
		}
		if got.SkipEveryN != 0 {
			t.Fatalf("SkipEveryN should be cleared, got %d", got.SkipEveryN)
		}
	})

	t.Run("zero_legacy_field_is_a_noop", func(t *testing.T) {
		got := normalizeNodeParams(NodeParams{LogicKind: "probability", LogicP: 0.25})
		if got.LogicKind != "probability" || got.LogicP != 0.25 {
			t.Fatalf("unexpected: %+v", got)
		}
	})
}
