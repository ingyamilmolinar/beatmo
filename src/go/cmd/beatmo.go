package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/ui"
)

// defaultLog is overridden via -ldflags "-X main.defaultLog=..." at build time.
var defaultLog = "DEBUG"

func main() {
	logLevel := flag.String("log", defaultLog, "Log level (DEBUG, INFO, ERROR, NONE)")
	demo := flag.Bool("demo", false, "run a demo circuit and exit")
	benchBPM := flag.Int("bench-bpm", 0, "benchmark mode: play demo at this BPM (0 = disabled)")
	benchSecs := flag.Float64("bench-secs", 10, "benchmark mode: duration in seconds")
	benchProf := flag.String("bench-prof", "", "benchmark mode: write CPU profile to this path")
	screenshot := flag.String("screenshot", "", "capture a screenshot to this path and exit")
	scopeOpen := flag.Bool("scope", false, "open with scope panel visible")
	scopeExport := flag.Bool("scope-export", false, "enable continuous scope data export to JSONL")
	flag.Parse()

	logger := game_log.New(os.Stdout, game_log.LevelFromString(*logLevel))
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

	if *scopeExport || os.Getenv("SCOPE_EXPORT") == "1" {
		audio.EnableScopeExport()
	}

	// Create an instance of our game
	g := ui.New(logger)
	if *screenshot != "" {
		g.SetScreenshot(*screenshot)
	} else if *benchBPM > 0 {
		g.RunBenchmark(*benchBPM, time.Duration(*benchSecs*float64(time.Second)))
	} else if *demo {
		g.RunDemo()
	}
	if *scopeOpen {
		g.SetScopeVisible(true)
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
	ebiten.SetWindowSize(1280, 720)
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
