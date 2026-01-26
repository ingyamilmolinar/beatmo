//go:build js

package main

import (
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	ui "github.com/ingyamilmolinar/tunkul/internal/ui"
	"syscall/js"
	"time"
)

func main() {
	logger := game_log.New(nil, game_log.LevelError)
	// Disable default demo/start behavior in this harness so tests control
	// the state explicitly via exported JS helpers.
	ui.SetDefaultStartForTest(false)
	g := ui.New(logger)
	g.Layout(800, 600)
	// Drive the UI Update loop at ~60Hz so JS helpers (startPlay,
	// incrementBPM, currentBeat) reflect live behavior without running the
	// full Ebiten game.
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
