package ui

import (
	"encoding/json"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNodeMenuButtonsRespondOnPress verifies popup buttons trigger on tap
// (press+release) and are decoupled from direct state changes (processed via
// queue within Update()). With the deferred-tap pattern, buttons fire on
// release to prevent accidental triggers during touch scrolling.
func TestNodeMenuButtonsRespondOnPress(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Add a node and open its menu
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	r, ok := g.sidebar.rects["vol+"]
	if !ok {
		t.Fatalf("vol+ rect missing")
	}

	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	left := false
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(btn ebiten.MouseButton) bool { return left && btn == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	// Capture initial volume and tap (press frame + release frame).
	before := g.graph.Nodes[n.ID].Params.Volume
	left = true
	if err := g.Update(); err != nil {
		t.Fatalf("update press: %v", err)
	}
	left = false
	if err := g.Update(); err != nil {
		t.Fatalf("update release: %v", err)
	}
	after := g.graph.Nodes[n.ID].Params.Volume
	if !(after > before) {
		t.Fatalf("volume did not increase on tap: %f -> %f", before, after)
	}
}

// TestMuteSoloNoAutoRepeat ensures holding down the mouse does not auto-toggle
// mute/solo buttons repeatedly.
func TestMuteSoloNoAutoRepeat(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowMuteBtns()) == 0 {
		t.Fatalf("no mute button")
	}
	btn := dv.rowMuteBtns()[0]
	r := btn.Rect()
	mx := (r.Min.X + r.Max.X) / 2
	my := (r.Min.Y + r.Max.Y) / 2
	// Press and hold for many frames.
	toggles := 0
	base := dv.Rows[0].Muted
	prev := base
	for i := 0; i < 120; i++ {
		_ = btn.Handle(mx, my, true)
		if dv.Rows[0].Muted != prev {
			toggles++
			prev = dv.Rows[0].Muted
		}
	}
	// Release
	_ = btn.Handle(mx, my, false)
	if toggles != 1 {
		t.Fatalf("expected exactly 1 toggle, got %d", toggles)
	}
}

// TestExportImportNodeParams verifies node params and type survive export/import.
func TestExportImportNodeParams(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeSilent)
	g.addEdge(a, b)
	p := g.graph.Nodes[b.ID].Params
	p.Volume = 1.5
	p.Pitch = 2
	p.Duration = 1.25
	p.LogicKind = "skip_every_n"
	p.LogicN = 3
	g.graph.SetNodeParams(b.ID, p)
	// Row origin
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	// Export bytes
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	// Find the silent node and verify params serialized
	found := false
	for _, n := range f.Nodes {
		if n.I == 2 && n.J == 0 { // scaled 1:1
			if n.Type != "silent" {
				t.Fatalf("type=%s want silent", n.Type)
			}
			if n.Volume <= 0 || n.Pitch != 2 || n.Duration <= 1 || n.LogicKind != "skip_every_n" || n.LogicN != 3 {
				t.Fatalf("params not serialized: %+v", n)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("exported nodes did not include the silent node with params")
	}
	// Import back and verify graph values
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	have := false
	for _, n := range g.graph.Nodes {
		if n.I == 2 && n.J == 0 {
			if n.Type != model.NodeTypeSilent {
				t.Fatalf("import type wrong: %v", n.Type)
			}
			if n.Params.LogicKind != "skip_every_n" || n.Params.LogicN != 3 || n.Params.Pitch != 2 {
				t.Fatalf("import params wrong: %+v", n.Params)
			}
			have = true
		}
	}
	if !have {
		t.Fatalf("imported graph missing target node")
	}
}

func TestNodeMenuMuteCycle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	btn := g.sidebar.btns["aud"]
	if btn == nil {
		t.Fatalf("aud button missing")
	}

	// Cycle: regular -> silent
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if node, ok := g.graph.GetNodeByID(n.ID); !ok || node.Type != model.NodeTypeSilent {
		t.Fatalf("after first toggle want silent, got %+v", node)
	}

	g.sidebar.layout()
	if btn = g.sidebar.btns["aud"]; btn == nil {
		t.Fatalf("aud button missing after toggle")
	}
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if node, ok := g.graph.GetNodeByID(n.ID); !ok || node.Type != model.NodeTypeMute {
		t.Fatalf("after second toggle want mute, got %+v", node)
	}

	g.sidebar.layout()
	if btn = g.sidebar.btns["aud"]; btn == nil {
		t.Fatalf("aud button missing after mute")
	}
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if node, ok := g.graph.GetNodeByID(n.ID); !ok || node.Type != model.NodeTypeRegular {
		t.Fatalf("after third toggle want regular, got %+v", node)
	}
}

func TestNodeMenuMuteHidesVolumeAndPitch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	btn := g.sidebar.btns["aud"]
	if btn == nil {
		t.Fatalf("aud button missing")
	}
	// Toggle to mute (regular -> silent -> mute)
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	g.sidebar.layout()
	btn = g.sidebar.btns["aud"]
	if btn == nil {
		t.Fatalf("aud button missing after silent")
	}
	btn.OnClick()
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}

	node, ok := g.graph.GetNodeByID(n.ID)
	if !ok || node.Type != model.NodeTypeMute {
		t.Fatalf("node not mute after toggle: %+v", node)
	}

	g.sidebar.layout()

	if !g.sidebar.rects["vol-"].Empty() || !g.sidebar.rects["pit-"].Empty() {
		t.Fatalf("volume or pitch rect still visible for mute node: vol=%v pit=%v", g.sidebar.rects["vol-"], g.sidebar.rects["pit-"])
	}
	if _, ok := g.sidebar.btns["vol-"]; ok {
		t.Fatalf("volume button still registered for mute node")
	}
	if _, ok := g.sidebar.btns["pit-"]; ok {
		t.Fatalf("pitch button still registered for mute node")
	}
	if !g.sidebar.rects["dur-"].Empty() || !g.sidebar.rects["dur+"].Empty() {
		t.Fatalf("duration controls should be hidden for mute node")
	}
	if g.sidebar.rects["grv"].Empty() {
		t.Fatalf("groove controls missing for mute node")
	}
}

