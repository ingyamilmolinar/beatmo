package ui

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
	// Pre-warm the standing async pools so they're part of the baseline.
	// internal/ui shares pools with internal/audio (recording.lifecycle),
	// internal/eventstream (eventstream.persist), internal/hooks (hooks.fanout),
	// and the audio scheduler/dialog pools — all spawned lazily on first
	// game construction. Constructing one game here forces them to start
	// before goleak.IgnoreCurrent captures the baseline; otherwise every
	// test that builds a Game appears to leak the shared workers.
	prewarm := New(testLogger)
	prewarm.CloseForTest()
	// Capture the baseline goroutines before m.Run so post-test goleak.Find
	// reports only test-introduced leaks. Mirrors the discipline in the
	// seven sibling packages (async, hooks, eventstream, audio, engine,
	// eventlogger, userprefs) per CLAUDE.md.
	baseline := goleak.IgnoreCurrent()
	code := m.Run()
	closeTestGames()
	if code == 0 {
		if err := goleak.Find(
			baseline,
			goleak.IgnoreTopFunction("github.com/hajimehoshi/ebiten/v2/internal/ui.(*UserInterface).runSingleThread"),
			goleak.IgnoreTopFunction("github.com/hajimehoshi/ebiten/v2/internal/ui.(*UserInterface).loopGame"),
			// async.Pool workers are shared across the package via
			// DefaultRegistry; CloseForTest releases registry refs but a
			// production-pool worker may still be parked in chan receive.
			goleak.IgnoreTopFunction("github.com/ingyamilmolinar/beatmo/internal/async.(*Pool).run"),
			goleak.IgnoreTopFunction("github.com/ingyamilmolinar/beatmo/internal/async.(*Scheduler).run"),
			// audio.initContext singletons. These are spawned lazily on the
			// first PlayParams call (NOT during prewarm), so they aren't in
			// the goleak baseline; once spawned they live for the rest of
			// the process by design (single audio output mux per process).
			// Previously masked by the cascade-of-failures abort path —
			// surfaced once the cascade was fixed.
			//
			// Use IgnoreAnyFunction (matches any frame) since these
			// goroutines are usually parked in `sync.Mutex.Lock` /
			// `sync.Cond.Wait` / cgo syscalls — the actual top frame is
			// `runtime.semacquire` etc., not the package's Run/loop func.
			goleak.IgnoreAnyFunction("github.com/ingyamilmolinar/beatmo/internal/analyzer.(*Service).Run"),
			goleak.IgnoreAnyFunction("github.com/ingyamilmolinar/beatmo/internal/scope.(*Service).Run"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3.(*context).readAndWrite"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3.(*context).readAndWrite.func1"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3/internal/mux.(*Mux).loop"),
			goleak.IgnoreAnyFunction("github.com/ebitengine/oto/v3/internal/mux.(*Mux).wait"),
			// NB: Game.audioLoop and Game.bpmLoop are now joined by
			// Game.Close() via g.bgWG.Wait(), and Engine.run() is joined by
			// Engine.Close() via runDone. They no longer need explicit
			// IgnoreAnyFunction entries — if they show up here it's a real
			// leak (a test built a Game without registering it for
			// CloseForTest, or Close was called concurrently with a still
			// blocked goroutine).
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
