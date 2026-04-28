package ui

import "fmt"

// drumview_open_dialogs.go centralizes public openers/closers for color,
// instrument, subdiv, rename, WAV-naming, and EQ-channel-dropdown overlays.
// Each delegates to existing private logic so the screenshot harness, scene
// catalog, and tests can drive them without simulating gestures.

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

// OpenSubdivMenu opens the subdivision/logic-kind menu. Mirrors the
// transport-button click path so JS exports and scene drivers leave the
// view in the same state as a real user click (component props set,
// portal open, legacy subdivMenuBtns populated for hit-test queries).
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

// CloseSubdivMenu closes the subdivision/logic-kind menu.
func (dv *DrumView) CloseSubdivMenu() { dv.closeSubdivMenuPortal() }

// CloseAllDialogs closes every popup/overlay/dialog. Convenience for
// scene cleanup between screenshots.
func (dv *DrumView) CloseAllDialogs() { dv.CloseAllPopups() }
