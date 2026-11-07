//go:build !js

package main

// An essence-driven candidate document for one instrument: CMA-ES over the document's own continuous values, each in
// a trust box around its shipped value, minimising the RAW essence distance over every recording of it played as refs
// match plays it. It never writes a shipped document: make ab-tuned plays it, make tune-adopt ships it after a y.

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// tunePick (testdata/tune_picks.json): why it was picked, harmonics the search may raise from silence, a restriction to
// named fields, edits applied before the search.
type tunePick struct {
	Inst         instrument.ID      `json:"inst"`
	Why          string             `json:"why"`
	AddHarmonics int                `json:"add_harmonics,omitempty"`
	Only         []string           `json:"only,omitempty"`
	Set          map[string]float64 `json:"set,omitempty"`
	// From starts from ANOTHER instrument's document when the picked voice is the wrong instrument altogether and no move
	// near its own values can reach the recording (the rimshot). The candidate keeps the picked id and name.
	From instrument.ID `json:"from,omitempty"`
	// Skip names catalogued recordings filed under the instrument that are not what it should sound like (a snare hit
	// with its snares off): averaged in, they drag the search toward the wrong instrument.
	Skip []string `json:"skip,omitempty"`
}

// tuneStart is the document a pick's search starts from and the param
// definitions that bound its values: the shipped document, or with From the
// donor's, re-identified as the picked instrument.
func tuneStart(pick tunePick) (instrument.Doc, map[string]audio.ParamDef, error) {
	shipped, ok := audio.ShippedDoc(pick.Inst)
	if !ok {
		return instrument.Doc{}, nil, fmt.Errorf("refs tune: unknown instrument %q", pick.Inst)
	}
	src, d := pick.Inst, shipped
	if pick.From != "" {
		donor, ok := audio.ShippedDoc(pick.From)
		if !ok {
			return instrument.Doc{}, nil, fmt.Errorf("refs tune: %s: unknown donor instrument %q", pick.Inst, pick.From)
		}
		src, d = pick.From, donor.Clone()
		d.ID, d.Origin, d.Name, d.Category = shipped.ID, shipped.Origin, shipped.Name, shipped.Category
		d.Notes = fmt.Sprintf("Started from %s's document by `refs tune` (pick from: %s): the shipped %s could not reach its recording.\n%s", pick.From, pick.From, pick.Inst, donor.Notes)
	}
	defs := map[string]audio.ParamDef{}
	for _, pd := range audio.InstrumentParamDefs(src) {
		defs[pd.Name] = pd
	}
	return d, defs, nil
}

//go:embed testdata/tune_picks.json
var tunePicksJSON []byte

func loadTunePicks() ([]tunePick, error) {
	var ps []tunePick
	if err := json.Unmarshal(tunePicksJSON, &ps); err != nil {
		return nil, fmt.Errorf("refs tune: tune_picks.json: %w", err)
	}
	return ps, nil
}

// tuneSetSeconds is the render field a pick may set besides flat ABI fields.
const tuneSetSeconds = "render.seconds"

// Nothing an essence term judges or the search may move: core (gain normalised, pitch the player's), vibrato LFO,
// unison and burst (rhythmic), formant (discrete vowel tables).
var tuneSkippedStages = map[string]bool{"core": true, "lfo": true, "unison": true, "burst": true, "formant": true}

// tuneSkippedValues are structural or pitch-setting values (a layer's partial
// ratio, waveform, filter type, a drum's tune), never timbre knobs.
var tuneSkippedValues = map[string]bool{
	"freq": true, "freq_mode": true, "phase": true, "phase_mode": true, "wave": true,
	"noise_offset": true, "osc_type": true, "filter_type": true, "filt_type": true,
	"amp_curve": true, "out_clamp": true, "post_pitch": true, "post_decay_rate": true,
	"voice_freq_hz": true, "snare_tune": true, "ks_blow": true, "out_scale": true,
}

// tuneDim is one searched value: its flat ABI field, the trust box it moves
// in, and where it starts (the document's value). Log dims move in log space.
type tuneDim struct {
	Field  string  `json:"field"`
	Lo     float64 `json:"lo"`
	Hi     float64 `json:"hi"`
	Start  float64 `json:"start"`
	Log    bool    `json:"log,omitempty"`
	Added  bool    `json:"added,omitempty"`
	phase0 float64
}

// Trust-box sizes: a linear value moves at most tuneLinearReach of its
// range either way, a log value a factor tuneLogReach, a harmonic raised from
// silence up to tuneAddedGainMax.
const (
	tuneLinearReach  = 0.3
	tuneLogReach     = 3.0
	tuneAddedGainMax = 0.35
)

