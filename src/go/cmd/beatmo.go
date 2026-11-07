package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/eventstream"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/ui"
	"github.com/ingyamilmolinar/beatmo/internal/ui/uistate"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// logRuntimeFinal logs the post-run snapshot so a bench output is
// self-documenting about which scheduler/GC knobs were active.
func logRuntimeFinal(logger *game_log.Logger, _ async.RuntimeSnapshot) {
	final := async.Snapshot()
	logger.Infof("[runtime] final snapshot: NumCPU=%d GOMAXPROCS=%d GCPercent=%d MemoryLimit=%d goroutines=%d",
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
	benchStorm := flag.String("bench-synth-storm", "", "synth param-mutation storm during bench: off|toggle|ctrl|combined")
	benchStormInst := flag.String("bench-synth-storm-inst", string(roster.Cello), "instrument id the synth storm mutates")
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
	devTools := flag.Bool("devtools", true, "show the developer-only surfaces (Chain tab, low-level synth controls); false previews the production UI beatmo.io ships")
	flag.Parse()
	if !*devTools {
		ui.SetDevToolsForcedOff(true)
	}

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
	// One pipeline: packages with no injected logger (internal/audio) log
	// through the default, so their lines honour -log too.
	game_log.SetDefault(logger)

	// Before any goroutine spawns, so it holds for the whole process. On WASM: a 1500 MB soft heap cap and GOGC=50, well
	// under the ~2 GB linear-memory ceiling that crashes the tab (BEATMO_MEMORY_LIMIT_MB / BEATMO_GC_PERCENT override).
	rtSnap := async.ConfigureRuntime(wasmFriendlyRuntimeOpts())
	logger.Infof("[runtime] startup snapshot: NumCPU=%d GOMAXPROCS=%d GCPercent=%d MemoryLimit=%d",
		rtSnap.NumCPU, rtSnap.GOMAXPROCS, rtSnap.GCPercent, rtSnap.MemoryLimit)
	defer logRuntimeFinal(logger, rtSnap)

	if stop := startPyroscope(logger); stop != nil {
		defer stop()
	}

	// PPROF=1 → http://localhost:6060. A no-op on js/wasm, so the browser build never pulls in net/http + crypto/tls.
	maybeStartPprofServer()

	// CPU profiling for benchmark mode (write to file, avoids HTTP race).
	if *benchBPM > 0 && *benchProf != "" {
		stop, err := startBenchCPUProfile(*benchProf)
		if err != nil {
			logger.Errorf("[bench-prof] %v", err)
			return
		}
		defer stop()
	}

	// Auto-record benchmark mode: orchestrates demo+recording+profile so a
	// single CLI invocation produces a reproducible pre/post snapshot.
	var recordBenchDir string
	if *recordBench > 0 {
		dir, stop, err := startRecordBench(logger)
		if err != nil {
			logger.Errorf("[record-bench] %v", err)
			return
		}
		recordBenchDir = dir
		defer stop()
	}

	// Optional hooks config: bind events to built-in trigger actions.
	if *hooksConfig != "" {
		hookCleanup, err := loadHooksConfig(*hooksConfig, logger)
		if err != nil {
			logger.Errorf("[hooks-config] %v", err)
			return
		}
		defer hookCleanup()
	}

	// A record-bench run also opens one inside its output dir, so events.jsonl sits beside the profiles.
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
			logger.Errorf("[event-log] %v", err)
		} else {
			logger.Infof("[eventlog] writing to %s (verbose=%v)", eventLogPath, verbose)
			defer sink.Close()
		}
	}

	// Must be set before Open subscribes: the Logs panel is fed by the same eventlogger consumer. Runs on both platforms.
	eventlogger.SetLogHistory(eventlogger.NewRingHistory(1000))

	// The INFO narrative (BEATMO_INFO_LOG=off disables it); verbose kinds need BEATMO_EVENT_LOG_VERBOSE=1.
	if os.Getenv("BEATMO_INFO_LOG") != "off" {
		infoLog, err := eventlogger.Open(hooks.GlobalBus(), logger, eventlogger.Options{
			Verbose: verbose,
		})
		if err != nil {
			logger.Errorf("[info-log] %v", err)
		} else {
			defer infoLog.Close()
		}
	}

	if *scopeExport || os.Getenv("SCOPE_EXPORT") == "1" {
		audio.EnableScopeExport()
	}

	// Non-fatal: the UI falls back to in-memory favorites for the session.
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

	// The instrument library has its OWN store, apart from prefs.json, so instruments can be shared without touching
	// favorites / language / UI state. Loaded before the first ResetInstruments, so saved instruments play from frame 1.
	instStore := userprefs.NewInstrumentStore(userprefs.Options{})
	defer instStore.Close()
	audio.SetInstrumentLibrarySink(instStore)
	// Sampler edits saved before versioning join their instrument's first
	// version (read once, never written again).
	oldEdits, _ := prefsStore.(audio.SampleEditSource)
	if err := audio.LoadUserInstrumentLibrary(instStore, oldEdits); err != nil {
		logger.Warnf("[instruments] %v", err)
	}
	audio.ResetInstruments()

	// ApplySavedSamples re-registers persisted user samples for playback before the UI starts.
	if ss, ok := prefsStore.(userprefs.SampleStore); ok {
		adapter := ui.NewSamplePersistAdapter(ss)
		audio.SetSampleSink(adapter)
		audio.SetUseSampleStore(true)
		audio.ApplySavedSamples(adapter)
	}

	// Knob step-rung persistence: remember each param's chosen step badge
	// rung across restarts. Keyed by param name (global, not per-instrument).
	if ap, ok := prefsStore.(userprefs.AudioPanelStore); ok {
		ui.SetAudioPanelStateSink(ap)
	}
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

	// Before the first Layout so frame 1 renders in the user's colors (prefs.json on desktop, localStorage on WASM).
	if ts, ok := prefsStore.(userprefs.ThemeStore); ok {
		ui.SetThemeSink(ts)
		ui.ApplyStoredTheme(g)
	}

	if *forceMobile {
		g.SetForceMobileProfile(true)
	}

	// Circuit, then ui-state, then the scene, so menus a scene opens overlay the imported circuit.
	if *importPath != "" {
		data, err := os.ReadFile(*importPath)
		if err != nil {
			logger.Errorf("[import] %v", err)
			return
		}
		if err := g.Import(data); err != nil {
			logger.Errorf("[import] %v", err)
			return
		}
	}
	if *uiStatePath != "" {
		if err := uistate.ApplyFile(g, *uiStatePath); err != nil {
			logger.Errorf("[ui-state] %v", err)
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
			logger.Errorf("[scene] %v", err)
			return
		}
	}

	switch {
	case *screenshot != "":
		g.SetScreenshot(*screenshot)
	case *recordBench > 0:
		g.RunRecordBenchmark(*recordBenchBPM, time.Duration(*recordBench*float64(time.Second)), recordBenchDir)
	case *benchBPM > 0:
		var stormInst instrument.ID
		if *benchStormInst != "" {
			id, err := instrument.ParseID(*benchStormInst)
			if err != nil {
				logger.Errorf("[bench-synth-storm-inst] %v", err)
				os.Exit(1)
			}
			stormInst = id
		}
		g.RunBenchmark(*benchBPM, time.Duration(*benchSecs*float64(time.Second)), ui.ParseBenchStormScenario(*benchStorm), stormInst)
	case *demo:
		g.RunDemo()
	}
	if *scopeOpen {
		g.SetChainVisible(true)
	}
	if err := startTour(g); err != nil {
		logger.Errorf("[tour] %v", err)
		return
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
	// A fixed TPS decouples Update (audio/scheduler) from Draw, so slow frames on web do not stall playback timing.
	ebiten.SetTPS(60)

	// Run the game. On WASM, this will create a <canvas> in index.html
	if err := ebiten.RunGame(g); err != nil {
		logger.Errorf("[run] %v", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: acceptable for main()
	}
	// Clean up audio on normal window close.
	g.Close()
}
