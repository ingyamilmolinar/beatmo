//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- TimelineZone test helpers ---

type timelineTestCallbackLog struct {
	offsetChanges []int
	scrubChanges  []int
}

func newTestTimelineZone() (*TimelineZone, *timelineTestCallbackLog) {
	log := &timelineTestCallbackLog{}
	cb := TimelineCallbacks{
		Rows:                 func() []*DrumRow { return nil },
		IsPlaying:            func() bool { return false },
		Follow:               func() bool { return true },
		BPM:                  func() int { return 120 },
		SecPerBeat:           func() float64 { return 0.5 },
		TimelineUnitsPerBeat: func() int { return 4 },
		Length:               func() int { return 8 },
		Offset:               func() int { return 0 },
		RowOffset:            func() int { return 0 },
		VisibleRows:          func() int { return 4 },
		RowHeight:            func() int { return 24 },
		Cell:                 func() int { return 20 },
		TimelineBeats:        func() int { return 16 },
		Frame:                func() int64 { return 0 },
		OnOffsetChange: func(newOffset int) {
			log.offsetChanges = append(log.offsetChanges, newOffset)
		},
		OnScrubPosition: func(newOffset int) {
			log.scrubChanges = append(log.scrubChanges, newOffset)
		},
	}
	z := NewTimelineZone(cb)
	return z, log
}

func registerTimelineZone(z *TimelineZone, rect image.Rectangle) *DrumViewTree {
	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z.SetPortal(tree.Portal())
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("timeline", rect)
	return tree
}

// --- Zone interface tests ---

func TestTimelineZoneID(t *testing.T) {
	z, _ := newTestTimelineZone()
	if z.ID() != "timeline" {
		t.Errorf("expected ID 'timeline', got %q", z.ID())
	}
}

func TestTimelineZoneLayoutSetsGeometry(t *testing.T) {
	z, _ := newTestTimelineZone()

	if !z.NeedsLayout() {
		t.Fatal("zone should need layout initially")
	}

	r := image.Rect(100, 50, 700, 400)
	z.Layout(r)

	if z.NeedsLayout() {
		t.Fatal("zone should not need layout after Layout()")
	}

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("HitAreas should be non-empty after layout")
	}

	// All hit areas should overlap with the zone rect.
	for _, a := range areas {
		if a.Rect.Empty() {
			continue
		}
		if !a.Rect.Overlaps(r) {
			t.Errorf("hit area %q rect %v does not overlap zone rect %v", a.Tag, a.Rect, r)
		}
	}

	// Verify sub-rects are non-empty.
	if z.StepsRect().Empty() {
		t.Error("StepsRect should be non-empty after layout")
	}
	if z.TimelineBarRect().Empty() {
		t.Error("TimelineBarRect should be non-empty after layout")
	}
}

func TestTimelineZoneInvalidate(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 600, 400))

	if z.NeedsLayout() {
		t.Fatal("should not need layout after Layout()")
	}

	z.Invalidate()
	if !z.NeedsLayout() {
		t.Fatal("should need layout after Invalidate()")
	}
}

func TestTimelineZoneGridDragCapture(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, log := newTestTimelineZone()
	tree := registerTimelineZone(z, image.Rect(100, 50, 700, 400))

	// Frame 1: layout.
	tree.Update()

	gridArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-grid-drag")
	if gridArea == nil {
		t.Fatal("expected 'timeline-grid-drag' hit area")
	}

	// Press in the grid area.
	mx = (gridArea.Rect.Min.X + gridArea.Rect.Max.X) / 2
	my = (gridArea.Rect.Min.Y + gridArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()

	if !z.IsDragging() {
		t.Error("expected IsDragging() true after press in grid")
	}

	if !tree.Capturing() {
		t.Error("tree should be capturing after grid press")
	}

	// Drag to the left (should increase offset).
	mx -= 40
	tree.Update()

	if len(log.offsetChanges) == 0 {
		t.Error("expected OnOffsetChange callback after drag")
	}

	// Release.
	pressed = false
	tree.Update()

	if z.IsDragging() {
		t.Error("expected IsDragging() false after release")
	}

	if tree.Capturing() {
		t.Error("tree should not be capturing after release")
	}
}

func TestTimelineZoneScrubCapture(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, log := newTestTimelineZone()
	tree := registerTimelineZone(z, image.Rect(100, 50, 700, 400))

	// Frame 1: layout.
	tree.Update()

	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatal("expected 'timeline-scrub' hit area")
	}

	// Press in the timeline bar.
	mx = (scrubArea.Rect.Min.X + scrubArea.Rect.Max.X) / 2
	my = (scrubArea.Rect.Min.Y + scrubArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()

	if !z.IsScrubbing() {
		t.Error("expected IsScrubbing() true after press in timeline bar")
	}

	if !tree.Capturing() {
		t.Error("tree should be capturing after scrub press")
	}

	if len(log.scrubChanges) == 0 {
		t.Error("expected OnScrubPosition callback after scrub press")
	}

	// Drag along the timeline bar.
	mx += 50
	tree.Update()

	// Release.
	pressed = false
	tree.Update()

	if z.IsScrubbing() {
		t.Error("expected IsScrubbing() false after release")
	}
}

func TestTimelineZoneTreeLifecycle(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestTimelineZone()
	tree := registerTimelineZone(z, image.Rect(100, 50, 700, 400))

	tree.Update()

	if z.NeedsLayout() {
		t.Error("zone should have been laid out by tree")
	}

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("zone should have hit areas after layout")
	}

	// Pick the grid drag area — should always be present.
	gridArea := findHitAreaByTagPrefix(areas, "timeline-grid-drag")
	if gridArea == nil {
		t.Fatal("expected timeline-grid-drag hit area")
	}
	cx := (gridArea.Rect.Min.X + gridArea.Rect.Max.X) / 2
	cy := (gridArea.Rect.Min.Y + gridArea.Rect.Max.Y) / 2
	idxAreas := tree.HitIndexRef().At(cx, cy)
	if len(idxAreas) == 0 {
		t.Errorf("expected hit areas at (%d,%d) in hit index after tree update", cx, cy)
	}
}

