//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// --- Grid drag hit adapter tests ---

func TestGridDragHitAdapter_OnPress(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	result := adapter.OnPress(100, 100)

	if result != InputCaptured {
		t.Errorf("expected InputCaptured, got %d", result)
	}
	if !z.IsDragging() {
		t.Error("expected IsDragging() true after OnPress")
	}
}

func TestGridDragHitAdapter_OnDrag_Horizontal(t *testing.T) {
	offsetChanged := false
	var lastOffset int
	z, _ := newTestTimelineZone()
	z.callbacks.OnOffsetChange = func(newOffset int) {
		offsetChanged = true
		lastOffset = newOffset
	}
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	adapter.OnPress(100, 100)

	// Drag horizontally past deadzone (8px) — dragging left increases offset.
	adapter.OnDrag(150, 100)

	if !offsetChanged {
		t.Error("expected OnOffsetChange to fire after horizontal drag past deadzone")
	}
	// Dragging right (x increasing) means dragStartX - x < 0, so offset stays 0.
	// Drag left instead to increase offset.
	offsetChanged = false
	adapter2 := &gridDragHitAdapter{zone: z}
	adapter2.OnPress(200, 100)
	adapter2.OnDrag(150, 100) // 50px left, cell=20, delta=50/20=2

	if !offsetChanged {
		t.Error("expected OnOffsetChange to fire after leftward drag")
	}
	if lastOffset != 2 {
		t.Errorf("expected offset 2, got %d", lastOffset)
	}
}

func TestGridDragHitAdapter_OnDrag_Vertical(t *testing.T) {
	scrolled := false
	var scrollTarget int
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollDrag = func(targetRowOffset int) {
		scrolled = true
		scrollTarget = targetRowOffset
	}
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	adapter.OnPress(100, 100)

	// Drag vertically past deadzone — dy > dx.
	adapter.OnDrag(100, 50) // 50px up, rowH=24, (100-50)/24 = 2

	if !scrolled {
		t.Error("expected OnRowScrollDrag to fire after vertical drag")
	}
	if scrollTarget != 2 {
		t.Errorf("expected scroll target 2, got %d", scrollTarget)
	}
}

func TestGridDragHitAdapter_OnDrag_DeadZone(t *testing.T) {
	offsetChanged := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnOffsetChange = func(int) { offsetChanged = true }
	z.callbacks.OnRowScrollDrag = func(int) { offsetChanged = true }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	adapter.OnPress(100, 100)

	// Small drag within deadzone (< 8px in both axes).
	adapter.OnDrag(105, 103)

	if offsetChanged {
		t.Error("expected no callback within deadzone")
	}
}

func TestGridDragHitAdapter_OnRelease(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	adapter.OnPress(100, 100)

	if !z.IsDragging() {
		t.Fatal("should be dragging after press")
	}

	adapter.OnRelease(100, 100)

	if z.IsDragging() {
		t.Error("expected IsDragging() false after OnRelease")
	}
}

func TestGridDragHitAdapter_OnWheel(t *testing.T) {
	wheelCalled := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollWheel = func(steps int) bool {
		wheelCalled = true
		return true
	}
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	result := adapter.OnWheel(100, 100, 3)

	if !wheelCalled {
		t.Error("expected OnRowScrollWheel callback")
	}
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}
}

func TestGridDragHitAdapter_OnWheel_Zero(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	result := adapter.OnWheel(100, 100, 0)

	if result != InputIgnored {
		t.Errorf("expected InputIgnored for steps=0, got %d", result)
	}
}

func TestGridDragHitAdapter_OnWheel_NotConsumed(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollWheel = func(steps int) bool { return false }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	result := adapter.OnWheel(100, 100, 1)

	if result != InputIgnored {
		t.Errorf("expected InputIgnored when wheel callback returns false, got %d", result)
	}
}

// --- Timeline scrub hit adapter tests ---

func TestTimelineScrubHitAdapter_OnPress(t *testing.T) {
	scrubCalled := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnScrubPosition = func(int) { scrubCalled = true }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}
	result := adapter.OnPress(200, 5)

	if result != InputCaptured {
		t.Errorf("expected InputCaptured, got %d", result)
	}
	if !z.IsScrubbing() {
		t.Error("expected IsScrubbing() true after OnPress")
	}
	if !scrubCalled {
		t.Error("expected OnScrubPosition callback on press")
	}
}

