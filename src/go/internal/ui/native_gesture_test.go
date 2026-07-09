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

	// With no menus/inputs open there are no candidates EXCEPT the BPM native
	// input, which is armed unconditionally on mobile every layout pass (the JS
	// touchend gesture handler needs "bpm" already registered at touchend time —
	// see (*DrumView).nativeInputCandidates). Nothing else (rename, inst-search,
	// wav-name, file-picker) may be armed with nothing open.
	g.Update()
	for _, r := range g.drum.lastNativeRects {
		if r.Intent.Channel == NativeTextInput && r.Intent.ID == "bpm" {
			continue
		}
		t.Fatalf("expected no armed native rects with nothing open except bpm, got %+v", g.drum.lastNativeRects)
	}
}

func TestNativeInputCandidate_BPMArmedOnMobile(t *testing.T) {
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

	g.Update()
	found := false
	for _, r := range g.drum.lastNativeRects {
		if r.Intent.Channel == NativeTextInput && r.Intent.ID == "bpm" {
			found = true
		}
	}
	if !found {
		t.Fatal("BPM native-input rect not armed on mobile — native-input migration incomplete")
	}
}

func TestSoftKeyboardCandidate_BPMArmedOnDesktopBrowser(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Desktop browser (NOT mobile) → soft-keyboard channel is the active one.
	g.Layout(1200, 800)

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	g.Update()
	found := false
	for _, r := range g.drum.lastNativeRects {
		if r.Intent.Channel == NativeSoftKeyboard && r.Intent.ID == "bpm" {
			found = true
		}
	}
	if !found {
		t.Fatal("BPM soft-keyboard rect not armed on desktop browser — soft-keyboard channel migration incomplete")
	}
}

// TestParamEditorNativeRectArmedBySync proves the pragmatic exception:
// the shared numeric ParamValueEditor (synth-param / sampler-param / eq-db) is
// not a tree-gated candidate, but while it is open on mobile its <input> rect
// must be re-armed by syncNativeGestures every frame — otherwise
// platformSyncNativeRects' clear-and-re-arm cycle would wipe the rect that
// ParamValueEditor.OpenValue registered imperatively (mobile param-entry
// regression). Asserts the active editor's rect lands in lastNativeRects.
func TestParamEditorNativeRectArmedBySync(t *testing.T) {
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

	// Simulate an open synth-param numeric editor, as OpenValue would leave it.
	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	dv.paramEditor.active = true
	dv.paramEditor.mobileID = "synth-param"
	dv.paramEditor.ti.Rect = image.Rect(100, 100, 200, 140)
	dv.paramEditor.ti.SetText("42")

	dv.syncNativeGestures()

	found := false
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeTextInput && r.Intent.ID == "synth-param" {
			found = true
		}
	}
	if !found {
		t.Fatalf("active synth-param editor rect not preserved by syncNativeGestures; "+
			"lastNativeRects=%+v — the seam clear-cycle would wipe mobile param-entry", dv.lastNativeRects)
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

	// Open the overflow menu (a blocking scrim portal). Any non-menu native rect
	// (e.g. a BPM native input behind the menu) must be suppressed because the
	// menu owns input — TopmostOwnerAt returns "overflow-menu", not the widget.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()

	for _, r := range dv.lastNativeRects {
		if r.Intent.ID == "bpm" {
			t.Fatal("BPM native rect armed while a blocking overlay is open — occlusion gate failed")
		}
	}
}
