//go:build js

package main

import (
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	ui "github.com/ingyamilmolinar/beatmo/internal/ui"
	"syscall/js"
	"time"
)

func main() {
	logger := game_log.New(nil, game_log.LevelError)
	// Tests set up state explicitly through the JS helpers.
	ui.SetDefaultStartForTest(false)
	g := ui.New(logger)
	g.Layout(1280, 720)
	// ~60 Hz so the JS helpers see live behaviour without the full Ebiten game.
	go func() {
		ticker := time.NewTicker(time.Second / 60)
		defer ticker.Stop()
		for range ticker.C {
			_ = g.Update()
		}
	}()
	js.Global().Set("__wasmReady", js.ValueOf(true))
	select {}
}
