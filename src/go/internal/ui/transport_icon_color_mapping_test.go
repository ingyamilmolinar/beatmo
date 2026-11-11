//go:build test

package ui

import (
	"image/color"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTransport_IconColorMapping enforces DESIGN.md:1495-1508 IconColor
// mapping. Without this guard, buttons regress to default (black/white)
// IconColors, making the UI look monochrome — the user complaint that
// prompted Theme 4.
func TestTransport_IconColorMapping(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	tz := g.drum.transportZone

	type tc struct {
		name      string
		got, want color.Color
	}
	cases := []tc{
		{"play", tz.playBtn.IconColor, Profile().PlayIconColor},
		{"stop", tz.stopBtn.IconColor, Profile().StopIconColor},
		{"rec", tz.recordBtn.IconColor, colRecordIdle},
		{"bpm-", tz.bpmDecBtn.IconColor, Profile().BPMIconColor},
		{"bpm+", tz.bpmIncBtn.IconColor, Profile().BPMIconColor},
	}
	for _, c := range cases {
		if c.got == nil {
			t.Errorf("%s: IconColor is nil — must set explicitly per spec", c.name)
		} else if c.got != c.want {
			t.Errorf("%s: IconColor=%v, want %v", c.name, c.got, c.want)
		}
	}
}
