// Command clone turns a reference recording of a struck / modal instrument
// into a candidate instrument document (spec
// docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md):
//
//	clone -inst <id> [-ref <ref>] [-wav <path>] [-f0 <Hz>] [-as <newid> | -into <id>]
//	      [-no-refine] [-seed N] [-gens N] [-workers N] [-json] [-out <dir>] [-cpuprofile <f>]
//	clone adopt -inst <id>
//
// The first form analyses (Prepare → Analyze), inverts the analysis into a
// document (model numbers only — never audio), refines it with seeded
// parallel CMA-ES against the reference, and writes everything under
// $BEATMO_WAV_REFS/clones/<inst>/: analysis.json, candidate.json, meta.json,
// docs/<sha>.json, report.md / report.json and level-matched reference /
// current / candidate WAVs. Listening is `make ab-session FAMILIES=<inst>
// CLONE=1`; `clone adopt` ships the candidate only after that session
// recorded accept = y for this exact document.
//
// Exit codes: 0 ok, 2 refused (not a struck/modal reference — the partial
// analysis is still written), 1 error.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// MetaFormat identifies meta.json — what `refs session -clone` and `clone
// adopt` need to know about the candidate in the directory.
const MetaFormat = "beatmo.clone.meta/1"

// Meta is meta.json.
type Meta struct {
	Format   string        `json:"format"`
	Inst     instrument.ID `json:"inst"`
	Ref      string        `json:"ref,omitempty"` // catalogue reference id (empty for -wav)
	Source   string        `json:"source"`
	FitPitch float64       `json:"fit_pitch"`
	Rate     int           `json:"rate"`
	DocID    instrument.ID `json:"doc_id"`
	DocSHA   string        `json:"doc_sha"`
	Melodic  bool          `json:"melodic"`
}