func TestTimelineScrubHitAdapter_OnDrag(t *testing.T) {
	scrubCount := 0
	z, _ := newTestTimelineZone()
	z.callbacks.OnScrubPosition = func(int) { scrubCount++ }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(200, 5)
	initialCount := scrubCount

	adapter.OnDrag(250, 5)

	if scrubCount <= initialCount {
		t.Error("expected OnScrubPosition callback on drag")
	}
}

func TestTimelineScrubHitAdapter_OnDrag_NotScrubbing(t *testing.T) {
	scrubCalled := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnScrubPosition = func(int) { scrubCalled = true }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}
	// Don't press first — drag without scrubbing.
	adapter.OnDrag(250, 5)

	if scrubCalled {
		t.Error("OnScrubPosition should not fire when not scrubbing")
	}
}

func TestTimelineScrubHitAdapter_OnRelease(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(200, 5)

	if !z.IsScrubbing() {
		t.Fatal("should be scrubbing after press")
	}

	adapter.OnRelease(200, 5)

	if z.IsScrubbing() {
		t.Error("expected IsScrubbing() false after OnRelease")
	}
}

func TestTimelineScrubHitAdapter_OnWheel(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}
	result := adapter.OnWheel(100, 100, 3)

	if result != InputIgnored {
		t.Errorf("expected InputIgnored, got %d", result)
	}
}

// --- Cache management tests ---

func TestTimelineZone_MarkRowCellsDirty(t *testing.T) {
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

	z.MarkRowCellsDirty(0)

	if !z.RowDirty[0] {
		t.Error("RowDirty[0] should be true after MarkRowCellsDirty")
	}
	if z.RowFullDirty[0] {
		t.Error("RowFullDirty[0] should remain false after MarkRowCellsDirty")
	}
	if z.RowDirty[1] {
		t.Error("RowDirty[1] should remain false")
	}
	if !layerDirtied {
		t.Error("OnRowsLayerDirty should have been called")
	}
}

func TestTimelineZone_MarkRowShiftDirty(t *testing.T) {
	layerDirtied := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowsLayerDirty = func() { layerDirtied = true }
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()
	// Clear dirty flags, then set RowFullDirty[1] to true to verify it gets set to false.
	for i := range z.RowDirty {
		z.RowDirty[i] = false
	}
	for i := range z.RowFullDirty {
		z.RowFullDirty[i] = true
	}

	z.MarkRowShiftDirty(1)

	if !z.RowDirty[1] {
		t.Error("RowDirty[1] should be true after MarkRowShiftDirty")
	}
	if z.RowFullDirty[1] {
		t.Error("RowFullDirty[1] should be false after MarkRowShiftDirty")
	}
	if !layerDirtied {
		t.Error("OnRowsLayerDirty should have been called")
	}
}

func TestTimelineZone_MarkRowsShiftDirty(t *testing.T) {
	layerDirtied := false
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowsLayerDirty = func() { layerDirtied = true }
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()
	for i := range z.RowDirty {
		z.RowDirty[i] = false
	}

	z.MarkRowsShiftDirty()

	for i := range z.RowDirty {
		if !z.RowDirty[i] {
			t.Errorf("RowDirty[%d] should be true after MarkRowsShiftDirty", i)
		}
	}
	if !layerDirtied {
		t.Error("OnRowsLayerDirty should have been called")
	}
}

func TestTimelineZone_MarkRowDirty_OutOfBounds(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()

	// Should not panic for out of bounds index.
	z.MarkRowDirty(-1)
	z.MarkRowDirty(99)
}

func TestTimelineZone_MarkRowCellsDirty_OutOfBounds(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()

	// Should not panic.
	z.MarkRowCellsDirty(-1)
	z.MarkRowCellsDirty(99)
}

func TestTimelineZone_ResetAfterDelete_AllNil(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()

	// Set some non-nil values.
	z.RowsDrawnMask = make([]bool, 2)

	z.ResetAfterDelete()

	if z.RowCache != nil {
		t.Error("RowCache should be nil")
	}
	if z.RowDirty != nil {
		t.Error("RowDirty should be nil")
	}
	if z.RowFullDirty != nil {
		t.Error("RowFullDirty should be nil")
	}
	if z.RowFrame != nil {
		t.Error("RowFrame should be nil")
	}
	if z.RowRepaint != nil {
		t.Error("RowRepaint should be nil")
	}
	if z.RowsDrawnMask != nil {
		t.Error("RowsDrawnMask should be nil")
	}
}

