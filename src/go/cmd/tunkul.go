package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"github.com/ingyamilmolinar/tunkul/internal/ui"
)

// defaultLog is overridden via -ldflags "-X main.defaultLog=..." at build time.
var defaultLog = "DEBUG"

func main() {
	logLevel := flag.String("log", defaultLog, "Log level (DEBUG, INFO, ERROR, NONE)")
	demo := flag.Bool("demo", false, "run a demo circuit and exit")
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

	// Create an instance of our game
	g := ui.New(logger)
	if *demo {
		g.RunDemo()
	}

	// Optional window settings (not used in WASM, but for desktop builds)
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Tunkul - Node Music Game")
	// Keep simulation running at a fixed TPS even if rendering falls behind. This
	// decouples Update() (audio/scheduler) from Draw() so slow frames on web do not
	// stall playback timing.
	ebiten.SetTPS(60)

	// Run the game. On WASM, this will create a <canvas> in index.html
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
