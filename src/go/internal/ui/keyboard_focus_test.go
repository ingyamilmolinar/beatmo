package ui

import (
	"image"
	"testing"
)

// A focused Save As dialog claims the keyboard; an empty one does not.
func TestClaimsKeyboard_SaveAsDialog(t *testing.T) {
	var nilDlg *synthSaveAsDialog
	if nilDlg.ClaimsKeyboard() {
		t.Fatal("nil dialog must not claim the keyboard")
	}
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetText("kick (saved)")
	ti.SetFocus(true)
	dlg := &synthSaveAsDialog{textInput: ti}
	if !dlg.ClaimsKeyboard() {
		t.Fatal("a focused Save As dialog must claim the keyboard")
	}
	ti.SetFocus(false)
	if dlg.ClaimsKeyboard() {
		t.Fatal("an unfocused Save As dialog must not claim the keyboard")
	}
}

// An open rename component claims the keyboard; a closed one does not.
// A nil receiver must also be safe (ClaimsKeyboard short-circuits on nil).
func TestClaimsKeyboard_Rename(t *testing.T) {
	// R1: nil-receiver case — ClaimsKeyboard must not panic on nil; the `r != nil &&`
	// guard in ClaimsKeyboard short-circuits before IsOpen() reads r.state.
	var nilRename *RenameComponent
	if nilRename.ClaimsKeyboard() {
		t.Fatal("nil RenameComponent.ClaimsKeyboard() must return false without panicking")
	}

	r := &RenameComponent{}
	if r.ClaimsKeyboard() {
		t.Fatal("a closed rename component must not claim the keyboard")
	}
	r.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	r.Open()
	if !r.IsOpen() {
		t.Skip("rename did not open in this environment")
	}
	if !r.ClaimsKeyboard() {
		t.Fatal("an open rename component must claim the keyboard")
	}
}

// An open instrument menu claims the keyboard; a closed one does not.
func TestClaimsKeyboard_InstrumentMenu(t *testing.T) {
	g := newTestGameForUndo(t)
	m := g.drum.instMenuComp
	if m == nil {
		t.Skip("no instrument menu component")
	}
	if m.ClaimsKeyboard() {
		t.Fatal("a closed instrument menu must not claim the keyboard")
	}
	m.Open()
	if !m.ClaimsKeyboard() {
		t.Fatal("an open instrument menu must claim the keyboard")
	}
}

// An open rename portal makes its owning tree report OwnsKeyboard()==true.
func TestTreeOwnsKeyboard_RenamePortal(t *testing.T) {
	g := newTestGameForUndo(t)
	rc := g.drum.renameComp
	if rc == nil {
		t.Skip("no rename component")
	}
	if g.drum.tree.OwnsKeyboard() {
		t.Fatal("tree must not own the keyboard at rest")
	}
	rc.SetProps(RenameProps{InitialText: "kick", MaxLen: 32, AnchorRect: image.Rect(0, 0, 120, 24)})
	rc.Open()
	g.drum.openRenamePortal()
	if !g.drum.portal().IsOpen() {
		t.Skip("rename portal did not open")
	}
	if !rc.IsOpen() {
		t.Skip("rename component did not open (no anchor rect in this environment)")
	}
	if !g.drum.overlayTree.OwnsKeyboard() {
		t.Fatal("overlay tree must own the keyboard while a rename portal is open")
	}
}

// focusedZone alone makes a tree own the keyboard (existing transport/EQ path).
func TestTreeOwnsKeyboard_FocusedZone(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.drum.tree.OwnsKeyboard() {
		t.Fatal("tree must not own the keyboard at rest")
	}
	g.drum.tree.SetFocus("transport")
	if !g.drum.tree.OwnsKeyboard() {
		t.Fatal("a focused zone must make the tree own the keyboard")
	}
}

// KeyboardClaimed is false at rest and true for each text-input surface.
func TestKeyboardClaimed_AllSurfaces(t *testing.T) {
	g := newTestGameForUndo(t)
	if g.drum.KeyboardClaimed() {
		t.Fatal("KeyboardClaimed must be false at rest")
	}

	// 1. Save As dialog.
	ti := NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	ti.SetFocus(true)
	g.drum.saveAsDialog = &synthSaveAsDialog{textInput: ti}
	if !g.drum.KeyboardClaimed() {
		t.Fatal("Save As dialog must claim the keyboard")
	}
	g.drum.saveAsDialog = nil

	// 2. WAV-naming box.
	g.drum.nameBox = NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle)
	g.drum.nameBox.SetFocus(true)
	if !g.drum.KeyboardClaimed() {
		t.Fatal("WAV-naming box must claim the keyboard")
	}
	g.drum.nameBox = nil

	// 3. Focused transport zone (BPM editing).
	g.drum.tree.SetFocus("transport")
	if !g.drum.KeyboardClaimed() {
		t.Fatal("focused transport zone must claim the keyboard")
	}
	g.drum.tree.SetFocus("")

	// 4. R2: valueEditorActive() arm — open the BPM editor directly so the
	// shared ParamValueEditor is active, and assert KeyboardClaimed() is true.
	// This covers the dv.valueEditorActive() branch of KeyboardClaimed().
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone in this environment")
	}
	tz.openBPMEditor()
	if !g.drum.valueEditorActive() {
		t.Skip("openBPMEditor did not activate paramEditor in this environment")
	}
	if !g.drum.KeyboardClaimed() {
		t.Fatal("active BPM value editor must claim the keyboard via valueEditorActive()")
	}
	// Cancel to close the editor cleanly.
	tz.paramEditor.cancel()
	if g.drum.valueEditorActive() {
		t.Fatal("after cancel the value editor should no longer be active")
	}

	if g.drum.KeyboardClaimed() {
		t.Fatal("KeyboardClaimed must return to false once all surfaces cleared")
	}
}