func newTuneDim(field string, def audio.ParamDef, start float64, added bool) (tuneDim, bool) {
	lo, hi := def.Min, def.Max
	if !(hi > lo) {
		return tuneDim{}, false
	}
	d := tuneDim{Field: field, Start: math.Min(hi, math.Max(lo, start)), Added: added}
	switch {
	case added:
		d.Lo, d.Hi = lo, math.Min(hi, lo+tuneAddedGainMax)
	case lo > 0 && hi/lo >= 20 && d.Start > 0:
		d.Log = true
		d.Lo, d.Hi = math.Max(lo, d.Start/tuneLogReach), math.Min(hi, d.Start*tuneLogReach)
	default:
		r := tuneLinearReach * (hi - lo)
		d.Lo, d.Hi = math.Max(lo, d.Start-r), math.Min(hi, d.Start+r)
	}
	if !(d.Hi > d.Lo) {
		return tuneDim{}, false
	}
	d.phase0 = math.Asin(2*d.unit(d.Start) - 1)
	return d, true
}

// unit is v's position in the trust box (0..1, log or linear).
func (d tuneDim) unit(v float64) float64 {
	if d.Log {
		return math.Min(1, math.Max(0, math.Log(v/d.Lo)/math.Log(d.Hi/d.Lo)))
	}
	return math.Min(1, math.Max(0, (v-d.Lo)/(d.Hi-d.Lo)))
}

// Through the clone's sin mapping, shifted so the origin returns Start verbatim: the starting document is the
// shipped one bit for bit.
func (d tuneDim) value(y float64) float64 {
	if y == 0 {
		return d.Start
	}
	u := (1 + math.Sin(y+d.phase0)) / 2
	if d.Log {
		return d.Lo * math.Pow(d.Hi/d.Lo, u)
	}
	return d.Lo + (d.Hi-d.Lo)*u
}

// Only ON modules count: a value of an off stage moves nothing.
func tuneSpace(d instrument.Doc, defs map[string]audio.ParamDef, pick tunePick) (instrument.Doc, []tuneDim, error) {
	reg := audio.BuiltinModuleRegistry()
	d = d.Clone()
	keys := make([]string, 0, len(pick.Set))
	for k := range pick.Set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := pick.Set[k]
		switch k {
		case tuneSetSeconds:
			d.Render.Seconds = v
		default:
			nd, err := instrument.SetField(d, reg, k, v)
			if err != nil {
				return d, nil, fmt.Errorf("refs tune: %s set %s: %w", pick.Inst, k, err)
			}
			d = nd
		}
	}
	added := map[string]bool{}
	if pick.AddHarmonics > 0 {
		var err error
		if d, added, err = addHarmonicLayers(d, reg, pick.AddHarmonics); err != nil {
			return d, nil, fmt.Errorf("refs tune: %s: %w", pick.Inst, err)
		}
	}
	only := map[string]bool{}
	for _, f := range pick.Only {
		only[f] = true
	}
	var dims []tuneDim
	consider := func(field, name string, v float64) {
		if tuneSkippedValues[name] || strings.HasSuffix(name, "_enabled") || strings.HasSuffix(name, "_type") ||
			strings.HasSuffix(name, "_mode") || strings.HasSuffix(name, "_variant") || strings.HasSuffix(name, "_count") {
			return
		}
		if len(only) > 0 && !only[field] {
			return
		}
		def, ok := defs[field]
		if !ok || len(def.Enum) > 0 || def.Step > 0 {
			return
		}
		if dm, ok := newTuneDim(field, def, v, added[field]); ok {
			dims = append(dims, dm)
		}
	}
	stages := make([]string, 0, len(d.Stages))
	for k := range d.Stages {
		stages = append(stages, k)
	}
	sort.Strings(stages)
	for _, k := range stages {
		st := d.Stages[k]
		if !st.On || tuneSkippedStages[k] {
			continue
		}
		for _, name := range sortedValueNames(st.Values) {
			consider(name, name, st.Values[name])
		}
	}
	if d.Source != nil && d.Source.On {
		for _, name := range sortedValueNames(d.Source.Values) {
			consider(name, name, d.Source.Values[name])
		}
	}
	for i, l := range d.Layers {
		if l == nil || !l.On {
			continue
		}
		for _, name := range sortedValueNames(l.Values) {
			switch {
			case name == "gain" && l.Type != "gen.wavetable" && l.Type != "gen.noise":
				continue // a lone voice's level is normalised away
			case strings.HasPrefix(name, "env_") && l.Type != "gen.noise":
				continue // a partial's own envelope is held flat by design
			case strings.HasPrefix(name, "filt_") && l.Values["filt_type"] == 0:
				continue // the layer's filter is off
			}
			consider(instrument.LayerField(i, name), name, l.Values[name])
		}
	}
	return d, dims, nil
}

