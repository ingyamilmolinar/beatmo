//go:build !test && !js

package main

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestTimbreAttribution validates TimbreDistance as an instrument classifier:
// it renders each melodic reference's candidate pool ONCE (fingerprints cached),
// then sweeps several TimbreWeights configs, reporting how often the EXPECTED
// instrument ranks #1 / top-3. Run:
//
//	go test -run TestTimbreAttribution -v ./cmd/synth-match/
func TestTimbreAttribution(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	pool := []string{
		"violin", "cello", "harp",
		"flute", "oboe", "sax",
		"trumpet", "french-horn",
		"piano-grand", "piano-felt", "organ",
		"guitar-nylon", "guitar-steel",
		"bass-guitar", "sub-bass",
		"fm-lead", "fm-bell",
	}
	type mref struct {
		file   string
		f0     float64
		expect []string // acceptable correct ids
	}
	refs := []mref{
		{"127160__daphne_in_wonderland__celtic_harp_g2.wav", 98.00, []string{"harp"}},
		{"22686__acclivity__oboe_a_440.wav", 440.00, []string{"oboe"}},
		{"257996__xserra__cello-d2.wav", 73.42, []string{"cello"}},
		{"356181__mtg__violin-d5.wav", 587.33, []string{"violin"}},
		{"360251__mtg__sax-baritone-c3.wav", 130.81, []string{"sax"}},
		{"373313__sgossner__flute-expressive-sustain-e5-ldflute_expvib_e4_v1_1.wav", 659.25, []string{"flute"}},
		{"374162__sgossner__trumpet-sustain-f4-sum_shtrumpet_sus_g3_v3_rr1.wav", 349.23, []string{"trumpet"}},
		{"682231__henkonen__fhorn-18.wav", 0, []string{"french-horn"}},
		{"68448__pinkyfinger__piano-g.wav", 0, []string{"piano-grand", "piano-felt"}},
	}

	type entry struct {
		file   string
		expect []string
		refFP  fingerprint.Fingerprint
		cand   map[string]fingerprint.Fingerprint
	}
	var entries []entry
	for _, r := range refs {
		pcm, sr, err := audio.DecodeWAVToPCM(root + r.file)
		if err != nil {
			t.Logf("skip %s: %v", r.file, err)
			continue
		}
		seg := fingerprint.AutoSegment(wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}, fingerprint.SustainLenSec)
		refFP := fingerprint.FromWave(seg, "ref")
		f0 := r.f0
		if f0 <= 0 {
			f0 = fingerprint.DetectF0(seg)
		}
		pitch := 0.0
		if f0 > 0 {
			pitch = 12 * math.Log2(f0/220.0)
		}
		cand := map[string]fingerprint.Fingerprint{}
		for _, id := range pool {
			cw, err := synthmatch.RenderInstrument(id, pitch, sr, 2.0)
			if err != nil || len(cw.Samples) == 0 {
				continue
			}
			cand[id] = fingerprint.FromWave(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), id)
		}
		entries = append(entries, entry{r.file, r.expect, refFP, cand})
	}
	fmt.Printf("\ncached %d refs × %d candidates\n", len(entries), len(pool))

	configs := []struct {
		name string
		w    fingerprint.TimbreWeights
	}{
		{"default", fingerprint.DefaultTimbreWeights},
		{"profile-only", fingerprint.TimbreWeights{Profile: 1}},
		{"mfcc-only", fingerprint.TimbreWeights{MFCC: 1}},
		{"profile+mfcc", fingerprint.TimbreWeights{Profile: 3, MFCC: 2}},
		{"profile+formant", fingerprint.TimbreWeights{Profile: 3, MFCC: 1, Formant: 2, EvenOdd: 1}},
		{"all-strong-profile", fingerprint.TimbreWeights{Profile: 4, MFCC: 2, Tristimulus: 1.5, EvenOdd: 1, Centroid: 0.5, Inharmonic: 0.5, Flatness: 0.5, Formant: 1.5}},
	}
	for _, cfg := range configs {
		top1, top3, n := 0, 0, 0
		var detail []string
		for _, e := range entries {
			type sc struct {
				id string
				d  float64
			}
			var scores []sc
			for id, fp := range e.cand {
				scores = append(scores, sc{id, fingerprint.TimbreDistanceWeighted(e.refFP, fp, cfg.w)})
			}
			sort.Slice(scores, func(i, j int) bool { return scores[i].d < scores[j].d })
			rank := -1
			for i, s := range scores {
				if contains(e.expect, s.id) {
					rank = i + 1
					break
				}
			}
			n++
			if rank == 1 {
				top1++
			}
			if rank >= 1 && rank <= 3 {
				top3++
			}
			best := ""
			if len(scores) > 0 {
				best = scores[0].id
			}
			detail = append(detail, fmt.Sprintf("%-42s best=%-14s exp=%v rank=%d", short(e.file), best, e.expect, rank))
		}
		fmt.Printf("\n=== weights %-18s  top1=%d/%d  top3=%d/%d ===\n", cfg.name, top1, n, top3, n)
		if cfg.name == "default" || cfg.name == "all-strong-profile" {
			for _, d := range detail {
				fmt.Println("   " + d)
			}
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func short(f string) string {
	if len(f) > 42 {
		return f[:42]
	}
	return f
}
