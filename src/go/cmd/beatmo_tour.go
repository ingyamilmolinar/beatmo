//go:build !js

package main

import (
	"flag"

	"github.com/ingyamilmolinar/beatmo/internal/ui"
)

var (
	tourFlag   = flag.String("tour", "", "play the landing-page showcase tour: desktop|phone (desktop only)")
	tourRecord = flag.String("record", "", "with -tour: record the tour into this directory (frames.mkv, audio, <cut>.vtt, report.json)")
)

func startTour(g *ui.Game) error {
	if *tourFlag == "" {
		return nil
	}
	return g.StartTour(*tourFlag, *tourRecord)
}