func sortedValueNames(v instrument.Values) []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Silent copies of the last partial above the highest harmonic, for the search to raise; only a harmonic-series document
// (ratio mode, integer ratios) qualifies, and the gen bank's slot capacity caps n.
func addHarmonicLayers(d instrument.Doc, reg *instrument.Registry, n int) (instrument.Doc, map[string]bool, error) {
	var last *instrument.Module
	maxH := 0.0
	for _, l := range d.Layers {
		if l == nil || l.Type != "gen.wavetable" {
			continue
		}
		f := l.Values["freq"]
		if l.Values["freq_mode"] != 0 || f != math.Round(f) || f < 1 {
			return d, nil, fmt.Errorf("add_harmonics: partial at ratio %g is not a harmonic", f)
		}
		if f > maxH {
			maxH, last = f, l
		}
	}
	if last == nil {
		return d, nil, fmt.Errorf("add_harmonics: no gen.wavetable partials to extend")
	}
	added := map[string]bool{}
	for k := 0; k < n && len(d.Layers) < reg.Slots(); k++ {
		m := instrument.Module{Type: last.Type, V: last.V, On: true, Values: instrument.Values{}}
		for name, v := range last.Values {
			m.Values[name] = v
		}
		m.Values["freq"] = maxH + float64(k+1)
		m.Values["gain"] = 0
		m.Notes = fmt.Sprintf("H%d added by refs tune (starts silent)", int(maxH)+k+1)
		d.Layers = append(d.Layers, &m)
		added[instrument.LayerField(len(d.Layers)-1, "gain")] = true
	}
	return d, added, nil
}

// applyTune writes the coordinates y into a copy of d.
func applyTune(d instrument.Doc, dims []tuneDim, y []float64) (instrument.Doc, error) {
	reg := audio.BuiltinModuleRegistry()
	out := d.Clone()
	for i, dm := range dims {
		v := dm.Start
		if y != nil {
			v = dm.value(y[i])
		}
		nd, err := instrument.SetField(out, reg, dm.Field, v)
		if err != nil {
			return d, fmt.Errorf("refs tune: %s: %w", dm.Field, err)
		}
		out = nd
	}
	return out, nil
}

// tuneRegWeight keeps the search near the shipped voice: the mean squared
// move in trust-box units, weighted against essence distances of ~0.1-0.9.
const tuneRegWeight = 0.05

func tuneRegularizer(dims []tuneDim, y []float64) float64 {
	if len(dims) == 0 || y == nil {
		return 0
	}
	s := 0.0
	for i, dm := range dims {
		du := dm.unit(dm.value(y[i])) - dm.unit(dm.Start)
		s += du * du
	}
	return tuneRegWeight * s / float64(len(dims))
}

// tuneSilentPenalty stands in for a silent rendition's +Inf so the optimiser
// sees a finite, terrible score.
const tuneSilentPenalty = 10.0

// The search pitch is fixed during the search; the final scoring re-runs the whole pitch plan.
type tuneRec struct {
	Name  string
	RefID string
	f0    float64
	w     wave.Wave
	t     matchTarget
	pitch float64
}

// tuneEvaluator scores one document on one recording at the search pitch.
// Injected so the search can be tested without rendering a note.
type tuneEvaluator func(d instrument.Doc, rec tuneRec) (float64, error)

func essenceEvaluator(inst instrument.ID) tuneEvaluator {
	return func(d instrument.Doc, rec tuneRec) (float64, error) {
		r, err := rec.t.evalDoc(d, inst, rec.pitch)
		if err != nil {
			return 0, err
		}
		if r.silent {
			return tuneSilentPenalty, nil
		}
		return fingerprint.EssenceTotal(r.ess), nil
	}
}

type tuneOpts struct {
	Gens, Lambda, Workers, Stall int
	Seed                         uint64
	Sigma                        float64
}

type tuneSearchResult struct {
	Y           []float64
	Start, Best float64
	Gens, Evals int
	BestPerRec  []float64
	StartPerRec []float64
}

