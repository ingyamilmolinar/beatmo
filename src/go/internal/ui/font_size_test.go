package ui

import "testing"

// Cap height is ~70% of the font size, so FontSizeBody ≥ 14 keeps TrueType text readable.
func TestFontSizeBodyAdequateForDesktopRow(t *testing.T) {
	const minFontSize float64 = 14
	if FontSizeBody < minFontSize {
		t.Fatalf("FontSizeBody=%.0f too small for readable TrueType text; "+
			"cap height ≈ %.0fpx within %dpx rows; need FontSizeBody >= %.0f",
			FontSizeBody, FontSizeBody*0.7, desktopRowHeightPx, minFontSize)
	}
}

func TestFontSizeBodyFitsDesktopRow(t *testing.T) {
	// ~1.2 × font size, with at least 2 px to spare.
	lineH := FontSizeBody * 1.2
	if int(lineH)+2 > desktopRowHeightPx {
		t.Fatalf("FontSizeBody=%.0f produces ~%.0fpx line height, "+
			"exceeds desktopRowHeightPx=%d with 2px padding",
			FontSizeBody, lineH, desktopRowHeightPx)
	}
}

// Ordered, with minimum separation between steps.
func TestFontSizeHierarchy(t *testing.T) {
	if FontSizeCaption >= FontSizeSmall {
		t.Fatalf("FontSizeCaption (%.0f) must be < FontSizeSmall (%.0f)", FontSizeCaption, FontSizeSmall)
	}
	if FontSizeSmall >= FontSizeBody {
		t.Fatalf("FontSizeSmall (%.0f) must be < FontSizeBody (%.0f)", FontSizeSmall, FontSizeBody)
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
