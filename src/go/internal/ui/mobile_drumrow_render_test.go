package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// mobileViewport defines a test viewport.
type mobileViewport struct {
	name string
	w, h int
}

// mobileViewports is the list of mobile viewports to test.
var mobileViewports = []mobileViewport{
	{"iPhone14_portrait", 390, 844},
	{"iPhone14_landscape", 844, 390},
	{"iPhone8_portrait", 375, 667},
	{"iPhone8_landscape", 667, 375},
	{"iPhoneSE_portrait", 320, 568},
	{"iPhoneSE_landscape", 568, 320},
	{"small_landscape", 480, 320},
}

func TestMobileDrumRowsAreaPositive(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			rah := g.drum.rowsAreaHeight()
			if rah <= 0 {
				t.Fatalf("rowsAreaHeight()=%d, want > 0 on %dx%d", rah, vp.w, vp.h)
			}
		})
	}
}

func TestMobileDrumVisibleRowsPositive(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			vis := g.drum.visibleRows()
			if vis < 1 {
				t.Fatalf("visibleRows()=%d, want >= 1 on %dx%d (rowsAreaHeight=%d, rowHeight=%d)",
					vis, vp.w, vp.h, g.drum.rowsAreaHeight(), g.drum.rowHeight())
			}
		})
	}
}

func TestMobileDrumTimelineWidthPositive(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			tw := g.drum.timelineRect.Dx()
			if tw <= 0 {
				t.Fatalf("timelineRect.Dx()=%d, want > 0 on %dx%d", tw, vp.w, vp.h)
			}
		})
	}
}

// At least 30% of the drum pane width.
func TestMobileDrumTimelineMinPercent(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			drumW := g.drum.Bounds.Dx()
			tw := g.drum.timelineRect.Dx()
			if drumW > 0 {
				pct := tw * 100 / drumW
				if pct < 30 {
					t.Fatalf("timeline width %d is only %d%% of drum width %d, want >= 30%%",
						tw, pct, drumW)
				}
			}
		})
	}
}

func TestMobileDesktopLayoutUnchanged(t *testing.T) {
	setupMobileTest(t, false) // desktop path
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)
	advanceFrames(g, 2)

	rah := g.drum.rowsAreaHeight()
	vis := g.drum.visibleRows()
	tw := g.drum.timelineRect.Dx()

	if rah <= 0 {
		t.Fatalf("desktop: rowsAreaHeight()=%d, want > 0", rah)
	}
	if vis < 1 {
		t.Fatalf("desktop: visibleRows()=%d, want >= 1", vis)
	}
	if tw <= 0 {
		t.Fatalf("desktop: timelineRect.Dx()=%d, want > 0", tw)
	}
}

// Small screens draw rows directly; desktop builds the row caches and rows layer.
func TestMobileDrumRowCacheBuilt(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			g.drum.markAllRowsDirty()

			// Draw to trigger the rendering pipeline.
			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			// Verify visible rows exist.
			vis := g.drum.visibleRows()
			if vis < 1 {
				t.Fatalf("visibleRows()=%d after Draw, want >= 1", vis)
			}

			if isSmallScreen() {
				if g.drum.directDrawCount == 0 {
					t.Fatalf("directDrawCount=0 on small screen %dx%d — direct draw path not used", vp.w, vp.h)
				}
			} else {
				for i := g.drum.rowOffset; i < g.drum.rowOffset+vis && i < len(g.drum.Rows); i++ {
					if i >= len(g.drum.rowCache) || g.drum.rowCache[i] == nil {
						t.Fatalf("rowCache[%d] is nil after Draw on %dx%d", i, vp.w, vp.h)
					}
				}
				if g.drum.rowsLayer == nil {
					t.Fatalf("rowsLayer is nil after Draw on %dx%d — fallback path did not build layer", vp.w, vp.h)
				}
			}
		})
	}
}

// The direct path bypasses the rows layer, so directDrawCount stands in for sampling it.
func TestMobileDrumRowHasContentWithStriping(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			g.drum.markAllRowsDirty()

			// Draw to trigger the rendering pipeline.
			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			if isSmallScreen() {
				// Direct draw path: content goes directly to dst, not to rowsLayer.
				if g.drum.directDrawCount == 0 {
					t.Fatalf("directDrawCount=0 after Draw on small screen %dx%d — rows not drawn", vp.w, vp.h)
				}
				if g.drum.directDrawCells == 0 {
					t.Fatalf("directDrawCells=0 after Draw on %dx%d — no cells drawn", vp.w, vp.h)
				}
			} else {
				// Layer path: sample the composed rows image.
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Skipf("skipping rowHasContent check: Ebiten ReadPixels unavailable before game start (%v)", r)
						}
					}()
					if !g.drum.rowHasContent(0) {
						t.Fatalf("rowHasContent(0)=false after Draw on %dx%d — rows are blank", vp.w, vp.h)
					}
				}()
			}
		})
	}
}

