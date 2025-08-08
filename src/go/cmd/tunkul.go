package main

import (
	"flag"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"github.com/ingyamilmolinar/tunkul/internal/ui"
)

func main() {
	logLevel := flag.String("log", "DEBUG", "Log level (DEBUG, INFO, ERROR, NONE)")
	flag.Parse()

	logger := game_log.New(os.Stdout, game_log.LevelFromString(*logLevel))

	// Create an instance of our game
	g := ui.New(logger)

	// Optional window settings (not used in WASM, but for desktop builds)
	ebiten.SetWindowSize(1280, 720)
	ebiten.SetWindowTitle("Tunkul - Node Music Game")
	// Run as fast as possible for smoother gameplay
	ebiten.SetTPS(ebiten.SyncWithFPS)

	// Run the game. On WASM, this will create a <canvas> in index.html
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