// tuneSearch runs CMA-ES from the start document (y = 0) and returns the best
// coordinates found (nil = the start was never beaten). The objective is the
// mean essence distance over recs plus the regulariser.
func tuneSearch(d0 instrument.Doc, dims []tuneDim, recs []tuneRec, eval tuneEvaluator, o tuneOpts, diag io.Writer) (tuneSearchResult, error) {
	score := func(y []float64) (float64, []float64, error) {
		d, err := applyTune(d0, dims, y)
		if err != nil {
			return 0, nil, err
		}
		per := make([]float64, len(recs))
		errs := make([]error, len(recs))
		forEachParallel(len(recs), 1, func(i int) { per[i], errs[i] = eval(d, recs[i]) })
		m := 0.0
		for i := range per {
			if errs[i] != nil {
				return 0, nil, errs[i]
			}
			m += per[i] / float64(len(per))
		}
		return m + tuneRegularizer(dims, y), per, nil
	}
	start, startPer, err := score(nil)
	if err != nil {
		return tuneSearchResult{}, err
	}
	res := tuneSearchResult{Start: start, Best: start, StartPerRec: startPer, BestPerRec: startPer}
	if len(dims) == 0 || o.Gens <= 0 {
		return res, nil
	}
	es := clone.NewCMAES(len(dims), o.Sigma, o.Lambda, o.Seed)
	stall, last := 0, start
	for g := 0; g < o.Gens; g++ {
		ys := es.Ask()
		f := make([]float64, len(ys))
		per := make([][]float64, len(ys))
		errs := make([]error, len(ys))
		forEachParallel(len(ys), o.Workers, func(k int) { f[k], per[k], errs[k] = score(ys[k]) })
		for k := range ys {
			if errs[k] != nil {
				return res, errs[k]
			}
			if f[k] < res.Best {
				res.Best, res.Y, res.BestPerRec = f[k], append([]float64(nil), ys[k]...), per[k]
			}
		}
		es.Tell(ys, f)
		res.Gens, res.Evals = g+1, res.Evals+len(ys)
		if diag != nil {
			fmt.Fprintf(diag, "  gen %3d  best %.4f  (start %.4f)\n", g, res.Best, start)
		}
		if res.Best < last*(1-1e-3) {
			last, stall = res.Best, 0
		} else if stall++; stall >= o.Stall {
			break
		}
	}
	if res.Y == nil {
		return res, nil
	}
	// Every value goes back to its shipped setting unless the move pays for itself, smallest first, while the reverted
	// moves together cost at most tunePruneBudget: CMA-ES leaves every dimension a little off, and a candidate should carry
	// only the changes that matter.
	order := make([]int, len(dims))
	for i := range order {
		order[i] = i
	}
	moved := func(i int) float64 {
		return math.Abs(dims[i].unit(dims[i].value(res.Y[i])) - dims[i].unit(dims[i].Start))
	}
	sort.SliceStable(order, func(a, b int) bool { return moved(order[a]) < moved(order[b]) })
	y := append([]float64(nil), res.Y...)
	ceiling := res.Best * (1 + tunePruneBudget)
	for _, i := range order {
		if y[i] == 0 {
			continue
		}
		keep := y[i]
		y[i] = 0
		f, per, err := score(y)
		if err != nil {
			return res, err
		}
		res.Evals++
		if f <= ceiling {
			res.Best, res.BestPerRec = f, per
			continue
		}
		y[i] = keep
	}
	res.Y = y
	return res, nil
}

// tunePruneBudget is how much of the searched objective the pruned moves may
// cost together: values that buy less than 1 % between them are not part of
// the fix.
const tunePruneBudget = 0.01

// newTuneRec loads one recording exactly as refs match plays it (conversion, silence trim, maxSec cap with a faded cut).
func newTuneRec(path, name, refID, tmp string, maxSec float64) (tuneRec, error) {
	ref, err := loadMatchReference(path, tmp, maxSec)
	if err != nil {
		return tuneRec{}, err
	}
	f0 := matchRefPitch(ref)
	return tuneRec{Name: name, RefID: refID, f0: f0, w: ref,
		t: matchTarget{f0: f0, refE: fingerprint.AnalyzeEssence(ref, fingerprint.EssenceOpts{}), sr: wavref.Rate, clipSec: ref.Duration()}}, nil
}

// settlePitch sets the search pitch: where the SHIPPED voice sounds closest
// under its own pitch plan (a melodic voice: the recording's note).
func (rec *tuneRec) settlePitch(inst instrument.ID) error { return rec.settlePitchDoc(inst, nil) }

// A pick From another instrument sounds closest at its OWN pitch, not where the voice it replaces did.
func (rec *tuneRec) settlePitchDoc(inst instrument.ID, d *instrument.Doc) error {
	s := newMatchSearch(inst, rec.f0)
	s.doc = d
	if err := s.scout(rec.t); err != nil {
		return err
	}
	if err := s.settle(rec.t); err != nil {
		return err
	}
	if !s.have {
		return fmt.Errorf("refs tune: %s cannot be played against %s", inst, rec.Name)
	}
	rec.pitch = s.best.cand.RenderPitch
	return nil
}

