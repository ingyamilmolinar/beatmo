//go:build test

package ui

import "testing"

// TestDesktopRowHeight_AtLeastBtnMd pins the desktop instrument-row height
// at btn-md (36 px) or larger. Earlier value (28 px) made the per-row
// buttons feel cramped — speaker / mute / solo / fx glyphs only got ~18 px
// of icon space (rect * 18% pad). With 36 px rows they get ~24 px, matching
// the rest of the desktop chrome.
func TestDesktopRowHeight_AtLeastBtnMd(t *testing.T) {
	if got, want := genDesktopProfile.RowHeight, 36; got < want {
		t.Errorf("desktop RowHeight = %d, want >= %d (instrument-row buttons must use btn-md vertical rhythm)", got, want)
	}
}

// TestDesktopRowControlBtnSize_HasFloor pins a non-zero minimum width for
// row-control cells on desktop so a narrow window cannot shrink mute/solo/
// fx/volume below a comfortable mouse target. Mobile already has a 32 px
// floor (touch); desktop's floor is btn-sm (28 px).
func TestDesktopRowControlBtnSize_HasFloor(t *testing.T) {
	if got, want := genDesktopProfile.RowControlBtnSize, 28; got < want {
		t.Errorf("desktop RowControlBtnSize = %d, want >= %d (button cells must not shrink below btn-sm)", got, want)
	}
}

// TestDesktopRowHeightConstantTracksProfile guards against drift between
// the design_profile.gen.go value and the desktopRowHeightPx compile-time
// constant relied on by font_size_test.go.
func TestDesktopRowHeightConstantTracksProfile(t *testing.T) {
	if desktopRowHeightPx != genDesktopProfile.RowHeight {
		t.Errorf("desktopRowHeightPx (%d) != genDesktopProfile.RowHeight (%d) — keep both in sync",
			desktopRowHeightPx, genDesktopProfile.RowHeight)
	}
}