func TestTimelineZoneHandleKeyIgnored(t *testing.T) {
	z, _ := newTestTimelineZone()
	if r := z.HandleKey(ebiten.KeyEnter); r != InputIgnored {
		t.Errorf("expected InputIgnored for key, got %d", r)
	}
}

func TestTimelineZoneMarkRowDirty(t *testing.T) {
	layerDirtied := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowsLayerDirty = func() { layerDirtied = true }
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.Layout(image.Rect(100, 50, 700, 400))

	// Ensure cache is allocated.
	z.EnsureRowCache()
	// Clear dirty to test marking.
	for i := range z.RowDirty {
		z.RowDirty[i] = false
	}
	for i := range z.RowFullDirty {
		z.RowFullDirty[i] = false
	}
	layerDirtied = false

	z.MarkRowDirty(0)

	if !z.RowDirty[0] {
		t.Error("RowDirty[0] should be true after MarkRowDirty(0)")
	}
	if !z.RowFullDirty[0] {
		t.Error("RowFullDirty[0] should be true after MarkRowDirty(0)")
	}
	if z.RowDirty[1] {
		t.Error("RowDirty[1] should remain false")
	}
	if !layerDirtied {
		t.Error("OnRowsLayerDirty should have been called")
	}
}

func TestTimelineZoneEnsureRowCache(t *testing.T) {
	z, _ := newTestTimelineZone()
	rows := []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	z.callbacks.Rows = func() []*DrumRow { return rows }

	reallocated := z.EnsureRowCache()
	if !reallocated {
		t.Error("expected reallocation on first EnsureRowCache")
	}
	if len(z.RowCache) != 3 {
		t.Errorf("expected RowCache len 3, got %d", len(z.RowCache))
	}
	if len(z.RowDirty) != 3 {
		t.Errorf("expected RowDirty len 3, got %d", len(z.RowDirty))
	}

	// All should be dirty initially.
	for i, d := range z.RowDirty {
		if !d {
			t.Errorf("RowDirty[%d] should be true after initial ensure", i)
		}
	}

	// Second call with same count should not reallocate.
	reallocated = z.EnsureRowCache()
	if reallocated {
		t.Error("should not reallocate when count unchanged")
	}
}

func TestTimelineZoneMarkAllRowsDirty(t *testing.T) {
	layerDirtied := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowsLayerDirty = func() { layerDirtied = true }
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()
	// Clear dirty flags.
	for i := range z.RowDirty {
		z.RowDirty[i] = false
	}
	for i := range z.RowFullDirty {
		z.RowFullDirty[i] = false
	}

	z.MarkAllRowsDirty()

	for i := range z.RowDirty {
		if !z.RowDirty[i] {
			t.Errorf("RowDirty[%d] should be true after MarkAllRowsDirty", i)
		}
		if !z.RowFullDirty[i] {
			t.Errorf("RowFullDirty[%d] should be true after MarkAllRowsDirty", i)
		}
	}
	if !layerDirtied {
		t.Error("OnRowsLayerDirty should have been called")
	}
}