func tuneRecordings(inst instrument.ID, skip []string, glob string, maxSec float64, tmp string, diag io.Writer) ([]tuneRec, error) {
	cat, err := loadCatalogue()
	if err != nil {
		return nil, err
	}
	var recs []tuneRec
	seen := map[string]bool{}
	add := func(path, name, refID string) error {
		rec, err := newTuneRec(path, name, refID, tmp, maxSec)
		if err != nil {
			return err
		}
		recs = append(recs, rec)
		return nil
	}
	for _, r := range cat.ForInstrument(inst) {
		if slices.Contains(skip, r.ID) {
			seen[r.ID] = true // also keeps -wavs from adding the same file back
			continue
		}
		p, ok := wavref.Resolve(r.ID)
		if !ok {
			fmt.Fprintf(diag, "  %s: reference %s is not in %s (make fetch-refs ID=%s)\n", inst, r.ID, wavref.Root(), r.ID)
			continue
		}
		if err := add(p, r.ID, r.ID); err != nil {
			return nil, err
		}
		seen[r.ID] = true
	}
	if glob != "" {
		paths, err := expandMatchGlob(glob)
		if err != nil {
			return nil, err
		}
		ids := rosterIDs()
		for _, p := range paths {
			if cr, ok := matchCatalogueRef(p, cat); ok && seen[cr.ID] {
				continue
			}
			label := matchLabel(p, namesakeInFilename(filepath.Base(p), ids), cat)
			if !containsStr(label, inst) {
				continue
			}
			if err := add(p, filepath.Base(p), ""); err != nil {
				return nil, err
			}
		}
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("refs tune: no recording of %s found (catalogue or -wavs)", inst)
	}
	return recs, nil
}

func containsStr[S ~string](xs []S, s S) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// tuneScore is one recording's verdict on a document: where it ranks among
// the whole roster (hubness-corrected, exactly as the map ranks), its raw
// essence distance, the losing terms and the direction it is off.
type tuneScore struct {
	Rank   int                       `json:"rank"`
	Raw    float64                   `json:"raw"`
	Pitch  float64                   `json:"pitch"`
	Terms  fingerprint.EssenceScore  `json:"terms"`
	Traits fingerprint.EssenceTraits `json:"traits"`
}

type tuneRecReport struct {
	Recording string                    `json:"recording"`
	Ref       string                    `json:"ref,omitempty"`
	RefTraits fingerprint.EssenceTraits `json:"ref_traits"`
	Shipped   tuneScore                 `json:"shipped"`
	Candidate tuneScore                 `json:"candidate"`
	// refs session -tuned plays T here and C where the shipped voice settles. Absent in older runs (the shipped voice's).
	SearchPitch *float64 `json:"search_pitch,omitempty"`
}

// scoreAgainstRoster is T1: the shipped voice and the candidate, each with
// its full pitch plan, ranked among the roster on one recording.
func scoreAgainstRoster(inst instrument.ID, cand instrument.Doc, rec tuneRec, workers int) (tuneRecReport, error) {
	ids := rosterIDs()
	ranked, err := rankRoster(rec.w, rec.f0, ids, workers, func(id instrument.ID) bool { return id == inst })
	if err != nil {
		return tuneRecReport{}, err
	}
	hubs := map[instrument.ID]float64{}
	if h, err := committedHubness(); err == nil {
		hubs, _ = h.hubValues(ids)
	}
	out := tuneRecReport{Recording: rec.Name, Ref: rec.RefID, RefTraits: rec.t.refE.Traits()}
	for i, r := range ranked {
		if r.cand.Instrument == inst {
			out.Shipped = tuneScore{Rank: i + 1, Raw: r.rawEssence, Pitch: r.cand.RenderPitch, Terms: r.ess, Traits: r.traits}
		}
	}
	s := newMatchSearch(inst, rec.f0)
	s.doc = &cand
	if err := s.scout(rec.t); err != nil {
		return out, err
	}
	if err := s.settle(rec.t); err != nil {
		return out, err
	}
	raw := math.Inf(1)
	if s.have && !s.best.silent {
		raw = fingerprint.EssenceTotal(s.best.ess)
	}
	corr := hubCorrected(raw, hubs[inst])
	rank := 1
	for _, r := range ranked {
		if r.cand.Instrument != inst && r.cand.Score < corr {
			rank++
		}
	}
	out.Candidate = tuneScore{Rank: rank, Raw: raw, Pitch: s.best.cand.RenderPitch, Terms: s.best.ess, Traits: s.best.traits}
	return out, nil
}

