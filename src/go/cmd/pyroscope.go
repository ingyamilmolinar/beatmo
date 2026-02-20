//go:build !js

package main

import (
	"os"
	"strings"

	"github.com/grafana/pyroscope-go"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func startPyroscope(logger *game_log.Logger) func() {
	url := strings.TrimSpace(os.Getenv("PYROSCOPE_URL"))
	if url == "" {
		return nil
	}
	app := strings.TrimSpace(os.Getenv("PYROSCOPE_APP"))
	if app == "" {
		app = "beatmo"
	}
	prof, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: app,
		ServerAddress:   url,
		Logger:          pyroscope.StandardLogger,
	})
	if err != nil {
		if logger != nil {
			logger.Warnf("[PYROSCOPE] start failed: %v", err)
		}
		return nil
	}
	if logger != nil {
		logger.Infof("[PYROSCOPE] started app=%s url=%s", app, url)
	}
	return func() {
		_ = prof.Stop()
	}
}
