//go:build test

package ui

import "testing"

// At least btn-md (36 px), so the row glyphs get ~24 px of icon space.
func TestDesktopRowHeight_AtLeastBtnMd(t *testing.T) {
	if got, want := genDesktopProfile.RowHeight, 36; got < want {
		t.Errorf("desktop RowHeight = %d, want >= %d (instrument-row buttons must use btn-md vertical rhythm)", got, want)
	}
}

// A btn-sm (28 px) floor, so a narrow window keeps a comfortable mouse target.
func TestDesktopRowControlBtnSize_HasFloor(t *testing.T) {
	if got, want := genDesktopProfile.RowControlBtnSize, 28; got < want {
		t.Errorf("desktop RowControlBtnSize = %d, want >= %d (button cells must not shrink below btn-sm)", got, want)
	}
}

// font_size_test.go relies on the constant.
func TestDesktopRowHeightConstantTracksProfile(t *testing.T) {
	if desktopRowHeightPx != genDesktopProfile.RowHeight {
		t.Errorf("desktopRowHeightPx (%d) != genDesktopProfile.RowHeight (%d) — keep both in sync",
			desktopRowHeightPx, genDesktopProfile.RowHeight)
	}
}
