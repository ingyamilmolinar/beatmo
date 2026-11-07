//go:build !js

package main

// A few "central" voices sit near every sustained recording and top ranking after ranking. CSLS-style scaling corrects
// it: each voice's distance minus ½ of its hub (the mean of its k smallest distances to the catalogued references; the
// whole hub over-rewards voices far from everything). A committed corpus calibration; a retuned voice gets the median.

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

//go:embed testdata/match_hubness.json
var matchHubnessJSON []byte

const (
	// Fixed before measuring; leave-one-out grades k = 5..20 about equally, so the choice is not a spike.
	matchHubnessK = 10
	// Bumped whenever the essence distance changes shape.
	matchHubnessVersion = 5
	// matchHubnessAlpha is the share of the hub subtracted (CSLS: ½).
	matchHubnessAlpha = 0.5
	// matchHubnessRel is the committed calibration, relative to the module root.
	matchHubnessRel = "cmd/refs/testdata/match_hubness.json"
)

type matchHubEntry struct {
	SoundHash string  `json:"sound_hash"`
	Hub       float64 `json:"hub"`
}

type matchHubness struct {
	Version int `json:"version"`
	K       int `json:"k"`
	// Refs is how many catalogued references the hubs were measured over.
	Refs        int                             `json:"refs"`
	Instruments map[instrument.ID]matchHubEntry `json:"instruments"`
}

var committedHubness = sync.OnceValues(func() (matchHubness, error) {
	var h matchHubness
	if err := json.Unmarshal(matchHubnessJSON, &h); err != nil {
		return matchHubness{}, fmt.Errorf("refs match: %s: %w", matchHubnessRel, err)
	}
	return h, nil
})

// stale ids (retuned, new, or calibrated on an older distance) get the median, uncorrected relative to a typical voice.
func (h matchHubness) hubValues(ids []instrument.ID) (hubs map[instrument.ID]float64, stale []instrument.ID) {
	hubs = map[instrument.ID]float64{}
	var fresh []float64
	for _, id := range ids {
		e, ok := h.Instruments[id]
		if ok && h.Version == matchHubnessVersion && e.SoundHash == audio.InstrumentSoundHash(id, nil) {
			hubs[id] = e.Hub
			fresh = append(fresh, e.Hub)
			continue
		}
		stale = append(stale, id)
	}
	med := medianOf(fresh)
	for _, id := range stale {
		hubs[id] = med
	}
	return hubs, stale
}

// skip (-1 = none) leaves one reference out, so -validate grades it against hubs it did not shape.
func computeHubs(rows []map[instrument.ID]float64, k, skip int) map[instrument.ID]float64 {
	per := map[instrument.ID][]float64{}
	for i, row := range rows {
		if i == skip {
			continue
		}
		for inst, d := range row {
			if !math.IsInf(d, 0) && !math.IsNaN(d) {
				per[inst] = append(per[inst], d)
			}
		}
	}
	out := map[instrument.ID]float64{}
	for inst, ds := range per {
		sort.Float64s(ds)
		n := min(k, len(ds))
		var s float64
		for _, d := range ds[:n] {
			s += d
		}
		if n > 0 {
			out[inst] = s / float64(n)
		}
	}
	return out
}

// applyHubs sets every render's score: its raw essence distance corrected
// by its hub (none in hubs: uncorrected; silent stays +Inf).
func applyHubs(rs []matchRender, hubs map[instrument.ID]float64) {
	for i := range rs {
		rs[i].cand.Score = hubCorrected(rs[i].rawEssence, hubs[rs[i].cand.Instrument])
	}
}

// hubCorrected is a raw essence distance minus matchHubnessAlpha of the
// voice's hub.
func hubCorrected(raw, hub float64) float64 {
	if math.IsInf(raw, 0) || math.IsNaN(raw) {
		return raw
	}
	return raw - matchHubnessAlpha*hub
}

func medianOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2
}

// warnStaleHubs tells the user when the calibration no longer covers the
// roster: those voices are ranked without their hubness correction.
func warnStaleHubs(diag io.Writer, ids []instrument.ID) {
	h, err := committedHubness()
	if err != nil {
		fmt.Fprintf(diag, "warning: %v -- ranking without the hubness correction\n", err)
		return
	}
	if _, stale := h.hubValues(ids); len(stale) > 0 {
		fmt.Fprintf(diag, "warning: %d instrument(s) retuned or added since the hubness calibration (e.g. %s) are ranked uncorrected -- `make wav-match-calibrate` refreshes it\n", len(stale), stale[0])
	}
}

// hubnessFor builds the calibration file from per-reference raw distances.
func hubnessFor(rows []map[instrument.ID]float64, ids []instrument.ID) matchHubness {
	hubs := computeHubs(rows, matchHubnessK, -1)
	h := matchHubness{Version: matchHubnessVersion, K: matchHubnessK, Refs: len(rows), Instruments: map[instrument.ID]matchHubEntry{}}
	for _, id := range ids {
		if v, ok := hubs[id]; ok {
			h.Instruments[id] = matchHubEntry{SoundHash: audio.InstrumentSoundHash(id, nil), Hub: math.Round(v*1e6) / 1e6}
		}
	}
	return h
}

// runMatchCalibrate measures every instrument's hub over the catalogued
// references and writes the committed calibration.
func runMatchCalibrate(out, diag io.Writer, workers int) error {
	rows, err := collectMatchCorpus(diag, workers, matchSettleAll)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("refs match -calibrate: no catalogued reference audio present (fetch it with `make fetch-refs TIER=instrument` or point %s at it)", wavref.EnvVar)
	}
	h := hubnessFor(rawEssenceRows(rows), rosterIDs())
	root, ok := wavref.ModuleRoot()
	if !ok {
		return fmt.Errorf("refs match -calibrate: cannot locate the module root (no go.mod above cwd)")
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(root, matchHubnessRel)
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "hubness of %d instruments over %d references -> %s\n", len(h.Instruments), len(rows), path)
	return nil
}

// checkMatchHubness is the corpus gate: the committed calibration covers the
// whole roster at its current sounds and reproduces from a fresh measurement.
func checkMatchHubness(r *checkReport) error {
	committed, err := committedHubness()
	if err != nil {
		return err
	}
	rows, err := collectMatchCorpus(io.Discard, defaultMatchWorkers(), matchSettleAll)
	if err != nil {
		return err
	}
	fresh := hubnessFor(rawEssenceRows(rows), rosterIDs())
	if committed.Version != fresh.Version || committed.K != fresh.K {
		r.Errorf("%s is version %d / k %d, the matcher wants %d / %d -- `make wav-match-calibrate`", matchHubnessRel, committed.Version, committed.K, fresh.Version, fresh.K)
	}
	for _, id := range rosterIDs() {
		want, ok := fresh.Instruments[id]
		got, had := committed.Instruments[id]
		switch {
		case !ok:
			r.Logf("%s: no finite distance to any reference", id)
		case !had:
			r.Errorf("%s has no hub in %s -- `make wav-match-calibrate`", id, matchHubnessRel)
		case got.SoundHash != want.SoundHash:
			r.Errorf("%s was retuned since the calibration -- `make wav-match-calibrate`", id)
		case math.Abs(got.Hub-want.Hub) > 1e-6:
			r.Errorf("%s hub %.6f, a fresh measurement gives %.6f -- `make wav-match-calibrate`", id, got.Hub, want.Hub)
		}
	}
	r.Logf("%d instruments over %d references", len(fresh.Instruments), len(rows))
	return nil
}
