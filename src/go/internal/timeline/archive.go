package timeline

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// archiveDefaultMaxEntries is the per-row hard ceiling on cold-archive size.
// Tuned to ~24 days of constant 1 ev/s/row activity — far beyond any human
// session length while guarding against pathological runaway. When exceeded
// the oldest entries are dropped in 65 536-entry chunks to amortize cost.
const archiveDefaultMaxEntries = 2_000_000

// archiveEvictChunk amortizes the cost of dropping over-cap entries.
const archiveEvictChunk = 65_536

// archiveEntry is a packed historical commit. Stored sorted by abs ascending
// so binary search resolves lookups in O(log n). int32 abs covers up to
// ~2.1B subdivisions (≈ 8 200 days at 8 subdiv/s).
type archiveEntry struct {
	abs  int32
	val  bool
	typ  model.NodeType
	kind CommitKind
}

// rowArchive is the per-row append-mostly cold-store of historical
// CommitKindPlayback / CommitKindImport entries that have been pruned from
// the live immutables sidecar. Sparse: only abs values that actually
// committed appear (gap-pad entries are NOT migrated).
//
// Append is amortized O(1) when calls are monotonic — the dominant case
// since playback advances forward. Out-of-order migrations (paused scrub,
// edit/import flows) fall through to a sort-position insert which is
// O(log n) for the search and O(n) for the slice shift; this path is rare.
type rowArchive struct {
	entries []archiveEntry
}

// append inserts e into the archive in abs-sorted order. Existing immutable
// entries (Playback/Import) at the same abs are never overwritten. When the
// archive exceeds maxEntries (when > 0), oldest entries are dropped in
// archiveEvictChunk-sized batches to keep amortized cost O(1).
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

// trimBefore drops entries with abs < minAbs. Mirrors TrimBefore semantics
// so explicit user-driven trims propagate to the cold archive.
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
