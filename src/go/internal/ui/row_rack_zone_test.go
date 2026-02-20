//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- RowRackZone test helpers ---

type rowRackTestCallbackLog struct {
	muteToggles   []int
	soloToggles   []int
	deleteRows    []int
	originReqs    []int
	addRows       int
	rowSelects    []int
	volumeChanges []struct {
		row int
		vol float64
	}
	contextMenuOpens []int
	instMenuOpens    []int
	colorWheelOpens  []int
	renameOpens      []int
	fxPanelToggles   []int
	volPopupOpens    []int
	saveInstruments  []int
}

func newTestRowRackZone(rows []*DrumRow) (*RowRackZone, *rowRackTestCallbackLog) {
	log := &rowRackTestCallbackLog{}
	cb := RowRackCallbacks{
		OnMuteToggle: func(row int) {
			log.muteToggles = append(log.muteToggles, row)
		},
		OnSoloToggle: func(row int) {
			log.soloToggles = append(log.soloToggles, row)
		},
		OnDeleteRow: func(row int) {
			log.deleteRows = append(log.deleteRows, row)
		},
		OnOriginReq: func(row int) {
			log.originReqs = append(log.originReqs, row)
		},
		OnAddRow: func() {
			log.addRows++
		},
		OnRowSelect: func(row int) {
			log.rowSelects = append(log.rowSelects, row)
		},
		OnVolumeChange: func(row int, vol float64) {
			log.volumeChanges = append(log.volumeChanges, struct {
				row int
				vol float64
			}{row, vol})
		},
		OnContextMenuOpen: func(row int) {
			log.contextMenuOpens = append(log.contextMenuOpens, row)
		},
		OnInstMenuOpen: func(row int) {
			log.instMenuOpens = append(log.instMenuOpens, row)
		},
		OnColorWheelOpen: func(row int) {
			log.colorWheelOpens = append(log.colorWheelOpens, row)
		},
		OnRenameOpen: func(row int) {
			log.renameOpens = append(log.renameOpens, row)
		},
		OnFXPanelToggle: func(row int) {
			log.fxPanelToggles = append(log.fxPanelToggles, row)
		},
		OnVolPopupOpen: func(row int) {
			log.volPopupOpens = append(log.volPopupOpens, row)
		},
		OnSaveInstrument: func(row int) {
			log.saveInstruments = append(log.saveInstruments, row)
		},
		OnScrollChanged: func() {
			// no-op in isolated zone tests
		},
		Rows:              func() []*DrumRow { return rows },
		IsInstrumentAvail: func(id string) bool { return true },
		RowHeight:         func() int { return TouchRowHeight() },
		DeleteConfirm:     func() (int, int64) { return -1, 0 },
		RenameRow:         func() int { return -1 },
		IsMobileEQMode:    func() bool { return false },
		Frame:             func() int64 { return 0 },
	}
	z := NewRowRackZone(cb)
	return z, log
}

func registerRowRackZone(z *RowRackZone, rect image.Rectangle) *DrumViewTree {
	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z.SetPortal(tree.Portal())
	tree.RegisterZone(z, 120)
	tree.SetZoneRect("row-rack", rect)
	return tree
}

func makeTestRows(n int) []*DrumRow {
	rows := make([]*DrumRow, n)
	for i := range rows {
		rows[i] = &DrumRow{
			Name:       "Row " + string(rune('A'+i)),
			Instrument: "inst" + string(rune('a'+i)),
			Steps:      make([]bool, 8),
			Color:      color.RGBA{200, 100, uint8(50 * i), 255},
			Volume:     1.0,
		}
	}
	return rows
}

// --- Zone interface tests ---

func TestRowRackZoneID(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	if z.ID() != "row-rack" {
		t.Errorf("expected ID 'row-rack', got %q", z.ID())
	}
}

func TestRowRackZoneLayoutSetsGeometry(t *testing.T) {
	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)

	if !z.NeedsLayout() {
		t.Fatal("zone should need layout initially")
	}

	r := image.Rect(0, 0, 300, 400)
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
}

func TestRowRackZone_VolSliderTouchFlag(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	volArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-vol-slider")
	if volArea == nil {
		t.Skip("no volume slider hit area (may be hidden on mobile layout)")
	}
	if !volArea.Touch {
		t.Fatal("row-rack-vol-slider HitArea must have Touch == true for mobile touch expansion")
	}
}

