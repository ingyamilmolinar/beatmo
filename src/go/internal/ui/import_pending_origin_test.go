package ui

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Build a minimal export with two instruments and two nodes and verify that
// after import there is no pending origin selection and clicking a node does
// not reassign origins implicitly.
func TestImportClearsPendingOriginAndPopupClickDoesNotReassign(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Construct a simple exported file with two nodes and two instruments
	exp := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular"},
			{ID: 1, I: 3, J: 0, Type: "regular"},
		},
		Instruments: []exportInstrument{
			{Name: "Row0", ID: "snare", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
			{Name: "Row1", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"},
		},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Next Update should not arm origin selection
	_ = g.Update()
	if g.pendingStartRow != -1 {
		t.Fatalf("pendingStartRow=%d after import, want -1", g.pendingStartRow)
	}
	// Click first node to open its popup; origins should remain intact.
	n0 := g.nodeAt(0, 0)
	if n0 == nil {
		t.Fatalf("node not found")
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n0)
	sx, sy := int((x1+x2)/2), int((y1+y2)/2)
	// Press-release
	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
	restore = SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
	// Verify row origins unchanged
	if len(g.drum.Rows) < 2 {
		t.Fatalf("expected 2 rows")
	}
	if g.drum.Rows[0].Origin == g.drum.Rows[1].Origin {
		t.Fatalf("origins collapsed after click")
	}
}
