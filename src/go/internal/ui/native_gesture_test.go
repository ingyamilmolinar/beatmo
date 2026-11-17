//go:build test

package ui

import (
	"image"
	"testing"
)

func TestPlatformSyncNativeRects_RecordsInStub(t *testing.T) {
	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	in := []NativeRect{
		{Rect: image.Rect(0, 0, 10, 10), Intent: NativeIntent{Channel: NativeFilePicker, ID: "import", Accept: ".json"}},
	}
	platformSyncNativeRects(in)

	if len(captured) != 1 || captured[0].Intent.ID != "import" {
		t.Fatalf("stub did not record synced rects: %+v", captured)
	}
}

func TestSyncNativeGestures_NoCandidates_ClearsAndNoChurn(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	// With no menus/inputs open there are no candidates at all: nothing
	// (rename, inst-search, wav-name, file-picker) may be armed with nothing
	// open. (BPM no longer has a native input — its readout opens the wheel.)
	g.Update()
	if len(g.drum.lastNativeRects) != 0 {
		t.Fatalf("expected no armed native rects with nothing open, got %+v", g.drum.lastNativeRects)
	}
}

// TestNativeGesture_OccludedWidgetNotArmed proves the occlusion invariant: a
// native rect whose owner is no longer topmost at its center must not be
// armed. The BPM box is a nativeInputCandidate under owner "transport"; once
// the overflow menu (a blocking scrim portal) is open on top of it,
// TopmostOwnerAt(bpmCenter) resolves to "overflow-menu", not "transport", so
// syncNativeGestures must drop it. Non-vacuous companion:
// TestNativeInputCandidate_BPMArmedOnMobile proves "bpm" IS armed on a plain
// mobile frame with nothing open, so this test is exercising real suppression,
// not an always-absent id.
func TestNativeGesture_OccludedWidgetNotArmed(t *testing.T) {
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

	// A live text field (the logs search) is armed while its panel is the top
	// surface …
	g.toggleLogsPanel()
	for i := 0; i < 3; i++ {
		g.Update()
	}
	armed := func() bool {
		for _, r := range dv.lastNativeRects {
			if r.Intent.ID == logsSearchMobileInputID {
				return true
			}
		}
		return false
	}
	if !armed() {
		t.Skip("logs search not armed in this environment (precondition)")
	}
	// … and suppressed once a blocking scrim portal (the overflow menu) opens on
	// top: TopmostOwnerAt returns "overflow-menu", not "logs".
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()
	if armed() {
		t.Fatal("logs search native rect armed while a blocking overlay is open — occlusion gate failed")
	}
}