// --- SetButtons / RefreshButtonHitAreas ---

func TestTimelineZone_SetButtons(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	initialAreas := len(z.HitAreas())

	trackBtn := NewButton("T", nil, func() {})
	trackBtn.SetRect(image.Rect(10, 10, 40, 30))
	lenDecBtn := NewButton("-", nil, func() {})
	lenDecBtn.SetRect(image.Rect(50, 10, 80, 30))
	lenIncBtn := NewButton("+", nil, func() {})
	lenIncBtn.SetRect(image.Rect(90, 10, 120, 30))

	z.SetButtons(trackBtn, lenDecBtn, lenIncBtn)
	z.RefreshButtonHitAreas()

	newAreas := len(z.HitAreas())
	if newAreas <= initialAreas {
		t.Errorf("expected more hit areas after SetButtons+Refresh, had %d now %d", initialAreas, newAreas)
	}

	// Verify button hit areas are present.
	found := map[string]bool{}
	for _, a := range z.HitAreas() {
		found[a.Tag] = true
	}
	for _, tag := range []string{"timeline-len-dec", "timeline-len-inc"} {
		if !found[tag] {
			t.Errorf("expected hit area with tag %q", tag)
		}
	}
}

func TestTimelineZone_SetButtons_NilButtons(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	// Setting nil buttons should not add extra hit areas.
	z.SetButtons(nil, nil, nil)
	z.RefreshButtonHitAreas()

	for _, a := range z.HitAreas() {
		if a.Tag == "timeline-track" || a.Tag == "timeline-len-dec" || a.Tag == "timeline-len-inc" {
			t.Errorf("unexpected button hit area %q when buttons are nil", a.Tag)
		}
	}
}

// --- computeTimelineBeats ---

func TestTimelineZone_ComputeTimelineBeats(t *testing.T) {
	currentBeats := 4
	z, _ := newTestTimelineZone()
	z.callbacks.TimelineBeats = func() int { return currentBeats }
	z.callbacks.SetTimelineBeats = func(n int) { currentBeats = n }
	z.callbacks.BeatLength = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	// With low elapsed beats, timeline stays at initial.
	z.computeTimelineBeats(1.0)
	if currentBeats < 4 {
		t.Errorf("timeline beats should not shrink below initial, got %d", currentBeats)
	}

	// With high elapsed beats, timeline grows.
	z.computeTimelineBeats(100.0)
	if currentBeats <= 4 {
		t.Errorf("timeline beats should grow for high elapsed beats, got %d", currentBeats)
	}

	// It should never shrink.
	highMark := currentBeats
	z.computeTimelineBeats(1.0)
	if currentBeats < highMark {
		t.Errorf("timeline beats should not shrink from %d, got %d", highMark, currentBeats)
	}
}

func TestTimelineZone_ComputeTimelineBeats_BeatLength(t *testing.T) {
	currentBeats := 2
	z, _ := newTestTimelineZone()
	z.callbacks.TimelineBeats = func() int { return currentBeats }
	z.callbacks.SetTimelineBeats = func(n int) { currentBeats = n }
	// Graph has 20 beat-length units, unitsPerBeat=4, so ceil(20/4)=5 beats.
	z.callbacks.BeatLength = func() int { return 20 }
	z.callbacks.IsPlaying = func() bool { return false }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.computeTimelineBeats(0.0)
	if currentBeats < 5 {
		t.Errorf("expected at least 5 beats from BeatLength, got %d", currentBeats)
	}
}

func TestTimelineZone_ComputeTimelineBeats_OffsetExpands(t *testing.T) {
	currentBeats := 2
	z, _ := newTestTimelineZone()
	z.callbacks.TimelineBeats = func() int { return currentBeats }
	z.callbacks.SetTimelineBeats = func(n int) { currentBeats = n }
	z.callbacks.BeatLength = func() int { return 0 }
	// Large offset: offset=60, length=8, unitsPerBeat=4.
	// offsetBeats = 60/4 = 15, lengthBeats = 8/4 = 2, needBeats2 = ceil(15+2) = 17.
	z.callbacks.Offset = func() int { return 60 }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.computeTimelineBeats(0.0)
	if currentBeats < 17 {
		t.Errorf("expected at least 17 beats from offset, got %d", currentBeats)
	}
}

// --- timelineInfoCached ---

