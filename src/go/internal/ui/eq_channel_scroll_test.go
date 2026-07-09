//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestEQChannelMenuWheelScroll verifies that mouse wheel events scroll the
// EQ channel dropdown when it has more items than fit in the visible area.
// This is a regression test: after the portal refactoring,
// wheel events stopped reaching the EQ channel menu because:
//   - The legacy Update path had no wheel handling for eqChannelOpen
//   - The portal path's buttonHitAdapter returns InputIgnored for wheel
//   - The tree skips wheel dispatch when popupActive() returns true
func TestEQChannelMenuWheelScroll(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	// Add enough rows to overflow the visible limit (eqChannelMenuMaxVisibleRows = 8).
	// Total items = 1 (Master) + len(Rows). Need total > 8 → need ≥ 9 rows.
	dv.Rows = make([]*DrumRow, 10)
	for i := range dv.Rows {
		dv.Rows[i] = &DrumRow{
			Name:       "Inst" + string(rune('A'+i)),
			Instrument: "inst_" + string(rune('a'+i)),
			Steps:      make([]bool, 8),
			Volume:     1.0,
		}
	}
	dv.recalcButtons()
	dv.calcLayout()

	// Open the EQ channel menu via the zone's channel button (creates portal entry).
	dv.eqPanelZone.stickyBar.ChannelBtn().OnClick()

	// Use the zone's channelScroll (portal path).
	scroll := dv.eqPanelZone.channelScroll
	if !scroll.HasScroll() {
		t.Fatal("channelScroll should have scroll with 11 items (1 Master + 10 rows)")
	}

	initialFirst := scroll.VS.First
	if initialFirst != 0 {
		t.Fatalf("initial scroll position should be 0, got %d", initialFirst)
	}

	// Simulate a wheel-down event (scroll down = negative wy = step -1).
	menuRect := scroll.VS.View
	mx := menuRect.Min.X + menuRect.Dx()/2
	my := menuRect.Min.Y + menuRect.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, -1 }, // scroll down
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	newFirst := scroll.VS.First
	if newFirst <= initialFirst {
		t.Fatalf("wheel scroll down should increase VS.First: before=%d, after=%d",
			initialFirst, newFirst)
	}

	// The menu wheel is clicky (one item per notch + cooldown), matching every
	// other menu. Advance the cooldown before the opposite-direction notch —
	// otherwise it is (correctly) locked out within the same cooldown window.
	for i := 0; i < controlGridScrollCooldownFrames; i++ {
		dv.tickMenuScrollCooldowns()
	}

	// Scroll back up.
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 1 }, // scroll up
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	if scroll.VS.First != initialFirst {
		t.Fatalf("wheel scroll up should return to initial: want=%d, got=%d",
			initialFirst, scroll.VS.First)
	}
}

// --- Portal-path tests for EQ channel dropdown ---

// makeOverflowRows creates enough DrumRows to overflow the channel dropdown.
func makeOverflowRows(n int) []*DrumRow {
	rows := make([]*DrumRow, n)
	for i := range rows {
		rows[i] = &DrumRow{
			Name:       "Inst" + string(rune('A'+i)),
			Instrument: "inst_" + string(rune('a'+i)),
			Steps:      make([]bool, 8),
			Volume:     1.0,
		}
	}
	return rows
}

// clickAndRelease simulates a press→release on the tree, clearing the global
// suppress flag that DrumView.Update() would normally clear.
func clickAndRelease(t *testing.T, tree *DrumViewTree, x, y int, restore func(),
	setMx func(int), setMy func(int), setPressed func(bool),
) {
	t.Helper()
	setMx(x)
	setMy(y)
	setPressed(true)
	tree.Update()
	setPressed(false)
	tree.Update()
	// DrumView.Update() clears suppressClicksUntilRelease on mouse-up (line 22-24).
	// Portal-only tests must do this manually.
	suppressClicksUntilRelease = false
}

// TestEQChannelDropdownPortalWheelScroll opens the channel dropdown via
// EQPanelZone with >8 rows, simulates wheel through tree.Update() (not
// DrumView.Update), and verifies the scroll position changes.
func TestEQChannelDropdownPortalWheelScroll(t *testing.T) {
	rows := makeOverflowRows(10) // 11 total items (1 Master + 10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	// Layout frame with no input.
	restore := noInputForTest()
	tree.Update()
	restore()

	// Click the channel button to open the dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	var wy float64
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected eq-channel-dropdown portal to be open")
	}

	// Verify the dropdown has scroll (>8 items).
	if !z.channelScroll.HasScroll() {
		t.Fatal("channelScroll should have scroll with 11 items")
	}
	initialFirst := z.channelScroll.VS.First

	// Simulate wheel-down through tree. Position cursor over dropdown.
	menuRect := z.channelScroll.VS.View
	mx = menuRect.Min.X + menuRect.Dx()/2
	my = menuRect.Min.Y + menuRect.Dy()/2

	wy = -1 // scroll down
	tree.Update()
	wy = 0

	if z.channelScroll.VS.First <= initialFirst {
		t.Fatalf("wheel through tree should scroll: before=%d, after=%d",
			initialFirst, z.channelScroll.VS.First)
	}
}

