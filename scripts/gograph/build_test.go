package main

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestBuildGraphGolden(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	g, err := BuildGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const golden = "testdata/fixture_golden.json"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run -update first): %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("model mismatch.\n--- got ---\n%s", got)
	}
}

func TestBuildGraphCallDepthAttached(t *testing.T) {
	cfg := Config{Root: "testdata/fixturemod", Module: "example.com/fixture", GOOS: "linux"}
	g, err := BuildGraph(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range g.Packages {
		if p.Path == "example.com/fixture/bar" {
			for _, f := range p.Funcs {
				if f.Name == "Run" && f.CallDepth < 2 {
					t.Fatalf("Run CallDepth = %d, want >=2 (must be joined from ResolveEdges)", f.CallDepth)
				}
			}
		}
		if p.Path == "example.com/fixture/foo" {
			var found bool
			for _, ty := range p.Types {
				if ty.Name != "Thing" {
					continue
				}
				for _, m := range ty.Methods {
					if m.Name == "Deep" {
						found = true
						if m.CallDepth != 2 {
							t.Fatalf("Thing.Deep CallDepth = %d, want 2 (method-receiver join must match; a broken join would default to 1)", m.CallDepth)
						}
					}
				}
			}
			if !found {
				t.Fatalf("Thing.Deep method not found in foo package")
			}
		}
	}
}
