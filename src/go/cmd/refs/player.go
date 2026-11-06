//go:build !js

package main

// player.go -- how `refs session` actually gets a clip heard.
//
// The older `make ab-all` loop shells out to xdg-open/open, which hands the
// file to a desktop player and returns IMMEDIATELY, so the loop has to guess
// the clip's length and sleep. That guess is the reason clips overlapped
// whenever a player took a moment to start. A session instead wants a
// BLOCKING play: hand over the path, come back when the sound has finished.
// paplay/aplay/ffplay all do exactly that, so we prefer them and keep the
// open-and-sleep behaviour only as the last resort.
//
// A blocking player must also ask for a SMALL buffer. Left at its default,
// paplay requests ~186 ms (node.latency 8192/44100), which drives PipeWire's
// graph to its 2048-frame quantum, and a USB S/PDIF DAC then underruns all
// through the clip: measured with pw-top on the listener's machine, 36-40
// xruns per R/C/T round with plain paplay, pw-play or aplay, and 0 with
// paplay --latency-msec=20/50 or ffplay. The listener heard every clip,
// the reference included, as "lagged / chopped". A loopback of the sink
// monitor cannot see this -- the dropouts happen at the device write.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// player plays one clip and returns once it has finished sounding.
type player interface{ Play(path string) error }

// execPlayer runs a command-line player with the clip path as its final
// argument and waits for it to exit.
type execPlayer struct {
	bin  string
	args []string
}

func (p execPlayer) Play(path string) error {
	return exec.Command(p.bin, append(append([]string{}, p.args...), path)...).Run()
}

// funcPlayer adapts a bare function to the player interface, so the
// open-and-sleep fallback (and tests) need no type of their own.
type funcPlayer func(string) error

func (f funcPlayer) Play(path string) error { return f(path) }

// playerLatencyMsec is the buffer the blocking players ask for (see above).
const playerLatencyMsec = "50"

// detectPlayer picks the first blocking player available: $BEATMO_PLAYER,
// then paplay and ffplay (both with a small buffer: 0 xruns measured), then
// aplay; else the caller's open-and-sleep fallback.
//
// env and look are injected rather than read straight from the process so the
// preference order is testable without depending on what happens to be
// installed on the machine running the test.
func detectPlayer(env func(string) string, look func(string) (string, error), sleepFallback func(string) error) player {
	if v := env("BEATMO_PLAYER"); v != "" {
		parts := strings.Fields(v)
		return execPlayer{bin: parts[0], args: parts[1:]}
	}
	for _, c := range []execPlayer{
		{bin: "paplay", args: []string{"--latency-msec=" + playerLatencyMsec}},
		{bin: "ffplay", args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}},
		{bin: "aplay", args: []string{"-q", "--buffer-time=" + playerLatencyMsec + "000"}},
	} {
		if p, err := look(c.bin); err == nil {
			c.bin = p
			return c
		}
	}
	if sleepFallback == nil {
		return funcPlayer(func(path string) error { return fmt.Errorf("no audio player found; set BEATMO_PLAYER") })
	}
	return funcPlayer(sleepFallback)
}

// clipGap is the silence a session leaves after each clip. A blocking
// player returns the moment a clip ends, so without it R, C and T ran
// together as one chopped sound (measured on a loopback capture of `make
// ab-tuned`); the open-and-sleep loop of `make ab-all` always left ~0.6 s.
const clipGap = 600 * time.Millisecond

// gappedPlayer plays a clip, then holds clipGap of silence.
type gappedPlayer struct {
	p     player
	gap   time.Duration
	sleep func(time.Duration)
}

func (g gappedPlayer) Play(path string) error {
	err := g.p.Play(path)
	g.sleep(g.gap)
	return err
}

// defaultPlayer is the production wiring: the real environment and PATH, with
// the open-and-sleep fallback sized to the clip duration plus a little slack
// so two clips never overlap even when the desktop player is slow to start.
// A blocking player gets the same pause between clips.
func defaultPlayer(durSec float64) player {
	p := detectPlayer(os.Getenv, exec.LookPath, func(path string) error {
		open := "xdg-open"
		if _, err := exec.LookPath("open"); err == nil {
			open = "open"
		}
		_ = exec.Command(open, path).Start()
		time.Sleep(time.Duration(durSec*float64(time.Second)) + clipGap)
		return nil
	})
	if _, ok := p.(execPlayer); ok {
		return gappedPlayer{p: p, gap: clipGap, sleep: time.Sleep}
	}
	return p
}
