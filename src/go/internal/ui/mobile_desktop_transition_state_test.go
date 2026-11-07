package ui

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Equality is the invariant: any field that differs after mobile→desktop is profile state baked at
// construction that survived the transition.
type layoutFingerprint struct {
	// Profile + size-derived
	profileClass ScreenSizeClass
	headerH      int
	eqH          int
	controlsW    int
	labelW       int

	// Layout widget rects
	widgetTransport image.Rectangle
	widgetRack      image.Rectangle
	widgetTimeline  image.Rectangle
	widgetWave      image.Rectangle

	// Profile-dependent button styles (compared via reflect.DeepEqual)
	lenDecStyle     ButtonVisual
	lenIncStyle     ButtonVisual
	lenDecIconColor color.Color
	lenIncIconColor color.Color
	playStyle       ButtonVisual
	stopStyle       ButtonVisual
	bpmDecStyle     ButtonVisual
	bpmIncStyle     ButtonVisual
	subdivStyle     ButtonVisual
	viewSwitchStyle ButtonVisual
	overflowStyle   ButtonVisual

	// Row label typography (mobile bumps TextScale and overrides TextColor)
	rowLabel0Style     ButtonVisual
	rowLabel0TextScale float64
	rowLabel0TextColor color.Color

	// Same width, different MinThumbH: desktop 10, mobile 44 for the touch target.
	rowScrollWidth     int
	rowScrollMinThumbH int

	// Layout-managed surfaces
	bottomActionBarRect image.Rectangle
	addRowBtnRect       image.Rectangle
	lenIncBtnRect       image.Rectangle
	rowZoomChipRect     image.Rectangle
}

func captureFingerprint(dv *DrumView) layoutFingerprint {
	fp := layoutFingerprint{
		profileClass:        Profile().Class,
		headerH:             dv.headerH,
		eqH:                 dv.eqH,
		controlsW:           dv.controlsW,
		labelW:              dv.labelW,
		widgetTransport:     dv.widgetRects[WidgetTransport],
		widgetRack:          dv.widgetRects[WidgetRack],
		widgetTimeline:      dv.widgetRects[WidgetTimeline],
		widgetWave:          dv.widgetRects[WidgetWave],
		bottomActionBarRect: dv.bottomActionBarRect,
		rowZoomChipRect:     dv.rowZoomChipRect,
	}
	if dv.lenDecBtn != nil {
		fp.lenDecStyle = dv.lenDecBtn.Style
		fp.lenDecIconColor = dv.lenDecBtn.IconColor
		fp.lenIncBtnRect = dv.lenIncBtn.Rect()
	}
	if dv.lenIncBtn != nil {
		fp.lenIncStyle = dv.lenIncBtn.Style
		fp.lenIncIconColor = dv.lenIncBtn.IconColor
	}
	if dv.transportZone != nil {
		if dv.transportZone.playBtn != nil {
			fp.playStyle = dv.transportZone.playBtn.Style
		}
		if dv.transportZone.stopBtn != nil {
			fp.stopStyle = dv.transportZone.stopBtn.Style
		}
		if dv.transportZone.bpmDecBtn != nil {
			fp.bpmDecStyle = dv.transportZone.bpmDecBtn.Style
		}
		if dv.transportZone.bpmIncBtn != nil {
			fp.bpmIncStyle = dv.transportZone.bpmIncBtn.Style
		}
		if dv.transportZone.subdivBtn != nil {
			fp.subdivStyle = dv.transportZone.subdivBtn.Style
		}
		if dv.transportZone.viewSwitchBtn != nil {
			fp.viewSwitchStyle = dv.transportZone.viewSwitchBtn.Style
		}
	}
	if dv.rowRackZone != nil {
		if labels := dv.rowRackZone.RowLabels(); len(labels) > 0 && labels[0] != nil {
			fp.rowLabel0Style = labels[0].Style
			fp.rowLabel0TextScale = labels[0].TextScale
			fp.rowLabel0TextColor = labels[0].TextColor
		}
		if dv.rowRackZone.RowScroll() != nil {
			fp.rowScrollWidth = dv.rowRackZone.RowScroll().Style.Width
			fp.rowScrollMinThumbH = dv.rowRackZone.RowScroll().Style.MinThumbH
		}
		if dv.addRowBtn() != nil {
			fp.addRowBtnRect = dv.addRowBtn().Rect()
		}
	}
	return fp
}

// DeepEqual: the specs hold color.Color interfaces, which `==` compares unreliably.
func styleSnapshotsEqual(a, b ButtonVisual) bool {
	return reflect.DeepEqual(a, b)
}

