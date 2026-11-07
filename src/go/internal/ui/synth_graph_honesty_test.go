//go:build !test && !js

package ui

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// On the real engine: a control that changes the sound moves its graph, and the printed numbers rank sounds like the
// descriptors the dial tests trust, so a still graph means an unchanged sound.

const (
	honestyW, honestyH = relevanceViewW, relevanceViewH // the reference graph size
	honestyMinPx       = 3.0                            // a visible move: 3 px between the two ends
)

type honestyCase struct {
	inst       instrument.ID
	card, dial string
	view       synthView
	lo, hi     map[string]float64
}

func honestyCases(t *testing.T) []honestyCase {
	t.Helper()
	var out []honestyCase
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		merged := audio.MergedInstrumentParamsRO(id)
		voice := 0.0
		if d, ok := audio.InstrumentDoc(id); ok {
			voice = d.Render.Seconds
		}
		for _, cg := range productionMacroCards(shownRootMacros(id, voice)) {
			for _, d := range cg.Dials {
				v, ok := synthViewByName(d.Dial.View, merged)
				if !ok {
					t.Fatalf("%s %s/%s: unknown view %q", id, cg.ID, d.Dial.ID, d.Dial.View)
				}
				start := map[string]float64{}
				for _, term := range d.Terms {
					if val, ok := merged[term.Param]; ok {
						start[term.Param] = val
					}
				}
				out = append(out, honestyCase{inst: id, card: cg.ID, dial: d.Dial.ID, view: v,
					lo: d.Turn(start, 0), hi: d.Turn(start, 1)})
			}
		}
	}
	return out
}

func renderForHonesty(id instrument.ID, fields map[string]float64, ident string) (*graphRender, error) {
	doc, err := audio.InstrumentDocWithFields(id, fields)
	if err != nil {
		return nil, err
	}
	pcm, sr, err := audio.RenderDocForGraph(id, doc, 0)
	if err != nil {
		return nil, err
	}
	voice := audio.GraphVoiceSec(doc)
	an := soundview.New(pcm, sr, voice)
	an.Warm(soundview.NeedAll, nil)
	return &graphRender{ident: graphIdent{hash: ident}, sr: sr, voiceSec: voice, an: an}, nil
}

// The view chooseDialView shows for this dial and instrument.
func graphMove(v synthView, lo, hi *graphRender) (px float64, readoutDiffers bool) {
	v, px = chooseDialView(v, lo, hi, honestyW, honestyH)
	a := buildSynthViewPlot(v, honestyW, honestyH, synthViewInput{now: lo, numbers: true})
	b := buildSynthViewPlot(v, honestyW, honestyH, synthViewInput{now: hi, numbers: true})
	return px, a.readout != b.readout
}

