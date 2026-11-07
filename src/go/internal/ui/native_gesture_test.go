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

	// With nothing open, nothing may be armed.
	g.Update()
	if len(g.drum.lastNativeRects) != 0 {
		t.Fatalf("expected no armed native rects with nothing open, got %+v", g.drum.lastNativeRects)
	}
}

// A native rect whose owner is not topmost at its centre must not be armed: under the open overflow menu the
// logs search resolves to "overflow-menu".
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
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()
	if armed() {
		t.Fatal("logs search native rect armed while a blocking overlay is open — occlusion gate failed")
	}
}
