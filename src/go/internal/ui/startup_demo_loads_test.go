package ui

import (
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
)

// Nothing about WHAT the demo holds: it is free to change by hand. A demo that fails to import launches the app
// empty with only an ERROR line.
func TestStartupDemoLoads(t *testing.T) {
	g := buildTestGame(t)
	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		t.Fatalf("the embedded startup demo does not import: %v", err)
	}
}