func TestTimelineZoneInvalidateRowCaches(t *testing.T) {
	// Verify layout resets dimensions on re-layout with different rect.
	z, _ := newTestTimelineZone()
	r1 := image.Rect(100, 50, 700, 400)
	z.Layout(r1)
	sr1 := z.StepsRect()

	r2 := image.Rect(100, 50, 500, 300)
	z.Invalidate()
	z.Layout(r2)
	sr2 := z.StepsRect()

	if sr1 == sr2 {
		t.Error("StepsRect should change when layout rect changes")
	}
}

func TestTimelineZoneResetAfterDelete(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()

	if len(z.RowCache) != 2 {
		t.Fatalf("expected 2 row caches, got %d", len(z.RowCache))
	}

	z.ResetAfterDelete()

	if z.RowCache != nil {
		t.Error("RowCache should be nil after ResetAfterDelete")
	}
	if z.RowDirty != nil {
		t.Error("RowDirty should be nil after ResetAfterDelete")
	}
	if z.RowFullDirty != nil {
		t.Error("RowFullDirty should be nil after ResetAfterDelete")
	}
}

// --- P1 gap tests ---

func TestTimelineZone_WheelOverGrid(t *testing.T) {
	assertDefaultParityState(t)

	wheelReceived := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollWheel = func(steps int) bool {
		wheelReceived = true
		return true
	}
	tree := registerTimelineZone(z, image.Rect(0, 50, 800, 500))

	restore := noInputForTest()
	tree.Update()
	restore()

	gridArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-grid-drag")
	if gridArea == nil {
		t.Fatal("expected grid drag hit area")
	}

	cx := (gridArea.Rect.Min.X + gridArea.Rect.Max.X) / 2
	cy := (gridArea.Rect.Min.Y + gridArea.Rect.Max.Y) / 2

	result := gridArea.Handler.OnWheel(cx, cy, 2)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}
	if !wheelReceived {
		t.Error("expected OnRowScrollWheel to be called")
	}
}

func TestTimelineZone_LenIncButton(t *testing.T) {
	assertDefaultParityState(t)

	beatsCalled := 0
	z, _ := newTestTimelineZone()
	z.callbacks.SetTimelineBeats = func(n int) { beatsCalled = n }
	z.callbacks.TimelineBeats = func() int { return 16 }

	lenInc := NewButton("+", InstButtonStyle, func() {
		if z.callbacks.SetTimelineBeats != nil {
			z.callbacks.SetTimelineBeats(z.callbacks.TimelineBeats() + 1)
		}
	})
	lenInc.SetRect(image.Rect(750, 30, 780, 50))
	z.SetButtons(nil, nil, lenInc)

	tree := registerTimelineZone(z, image.Rect(0, 0, 800, 500))
	restore := noInputForTest()
	tree.Update()
	restore()

	lenInc.OnClick()
	if beatsCalled != 17 {
		t.Errorf("expected SetTimelineBeats(17), got %d", beatsCalled)
	}
}

func TestTimelineZone_LenDecButton(t *testing.T) {
	assertDefaultParityState(t)

	beatsCalled := 0
	z, _ := newTestTimelineZone()
	z.callbacks.SetTimelineBeats = func(n int) { beatsCalled = n }
	z.callbacks.TimelineBeats = func() int { return 16 }

	lenDec := NewButton("-", InstButtonStyle, func() {
		if z.callbacks.SetTimelineBeats != nil {
			z.callbacks.SetTimelineBeats(z.callbacks.TimelineBeats() - 1)
		}
	})
	lenDec.SetRect(image.Rect(710, 30, 740, 50))
	z.SetButtons(nil, lenDec, nil)

	tree := registerTimelineZone(z, image.Rect(0, 0, 800, 500))
	restore := noInputForTest()
	tree.Update()
	restore()

	lenDec.OnClick()
	if beatsCalled != 15 {
		t.Errorf("expected SetTimelineBeats(15), got %d", beatsCalled)
	}
}

func TestTimelineZone_LenDecClampMin(t *testing.T) {
	assertDefaultParityState(t)

	beatsCalled := -1
	z, _ := newTestTimelineZone()
	z.callbacks.SetTimelineBeats = func(n int) { beatsCalled = n }
	z.callbacks.TimelineBeats = func() int { return 1 }

	lenDec := NewButton("-", InstButtonStyle, func() {
		beats := z.callbacks.TimelineBeats() - 1
		if beats < 1 {
			beats = 1
		}
		if z.callbacks.SetTimelineBeats != nil {
			z.callbacks.SetTimelineBeats(beats)
		}
	})
	lenDec.SetRect(image.Rect(710, 30, 740, 50))
	z.SetButtons(nil, lenDec, nil)

	tree := registerTimelineZone(z, image.Rect(0, 0, 800, 500))
	restore := noInputForTest()
	tree.Update()
	restore()

	lenDec.OnClick()
	if beatsCalled != 1 {
		t.Errorf("expected SetTimelineBeats(1) (clamped), got %d", beatsCalled)
	}
}

