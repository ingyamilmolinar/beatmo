package fingerprint

import (
	"fmt"
	"sort"
	"sync"
)

// registry.go — the descriptor table.
//
// One package-level DefaultRegistry, populated by init() in the registry_*.go
// files, is the single source of truth for the analysis battery. Consumers
// (cmd/refs/diagnose, the validation harnesses, future report surfaces)
// iterate it rather than hand-listing measurements, so a descriptor cannot
// exist in one surface and be missing from another.

// Registry holds descriptors keyed by their stable ID.
type Registry struct {
	byID  map[string]Descriptor
	order []string // insertion order, retained for stable All() tie-breaking

	// thresholdFile overrides Drift/Mismatch per descriptor ID. Nil disables
	// calibration entirely, which is what NewRegistry gives tests; only
	// DefaultRegistry loads the embedded file. See thresholds.go.
	thresholdFile ThresholdFile
	// calibrateOnce runs the calibration pass exactly once. It is a
	// sync.Once and not a plain bool because ensureCalibrated WRITES to byID
	// from what is otherwise a read path: without the Once's happens-before,
	// two goroutines calling All() concurrently would race on the map. Every
	// read path funnels through it, so all readers synchronize on it.
	calibrateOnce sync.Once
	// calibrated records that the pass has run, so Register can keep late
	// arrivals in step.
	calibrated bool
	// sources maps descriptor ID to the applied entry's Source, for the
	// report's CAL column.
	sources map[string]string
}

// NewRegistry returns an empty registry. Tests use this; production code uses
// DefaultRegistry.
//
// It deliberately does NOT load the embedded threshold file: a test that
// registers a synthetic descriptor and asserts on its thresholds must not
// have them silently rewritten by a calibration artifact. Tests that want
// calibration set thresholdFile themselves.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Descriptor)}
}

var defaultRegistry = &Registry{
	byID:          make(map[string]Descriptor),
	thresholdFile: LoadThresholdFile(),
}

// DefaultRegistry returns the package-level registry that the registry_*.go
// init() functions populate.
func DefaultRegistry() *Registry { return defaultRegistry }

// Register adds d.
//
// Panics on a duplicate ID, an empty ID, or a nil Extract. These are
// programming errors in a table that is populated at init time — failing
// loudly at startup is strictly better than a report that silently drops or
// silently shadows a measurement.
func (r *Registry) Register(d Descriptor) {
	if d.ID == "" {
		panic("fingerprint: Register called with an empty descriptor ID")
	}
	if d.Extract == nil {
		panic(fmt.Sprintf("fingerprint: descriptor %q has a nil Extract", d.ID))
	}
	if _, dup := r.byID[d.ID]; dup {
		panic(fmt.Sprintf("fingerprint: duplicate descriptor ID %q", d.ID))
	}
	// A row registered after calibration has already happened (init order is
	// not something a caller controls) must still be calibrated, otherwise
	// whether a descriptor honours the file would depend on which file's
	// init() ran first.
	//
	// Reading r.calibrated outside calibrateOnce is safe because Register is
	// an INIT-TIME call — the registry_*.go init() functions, plus tests
	// building their own Registry — and package init runs on one goroutine
	// before any reader exists. Taking the Once here instead would be worse
	// than useless: it would run the full calibration pass over a
	// half-populated table and latch it as done.
	if r.calibrated {
		r.applyThreshold(&d)
	}
	r.byID[d.ID] = d
	r.order = append(r.order, d.ID)
}

// applyThreshold rewrites d's thresholds from the calibration file, if it has
// an entry.
//
// An entry with a non-positive Mismatch is IGNORED rather than applied: a zero
// mismatch would mean "this axis never fires", which is not a threshold but a
// malformed row, and silently muting a descriptor is the worst possible
// failure mode for a diagnostic.
func (r *Registry) applyThreshold(d *Descriptor) {
	if r.thresholdFile == nil {
		return
	}
	e, ok := r.thresholdFile[d.ID]
	if !ok || e.Mismatch <= 0 {
		return
	}
	d.Drift, d.Mismatch = e.Drift, e.Mismatch
	if r.sources == nil {
		r.sources = map[string]string{}
	}
	r.sources[d.ID] = e.Source
}

