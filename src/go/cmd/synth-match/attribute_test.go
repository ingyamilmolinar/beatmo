//go:build !test && !js

package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestAttributeReferences ranks, for each reference WAV in the repo root, which
// synth INSTRUMENT most closely matches it — kicks through the enriched
// KickDistance (roughness / T60 / flux / inharmonicity / LAT included), melodic
// & percussion through the general fingerprint.Distance, each candidate rendered
// at the reference's own pitch. Diagnostic: run
//
//	go test -run TestAttributeReferences -v ./cmd/synth-match/
func TestAttributeReferences(t *testing.T) {
	const root = "/home/ymolinar/Repos/beatmo/"
	audio.Reset()
	audio.ResetInstruments()

	kickPool := audio.InstrumentsByCategory(audio.CatKick)
	// Curated, representative melodic pool (one–two per family). FromWave analysis
	// is ~1.6 s/candidate, so the full ~35-instrument catalogue × 11 refs is too
	// slow; this spans every family incl. the expected matches for each ref.
	melodicPool := []string{
		"violin", "cello", "harp",
		"flute", "oboe", "sax",
		"trumpet", "french-horn",
		"piano-grand", "piano-felt", "organ",
		"guitar-nylon", "guitar-steel",
		"bass-guitar", "sub-bass",
		"fm-lead", "fm-bell",
	}
	percPool := []string{
		"conga", "conga-open", "conga-tumba", "clap", "clap-tight",
		"cowbell", "shaker", "tom", "tom-1", "high-tom-organic",
	}

	type ref struct {
		file, kind, expect string
		f0Hz               float64 // known pitch (0 → auto-detect)
	}
	refs := []ref{
		{"36010__sandyrb__dnb-kick-003.wav", "kick", "dnb / acoustic kick", 0},
		{"403244__yellowtree__hybrid-kick-1.wav", "kick", "hybrid kick", 0},
		{"412509__dflee4__kick-stuborn.wav", "kick", "?", 0},
		{"83768__zgump__kick-pack-0708.wav", "kick", "organic (zgump) kick", 0},
		{"455509__mrrentapercussionist__lp-congas-quinto-muted-slap.wav", "perc", "conga", 0},
		{"127160__daphne_in_wonderland__celtic_harp_g2.wav", "melodic", "harp", 98.00},                           // G2
		{"22686__acclivity__oboe_a_440.wav", "melodic", "oboe", 440.00},                                          // A4
		{"257996__xserra__cello-d2.wav", "melodic", "cello", 73.42},                                              // D2
		{"356181__mtg__violin-d5.wav", "melodic", "violin", 587.33},                                              // D5
		{"360251__mtg__sax-baritone-c3.wav", "melodic", "sax", 130.81},                                           // C3
		{"373313__sgossner__flute-expressive-sustain-e5-ldflute_expvib_e4_v1_1.wav", "melodic", "flute", 659.25}, // E5
		{"374162__sgossner__trumpet-sustain-f4-sum_shtrumpet_sus_g3_v3_rr1.wav", "melodic", "trumpet", 349.23},   // F4
		{"682231__henkonen__fhorn-18.wav", "melodic", "french horn", 0},
		{"68448__pinkyfinger__piano-g.wav", "melodic", "piano", 0},
		{"826038__ixwolf__g1_rr1.wav", "melodic", "? (G1 low)", 49.00}, // G1
		{"117852__kyster__2-oct-c.wav", "melodic", "? (2-oct C)", 0},
	}

	for _, r := range refs {
		pcm, sr, err := audio.DecodeWAVToPCM(root + r.file)
		if err != nil {
			fmt.Printf("\n■ %-52s SKIP (%v)\n", r.file, err)
			continue
		}
		refWave := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}
		fmt.Printf("\n■ %s  [expect: %s]\n", r.file, r.expect)
		switch r.kind {
		case "kick":
			rankKick(refWave, kickPool)
		case "perc":
			rankMelodic(refWave, percPool, r.f0Hz, true) // percussion: render at natural pitch
		default:
			rankMelodic(refWave, melodicPool, r.f0Hz, false)
		}
	}
}

type scored struct {
	id string
	d  float64
}

func printTop(scores []scored, n int) {
	sort.Slice(scores, func(i, j int) bool { return scores[i].d < scores[j].d })
	for i := 0; i < n && i < len(scores); i++ {
		mark := "  "
		if i == 0 {
			mark = "→ "
		}
		fmt.Printf("   %s%d. %-28s dist=%.4f\n", mark, i+1, scores[i].id, scores[i].d)
	}
}

func rankKick(refWave wave.Wave, pool []string) {
	refFP := fingerprint.KickAnalyze(refWave)
	var scores []scored
	for _, id := range pool {
		buf, csr := audio.RenderInstrumentOneShotRaw(id)
		if len(buf) == 0 {
			continue
		}
		candFP := fingerprint.KickAnalyze(wave.Wave{Samples: f32ToF64(buf), SampleRate: csr})
		d := fingerprint.KickDistanceWeighted(refFP, candFP, fingerprint.DefaultKickWeights).Total
		scores = append(scores, scored{id, d})
	}
	printTop(scores, 5)
}

func rankMelodic(refWave wave.Wave, pool []string, knownF0 float64, natural bool) {
	seg := fingerprint.AutoSegment(refWave, fingerprint.SustainLenSec)
	refFP := fingerprint.FromWave(seg, "ref")
	f0 := knownF0
	if f0 <= 0 {
		f0 = fingerprint.DetectF0(seg)
	}
	pitch := 0.0
	if f0 > 0 && !natural {
		pitch = 12 * math.Log2(f0/220.0)
	}
	fmt.Printf("   (f0=%.1f Hz → pitch=%.1f st%s)\n", f0, pitch, map[bool]string{true: " [natural]", false: ""}[natural])
	var scores []scored
	for _, id := range pool {
		cw, err := synthmatch.RenderInstrument(id, pitch, refWave.SampleRate, 2.0)
		if err != nil || len(cw.Samples) == 0 {
			continue
		}
		candFP := fingerprint.FromWave(fingerprint.AutoSegment(cw, fingerprint.SustainLenSec), "cand")
		d := fingerprint.Distance(refFP, candFP).Total
		scores = append(scores, scored{id, d})
	}
	printTop(scores, 5)
}

var _ = os.Stat
