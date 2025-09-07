//go:build js

package main

import (
    "syscall/js"
    game_log "github.com/ingyamilmolinar/tunkul/internal/log"
    ui "github.com/ingyamilmolinar/tunkul/internal/ui"
)

func main() {
    logger := game_log.New(nil, game_log.LevelError)
    g := ui.New(logger)
    g.Layout(800, 600)
    js.Global().Set("__wasmReady", js.ValueOf(true))
    select {}
}