func TestRowRackZoneInvalidate(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 300, 400))

	if z.NeedsLayout() {
		t.Fatal("should not need layout after Layout()")
	}

	z.Invalidate()
	if !z.NeedsLayout() {
		t.Fatal("should need layout after Invalidate()")
	}
}

func TestRowRackZoneMuteButtonCallback(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))

	// Frame 1: layout.
	tree.Update()

	muteArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-mute")
	if muteArea == nil {
		t.Skip("no mute hit area (may be hidden on mobile)")
	}

	mx = (muteArea.Rect.Min.X + muteArea.Rect.Max.X) / 2
	my = (muteArea.Rect.Min.Y + muteArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()

	pressed = false
	tree.Update()

	if len(log.muteToggles) == 0 {
		t.Error("expected OnMuteToggle callback after clicking mute button")
	}
}

func TestRowRackZoneSoloButtonCallback(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	soloArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-solo")
	if soloArea == nil {
		t.Skip("no solo hit area (may be hidden on mobile)")
	}

	mx = (soloArea.Rect.Min.X + soloArea.Rect.Max.X) / 2
	my = (soloArea.Rect.Min.Y + soloArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.soloToggles) == 0 {
		t.Error("expected OnSoloToggle callback after clicking solo button")
	}
}

func TestRowRackZoneFXButtonCallback(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	fxArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-fx")
	if fxArea == nil {
		t.Skip("no fx hit area (may be hidden on mobile)")
	}

	mx = (fxArea.Rect.Min.X + fxArea.Rect.Max.X) / 2
	my = (fxArea.Rect.Min.Y + fxArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.fxPanelToggles) == 0 {
		t.Error("expected OnFXPanelToggle callback after clicking FX button")
	}
}

func TestRowRackZoneLabelClickCallback(t *testing.T) {
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

	rows := makeTestRows(2)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	labelArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-label")
	if labelArea == nil {
		t.Fatal("expected 'row-rack-label' hit area")
	}

	mx = (labelArea.Rect.Min.X + labelArea.Rect.Max.X) / 2
	my = (labelArea.Rect.Min.Y + labelArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	// Desktop: label click opens inst menu.
	if len(log.instMenuOpens) == 0 {
		t.Error("expected OnInstMenuOpen callback after clicking label (desktop)")
	}
}

func TestRowRackZoneDeleteButtonCallback(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	delArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-delete")
	if delArea == nil {
		t.Skip("no delete hit area (may be hidden on mobile)")
	}

	mx = (delArea.Rect.Min.X + delArea.Rect.Max.X) / 2
	my = (delArea.Rect.Min.Y + delArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.deleteRows) == 0 {
		t.Error("expected OnDeleteRow callback after clicking delete button")
	}
}

func TestRowRackZoneVolumeSliderOpensPopup(t *testing.T) {
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

	rows := makeTestRows(2)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	volArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-vol-slider")
	if volArea == nil {
		t.Skip("no volume slider hit area")
	}

	// Find a slider rect that's non-empty.
	sr := z.RowVolSliders()[0].Rect()
	if sr.Empty() {
		t.Skip("slider rect is empty (mobile layout)")
	}

	mx, my = sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2
	pressed = true
	tree.Update()

	// Release.
	pressed = false
	tree.Update()

	// Both desktop and mobile now open the volume popup instead of inline change.
	if len(log.volPopupOpens) == 0 {
		t.Error("expected OnVolPopupOpen callback after slider interaction")
	}
	if len(log.volumeChanges) != 0 {
		t.Error("expected no OnVolumeChange — slider group now opens popup on all platforms")
	}
}

func TestRowRackZoneScrollWheel(t *testing.T) {
	rows := makeTestRows(10) // enough rows to require scroll
	z, _ := newTestRowRackZone(rows)

	// Small rect so not all rows fit.
	r := image.Rect(0, 0, 300, 100)
	z.Layout(r)

	// Sync scroll state.
	z.syncScroll()

	if z.RowOffset() != 0 {
		t.Fatalf("expected initial offset 0, got %d", z.RowOffset())
	}

	scrollArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-scroll")
	if scrollArea == nil {
		t.Fatal("expected 'row-rack-scroll' hit area")
	}

	// Scroll down.
	result := scrollArea.Handler.OnWheel(50, 50, -1)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from scroll wheel, got %d", result)
	}

	if z.RowOffset() <= 0 {
		t.Error("expected row offset to increase after scroll down")
	}
}

