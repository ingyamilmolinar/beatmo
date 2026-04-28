package ui_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/ui"
	"github.com/ingyamilmolinar/beatmo/internal/ui/uistate"
)

// uistate_apply_integration_test.go closes the gap that Round 1 documented
// in MEMORY.md: uistate.Apply / ApplyFile sit at 0% because the unit tests
// inside the uistate package decouple via an `applier` interface. These
// tests drive the *real* *ui.Game so the contract "Game implements applier"
// is enforced at compile + behavior level. They also prove ApplyFile's
// JSON I/O end-to-end.
//
// Lives in package ui_test (external) to break the import cycle:
// uistate imports ui, so a uistate import inside package ui is forbidden.

// silentLogger discards UI log output so test runs stay quiet.
var silentLogger = game_log.New(io.Discard, game_log.LevelError)

// newTestGame mirrors the convention from the in-package tests but works
// from the external package. It still has to call out to the real audio
// reset helpers since import-time wiring registers a default catalog.
func newTestGame(t *testing.T) *ui.Game {
	t.Helper()
	ui.SetDefaultStartForTest(false)
	audio.ResetInstruments()
	audio.ResetCatalogForTest(nil)
	g := ui.New(silentLogger)
	t.Cleanup(func() {
		g.CloseForTest()
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
		ui.SetDefaultStartForTest(true) // restore default
	})
	return g
}

func writeUistateJSON(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "ui-state.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

func TestUistateApplyFileFullConfigDrivesGame(t *testing.T) {
	g := newTestGame(t)
	g.Layout(1280, 720)

	// Note: sidebar.open_node_id is intentionally omitted here — opening
	// the sidebar from an external test would require a way to seed a
	// node from the public API. The (camera, splitter, view, profile)
	// path covers every other applier method.
	body := `{
		"profile": "mobile",
		"force_auto_size": true,
		"camera": {"x": 75, "y": 40, "scale": 1.5},
		"splitter": {"frac": 0.4},
		"view": {"mode": "audio", "mobile_eq_collapsed": true}
	}`
	path := writeUistateJSON(t, body)

	if err := uistate.ApplyFile(g, path); err != nil {
		t.Fatalf("ApplyFile: %v", err)
	}

	if x, y := g.CameraOffsets(); x != 75 || y != 40 {
		t.Errorf("camera offsets: x=%v y=%v want 75,40", x, y)
	}
	if s := g.CameraScale(); s != 1.5 {
		t.Errorf("camera scale=%v want 1.5", s)
	}
	// Sidebar should remain closed since open_node_id was omitted.
	if g.SidebarOpen() {
		t.Errorf("sidebar opened despite missing open_node_id")
	}
}

func TestUistateApplyFileMissingFile(t *testing.T) {
	g := newTestGame(t)
	g.Layout(640, 480)
	if err := uistate.ApplyFile(g, "/nonexistent/__no_such_path__/ui-state.json"); err == nil {
		t.Fatalf("want error for missing file")
	}
}

func TestUistateApplyFileMalformedJSON(t *testing.T) {
	g := newTestGame(t)
	g.Layout(640, 480)
	path := writeUistateJSON(t, "{ this is not valid JSON }")
	if err := uistate.ApplyFile(g, path); err == nil {
		t.Fatalf("want error for malformed JSON")
	}
}

func TestUistateApplyEmptyConfigIsNoOp(t *testing.T) {
	g := newTestGame(t)
	g.Layout(640, 480)

	prevX, prevY := g.CameraOffsets()
	prevScale := g.CameraScale()

	if err := uistate.Apply(g, uistate.Config{}); err != nil {
		t.Fatalf("Apply(empty): %v", err)
	}

	if x, y := g.CameraOffsets(); x != prevX || y != prevY {
		t.Errorf("empty Config moved camera: now=(%v,%v) was=(%v,%v)", x, y, prevX, prevY)
	}
	if g.CameraScale() != prevScale {
		t.Errorf("empty Config changed scale")
	}
}
