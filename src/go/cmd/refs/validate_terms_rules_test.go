//go:build !js

package main

import "testing"

func TestTermRulesBindToRegisteredAxes(t *testing.T) {
	if len(termRules) != 11 {
		t.Fatalf("spec §2.4 defines 11 rules, got %d", len(termRules))
	}
	for _, r := range termRules {
		if !registryHasAxis(r.Axis) {
			t.Errorf("rule %v names unknown axis %q", r.Words, r.Axis)
		}
		if r.WantDir != "high" && r.WantDir != "low" {
			t.Errorf("rule %v: WantDir must be high|low", r.Words)
		}
	}
}

// TestAggregateTermScores buckets synthetic termScores. The point of the test
// is the PARTITION: agreeing + backwards + silent must equal the number of
// rules that saw at least one family, because the spec §9.2 criterion is
// stated as a fraction of those rules and was once mis-counted by treating
// MISS rows (silent) as agreement.
func TestAggregateTermScores(t *testing.T) {
	hit := termHit{Family: "x"}
	scores := []termScore{
		// no family at all -- the word was never used; counted nowhere.
		{Rule: termRule{Words: []string{"unused"}}},
		// fired twice, both in the implied direction.
		{Rule: termRule{Words: []string{"airy"}}, Hits: []termHit{hit, hit}, Fired: 2, Agree: 2, Measured: 2},
		// fired once, in the implied direction.
		{Rule: termRule{Words: []string{"metallic"}}, Hits: []termHit{hit}, Fired: 1, Agree: 1, Measured: 1},
		// fired once, the OTHER way.
		{Rule: termRule{Words: []string{"wooden"}}, Hits: []termHit{hit}, Fired: 1, Agree: 0, Measured: 1},
		// fired twice, agreed once -- still not a clean agreement.
		{Rule: termRule{Words: []string{"mixed"}}, Hits: []termHit{hit, hit}, Fired: 2, Agree: 1, Measured: 2},
		// saw families but never fired: the axis says nothing about the word.
		{Rule: termRule{Words: []string{"nasal"}}, Hits: []termHit{hit, hit}, Fired: 0, Agree: 0, Measured: 2},
	}
	got := aggregateTermScores(scores)
	want := termAggregate{WithFamily: 5, Agreeing: 2, Backwards: 2, Silent: 1}
	if got != want {
		t.Fatalf("aggregateTermScores = %+v, want %+v", got, want)
	}
	if got.Agreeing+got.Backwards+got.Silent != got.WithFamily {
		t.Errorf("buckets do not partition: %+v", got)
	}
	const wantStr = "rules with a family: 5; agreeing (AGREE==FIRED>0): 2; backwards: 2; silent (FIRED==0): 1"
	if got.String() != wantStr {
		t.Errorf("String() = %q, want %q", got.String(), wantStr)
	}
}

// TestAggregateTermScores_AllSilent is the degenerate case the report must not
// dress up: rules that saw families but never fired are silent, not agreeing.
func TestAggregateTermScores_AllSilent(t *testing.T) {
	hit := termHit{Family: "x"}
	got := aggregateTermScores([]termScore{
		{Rule: termRule{Words: []string{"a"}}, Hits: []termHit{hit}},
		{Rule: termRule{Words: []string{"b"}}, Hits: []termHit{hit}},
	})
	if got.Agreeing != 0 || got.Silent != 2 || got.Backwards != 0 || got.WithFamily != 2 {
		t.Fatalf("all-silent aggregate = %+v", got)
	}
}
