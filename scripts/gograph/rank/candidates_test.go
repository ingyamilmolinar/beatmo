package rank

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func candidatesFor(cs []*Candidate, unitName string, m *Model) []*Candidate {
	var out []*Candidate
	for _, c := range cs {
		for _, id := range c.Units {
			if u := m.byID[id]; u != nil && u.Name == unitName {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// unitCandidates keeps the per-unit kinds (split/flatten/verbose).
func unitCandidates(cs []*Candidate) []*Candidate {
	var out []*Candidate
	for _, c := range cs {
		switch c.Kind {
		case "split-function", "flatten", "simplify-verbose":
			out = append(out, c)
		}
	}
	return out
}

func TestDataAndGeneratedNeverYieldComplexityKinds(t *testing.T) {
	m := loadFixture(t)
	cs := Generate(m)
	for _, c := range candidatesFor(cs, "Gen", m) {
		t.Errorf("generated unit yielded candidate %s (%s)", c.Kind, c.Target)
	}
	for _, c := range candidatesFor(cs, "Table", m) {
		if c.Kind != "data-to-asset" {
			t.Errorf("data unit yielded %s, want only data-to-asset", c.Kind)
		}
	}
}

func TestDeepFunctionIsSplitOrFlattenWithTheOtherAsNote(t *testing.T) {
	m := loadFixture(t)
	cs := unitCandidates(candidatesFor(Generate(m), "Deep", m))
	if len(cs) != 1 {
		t.Fatalf("Deep: %d candidates, want 1 (one per unit)", len(cs))
	}
	c := cs[0]
	other := map[string]string{"split-function": "flatten", "flatten": "split-function"}[c.Kind]
	if other == "" {
		t.Fatalf("Deep kind = %s, want split-function or flatten", c.Kind)
	}
	if !slices.ContainsFunc(c.Notes, func(n string) bool { return strings.HasPrefix(n, other) }) {
		t.Errorf("Deep notes %v lack the %s signal", c.Notes, other)
	}
	if c.Score <= 0 || c.Payoff <= 0 || c.Risk < 1 || len(c.Breakdown) == 0 {
		t.Errorf("Deep score/payoff/risk/breakdown = %v/%v/%v/%v", c.Score, c.Payoff, c.Risk, c.Breakdown)
	}
}

// fixtureWeights scales the god-type threshold down to the fixture's 4-method
// type; every other weight is the shipped default.
func fixtureWeights() Weights {
	w := DefaultWeights
	w.GodMinMethods = 3
	return w
}

func TestCandidateKindsFromFixture(t *testing.T) {
	m := loadFixture(t)
	kinds := map[string][]string{}
	for _, c := range GenerateWith(m, fixtureWeights()) {
		kinds[c.Kind] = append(kinds[c.Kind], c.Target)
	}
	for _, want := range []struct{ kind, target string }{
		{"delete-unreachable", "core/dead.go"},
		{"move-test-only", "core/testonly.go"},
		{"dedupe-clone", "core.cloneA"},
		{"dedupe-clone", "core.w1"},
		{"extract-struct", "core.God ← core/god_fx.go"},
	} {
		found := false
		for _, tg := range kinds[want.kind] {
			if strings.HasPrefix(tg, want.target) {
				found = true
			}
		}
		if !found {
			t.Errorf("no %s candidate for %s; have %v", want.kind, want.target, kinds[want.kind])
		}
	}
	for _, tg := range kinds["extract-struct"] {
		if strings.Contains(tg, "god_menu.go") {
			t.Errorf("god_menu.go (2 exclusive fields) is below the extraction threshold")
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	m := loadFixture(t)
	a, _ := json.Marshal(Generate(m))
	b, _ := json.Marshal(Generate(m))
	if string(a) != string(b) {
		t.Fatal("two Generate runs differ")
	}
}

func TestExtractStructOnlyForASliceOfTheType(t *testing.T) {
	m := loadFixture(t)
	for _, c := range GenerateWith(m, fixtureWeights()) {
		if c.Kind == "extract-struct" && strings.Contains(c.Target, "Solo") {
			t.Errorf("solo.go holds every Solo method: not an extraction (%s)", c.Target)
		}
	}
}

func TestSplitFileSkipsFilesThatAreOneFunction(t *testing.T) {
	m := loadFixture(t)
	w := DefaultWeights
	w.SplitFileLines, w.SplitFileKeep = 30, 10
	got := map[string]bool{}
	for _, c := range GenerateWith(m, w) {
		if c.Kind == "split-file" {
			got[c.Target] = true
		}
	}
	if !got["core/clones.go"] {
		t.Errorf("clones.go (many units) not a split-file candidate: %v", got)
	}
	if got["core/deep.go"] {
		t.Errorf("deep.go is one function: split-function covers it, not split-file")
	}
}

func TestOwnSLOCExcludesClosures(t *testing.T) {
	m := loadFixture(t)
	reg, cl := oneUnit(t, m, "Reg"), oneUnit(t, m, "Reg.func1")
	if reg.OwnSLOC != reg.SLOC-cl.SLOC {
		t.Errorf("Reg OwnSLOC = %d, want SLOC %d − closure %d", reg.OwnSLOC, reg.SLOC, cl.SLOC)
	}
}

// Dev tooling counts at ToolShare of its payoff: Deep and branchy are the same
// function, one in the product, one in a tool.
func TestToolingIsDiscounted(t *testing.T) {
	m := loadFixture(t)
	cs := Generate(m)
	deep, tool := unitCandidates(candidatesFor(cs, "Deep", m)), unitCandidates(candidatesFor(cs, "branchy", m))
	if len(deep) != 1 || len(tool) != 1 {
		t.Fatalf("Deep/branchy candidates = %d/%d, want 1/1", len(deep), len(tool))
	}
	want := round3(deep[0].Payoff * DefaultWeights.ToolShare)
	if tool[0].Payoff != want {
		t.Errorf("tool payoff = %v, want %v (product payoff × %v)", tool[0].Payoff, want, DefaultWeights.ToolShare)
	}
	if !slices.ContainsFunc(tool[0].Breakdown, func(s string) bool { return strings.Contains(s, "tooling") }) {
		t.Errorf("tool breakdown does not say why it was discounted: %v", tool[0].Breakdown)
	}
}

func TestCgoBound(t *testing.T) {
	if !cgoBound("C.ifx_delay_process(&d.d, in, out, C.int(n))") {
		t.Error("a cgo call was not recognised")
	}
	if cgoBound("c.process(x); fmt.Println(\"C.x(\")"[:15]) {
		t.Error("a lower-case receiver call was taken for cgo")
	}
}

// A god type's accidental cost sits partly in its state, which no function
// metric sees: restructure-type scores its methods AND its field count.
func TestRestructureTypeCountsState(t *testing.T) {
	m := loadFixture(t)
	for _, c := range GenerateWith(m, fixtureWeights()) {
		if c.Kind == "restructure-type" && c.Target == "core.God" {
			if !slices.ContainsFunc(c.Breakdown, func(s string) bool { return strings.Contains(s, "12 fields") }) {
				t.Errorf("breakdown does not count God's 12 fields: %v", c.Breakdown)
			}
			return
		}
	}
	t.Error("no restructure-type candidate for core.God")
}
