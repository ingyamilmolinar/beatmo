package ui

import (
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

var (
	testGamesMu sync.Mutex
	testGames   []*Game

	defaultDrawRect      = drawRect
	defaultDrawEdgeLine  = drawEdgeLine
	defaultDrawButton    = drawButton
	defaultDrawCursor    = drawCursor
	defaultDrawPlayIcon  = drawPlayIcon
	defaultDrawPauseIcon = drawPauseIcon
	defaultDrawStopIcon  = drawStopIcon
	defaultDrawPencil    = drawPencilIcon
	defaultDrawSave      = drawSaveIcon
)

func init() {
	registerGameForTest = func(g *Game) {
		if g == nil {
			return
		}
		testGamesMu.Lock()
		testGames = append(testGames, g)
		testGamesMu.Unlock()
	}
}

func closeTestGames() {
	testGamesMu.Lock()
	games := append([]*Game(nil), testGames...)
	testGames = nil
	testGamesMu.Unlock()
	for _, g := range games {
		if g != nil {
			g.CloseForTest()
		}
	}
}

func TestMain(m *testing.M) {
	resetTestEnvAndGlobals()
	code := m.Run()
	closeTestGames()
	os.Exit(code)
}

func resetTestEnvAndGlobals() {
	envKeys := []string{
		"BPM_TIMING_TEST",
		"DEBUG_DRAW_NODES",
		"DEBUG_GEOM",
		"DEBUG_HISTORY_SEED",
		"DRAW_FRAMEBUFFER",
		"NO_EDGE_CACHE",
		"NO_GRID_DRAW",
		"NO_GRID_TILE_CACHE",
		"NO_PIXEL_SNAP",
		"NO_SPRITE_NODES",
		"PARITY_DUMP_STDERR",
		"PARITY_FATAL",
		"PARITY_SCAN_EVERY",
		"PARITY_SCAN_STRIDE",
		"PARITY_WATCH",
		"PARITY_WASM_FATAL",
		"PERF_BROWSER_UPDATE_MAX_MS",
		"PERF_FAST_PATH",
		"PERF_LOG",
		"PREVIEW_DEBUG",
		"RENDER_SAFE",
		"SCREEN_EDGES",
		"TIMELINE_TRACE",
		"TIMELINE_TRACE_ROW",
		"BEATMO_ASSETS",
		"BEATMO_CONFIG",
		"BEATMO_DEBUG_INST",
		"BEATMO_DEMO_CONFIG",
		"BEATMO_ROW_SNAPSHOTS",
	}
	for _, key := range envKeys {
		_ = os.Unsetenv(key)
	}

	// Reset global flags derived from env so tests run with default behavior.
	enableDefaultStart = true
	forceAutoSize = false
	forceSmallScreenForTest = false
	defaultPerfFastPath = false
	timelineTrace = false
	timelineTraceRow = 0
	debugGeom = false
	parityWatchDefault = parityWatchOff
	parityFatalEnabled.Store(true)
	parityScanEveryFrames = parityScanEveryDefault
	parityScanStride = parityScanStrideDefault
	if runtime.GOOS == "js" {
		parityWatchDefault = parityWatchLog
		parityFatalEnabled.Store(false)
	}
	predictorPerfThrottleEnabled = (runtime.GOARCH == "wasm")

	audio.ResetInstruments()
	audio.SetStopHook(nil)
	audio.SetBPMFuncForTest(func(int) {})
	audio.SetChannelEQ("main", 0)

	drawRect = defaultDrawRect
	drawEdgeLine = defaultDrawEdgeLine
	drawButton = defaultDrawButton
	drawCursor = defaultDrawCursor
	drawPlayIcon = defaultDrawPlayIcon
	drawPauseIcon = defaultDrawPauseIcon
	drawStopIcon = defaultDrawStopIcon
	drawPencilIcon = defaultDrawPencil
	drawSaveIcon = defaultDrawSave

	// Reset cached env vars (in case tests overrode them directly)
	envRenderSafe = false
	envScreenEdges = false
	envNoGridDraw = false
	envNoGridTileCache = false
	envNoPixelSnap = false
	envNoEdgeCache = false
	envNoSpriteNodes = false
}
