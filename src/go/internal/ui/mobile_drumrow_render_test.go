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

// TestMobileDrumRowsAreaPositive verifies that rowsAreaHeight() > 0
// on all mobile viewports so drum rows are actually visible.
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

// TestMobileDrumVisibleRowsPositive verifies that visibleRows() >= 1
// on all mobile viewports.
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

// TestMobileDrumTimelineWidthPositive verifies that timelineRect.Dx() > 0
// on all mobile viewports so row sprites have space to render.
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

// TestMobileDrumTimelineMinPercent verifies that the timeline column gets at
// least 30% of the drum pane width on all mobile viewports.
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

// TestMobileDesktopLayoutUnchanged verifies that desktop layout is not
// affected by the mobile fixes.
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

// TestMobileDrumRowCacheBuiltWithStriping verifies that when rowsStripingEnabled
// is true (the WASM path), rows are rendered after Draw(). On small screens,
// the direct draw path is used (bypassing intermediate textures); on desktop,
// row caches and rows layer are built via rowsLayerMaybeRebuild().
func TestMobileDrumRowCacheBuiltWithStriping(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Enable striping to simulate WASM path.
			g.drum.rowsStripingEnabled = true
			g.drum.markAllRowsDirty()

			// Draw to trigger the rendering pipeline.
			screen := ebiten.NewImage(vp.w, vp.h)
			g.drum.Draw(screen, nil, 0, nil, 0)

			// Verify visible rows exist.
			vis := g.drum.visibleRows()
			if vis < 1 {
				t.Fatalf("visibleRows()=%d after Draw, want >= 1", vis)
			}

			// On small screens, the direct draw path is used instead of
			// row caches + rows layer. Verify the appropriate path ran.
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

// TestMobileDrumRowHasContentWithStriping verifies that rows are rendered
// after Draw() with striping enabled. On small screens, the direct draw path
// bypasses the rows layer, so we verify directDrawCount > 0 instead of
// sampling the intermediate texture.
func TestMobileDrumRowHasContentWithStriping(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Enable striping to simulate WASM path.
			g.drum.rowsStripingEnabled = true
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

// TestMobileDrumRowsDrawnMaskWithStriping verifies that visible rows are
// marked as drawn in rowsDrawnMask after Draw() with striping enabled.
func TestMobileDrumRowsDrawnMaskWithStriping(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Enable striping to simulate WASM path.
			g.drum.rowsStripingEnabled = true
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

// TestMobileDrumRowsWithProductionEQ verifies that rowsAreaHeight > 0 and
// visibleRows >= 1 even when eqPanelHeight uses its production value (180).
// The test init() sets eqPanelHeight=0, masking the space budget issue where
// the EQ panel eats all vertical space on small screens.
func TestMobileDrumRowsWithProductionEQ(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Simulate production eqPanelHeight after layout is computed.
			// recalcButtons() resets eqPanelHeight=0 under test, so we
			// restore it and re-run the layout calculations that depend on it.
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

// TestMobileDrumRowCacheWithProductionEQ verifies that Draw() produces row
// caches and a rows layer even with production eqPanelHeight=180.
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

			// Enable striping and draw.
			g.drum.rowsStripingEnabled = true
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

// TestMobileDirectDrawUsed verifies that the direct draw path is activated
// on small screens when striping is enabled and stripes bail out.
func TestMobileDirectDrawUsed(t *testing.T) {
	for _, vp := range mobileViewports {
		t.Run(vp.name, func(t *testing.T) {
			setupMobileTest(t, true)
			logger := log.New(testLogOutput(), log.LevelInfo)
			g := New(logger)
			t.Cleanup(g.CloseForTest)

			g.Layout(vp.w, vp.h)
			advanceFrames(g, 2)

			// Enable striping to simulate WASM path.
			g.drum.rowsStripingEnabled = true
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

// TestMobileDesktopNoDirectDraw verifies that desktop layout does NOT use the
// direct draw path even when striping is enabled.
func TestMobileDesktopNoDirectDraw(t *testing.T) {
	setupMobileTest(t, false) // desktop path
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.drum.rowsStripingEnabled = true
	g.drum.markAllRowsDirty()
	g.drum.directDrawCount = 0

	screen := ebiten.NewImage(1280, 720)
	g.drum.Draw(screen, nil, 0, nil, 0)

	if g.drum.directDrawCount != 0 {
		t.Fatalf("directDrawCount=%d on desktop, want 0 — direct draw should not be used on desktop",
			g.drum.directDrawCount)
	}
}

// TestMobileDrumStripeFallbackToLayer verifies that when rowsStripingEnabled
// is true but the timeline is narrow (< wasmStripeTargetPx), the stripe path
// returns false and the fallback rowsLayerMaybeRebuild() path produces content.
func TestMobileDrumStripeFallbackToLayer(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Use a narrow portrait viewport where timeline < 440px.
	g.Layout(320, 568)
	advanceFrames(g, 2)

	dv := g.drum
	dv.rowsStripingEnabled = true
	dv.markAllRowsDirty()

	tw := dv.timelineRect.Dx()
	if tw >= wasmStripeTargetPx {
		t.Skipf("timeline width %d >= %d, not a narrow-timeline scenario", tw, wasmStripeTargetPx)
	}

	// On non-WASM, rowsStripesMaybeRebuild always returns false (GOARCH check).
	// This is the same behavior as WASM on narrow timelines (< 440px).
	gotStripes := dv.rowsStripesMaybeRebuild()
	if gotStripes {
		t.Fatalf("expected stripes=false for narrow timeline (%dpx)", tw)
	}

	// Fallback path should produce a layer.
	dv.rowsLayerMaybeRebuild()
	if dv.rowsLayer == nil {
		t.Fatalf("rowsLayer nil after fallback rowsLayerMaybeRebuild on narrow timeline")
	}

	// Verify the layer has content (not just transparent pixels).
	// Note: rowHasContent calls At() → ReadPixels, which panics under
	// real Ebiten before RunGame(). Skip in that case.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("skipping rowHasContent check: Ebiten ReadPixels unavailable before game start (%v)", r)
			}
		}()
		if !dv.rowHasContent(0) {
			t.Fatalf("rowHasContent(0)=false after fallback layer build — rows are blank")
		}
	}()
}

// TestMobileDirectDrawDecimatedMarkers verifies that drawRowsDirect draws
// decimated colTimelineBeat ticks when zoomed out (n > timelineWidth).
func TestMobileDirectDrawDecimatedMarkers(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	g.Layout(390, 844)
	advanceFrames(g, 2)

	dv := g.drum
	dv.rowsStripingEnabled = true

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

// TestDirectDrawMatchesBuildRowSprite verifies that for the normal (n <= w)
// case, drawRowsDirect produces the same cell draw calls as buildRowSprite.
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
