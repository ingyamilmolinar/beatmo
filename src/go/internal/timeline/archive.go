package timeline

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Weeks of constant activity: far past any session, a guard against runaway. Overflow drops the oldest in chunks to
// amortise the cost.
const archiveDefaultMaxEntries = 2_000_000

// archiveEvictChunk amortizes the cost of dropping over-cap entries.
const archiveEvictChunk = 65_536

// Sorted by abs, so lookups binary-search; int32 abs covers ~2.1B subdivisions.
type archiveEntry struct {
	abs  int32
	val  bool
	typ  model.NodeType
	kind CommitKind
}

// Playback/Import commits pruned from the live immutables sidecar, sparse (gap pads are not migrated). Monotonic appends
// (playback moving forward) are amortised O(1); out-of-order migrations (paused scrub, edits, import) take a rare sorted
// insert.
type rowArchive struct {
	entries []archiveEntry
}

// Never overwrites an existing Playback/Import entry at the same abs; past maxEntries (> 0) the oldest drop in chunks.
func (a *rowArchive) append(abs int, val bool, typ model.NodeType, kind CommitKind, maxEntries int) {
	if a == nil {
		return
	}
	e := archiveEntry{abs: int32(abs), val: val, typ: typ, kind: kind}
	n := len(a.entries)
	if n == 0 || a.entries[n-1].abs < e.abs {
		a.entries = append(a.entries, e)
	} else {
		pos := sort.Search(n, func(i int) bool { return a.entries[i].abs >= e.abs })
		if pos < n && a.entries[pos].abs == e.abs {
			if a.entries[pos].kind == CommitKindPlayback || a.entries[pos].kind == CommitKindImport {
				return
			}
			a.entries[pos] = e
			return
		}
		a.entries = append(a.entries, archiveEntry{})
		copy(a.entries[pos+1:], a.entries[pos:])
		a.entries[pos] = e
	}
	if maxEntries > 0 && len(a.entries) > maxEntries {
		drop := len(a.entries) - maxEntries
		if drop < archiveEvictChunk {
			drop = archiveEvictChunk
		}
		if drop > len(a.entries) {
			drop = len(a.entries)
		}
		// Compact in place so capacity is reused.
		a.entries = append(a.entries[:0], a.entries[drop:]...)
	}
}

// lookup returns the entry at abs, or ok=false if no entry exists.
func (a *rowArchive) lookup(abs int) (val bool, typ model.NodeType, kind CommitKind, ok bool) {
	if a == nil {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	n := len(a.entries)
	if n == 0 {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	pos := sort.Search(n, func(i int) bool { return int(a.entries[i].abs) >= abs })
	if pos >= n || int(a.entries[pos].abs) != abs {
		return false, model.NodeTypeInvisible, CommitKindPlayback, false
	}
	e := a.entries[pos]
	return e.val, e.typ, e.kind, true
}

// So user-driven trims reach the archive too.
func (a *rowArchive) trimBefore(minAbs int) {
	if a == nil || len(a.entries) == 0 {
		return
	}
	n := len(a.entries)
	pos := sort.Search(n, func(i int) bool { return int(a.entries[i].abs) >= minAbs })
	if pos > 0 {
		a.entries = append(a.entries[:0], a.entries[pos:]...)
	}
}

// trimAfter drops entries with abs > maxAbs. Mirrors TrimAfter semantics.
func (a *rowArchive) trimAfter(maxAbs int) {
	if a == nil || len(a.entries) == 0 {
		return
	}
	n := len(a.entries)
	pos := sort.Search(n, func(i int) bool { return int(a.entries[i].abs) > maxAbs })
	if pos < n {
		a.entries = a.entries[:pos]
	}
}

// takeFrom removes and returns the entries with abs >= minAbs (abs order).
func (a *rowArchive) takeFrom(minAbs int) []archiveEntry {
	if a == nil || len(a.entries) == 0 {
		return nil
	}
	n := len(a.entries)
	pos := sort.Search(n, func(i int) bool { return int(a.entries[i].abs) >= minAbs })
	if pos >= n {
		return nil
	}
	out := append([]archiveEntry(nil), a.entries[pos:]...)
	a.entries = a.entries[:pos]
	return out
}

// clear empties the archive while retaining capacity for reuse.
func (a *rowArchive) clear() {
	if a == nil {
		return
	}
	a.entries = a.entries[:0]
}

// lenForTest returns the current entry count. Test-only diagnostic.
func (a *rowArchive) lenForTest() int {
	if a == nil {
		return 0
	}
	return len(a.entries)
}
