package ui

import (
	"fmt"
	"image"
)

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

// OpenJumpMenu opens the FF/RW jump-size (×N) dropdown, mirroring
// OpenSubdivMenu: it seeds the menu component's props (beat-count options +
// OnSelect that updates the jump size and the pill label), opens the
// component, and shows it through the portal anchored on the overflow popup
// (the transport's dedicated ×N pill was removed — see NavigationPanel's
// OnPickAmount, the sole caller now that the transport bar no longer hosts a
// jump-size control directly). This is the single entry point used by the
// NAVIGATION panel's amount picker and the screenshot scene, so the open
// logic lives in one place.
func (dv *DrumView) OpenJumpMenu() {
	if dv.jumpMenuComp == nil {
		return
	}
	// The picker is a nested portal that simply STACKS over the already-open
	// nav-menu portal (portals z-order by stack position), so there is no
	// close/reopen dance — that would orphan the nav menu's in-flight captured
	// gesture. OnPickAmount fires this from inside the nav menu.
	// Anchor the picker on the NAVIGATION panel's ×N pill so it pops right at
	// the control, falling back to the nav-menu card before the panel is laid out.
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
			// dv.jumpBeats is read live by NavigationPanel.Layout (the ×N
			// label) each frame; no transport pill mirror needed anymore.
			dv.jumpBeats = value
			dv.closeJumpMenuPortal()
		},
		OnClose: func() {},
	})
	dv.jumpMenuComp.Open()
	dv.openJumpMenuPortal()
}

// navGotoEditor returns the navigation menu's OWNED go-to numeric editor (a
// child of navMenuComp, driven/drawn/keyboard-claimed by that portal
// component). Nil only before construction.
func (dv *DrumView) navGotoEditor() *ParamValueEditor {
	if dv.navMenuComp == nil {
		return nil
	}
	return dv.navMenuComp.Editor()
}

// navGotoAnchor returns r (a NAVIGATION panel control's rect) as the anchor for
// the go-to editor, falling back to the nav-menu card rect when the panel
// hasn't been laid out yet (e.g. first frame after open) so r is empty.
func (dv *DrumView) navGotoAnchor(r image.Rectangle) image.Rectangle {
	if !r.Empty() {
		return r
	}
	if dv.navMenuComp != nil {
		return dv.navMenuComp.Bounds()
	}
	return dv.Bounds
}

// openGotoBeatEditor pops the nav menu's owned position editor over the
// NAVIGATION panel's "Go to beat…" button, pre-filled with the current playhead
// step. Commits via an absolute RequestSeekSteps.
func (dv *DrumView) openGotoBeatEditor() {
	ed := dv.navGotoEditor()
	if ed == nil {
		return
	}
	div := max1(dv.timelineUnitsPerBeat)
	maxSteps := dv.timelineBeats * div
	var anchor image.Rectangle
	if dv.navPanel != nil {
		anchor = dv.navPanel.GotoBeatRect()
	}
	ed.OpenValue(ValueOpen{
		Spec:   positionValueSpec(div, maxSteps),
		Anchor: dv.navGotoAnchor(anchor),
		Clamp:  dv.Bounds,
		Get: func() float64 {
			if dv.game != nil {
				return float64(dv.game.playheadAbsSubdiv())
			}
			return 0
		},
		Set: func(v float64) { dv.RequestSeekSteps(int(v), seekOriginCounter) },
	})
}

// openGotoTimeEditor pops the nav menu's owned position editor over the
// NAVIGATION panel's "Go to time…" button in M:SS form; commits via timeToSteps'
// closest-beat-at-or-after rule (nav_time.go).
func (dv *DrumView) openGotoTimeEditor() {
	ed := dv.navGotoEditor()
	if ed == nil {
		return
	}
	div := max1(dv.timelineUnitsPerBeat)
	bpm := dv.BPM()
	maxSteps := dv.timelineBeats * div
	var anchor image.Rectangle
	if dv.navPanel != nil {
		anchor = dv.navPanel.GotoTimeRect()
	}
	ed.OpenValue(ValueOpen{
		Spec: ValueSpec{
			MinW: 72, MaxLen: 6,
			Format: func(v float64) string { return formatClock(int(v)) },
			Parse:  func(s string) (float64, bool) { return parseClock(s) },
		},
		Anchor: dv.navGotoAnchor(anchor),
		Clamp:  dv.Bounds,
		Get: func() float64 {
			if dv.game == nil {
				return 0
			}
			steps := dv.game.playheadAbsSubdiv()
			return float64((steps / div) * 60 / max1(bpm))
		},
		Set: func(v float64) { dv.RequestSeekSteps(timeToSteps(v, bpm, div, maxSteps), seekOriginCounter) },
	})
}

// OpenNavigation opens the navigation menu as its OWN first-class portal
// component (navMenuComp), anchored under the beat counter. Entry point for the
// beat-counter tap. It is no longer a mode of the file/overflow portal.
func (dv *DrumView) OpenNavigation() {
	dv.openNavMenuPortal()
}

// OpenFileMenu opens the overflow (file) portal: the file actions (Upload /
// Import / Export / Load Template → template browser), pixel-anchored under the
// grid-pane folder button. Entry point for the grid folder button.
func (dv *DrumView) OpenFileMenu() {
	dv.overflowPage = 0
	dv.OpenOverflowMenu()
}

// CloseSubdivMenu closes the subdivision/logic-kind menu.
func (dv *DrumView) CloseSubdivMenu() { dv.closeSubdivMenuPortal() }

// CloseAllDialogs closes every popup/overlay/dialog. Convenience for
// scene cleanup between screenshots.
func (dv *DrumView) CloseAllDialogs() { dv.CloseAllPopups() }