func TestRowRackZoneResponsiveLayout(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)

	// Desktop layout.
	desktopRect := image.Rect(0, 0, 400, 300)
	z.Layout(desktopRect)

	muteArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-mute")
	if muteArea == nil {
		t.Error("desktop layout should have 'row-rack-mute' hit area")
	}

	// Mobile layout.
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	zm, _ := newTestRowRackZone(rows)
	mobileRect := image.Rect(0, 0, 300, 200)
	zm.Layout(mobileRect)

	muteMobile := findHitAreaByTagPrefix(zm.HitAreas(), "row-rack-mute")
	if muteMobile != nil {
		t.Error("mobile layout should NOT have 'row-rack-mute' hit area (hidden)")
	}
}

func TestRowRackZoneAddRowFAB(t *testing.T) {
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

	rows := makeTestRows(2)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	addArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-add")
	if addArea == nil {
		t.Fatal("expected 'row-rack-add' hit area")
	}

	mx = (addArea.Rect.Min.X + addArea.Rect.Max.X) / 2
	my = (addArea.Rect.Min.Y + addArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.addRows == 0 {
		t.Error("expected OnAddRow callback after clicking add button")
	}
}

func TestRowRackZoneTreeLifecycle(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))

	tree.Update()

	if z.NeedsLayout() {
		t.Error("zone should have been laid out by tree")
	}

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("zone should have hit areas after layout")
	}

	// Pick the label button — should always be present.
	labelArea := findHitAreaByTagPrefix(areas, "row-rack-label")
	if labelArea == nil {
		t.Fatal("expected row-rack-label hit area")
	}
	cx := (labelArea.Rect.Min.X + labelArea.Rect.Max.X) / 2
	cy := (labelArea.Rect.Min.Y + labelArea.Rect.Max.Y) / 2
	idxAreas := tree.HitIndexRef().At(cx, cy)
	if len(idxAreas) == 0 {
		t.Errorf("expected hit areas at (%d,%d) in hit index after tree update", cx, cy)
	}
}

func TestRowRackZoneHandleKeyIgnored(t *testing.T) {
	rows := makeTestRows(1)
	z, _ := newTestRowRackZone(rows)
	if r := z.HandleKey(ebiten.KeyEnter); r != InputIgnored {
		t.Errorf("expected InputIgnored for key, got %d", r)
	}
	if r := z.HandleChars([]rune{'a'}); r != InputIgnored {
		t.Errorf("expected InputIgnored for chars, got %d", r)
	}
}

// --- Update() tests ---

func TestRowRackZoneUpdateDetectsRowCountChange(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	rowsPtr := &rows
	z, _ := newTestRowRackZone(*rowsPtr)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update() // initial layout

	if z.NeedsLayout() {
		t.Fatal("zone should not need layout right after tree.Update()")
	}

	// Simulate adding a row by updating the callback to return more rows.
	newRows := makeTestRows(4)
	z.callbacks.Rows = func() []*DrumRow { return newRows }

	// Call tree.Update() which invokes zone.Update().
	tree.Update()

	// Update() should detect the row count mismatch and set needLayout.
	if !z.NeedsLayout() {
		t.Error("zone should need layout after row count changed")
	}
}

func TestRowRackZoneUpdateSyncsMuteSoloVisuals(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update() // initial layout + first Update

	// Set Muted and Solo on the underlying rows.
	rows[0].Muted = true
	rows[1].Solo = true

	tree.Update()

	// Verify mute/solo buttons reflect the state.
	muteBtns := z.RowMuteBtns()
	soloBtns := z.RowSoloBtns()
	if len(muteBtns) > 0 && !muteBtns[0].Rect().Empty() {
		if !muteBtns[0].pressed {
			t.Error("expected mute button 0 pressed=true after rows[0].Muted=true")
		}
	}
	if len(soloBtns) > 1 && !soloBtns[1].Rect().Empty() {
		if !soloBtns[1].pressed {
			t.Error("expected solo button 1 pressed=true after rows[1].Solo=true")
		}
	}

	// Unmute and verify sync.
	rows[0].Muted = false
	tree.Update()

	muteBtns = z.RowMuteBtns()
	if len(muteBtns) > 0 && !muteBtns[0].Rect().Empty() {
		if muteBtns[0].pressed {
			t.Error("expected mute button 0 pressed=false after rows[0].Muted=false")
		}
	}
}