func TestTimelineZone_TimelineInfoCached(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	// First call generates the info string.
	info1 := z.timelineInfoCached(2.0)
	if info1 == "" {
		t.Fatal("expected non-empty info string")
	}

	// Second call with same params should return cached result.
	info2 := z.timelineInfoCached(2.0)
	if info2 != info1 {
		t.Errorf("expected cached result %q, got %q", info1, info2)
	}

	// Different elapsed beats should produce different text.
	info3 := z.timelineInfoCached(10.0)
	if info3 == info1 {
		t.Error("expected different text for different elapsed beats")
	}
}

func TestTimelineZone_TimelineInfoCached_Playing(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.IsPlaying = func() bool { return true }
	z.Layout(image.Rect(0, 0, 400, 300))

	// When playing and elapsed > pattern beats, total should use elapsed.
	info := z.timelineInfoCached(20.0)
	if info == "" {
		t.Fatal("expected non-empty info string")
	}
	// Verify string contains the beat number.
	if len(info) < 5 {
		t.Errorf("info string too short: %q", info)
	}
}

// --- ensureHighlightSprites ---

func TestTimelineZone_EnsureHighlightSprites(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	z.ensureHighlightSprites()

	if z.hlSpriteReg == nil {
		t.Error("hlSpriteReg should not be nil after ensureHighlightSprites")
	}
	if z.hlSpriteMute == nil {
		t.Error("hlSpriteMute should not be nil after ensureHighlightSprites")
	}
	if z.hlSpriteH != 24 { // RowHeight from newTestTimelineZone
		t.Errorf("expected hlSpriteH=24, got %d", z.hlSpriteH)
	}

	// Second call should not reallocate (cache hit).
	prevReg := z.hlSpriteReg
	z.ensureHighlightSprites()
	if z.hlSpriteReg != prevReg {
		t.Error("expected cache hit on second call with same height")
	}
}

func TestTimelineZone_EnsureHighlightSprites_HeightChange(t *testing.T) {
	rh := 24
	z, _ := newTestTimelineZone()
	z.callbacks.RowHeight = func() int { return rh }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.ensureHighlightSprites()
	prevReg := z.hlSpriteReg

	// Change row height — should rebuild sprites.
	rh = 32
	z.ensureHighlightSprites()
	if z.hlSpriteReg == prevReg {
		t.Error("expected new sprites after height change")
	}
	if z.hlSpriteH != 32 {
		t.Errorf("expected hlSpriteH=32, got %d", z.hlSpriteH)
	}
}

func TestTimelineZone_EnsureHighlightSprites_ZeroHeight(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.RowHeight = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	// Should not panic, and should clamp height to 1.
	z.ensureHighlightSprites()
	if z.hlSpriteH != 1 {
		t.Errorf("expected hlSpriteH=1 for zero row height, got %d", z.hlSpriteH)
	}
}

// --- drawGapFill ---

func TestTimelineZone_DrawGapFill(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8)},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 4 } // More visible rows than actual rows.
	z.callbacks.RowHeight = func() int { return 30 }
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// Should not panic — there is a gap below the single visible row.
	z.drawGapFill(dst)
}

func TestTimelineZone_DrawGapFill_NoGap(t *testing.T) {
	rows := make([]*DrumRow, 10)
	for i := range rows {
		rows[i] = &DrumRow{Instrument: "kick", Steps: make([]bool, 8)}
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 4 }
	z.callbacks.RowHeight = func() int { return 100 } // 4*100=400 > stepsRect height
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// Should not panic even when rows fill or exceed the area.
	z.drawGapFill(dst)
}

// --- drawMuteSoloDimming ---

func TestTimelineZone_DrawMuteSoloDimming_NoSoloOrMute(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8), Color: color.RGBA{255, 0, 0, 255}},
		{Instrument: "snare", Steps: make([]bool, 8), Color: color.RGBA{0, 255, 0, 255}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 2 }
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// No muted or soloed rows — should be a no-op, no panic.
	z.drawMuteSoloDimming(dst)
}

func TestTimelineZone_DrawMuteSoloDimming_MutedRow(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8), Muted: true, Color: color.RGBA{255, 0, 0, 255}},
		{Instrument: "snare", Steps: make([]bool, 8), Color: color.RGBA{0, 255, 0, 255}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 2 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// Should render dimming for muted row without panic.
	z.drawMuteSoloDimming(dst)
}

