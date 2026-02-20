//go:build test

package ui

import "testing"

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
	if TransportBtnSize() != 0 {
		t.Errorf("TransportBtnSize() = %d, want 0", TransportBtnSize())
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
	if RowControlBtnSize() != 36 {
		t.Errorf("RowControlBtnSize() = %d, want 36", RowControlBtnSize())
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

func TestDesignTokenConstants(t *testing.T) {
	if SpaceXS != 2 {
		t.Errorf("SpaceXS = %d, want 2", SpaceXS)
	}
	if SpaceSM != 4 {
		t.Errorf("SpaceSM = %d, want 4", SpaceSM)
	}
	if SpaceMD != 8 {
		t.Errorf("SpaceMD = %d, want 8", SpaceMD)
	}
	if SpaceLG != 12 {
		t.Errorf("SpaceLG = %d, want 12", SpaceLG)
	}
	if SpaceXL != 16 {
		t.Errorf("SpaceXL = %d, want 16", SpaceXL)
	}
	if SpaceXXL != 24 {
		t.Errorf("SpaceXXL = %d, want 24", SpaceXXL)
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