// --- VisibleRows() tests ---

func TestRowRackZoneVisibleRowsCalculation(t *testing.T) {
	rows := makeTestRows(10)
	z, _ := newTestRowRackZone(rows)

	// Layout into a rect that can fit about 4 rows.
	rh := TouchRowHeight()
	// Desktop reserves one rowHeight for the "+" footer.
	height := rh*4 + rh // 4 visible + 1 for footer
	z.Layout(image.Rect(0, 0, 400, height))

	vis := z.VisibleRows()
	if vis != 4 {
		t.Errorf("expected 4 visible rows (height=%d, rowH=%d), got %d", height, rh, vis)
	}
}

func TestRowRackZoneVisibleRowsOverride(t *testing.T) {
	rows := makeTestRows(10)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	// Set override.
	z.SetVisibleRowsOverride(7)
	if vis := z.VisibleRows(); vis != 7 {
		t.Errorf("expected VisibleRows()=7 with override, got %d", vis)
	}

	// Clear override.
	z.SetVisibleRowsOverride(0)
	if vis := z.VisibleRows(); vis == 7 {
		t.Error("expected VisibleRows() to not be 7 after clearing override")
	}
}

func TestRowRackZoneVisibleRowsZeroHeight(t *testing.T) {
	rows := makeTestRows(5)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 0))

	if vis := z.VisibleRows(); vis != 0 {
		t.Errorf("expected VisibleRows()=0 with zero height, got %d", vis)
	}
}

// --- syncScroll() / flushScroll() tests ---

func TestRowRackZoneSyncScrollSetsState(t *testing.T) {
	rows := makeTestRows(10)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 300, 200))

	z.syncScroll()

	if z.rowScroll.VS.Total != 10 {
		t.Errorf("expected scroll total=10, got %d", z.rowScroll.VS.Total)
	}
	if z.rowScroll.VS.First != z.rowOffset {
		t.Errorf("expected scroll first=%d, got %d", z.rowOffset, z.rowScroll.VS.First)
	}
	if z.rowScroll.VS.Visible != z.VisibleRows() {
		t.Errorf("expected scroll visible=%d, got %d", z.VisibleRows(), z.rowScroll.VS.Visible)
	}
}

func TestRowRackZoneFlushScrollUpdatesOffset(t *testing.T) {
	rows := makeTestRows(10)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 300, 200))

	z.syncScroll()

	// Simulate the scroll behavior moving First.
	z.rowScroll.VS.First = 3
	z.flushScroll()

	if z.RowOffset() != 3 {
		t.Errorf("expected row offset 3 after flushScroll, got %d", z.RowOffset())
	}
	if !z.NeedsLayout() {
		t.Error("expected needLayout after flushScroll changed offset")
	}
}

func TestRowRackZoneFlushScrollNoChangeNoLayout(t *testing.T) {
	rows := makeTestRows(5)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 300, 200))

	z.syncScroll()
	// First is already 0 == rowOffset.
	z.flushScroll()

	if z.NeedsLayout() {
		t.Error("flushScroll should not set needLayout when offset is unchanged")
	}
}

// --- Accessor tests ---

func TestRowRackZoneSetRowOffsetAndGet(t *testing.T) {
	rows := makeTestRows(5)
	z, _ := newTestRowRackZone(rows)

	z.SetRowOffset(2)
	if z.RowOffset() != 2 {
		t.Errorf("expected RowOffset()=2, got %d", z.RowOffset())
	}
}

func TestRowRackZoneSetSelRowAndGet(t *testing.T) {
	rows := makeTestRows(5)
	z, _ := newTestRowRackZone(rows)

	z.SetSelRow(3)
	if z.SelRow() != 3 {
		t.Errorf("expected SelRow()=3, got %d", z.SelRow())
	}
}

