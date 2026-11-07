package ui

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"go.uber.org/goleak"
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
	unregisterGameForTest = func(g *Game) {
		if g == nil {
			return
		}
		testGamesMu.Lock()
		defer testGamesMu.Unlock()
		for i, q := range testGames {
			if q == g {
				testGames = append(testGames[:i], testGames[i+1:]...)
				return
			}
		}
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
	// The shared pools start lazily on the first Game; start them before the baseline, or every test that builds a
	// Game appears to leak them.
	prewarm := New(testLogger)
	prewarm.CloseForTest()
	baseline := goleak.IgnoreCurrent()
	code := m.Run()
	closeTestGames()
	if code == 0 {
		// A swapped input function never put back re-aims the cursor for every later test, far from the culprit.
		if msg := inputGlobalsLeakMsg(); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		}
		if err := goleak.Find(
			baseline,
			goleak.IgnoreTopFunction("github.com/hajimehoshi/ebiten/v2/internal/ui.(*UserInterface).runSingleThread"),
			goleak.IgnoreTopFunction("github.com/hajimehoshi/ebiten/v2/internal/ui.(*UserInterface).loopGame"),
			// Shared pool workers may still be parked after CloseForTest releases the registry refs.
			goleak.IgnoreTopFunction("github.com/ingyamilmolinar/beatmo/internal/async.(*Pool).run"),
			goleak.IgnoreTopFunction("github.com/ingyamilmolinar/beatmo/internal/async.(*Scheduler).run"),
			// The audio output singletons start on the first PlayParams (after the baseline) and live for the process.
			// IgnoreAnyFunction because they park in locks or cgo, so their top frame is runtime's.
			goleak.IgnoreAnyFunction("github.com/ingyamilmolinar/beatmo/internal/analyzer.(*Service).Run"),
			goleak.IgnoreAnyFunction("github.com/ingyamilmolinar/beatmo/internal/scope.(*Service).Run"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3.(*context).readAndWrite"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3.(*context).readAndWrite.func1"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3/internal/mux.(*Mux).loop"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3/internal/mux.(*Mux).wait"),
			// audioLoop, bpmLoop and Engine.run are joined by Close; if they show up here a Game escaped CloseForTest.
		); err != nil {
			fmt.Fprintf(os.Stderr, "goleak: %v\n", err)
			code = 1
		}
	}
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
	parityWatchDefault = testParityWatchDefault
	parityFatalEnabled.Store(true)
	parityScanEveryFrames = parityScanEveryDefault
	parityScanStride = parityScanStrideDefault
	predictorPerfThrottleEnabled = (runtime.GOARCH == "wasm")

	audio.ResetInstruments()
	audio.SetStopHook(nil)
	audio.SetBPMFuncForTest(func(int) {})
	audio.SetChannelEQ(channel.Master, 0)

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