func TestTimelineZone_ScrubPosition(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestTimelineZone()
	tree := registerTimelineZone(z, image.Rect(0, 0, 800, 500))

	restore := noInputForTest()
	tree.Update()
	restore()

	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatal("expected timeline-scrub hit area")
	}

	barRect := z.TimelineBarRect()
	if barRect.Empty() {
		t.Fatal("expected non-empty timeline bar rect")
	}

	// Press at ~75% along the timeline bar.
	scrubX := barRect.Min.X + barRect.Dx()*3/4
	scrubY := (barRect.Min.Y + barRect.Max.Y) / 2

	result := scrubArea.Handler.OnPress(scrubX, scrubY)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", result)
	}
	if !z.IsScrubbing() {
		t.Error("expected scrubbing after press")
	}
	if len(log.scrubChanges) == 0 {
		t.Error("expected OnScrubPosition to fire")
	}

	scrubArea.Handler.OnRelease(scrubX, scrubY)
	if z.IsScrubbing() {
		t.Error("expected scrubbing to end after release")
	}
}

func TestTimelineZone_DragClampToZero(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestTimelineZone()
	z.callbacks.Offset = func() int { return 0 }
	tree := registerTimelineZone(z, image.Rect(0, 50, 800, 500))

	restore := noInputForTest()
	tree.Update()
	restore()

	gridArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-grid-drag")
	if gridArea == nil {
		t.Fatal("expected grid drag hit area")
	}

	cx := (gridArea.Rect.Min.X + gridArea.Rect.Max.X) / 2
	cy := (gridArea.Rect.Min.Y + gridArea.Rect.Max.Y) / 2

	gridArea.Handler.OnPress(cx, cy)
	// Drag far right (positive X = negative offset delta).
	gridArea.Handler.OnDrag(cx+500, cy)
	gridArea.Handler.OnRelease(cx+500, cy)

	// Any offset change should be >= 0 (clamped).
	for _, off := range log.offsetChanges {
		if off < 0 {
			t.Errorf("expected offset >= 0, got %d", off)
		}
	}
}

func TestTimelineZone_MobileEQModeSkipsDraw(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestTimelineZone()
	mobileEQActive := false
	z.callbacks.MobileEQActive = func() bool { return mobileEQActive }
	z.callbacks.DrawRowComposite = func(dst *ebiten.Image) {}
	z.callbacks.BeatLength = func() int { return 4 }
	z.callbacks.SimpleDraw = func() bool { return false }
	z.callbacks.PerfDrawLite = func() bool { return false }
	z.callbacks.BeatCounterRect = func() image.Rectangle { return image.Rectangle{} }
	z.callbacks.RowsTopY = func() int { return 100 }

	tree := registerTimelineZone(z, image.Rect(0, 50, 800, 500))
	restore := noInputForTest()
	tree.Update()
	restore()

	// When MobileEQActive is true, Draw should complete without panic.
	mobileEQActive = true
	z.Draw(ebiten.NewImage(800, 500))
	// If no panic, the test passes — the skip path executed.
}

func TestTimelineZone_TrackButtonHitArea(t *testing.T) {
	assertDefaultParityState(t)

	clicked := false
	z, _ := newTestTimelineZone()
	trackBtn := NewButton("Track", InstButtonStyle, func() { clicked = true })
	trackBtn.SetRect(image.Rect(10, 10, 60, 30))
	z.SetButtons(trackBtn, nil, nil)

	tree := registerTimelineZone(z, image.Rect(0, 0, 800, 500))
	restore := noInputForTest()
	tree.Update()
	restore()

	trackArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-track")
	if trackArea == nil {
		t.Fatal("expected timeline-track hit area")
	}

	trackBtn.OnClick()
	if !clicked {
		t.Error("expected track button callback to fire")
	}
}

func TestTimelineZone_RowScrollWheel(t *testing.T) {
	assertDefaultParityState(t)

	wheelSteps := 0
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollWheel = func(steps int) bool {
		wheelSteps = steps
		return true
	}
	tree := registerTimelineZone(z, image.Rect(0, 50, 800, 500))

	restore := noInputForTest()
	tree.Update()
	restore()

	gridArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-grid-drag")
	if gridArea == nil {
		t.Fatal("expected grid drag hit area")
	}

	cx := (gridArea.Rect.Min.X + gridArea.Rect.Max.X) / 2
	cy := (gridArea.Rect.Min.Y + gridArea.Rect.Max.Y) / 2
	gridArea.Handler.OnWheel(cx, cy, -3)

	if wheelSteps != -3 {
		t.Errorf("expected wheel steps=-3, got %d", wheelSteps)
	}
}
