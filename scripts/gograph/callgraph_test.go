package main

import "testing"

func TestResolveEdgesFixture(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	edges, depth, memberEdges, err := ResolveEdges(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Intra-package member edge: foo.Thing.Deep calls the top-level foo.Helper,
	// so the foo package must have a Thing -> Helper member edge (method attributed
	// to its receiver type). No cross-package edge is created for it.
	foo := memberEdges["example.com/fixture/foo"]
	var deepHelper *MemberEdge
	for i := range foo {
		if foo[i].From == "Thing" && foo[i].To == "Helper" {
			deepHelper = &foo[i]
		}
	}
	if deepHelper == nil {
		t.Fatalf("expected foo member edge Thing->Helper, got %+v", foo)
	}
	if deepHelper.Calls < 1 {
		t.Errorf("Thing->Helper calls = %d, want >=1", deepHelper.Calls)
	}
	// A member must never edge to itself.
	for _, me := range foo {
		if me.From == me.To {
			t.Errorf("unexpected self member-edge %+v", me)
		}
	}

	// bar -> foo must exist, and MUST include the method-on-imported-type call
	// foo.Thing.Process (the case AST-only analysis would miss) plus foo.Helper.
	var barFoo *Edge
	for i := range edges {
		if edges[i].From == "example.com/fixture/bar" && edges[i].To == "example.com/fixture/foo" {
			barFoo = &edges[i]
		}
	}
	if barFoo == nil {
		t.Fatalf("expected bar->foo edge, got %+v", edges)
	}
	if barFoo.Calls < 2 {
		t.Errorf("bar->foo calls = %d, want >=2 (Helper + Process)", barFoo.Calls)
	}
	if !hasMethodLike(barFoo.Methods, "Process") {
		t.Errorf("bar->foo methods missing Process (method-on-imported-type): %v", barFoo.Methods)
	}
	if !hasMethodLike(barFoo.Methods, "Helper") {
		t.Errorf("bar->foo methods missing Helper: %v", barFoo.Methods)
	}

	// No self-edge (same-package calls excluded).
	for _, e := range edges {
		if e.From == e.To {
			t.Errorf("unexpected self-edge %+v", e)
		}
	}

	// Run() calls into foo, so its call depth is >=2.
	runID := FuncID("example.com/fixture/bar", "", "Run")
	if depth[runID] < 2 {
		t.Errorf("Run callDepth = %d, want >=2", depth[runID])
	}
}

func hasMethodLike(xs []string, sub string) bool {
	for _, x := range xs {
		if len(x) >= len(sub) && (x == sub || containsSub(x, sub)) {
			return true
		}
	}
	return false
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestComputeDepthsDeterministicWithCycle(t *testing.T) {
	// D -> C, B -> C, C -> B  (B<->C cycle; D reaches C from outside the cycle)
	callees := map[string]map[string]bool{
		"D": {"C": true},
		"B": {"C": true},
		"C": {"B": true},
	}
	first := computeDepths(callees)
	for i := 0; i < 30; i++ {
		got := computeDepths(callees)
		for k, v := range first {
			if got[k] != v {
				t.Fatalf("nondeterministic depth for %s: %d vs %d", k, v, got[k])
			}
		}
	}
	// B,C collapse to one sink SCC => depth 1; D is one hop above => 2.
	if first["B"] != 1 || first["C"] != 1 {
		t.Errorf("cycle SCC depth: B=%d C=%d, want 1,1", first["B"], first["C"])
	}
	if first["D"] != 2 {
		t.Errorf("D depth = %d, want 2", first["D"])
	}
}

func TestComputeDepthsSelfRecursion(t *testing.T) {
	d := computeDepths(map[string]map[string]bool{"R": {"R": true}})
	if d["R"] != 1 {
		t.Errorf("self-recursive R depth = %d, want 1", d["R"])
	}
}

func TestComputeDepthsChain(t *testing.T) {
	// A -> B -> C (acyclic) => A=3, B=2, C=1
	d := computeDepths(map[string]map[string]bool{
		"A": {"B": true},
		"B": {"C": true},
	})
	if d["A"] != 3 || d["B"] != 2 || d["C"] != 1 {
		t.Errorf("chain depths A=%d B=%d C=%d, want 3,2,1", d["A"], d["B"], d["C"])
	}
}
