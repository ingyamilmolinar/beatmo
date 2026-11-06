//go:build !js

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// validate_loss.go — `refs validate -loss` (spec §4b): does each term of the
// WAV-clone loss agree with the listener? Only rankings WITHIN a family mean
// anything (a cymbal's spectral error and a bassoon's are on different
// scales), and every preference pair is within one family by construction,
// so the per-term agreement over the pairs is the within-family test. A term
// that agrees with the ear less than half the time over ≥ 10 pairs is ANTI
// and gets weight 0. fingerprint.Distance is scored the same way for
// comparison. Advisory: the ear accepts candidates, the loss only ranks them.

const (
	lossAntiMinPairs = 10
	lossTermTotal    = "loss.total"
	lossTermDistance = "fingerprint.distance"
)

var lossTermOrder = []string{"loss.mrstft", "loss.envelope", "loss.descriptors", lossTermTotal, lossTermDistance}

// lossTermNorms is one clip's per-term distance from the reference (lower =
// closer), in scorePreferencesFromNorms's currency.
func lossTermNorms(rep clone.LossReport, distance float64) map[string]float64 {
	return map[string]float64{
		"loss.mrstft": rep.MRSTFT, "loss.envelope": rep.Envelope, "loss.descriptors": rep.Descriptors,
		lossTermTotal: rep.MRSTFT + rep.Envelope/10 + rep.Descriptors, lossTermDistance: distance,
	}
}

// lossVerdict classifies a term's agreement.
func lossVerdict(agree, total int) string {
	switch {
	case total < lossAntiMinPairs:
		return "too few pairs"
	case 2*agree < total:
		return "ANTI → weight 0"
	default:
		return "agrees"
	}
}

func runValidateLoss(w, diag io.Writer, verdictsPath, sessionsPath, clonesRoot string) error {
	vs, err := readVerdicts(verdictsPath)
	if err != nil {
		return err
	}
	ss, _ := readSessions(sessionsPath)
	pairs := append(prefPairsFromVerdicts(vs), prefPairsFromSessions(ss)...)
	cat, err := loadCatalogue()
	if err != nil {
		return err
	}
	type refState struct {
		lref  *clone.LossRef
		fp    fingerprint.Fingerprint
		pitch float64
		dur   float64
	}
	refs := map[string]*refState{}
	refOf := func(id string) (*refState, error) {
		if r, ok := refs[id]; ok {
			return r, nil
		}
		ref, ok := cat.Ref(id)
		if !ok || ref.Measured == nil {
			return nil, fmt.Errorf("unknown or unmeasured reference %q", id)
		}
		path, ok := wavref.Resolve(id)
		if !ok {
			return nil, fmt.Errorf("%s is not available locally", id)
		}
		p, err := clone.PrepareFile(path, clone.PrepareOptions{Label: id, SemisFromA3: ref.Measured.SemisFromA3, HasSemis: true})
		if err != nil {
			return nil, err
		}
		real := wave.Wave{Samples: p.Samples, SampleRate: p.Rate, Label: id}
		r := &refState{lref: clone.NewLossRef(p, 3), fp: fingerprint.FromWave(real, id),
			pitch: renderPitch(*ref.Measured, fingerprint.DetectF0Detailed(real).Reliable()), dur: ref.Measured.DurationSec}
		refs[id] = r
		return r, nil
	}
	normsOf := func(pr prefPair, c sessionClip) map[string]float64 {
		r, err := refOf(pr.RefID)
		if err != nil {
			fmt.Fprintf(diag, "loss: skipping %s: %v\n", pr.Family, err)
			return nil
		}
		var x wave.Wave
		if c.DocSHA != "" {
			p := filepath.Join(clonesRoot, string(pr.Family), "docs", strings.TrimPrefix(c.DocSHA, "sha256:")+".json")
			b, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(diag, "loss: skipping %s/%s: clone document not kept locally (%v)\n", pr.Family, c.ID, err)
				return nil
			}
			d, err := instrument.ParseFile(b)
			if err != nil {
				fmt.Fprintf(diag, "loss: skipping %s/%s: %v\n", pr.Family, c.ID, err)
				return nil
			}
			x, err = synthmatch.RenderShippedDoc(d, r.pitch, r.lref.Rate, r.dur)
			if err != nil {
				return nil
			}
		} else {
			x, err = synthmatch.RenderShipped(c.renderInst(), r.pitch, r.lref.Rate, r.dur, c.Overrides)
			if err != nil {
				fmt.Fprintf(diag, "loss: skipping %s/%s: %v\n", pr.Family, c.ID, err)
				return nil
			}
		}
		rep := r.lref.Score(x.Samples, 0, clone.LossWeights{MRSTFT: 1, Envelope: 1, Descriptors: 1}, clone.LossReport{})
		return lossTermNorms(rep, fingerprint.Distance(r.fp, fingerprint.FromWave(x, c.ID)).Total)
	}
	ch := make([]map[string]float64, len(pairs))
	rj := make([]map[string]float64, len(pairs))
	for i, p := range pairs {
		ch[i], rj[i] = normsOf(p, p.Chosen), normsOf(p, p.Rejected)
	}
	printLossAgreement(w, len(pairs), scorePreferencesFromNorms(pairs, ch, rj))
	return nil
}

func printLossAgreement(w io.Writer, pairs int, scores map[string][2]int) {
	fmt.Fprintf(w, "WAV-clone loss terms vs %d within-family preference pairs (agree = the clip the listener preferred scored lower)\n\n", pairs)
	fmt.Fprintf(w, "%-22s %9s  %s\n", "term", "agree", "verdict")
	ids := append([]string(nil), lossTermOrder...)
	for id := range scores {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		sc := scores[id]
		fmt.Fprintf(w, "%-22s %4d/%-4d  %s\n", id, sc[0], sc[1], lossVerdict(sc[0], sc[1]))
	}
}