func TestEveryControlMovesItsGraph(t *testing.T) {
	cases := honestyCases(t)
	if len(cases) < 200 {
		t.Fatalf("only %d dial cases across the roster — the enumeration is broken", len(cases))
	}
	type result struct {
		key  string
		px   float64
		text bool
		err  error
	}
	results := make([]result, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, c := range cases {
		wg.Add(1)
		go func(i int, c honestyCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			key := fmt.Sprintf("%s/%s/%s", c.inst, c.card, c.dial)
			lo, err := renderForHonesty(c.inst, c.lo, key+"/lo")
			if err != nil {
				results[i] = result{key: key, err: err}
				return
			}
			hi, err := renderForHonesty(c.inst, c.hi, key+"/hi")
			if err != nil {
				results[i] = result{key: key, err: err}
				return
			}
			px, text := graphMove(c.view, lo, hi)
			results[i] = result{key: key, px: px, text: text}
		}(i, c)
	}
	wg.Wait()

	failed := 0
	for _, r := range results {
		if r.err != nil {
			t.Errorf("%s: %v", r.key, r.err)
			continue
		}
		// The curve must move; a changing readout over a still picture shows nothing.
		if r.px < honestyMinPx {
			failed++
			t.Errorf("%s: the dial is shown (its sound changes audibly) but turning it end to end moves its graph %.1f px (readout changes: %v) — the view does not show this change", r.key, r.px, r.text)
		}
	}
	t.Logf("%d shown dials, %d whose graph does not move", len(results), failed)
}

// Different definitions of the same properties must rank the roster alike (Spearman ρ). Tone vs noise ranks against
// spectral flatness: noise.fraction counts every inharmonic partial as noise, the graph any peak as tone.
func TestSoundviewAgreesWithFingerprint(t *testing.T) {
	type pair struct{ sv, fp []float64 }
	props := map[string]*pair{"attack": {}, "audible": {}, "centroid": {}, "noise": {}}
	reg := fingerprint.DefaultRegistry()
	for _, id := range roster.All {
		d, ok := audio.EnsureInstrumentDoc(id)
		if !ok {
			continue
		}
		pcm, sr, err := audio.RenderDocForGraph(id, d, 0)
		if err != nil || len(pcm) == 0 {
			continue
		}
		an := soundview.New(pcm, sr, audio.GraphVoiceSec(d))
		w := wave.Wave{SampleRate: sr, Samples: make([]float64, len(pcm))}
		for i, s := range pcm {
			w.Samples[i] = float64(s)
		}
		b := fingerprint.NewBundle(w, string(id))
		add := func(name string, sv float64, descID string) {
			desc, ok := reg.ByID(descID)
			if !ok {
				t.Fatalf("fingerprint has no descriptor %q", descID)
			}
			if desc.Applies != nil && !desc.Applies(b) {
				return
			}
			if m := desc.Extract(b); m.OK && !math.IsNaN(m.Scalar) && !math.IsNaN(sv) {
				props[name].sv = append(props[name].sv, sv)
				props[name].fp = append(props[name].fp, m.Scalar)
			}
		}
		add("attack", an.AttackSec(), fingerprint.IDEnvAttack)
		add("audible", an.AudibleSec(), fingerprint.IDEnvTotalAudible)
		add("centroid", an.CentroidHz(), fingerprint.IDCentroid)
		add("noise", an.Noise().Fraction, fingerprint.IDNoiseFlatness)
	}
	// Each floor ~0.05 under the measured ρ.
	floors := map[string]float64{"attack": 0.8, "audible": 0.9, "centroid": 0.8, "noise": 0.5}
	for name, p := range props {
		if len(p.sv) < 50 {
			t.Errorf("%s: only %d instruments measured", name, len(p.sv))
			continue
		}
		rho := spearman(p.sv, p.fp)
		t.Logf("%-9s ρ = %.3f over %d voices", name, rho, len(p.sv))
		if rho < floors[name] {
			t.Errorf("%s: the graph's readout ranks the voices unlike fingerprint (ρ %.3f < %.2f)", name, rho, floors[name])
		}
	}
}

func spearman(a, b []float64) float64 {
	ra, rb := ranks(a), ranks(b)
	n := float64(len(a))
	var ma, mb float64
	for i := range ra {
		ma += ra[i]
		mb += rb[i]
	}
	ma, mb = ma/n, mb/n
	var cov, va, vb float64
	for i := range ra {
		cov += (ra[i] - ma) * (rb[i] - mb)
		va += (ra[i] - ma) * (ra[i] - ma)
		vb += (rb[i] - mb) * (rb[i] - mb)
	}
	if va == 0 || vb == 0 {
		return 0
	}
	return cov / math.Sqrt(va*vb)
}

func ranks(v []float64) []float64 {
	idx := make([]int, len(v))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return v[idx[i]] < v[idx[j]] })
	r := make([]float64, len(v))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && v[idx[j+1]] == v[idx[i]] {
			j++
		}
		for k := i; k <= j; k++ {
			r[idx[k]] = float64(i+j) / 2
		}
		i = j + 1
	}
	return r
}
