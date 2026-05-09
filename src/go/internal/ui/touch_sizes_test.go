//go:build test

package ui

import (
	"image"
	"testing"
)

func TestTouchSizesDesktop(t *testing.T) {
	if SplitterHandleLen() != 50 {
		t.Errorf("SplitterHandleLen() = %d, want 50", SplitterHandleLen())
	}
	if SplitterHandleThick() != 8 {
		t.Errorf("SplitterHandleThick() = %d, want 8", SplitterHandleThick())
	}
	if TouchGrabZone() != 5 {
		t.Errorf("TouchGrabZone() = %d, want 5", TouchGrabZone())
	}
	if TouchRowHeight() != 28 {
		t.Errorf("TouchRowHeight() = %d, want 28", TouchRowHeight())
	}
	if TouchMinTarget() != 0 {
		t.Errorf("TouchMinTarget() = %d, want 0", TouchMinTarget())
	}
	if TransportBtnSize() != 32 {
		t.Errorf("TransportBtnSize() = %d, want 32", TransportBtnSize())
	}
	if RowControlBtnSize() != 0 {
		t.Errorf("RowControlBtnSize() = %d, want 0", RowControlBtnSize())
	}
	if MinCellWidth() != 2 {
		t.Errorf("MinCellWidth() = %d, want 2", MinCellWidth())
	}
}

func TestTouchSizesMobile(t *testing.T) {
	withSmallScreen(t, true)

	if SplitterHandleLen() != 56 {
		t.Errorf("SplitterHandleLen() = %d, want 56", SplitterHandleLen())
	}
	if SplitterHandleThick() != 6 {
		t.Errorf("SplitterHandleThick() = %d, want 6", SplitterHandleThick())
	}
	if TouchGrabZone() != 16 {
		t.Errorf("TouchGrabZone() = %d, want 16", TouchGrabZone())
	}
	if TouchRowHeight() != 44 {
		t.Errorf("TouchRowHeight() = %d, want 44", TouchRowHeight())
	}
	if TouchMinTarget() != 44 {
		t.Errorf("TouchMinTarget() = %d, want 44", TouchMinTarget())
	}
	if TransportBtnSize() != 44 {
		t.Errorf("TransportBtnSize() = %d, want 44", TransportBtnSize())
	}
	if RowControlBtnSize() != 32 {
		t.Errorf("RowControlBtnSize() = %d, want 32", RowControlBtnSize())
	}
	if MinCellWidth() != 2 {
		t.Errorf("MinCellWidth() = %d, want 2", MinCellWidth())
	}
}

func TestExpandHitAreaDesktop(t *testing.T) {
	if got := ExpandHitArea(20); got != 20 {
		t.Errorf("ExpandHitArea(20) = %d, want 20", got)
	}
}

func TestExpandHitAreaMobile(t *testing.T) {
	withSmallScreen(t, true)

	if got := ExpandHitArea(20); got != 44 {
		t.Errorf("ExpandHitArea(20) = %d, want 44 (expanded to minimum)", got)
	}
	if got := ExpandHitArea(50); got != 50 {
		t.Errorf("ExpandHitArea(50) = %d, want 50 (already large enough)", got)
	}
	if got := ExpandHitArea(44); got != 44 {
		t.Errorf("ExpandHitArea(44) = %d, want 44 (exactly at minimum)", got)
	}
}

func TestPopupSizingDesktop(t *testing.T) {
	if PopupPanelW() <= 0 {
		t.Errorf("PopupPanelW() = %d, want > 0", PopupPanelW())
	}
	if PopupBtnW() <= 0 {
		t.Errorf("PopupBtnW() = %d, want > 0", PopupBtnW())
	}
	if PopupBtnH() <= 0 {
		t.Errorf("PopupBtnH() = %d, want > 0", PopupBtnH())
	}
	if PopupGap() <= 0 {
		t.Errorf("PopupGap() = %d, want > 0", PopupGap())
	}
	if PopupPad() <= 0 {
		t.Errorf("PopupPad() = %d, want > 0", PopupPad())
	}
	if PopupTextScale() <= 0 {
		t.Errorf("PopupTextScale() = %f, want > 0", PopupTextScale())
	}
	if PopupLabelScale() <= 0 {
		t.Errorf("PopupLabelScale() = %f, want > 0", PopupLabelScale())
	}
	if PopupValueScale() <= 0 {
		t.Errorf("PopupValueScale() = %f, want > 0", PopupValueScale())
	}
	if PopupTitleScale() <= 0 {
		t.Errorf("PopupTitleScale() = %f, want > 0", PopupTitleScale())
	}
	if PopupSectionGap() <= 0 {
		t.Errorf("PopupSectionGap() = %d, want > 0", PopupSectionGap())
	}
	if PopupRowH() <= 0 {
		t.Errorf("PopupRowH() = %d, want > 0", PopupRowH())
	}
}

