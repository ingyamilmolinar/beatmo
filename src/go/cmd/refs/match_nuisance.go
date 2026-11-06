//go:build !js

package main

// match_nuisance.go -- `refs match -nuisance`: does the matcher hear the
// INSTRUMENT, whatever the player and the recording did to the note?
//
// Every case re-plays a shipped voice with ONE nuisance -- a different note,
// a note cut short (half its shipped length, and the shipped length where a
// drum stops), a room, a reverb, a brighter or darker microphone -- and
// ranks the whole roster against it exactly as `refs
// match` ranks a download. The voice must still find itself (or a twin with
// the identical shipped sound): whatever moves its rank is something the
// essence distance mistakes for identity. The `all` row applies them at once.
// It needs no reference audio. TestEssenceSelfIdentifiesUnderNuisance runs
// the combined case on a fixed set as a gate; this sweeps each nuisance on
// its own so a regression says WHICH nuisance leaked into the ranking.

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// matchNuisance is what a player or a recording changes about a note and
// the instrument does not.
type matchNuisance struct {
	Name string
	// Semis plays the note this far from the voice's natural pitch.
	Semis float64
	// Hold sets the note length as a multiple of the shipped voice's
	// render.seconds. 0 = the voice rings out the way a recording of it does
	// (synthmatch.HeldSeconds the analysis window): the shipped conga is a
	// 0.25 s voice, cut off mid-ring -- a recording never is, and a reverb
	// tail after that cut was judged against the matcher's (correctly)
	// un-truncated conga.
	Hold float64
	// Room puts the note 10 dB down in a room: 0.15 s of room tone before the
	// onset and a noise floor RoomFloorDB under the peak, as a download has.
	Room bool
	// WetDB is a small room's reverb (Schroeder), relative to the dry note;
	// 0 = dry.
	WetDB float64
	// ShelfDB is a high shelf at 2 kHz: a brighter (+) or darker (-)
	// microphone, or the row's EQ -- both within a user's reach.
	ShelfDB float64
}

// matchRoomFloorDB is the room's noise floor under the note's peak.
const matchRoomFloorDB = -55.0

// matchNuisances is the sweep `refs match -nuisance` runs, each nuisance
// alone, then all of them at once.
var matchNuisances = []matchNuisance{
	{Name: "none"},
	{Name: "pitch +5", Semis: 5},
	{Name: "pitch -5", Semis: -5},
	{Name: "short note", Hold: 0.5},
	{Name: "app length", Hold: 1}, // drums cut at their shipped length
	{Name: "room", Room: true},
	{Name: "reverb", WetDB: -9},
	{Name: "bright mic", ShelfDB: 6},
	{Name: "dark mic", ShelfDB: -6},
	{Name: "all", Semis: 3, Hold: 1.5, Room: true, WetDB: -12, ShelfDB: -4},
}

// matchNuisanceCases are the voices the sweep re-plays: every broad kind the
// roster has, chosen before measuring.
var matchNuisanceCases = []instrument.ID{
	roster.Violin, roster.Cello, roster.DoubleBass, roster.Trumpet, roster.FrenchHorn, roster.Tuba, roster.Oboe, roster.Clarinet, roster.Sax, roster.Bassoon,
	roster.Flute, roster.PanFlute, roster.VoiceAhh, roster.VoiceSoprano, roster.Organ, roster.OrganChurch, roster.Accordion,
	roster.PianoGrand, roster.PianoRhodes, roster.Clavinet, roster.Harp, roster.Harpsichord, roster.GuitarNylon, roster.GuitarElectric,
	roster.BassGuitar, roster.Koto, roster.Marimba, roster.Vibraphone, roster.Glockenspiel, roster.TubularBells, roster.Kalimba,
	roster.LeadSupersaw, roster.PwmPad, roster.FmBass, roster.Bass808, roster.Kick, roster.Kick808, roster.KickAcoustic, roster.Snare,
	roster.SnareAcoustic, roster.Rimshot, roster.Tom, roster.Conga, roster.Hihat, roster.Ride, roster.Crash, roster.Clap, roster.Shaker, roster.Cowbell,
}