func TestRowRackZoneMarkDirty(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	// Clear the dirty flag.
	z.controlsCacheDirty = false
	z.MarkDirty()

	if !z.controlsCacheDirty {
		t.Error("MarkDirty() should set controlsCacheDirty=true")
	}
}

func TestRowRackZoneSetScreenBounds(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)

	sb := image.Rect(0, 0, 1920, 1080)
	z.SetScreenBounds(sb)

	if z.screenBounds != sb {
		t.Errorf("expected screenBounds %v, got %v", sb, z.screenBounds)
	}
}

func TestRowRackZoneAccessors(t *testing.T) {
	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	if len(z.RowLabels()) != 3 {
		t.Errorf("expected 3 row labels, got %d", len(z.RowLabels()))
	}
	if len(z.RowEditBtns()) != 3 {
		t.Errorf("expected 3 edit buttons, got %d", len(z.RowEditBtns()))
	}
	if len(z.RowSaveBtns()) != 3 {
		t.Errorf("expected 3 save buttons, got %d", len(z.RowSaveBtns()))
	}
	if len(z.RowColorBtns()) != 3 {
		t.Errorf("expected 3 color buttons, got %d", len(z.RowColorBtns()))
	}
	if len(z.RowDeleteBtns()) != 3 {
		t.Errorf("expected 3 delete buttons, got %d", len(z.RowDeleteBtns()))
	}
	if len(z.RowVolSliders()) != 3 {
		t.Errorf("expected 3 vol sliders, got %d", len(z.RowVolSliders()))
	}
	if len(z.RowOriginBtns()) != 3 {
		t.Errorf("expected 3 origin buttons, got %d", len(z.RowOriginBtns()))
	}
	if len(z.RowMuteBtns()) != 3 {
		t.Errorf("expected 3 mute buttons, got %d", len(z.RowMuteBtns()))
	}
	if len(z.RowSoloBtns()) != 3 {
		t.Errorf("expected 3 solo buttons, got %d", len(z.RowSoloBtns()))
	}
	if len(z.RowMenuBtns()) != 3 {
		t.Errorf("expected 3 menu buttons, got %d", len(z.RowMenuBtns()))
	}
	if len(z.RowFXBtns()) != 3 {
		t.Errorf("expected 3 FX buttons, got %d", len(z.RowFXBtns()))
	}
	if len(z.RowGroups()) != 3 {
		t.Errorf("expected 3 row groups, got %d", len(z.RowGroups()))
	}
	if z.AddRowButton() == nil {
		t.Error("expected non-nil AddRowButton()")
	}
	if z.RowScroll() == nil {
		t.Error("expected non-nil RowScroll()")
	}
	if z.RowVolGroup() == nil {
		t.Error("expected non-nil RowVolGroup()")
	}
}

// --- repositionEntries (same row count) ---

func TestRowRackZoneRepositionOnRelayout(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// Grab initial label text.
	if len(z.RowLabels()) < 1 {
		t.Fatal("expected at least 1 label")
	}
	origLabel := z.RowLabels()[0].Text

	// Change the row name in the underlying data.
	rows[0].Name = "Renamed"

	// Force re-layout (same row count, so repositionEntries is used).
	z.Invalidate()
	tree.Update()

	if z.RowLabels()[0].Text != "Renamed" {
		t.Errorf("expected label updated to 'Renamed', got %q (was %q)", z.RowLabels()[0].Text, origLabel)
	}
}

// --- controlsCacheValid() coverage ---

func TestRowRackZoneControlsCacheValidInitially(t *testing.T) {
	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	// Cache should be dirty after initial layout.
	if z.controlsCacheValid() {
		t.Error("controls cache should not be valid after initial layout (dirty flag set)")
	}
}

func TestRowRackZoneComputeControlsBoundsEmpty(t *testing.T) {
	rows := makeTestRows(0)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	bounds := z.computeControlsBounds()
	if !bounds.Empty() {
		t.Errorf("expected empty bounds for 0 rows, got %v", bounds)
	}
}

func TestRowRackZoneComputeControlsBoundsNonEmpty(t *testing.T) {
	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	bounds := z.computeControlsBounds()
	if bounds.Empty() {
		t.Error("expected non-empty bounds for 3 rows")
	}
}

// --- P2 gap tests ---

