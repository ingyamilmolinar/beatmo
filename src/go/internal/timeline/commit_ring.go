package timeline

import "github.com/ingyamilmolinar/beatmo/core/model"

// commitRing keeps the most recent committed beats in-order. Entries are
// addressed by absolute subdivision index; once written they are immutable.
//
// capMax bounds the underlying slice length. When zero (uninitialized) the
// ring grows geometrically without limit — the historical behavior, retained
// for callers that have not opted in to a retention policy. When set, the
// ring stops doubling at capMax and drops the oldest entry on append. The
// per-row immutables sidecar in Service preserves history that the ring has
// trimmed, so reconcileFrozen lookups for past abs still resolve correctly.
type commitRing struct {
	base   int // absolute index stored at head
	size   int // number of valid entries
	head   int // ring index for base
	capMax int // 0 = unlimited; otherwise hard ceiling on len(vals)
	vals   []bool
	types  []model.NodeType
	kinds  []CommitKind
}

// setCapMax records the hard ceiling on ring capacity. Idempotent: only the
// largest value sticks, so a caller cannot accidentally shrink an existing
// ceiling on a row whose ring already grew.
func (r *commitRing) setCapMax(n int) {
	if n <= 0 || r == nil {
		return
	}
	if r.capMax == 0 || n > r.capMax {
		r.capMax = n
	}
}

func (r *commitRing) ensureCapacity(capacity int) {
	if len(r.kinds) != len(r.vals) {
		r.kinds = make([]CommitKind, len(r.vals))
	}
	if capacity <= len(r.vals) {
		return
	}
	if capacity <= 0 {
		capacity = 0
	}
	// Honor the ring's hard ceiling. Callers asking for more than capMax
	// receive a ring sized to capMax; append() handles the wrap-and-drop on
	// further writes.
	if r.capMax > 0 && capacity > r.capMax {
		capacity = r.capMax
	}
	if capacity <= len(r.vals) {
		return
	}
	newVals := make([]bool, capacity)
	newTypes := make([]model.NodeType, capacity)
	newKinds := make([]CommitKind, capacity)
	if r.size > 0 && len(r.vals) > 0 {
		for i := 0; i < r.size; i++ {
			idx := (r.head + i) % len(r.vals)
			newVals[i] = r.vals[idx]
			newTypes[i] = r.types[idx]
			newKinds[i] = r.kinds[idx]
		}
	}
	r.vals = newVals
	r.types = newTypes
	r.kinds = newKinds
	r.head = 0
	if r.size > capacity {
		r.size = capacity
		if r.size == 0 {
			r.base = 0
		}
	}
}

func (r *commitRing) clear() {
	r.base = 0
	r.size = 0
	r.head = 0
	if len(r.vals) > 0 {
		r.kinds = make([]CommitKind, len(r.vals))
	} else {
		r.kinds = nil
	}
}

func (r *commitRing) last() int {
	if r.size == 0 {
		return -1
	}
	return r.base + r.size - 1
}

func (r *commitRing) trimBefore(minAbs int) {
	if r.size == 0 {
		return
	}
	if minAbs <= r.base {
		return
	}
	if minAbs >= r.base+r.size {
		r.clear()
		r.base = minAbs
		return
	}
	drop := minAbs - r.base
	r.base = minAbs
	r.size -= drop
	if len(r.vals) == 0 {
		r.head = 0
		return
	}
	r.head = (r.head + drop) % len(r.vals)
}

func (r *commitRing) trimAfter(maxAbs int) {
	if r.size == 0 {
		return
	}
	last := r.base + r.size - 1
	if maxAbs >= last {
		return
	}
	if maxAbs < r.base {
		r.clear()
		return
	}
	newSize := maxAbs - r.base + 1
	if newSize < 0 {
		newSize = 0
	}
	if newSize < r.size {
		r.size = newSize
	}
}

func (r *commitRing) trimAfterMutable(maxAbs int) {
	for r.size > 0 {
		last := r.base + r.size - 1
		if last <= maxAbs {
			return
		}
		idx := (r.head + (r.size - 1)) % len(r.vals)
		kind := r.kinds[idx]
		if kind == CommitKindPlayback || kind == CommitKindImport {
			return
		}
		r.size--
	}
}

func (r *commitRing) append(abs int, val bool, typ model.NodeType, kind CommitKind) {
	if len(r.vals) == 0 {
		return
	}
	// Grow the ring instead of evicting immutable history when capacity is
	// exhausted, up to capMax. Past playback/import commits beyond the ceiling
	// are preserved by Service.immutables, so dropping the oldest ring entry
	// here is safe.
	if r.size == len(r.vals) {
		newCap := len(r.vals) * 2
		if newCap == 0 {
			newCap = 1
		}
		if r.capMax > 0 && newCap > r.capMax {
			// At ceiling: drop oldest in place rather than continuing to grow.
			r.head = (r.head + 1) % len(r.vals)
			r.base++
			r.size--
		} else {
			r.ensureCapacity(newCap)
			// If capacity could not grow (len still zero), bail to avoid overwrite.
			if len(r.vals) == 0 {
				return
			}
		}
	}
	if r.size == 0 {
		r.base = abs
		r.head = 0
		r.size = 1
		r.vals[0] = val
		r.types[0] = typ
		r.kinds[0] = kind
		return
	}
	last := r.base + r.size - 1
	if abs <= last {
		if abs >= r.base {
			idx := (r.head + (abs - r.base)) % len(r.vals)
			existingKind := r.kinds[idx]
			if existingKind != CommitKindPlayback && existingKind != CommitKindImport {
				r.vals[idx] = val
				r.types[idx] = typ
				r.kinds[idx] = kind
			}
		}
		return
	}
	expected := last + 1
	if abs > expected {
		// Preserve existing history by padding the gap with invisible commits.
		gap := abs - expected
		need := r.size + gap + 1
		r.ensureCapacity(need)
		if need > len(r.vals) {
			return
		}
		for i := 0; i < gap; i++ {
			idx := (r.head + r.size) % len(r.vals)
			r.vals[idx] = false
			r.types[idx] = model.NodeTypeInvisible
			r.kinds[idx] = CommitKindGapPad
			r.size++
		}
	}
	if r.size == len(r.vals) {
		r.ensureCapacity(r.size + 1)
		if r.size == len(r.vals) {
			// Capacity could not grow (unlikely); drop oldest to proceed.
			r.head = (r.head + 1) % len(r.vals)
			r.base++
			r.size--
		}
	}
	idx := (r.head + r.size) % len(r.vals)
	r.vals[idx] = val
	r.types[idx] = typ
	r.kinds[idx] = kind
	r.size++
}

func (r *commitRing) value(abs int) (bool, model.NodeType, CommitKind, bool) {
	if r.size == 0 {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	if abs < r.base || abs >= r.base+r.size {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	if len(r.vals) == 0 {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	idx := (r.head + (abs - r.base)) % len(r.vals)
	return r.vals[idx], r.types[idx], r.kinds[idx], true
}

// replace updates an existing commit in-place. It is safe for seeded/gap
// entries; callers should avoid mutating immutable playback/import history.
func (r *commitRing) replace(abs int, val bool, typ model.NodeType, kind CommitKind) bool {
	if r.size == 0 || abs < r.base || abs >= r.base+r.size || len(r.vals) == 0 {
		return false
	}
	idx := (r.head + (abs - r.base)) % len(r.vals)
	r.vals[idx] = val
	r.types[idx] = typ
	if len(r.kinds) > 0 {
		r.kinds[idx] = kind
	}
	return true
}