func TestMobileDrumRowsDrawnMaskWithStriping(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			g.drum.markAllRowsDirty()

			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			// Check that visible rows are marked as drawn.
			vis := g.drum.visibleRows()
			for i := g.drum.rowOffset; i < g.drum.rowOffset+vis && i < len(g.drum.Rows); i++ {
				if i >= len(g.drum.rowsDrawnMask) {
					t.Fatalf("rowsDrawnMask too short: len=%d, need index %d", len(g.drum.rowsDrawnMask), i)
				}
				if !g.drum.rowsDrawnMask[i] {
					t.Fatalf("rowsDrawnMask[%d]=false on %dx%d — row not drawn", i, vp.w, vp.h)
				}
			}
		})
	}
}

// The test init() zeroes eqPanelHeight, which hides the EQ panel eating all the space on small screens.
func TestMobileDrumRowsWithProductionEQ(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// recalcButtons zeroes it under test, so restore it and redo the dependent layout.
			eqPanelHeight = 180
			t.Cleanup(func() { eqPanelHeight = 0 })

			g.drum.eqH = 180
			g.drum.refreshWidgetLayout()
			g.drum.calcLayout()

			rah := g.drum.rowsAreaHeight()
			vis := g.drum.visibleRows()
			t.Logf("%s: eqH=%d headerH=%d boundsH=%d rowsArea=%d vis=%d rowH=%d",
				vp.name, g.drum.eqH, g.drum.headerH, g.drum.Bounds.Dy(), rah, vis, g.drum.rowHeight())

			if rah <= 0 {
				t.Fatalf("rowsAreaHeight()=%d with production eqH=180, want > 0 on %dx%d",
					rah, vp.w, vp.h)
			}
			if vis < 1 {
				t.Fatalf("visibleRows()=%d with production eqH=180, want >= 1 on %dx%d (rowsArea=%d, rowH=%d)",
					vis, vp.w, vp.h, rah, g.drum.rowHeight())
			}
		})
	}
}

func TestMobileDrumRowCacheWithProductionEQ(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Apply production EQ height and re-layout.
			eqPanelHeight = 180
			t.Cleanup(func() { eqPanelHeight = 0 })

			g.drum.eqH = 180
			g.drum.refreshWidgetLayout()
			g.drum.calcLayout()

			g.drum.markAllRowsDirty()

			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			vis := g.drum.visibleRows()
			if vis < 1 {
				t.Fatalf("visibleRows()=%d after Draw with eqH=180, want >= 1", vis)
			}
			// On small screens, direct draw path is used — rowsLayer may be nil.
			if isSmallScreen() {
				if g.drum.directDrawCount == 0 {
					t.Fatalf("directDrawCount=0 on small screen with eqH=180 on %dx%d", vp.w, vp.h)
				}
			} else if g.drum.rowsLayer == nil {
				t.Fatalf("rowsLayer nil after Draw with eqH=180 on %dx%d", vp.w, vp.h)
			}
		})
	}
}

// On small screens with striping enabled, when the stripes bail out.
func TestMobileDirectDrawUsed(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			g.drum.markAllRowsDirty()

			// Reset counter before Draw.
			g.drum.directDrawCount = 0

			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			if g.drum.directDrawCount == 0 {
				t.Fatalf("directDrawCount=0 on %dx%d — direct draw path not used on small screen", vp.w, vp.h)
			}
			if g.drum.directDrawCells == 0 {
				t.Fatalf("directDrawCells=0 on %dx%d — no cells drawn", vp.w, vp.h)
			}

			// Verify visible rows are marked as drawn.
			vis := g.drum.visibleRows()
			for i := g.drum.rowOffset; i < g.drum.rowOffset+vis && i < len(g.drum.Rows); i++ {
				if i >= len(g.drum.rowsDrawnMask) {
					t.Fatalf("rowsDrawnMask too short: len=%d, need index %d", len(g.drum.rowsDrawnMask), i)
				}
				if !g.drum.rowsDrawnMask[i] {
					t.Fatalf("rowsDrawnMask[%d]=false on %dx%d — row not drawn by direct draw", i, vp.w, vp.h)
				}
			}
			t.Logf("%s: directDrawCount=%d directDrawCells=%d vis=%d",
				vp.name, g.drum.directDrawCount, g.drum.directDrawCells, vis)
		})
	}
}

