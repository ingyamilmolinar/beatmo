package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFuncIDFormat(t *testing.T) {
	got := FuncID("internal/audio", "Engine", "Process")
	want := "internal/audio\tEngine\tProcess"
	if got != want {
		t.Fatalf("FuncID = %q, want %q", got, want)
	}
	free := FuncID("internal/ui", "", "NewDrumView")
	if strings.Count(free, "\t") != 2 {
		t.Fatalf("FuncID must always have two tabs, got %q", free)
	}
}

func TestGraphJSONRoundTrip(t *testing.T) {
	g := Graph{
		Module: "m",
		Config: map[string]string{"goos": "linux", "tags": ""},
		Packages: []Package{{
			Path: "p", LOC: 10, Files: 1,
			Funcs: []FuncMetric{{Name: "F", LOC: 5, Nesting: 2, Cyclo: 3, CallDepth: 1}},
		}},
		Edges: []Edge{{From: "a", To: "b", Calls: 2, Methods: []string{"b.X"}}},
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var back Graph
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Packages[0].Funcs[0].Cyclo != 3 || back.Edges[0].Calls != 2 {
		t.Fatalf("round trip lost data: %+v", back)
	}
}
