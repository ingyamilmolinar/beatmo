package main

import "testing"

func TestAnalyzeMetricsFixture(t *testing.T) {
	pkgs, err := AnalyzeMetrics("testdata/fixturemod", "example.com/fixture")
	if err != nil {
		t.Fatal(err)
	}
	foo := pkgs["example.com/fixture/foo"]
	if foo == nil {
		t.Fatalf("foo package not found; got keys %v", keys(pkgs))
	}

	// Thing type present, is a struct, has the Process method.
	var thing *TypeInfo
	for i := range foo.Types {
		if foo.Types[i].Name == "Thing" {
			thing = &foo.Types[i]
		}
	}
	if thing == nil || thing.Kind != "struct" {
		t.Fatalf("Thing struct not modeled: %+v", foo.Types)
	}
	var process *FuncMetric
	for i := range thing.Methods {
		if thing.Methods[i].Name == "Process" {
			process = &thing.Methods[i]
		}
	}
	if process == nil {
		t.Fatalf("Process method not attached to Thing: %+v", thing.Methods)
	}
	if process.Recv != "Thing" {
		t.Fatalf("Process recv = %q, want Thing", process.Recv)
	}

	// Extra is declared in a different file than Thing (extra.go vs foo.go);
	// renestMethods must still attach it to Thing.Methods, not foo.Funcs.
	var extra *FuncMetric
	for i := range thing.Methods {
		if thing.Methods[i].Name == "Extra" {
			extra = &thing.Methods[i]
		}
	}
	if extra == nil {
		t.Fatalf("cross-file method Extra not nested under Thing (renestMethods): %+v", thing.Methods)
	}
	// Extra must NOT also appear as a free func in foo.
	for _, f := range foo.Funcs {
		if f.Name == "Extra" {
			t.Fatalf("Extra leaked into foo.Funcs; must be nested under Thing only")
		}
	}

	// Helper free func: nesting depth 3 (if > for > if), cyclo 4 (1 + 2 ifs + 1 for).
	var helper *FuncMetric
	for i := range foo.Funcs {
		if foo.Funcs[i].Name == "Helper" {
			helper = &foo.Funcs[i]
		}
	}
	if helper == nil {
		t.Fatalf("Helper func not found: %+v", foo.Funcs)
	}
	if helper.Nesting != 3 {
		t.Errorf("Helper nesting = %d, want 3", helper.Nesting)
	}
	if helper.Cyclo != 4 {
		t.Errorf("Helper cyclo = %d, want 4", helper.Cyclo)
	}
	if helper.LOC < 5 {
		t.Errorf("Helper LOC = %d, want >=5", helper.LOC)
	}

	// bar imports foo.
	bar := pkgs["example.com/fixture/bar"]
	if bar == nil || !contains(bar.Imports, "example.com/fixture/foo") {
		t.Fatalf("bar should import foo: %+v", bar)
	}
}

func keys(m map[string]*Package) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
