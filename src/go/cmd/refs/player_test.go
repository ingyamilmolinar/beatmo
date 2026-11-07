//go:build !js

package main

import (
	"errors"
	"slices"
	"testing"
)

func TestDetectPlayer_PrefersEnvThenPaplay(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "paplay" || name == "ffplay" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	p := detectPlayer(func(string) string { return "" }, look, nil)
	if p.(execPlayer).bin != "/usr/bin/paplay" {
		t.Fatalf("want paplay, got %+v", p)
	}
	p = detectPlayer(func(k string) string {
		if k == "BEATMO_PLAYER" {
			return "mpv"
		}
		return ""
	}, look, nil)
	if p.(execPlayer).bin != "mpv" {
		t.Fatalf("env must win, got %+v", p)
	}
	fallbackHit := false
	p = detectPlayer(func(string) string { return "" }, func(string) (string, error) { return "", errors.New("no") },
		func(string) error { fallbackHit = true; return nil })
	_ = p.Play("x.wav")
	if !fallbackHit {
		t.Fatal("with no binary the sleep fallback must be used")
	}
}

// At its default buffer a blocking player drove PipeWire to a 2048-frame quantum and a USB DAC underran through every
// clip; each must request a small one.
func TestBlockingPlayersAskForASmallBuffer(t *testing.T) {
	want := map[string]string{
		"paplay": "--latency-msec=" + playerLatencyMsec,
		"aplay":  "--buffer-time=" + playerLatencyMsec + "000",
		"ffplay": "-nodisp", // ffplay's own buffer measured 0 xruns
	}
	for bin, arg := range want {
		look := func(name string) (string, error) {
			if name == bin {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		}
		p, ok := detectPlayer(func(string) string { return "" }, look, nil).(execPlayer)
		if !ok || p.bin != "/usr/bin/"+bin || !slices.Contains(p.args, arg) {
			t.Errorf("%s: got %+v, want %q in its arguments", bin, p, arg)
		}
	}
}