// nuisanceRecording is inst played the way n says, as a recording of it
// would arrive: room tone, reverb and microphone included.
func nuisanceRecording(inst instrument.ID, n matchNuisance) (wave.Wave, error) {
	sr := wavref.Rate
	voiceSec := synthmatch.HeldSeconds(inst, n.Semis, adoptMaxSecDefault+matchVoiceMarginSec)
	if n.Hold > 0 {
		voiceSec = 0
		if d, ok := audio.InstrumentDoc(inst); ok {
			voiceSec = n.Hold * d.Render.Seconds
		}
	}
	w, err := synthmatch.RenderHeld(inst, n.Semis, sr, 0, voiceSec, nil)
	if err != nil {
		return wave.Wave{}, err
	}
	pre := 0
	if n.Room {
		pre = int(0.15 * float64(sr))
	}
	dry := append(make([]float64, pre), w.Samples...)
	dry = append(dry, make([]float64, int(0.6*float64(sr)))...)
	out := append([]float64(nil), dry...)
	if n.WetDB != 0 {
		wet := schroederReverb(dry, sr)
		g := math.Pow(10, n.WetDB/20)
		for i := range out {
			out[i] += g * wet[i]
		}
	}
	if n.ShelfDB != 0 {
		out = highShelf(out, sr, 2000, n.ShelfDB)
	}
	if n.Room {
		var peak float64
		for _, v := range out {
			peak = math.Max(peak, math.Abs(v))
		}
		r := rand.New(rand.NewSource(int64(len(inst)) * 7919))
		floor := peak * math.Pow(10, matchRoomFloorDB/20)
		for i := range out {
			out[i] = 0.316*out[i] + 0.316*floor*(2*r.Float64()-1)
		}
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: string(inst)}, nil
}

// schroederReverb: four parallel feedback combs into two allpasses (the
// classic Freeverb-style room), deterministic.
func schroederReverb(x []float64, sr int) []float64 {
	scale := float64(sr) / 44100
	combs := []int{1557, 1617, 1491, 1422}
	out := make([]float64, len(x))
	for _, d := range combs {
		d = int(float64(d) * scale)
		buf := make([]float64, d)
		for i, v := range x {
			y := buf[i%d]
			buf[i%d] = v + 0.84*y
			out[i] += y / float64(len(combs))
		}
	}
	for _, d := range []int{556, 225} {
		d = int(float64(d) * scale)
		buf := make([]float64, d)
		for i, v := range out {
			b := buf[i%d]
			y := -v + b
			buf[i%d] = v + 0.5*b
			out[i] = y
		}
	}
	return out
}

// highShelf is the RBJ-cookbook high shelf (slope 0.5: a broad tilt, the
// shape of a microphone's presence region or a tone control) at fc.
func highShelf(x []float64, sr int, fc, gainDB float64) []float64 {
	a := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * fc / float64(sr)
	cw, sw := math.Cos(w0), math.Sin(w0)
	const slope = 0.5
	alpha := sw / 2 * math.Sqrt((a+1/a)*(1/slope-1)+2)
	sa := 2 * math.Sqrt(a) * alpha
	b0 := a * ((a + 1) + (a-1)*cw + sa)
	b1 := -2 * a * ((a - 1) + (a+1)*cw)
	b2 := a * ((a + 1) + (a-1)*cw - sa)
	a0 := (a + 1) - (a-1)*cw + sa
	a1 := 2 * ((a - 1) - (a+1)*cw)
	a2 := (a + 1) - (a-1)*cw - sa
	out := make([]float64, len(x))
	var x1, x2, y1, y2 float64
	for i, v := range x {
		y := (b0*v + b1*x1 + b2*x2 - a1*y1 - a2*y2) / a0
		x2, x1, y2, y1 = x1, v, y1, y
		out[i] = y
	}
	return out
}

