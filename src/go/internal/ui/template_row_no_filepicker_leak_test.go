//go:build test

package ui

import (
	"testing"
)

// Reproduces the reported WASM-mobile bug: on the overflow menu's Templates
// page, a stale File-page file-picker rect used to sit under a template row, so
// tapping a template row opened the OS file picker. With the tree-owned
// projection, no file-picker rect may be armed while the Templates page is
// showing; the File page still arms Upload/Import.
func TestOverflowTemplatePage_NoFilePickerRectArmed(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)
	dv := g.drum

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	// Open menu on the File page → Upload+Import file-picker rects armed.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()
	fileArmed := 0
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			fileArmed++
		}
	}
	if fileArmed == 0 {
		t.Fatal("File page armed no file-picker rects — precondition failed")
	}

	// Switch to Templates page. No file-picker rect may remain armed.
	dv.overflowPage = 1
	g.Update()
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			t.Fatalf("file-picker rect %q armed on Templates page (id=%q rect=%v) — leaks onto template rows",
				r.Intent.ID, r.Intent.ID, r.Rect)
		}
	}

	// Switch back to the File page. Upload+Import file-picker rects must
	// return — the Templates-page clear must not be sticky.
	dv.overflowPage = 0
	g.Update()
	fileArmed = 0
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			fileArmed++
		}
	}
	if fileArmed == 0 {
		t.Fatal("switching back to File page did not re-arm file-picker rects — recovery leg failed")
	}
}