func TestRowRack_DeleteButtonDisabledSingleRow(t *testing.T) {
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

	rows := makeTestRows(1)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// With a single row, the delete button should have no OnClick and
	// should use DisabledButtonStyle.
	delBtns := z.RowDeleteBtns()
	if len(delBtns) != 1 {
		t.Fatalf("expected 1 delete button, got %d", len(delBtns))
	}
	if delBtns[0].OnClick != nil {
		t.Error("expected delete button OnClick to be nil for single row")
	}
	if delBtns[0].Style != DisabledButtonStyle {
		t.Error("expected delete button to use DisabledButtonStyle for single row")
	}

	// Even if we click the delete area, no callback should fire.
	delArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-delete")
	if delArea != nil && !delArea.Rect.Empty() {
		mx = (delArea.Rect.Min.X + delArea.Rect.Max.X) / 2
		my = (delArea.Rect.Min.Y + delArea.Rect.Max.Y) / 2
		pressed = true
		tree.Update()
		pressed = false
		tree.Update()
	}

	if len(log.deleteRows) != 0 {
		t.Errorf("expected 0 delete callbacks for single row, got %d", len(log.deleteRows))
	}
}

func TestRowRack_DeleteConfirmVisualTiming(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	confirmRow := -1
	var confirmFrame int64

	z, _ := newTestRowRackZone(rows)
	z.callbacks.DeleteConfirm = func() (int, int64) {
		return confirmRow, confirmFrame
	}
	var currentFrame int64
	z.callbacks.Frame = func() int64 {
		return currentFrame
	}

	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// Simulate delete confirm active on row 1 at frame 50.
	confirmRow = 1
	confirmFrame = 50
	currentFrame = 60 // within 120-frame window

	// Force re-draw by marking dirty and triggering cache rebuild.
	z.MarkDirty()
	screen := ebiten.NewImage(800, 600)
	z.Draw(screen)

	// After drawing with delete confirm active within the window,
	// the delete button for row 1 should show "!!" with danger style.
	delBtns := z.RowDeleteBtns()
	if len(delBtns) < 2 {
		t.Fatal("expected at least 2 delete buttons")
	}
	if delBtns[1].Text != "!!" {
		t.Errorf("expected delete button text '!!' during confirm, got %q", delBtns[1].Text)
	}
	if delBtns[1].Style != DeleteConfirmButtonStyle {
		t.Error("expected DeleteConfirmButtonStyle during confirm window")
	}

	// Now advance past the 120-frame window.
	currentFrame = 200 // 200 - 50 = 150 > 120

	z.MarkDirty()
	z.Draw(screen)

	// After the confirm window expires, the button should revert.
	if delBtns[1].Text != "X" {
		t.Errorf("expected delete button text 'X' after confirm expires, got %q", delBtns[1].Text)
	}
	if delBtns[1].Style == DeleteConfirmButtonStyle {
		t.Error("expected non-DeleteConfirmButtonStyle after confirm window expires")
	}
}

func TestRowRack_LabelSkippedDuringRename(t *testing.T) {
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

	rows := makeTestRows(3)
	renameIdx := -1
	z, log := newTestRowRackZone(rows)
	z.callbacks.RenameRow = func() int { return renameIdx }

	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// Verify the label click works normally when no rename is active.
	labelArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-label")
	if labelArea == nil {
		t.Fatal("expected row-rack-label hit area")
	}

	mx = (labelArea.Rect.Min.X + labelArea.Rect.Max.X) / 2
	my = (labelArea.Rect.Min.Y + labelArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.instMenuOpens) == 0 {
		t.Fatal("expected inst menu to open when rename is not active")
	}

	// Now activate rename for row 0.
	renameIdx = 0

	// Force re-layout to rebuild hit areas with rename active.
	z.Invalidate()
	tree.Update()

	// The label hit area for row 0 should now be excluded from hit areas
	// because rebuildHitAreas skips it when renameRow == i.
	// Clear the log and try clicking where the label was.
	log.instMenuOpens = nil
	log.contextMenuOpens = nil

	// After invalidation, the label for the renamed row should be excluded.
	// Find any label areas and click the first one we see.
	labelArea2 := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-label")
	if labelArea2 != nil {
		// If there's still a label area, it should be for a different row.
		// Click it to verify it works for non-renamed rows.
		mx = (labelArea2.Rect.Min.X + labelArea2.Rect.Max.X) / 2
		my = (labelArea2.Rect.Min.Y + labelArea2.Rect.Max.Y) / 2
		pressed = true
		tree.Update()
		pressed = false
		tree.Update()

		// The callback should still fire for non-renamed rows.
		if len(log.instMenuOpens) == 0 {
			t.Error("expected inst menu to open for non-renamed row label click")
		}
	}

	// Additionally, verify the label OnClick itself guards against rename.
	// Manually invoke the label's OnClick for row 0 to confirm it no-ops.
	labels := z.RowLabels()
	log.instMenuOpens = nil
	log.contextMenuOpens = nil
	if labels[0].OnClick != nil {
		labels[0].OnClick()
	}
	if len(log.instMenuOpens) != 0 || len(log.contextMenuOpens) != 0 {
		t.Error("expected label OnClick to be no-op when rename is active for that row")
	}
}