func TestNodeMenuMuteClickDoesNotTriggerLogic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 960)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	audRect := g.sidebar.rects["aud"]
	if audRect.Empty() {
		t.Fatalf("audible button rect missing")
	}
	mx := (audRect.Min.X + audRect.Max.X) / 2
	my := (audRect.Min.Y + audRect.Max.Y) / 2
	left := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(btn ebiten.MouseButton) bool {
			if btn != ebiten.MouseButtonLeft {
				return false
			}
			return left
		},
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	left = true
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	left = false
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	if mn, ok := g.graph.GetNodeByID(n.ID); !ok || mn.Type != model.NodeTypeSilent {
		t.Fatalf("node not silent after first toggle: %+v", mn)
	}
	g.sidebar.layout()
	audRect = g.sidebar.rects["aud"]
	if audRect.Empty() {
		t.Fatalf("audible rect missing after silent toggle")
	}
	mx = (audRect.Min.X + audRect.Max.X) / 2
	my = (audRect.Min.Y + audRect.Max.Y) / 2
	if !g.sidebar.rects["logic"].Empty() {
		t.Fatalf("logic should be hidden for silent node")
	}

	// Second tap: press+release to toggle Silent→Mute (deferred tap fires on release)
	left = true
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	left = false
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
	g.sidebar.layout()
	logicRect := g.sidebar.rects["logic"]
	if logicRect.Empty() {
		t.Fatalf("logic rect missing after mute toggle")
	}
	if !image.Pt(mx, my).In(logicRect) {
		t.Fatalf("cursor not over logic rect: cursor=(%d,%d) logic=%v", mx, my, logicRect)
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatalf("logic menu opened due to overlapping click")
	}
	left = false
	if err := g.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestExportImportMuteNode(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	src := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(2, 0, model.NodeTypeMute)
	g.addEdge(src, mute)
	g.drum.Rows[0].Origin = src.ID
	g.drum.Rows[0].Node = src
	p := g.graph.Nodes[mute.ID].Params
	p.Duration = 2.5
	g.graph.SetNodeParams(mute.ID, p)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	found := false
	for _, n := range f.Nodes {
		if n.Type == "mute" {
			if n.Duration != 2.5 {
				t.Fatalf("export duration mismatch: %.2f", n.Duration)
			}
			if n.Volume != 0 || n.Pitch != 0 {
				t.Fatalf("volume/pitch should be default for mute nodes: %+v", n)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("mute node missing from export")
	}

	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	have := false
	for _, node := range g.graph.Nodes {
		if node.Type == model.NodeTypeMute {
			if math.Abs(node.Params.Duration-2.5) > 1e-6 {
				t.Fatalf("import duration mismatch: %.2f", node.Params.Duration)
			}
			if node.Params.Volume != 1 {
				t.Fatalf("imported mute node volume changed: %.2f", node.Params.Volume)
			}
			have = true
		}
	}
	if !have {
		t.Fatalf("mute node missing after import")
	}
}
