// Package userprefs persists per-user UI state outside of the project
// file. The first consumer is the instrument-menu favorites set
// (filled-star toggles); the package is structured so other per-user
// preferences (pinned categories, recent items, layout overrides) can
// land alongside without changing the storage contract.
//
// Backends are selected by build tag:
//
//   - //go:build js   — localStorage (synchronous, key beatmo.favorites.v1)
//   - //go:build !js  — JSON file at os.UserConfigDir()/beatmo/favorites.json
//     written via atomic rename on a single-worker async.Pool named
//     "userprefs.persist" (lazy-acquired on first SaveFavorites).
//
// Load semantics match across backends: missing/malformed/version-mismatched
// data returns an empty map without error so the caller can assume a Store
// always returns a usable map. Logged warnings make a corrupted file
// recoverable by the operator.
package userprefs

import (
	"encoding/json"
	"sort"
)

// SchemaVersion is the current on-disk / localStorage schema version.
// Bumping this is the trigger for forward-compat handling: reads of a
// future version return empty + warning; the file is left in place so
// the user can roll back.
const SchemaVersion = 1

// LocalStorageKey is the WebStorage key used by the WASM backend.
// Stable name so a user's stars survive a client refresh.
const LocalStorageKey = "beatmo.favorites.v1"

// PoolName is the name registered with async.DefaultRegistry on
// desktop builds the first time SaveFavorites is invoked. Surfaced as
// a constant so tests and the CLAUDE.md standing-pools table reference
// the same string.
const PoolName = "userprefs.persist"

// favoritesV1 is the on-the-wire shape. Kept private; callers exchange
// a map[string]bool with the store.
type favoritesV1 struct {
	Version   int      `json:"version"`
	Favorites []string `json:"favorites"`
}

// Store persists a single set of favorited instrument IDs. Implementations
// must be safe for concurrent use from the UI goroutine (LoadFavorites,
// SaveFavorites) and from the background worker that drains the queue.
//
// SaveFavorites is non-blocking on the desktop backend (returns once the
// job is queued); WaitFlushed exists for tests and clean shutdown that
// need to know the disk is consistent.
type Store interface {
	LoadFavorites() (map[string]bool, error)
	SaveFavorites(map[string]bool) error
	WaitFlushed() error
	Close() error
}

// Options configure a Store at construction. Zero values pick
// production defaults; tests pass overrides.
type Options struct {
	// Path overrides the desktop file location. Empty = use the
	// platform default (os.UserConfigDir/beatmo/favorites.json).
	// Ignored by the WASM backend.
	Path string

	// PoolName overrides the async-pool name. Empty = "userprefs.persist".
	// Tests pass a per-test name to avoid sharing a long-lived production
	// pool with sibling tests.
	PoolName string

	// Logger receives non-fatal warnings (corrupt file, version mismatch,
	// pool backpressure). nil = silent.
	Logger func(format string, args ...any)
}

// NewBackingStore returns the platform-appropriate Store. The body lives
// in store_js.go / store_notjs.go.
func NewBackingStore(opts Options) Store {
	return newBackingStore(opts)
}

// marshalFavorites serializes the map as a sorted v1 document. Sort
// makes the on-disk diff stable across saves with the same content.
func marshalFavorites(favs map[string]bool) ([]byte, error) {
	ids := make([]string, 0, len(favs))
	for id, ok := range favs {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	doc := favoritesV1{Version: SchemaVersion, Favorites: ids}
	return json.Marshal(doc)
}

// parseFavorites is the cross-platform load semantics. It NEVER returns
// an error to the caller — every fault produces an empty map plus a
// logged warning. The file is left intact so the user can recover.
func parseFavorites(data []byte, logf func(string, ...any)) map[string]bool {
	if len(data) == 0 {
		return map[string]bool{}
	}
	var doc favoritesV1
	if err := json.Unmarshal(data, &doc); err != nil {
		if logf != nil {
			logf("[USERPREFS] favorites: corrupted JSON, ignoring: %v", err)
		}
		return map[string]bool{}
	}
	if doc.Version != SchemaVersion {
		if logf != nil {
			logf("[USERPREFS] favorites: schema version %d unsupported (want %d), ignoring", doc.Version, SchemaVersion)
		}
		return map[string]bool{}
	}
	out := make(map[string]bool, len(doc.Favorites))
	for _, id := range doc.Favorites {
		if id == "" {
			continue
		}
		out[id] = true
	}
	return out
}