func TestRowRack_MobileLayoutHidesControls(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	z.Layout(image.Rect(0, 0, 400, 300))

	// On mobile, mute/solo/FX/origin/delete/edit/save/color buttons should
	// all have empty rects (hidden behind context menu).
	for i, e := range z.entries {
		if !e.muteBtn.Rect().Empty() {
			t.Errorf("row %d: mute button should have empty rect on mobile", i)
		}
		if !e.soloBtn.Rect().Empty() {
			t.Errorf("row %d: solo button should have empty rect on mobile", i)
		}
		if !e.fxBtn.Rect().Empty() {
			t.Errorf("row %d: FX button should have empty rect on mobile", i)
		}
		if !e.originBtn.Rect().Empty() {
			t.Errorf("row %d: origin button should have empty rect on mobile", i)
		}
		if !e.deleteBtn.Rect().Empty() {
			t.Errorf("row %d: delete button should have empty rect on mobile", i)
		}
		if !e.editBtn.Rect().Empty() {
			t.Errorf("row %d: edit button should have empty rect on mobile", i)
		}
		if !e.saveBtn.Rect().Empty() {
			t.Errorf("row %d: save button should have empty rect on mobile", i)
		}
		if !e.colorBtn.Rect().Empty() {
			t.Errorf("row %d: color button should have empty rect on mobile", i)
		}
	}
}

func TestRowRack_ColorButtonCallback(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	colorArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-color")
	if colorArea == nil {
		t.Skip("no color hit area (may be hidden on mobile)")
	}

	mx = (colorArea.Rect.Min.X + colorArea.Rect.Max.X) / 2
	my = (colorArea.Rect.Min.Y + colorArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.colorWheelOpens) == 0 {
		t.Error("expected OnColorWheelOpen callback after clicking color button")
	}
}

func TestRowRack_EditSaveButtonCallbacks(t *testing.T) {
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

	rows := makeTestRows(3)
	z, log := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 400, 300))
	tree.Update()

	// Test edit button.
	editArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-edit")
	if editArea == nil {
		t.Skip("no edit hit area (may be hidden on mobile)")
	}

	mx = (editArea.Rect.Min.X + editArea.Rect.Max.X) / 2
	my = (editArea.Rect.Min.Y + editArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.renameOpens) == 0 {
		t.Error("expected OnRenameOpen callback after clicking edit button")
	}

	// Test save button.
	saveArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-save")
	if saveArea == nil {
		t.Skip("no save hit area (may be hidden on mobile)")
	}

	mx = (saveArea.Rect.Min.X + saveArea.Rect.Max.X) / 2
	my = (saveArea.Rect.Min.Y + saveArea.Rect.Max.Y) / 2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if len(log.saveInstruments) == 0 {
		t.Error("expected OnSaveInstrument callback after clicking save button")
	}
}

