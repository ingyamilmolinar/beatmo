//go:build !js

package main

// A BLOCKING play (paplay/aplay/ffplay), with open-and-sleep only as the last resort: xdg-open returns at once and the
// loop had to guess clip lengths, so clips overlapped. And a SMALL buffer: paplay's default drives PipeWire to a 2048-frame
// quantum, and a USB DAC then underruns through every clip (dropouts at the device write, invisible to a loopback).

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

// $BEATMO_PLAYER, then paplay and ffplay (small buffer), then aplay, else the fallback. env and look are injected so
// the order is testable whatever is installed.
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

// A blocking player returns the moment a clip ends, so without a gap R, C and T run together as one chopped sound.
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

// The fallback sleeps the clip's duration plus slack, so a slow desktop player never overlaps two clips.
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
