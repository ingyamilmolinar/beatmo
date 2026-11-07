//go:build test

package ui

import (
	"image"
	"testing"

	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Found by identity, never as the last button appended: an index lookup drifts onto any trailing button
// added later.
func TestContextMenuCloseBtnFoundByIdentity(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	// The close × exists only on desktop; mobile dismisses by tap-away.
	withSmallScreen(t, false)

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

// Found by identity, not by alignment with a parallel icon slice: a desync once made "Color" open rename.
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

	// The button slice drifting out of lockstep with the icon metadata.
	dummy := NewButton("", PopupButtonStyle, nil)
	dv.contextMenuBtns = append([]*Button{dummy}, dv.contextMenuBtns...)

	if got := dv.contextMenuRenameBtn(); got != renameBefore {
		t.Errorf("contextMenuRenameBtn() drifted after button insertion (got %p, want %p) — icon association is index-coupled, not identity-coupled", got, renameBefore)
	}
}

// The same identity contract for the FX panel's close button.
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