// colorsEqual handles nil-vs-concrete interface comparison for color.Color.
func colorsEqualTest(a, b color.Color) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// Rects may differ; only the tests that check them compare them.
func fingerprintsStyleEqual(a, b layoutFingerprint) (string, bool) {
	if a.profileClass != b.profileClass {
		return "profileClass", false
	}
	if !styleSnapshotsEqual(a.lenDecStyle, b.lenDecStyle) {
		return "lenDecStyle", false
	}
	if !styleSnapshotsEqual(a.lenIncStyle, b.lenIncStyle) {
		return "lenIncStyle", false
	}
	if !colorsEqualTest(a.lenDecIconColor, b.lenDecIconColor) {
		return "lenDecIconColor", false
	}
	if !colorsEqualTest(a.lenIncIconColor, b.lenIncIconColor) {
		return "lenIncIconColor", false
	}
	if !styleSnapshotsEqual(a.playStyle, b.playStyle) {
		return "playStyle", false
	}
	if !styleSnapshotsEqual(a.stopStyle, b.stopStyle) {
		return "stopStyle", false
	}
	if !styleSnapshotsEqual(a.bpmDecStyle, b.bpmDecStyle) {
		return "bpmDecStyle", false
	}
	if !styleSnapshotsEqual(a.bpmIncStyle, b.bpmIncStyle) {
		return "bpmIncStyle", false
	}
	if !styleSnapshotsEqual(a.subdivStyle, b.subdivStyle) {
		return "subdivStyle", false
	}
	if !styleSnapshotsEqual(a.viewSwitchStyle, b.viewSwitchStyle) {
		return "viewSwitchStyle", false
	}
	if !styleSnapshotsEqual(a.overflowStyle, b.overflowStyle) {
		return "overflowStyle", false
	}
	if !styleSnapshotsEqual(a.rowLabel0Style, b.rowLabel0Style) {
		return "rowLabel0Style", false
	}
	if a.rowLabel0TextScale != b.rowLabel0TextScale {
		return "rowLabel0TextScale", false
	}
	if !colorsEqualTest(a.rowLabel0TextColor, b.rowLabel0TextColor) {
		return "rowLabel0TextColor", false
	}
	if a.rowScrollWidth != b.rowScrollWidth {
		return "rowScrollWidth", false
	}
	if a.rowScrollMinThumbH != b.rowScrollMinThumbH {
		return "rowScrollMinThumbH", false
	}
	return "", true
}

// Two settle frames. Register cleanup before any mutation that depends on the screen class.
func buildFreshGame(t *testing.T, w, h int, small bool) *Game {
	t.Helper()
	forceSmallScreenForTest = small
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	advanceFrames(g, 2)
	return g
}

// ─── Test 1: Strongest invariant — full fingerprint equality after transition

// Any state sampled at construction that survives the class change shows up here.
func TestMobileToDesktopFingerprintMatchesFreshDesktop(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	// Reference — fresh desktop
	refG := buildFreshGame(t, 1400, 900, false)
	ref := captureFingerprint(refG.drum)

	// Subject — start mobile, then resize to desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	forceSmallScreenForTest = true
	UpdateProfile()
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 800)
	advanceFrames(g, 2)
	// Transition
	forceSmallScreenForTest = false
	UpdateProfile()
	g.Layout(1400, 900)
	advanceFrames(g, 2)
	got := captureFingerprint(g.drum)

	if field, ok := fingerprintsStyleEqual(ref, got); !ok {
		t.Fatalf("post-transition fingerprint differs from fresh desktop on field %q:\n"+
			"  ref=%+v\n"+
			"  got=%+v",
			field, ref, got)
	}
}

// ─── Test 2: Symmetric direction — desktop→mobile fingerprint equality

// The control case: most baked state is mobile-only, so a failure here is a desktop-only bake.
func TestDesktopToMobileFingerprintMatchesFreshMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	// Reference — fresh mobile
	refG := buildFreshGame(t, 400, 800, true)
	ref := captureFingerprint(refG.drum)

	// Subject — start desktop, then resize to mobile
	forceSmallScreenForTest = false
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1400, 900)
	advanceFrames(g, 2)
	// Transition
	forceSmallScreenForTest = true
	UpdateProfile()
	g.Layout(400, 800)
	advanceFrames(g, 2)
	got := captureFingerprint(g.drum)

	if field, ok := fingerprintsStyleEqual(ref, got); !ok {
		t.Fatalf("post-transition fingerprint differs from fresh mobile on field %q:\n"+
			"  ref=%+v\n"+
			"  got=%+v",
			field, ref, got)
	}
}

// ─── Test 3: Row rack desktop branch selected after transition

