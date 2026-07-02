//go:build test

package ui

import (
	"image"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestContextMenuCloseBtnFoundByIdentity pins that the row context menu's close
// button is retrieved by IDENTITY, not as "whatever was appended last". The
// old `contextMenuBtns[len-1]` lookup silently drifts onto any trailing button
// added later (the same fragility class as the rename-trigger index bug). This
// test simulates a future change that appends a trailing non-close button and
// asserts the close button is still found.
func TestContextMenuCloseBtnFoundByIdentity(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)

	dv.OpenContextMenu(0)
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu should be open")
	}

	closeBtn := dv.contextMenuCloseBtn()
	if closeBtn == nil {
		t.Fatal("contextMenuCloseBtn() returned nil")
	}
	if closeBtn.Icon != "close" {
		t.Fatalf("expected close button (Icon==close), got Icon=%q", closeBtn.Icon)
	}

	// Simulate a future code change that appends a trailing (non-close) button.
	dummy := NewButton("", PopupButtonStyle, nil)
	dv.contextMenuBtns = append(dv.contextMenuBtns, dummy)

	if got := dv.contextMenuCloseBtn(); got != closeBtn {
		gotIcon := "<nil>"
		if got != nil {
			gotIcon = got.Icon
		}
		t.Errorf("contextMenuCloseBtn() drifted onto a trailing button (got Icon=%q), want the real close button", gotIcon)
	}
}

// TestContextMenuRenameBtnRobustToButtonDrift pins that the Rename item is
// found by button identity, not by index alignment between the button slice and
// a parallel icon slice. Inserting a button ahead of the others (simulating a
// future menu change / lockstep-append desync) must NOT cause a different button
// to be mistaken for Rename — the failure mode that let "Color" open rename.
func TestContextMenuRenameBtnRobustToButtonDrift(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 700), nil, logger)

	dv.OpenContextMenu(0)
	renameBefore := dv.contextMenuRenameBtn()
	if renameBefore == nil {
		t.Fatal("contextMenuRenameBtn() returned nil for open menu")
	}

	// Simulate the button slice drifting out of lockstep with icon metadata by
	// inserting an unrelated button at the front.
	dummy := NewButton("", PopupButtonStyle, nil)
	dv.contextMenuBtns = append([]*Button{dummy}, dv.contextMenuBtns...)

	if got := dv.contextMenuRenameBtn(); got != renameBefore {
		t.Errorf("contextMenuRenameBtn() drifted after button insertion (got %p, want %p) — icon association is index-coupled, not identity-coupled", got, renameBefore)
	}
}

// TestFXPanelCloseBtnFoundByIdentity pins the same identity contract for the
// per-row insert-effects panel's close button (previously read via
// fxPanelBtns[len-1] at multiple draw sites).
func TestFXPanelCloseBtnFoundByIdentity(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)

	dv.OpenFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel should be open")
	}

	closeBtn := dv.fxPanelCloseBtn()
	if closeBtn == nil {
		t.Fatal("fxPanelCloseBtn() returned nil")
	}
	if closeBtn.Icon != "close" {
		t.Fatalf("expected close button (Icon==close), got Icon=%q", closeBtn.Icon)
	}

	dummy := NewButton("", PopupButtonStyle, nil)
	dv.fxPanelBtns = append(dv.fxPanelBtns, dummy)

	if got := dv.fxPanelCloseBtn(); got != closeBtn {
		gotIcon := "<nil>"
		if got != nil {
			gotIcon = got.Icon
		}
		t.Errorf("fxPanelCloseBtn() drifted onto a trailing button (got Icon=%q), want the real close button", gotIcon)
	}
}