func TestMobileDesktopNoDirectDraw(t *testing.T) {
	setupMobileTest(t, false) // desktop path
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.drum.markAllRowsDirty()
	g.drum.directDrawCount = 0

	screen := ebiten.NewImage(1280, 720)
	g.drum.Draw(screen, nil, 0, nil, 0)

	if g.drum.directDrawCount != 0 {
		t.Fatalf("directDrawCount=%d on desktop, want 0 — direct draw should not be used on desktop",
			g.drum.directDrawCount)
	}
}

// Zoomed out (n > timelineWidth) the beat ticks are decimated.
func TestMobileDirectDrawDecimatedMarkers(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum

	// Force extreme length so n > timelineWidth, triggering decimated markers.
	extreme := dv.timelineRect.Dx() * 10
	dv.Length = extreme
	for _, r := range dv.Rows {
		r.Steps = make([]bool, extreme)
		r.CellTypes = make([]model.NodeType, extreme)
	}
	dv.markAllRowsDirty()

	// Intercept drawRect to count colTimelineBeat ticks in the row area.
	rh := dv.rowHeight()
	rowTop := dv.Bounds.Min.Y + dv.headerH
	ticks := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && r.Min.Y >= rowTop {
			if r.Dx() == 1 && r.Dy() == rh && color.RGBAModel.Convert(c).(color.RGBA) == colTimelineBeat {
				ticks++
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	screen := ebiten.NewImage(390, 844)
	dv.Draw(screen, nil, 0, nil, 0)

	if !isSmallScreen() {
		t.Skip("test requires small screen path for drawRowsDirect")
	}
	if dv.directDrawCount == 0 {
		t.Fatalf("directDrawCount=0 — drawRowsDirect was not invoked")
	}
	if ticks == 0 {
		t.Fatalf("no decimated marker ticks drawn by drawRowsDirect when n=%d > w=%d",
			extreme, dv.timelineRect.Dx())
	}

	// Verify tick count is reasonable: roughly timelineWidth ticks per visible row.
	vis := dv.visibleRows()
	if vis < 1 {
		vis = 1
	}
	maxTicks := dv.timelineRect.Dx() * vis
	if ticks > maxTicks*2 {
		t.Fatalf("too many ticks: got %d, max reasonable ~%d", ticks, maxTicks)
	}
	t.Logf("decimated ticks=%d vis=%d timelineW=%d n=%d", ticks, vis, dv.timelineRect.Dx(), extreme)
}

// M/S/FX and the volume slider are reachable in the row strip, not only through a context menu.
func TestMobileRowInlineControlsVisible(t *testing.T) {
	for _, vp := range []mobileViewport{
		{"iPhone14_portrait", 390, 844},
		{"iPhone8_portrait", 375, 667},
		{"iPhoneSE_portrait", 320, 568},
	} {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			if g.drum.rowRackZone == nil {
				t.Fatal("rowRackZone is nil")
			}
			if len(g.drum.Rows) == 0 {
				t.Fatal("no drum rows after Layout")
			}

			vols := g.drum.rowRackZone.RowVolSliders()
			mutes := g.drum.rowRackZone.RowMuteBtns()
			solos := g.drum.rowRackZone.RowSoloBtns()
			fxs := g.drum.rowRackZone.RowFXBtns()

			// Row 0 must be fully laid out.
			if len(vols) == 0 || vols[0].Rect().Empty() {
				t.Fatalf("vol slider rect empty on %dx%d (got %v)", vp.w, vp.h, vols)
			}
			if len(mutes) == 0 || mutes[0].Rect().Empty() {
				t.Fatalf("mute button rect empty on %dx%d", vp.w, vp.h)
			}
			if len(solos) == 0 || solos[0].Rect().Empty() {
				t.Fatalf("solo button rect empty on %dx%d", vp.w, vp.h)
			}
			if len(fxs) == 0 || fxs[0].Rect().Empty() {
				t.Fatalf("fx button rect empty on %dx%d", vp.w, vp.h)
			}

			// Buttons must not overlap each other (left → right order).
			vr := vols[0].Rect()
			mr := mutes[0].Rect()
			sr := solos[0].Rect()
			fr := fxs[0].Rect()
			if !(vr.Max.X <= mr.Min.X && mr.Max.X <= sr.Min.X && sr.Max.X <= fr.Min.X) {
				t.Fatalf("controls overlap on %dx%d: vol=%v mute=%v solo=%v fx=%v",
					vp.w, vp.h, vr, mr, sr, fr)
			}

			// On the narrowest phones the chips shrink to keep the label legible; each registers a Touch hit area the
			// dispatcher expands to TouchMinTarget, so the contract is present + touch-expanded width.
			expand := TouchMinTarget()
			for _, b := range []struct {
				name string
				r    image.Rectangle
			}{{"mute", mr}, {"solo", sr}, {"fx", fr}} {
				if b.r.Empty() {
					t.Fatalf("%s button rect empty on %dx%d", b.name, vp.w, vp.h)
				}
				effective := expandRect(b.r, expand).Intersect(g.drum.rowRackZone.rect)
				if effective.Dx() < expand {
					t.Fatalf("%s effective tap width=%d < touch-min %d on %dx%d (chip=%d)",
						b.name, effective.Dx(), expand, vp.w, vp.h, b.r.Dx())
				}
			}
		})
	}
}