func TestTimelineZone_DrawMuteSoloDimming_SoloRow(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8), Solo: true, Color: color.RGBA{255, 0, 0, 255}},
		{Instrument: "snare", Steps: make([]bool, 8), Color: color.RGBA{0, 255, 0, 255}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 2 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// Row 0 is soloed: row 1 (not soloed) should be dimmed.
	z.drawMuteSoloDimming(dst)
}

// --- drawHighlights ---

func TestTimelineZone_DrawHighlights_NoHighlights(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Color: color.RGBA{255, 0, 0, 255}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.Layout(image.Rect(0, 0, 400, 300))

	dst := ebiten.NewImage(400, 300)
	// No highlights set — should be no-op.
	z.drawHighlights(dst, false)
}

func TestTimelineZone_DrawHighlights_WithHighlights(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	rows[0].Steps[2] = true
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	// Set up highlights for row 0, index 2.
	z.highlightsByRow = [][]highlightEntry{
		{{idx: 2, val: 100}}, // regular highlight
	}

	dst := ebiten.NewImage(400, 300)
	// Non-simple draw path.
	z.drawHighlights(dst, false)
}

func TestTimelineZone_DrawHighlights_SimpleDraw(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.highlightsByRow = [][]highlightEntry{
		{{idx: 0, val: 100}},
	}

	dst := ebiten.NewImage(400, 300)
	// Simple draw path uses sprite scaling.
	z.drawHighlights(dst, true)

	if z.hlSpriteReg == nil {
		t.Error("hlSpriteReg should have been created in simpleDraw path")
	}
}

func TestTimelineZone_DrawHighlights_MuteHighlight(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	// Set mute highlight (high bit set).
	z.highlightsByRow = [][]highlightEntry{
		{{idx: 0, val: 100 | highlightMuteFlag}},
	}

	dst := ebiten.NewImage(400, 300)
	// Non-simple path — should use mute highlight color.
	z.drawHighlights(dst, false)
}

func TestTimelineZone_DrawHighlights_MuteSimpleDraw(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.highlightsByRow = [][]highlightEntry{
		{{idx: 0, val: 100 | highlightMuteFlag}},
	}

	dst := ebiten.NewImage(400, 300)
	z.drawHighlights(dst, true)

	if z.hlSpriteMute == nil {
		t.Error("hlSpriteMute should have been created for mute highlight in simpleDraw")
	}
}

// --- InvalidateRowCaches ---

func TestTimelineZone_InvalidateRowCaches(t *testing.T) {
	layerDirtyCount := 0
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowsLayerDirty = func() { layerDirtyCount++ }
	z.callbacks.Rows = func() []*DrumRow {
		return []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	}
	z.EnsureRowCache()
	for i := range z.RowDirty {
		z.RowDirty[i] = false
	}
	for i := range z.RowFullDirty {
		z.RowFullDirty[i] = false
	}

	z.InvalidateRowCaches()

	for i := range z.RowDirty {
		if !z.RowDirty[i] {
			t.Errorf("RowDirty[%d] should be true after InvalidateRowCaches", i)
		}
	}
	for i := range z.RowFullDirty {
		if !z.RowFullDirty[i] {
			t.Errorf("RowFullDirty[%d] should be true after InvalidateRowCaches", i)
		}
	}
	// MarkAllRowsDirty calls OnRowsLayerDirty, then InvalidateRowCaches calls it again.
	if layerDirtyCount < 2 {
		t.Errorf("expected OnRowsLayerDirty to be called at least twice, got %d", layerDirtyCount)
	}
}

// --- NeedsRowRebuild ---

func TestTimelineZone_NeedsRowRebuild(t *testing.T) {
	rows := []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.EnsureRowCache()

	// Freshly allocated — all rows dirty.
	if !z.NeedsRowRebuild(0) {
		t.Error("expected NeedsRowRebuild(0) true after initial allocation")
	}

	// Clear dirty and set cache image.
	z.RowDirty[0] = false
	z.RowFullDirty[0] = false
	z.RowCache[0] = ebiten.NewImage(1, 1)

	if z.NeedsRowRebuild(0) {
		t.Error("expected NeedsRowRebuild(0) false when not dirty and cache exists")
	}

	// Nil cache should need rebuild.
	z.RowCache[1] = nil
	z.RowDirty[1] = false
	z.RowFullDirty[1] = false
	if !z.NeedsRowRebuild(1) {
		t.Error("expected NeedsRowRebuild(1) true when RowCache[1] is nil")
	}

	// Out of bounds returns false.
	if z.NeedsRowRebuild(-1) {
		t.Error("expected NeedsRowRebuild(-1) false")
	}
	if z.NeedsRowRebuild(99) {
		t.Error("expected NeedsRowRebuild(99) false")
	}
}