// lossSec bounds the reference span the loss compares (and so the render
// length during refinement).
const lossSec = 3.0

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "adopt" {
		return runAdopt(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("clone", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inst := fs.String("inst", "", "instrument id the clone is for (names the output directory)")
	refID := fs.String("ref", "", "catalogue reference id (default: the instrument's first reference)")
	wavPath := fs.String("wav", "", "analyse this WAV instead of a catalogue reference")
	f0 := fs.Float64("f0", 0, "nominal pitch in Hz (overrides detection)")
	asID := fs.String("as", "", "candidate document id (default <inst>-clone)")
	intoID := fs.String("into", "", "replace this shipped instrument's sound, keeping its identity")
	noRefine := fs.Bool("no-refine", false, "skip CMA-ES refinement (analysis-only candidate)")
	seed := fs.Uint64("seed", 1, "refinement seed")
	gens := fs.Int("gens", 0, "CMA-ES generations (default 150)")
	workers := fs.Int("workers", 0, "refinement workers (default NumCPU-1)")
	outDir := fs.String("out", "", "output directory (default $BEATMO_WAV_REFS/clones/<inst>)")
	asJSON := fs.Bool("json", false, "print analysis.json to stdout")
	cpuProfile := fs.String("cpuprofile", "", "write a CPU profile to this file")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		defer pprof.StopCPUProfile()
	}
	if *inst == "" {
		fmt.Fprintln(stderr, "clone: -inst is required")
		return 1
	}
	instID := instrument.ID(*inst)
	if *asID != "" && *intoID != "" {
		fmt.Fprintln(stderr, "clone: -as and -into are exclusive")
		return 1
	}
	opt := clone.PrepareOptions{F0Hz: *f0, Label: *inst}
	path, ref := *wavPath, *refID
	if path == "" {
		p, r, semis, has, err := resolveReference(instID, *refID)
		if err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return 1
		}
		path, ref, opt.SemisFromA3, opt.HasSemis = p, r, semis, has
	}
	dir := *outDir
	if dir == "" {
		// Beside sources/ and candidates/ in the reference root — local dev
		// output, never in the repo.
		dir = filepath.Join(wavref.Root(), "clones", *inst)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}
	write := func(name string, b []byte) bool {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			fmt.Fprintln(stderr, "clone:", err)
			return false
		}
		return true
	}

	start := time.Now()
	p, perr := clone.PrepareFile(path, opt)
	prepMs := msSince(start)
	var a clone.Analysis
	aerr := perr
	if perr == nil {
		a, aerr = clone.Analyze(p)
	} else {
		a = clone.Analysis{Format: clone.AnalysisFormat, Label: *inst, Notes: []string{perr.Error()}}
	}
	if aerr != nil && perr == nil {
		a.Notes = append(a.Notes, aerr.Error())
	}
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "clone: encode analysis:", err)
		return 1
	}
	md := a.Markdown() + fmt.Sprintf("\nPrepare: %.1f ms · source `%s`\n", prepMs, path)
	if !write("analysis.json", raw) {
		return 1
	}
	if *asJSON {
		stdout.Write(raw)
		fmt.Fprintln(stdout)
	}
	var refusal *clone.Refusal
	if aerr != nil {
		write("report.md", []byte(md))
		if errors.As(aerr, &refusal) {
			fmt.Fprintln(stderr, aerr)
			return 2
		}
		fmt.Fprintln(stderr, "clone:", aerr)
		return 1
	}

	// ── Invert ──
	ti := time.Now()
	iopt := clone.InvertOptions{ID: instrument.ID(*inst + "-clone"), Name: displayName(instID) + " (clone)"}
	if *asID != "" {
		iopt.ID, iopt.Name = instrument.ID(*asID), *asID
	}
	if base, ok := audio.ShippedDoc(instID); ok {
		iopt.Category = base.Category
	}
	if *intoID != "" {
		base, ok := audio.ShippedDoc(instrument.ID(*intoID))
		if !ok {
			fmt.Fprintf(stderr, "clone: -into %s: not a shipped instrument\n", *intoID)
			return 1
		}
		iopt.Into = &base
	}
	doc, irep, err := clone.Invert(a, iopt)
	if err != nil {
		write("report.md", []byte(md))
		fmt.Fprintln(stderr, "clone: invert:", err)
		return 1
	}
	invMs := msSince(ti)
	pitch := irep.FitPitch
	cr := clone.CandidateReport{Invert: irep, Notes: []string{}, PrepareMs: prepMs, InvertMs: invMs}
	if !realEngine {
		cr.Notes = append(cr.Notes, "built with -tags test: renders are the Go mode model, not the C engine")
	}

	// ── Refine ──
	if !*noRefine {
		lref := clone.NewLossRef(p, lossSec)
		res, err := clone.Refine(doc, lref, a, clone.RefineOptions{
			Render: func(d instrument.Doc) []float64 { return renderShipped(capRender(d, lossSec+0.25), p.Rate, pitch) },
			Pitch:  pitch, Gens: *gens, Seed: *seed, Workers: *workers,
		})
		switch {
		case err != nil:
			cr.Notes = append(cr.Notes, "refine failed, analysis-only document kept: "+err.Error())
		default:
			cr.Refine = &res
			doc = res.Doc
		}
	} else {
		cr.Notes = append(cr.Notes, "refinement skipped (-no-refine)")
	}

	// ── Level: the measure-loudness rule against the shipped instrument ──
	doc.Render.Amplitude = 1
	if base, ok := audio.ShippedDoc(instID); ok {
		cur, ok1 := clone.LoudnessNorm(renderShipped(base, p.Rate, 0), p.Rate)
		cand, ok2 := clone.LoudnessNorm(renderShipped(doc, p.Rate, 0), p.Rate)
		if ok1 && ok2 {
			doc.Render.Amplitude = math.Min(1, base.Render.Amplitude*cur/cand)
		}
	}
	cr.Amplitude = doc.Render.Amplitude
	cr.DocID, cr.DocSHA = doc.ID, clone.DocSHA(doc)
	cr.TotalMs = msSince(start)

	docBytes, err := instrument.MarshalFile(doc)
	if err != nil {
		fmt.Fprintln(stderr, "clone: encode document:", err)
		return 1
	}
	meta := Meta{Format: MetaFormat, Inst: instID, Ref: ref, Source: path, FitPitch: pitch, Rate: p.Rate,
		DocID: doc.ID, DocSHA: cr.DocSHA, Melodic: doc.Render.Melodic}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	repBytes, _ := json.MarshalIndent(cr, "", "  ")
	hist := filepath.Join("docs", strings.TrimPrefix(cr.DocSHA, "sha256:")+".json")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}
	if !write("candidate.json", docBytes) || !write(hist, docBytes) || !write("meta.json", metaBytes) ||
		!write("report.json", repBytes) || !write("report.md", []byte(md+cr.Markdown())) {
		return 1
	}
	if err := writeListeningWAVs(dir, p, instID, doc, pitch); err != nil {
		fmt.Fprintln(stderr, "clone:", err)
		return 1
	}
	if !*asJSON {
		loss := ""
		if r := cr.Refine; r != nil {
			loss = fmt.Sprintf(", loss %.3f → %.3f", r.Before.Total, r.After.Total)
		}
		fmt.Fprintf(stdout, "clone %s: %d modes, %d noise rows%s, %.1f s → %s\n  listen: make ab-session FAMILIES=%s CLONE=1\n",
			*inst, irep.Kept, irep.NoiseRows, loss, cr.TotalMs/1000, dir, *inst)
	}
	return 0
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