func TestPopupSizingMobile(t *testing.T) {
	withSmallScreen(t, true)

	if PopupPanelW() <= 0 {
		t.Errorf("PopupPanelW() = %d, want > 0", PopupPanelW())
	}
	if PopupBtnW() <= 0 {
		t.Errorf("PopupBtnW() = %d, want > 0", PopupBtnW())
	}
	if PopupBtnH() <= 0 {
		t.Errorf("PopupBtnH() = %d, want > 0", PopupBtnH())
	}
	if PopupGap() <= 0 {
		t.Errorf("PopupGap() = %d, want > 0", PopupGap())
	}
	if PopupPad() <= 0 {
		t.Errorf("PopupPad() = %d, want > 0", PopupPad())
	}
	if PopupTextScale() <= 1.0 {
		t.Errorf("PopupTextScale() = %f, want > 1.0 on mobile", PopupTextScale())
	}
	if PopupLabelScale() <= 1.0 {
		t.Errorf("PopupLabelScale() = %f, want > 1.0 on mobile", PopupLabelScale())
	}
	if PopupValueScale() <= 1.0 {
		t.Errorf("PopupValueScale() = %f, want > 1.0 on mobile", PopupValueScale())
	}
	if PopupTitleScale() <= 1.0 {
		t.Errorf("PopupTitleScale() = %f, want > 1.0 on mobile", PopupTitleScale())
	}
	if PopupSectionGap() <= 0 {
		t.Errorf("PopupSectionGap() = %d, want > 0", PopupSectionGap())
	}
	if PopupRowH() <= 0 {
		t.Errorf("PopupRowH() = %d, want > 0", PopupRowH())
	}
}

// TestMobileTransportButtons_AtTouchMin is a 44 px touch-target ratchet
// for every transport button placed by the mobile layout. It iterates
// every button rect produced by `layoutMobile()` and asserts each one
// satisfies the DESIGN.md touch-target floor. New buttons added to the
// mobile transport must clear this bar; existing exceptions (notably
// the BPM ± stepper, which currently inherits the 2-row toolbar's 28 px
// row height pending B3 single-row collapse) live in `knownBelowMin`
// so the regression guard surfaces *new* violations rather than stale
// known ones. Remove an entry from `knownBelowMin` once its layout is
// fixed — the test will then enforce the 44 px floor for that button.
func TestMobileTransportButtons_AtTouchMin(t *testing.T) {
	withSmallScreen(t, true)
	logger := testLogger
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}

	min := TouchMinTarget()
	type btnCheck struct {
		name string
		rect func() image.Rectangle
	}
	all := []btnCheck{
		{"playBtn", func() image.Rectangle { return tz.playBtn.Rect() }},
		{"stopBtn", func() image.Rectangle { return tz.stopBtn.Rect() }},
		{"recordBtn", func() image.Rectangle { return tz.recordBtn.Rect() }},
		{"subdivBtn", func() image.Rectangle { return tz.subdivBtn.Rect() }},
		{"viewSwitchBtn", func() image.Rectangle {
			if tz.viewSwitchBtn == nil {
				return image.Rectangle{}
			}
			return tz.viewSwitchBtn.Rect()
		}},
		{"overflowBtn", func() image.Rectangle {
			if tz.overflowBtn == nil {
				return image.Rectangle{}
			}
			return tz.overflowBtn.Rect()
		}},
		{"mainVolIcon", func() image.Rectangle { return tz.mainVolIconRect }},
		{"bpmIncBtn", func() image.Rectangle { return tz.bpmIncBtn.Rect() }},
		{"bpmDecBtn", func() image.Rectangle { return tz.bpmDecBtn.Rect() }},
	}
	// Buttons still below the 44 px floor.
	// After B3 (single-row toolbar + bottom action bar), play/stop/subdiv
	// and the BPM stepper now occupy the full mobile header row (≥ 56 px)
	// and vol-icon / view-switch / overflow live in the 44 px bottom action
	// bar — all hit the floor. recordBtn remains intentionally demoted on
	// mobile (B12 critique: shrink-by-recordDemoteInsetMobile so the red
	// dot doesn't sit at equal visual weight with play/stop and invite
	// accidental record mid-jam). Hit-test still spans the full cell via
	// ExpandHitArea.
	knownBelowMin := map[string]bool{
		"recordBtn": true,
	}
	for _, b := range all {
		r := b.rect()
		if r.Empty() {
			continue
		}
		if h := r.Dy(); h < min {
			if !knownBelowMin[b.name] {
				t.Errorf("%s height %d px < TouchMinTarget %d px (DESIGN.md §Touch sizing)", b.name, h, min)
			}
			continue
		}
		// Button now meets the floor — must be removed from the allow-list.
		if knownBelowMin[b.name] {
			t.Errorf("%s height %d px >= %d, but is still listed in knownBelowMin — remove the entry", b.name, r.Dy(), min)
		}
	}
}

func TestDesignTokenConstants(t *testing.T) {
	// Spacing scale bumped ~25% for the cushioned-dark theme — see
	// DESIGN.md "Spacing scale" prose.
	if SpaceXS != 3 {
		t.Errorf("SpaceXS = %d, want 3", SpaceXS)
	}
	if SpaceSM != 6 {
		t.Errorf("SpaceSM = %d, want 6", SpaceSM)
	}
	if SpaceMD != 10 {
		t.Errorf("SpaceMD = %d, want 10", SpaceMD)
	}
	if SpaceLG != 14 {
		t.Errorf("SpaceLG = %d, want 14", SpaceLG)
	}
	if SpaceXL != 20 {
		t.Errorf("SpaceXL = %d, want 20", SpaceXL)
	}
	if SpaceXXL != 28 {
		t.Errorf("SpaceXXL = %d, want 28", SpaceXXL)
	}
	if BtnHeightSM != 28 {
		t.Errorf("BtnHeightSM = %d, want 28", BtnHeightSM)
	}
	if BtnHeightMD != 36 {
		t.Errorf("BtnHeightMD = %d, want 36", BtnHeightMD)
	}
	if BtnHeightLG != 44 {
		t.Errorf("BtnHeightLG = %d, want 44", BtnHeightLG)
	}
}
