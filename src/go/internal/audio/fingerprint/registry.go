package fingerprint

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultRegistry, filled by init() in the registry_*.go files, is the single source of the battery: consumers iterate it,
// so a descriptor cannot exist on one surface and be missing on another.

// Registry holds descriptors keyed by their stable ID.
type Registry struct {
	byID  map[string]Descriptor
	order []string // insertion order, retained for stable All() tie-breaking

	// nil disables calibration, which is what NewRegistry gives tests; only DefaultRegistry loads the embedded file.
	thresholdFile ThresholdFile
	// A sync.Once, not a bool: ensureCalibrated WRITES byID from a read path, so concurrent All() calls would race on the map
	// without its happens-before.
	calibrateOnce sync.Once
	// So Register keeps late arrivals in step.
	calibrated bool
	// For the report's CAL column.
	sources map[string]string
}

// It does NOT load the embedded threshold file: a test asserting a synthetic descriptor's thresholds must not have
// them rewritten by calibration.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Descriptor)}
}

var defaultRegistry = &Registry{
	byID:          make(map[string]Descriptor),
	thresholdFile: LoadThresholdFile(),
}

func DefaultRegistry() *Registry { return defaultRegistry }

// Panics on a duplicate or empty ID or a nil Extract: failing at startup beats a report that silently drops or shadows
// a measurement.
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
	// A row registered after calibration must still be calibrated, or honouring the file would depend on init order.
	// Reading r.calibrated here is safe because Register runs only at init, on one goroutine; taking the Once would
	// calibrate a half-populated table and latch it done.
	if r.calibrated {
		r.applyThreshold(&d)
	}
	r.byID[d.ID] = d
	r.order = append(r.order, d.ID)
}

// A non-positive Mismatch is ignored: it would mean "never fires", a malformed row, and silently muting a descriptor is
// the worst failure for a diagnostic.
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

// On every read path: the table is filled by init() across files, so first read is the only moment it is guaranteed
// complete.
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

// "corpus", "probe", "hand" (an explicit exclusion), or "" when the descriptor's own thresholds stand.
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

// REGISTRATION order, deterministic (files by name, then top to bottom), keeping the report's narrative order;
// registry files are named to match. The slice is freshly allocated.
func (r *Registry) All() []Descriptor {
	r.ensureCalibrated()
	out := make([]Descriptor, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}

// A canonical order independent of registration, for goldens that must not churn on a mid-file insert.
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

// Evaluated on the REFERENCE in a comparison: a synth that failed to sustain must not suppress the sustain rows, which
// would hide exactly the defect looked for.
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