// TestEQChannelDropdownPortalTouchScroll opens the dropdown, simulates
// press→drag→release through the tree, and verifies scroll position changes.
func TestEQChannelDropdownPortalTouchScroll(t *testing.T) {
	rows := makeOverflowRows(10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected dropdown to be open")
	}
	if !z.channelScroll.HasScroll() {
		t.Fatal("should have scroll")
	}

	initialFirst := z.channelScroll.VS.First

	// Press inside dropdown, drag UP to scroll down (VS.First increases).
	// Touch UX: drag up → see items further down the list.
	menuRect := z.channelScroll.VS.View
	startY := menuRect.Min.Y + menuRect.Dy()/2
	mx = menuRect.Min.X + menuRect.Dx()/2
	my = startY
	pressed = true
	tree.Update() // press

	// Drag up by enough pixels to scroll (past dead zone + item height).
	for dy := 4; dy < 80; dy += 4 {
		my = startY - dy
		tree.Update()
	}

	// Release.
	pressed = false
	tree.Update()
	suppressClicksUntilRelease = false

	if z.channelScroll.VS.First <= initialFirst {
		t.Fatalf("touch scroll should change First: before=%d, after=%d",
			initialFirst, z.channelScroll.VS.First)
	}
}

// TestEQChannelDropdownPortalDeferredTap opens the dropdown, simulates a
// quick press→release (no movement), and verifies the button fires
// (selected channel changes).
func TestEQChannelDropdownPortalDeferredTap(t *testing.T) {
	rows := makeOverflowRows(3) // 4 total items; no scroll overflow
	z, log := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected dropdown to be open")
	}

	// Quick tap on the second button (first row instrument).
	// The dropdown buttons are positioned below the channel button anchor.
	anchor := z.stickyBar.ChannelBtn().Rect()
	btnH := 24                           // default dropdown button height
	tapY := anchor.Max.Y + btnH + btnH/2 // center of second button
	tapX := (anchor.Min.X + anchor.Max.X) / 2

	// Press and release (deferred tap should fire on release).
	mx, my = tapX, tapY
	pressed = true
	tree.Update() // press → capture
	pressed = false
	tree.Update() // release → fire deferred tap
	suppressClicksUntilRelease = false

	// Channel should have changed (any channel change callback fired).
	if len(log.channelChanges) == 0 {
		t.Fatal("expected channel change after tap on dropdown item")
	}
}

// TestEQChannelDropdownPortalClickOutside opens the dropdown, simulates
// a click outside the menu rect, and verifies the portal entry is removed.
func TestEQChannelDropdownPortalClickOutside(t *testing.T) {
	rows := makeOverflowRows(3)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected dropdown to be open")
	}

	// Click far outside the dropdown.
	mx, my = 5, 5
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()
	suppressClicksUntilRelease = false

	if tree.Portal().Has("eq-channel-dropdown") {
		t.Error("expected dropdown closed after click outside")
	}
}

// TestEQChannelDropdownMomentumScroll opens the dropdown, simulates a touch
// drag then release with velocity, and verifies scroll continues via momentum.
func TestEQChannelDropdownMomentumScroll(t *testing.T) {
	rows := makeOverflowRows(10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected dropdown to be open")
	}

	// Touch drag UP with velocity inside dropdown (scroll down: VS.First increases).
	menuRect := z.channelScroll.VS.View
	startY := menuRect.Min.Y + menuRect.Dy()/2
	mx = menuRect.Min.X + menuRect.Dx()/2
	my = startY
	pressed = true
	tree.Update() // press

	// Fast drag up.
	for dy := 4; dy < 100; dy += 8 {
		my = startY - dy
		tree.Update()
	}

	// Release.
	pressed = false
	tree.Update()
	suppressClicksUntilRelease = false

	firstAfterRelease := z.channelScroll.VS.First

	// Run a few more frames for momentum.
	for i := 0; i < 10; i++ {
		tree.Update()
	}

	// Momentum should have continued scrolling (or at least not regressed).
	if z.channelScroll.VS.First < firstAfterRelease {
		t.Fatalf("expected momentum to not regress: afterRelease=%d, afterMomentum=%d",
			firstAfterRelease, z.channelScroll.VS.First)
	}
}

