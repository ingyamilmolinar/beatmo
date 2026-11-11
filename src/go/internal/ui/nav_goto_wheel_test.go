//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// The navigation menu's go-to wheel (docs/superpowers/specs/2026-10-03-value-
// wheel-and-text-field-design.md §2): scrolling PREVIEWS (a ghost on the
// timeline, no seek); Go / Enter seek once; Esc / tap-away cancel. Every input
// here goes through the real Game.Update path (makeHoldTap, the shortcut
// router) or the component's own hit handler.

func newGotoGame(t *testing.T) (*Game, *NavigationMenuComponent, func(x, y, frames int)) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	tap := makeHoldTap(g, 1280, 720)
	g.drum.OpenNavigation()
	advanceFrames(g, 2)
	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen) // comp-style menus lay out in Draw
	advanceFrames(g, 1)
	c := g.drum.navMenuComp
	if c == nil || !c.IsOpen() {
		t.Fatal("nav menu did not open")
	}
	return g, c, tap
}

func gotoScrollDown(c *NavigationMenuComponent, notches int) {
	wr := c.Panel().GotoWheelRect()
	x, y := center(wr)
	for i := 0; i < notches*wheelStepEventsPerNotch; i++ {
		c.HandleWheel(x, y, 1)
	}
}

func TestGotoWheelOpensAtThePlayheadWithOneBasedLabels(t *testing.T) {
	g, c, _ := newGotoGame(t)
	p := c.Panel()
	if p.PendingSteps() != g.playheadAbsSubdiv() {
		t.Fatalf("wheel opened at %d, playhead %d", p.PendingSteps(), g.playheadAbsSubdiv())
	}
	if got := p.GotoWheel().model().CenterLabel(); got != "Beat 1" {
		t.Errorf("label at the start = %q, want Beat 1", got)
	}
	if p.GotoWheelRect().Empty() || p.GoRect().Empty() || p.GotoModeRect().Empty() {
		t.Fatalf("go-to controls not laid out: wheel=%v go=%v mode=%v", p.GotoWheelRect(), p.GoRect(), p.GotoModeRect())
	}
}

func TestGotoWheelScrollPreviewsWithoutSeeking(t *testing.T) {
	g, c, _ := newGotoGame(t)
	start := g.playheadAbsSubdiv()
	gotoScrollDown(c, 3)
	advanceFrames(g, 1)
	p := c.Panel()
	if p.PendingSteps() <= start {
		t.Fatalf("scrolling down must raise the pending position: %d", p.PendingSteps())
	}
	if g.playheadAbsSubdiv() != start {
		t.Fatalf("scrolling must NOT seek: playhead %d → %d", start, g.playheadAbsSubdiv())
	}
	step, on := g.drum.timelineZone.GotoGhost()
	if !on || step != p.PendingSteps() {
		t.Fatalf("timeline ghost must mark the pending step: on=%v step=%d pending=%d", on, step, p.PendingSteps())
	}
}

func TestGotoWheelGoSeeksOnceAndStaysOpen(t *testing.T) {
	g, c, tap := newGotoGame(t)
	gotoScrollDown(c, 2)
	want := c.Panel().PendingSteps()
	gx, gy := center(c.Panel().GoRect())
	tap(gx, gy, 3)
	advanceFrames(g, 2)
	if got := g.playheadAbsSubdiv(); got != want {
		t.Fatalf("Go must seek to the pending step %d, playhead at %d", want, got)
	}
	if !c.IsOpen() {
		t.Fatal("the menu stays open after Go (like the jog buttons)")
	}
	if _, on := g.drum.timelineZone.GotoGhost(); on {
		t.Fatal("Go must clear the timeline ghost")
	}
	if c.Panel().PendingSteps() != want {
		t.Fatalf("after Go the wheel re-seats on the new position %d, at %d", want, c.Panel().PendingSteps())
	}
}

func TestGotoWheelEnterSeeks(t *testing.T) {
	g, c, _ := newGotoGame(t)
	gotoScrollDown(c, 1)
	want := c.Panel().PendingSteps()
	pressKeyThroughShortcuts(g, ebiten.KeyEnter)
	advanceFrames(g, 2)
	if got := g.playheadAbsSubdiv(); got != want {
		t.Fatalf("Enter must seek to %d, playhead %d", want, got)
	}
}