// selfRank is the 1-based rank of inst (or a voice with its identical
// shipped sound) in ranked; 0 = absent.
func selfRank(ranked []matchRender, inst instrument.ID) int {
	key := audio.ShippedSoundKey(inst)
	for i, r := range ranked {
		if r.cand.Instrument == inst || audio.ShippedSoundKey(r.cand.Instrument) == key {
			return i + 1
		}
	}
	return 0
}

// rankNuisance plays inst under n and ranks the roster against it the way
// `refs match` ranks a download.
func rankNuisance(inst instrument.ID, n matchNuisance, ids []instrument.ID, workers int) (rank int, f0 float64, top []instrument.ID, err error) {
	raw, err := nuisanceRecording(inst, n)
	if err != nil {
		return 0, 0, nil, err
	}
	ref := wave.Wave{Samples: trimMatchClip(raw.Samples, raw.SampleRate, adoptMaxSecDefault), SampleRate: raw.SampleRate}
	f0 = matchRefPitch(ref)
	ranked, err := rankRoster(ref, f0, ids, workers, nil)
	if err != nil {
		return 0, f0, nil, err
	}
	for i := 0; i < 3 && i < len(ranked); i++ {
		top = append(top, ranked[i].cand.Instrument)
	}
	return selfRank(ranked, inst), f0, top, nil
}

// runMatchNuisance sweeps every case through every nuisance and reports how
// often the voice still finds itself.
func runMatchNuisance(out, diag io.Writer, cases []instrument.ID, nuisances []matchNuisance, workers int) error {
	ids := rosterIDs()
	type cell struct {
		rank int
		top  []instrument.ID
	}
	grid := make([][]cell, len(cases))
	for ci, inst := range cases {
		grid[ci] = make([]cell, len(nuisances))
		fmt.Fprintf(diag, "%-16s", inst)
		for ni, n := range nuisances {
			rank, _, top, err := rankNuisance(inst, n, ids, workers)
			if err != nil {
				return fmt.Errorf("%s / %s: %w", inst, n.Name, err)
			}
			grid[ci][ni] = cell{rank, top}
			fmt.Fprintf(diag, " %3d", rank)
		}
		fmt.Fprintln(diag)
	}
	tw := tabwriter.NewWriter(out, 0, 0, 1, ' ', tabwriter.AlignRight)
	hdr := "instrument\t"
	for _, n := range nuisances {
		hdr += n.Name + "\t"
	}
	fmt.Fprintln(tw, hdr)
	for ci, inst := range cases {
		row := string(inst) + "\t"
		for _, c := range grid[ci] {
			row += fmt.Sprintf("%d\t", c.rank)
		}
		fmt.Fprintln(tw, row)
	}
	tw.Flush()
	fmt.Fprintf(out, "\n%-12s %8s %8s %9s  %s\n", "nuisance", "#1", "top-3", "mean rank", "worst (rank: top 3)")
	for ni, n := range nuisances {
		var hit1, hit3, sum int
		type miss struct {
			inst instrument.ID
			c    cell
		}
		var misses []miss
		for ci, inst := range cases {
			c := grid[ci][ni]
			r := c.rank
			if r == 0 {
				r = len(ids)
			}
			sum += r
			if r == 1 {
				hit1++
			}
			if r <= 3 {
				hit3++
			} else {
				misses = append(misses, miss{inst, c})
			}
		}
		sort.SliceStable(misses, func(a, b int) bool { return misses[a].c.rank > misses[b].c.rank })
		var worst []string
		for _, m := range misses[:min(4, len(misses))] {
			worst = append(worst, fmt.Sprintf("%s #%d: %s", m.inst, m.c.rank, strings.Join(instrument.Strings(m.c.top), ",")))
		}
		fmt.Fprintf(out, "%-12s %3d/%-4d %3d/%-4d %9.1f  %s\n", n.Name, hit1, len(cases), hit3, len(cases),
			float64(sum)/float64(max(1, len(cases))), strings.Join(worst, "; "))
	}
	return nil
}