// --- SetTimelineSegments ---

func TestTimelineZone_SetTimelineSegments(t *testing.T) {
	rows := []*DrumRow{{Steps: make([]bool, 8)}, {Steps: make([]bool, 8)}}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.EnsureRowCache()

	past := []bool{true, false, true}
	pastTypes := []model.NodeType{model.NodeTypeRegular, model.NodeTypeMute, model.NodeTypeRegular}
	present := []bool{false, true}
	future := []bool{true, true, false, false}

	z.SetTimelineSegments(0, 5, past, pastTypes, present, future)

	if z.TimelineOffset[0] != 5 {
		t.Errorf("expected TimelineOffset[0]=5, got %d", z.TimelineOffset[0])
	}
	if len(z.TimelinePast[0]) != 3 {
		t.Errorf("expected TimelinePast[0] len=3, got %d", len(z.TimelinePast[0]))
	}
	if len(z.TimelinePastTypes[0]) != 3 {
		t.Errorf("expected TimelinePastTypes[0] len=3, got %d", len(z.TimelinePastTypes[0]))
	}
	if len(z.TimelinePresent[0]) != 2 {
		t.Errorf("expected TimelinePresent[0] len=2, got %d", len(z.TimelinePresent[0]))
	}
	if len(z.TimelineFuture[0]) != 4 {
		t.Errorf("expected TimelineFuture[0] len=4, got %d", len(z.TimelineFuture[0]))
	}

	// Out of bounds should not panic.
	z.SetTimelineSegments(-1, 0, nil, nil, nil, nil)
	z.SetTimelineSegments(99, 0, nil, nil, nil, nil)
}

// --- CacheRowSteps ---