// The cluster is the same on both; what flips is the label typography: mobile sets TextScale and TextColor,
// desktop resets them to the Button zero values.
func TestMobileToDesktopRowControlsUseDesktopLayout(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	// Start mobile
	forceSmallScreenForTest = true
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 800)
	advanceFrames(g, 2)

	// The mobile baseline: ⋯ present, label typography upscaled.
	if g.drum.rowRackZone.RowMenuBtns()[0].Rect().Empty() {
		t.Fatal("baseline: mobile must show ⋯ menu button (unified control cluster)")
	}
	if got := g.drum.rowRackZone.RowLabels()[0].TextScale; got == 0 {
		t.Fatalf("baseline: mobile row label TextScale=%v, want non-zero (upscaled)", got)
	}

	// Transition to desktop
	forceSmallScreenForTest = false
	UpdateProfile()
	g.Layout(1400, 900)
	advanceFrames(g, 2)

	// Desktop assertions
	if g.drum.rowRackZone.RowMenuBtns()[0].Rect().Empty() {
		t.Fatal("post-transition: desktop must show ⋯ overflow menu per row (rect should be non-empty)")
	}
	if g.drum.rowRackZone.RowMuteBtns()[0].Rect().Empty() {
		t.Fatal("post-transition: row mute btn must be visible")
	}
	if g.drum.rowRackZone.RowSoloBtns()[0].Rect().Empty() {
		t.Fatal("post-transition: row solo btn must be visible")
	}
	if g.drum.rowRackZone.RowFXBtns()[0].Rect().Empty() {
		t.Fatal("post-transition: row fx btn must be visible")
	}

	// The Button defaults: TextScale 0, TextColor nil.
	if got := g.drum.rowRackZone.RowLabels()[0].TextScale; got != 0 {
		t.Fatalf("post-transition row label TextScale=%v, want 0 (desktop default)", got)
	}
	if got := g.drum.rowRackZone.RowLabels()[0].TextColor; got != nil {
		t.Fatalf("post-transition row label TextColor=%v, want nil (desktop default)", got)
	}
}

// ─── Test 4: Bisecting test — lenDec/lenInc button styles

// The length buttons' Style and IconColor are the desktop values after the transition.
func TestMobileToDesktopLenButtonStylesResetToDesktop(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	// Start mobile, transition to desktop
	forceSmallScreenForTest = true
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 800)
	advanceFrames(g, 2)
	forceSmallScreenForTest = false
	UpdateProfile()
	g.Layout(1400, 900)
	advanceFrames(g, 2)

	dv := g.drum
	if !styleSnapshotsEqual(dv.lenDecBtn.Style, LenDecStyle) {
		t.Fatalf("lenDecBtn.Style after transition: got=%+v want=LenDecStyle (%+v)",
			dv.lenDecBtn.Style, LenDecStyle)
	}
	if !styleSnapshotsEqual(dv.lenIncBtn.Style, LenIncStyle) {
		t.Fatalf("lenIncBtn.Style after transition: got=%+v want=LenIncStyle (%+v)",
			dv.lenIncBtn.Style, LenIncStyle)
	}
	if !colorsEqualTest(dv.lenDecBtn.IconColor, colTextSecondary) {
		t.Fatalf("lenDecBtn.IconColor after transition: got=%v want=colTextSecondary (%v)",
			dv.lenDecBtn.IconColor, colTextSecondary)
	}
	if !colorsEqualTest(dv.lenIncBtn.IconColor, colTextSecondary) {
		t.Fatalf("lenIncBtn.IconColor after transition: got=%v want=colTextSecondary (%v)",
			dv.lenIncBtn.IconColor, colTextSecondary)
	}
}

// ─── Test 5: Negative case — in-class resize must NOT mutate style state

// No over-reset: a resize that stays above 900 px changes no style, typography or scrollbar width.
func TestResizeWithinDesktopPreservesStyleState(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	forceSmallScreenForTest = false
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 800)
	advanceFrames(g, 2)
	before := captureFingerprint(g.drum)

	g.Layout(1600, 1000)
	advanceFrames(g, 2)
	after := captureFingerprint(g.drum)

	if field, ok := fingerprintsStyleEqual(before, after); !ok {
		t.Fatalf("in-class desktop resize mutated style state on field %q:\n"+
			"  before=%+v\n"+
			"  after=%+v",
			field, before, after)
	}
}

// ─── Test 6: Multi-cycle idempotency

// Every visit matches its class's reference, so nothing accumulates across cycles.
func TestRoundTripFingerprintIdempotent(t *testing.T) {
	assertDefaultParityState(t)
	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	})

	// References
	desktopG := buildFreshGame(t, 1400, 900, false)
	desktopRef := captureFingerprint(desktopG.drum)
	mobileG := buildFreshGame(t, 400, 800, true)
	mobileRef := captureFingerprint(mobileG.drum)

	// Cycle
	forceSmallScreenForTest = true
	UpdateProfile()
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 800)
	advanceFrames(g, 2)

	sequence := []struct {
		small bool
		w, h  int
	}{
		{false, 1400, 900},
		{true, 380, 820},
		{false, 1500, 900},
		{true, 410, 800},
		{false, 1400, 900},
	}
	for i, s := range sequence {
		forceSmallScreenForTest = s.small
		UpdateProfile()
		g.Layout(s.w, s.h)
		advanceFrames(g, 2)
		got := captureFingerprint(g.drum)
		want := desktopRef
		mode := "desktop"
		if s.small {
			want = mobileRef
			mode = "mobile"
		}
		if field, ok := fingerprintsStyleEqual(want, got); !ok {
			t.Fatalf("cycle step %d (%s @ %dx%d): fingerprint drifted from fresh baseline on field %q:\n"+
				"  ref=%+v\n"+
				"  got=%+v",
				i, mode, s.w, s.h, field, want, got)
		}
	}
}
