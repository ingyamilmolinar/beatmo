package main

import (
	"strings"
	"testing"
)

func TestRenderHTMLSelfContained(t *testing.T) {
	g := &Graph{
		Module: "m",
		Config: map[string]string{"goos": "linux", "tags": ""},
		Packages: []Package{{Path: "m/a", LOC: 100, Funcs: []FuncMetric{{Name: "F", LOC: 9, Nesting: 2, Cyclo: 3, CallDepth: 1}}}},
		Edges:    []Edge{{From: "m/a", To: "m/b", Calls: 4, Methods: []string{"b.X"}}},
	}
	out, err := RenderHTML(g)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// self-contained: no external RESOURCE LOADS. We check for real network-load
	// patterns, NOT a bare "http://" — app.js legitimately contains the SVG
	// namespace constant "http://www.w3.org/2000/svg" (required by
	// createElementNS; the browser never fetches it), which is not a network ref.
	for _, bad := range []string{
		"src=\"http", "src='http", "src=\"//",
		"href=\"http", "href=\"//",
		"<link", "<script src", "@import", "cdn.",
		"fetch(", "XMLHttpRequest", "import(",
	} {
		if strings.Contains(s, bad) {
			t.Errorf("HTML must be self-contained; found external-load pattern %q", bad)
		}
	}
	// belt-and-suspenders: the ONLY allowed http(s) literal is the SVG namespace.
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "http") && !strings.Contains(line, "www.w3.org/2000/svg") {
			t.Errorf("unexpected http reference: %q", strings.TrimSpace(line))
		}
	}
	// embeds the model and the control IDs the app.js references
	if !strings.Contains(s, "\"m/a\"") {
		t.Error("model JSON not embedded")
	}
	for _, id := range []string{"id=\"graph\"", "id=\"crumb\"", "id=\"filter\"", "id=\"panel\""} {
		if !strings.Contains(s, id) {
			t.Errorf("missing control %q", id)
		}
	}
}