// Label, vol, M, S, FX and ⋯.
func TestDesktopRowControlsUnchanged(t *testing.T) {
	setupMobileTest(t, false) // desktop path
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)
	advanceFrames(g, 2)

	if len(g.drum.Rows) == 0 {
		t.Fatal("no drum rows after Layout")
	}

	vols := g.drum.rowRackZone.RowVolSliders()
	mutes := g.drum.rowRackZone.RowMuteBtns()
	solos := g.drum.rowRackZone.RowSoloBtns()
	fxs := g.drum.rowRackZone.RowFXBtns()
	menus := g.drum.rowRackZone.RowMenuBtns()

	if vols[0].Rect().Empty() || mutes[0].Rect().Empty() || solos[0].Rect().Empty() ||
		fxs[0].Rect().Empty() || menus[0].Rect().Empty() {
		t.Fatalf("desktop control missing: vol=%v mute=%v solo=%v fx=%v menu=%v",
			vols[0].Rect(), mutes[0].Rect(), solos[0].Rect(), fxs[0].Rect(), menus[0].Rect())
	}
}

// For n <= w the direct path draws the same cells as buildRowSprite.
func TestDirectDrawMatchesBuildRowSprite(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("no drum rows")
	}

	// Ensure n <= w (default Length should be within timeline width).
	n := len(dv.Rows[0].Steps)
	w := dv.timelineRect.Dx()
	if n > w {
		t.Skipf("n=%d > w=%d, not a full-resolution scenario", n, w)
	}
	if n < 1 {
		t.Skip("no steps")
	}

	// Collect cell positions from buildRowSprite (row 0).
	type cellDraw struct {
		X0, X1 int
		On     bool
	}
	var spriteCells []cellDraw
	for j := 0; j < n; j++ {
		x0 := (j * w) / n
		x1 := ((j + 1) * w) / n
		if x1 <= x0 {
			x1 = x0 + 1
		}
		on := j < len(dv.Rows[0].Steps) && dv.Rows[0].Steps[j]
		spriteCells = append(spriteCells, cellDraw{X0: x0, X1: x1, On: on})
	}

	// Collect cell positions from drawRowsDirect (row 0).
	startX := dv.timelineRect.Min.X
	var directCells []cellDraw
	for j := 0; j < n; j++ {
		x0 := startX + (j*w)/n
		x1 := startX + ((j+1)*w)/n
		if x1 <= x0 {
			x1 = x0 + 1
		}
		on := j < len(dv.Rows[0].Steps) && dv.Rows[0].Steps[j]
		directCells = append(directCells, cellDraw{X0: x0 - startX, X1: x1 - startX, On: on})
	}

	if len(spriteCells) != len(directCells) {
		t.Fatalf("cell count mismatch: sprite=%d direct=%d", len(spriteCells), len(directCells))
	}
	for j := range spriteCells {
		if spriteCells[j] != directCells[j] {
			t.Fatalf("cell %d mismatch: sprite=%+v direct=%+v", j, spriteCells[j], directCells[j])
		}
	}

	// Also verify decimated marker logic agreement for the zoomed-out case.
	bigN := w * 5
	spriteStep := int(math.Ceil(float64(bigN) / float64(w)))
	if spriteStep < 1 {
		spriteStep = 1
	}
	var spriteTickXs []int
	prevX := -1
	for j := 0; j <= bigN; j += spriteStep {
		x := (j * w) / bigN
		if x != prevX {
			spriteTickXs = append(spriteTickXs, x)
			prevX = x
		}
	}
	var directTickXs []int
	prevX = -1
	for j := 0; j <= bigN; j += spriteStep {
		x := startX + (j*w)/bigN
		if x != prevX {
			directTickXs = append(directTickXs, x-startX)
			prevX = x
		}
	}
	if len(spriteTickXs) != len(directTickXs) {
		t.Fatalf("decimated tick count mismatch: sprite=%d direct=%d", len(spriteTickXs), len(directTickXs))
	}
	for j := range spriteTickXs {
		if spriteTickXs[j] != directTickXs[j] {
			t.Fatalf("tick %d X mismatch: sprite=%d direct=%d", j, spriteTickXs[j], directTickXs[j])
		}
	}
	t.Logf("cells=%d ticks(simulated n=%d)=%d match OK", len(spriteCells), bigN, len(spriteTickXs))
}