// TestEQChannelDropdownVisibleRowsCappedByAvailablePixels exercises the
// pixel-aware overflow rule: when the channel button is anchored near the
// bottom of the viewport so that fewer than eqChannelMenuMaxVisibleRows of
// 24px-tall items fit beneath it, the dropdown must shrink its visible-row
// count to whatever fits AND show a scrollbar — even when total items is
// well below the constant cap.
//
// Today buildButtons only honours eqChannelMenuMaxVisibleRows = 8 and has no
// view of screenBounds; the menu runs off the bottom of the screen and the
// scrollbar never appears in this regime.
func TestEQChannelDropdownVisibleRowsCappedByAvailablePixels(t *testing.T) {
	// 6 rows → 7 total items (Master + 6). Below the 8-item constant cap, so
	// only the new pixel-aware rule can introduce a scrollbar.
	rows := makeOverflowRows(6)
	z, _ := newTestEQPanelZone(rows)

	// Tight 800x300 viewport with the EQ panel hugging the bottom: anchor
	// sits at panel.Min.Y, so only ~3 rows of 24px fit before bottom.
	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 300))
	z.SetPortal(tree.Portal())
	tree.RegisterZone(z, 130)
	tree.SetZoneRect("eq-panel", image.Rect(0, 200, 600, 300))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open the dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected eq-channel-dropdown portal to be open")
	}

	scroll := z.channelScroll
	anchor := z.stickyBar.ChannelBtn().Rect()
	availPx := 300 - anchor.Max.Y // viewport bottom minus button bottom
	if availPx <= 0 {
		t.Fatalf("setup: anchor.Max.Y=%d should leave space below in 0..300 viewport", anchor.Max.Y)
	}
	availRows := availPx / scroll.ItemHeight
	if availRows >= 7 {
		t.Fatalf("setup: viewport too tall — %d rows fit (need < total items=7) so the pixel cap can't engage; anchor.Max.Y=%d availPx=%d",
			availRows, anchor.Max.Y, availPx)
	}

	// Total = 7 (1 Master + 6 rows). Visible must be the smaller of
	// (eqChannelMenuMaxVisibleRows=8, availRows, total=7) → availRows.
	if scroll.VS.Total != 7 {
		t.Fatalf("scroll.VS.Total = %d, want 7", scroll.VS.Total)
	}
	if scroll.VS.Visible != availRows {
		t.Fatalf("scroll.VS.Visible = %d, want %d (pixel-cap availRows)", scroll.VS.Visible, availRows)
	}
	if !scroll.HasScroll() {
		t.Fatalf("HasScroll() = false; want true because Total(%d) > Visible(%d)", scroll.VS.Total, scroll.VS.Visible)
	}
	if scroll.BarRect().Empty() {
		t.Error("scrollbar BarRect must be non-empty when HasScroll() is true")
	}
	if scroll.ThumbRect().Empty() {
		t.Error("scrollbar ThumbRect must be non-empty when HasScroll() is true")
	}
	// View must fit inside the viewport (not overflow the bottom).
	if scroll.VS.View.Max.Y > 300 {
		t.Errorf("scroll.VS.View.Max.Y = %d overflows viewport bottom (300)", scroll.VS.View.Max.Y)
	}
}

// TestEQChannelDropdownScrollbarPresent opens the dropdown with >8 items
// and verifies HasScroll() is true and scrollbar geometry is non-empty.
func TestEQChannelDropdownScrollbarPresent(t *testing.T) {
	rows := makeOverflowRows(10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected eq-channel-btn hit area")
	}
	cx, cy := (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2

	var mx, my int
	var pressed bool
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	clickAndRelease(t, tree, cx, cy,
		restore,
		func(v int) { mx = v }, func(v int) { my = v }, func(v bool) { pressed = v })

	if !tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("expected dropdown to be open")
	}
	if !z.channelScroll.HasScroll() {
		t.Fatal("expected HasScroll() == true with 11 items")
	}
	bar := z.channelScroll.BarRect()
	if bar.Empty() {
		t.Error("scrollbar BarRect should be non-empty")
	}
	thumb := z.channelScroll.ThumbRect()
	if thumb.Empty() {
		t.Error("scrollbar ThumbRect should be non-empty")
	}
}