func TestGotoWheelEscCancelsWithoutSeeking(t *testing.T) {
	g, c, _ := newGotoGame(t)
	start := g.playheadAbsSubdiv()
	gotoScrollDown(c, 2)
	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	advanceFrames(g, 1)
	restore()
	advanceFrames(g, 1)
	if c.IsOpen() {
		t.Fatal("Esc closes the nav menu")
	}
	if g.playheadAbsSubdiv() != start {
		t.Fatalf("Esc must not seek: %d → %d", start, g.playheadAbsSubdiv())
	}
	if _, on := g.drum.timelineZone.GotoGhost(); on {
		t.Fatal("Esc must clear the timeline ghost")
	}
}

func TestGotoWheelOutsideTapCancelsWithoutSeeking(t *testing.T) {
	g, c, tap := newGotoGame(t)
	start := g.playheadAbsSubdiv()
	gotoScrollDown(c, 2)
	ox, oy := 700, 500
	if image.Pt(ox, oy).In(c.card) {
		ox, oy = 200, 700
	}
	tap(ox, oy, 3)
	advanceFrames(g, 2)
	if c.IsOpen() {
		t.Fatal("an outside tap closes the nav menu")
	}
	if g.playheadAbsSubdiv() != start {
		t.Fatalf("an outside tap must not seek: %d → %d", start, g.playheadAbsSubdiv())
	}
	if _, on := g.drum.timelineZone.GotoGhost(); on {
		t.Fatal("closing must clear the timeline ghost")
	}
}

func TestGotoWheelModeSwitchConvertsThePendingValue(t *testing.T) {
	g, c, tap := newGotoGame(t)
	p := c.Panel()
	gotoScrollDown(c, 12) // 12 beats → step 12*div
	steps := p.PendingSteps()
	div := g.drum.timelineUnitsPerBeat
	bpm := g.drum.BPM()
	tapSegment(tap, p.GotoModeRect(), 1) // Time
	advanceFrames(g, 1)
	if !p.TimeMode() {
		t.Fatal("tapping Time must switch the wheel to time mode")
	}
	wantSec := gotoStepsToSeconds(steps, div, bpm)
	if got := p.GotoWheel().model().CenterLabel(); got != formatClock(wantSec) {
		t.Fatalf("time mode shows %q, want %q", got, formatClock(wantSec))
	}
	tapSegment(tap, p.GotoModeRect(), 0) // back to Beat
	advanceFrames(g, 1)
	if p.TimeMode() {
		t.Fatal("tapping Beat must switch back")
	}
	if got := p.PendingSteps(); got != timeToSteps(float64(wantSec), bpm, div, g.drum.timelineBeats*div) {
		t.Fatalf("back in beat mode at %d", got)
	}
}

// tapSegment taps segment i of a two-segment control laid out over r.
func tapSegment(tap func(x, y, frames int), r image.Rectangle, i int) {
	w := r.Dx() / 2
	tap(r.Min.X+w*i+w/2, (r.Min.Y+r.Max.Y)/2, 3)
}

func TestGotoWheelDoesNotChasePlayhead(t *testing.T) {
	g, c, _ := newGotoGame(t)
	gotoScrollDown(c, 4)
	want := c.Panel().PendingSteps()
	g.drum.RequestSeekSteps(want+3*g.drum.timelineUnitsPerBeat, hooks.SeekCounter) // the playhead moves (playback / jog)
	advanceFrames(g, 3)
	if got := c.Panel().PendingSteps(); got != want {
		t.Fatalf("the wheel must hold the user's position while the playhead moves: %d → %d", want, got)
	}
}

