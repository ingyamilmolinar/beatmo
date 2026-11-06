package ui

import (
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// TestStartupDemoLoads: the embedded launch circuit imports. It asserts nothing
// about WHAT the circuit holds — the demo is free to change by hand; circuit
// behaviour is tested on purpose-built circuits and the test fixtures. A demo
// that fails to import makes the app launch empty, with only an ERROR line.
func TestStartupDemoLoads(t *testing.T) {
	g := buildTestGame(t)
	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		t.Fatalf("the embedded startup demo does not import: %v", err)
	}
}
