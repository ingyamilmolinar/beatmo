package ui

import (
	"fmt"
	"image"
)

// OpenColorMenu opens the color wheel picker for the given row.
func (dv *DrumView) OpenColorMenu(rowIdx int) { dv.openColorPickerForRow(rowIdx) }

// CloseColorMenu closes the color wheel picker if open.
func (dv *DrumView) CloseColorMenu() {
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.colorWheelComp.Close()
	}
}

// OpenInstrumentMenu opens the instrument selector menu for the given row.
func (dv *DrumView) OpenInstrumentMenu(rowIdx int) { dv.openInstMenuForRow(rowIdx) }

// CloseInstrumentMenu closes the instrument selector menu.
func (dv *DrumView) CloseInstrumentMenu() {
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		dv.instMenuComp.Close()
	}
}

// Mirrors the button click, so JS exports and scenes leave the same state as a real click.
func (dv *DrumView) OpenSubdivMenu() {
	dv.CloseAllPopups()
	if dv.subdivMenuComp != nil {
		dv.subdivMenuComp.SetProps(SubdivMenuProps{
			AnchorRect: dv.subdivBtn().Rect(),
			Current:    dv.timelineUnitsPerBeat,
			Options:    []int{4, 8, 16, 32},
			RowHeight:  dv.rowHeight(),
			OnSelect: func(value int) {
				if dv.onChangeSubdiv != nil {
					if err := dv.onChangeSubdiv(value); err != nil {
						return
					}
				}
				dv.subdivBtn().Text = fmt.Sprintf("÷%d", value)
				dv.timelineUnitsPerBeat = value
			},
			OnClose: func() {},
		})
		dv.subdivMenuComp.Open()
	}
	dv.openSubdivMenuPortal()
	if dv.IsSubdivMenuOpen() {
		dv.buildSubdivMenu()
	}
}

// The single entry point for the navigation panel's amount picker and the scene.
func (dv *DrumView) OpenJumpMenu() {
	if dv.jumpMenuComp == nil {
		return
	}
	// Stacks over the open nav-menu portal; closing and reopening it would orphan the nav menu's
	// captured gesture.
	anchor := image.Rectangle{}
	if dv.navMenuComp != nil {
		anchor = dv.navMenuComp.Bounds()
	}
	if dv.navPanel != nil {
		if r := dv.navPanel.AmountRect(); !r.Empty() {
			anchor = r
		}
	}
	dv.jumpMenuComp.SetProps(SubdivMenuProps{
		AnchorRect: anchor,
		Current:    dv.jumpBeats,
		Options:    []int{1, 2, 4, 8, 16},
		RowHeight:  dv.rowHeight(),
		OnSelect: func(value int) {
			// NavigationPanel.Layout reads it live.
			dv.jumpBeats = value
			dv.closeJumpMenuPortal()
		},
		OnClose: func() {},
	})
	dv.jumpMenuComp.Open()
	dv.openJumpMenuPortal()
}

func (dv *DrumView) OpenNavigation() {
	dv.openNavMenuPortal()
}

func (dv *DrumView) OpenFileMenu() {
	dv.overflowPage = 0
	dv.OpenOverflowMenu()
}

// CloseSubdivMenu closes the subdivision/logic-kind menu.
func (dv *DrumView) CloseSubdivMenu() { dv.closeSubdivMenuPortal() }

func (dv *DrumView) CloseAllDialogs() { dv.CloseAllPopups() }