// ensureCalibrated applies the threshold file to every row, once.
//
// It is called from every read path rather than from NewRegistry because the
// registry is populated by init() functions across several files: at
// construction time there is nothing to calibrate, and there is no later
// moment the package controls. Doing it lazily on first read is the only
// point at which the table is guaranteed complete.
func (r *Registry) ensureCalibrated() {
	r.calibrateOnce.Do(func() {
		r.calibrated = true
		for _, id := range r.order {
			d := r.byID[id]
			r.applyThreshold(&d)
			r.byID[id] = d
		}
	})
}

// CalibratedFrom reports where the descriptor's thresholds came from:
// "corpus", "probe", "hand" (an explicit exclusion recorded in the file), or
// "" when no file entry applied and the descriptor's own hand-written
// thresholds stand.
func (r *Registry) CalibratedFrom(id string) string {
	r.ensureCalibrated()
	return r.sources[id]
}

// ByID returns the descriptor with the given ID.
func (r *Registry) ByID(id string) (Descriptor, bool) {
	r.ensureCalibrated()
	d, ok := r.byID[id]
	return d, ok
}

// Len returns the number of registered descriptors.
func (r *Registry) Len() int { return len(r.order) }

// All returns every descriptor in REGISTRATION order.
//
// Registration order is deterministic — Go runs each file's init() in
// filename order, and within a file top to bottom — so goldens and report
// output are stable. It is preferred over sorting by (Group, ID) because it
// preserves the deliberate narrative order the report groups read in (noise,
// then envelope, then partial structure, then brightness, then modulation —
// the order of docs/superpowers/specs/2026-08-22-verdict-to-metric-mapping.md's
// own argument). Alphabetical sorting would scramble that into
// brightness-first and put "spectral flatness" ahead of "noise fraction".
//
// Registry files are therefore named so filename order matches intended
// report order: registry_core.go, then registry_family.go, then
// registry_tracks.go.
//
// The returned slice is freshly allocated; callers cannot mutate the registry
// through it.
func (r *Registry) All() []Descriptor {
	r.ensureCalibrated()
	out := make([]Descriptor, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}

// SortedByGroup returns every descriptor sorted by (Group, ID), for consumers
// that want a canonical ordering independent of registration — golden files
// that must not churn when a descriptor is inserted mid-file, for instance.
func (r *Registry) SortedByGroup() []Descriptor {
	out := r.All()
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Filter returns the descriptors matching every non-nil criterion in f.
type Filter struct {
	Group *string
	Scope *Scope
	Repr  *Repr
}

// Filter returns the descriptors matching f, in All() order.
func (r *Registry) Filter(f Filter) []Descriptor {
	var out []Descriptor
	for _, d := range r.All() {
		if f.Group != nil && d.Group != *f.Group {
			continue
		}
		if f.Scope != nil && d.Scope != *f.Scope {
			continue
		}
		if f.Repr != nil && d.Repr != *f.Repr {
			continue
		}
		out = append(out, d)
	}
	return out
}

// Applicable returns the descriptors whose Applies predicate accepts b, in
// All() order.
//
// Applicability is evaluated against ONE bundle — in a reference-vs-synth
// comparison that must be the REFERENCE, since the reference defines what
// kind of instrument this is. Letting a synth that failed to sustain suppress
// the sustain descriptors would hide exactly the defect being looked for.
func (r *Registry) Applicable(b *Bundle) []Descriptor {
	var out []Descriptor
	for _, d := range r.All() {
		if d.Applies != nil && d.Applies(b) {
			out = append(out, d)
		}
	}
	return out
}

// Groups returns the distinct group names in All() order.
func (r *Registry) Groups() []string {
	seen := make(map[string]bool)
	var out []string
	for _, d := range r.All() {
		if !seen[d.Group] {
			seen[d.Group] = true
			out = append(out, d.Group)
		}
	}
	return out
}