// tuneMeta is meta.json: what `refs session -tuned` and `clone adopt -kind
// tuned` read, plus the run's provenance.
type tuneMeta struct {
	Inst      instrument.ID `json:"inst"`
	Ref       string        `json:"ref"`
	SamePitch bool          `json:"same_pitch"`
	// MaxSec is the recordings' analysis window (-max-sec): `refs session
	// -tuned` plays the reference cut exactly as the search heard it.
	MaxSec  float64            `json:"max_sec,omitempty"`
	Why     string             `json:"why,omitempty"`
	From    string             `json:"from,omitempty"`
	DocSHA  string             `json:"doc_sha"`
	Created string             `json:"created"`
	Seed    uint64             `json:"seed"`
	Gens    int                `json:"gens"`
	Evals   int                `json:"evals"`
	Start   float64            `json:"objective_start"`
	Best    float64            `json:"objective_best"`
	Set     map[string]float64 `json:"set,omitempty"`
	Changes []tuneChange       `json:"changes"`
	Records []tuneRecReport    `json:"recordings"`
}

type tuneChange struct {
	Field string  `json:"field"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
}

// tuneChanges lists every searched value that moved (relative change >
// 0.5 %, or any move off zero).
func tuneChanges(dims []tuneDim, y []float64) []tuneChange {
	var out []tuneChange
	for i, dm := range dims {
		v := dm.Start
		if y != nil {
			v = dm.value(y[i])
		}
		if math.Abs(v-dm.Start) > 0.005*math.Max(math.Abs(dm.Start), 1e-9) {
			out = append(out, tuneChange{Field: dm.Field, From: dm.Start, To: v})
		}
	}
	return out
}

type tuneRunOpts struct {
	tuneOpts
	Glob, OutRoot, Score string
	MaxSec               float64
	AddHarmonics         int // < 0 = the pick's
}

// runTune tunes (or, with Score, only scores) one instrument and writes its
// candidate directory. It returns the meta written.
func runTune(w, diag io.Writer, pick tunePick, o tuneRunOpts) (tuneMeta, error) {
	inst := pick.Inst
	start, defs, err := tuneStart(pick)
	if err != nil {
		return tuneMeta{}, err
	}
	var startDoc *instrument.Doc
	if pick.From != "" {
		startDoc = &start
	}
	if o.AddHarmonics >= 0 {
		pick.AddHarmonics = o.AddHarmonics
	}
	tmp, err := os.MkdirTemp("", "beatmo-tune-*")
	if err != nil {
		return tuneMeta{}, err
	}
	defer os.RemoveAll(tmp)
	fmt.Fprintf(diag, "== %s: loading recordings\n", inst)
	recs, err := tuneRecordings(inst, pick.Skip, o.Glob, o.MaxSec, tmp, diag)
	if err != nil {
		return tuneMeta{}, err
	}
	for i := range recs {
		if err := recs[i].settlePitchDoc(inst, startDoc); err != nil {
			return tuneMeta{}, err
		}
	}

	var cand instrument.Doc
	var dims []tuneDim
	res := tuneSearchResult{}
	if o.Score != "" {
		if cand, err = loadTuneCandidate(o.Score); err != nil {
			return tuneMeta{}, err
		}
	} else {
		var d0 instrument.Doc
		if d0, dims, err = tuneSpace(start, defs, pick); err != nil {
			return tuneMeta{}, err
		}
		fmt.Fprintf(diag, "== %s: %d recordings, %d values searched\n", inst, len(recs), len(dims))
		if res, err = tuneSearch(d0, dims, recs, essenceEvaluator(inst), o.tuneOpts, diag); err != nil {
			return tuneMeta{}, err
		}
		if cand, err = applyTune(d0, dims, res.Y); err != nil {
			return tuneMeta{}, err
		}
	}

	meta := tuneMeta{Inst: inst, SamePitch: true, MaxSec: o.MaxSec, Why: pick.Why, From: string(pick.From), DocSHA: clone.DocSHA(cand),
		Created: time.Now().UTC().Format(time.RFC3339), Seed: o.Seed, Gens: res.Gens, Evals: res.Evals,
		Start: res.Start, Best: res.Best, Set: pick.Set, Changes: tuneChanges(dims, res.Y)}
	for _, rec := range recs {
		fmt.Fprintf(diag, "== %s: ranking against the roster on %s\n", inst, rec.Name)
		rr, err := scoreAgainstRoster(inst, cand, rec, o.Workers)
		if err != nil {
			return tuneMeta{}, err
		}
		sp := rec.pitch
		rr.SearchPitch = &sp
		meta.Records = append(meta.Records, rr)
		if meta.Ref == "" && rec.RefID != "" {
			meta.Ref = rec.RefID
		}
	}
	dir := filepath.Join(o.OutRoot, string(inst))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return meta, err
	}
	improved := o.Score != "" || res.Y != nil || len(pick.Set) > 0 || pick.From != ""
	candPath := filepath.Join(dir, "candidate.json")
	if improved {
		b, err := instrument.MarshalFile(cand)
		if err != nil {
			return meta, err
		}
		if err := os.WriteFile(candPath, b, 0o644); err != nil {
			return meta, err
		}
	} else {
		_ = os.Remove(candPath) // nothing beat the shipped voice: nothing to audition
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), mb, 0o644); err != nil {
		return meta, err
	}
	report := tuneReport(meta, improved)
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report), 0o644); err != nil {
		return meta, err
	}
	fmt.Fprint(w, tuneSummary(meta, improved))
	return meta, nil
}

// loadTuneCandidate reads a candidate document: a JSON file, or a directory
// holding candidate.json (a tuned or cloned candidate).
func loadTuneCandidate(path string) (instrument.Doc, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, "candidate.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return instrument.Doc{}, err
	}
	return instrument.ParseFile(b)
}

func rankCell(s tuneScore) string { return fmt.Sprintf("#%d (%.3f)", s.Rank, s.Raw) }

func traitsCell(t fingerprint.EssenceTraits) string {
	return fmt.Sprintf("%.0f Hz · noise %.2f · %.0f dB", t.BrightnessHz, t.Noise, t.DecayDB)
}

func tuneSummary(m tuneMeta, improved bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s: objective %.4f -> %.4f (%d evals)\n", m.Inst, m.Start, m.Best, m.Evals)
	for _, r := range m.Records {
		fmt.Fprintf(&b, "  %-40s shipped %-14s candidate %-14s\n", r.Recording, rankCell(r.Shipped), rankCell(r.Candidate))
	}
	if !improved {
		fmt.Fprintf(&b, "  no candidate: nothing beat the shipped voice\n")
	} else {
		fmt.Fprintf(&b, "  listen: make ab-tuned FAMILIES=%s\n", m.Inst)
	}
	return b.String()
}

func tuneReport(m tuneMeta, improved bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# refs tune: %s\n\n%s · doc_sha `%s`\n\n", m.Inst, m.Created, m.DocSHA)
	if m.Why != "" {
		fmt.Fprintf(&b, "**Why this instrument:** %s\n\n", m.Why)
	}
	if m.From != "" {
		fmt.Fprintf(&b, "**Started from `%s`'s document**, not the shipped %s: the objective's start is %s's distance, and \"Values the search moved\" are moves from %s's values.\n\n", m.From, m.Inst, m.From, m.From)
	}
	fmt.Fprintf(&b, "Objective (mean raw essence over the recordings + %.2f × mean squared move): %.4f → %.4f after %d evaluations (%d generations, seed %d).\n\n",
		tuneRegWeight, m.Start, m.Best, m.Evals, m.Gens, m.Seed)
	if !improved {
		b.WriteString("**No candidate written**: nothing the search tried beat the shipped document.\n\n")
	}
	b.WriteString("## Against every recording (rank among the whole roster, hubness-corrected; raw essence in brackets)\n\n")
	b.WriteString("| recording | shipped | candidate | reference | shipped voice | candidate voice |\n|---|---|---|---|---|---|\n")
	for _, r := range m.Records {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", mdCell(r.Recording), rankCell(r.Shipped), rankCell(r.Candidate),
			traitsCell(r.RefTraits), traitsCell(r.Shipped.Traits), traitsCell(r.Candidate.Traits))
	}
	b.WriteString("\n| recording | term | shipped | candidate |\n|---|---|---|---|\n")
	for _, r := range m.Records {
		for _, t := range []struct {
			n    string
			a, c float64
		}{{"colour", r.Shipped.Terms.Color, r.Candidate.Terms.Color}, {"evolution", r.Shipped.Terms.Evolution, r.Candidate.Terms.Evolution},
			{"texture", r.Shipped.Terms.Texture, r.Candidate.Terms.Texture}, {"decay", r.Shipped.Terms.Decay, r.Candidate.Terms.Decay},
			{"attack", r.Shipped.Terms.Attack, r.Candidate.Terms.Attack}} {
			fmt.Fprintf(&b, "| %s | %s | %.3f | %.3f |\n", mdCell(r.Recording), t.n, t.a, t.c)
		}
	}
	if len(m.Set) > 0 {
		b.WriteString("\n## Edits applied before the search (from the listener's notes)\n\n")
		for _, k := range sortedFloatKeys(m.Set) {
			fmt.Fprintf(&b, "- `%s` = %g\n", k, m.Set[k])
		}
	}
	b.WriteString("\n## Values the search moved\n\n")
	if len(m.Changes) == 0 {
		b.WriteString("(none)\n")
	}
	for _, c := range m.Changes {
		fmt.Fprintf(&b, "- `%s`: %.4g → %.4g\n", c.Field, c.From, c.To)
	}
	b.WriteString("\nThe in-sample ranks above are a screen, not proof: the ear decides (`make ab-tuned FAMILIES=" + string(m.Inst) + "`), and only a `y` lets `make tune-adopt INST=" + string(m.Inst) + "` ship it.\n")
	return b.String()
}

func sortedFloatKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// tunedRoot is where candidates are written: $BEATMO_WAV_REFS/tuned.
func tunedRoot() string { return filepath.Join(wavref.Root(), "tuned") }

func cmdTune(args []string) error {
	fs := flag.NewFlagSet("tune", flag.ExitOnError)
	insts := fs.String("inst", "", "comma-separated instruments (default with -picks: every pick in testdata/tune_picks.json)")
	picks := fs.Bool("picks", false, "tune the curated picks (testdata/tune_picks.json), or those of them named by -inst")
	gens := fs.Int("gens", 40, "CMA-ES generations (0 = score the shipped voice only)")
	lambda := fs.Int("lambda", 0, "population per generation (0 = Hansen's 4 + 3 ln n, at least 8)")
	seed := fs.Uint64("seed", 1, "search seed (a run is a pure function of it)")
	stall := fs.Int("stall", 12, "stop after this many generations without a 0.1 % gain")
	sigma := fs.Float64("sigma", 0.35, "initial CMA-ES step, in sin-box radians")
	harm := fs.Int("add-harmonics", -1, "raise up to N extra harmonics from silence (-1 = the pick's setting)")
	glob := fs.String("wavs", "", "extra recordings (comma-separated globs) whose catalogue entry or filename names the instrument")
	maxSec := fs.Float64("max-sec", adoptMaxSecDefault, "analysis window after the leading-silence trim")
	workers := fs.Int("workers", defaultMatchWorkers(), "parallel renders")
	score := fs.String("score", "", "T1 only: score this candidate (a document JSON, or a directory holding candidate.json) instead of searching")
	out := fs.String("out", "", "candidate root (default $BEATMO_WAV_REFS/tuned)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var want []string
	for _, s := range strings.Split(*insts, ",") {
		if s = strings.TrimSpace(s); s != "" {
			want = append(want, s)
		}
	}
	all, err := loadTunePicks()
	if err != nil {
		return err
	}
	byInst := map[string]tunePick{}
	for _, p := range all {
		byInst[string(p.Inst)] = p
	}
	var run []tunePick
	switch {
	case len(want) > 0:
		for _, i := range want {
			p, ok := byInst[i]
			if !ok {
				if *picks {
					return fmt.Errorf("refs tune: %s is not a pick", i)
				}
				p = tunePick{Inst: instrument.ID(i)}
			}
			run = append(run, p)
		}
	case *picks:
		run = all
	default:
		return fmt.Errorf("refs tune: -inst <id[,id]> or -picks is required")
	}
	if *score != "" && len(run) != 1 {
		return fmt.Errorf("refs tune: -score takes exactly one -inst")
	}
	root := *out
	if root == "" {
		root = tunedRoot()
	}
	defer persistMatchFrames(filepath.Join(wavref.Root(), "matches", "frames.json"))()
	o := tuneRunOpts{tuneOpts: tuneOpts{Gens: *gens, Lambda: *lambda, Workers: *workers, Stall: *stall, Seed: *seed, Sigma: *sigma},
		Glob: *glob, OutRoot: root, Score: *score, MaxSec: *maxSec, AddHarmonics: *harm}
	var failed []string
	for _, p := range run {
		if _, err := runTune(os.Stdout, os.Stderr, p, o); err != nil {
			fmt.Fprintf(os.Stderr, "refs tune: %s: %v\n", p.Inst, err)
			failed = append(failed, string(p.Inst))
		}
	}
	fmt.Printf("\ncandidates: %s\nlisten: make ab-tuned [FAMILIES=a,b]   ship after a y: make tune-adopt INST=<id>\n", root)
	if len(failed) > 0 {
		return fmt.Errorf("refs tune: failed: %s", strings.Join(failed, ", "))
	}
	return nil
}
