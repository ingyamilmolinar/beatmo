//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

type organicValue struct {
	V  float64
	OK bool
	NA string
}

func organicValues(b *fingerprint.Bundle) map[string]organicValue {
	out := map[string]organicValue{}
	reg := fingerprint.DefaultRegistry()
	for _, id := range fingerprint.OrganicIDs() {
		d, ok := reg.ByID(id)
		if !ok {
			continue
		}
		if !d.Applies(b) {
			out[id] = organicValue{NA: "does not apply to this signal"}
			continue
		}
		m := d.Extract(b)
		if !m.OK {
			out[id] = organicValue{NA: m.NA}
			continue
		}
		out[id] = organicValue{V: m.Primary(), OK: true}
	}
	return out
}

func formatOrganicTable(w io.Writer, label string, vals map[string]organicValue, r fingerprint.OrganicRange) {
	fmt.Fprintf(w, "=== %s — organic axes against the real-recording range (p10–p90 over %d references) ===\n\n", label, organicN(r))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tVALUE\tREAL p10\tp50\tp90\tWHERE\tPOSITION")
	for _, id := range fingerprint.OrganicIDs() {
		d, _ := fingerprint.DefaultRegistry().ByID(id)
		v := vals[id]
		if !v.OK {
			fmt.Fprintf(tw, "%s\tn/a\t\t\t\t\t%s\n", d.Name, v.NA)
			continue
		}
		e, has := r[id]
		if !has {
			fmt.Fprintf(tw, "%s\t%.3g\t-\t-\t-\tuncalibrated\trun `refs calibrate-organic`\n", d.Name, v.V)
			continue
		}
		pos, where, ok := r.Place(id, v.V)
		if !ok {
			fmt.Fprintf(tw, "%s\t%.3g\t%.3g\t%.3g\t%.3g\tlow-n (n=%d)\t\n", d.Name, v.V, e.P10, e.P50, e.P90, e.N)
			continue
		}
		fmt.Fprintf(tw, "%s\t%.3g\t%.3g\t%.3g\t%.3g\t%s\t%+.2f\n", d.Name, v.V, e.P10, e.P50, e.P90, where, pos)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nPOSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; "+
		"'low-n (n=k)' means fewer than %d references qualified for that axis, too few to trust a band.\n", fingerprint.OrganicMinN)
	fmt.Fprintln(w, "'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.")
}

func organicN(r fingerprint.OrganicRange) int {
	n := 0
	for _, e := range r {
		if e.N > n {
			n = e.N
		}
	}
	return n
}

// runOrganic places inst's own render against the real-recording range.
// overrides, when non-empty, renders the instrument's recipe with those
// parameter overrides applied instead of its shipped sound -- the same seam
// `refs diagnose -override` uses, so a PROPOSED candidate can be checked
// against the organic band without editing its seed.
func runOrganic(w, diag io.Writer, inst instrument.ID, pitch, dur float64, overrides map[string]float64) error {
	if inst == "" {
		return fmt.Errorf("refs organic: -instrument is required")
	}
	sr := wavref.Rate
	render := synthmatch.RenderInstrument
	if len(overrides) > 0 {
		render = func(id instrument.ID, p float64, rate int, d float64) (wave.Wave, error) {
			return synthmatch.RenderWithParams(id, p, rate, d, overrides)
		}
	}
	wv, err := render(inst, pitch, sr, dur)
	if err != nil {
		return fmt.Errorf("refs organic: render %s: %w", inst, err)
	}
	b := fingerprint.NewBundle(wv, string(inst))
	formatOrganicTable(w, string(inst), organicValues(b), fingerprint.LoadOrganicRange())
	return nil
}

func defaultOrganicRangePath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs calibrate-organic: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, "internal", "audio", "fingerprint", "testdata", "organic_range.json"), nil
}

// locallyAdopted reports whether r's audio came from `refs adopt` pointing at a
// path on this machine (Source.Provider == "local"). Such a ref is catalogued
// but its bytes are unfetchable anywhere else, so it must not feed a COMMITTED
// cross-machine artifact. See the call site in runCalibrateOrganic.
func locallyAdopted(r wavref.Ref) bool {
	return r.Source != nil && r.Source.Provider == localProvider
}

func runCalibrateOrganic(w, diag io.Writer, outPath string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	values := map[string][]float64{}
	n := 0
	for _, ref := range c.Refs {
		if ref.Tier != wavref.TierInstrument {
			continue
		}
		// A provider:"local" ref is audio adopted from a path on ONE developer's
		// machine (`refs adopt`); `refs fetch` can only materialize it where that
		// path exists. The catalogue entry is committed, the bytes are not and
		// cannot be, so a range calibrated over it is a number nobody else can
		// reproduce -- and testdata/organic_range.json is a COMMITTED golden that
		// `refs check` re-derives (locally and in CI). Adopting one snare on
		// 2026-09-14 moved 10 descriptors' percentiles and every one of their n
		// by +1, failing that ratchet on a corpus change that no other machine
		// could ever match.
		//
		// So the committed BASELINE is a function of the shared, fetchable corpus
		// only. This is not a judgement about the audio: an adopted clip is real
		// recorded sound and still drives `refs compare`, `make ab` and the ear
		// verdicts, all of which are per-instrument and machine-local. It is the
		// cross-machine statistical artifact that has to be portable.
		if locallyAdopted(ref) {
			fmt.Fprintf(diag, "skipping %s: locally adopted (provider %q) — not reproducible off this machine\n",
				ref.ID, localProvider)
			continue
		}
		path, ok := wavref.Resolve(ref.ID)
		if !ok {
			fmt.Fprintf(diag, "skipping %s: not available locally\n", ref.ID)
			continue
		}
		samples, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
		if err != nil {
			fmt.Fprintf(diag, "skipping %s: %v\n", ref.ID, err)
			continue
		}
		b := fingerprint.NewBundle(toWave(samples, rate, ref.ID), ref.ID)
		for id, v := range organicValues(b) {
			if v.OK {
				values[id] = append(values[id], v.V)
			}
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("refs calibrate-organic: no reference could be analyzed (set %s)", wavref.EnvVar)
	}
	r := fingerprint.ComputeOrganicRange(values)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s from %d references\n", outPath, n)
	return nil
}

func cmdOrganic(args []string) error {
	fs := flag.NewFlagSet("organic", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to render and place against the real-recording range")
	pitch := fs.Float64("pitch", 0, "semitones from A3")
	dur := fs.Float64("dur", 2.0, "render duration in seconds")
	override := fs.String("override", "", "comma-separated key=value recipe-parameter overrides to render with "+
		"(e.g. -override amp_attack=1.0,gen1_gain=0.2) -- places a PROPOSED sound without editing its seed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ov, err := parseOverrideList(*override)
	if err != nil {
		return err
	}
	if err := validateOverrideKeys(instrument.ID(*inst), ov); err != nil {
		return err
	}
	return runOrganic(os.Stdout, os.Stderr, instrument.ID(*inst), *pitch, *dur, ov)
}

func cmdCalibrateOrganic(args []string) error {
	fs := flag.NewFlagSet("calibrate-organic", flag.ExitOnError)
	out := fs.String("out", "", "output JSON (default: internal/audio/fingerprint/testdata/organic_range.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := *out
	if p == "" {
		var err error
		if p, err = defaultOrganicRangePath(); err != nil {
			return err
		}
	}
	return runCalibrateOrganic(os.Stdout, os.Stderr, p)
}
