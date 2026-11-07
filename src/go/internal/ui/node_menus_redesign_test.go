package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Through the styled text role (embedded Inter): the bitmap sprite upscale pixelated every caption.
func TestGroupMenuButtonsUseStyledTextRole(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	drawGroupMenuFrame(g)

	for id, b := range g.groupMenu.btns {
		if b == nil || b.Icon != "" {
			continue // icon-only buttons don't render text
		}
		if _, ok := b.StyledTextRole(); !ok {
			t.Errorf("group-menu button %q has no TextRole (legacy pixelated text path)", id)
		}
		if b.TextScale != 0 {
			t.Errorf("group-menu button %q still sets TextScale=%v", id, b.TextScale)
		}
	}
}

// Plain DropdownStyle face: the group's identity lives in the title swatch and underline; the armed state is
// the gold label.
func TestGroupMenuSaveFaceNeutral(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	drawGroupMenuFrame(g)

	b := g.groupMenu.btns[gmSave]
	if b == nil {
		t.Fatal("save button missing")
	}
	bs, ok := b.Style.(ButtonStyle)
	if !ok {
		t.Fatalf("save style is %T, want ButtonStyle", b.Style)
	}
	want := color.RGBAModel.Convert(DropdownStyle.Fill).(color.RGBA)
	if got := color.RGBAModel.Convert(bs.Fill).(color.RGBA); got != want {
		t.Errorf("save face = %v, want neutral DropdownStyle fill %v", got, want)
	}
	if b.TextColor == nil {
		t.Error("save label should carry the accent TextColor (primary-action marker)")
	}
}

// wireButtons sets TextRole, never TextScale.
func TestSidebarButtonsUseStyledTextRole(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 1000)
	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	for id, b := range g.sidebar.btns {
		if b == nil || b.Icon != "" {
			continue // icon-only buttons (steppers, close) don't render text
		}
		if _, ok := b.StyledTextRole(); !ok {
			t.Errorf("sidebar button %q has no TextRole (legacy pixelated text path)", id)
		}
		if b.TextScale != 0 {
			t.Errorf("sidebar button %q still sets TextScale=%v", id, b.TextScale)
		}
	}
}

// Options start sidebarOptionIndent deeper than their trigger, so an open dropdown reads as a submenu.
func TestSidebarDropdownOptionsIndented(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 1000)
	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	trigger, ok := g.sidebar.rects[sbLogicSelect]
	if !ok || trigger.Empty() {
		t.Fatal("logic trigger rect missing")
	}
	opt, ok := g.sidebar.rects[sbLogicOption(model.LogicProbability)]
	if !ok || opt.Empty() {
		t.Fatal("logic option rect missing while dropdown open")
	}
	if got := opt.Min.X - trigger.Min.X; got != sidebarOptionIndent {
		t.Errorf("option indent = %d, want sidebarOptionIndent (%d)", got, sidebarOptionIndent)
	}
}