// capRender shortens a document's voice for the search renders: the loss only
// reads lossSec of it (the margin keeps the end fade outside that span).
func capRender(d instrument.Doc, sec float64) instrument.Doc {
	if d.Render.Seconds > sec {
		d.Render.Seconds = sec
	}
	return d
}

func displayName(id instrument.ID) string {
	if d, ok := audio.ShippedDoc(id); ok && d.Name != "" {
		return d.Name
	}
	return string(id)
}

// writeListeningWAVs writes reference.wav, current.wav (when the instrument
// ships) and candidate.wav at the reference's rate and the fit pitch, all at
// the reference's PEAK (the percussion A/B convention).
func writeListeningWAVs(dir string, p clone.Prepared, inst instrument.ID, doc instrument.Doc, pitch float64) error {
	from := max(0, p.Onset-int(0.01*float64(p.Rate)))
	refx := p.Samples[from:]
	peak := 0.0
	for _, v := range refx {
		peak = math.Max(peak, math.Abs(v))
	}
	put := func(name string, x []float64) error {
		pk := 0.0
		for _, v := range x {
			pk = math.Max(pk, math.Abs(v))
		}
		pcm := make([]float32, len(x))
		for i, v := range x {
			if pk > 0 {
				pcm[i] = float32(v * peak / pk)
			}
		}
		b, err := wavgen.EncodeWAV(pcm, p.Rate)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	if err := put("reference.wav", refx); err != nil {
		return err
	}
	if base, ok := audio.ShippedDoc(inst); ok {
		if err := put("current.wav", renderShipped(base, p.Rate, pitch)); err != nil {
			return err
		}
	}
	return put("candidate.wav", renderShipped(doc, p.Rate, pitch))
}

// resolveReference picks the catalogue reference for inst (or refID) and
// returns its local path, id and measured note.
func resolveReference(inst instrument.ID, refID string) (string, string, int, bool, error) {
	cat, err := wavref.Catalogue()
	if err != nil {
		return "", "", 0, false, err
	}
	var ref wavref.Ref
	if refID != "" {
		r, ok := cat.Ref(refID)
		if !ok {
			return "", "", 0, false, fmt.Errorf("unknown reference %q", refID)
		}
		ref = r
	} else {
		found := false
		for _, r := range cat.ForInstrument(inst) {
			if r.Status == "" {
				ref, found = r, true
				break
			}
		}
		if !found {
			return "", "", 0, false, fmt.Errorf("no reference recording for %q (use -wav, or `make refs-adopt`)", inst)
		}
	}
	path, ok := wavref.Resolve(ref.ID)
	if !ok {
		return "", "", 0, false, fmt.Errorf("audio for %s is not available locally; run `refs fetch -id %s`", ref.ID, ref.ID)
	}
	if ref.Measured != nil {
		return path, ref.ID, ref.Measured.SemisFromA3, true, nil
	}
	return path, ref.ID, 0, false, nil
}
