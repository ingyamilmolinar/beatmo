//go:build !test && !js

package main

import (
	"runtime"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Pitch, length, level, room and reverb are what a player and a recording change: each voice, played that way and
// ranked against the WHOLE roster as refs match ranks a download, must place itself (or an identical twin) in the top 3.
// The cases span every kind of sound, chosen before measuring, plus the pitch failures -nuisance found.
func TestEssenceSelfIdentifiesUnderNuisance(t *testing.T) {
	if testing.Short() {
		t.Skip("renders the whole roster per case")
	}
	cases := []struct {
		inst  instrument.ID
		semis float64
	}{
		{roster.PianoGrand, 5}, {roster.Cello, -3}, {roster.Trumpet, 2}, {roster.Flute, 4},
		{roster.GuitarNylon, -5}, {roster.Marimba, 3}, {roster.Organ, -2}, {roster.VoiceAhh, 1},
		{roster.Clarinet, -4}, {roster.Kick, 2}, {roster.Snare, -2}, {roster.Hihat, 3}, {roster.Conga, -3},
		// Inharmonic voices whose pitch reading flips under a nuisance, and a drum read an octave low.
		{roster.Kalimba, 3}, {roster.Cowbell, -2}, {roster.Tom, 5},
	}
	ids := rosterIDs()
	workers := max(1, runtime.NumCPU()-1)
	for _, c := range cases {
		t.Run(string(c.inst), func(t *testing.T) {
			n := matchNuisance{Name: "self-id", Semis: c.semis, Hold: 120.0 / 70, Room: true, WetDB: -12}
			rank, f0, top, err := rankNuisance(c.inst, n, ids, workers)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("f0 %.1f Hz: itself at #%d, top %v (first-ranked kind %s, own %s)", f0, rank, top, broadClass(top[0]), broadClass(c.inst))
			if rank < 1 || rank > 3 {
				t.Errorf("%s played %+g st, slower, quieter, in a room: ranked itself #%d (top %v)", c.inst, c.semis, rank, top)
			}
		})
	}
}
