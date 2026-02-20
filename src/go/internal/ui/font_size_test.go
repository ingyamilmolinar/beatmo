package ui

import "testing"

// TestFontSizeBodyAdequateForDesktopRow verifies that FontSizeBody is large
// enough for TrueType text to be clearly readable within standard UI elements.
//
// TrueType fonts at a given "size" produce visually smaller glyphs than a
// bitmap debug font at the same pixel height because the em-square includes
// ascent + descent whitespace. The cap height (uppercase letter height) is
// approximately 70% of the font size. At FontSizeBody=16:
//
//	0.7 * 16 = 11.2px cap height
//
// We require FontSizeBody >= 14 as a practical minimum for readability.
func TestFontSizeBodyAdequateForDesktopRow(t *testing.T) {
	const minFontSize float64 = 14
	if FontSizeBody < minFontSize {
		t.Fatalf("FontSizeBody=%.0f too small for readable TrueType text; "+
			"cap height ≈ %.0fpx within %dpx rows; need FontSizeBody >= %.0f",
			FontSizeBody, FontSizeBody*0.7, desktopRowHeightPx, minFontSize)
	}
}

// TestFontSizeBodyFitsDesktopRow verifies that the body text line height
// (ascent + descent) fits within the desktop row height with room to spare.
func TestFontSizeBodyFitsDesktopRow(t *testing.T) {
	// TrueType line height ≈ 1.2 × font size (ascent + descent + leading).
	// Must fit within desktopRowHeightPx with at least 2px padding.
	lineH := FontSizeBody * 1.2
	if int(lineH)+2 > desktopRowHeightPx {
		t.Fatalf("FontSizeBody=%.0f produces ~%.0fpx line height, "+
			"exceeds desktopRowHeightPx=%d with 2px padding",
			FontSizeBody, lineH, desktopRowHeightPx)
	}
}

// TestFontSizeHierarchy verifies the type scale has proper ordering and
// minimum separation between sizes for visual hierarchy.
func TestFontSizeHierarchy(t *testing.T) {
	if FontSizeCaption >= FontSizeBody {
		t.Fatalf("FontSizeCaption (%.0f) must be < FontSizeBody (%.0f)", FontSizeCaption, FontSizeBody)
	}
	if FontSizeBody >= FontSizeLabel {
		t.Fatalf("FontSizeBody (%.0f) must be < FontSizeLabel (%.0f)", FontSizeBody, FontSizeLabel)
	}
	if FontSizeLabel >= FontSizeTitle {
		t.Fatalf("FontSizeLabel (%.0f) must be < FontSizeTitle (%.0f)", FontSizeLabel, FontSizeTitle)
	}
	if FontSizeTitle >= FontSizeHeading {
		t.Fatalf("FontSizeTitle (%.0f) must be < FontSizeHeading (%.0f)", FontSizeTitle, FontSizeHeading)
	}
}