func TestRowRack_AddRowFABMobilePosition(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	rows := makeTestRows(2)
	z, _ := newTestRowRackZone(rows)

	// Set screen bounds wider than the rack panel.
	screenBounds := image.Rect(0, 0, 1024, 768)
	z.SetScreenBounds(screenBounds)

	// Layout with a narrower rack panel.
	rackRect := image.Rect(0, 0, 200, 400)
	z.Layout(rackRect)

	addBtn := z.AddRowButton()
	if addBtn == nil {
		t.Fatal("expected non-nil AddRowButton")
	}
	addRect := addBtn.Rect()
	if addRect.Empty() {
		t.Fatal("expected non-empty add button rect on mobile")
	}

	// On mobile, the FAB should use screenBounds.Max.X for positioning.
	// The FAB X position should be: screenBounds.Max.X - btnW - 20.
	// It should NOT be constrained to the narrow rackRect.Max.X.
	if addRect.Max.X <= rackRect.Max.X {
		t.Errorf("expected FAB to extend beyond rack panel (using screen bounds); "+
			"addRect.Max.X=%d, rackRect.Max.X=%d, screenBounds.Max.X=%d",
			addRect.Max.X, rackRect.Max.X, screenBounds.Max.X)
	}

	// Verify FAB is positioned relative to the screen bounds, not the panel.
	expectedFabMaxX := screenBounds.Max.X
	if addRect.Max.X > expectedFabMaxX {
		t.Errorf("expected FAB max X <= screenBounds.Max.X=%d, got %d",
			expectedFabMaxX, addRect.Max.X)
	}
}

// --- FAB bounds tests ---

func TestAddRowBtnStaysInBoundsAfterScroll(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)

	screenBounds := image.Rect(0, 0, 400, 600)
	z.SetScreenBounds(screenBounds)

	rackRect := image.Rect(0, 0, 120, 600)
	tree := registerRowRackZone(z, rackRect)
	tree.SetBounds(screenBounds)
	tree.Update()

	// Scroll down partway.
	z.SetRowOffset(5)
	z.Invalidate()
	tree.Update()

	addRect := z.AddRowButton().Rect()
	if addRect.Empty() {
		t.Fatal("expected non-empty add button rect after scroll")
	}
	if !addRect.In(screenBounds) {
		t.Errorf("FAB rect %v is outside screenBounds %v after scroll", addRect, screenBounds)
	}
	if addRect.Min.Y < rackRect.Min.Y+z.rowHeight() {
		t.Errorf("FAB Y=%d jumped to top area (below rowsTop=%d expected)", addRect.Min.Y, rackRect.Min.Y+z.rowHeight())
	}
}

func TestAddRowBtnClampedToScreenBounds(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)

	screenBounds := image.Rect(0, 0, 400, 600)
	z.SetScreenBounds(screenBounds)

	// Layout with an empty rect (simulating transient state).
	z.Layout(image.Rectangle{})

	addRect := z.AddRowButton().Rect()
	// When panelRect is empty, the button should be empty (not positioned at top-right).
	if !addRect.Empty() {
		t.Errorf("FAB rect should be empty when panelRect is empty, got %v", addRect)
	}
}

func TestAddRowBtnWithinBoundsAfterMomentumScroll(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)

	screenBounds := image.Rect(0, 0, 400, 600)
	z.SetScreenBounds(screenBounds)

	rackRect := image.Rect(0, 0, 120, 600)
	tree := registerRowRackZone(z, rackRect)
	tree.SetBounds(screenBounds)
	tree.Update()

	// Simulate momentum scroll through various offsets.
	for _, off := range []int{0, 3, 7, 12, 15} {
		z.SetRowOffset(off)
		z.Invalidate()
		tree.Update()

		addRect := z.AddRowButton().Rect()
		if addRect.Empty() {
			continue // acceptable during transient states
		}
		if !addRect.In(screenBounds) {
			t.Errorf("offset=%d: FAB rect %v is outside screenBounds %v", off, addRect, screenBounds)
		}
	}
}

func TestRowRack_MobileEQModeHidesAll(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	rows := makeTestRows(3)
	z, _ := newTestRowRackZone(rows)
	z.callbacks.IsMobileEQMode = func() bool { return true }

	z.Layout(image.Rect(0, 0, 400, 300))

	// VisibleRows should return 0 when mobile EQ mode is active
	// because rebuildEntries sets vis=0 in that case.
	// The Draw method returns early when IsMobileEQMode is true.
	screen := ebiten.NewImage(800, 600)
	z.Draw(screen) // should be a no-op, no panic

	// Verify the add row button is hidden (empty rect) in mobile EQ mode.
	addBtn := z.AddRowButton()
	if addBtn != nil && !addBtn.Rect().Empty() {
		t.Error("expected add row button to have empty rect in mobile EQ mode")
	}
}
