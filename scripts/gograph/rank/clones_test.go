package rank

import (
	"sort"
	"strings"
	"testing"
)

// cloneNames renders each group as "tier:whole|window:unitA,unitB,…" for
// readable assertions.
func cloneNames(m *Model) map[string]*CloneGroup {
	out := map[string]*CloneGroup{}
	for _, g := range m.Clones {
		var names []string
		for _, mem := range g.Members {
			names = append(names, m.byID[mem.Unit].Name)
		}
		sort.Strings(names)
		kind := "window"
		if g.Whole {
			kind = "whole"
		}
		out[g.Tier+":"+kind+":"+strings.Join(names, ",")] = g
	}
	return out
}

func keysOf(m map[string]*CloneGroup) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestClonesExactWithRenamedLocals(t *testing.T) {
	got := cloneNames(loadFixture(t))
	if _, ok := got["exact:whole:cloneA,cloneB"]; !ok {
		t.Errorf("cloneA/cloneB not an exact whole-function group; groups: %v", keysOf(got))
	}
}

func TestClonesSameShapeDifferentDataIsStructuralOnly(t *testing.T) {
	got := cloneNames(loadFixture(t))
	if _, ok := got["exact:whole:stringA,stringB"]; ok {
		t.Errorf("stringA/stringB reported as exact: different string data must not be exact")
	}
	if _, ok := got["structural:whole:stringA,stringB"]; !ok {
		t.Errorf("stringA/stringB not a structural group; groups: %v", keysOf(got))
	}
}

func TestClonesLargeGroupIsOneGroup(t *testing.T) {
	m := loadFixture(t)
	got := cloneNames(m)
	g, ok := got["exact:whole:w1,w2,w3,w4,w5"]
	if !ok {
		t.Fatalf("w1..w5 not one exact group; groups: %v", keysOf(got))
	}
	if len(g.Members) != 5 {
		t.Errorf("members = %d, want 5", len(g.Members))
	}
	n := 0
	for k := range got {
		if strings.Contains(k, "w1") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("w1 appears in %d groups, want 1", n)
	}
}

func TestClonesWindowsMergeIntoOneMaximalRegion(t *testing.T) {
	got := cloneNames(loadFixture(t))
	g, ok := got["exact:window:longA,longB"]
	if !ok {
		t.Fatalf("longA/longB shared run not found; groups: %v", keysOf(got))
	}
	if g.Lines != 14 {
		t.Errorf("region lines = %d, want 14 (one maximal region)", g.Lines)
	}
	n := 0
	for k := range got {
		if strings.Contains(k, "longA") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("longA appears in %d groups, want 1 (overlapping windows must merge)", n)
	}
}

func TestClonesSkipGeneratedCode(t *testing.T) {
	for k := range cloneNames(loadFixture(t)) {
		if strings.Contains(k, "Gen") {
			t.Errorf("generated unit in clone group %s", k)
		}
	}
}

func TestRepeatedLinesInsideOneUnit(t *testing.T) {
	fset, fd := parseFunc(t, `
func rep(a, b []int) {
	x := len(a)
	if x > 3 {
		x = 3
	}
	a = a[:x]
	y := len(b)
	if y > 3 {
		y = 3
	}
	b = b[:y]
	_, _ = a, b
}`, "rep")
	if got := measureVerbosity(fset, fd.Body, localsOf(fd.Recv, fd.Type, fd.Body)).RepeatedLines; got != 5 {
		t.Errorf("RepeatedLines = %d, want 5 (one repeat of a 5-line run)", got)
	}
}

func TestClonesRenamedTierForDifferentNonLocalNames(t *testing.T) {
	got := cloneNames(loadFixture(t))
	if _, ok := got["exact:whole:toggleLogs,toggleSettings"]; ok {
		t.Errorf("toggles differ in the package variable they touch: not exact")
	}
	if _, ok := got["renamed:whole:toggleLogs,toggleSettings"]; !ok {
		t.Errorf("toggles not a renamed-tier group; groups: %v", keysOf(got))
	}
}

func TestAssignmentTableIsNotRepeatedCode(t *testing.T) {
	if got := oneUnit(t, loadFixture(t), "aliases").Verb.RepeatedLines; got != 0 {
		t.Errorf("aliases RepeatedLines = %d, want 0 (distinct names are not a repeat)", got)
	}
}
