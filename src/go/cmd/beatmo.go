package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/eventstream"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/ui"
	"github.com/ingyamilmolinar/beatmo/internal/ui/uistate"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// logRuntimeFinal logs the post-run snapshot so a bench output is
// self-documenting about which scheduler/GC knobs were active.
func logRuntimeFinal(logger *game_log.Logger, _ async.RuntimeSnapshot) {
	final := async.Snapshot()
	logger.Infof("[RUNTIME] final snapshot: NumCPU=%d GOMAXPROCS=%d GCPercent=%d MemoryLimit=%d goroutines=%d",
		final.NumCPU, final.GOMAXPROCS, final.GCPercent, final.MemoryLimit, final.NumGoroutine)
}

// defaultLog is overridden via -ldflags "-X main.defaultLog=..." at build time.
var defaultLog = "DEBUG"

func main() {
	logLevel := flag.String("log", defaultLog, "Log level (DEBUG, INFO, ERROR, NONE)")
	demo := flag.Bool("demo", false, "run a demo circuit and exit")
	benchBPM := flag.Int("bench-bpm", 0, "benchmark mode: play demo at this BPM (0 = disabled)")
	benchSecs := flag.Float64("bench-secs", 10, "benchmark mode: duration in seconds")
	benchProf := flag.String("bench-prof", "", "benchmark mode: write CPU profile to this path")
	recordBench := flag.Float64("record-bench", 0, "play demo + record audio for this many seconds, write profiles to bench-results/, then exit (0 = disabled)")
	recordBenchBPM := flag.Int("record-bench-bpm", 120, "BPM to use during -record-bench")
	hooksConfig := flag.String("hooks-config", "", "path to a JSON hooks config to load at startup")
	eventLog := flag.String("event-log", "", "write all hooks events to this JSONL file (also honors BEATMO_EVENT_LOG)")
	eventLogVerbose := flag.Bool("event-log-verbose", false, "include high-frequency Verbose* events in the event log")
	screenshot := flag.String("screenshot", "", "capture a screenshot to this path and exit")
	winW := flag.Int("win-w", 1280, "window width")
	winH := flag.Int("win-h", 720, "window height")
	forceMobile := flag.Bool("mobile", false, "force the mobile layout profile")
	scopeOpen := flag.Bool("scope", false, "open with scope panel visible")
	scopeExport := flag.Bool("scope-export", false, "enable continuous scope data export to JSONL")
	sceneName := flag.String("scene", "", "name of catalog scene to apply at startup")
	scenePass := flag.String("scene-pass", "desktop", "scene capture pass: 'desktop' uses Setup, 'mobile' uses MobileSetup")
	listScenes := flag.Bool("list-scenes", false, "print scene names and exit")
	listMobileScenes := flag.Bool("list-mobile-scenes", false, "print mobile-eligible scene names and exit")
	listSceneSubjects := flag.Bool("list-scene-subjects", false, "print 'name<TAB>subject' per line and exit (subject is empty for full-screen scenes)")
	uiStatePath := flag.String("ui-state", "", "path to UI state JSON (camera, splitter, profile, view)")
	importPath := flag.String("import", "", "path to a tunkul.json project to import at startup")
	flag.Parse()

	if *listScenes {
		for _, name := range ui.SceneNames(true) {
			fmt.Println(name)
		}
		return
	}
	if *listMobileScenes {
		for _, name := range ui.MobileSceneNames() {
			fmt.Println(name)
		}
		return
	}
	if *listSceneSubjects {
		for _, s := range ui.ListScenes() {
			fmt.Printf("%s\t%s\n", s.Name, s.Subject)
		}
		return
	}

	logger := game_log.New(os.Stdout, game_log.LevelFromString(*logLevel))

	// Apply runtime config (GC tuning, profile rates) before any
	// goroutine spawns so settings take effect for the whole process.
	// On WASM we set aggressive defaults: a 1500 MB soft heap cap and
	// GOGC=50 so the runtime collects garbage well before the ~2 GB
	// linear-memory ceiling that crashes the browser tab. Both can be
	// overridden via BEATMO_MEMORY_LIMIT_MB / BEATMO_GC_PERCENT envs.
	rtSnap := async.ConfigureRuntime(wasmFriendlyRuntimeOpts())
	logger.Infof("[RUNTIME] startup snapshot: NumCPU=%d GOMAXPROCS=%d GCPercent=%d MemoryLimit=%d",
		rtSnap.NumCPU, rtSnap.GOMAXPROCS, rtSnap.GCPercent, rtSnap.MemoryLimit)
	defer logRuntimeFinal(logger, rtSnap)

	if stop := startPyroscope(logger); stop != nil {
		defer stop()
	}

	// Optional pprof server for profiling: enable with PPROF=1 and visit http://localhost:6060
	if os.Getenv("PPROF") == "1" {
		go func() {
			_ = http.ListenAndServe("localhost:6060", nil)
		}()
	}

	// CPU profiling for benchmark mode (write to file, avoids HTTP race).
	if *benchBPM > 0 && *benchProf != "" {
		f, err := os.Create(*benchProf)
		if err != nil {
			log.Printf("bench-prof: %v", err)
			return
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			log.Printf("bench-prof: %v", err)
			return
		}
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
		}()
	}

	// Auto-record benchmark mode: orchestrates demo+recording+profile so a
	// single CLI invocation produces a reproducible pre/post snapshot.
	var recordBenchDir string
	if *recordBench > 0 {
		dir, stop, err := startRecordBench(logger)
		if err != nil {
			log.Printf("record-bench: %v", err)
			return
		}
		recordBenchDir = dir
		defer stop()
	}

	// Optional hooks config: bind events to built-in trigger actions.
	if *hooksConfig != "" {
		hookCleanup, err := loadHooksConfig(*hooksConfig, logger)
		if err != nil {
			log.Printf("hooks-config: %v", err)
			return
		}
		defer hookCleanup()
	}

	// Open the event-stream JSONL sink if a path was provided. Also auto-
	// open one inside the record-bench output dir so every bench run is
	// self-documenting (events.jsonl alongside the profiles).
	eventLogPath := *eventLog
	if eventLogPath == "" {
		eventLogPath = os.Getenv("BEATMO_EVENT_LOG")
	}
	verbose := *eventLogVerbose || os.Getenv("BEATMO_EVENT_LOG_VERBOSE") == "1"
	if eventLogPath == "" && recordBenchDir != "" {
		eventLogPath = filepath.Join(recordBenchDir, "events.jsonl")
	}
	if eventLogPath != "" {
		sink, err := eventstream.Open(hooks.GlobalBus(), eventLogPath, eventstream.Options{
			Verbose: verbose,
		})
		if err != nil {
			log.Printf("event-log: %v", err)
		} else {
			logger.Infof("[eventlog] writing to %s (verbose=%v)", eventLogPath, verbose)
			defer sink.Close()
		}
	}

	// Wire the human-readable INFO narrative consumer of hooks.Bus. This is
	// the canonical "what is the user doing / what is the engine doing" view;
	// disable with BEATMO_INFO_LOG=off (default: on). Verbose kinds (camera
	// pan/zoom, drag progress) are filtered unless BEATMO_EVENT_LOG_VERBOSE=1.
	if os.Getenv("BEATMO_INFO_LOG") != "off" {
		infoLog, err := eventlogger.Open(hooks.GlobalBus(), logger, eventlogger.Options{
			Verbose: verbose,
		})
		if err != nil {
			log.Printf("info-log: %v", err)
		} else {
			defer infoLog.Close()
		}
	}

	if *scopeExport || os.Getenv("SCOPE_EXPORT") == "1" {
		audio.EnableScopeExport()
	}

	// Wire user-preference persistence (favorites). Failures here are
	// non-fatal; the UI degrades to in-memory favorites for the session.
	prefsStore := userprefs.NewBackingStore(userprefs.Options{
		Logger: func(format string, args ...any) { logger.Infof(format, args...) },
	})
	defer func() { _ = prefsStore.Close() }()
	ui.SetFavoritesStore(ui.NewPersistedFavoritesStore(prefsStore))

	// Notification history: persist the in-band notification log across
	// sessions so the history popup survives a restart.
	if ns, ok := prefsStore.(userprefs.NotificationHistoryStore); ok {
		ui.SetNotificationHistoryStore(ui.NewPersistedNotificationHistory(ns))
	}

	// Phase 3: apply user-saved recipe overrides + register user
	// recipes from disk. The parity gate is flipped on here (production
	// bootstrap only) so test binaries / parity goldens keep shipped
	// defaults exactly.
	if rs, ok := prefsStore.(userprefs.RecipeStore); ok {
		audio.SetUseUserRecipeOverrides(true)
		_ = audio.ApplyUserRecipeOverrides(rs)
		// Phase 4: wire the Synth-tab Save / Save-As persistence sink
		// to the same backing store. userprefs.RecipeStore directly
		// satisfies ui.RecipeSaveSink (subset of methods).
		ui.SetRecipeSink(rs)
	}

	// Sampler tab: cross-session user-sample persistence (per-file under the
	// prefs dir on desktop). Mirrors the recipe wiring above; ApplySavedSamples
	// re-registers persisted samples for playback before the UI starts.
	if ss, ok := prefsStore.(userprefs.SampleStore); ok {
		adapter := ui.NewSamplePersistAdapter(ss)
		audio.SetSampleSink(adapter)
		audio.SetUseSampleStore(true)
		audio.ApplySavedSamples(adapter)
	}

	// Non-destructive sample-edit descriptors (Sampler Save on a synth
	// source): tiny float maps in prefs.json, rehydrated through
	// audio.SetSampleEdit so the next trigger renders through the saved edit.
	// Save / Reset write back through the same store.
	if se, ok := prefsStore.(userprefs.SampleEditStore); ok {
		ui.SetSampleEditSink(se)
		audio.ApplySavedSampleEdits(se)
	}

	// Knob step-rung persistence: remember each param's chosen step badge
	// rung across restarts. Keyed by param name (global, not per-instrument).
	if ks, ok := prefsStore.(userprefs.KnobStepStore); ok {
		ui.SetKnobStepSink(ks)
	}

	// UI language: apply the persisted locale before the first Layout so the
	// first frame renders in the user's chosen language.
	if ls, ok := prefsStore.(userprefs.LanguageStore); ok {
		ui.SetLanguageSink(ls)
		ui.ApplyStoredLanguage()
	}

	// Create an instance of our game
	g := ui.New(logger)

	if *forceMobile {
		g.SetForceMobileProfile(true)
	}

	// Boot order for the screenshot harness: import circuit first, then
	// apply ui-state config, then run the named scene. Scene Setup runs last
	// so menus/sidebar opened by scenes overlay the imported circuit cleanly.
	if *importPath != "" {
		data, err := os.ReadFile(*importPath)
		if err != nil {
			log.Printf("import: %v", err)
			return
		}
		if err := g.Import(data); err != nil {
			log.Printf("import: %v", err)
			return
		}
	}
	if *uiStatePath != "" {
		if err := uistate.ApplyFile(g, *uiStatePath); err != nil {
			log.Printf("ui-state: %v", err)
			return
		}
		hooks.PublishWithSource(hooks.EventUIStateApplied,
			hooks.UIStatePayload{Path: *uiStatePath}, hooks.CaptureSource(0))
	}
	if *sceneName != "" {
		var err error
		if *scenePass == "mobile" {
			err = ui.RunSceneMobile(g, *sceneName)
		} else {
			err = ui.RunScene(g, *sceneName)
		}
		if err != nil {
			log.Printf("scene: %v", err)
			return
		}
	}

	switch {
	case *screenshot != "":
		g.SetScreenshot(*screenshot)
	case *recordBench > 0:
		g.RunRecordBenchmark(*recordBenchBPM, time.Duration(*recordBench*float64(time.Second)), recordBenchDir)
	case *benchBPM > 0:
		g.RunBenchmark(*benchBPM, time.Duration(*benchSecs*float64(time.Second)))
	case *demo:
		g.RunDemo()
	}
	if *scopeOpen {
		g.SetChainVisible(true)
	}

	// Trap OS signals for graceful audio shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		g.Close()
		os.Exit(0)
	}()

	// Optional window settings (not used in WASM, but for desktop builds)
	ebiten.SetWindowSize(*winW, *winH)
	ebiten.SetWindowTitle("Beatmo - Node Music Game")
	// Keep simulation running at a fixed TPS even if rendering falls behind. This
	// decouples Update() (audio/scheduler) from Draw() so slow frames on web do not
	// stall playback timing.
	ebiten.SetTPS(60)

	// Run the game. On WASM, this will create a <canvas> in index.html
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err) //nolint:gocritic // exitAfterDefer: acceptable for main()
	}
	// Clean up audio on normal window close.
	g.Close()
}