func TestGotoWheelClampsWhenTimelineShrinks(t *testing.T) {
	g, c, _ := newGotoGame(t)
	p := c.Panel()
	p.GotoWheel().model().SetLadderIndex(p.GotoWheel().model().LadderLen() - 1) // 16 beats
	gotoScrollDown(c, 40)
	g.drum.timelineBeats = 8
	advanceFrames(g, 1)
	g.Draw(ebiten.NewImage(1280, 720))
	if max := 8 * g.drum.timelineUnitsPerBeat; p.PendingSteps() > max {
		t.Fatalf("pending %d beyond the shrunk timeline end %d", p.PendingSteps(), max)
	}
}

func TestGotoWheelArrowsStepTheWheelNotTheCamera(t *testing.T) {
	g, c, _ := newGotoGame(t)
	x0, y0 := camOffsets(g)
	before := c.Panel().PendingSteps()
	pressKeyThroughShortcuts(g, ebiten.KeyArrowDown) // Down = value down at the start: clamps
	pressKeyThroughShortcuts(g, ebiten.KeyArrowUp)
	if c.Panel().PendingSteps() <= before {
		t.Fatalf("ArrowUp must raise the pending position: %d → %d", before, c.Panel().PendingSteps())
	}
	if x1, y1 := camOffsets(g); x1 != x0 || y1 != y0 {
		t.Fatalf("arrows must not pan the camera while the go-to wheel is open")
	}
	r0 := c.Panel().GotoWheel().model().LadderIndex()
	pressKeyThroughShortcuts(g, ebiten.KeyArrowRight) // coarser
	if c.Panel().GotoWheel().model().LadderIndex() != r0+1 {
		t.Fatalf("ArrowRight must coarsen the step: rung %d → %d", r0, c.Panel().GotoWheel().model().LadderIndex())
	}
}

func TestGotoWheelSpaceStillTogglesPlay(t *testing.T) {
	g, c, _ := newGotoGame(t)
	_ = c
	was := g.Playing()
	pressKeyThroughShortcuts(g, ebiten.KeySpace)
	advanceFrames(g, 1)
	if g.Playing() == was {
		t.Fatal("Space must still toggle playback while the nav menu is open")
	}
	stopPlaybackForTest(g)
}

func TestClosedNavMenuClaimsNoKeys(t *testing.T) {
	g, c, _ := newGotoGame(t)
	c.Close()
	advanceFrames(g, 1)
	for _, cl := range g.KeyClaimants() {
		if len(cl.ClaimedKeys()) > 0 {
			t.Fatalf("a closed nav menu must claim no keys, got %v from %T", cl.ClaimedKeys(), cl)
		}
	}
}

// Review I2: when the nav card crosses the pane divider it is repainted above
// it — and the jump-size picker stacked ON the card must be repainted after it,
// or the picker is hidden while still taking taps.
func TestNavCardRepaintKeepsTheStackedJumpPickerVisible(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480) // a short drum pane: the nav card crosses the divider
	advanceFrames(g, 2)
	screen := ebiten.NewImage(640, 480)
	g.drum.OpenNavigation()
	advanceFrames(g, 1)
	g.Draw(screen)
	if c := g.drum.navMenuComp.card; c.In(g.drum.Bounds) {
		t.Skipf("nav card %v does not cross the drum pane %v at this size", c, g.drum.Bounds)
	}
	g.drum.OpenJumpMenu()
	advanceFrames(g, 1)
	g.Draw(screen)
	jr := g.drum.jumpMenuComp.Bounds()
	if jr.Empty() {
		t.Fatal("jump picker has no bounds")
	}
	probe := jr.Intersect(g.drum.navMenuComp.card).Inset(4)
	if probe.Empty() {
		t.Skip("jump picker does not overlap the card")
	}
	sample := func() []uint32 {
		var px []uint32
		for y := probe.Min.Y; y < probe.Max.Y; y += 3 {
			for x := probe.Min.X; x < probe.Max.X; x += 3 {
				r, gg, b, _ := screen.At(x, y).RGBA()
				px = append(px, r<<16^gg<<8^b)
			}
		}
		return px
	}
	withPicker := sample()
	g.drum.portal().Close("jump-menu")
	advanceFrames(g, 1)
	g.Draw(screen)
	withoutPicker := sample()
	same := true
	for i := range withPicker {
		if withPicker[i] != withoutPicker[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("the jump picker is invisible: the nav-card repaint covered it")
	}
}