func TestTimelineZone_CacheRowSteps(t *testing.T) {
	rows := []*DrumRow{
		{Steps: []bool{true, false, true, false}, CellTypes: []model.NodeType{model.NodeTypeRegular, model.NodeTypeMute, model.NodeTypeRegular, model.NodeTypeMute}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.EnsureRowCache()

	z.CacheRowSteps(0)

	if len(z.RowCacheSteps[0]) != 4 {
		t.Errorf("expected cached steps len=4, got %d", len(z.RowCacheSteps[0]))
	}
	if !z.RowCacheSteps[0][0] || z.RowCacheSteps[0][1] {
		t.Error("cached steps content mismatch")
	}
	if len(z.RowCacheTypes[0]) != 4 {
		t.Errorf("expected cached types len=4, got %d", len(z.RowCacheTypes[0]))
	}

	// Out of bounds should not panic.
	z.CacheRowSteps(-1)
	z.CacheRowSteps(99)
}

// --- SetDrawParams ---

func TestTimelineZone_SetDrawParams(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.Layout(image.Rect(0, 0, 400, 300))

	highlights := [][]highlightEntry{
		{{idx: 0, val: 42}},
	}
	z.SetDrawParams(3.5, highlights)

	if z.elapsedBeats != 3.5 {
		t.Errorf("expected elapsedBeats=3.5, got %f", z.elapsedBeats)
	}
	if len(z.highlightsByRow) != 1 {
		t.Errorf("expected 1 row of highlights, got %d", len(z.highlightsByRow))
	}
}

// --- HandleChars ---

func TestTimelineZone_HandleCharsIgnored(t *testing.T) {
	z, _ := newTestTimelineZone()
	if r := z.HandleChars([]rune{'a'}); r != InputIgnored {
		t.Errorf("expected InputIgnored for chars, got %d", r)
	}
}

// --- SetTimelineBarHeight ---

func TestTimelineZone_SetTimelineBarHeight(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.SetTimelineBarHeight(20)
	z.Layout(image.Rect(0, 0, 400, 300))

	barRect := z.TimelineBarRect()
	if barRect.Dy() != 20 {
		t.Errorf("expected timeline bar height=20, got %d", barRect.Dy())
	}
}

// --- Scrub position clamping ---

func TestTimelineScrubHitAdapter_ScrubTo_Clamp(t *testing.T) {
	var scrubbed int
	z, _ := newTestTimelineZone()
	z.callbacks.OnScrubPosition = func(n int) { scrubbed = n }
	z.callbacks.TimelineBeats = func() int { return 16 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.TimelineUnitsPerBeat = func() int { return 4 }
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &timelineScrubHitAdapter{zone: z}

	// Scrub to far left — should clamp to 0.
	adapter.OnPress(-100, 5)
	if scrubbed < 0 {
		t.Errorf("scrub position should not be negative, got %d", scrubbed)
	}

	// Scrub to far right — should clamp to max.
	z.scrubbing = false
	adapter.OnPress(9999, 5)
	// Should not exceed maxOffSteps.
	maxBeats := 16 - 8/4 // 16 - 2 = 14 beats max offset
	maxSteps := maxBeats * 4
	if scrubbed > maxSteps {
		t.Errorf("scrub position %d exceeds max %d", scrubbed, maxSteps)
	}
}

// --- drawHighlights out of range index ---

func TestTimelineZone_DrawHighlights_OutOfRangeIndex(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	// Highlight with index outside visible range — should be skipped.
	z.highlightsByRow = [][]highlightEntry{
		{{idx: 99, val: 100}},
	}

	dst := ebiten.NewImage(400, 300)
	// Should not panic.
	z.drawHighlights(dst, false)
}

// --- Grid drag adapter: no callbacks set ---

func TestGridDragHitAdapter_OnDrag_NoCallbacks(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.OnOffsetChange = nil
	z.callbacks.OnRowScrollDrag = nil
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	adapter.OnPress(100, 100)
	// Should not panic even with nil callbacks.
	adapter.OnDrag(200, 100) // horizontal drag past deadzone
}

func TestGridDragHitAdapter_OnWheel_NoCallback(t *testing.T) {
	z, _ := newTestTimelineZone()
	z.callbacks.OnRowScrollWheel = nil
	z.Layout(image.Rect(0, 0, 400, 300))

	adapter := &gridDragHitAdapter{zone: z}
	result := adapter.OnWheel(100, 100, 3)

	if result != InputIgnored {
		t.Errorf("expected InputIgnored when OnRowScrollWheel is nil, got %d", result)
	}
}

// --- drawHighlights with row offset ---

func TestTimelineZone_DrawHighlights_WithRowOffset(t *testing.T) {
	rows := []*DrumRow{
		{Instrument: "kick", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Color: color.RGBA{255, 0, 0, 255}},
		{Instrument: "snare", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Color: color.RGBA{0, 255, 0, 255}},
		{Instrument: "hat", Steps: make([]bool, 8), CellTypes: make([]model.NodeType, 8), Color: color.RGBA{0, 0, 255, 255}},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 2 }
	z.callbacks.RowOffset = func() int { return 1 } // Rows 1 and 2 visible.
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.highlightsByRow = [][]highlightEntry{
		{{idx: 0, val: 100}},  // Row 0 — not visible (before rowOffset).
		{{idx: 2, val: 100}},  // Row 1 — visible.
		{{idx: 4, val: 100}},  // Row 2 — visible.
	}

	dst := ebiten.NewImage(400, 300)
	z.drawHighlights(dst, false)
}

// --- drawHighlights Steps length mismatch ---

func TestTimelineZone_DrawHighlights_StepsLengthMismatch(t *testing.T) {
	rows := []*DrumRow{
		{
			Instrument: "kick",
			Steps:      make([]bool, 4), // Mismatched: Steps len=4 but Length=8.
			CellTypes:  make([]model.NodeType, 4),
			Color:      color.RGBA{255, 0, 0, 255},
		},
	}
	z, _ := newTestTimelineZone()
	z.callbacks.Rows = func() []*DrumRow { return rows }
	z.callbacks.VisibleRows = func() int { return 1 }
	z.callbacks.RowHeight = func() int { return 30 }
	z.callbacks.Length = func() int { return 8 }
	z.callbacks.Offset = func() int { return 0 }
	z.Layout(image.Rect(0, 0, 400, 300))

	z.highlightsByRow = [][]highlightEntry{
		{{idx: 2, val: 100}},
	}

	dst := ebiten.NewImage(400, 300)
	// drawHighlights should resize Steps to match length, then render.
	z.drawHighlights(dst, false)

	if len(rows[0].Steps) != 8 {
		t.Errorf("expected Steps resized to 8, got %d", len(rows[0].Steps))
	}
}
