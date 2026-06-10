// Package userprefs persists per-user UI state outside of the project
// file. The first consumer is the instrument-menu favorites set
// (filled-star toggles); the package is structured so other per-user
// preferences (pinned categories, recent items, layout overrides) can
// land alongside without changing the storage contract.
//
// Backends are selected by build tag:
//
//   - //go:build js   — localStorage (synchronous, key beatmo.prefs)
//   - //go:build !js  — JSON file at os.UserConfigDir()/beatmo/prefs.json
//     written via atomic rename on a single-worker async.Pool named
//     "userprefs.persist" (lazy-acquired on first SaveFavorites).
//
// Load semantics match across backends: missing/malformed data returns an
// empty state without error so the caller can assume a Store always returns
// a usable result. Logged warnings make a corrupted file recoverable by the
// operator.
package userprefs

import (
	"encoding/json"
	"sort"
)

// LocalStorageKey is the WebStorage key the WASM backend reads and writes.
const LocalStorageKey = "beatmo.prefs"

// FileName is the prefs file name the desktop backend reads and writes
// under os.UserConfigDir()/beatmo/.
const FileName = "prefs.json"

// PoolName is the name registered with async.DefaultRegistry on
// desktop builds the first time SaveFavorites is invoked. Surfaced as
// a constant so tests and the CLAUDE.md standing-pools table reference
// the same string.
const PoolName = "userprefs.persist"

// prefsDoc is the on-the-wire shape. RecipeOverrides carries per-recipe
// param patches; UserRecipes carries opaque RecipeDoc bytes (the audio
// package owns decoding — keeping the shape opaque here prevents an
// audio↔userprefs import cycle).
type prefsDoc struct {
	Favorites       []string                      `json:"favorites,omitempty"`
	RecipeOverrides map[string]map[string]float64 `json:"recipe_overrides,omitempty"`
	UserRecipes     map[string]json.RawMessage    `json:"user_recipes,omitempty"`
	// Phase 5 audio-panel redesign (userprefs v3): tab-specific
	// audio-panel state that should survive across sessions. omitempty
	// so v1/v2 prefs files don't grow until the user actually changes
	// any value; SaveAudioPanelState writes nil when the whole struct
	// is at defaults.
	AudioPanel *AudioPanelStateDoc `json:"audio_panel,omitempty"`
	// SampleEdits carries the non-destructive Sampler-edit descriptors:
	// instrument-id → field-name → value (bools as 0/1). Tiny flat maps,
	// so they live in prefs.json like RecipeOverrides — NOT in the PCM
	// SampleStore. The audio package owns the SampleEdit↔map conversion.
	SampleEdits map[string]map[string]float64 `json:"sample_edits,omitempty"`
}

// AudioPanelStateDoc mirrors the persisted shape of the audio-panel
// state. Exposed via LoadAudioPanelState / SaveAudioPanelState. The
// UI uses an internal struct that wraps this with sensible defaults
// when the field is nil — we pin the JSON shape here so future
// renames don't break older save files.
type AudioPanelStateDoc struct {
	SpectrumSlopeIdx int  `json:"spectrum_slope_idx,omitempty"`
	PreOverlay       bool `json:"pre_overlay,omitempty"`
	K20View          bool `json:"k20_view,omitempty"`
	ChainTapA        int  `json:"chain_tap_a,omitempty"`
	ChainTapB        int  `json:"chain_tap_b,omitempty"`
}

// IsDefault reports whether every field is at its zero value. The
// persistence layer uses this to skip serialising the section when
// no user override is active, keeping older prefs files small.
func (s AudioPanelStateDoc) IsDefault() bool {
	return s.SpectrumSlopeIdx == 0 && !s.PreOverlay && !s.K20View && s.ChainTapA == 0 && s.ChainTapB == 0
}

// Store persists a single set of favorited instrument IDs. Implementations
// must be safe for concurrent use from the UI goroutine (LoadFavorites,
// SaveFavorites) and from the background worker that drains the queue.
//
// SaveFavorites is non-blocking on the desktop backend (returns once the
// job is queued); WaitFlushed exists for tests and clean shutdown that
// need to know the disk is consistent.
//
// Both concrete backends (fileStore, localStorageStore) also implement
// RecipeStore — callers that need the recipe surface should type-assert
// the return value from NewBackingStore.
type Store interface {
	LoadFavorites() (map[string]bool, error)
	SaveFavorites(map[string]bool) error
	WaitFlushed() error
	Close() error
}

// RecipeStore is the sibling surface for synth-recipe persistence.
// Implementations are the same concrete structs that satisfy Store;
// expose this view via type assertion. All write paths funnel through the
// same userprefs.persist pool as favorites and write the full prefs
// document atomically.
//
// User-recipe payloads are opaque to userprefs — the audio package
// marshals/unmarshals its own RecipeDoc shape into the bytes.
type RecipeStore interface {
	// LoadRecipeOverrides returns the recipe-id → param-name → value map
	// currently persisted. Empty map is returned (never nil) when no
	// overrides exist or the store is empty.
	LoadRecipeOverrides() (map[string]map[string]float64, error)
	// SaveRecipeOverride upserts an override for the given recipe id.
	// Nil or empty params deletes the override (equivalent to
	// DeleteRecipeOverride).
	SaveRecipeOverride(recipeID string, params map[string]float64) error
	// DeleteRecipeOverride removes any persisted override for the given
	// recipe id. No-op if no override exists.
	DeleteRecipeOverride(recipeID string) error
	// LoadUserRecipes returns recipe-id → opaque doc bytes. The audio
	// package decodes the bytes into RecipeDoc.
	LoadUserRecipes() (map[string][]byte, error)
	// SaveUserRecipe upserts the recipe payload for the given id.
	SaveUserRecipe(recipeID string, doc []byte) error
	// DeleteUserRecipe removes any persisted user recipe for the given
	// id. No-op if not present.
	DeleteUserRecipe(recipeID string) error
}

// AudioPanelStore is the sibling surface for audio-panel UI state
// persistence (Phase 5 redesign). Implementations share the same
// concrete struct that satisfies Store + RecipeStore; expose this
// view via type assertion. Save/Load are non-blocking on the
// desktop backend and synchronous in localStorage on browser builds.
type AudioPanelStore interface {
	LoadAudioPanelState() (AudioPanelStateDoc, error)
	SaveAudioPanelState(state AudioPanelStateDoc) error
}

// SampleBlob is one persisted Sampler-tab user sample: raw little-endian
// float32 PCM bytes plus the sample rate. The audio package owns the
// float32↔bytes conversion (keeping userprefs audio-agnostic), exactly as
// UserRecipes keeps RecipeDoc decoding in the audio package.
type SampleBlob struct {
	SampleRate int
	PCM        []byte
}

// SampleStore persists Sampler-tab user samples across sessions. PCM payloads
// are large (MBs), so implementations store them OUTSIDE prefs.json /
// localStorage: one file per sample on desktop, an IndexedDB object store in
// the browser (localStorage's ~5 MB cap can't hold them). Expose via type
// assertion on the Store returned by NewBackingStore.
type SampleStore interface {
	// LoadSamples returns id → blob for every persisted sample (never nil).
	LoadSamples() (map[string]SampleBlob, error)
	// SaveSample upserts the blob for id.
	SaveSample(id string, blob SampleBlob) error
	// DeleteSample removes the persisted sample for id. No-op if absent.
	DeleteSample(id string) error
}

// SampleEditStore is the sibling surface for the non-destructive sample-edit
// descriptors (Sampler-tab Save on a synth source). Same shape as the
// RecipeStore override methods: instrument-id keyed flat float maps inside
// the unified prefs document. Expose via type assertion on the Store
// returned by NewBackingStore.
type SampleEditStore interface {
	// LoadSampleEdits returns instrument-id → field → value for every
	// persisted descriptor. Empty map (never nil) when none exist.
	LoadSampleEdits() (map[string]map[string]float64, error)
	// SaveSampleEdit upserts the descriptor fields for an instrument.
	// Nil or empty fields deletes the entry.
	SaveSampleEdit(instID string, fields map[string]float64) error
	// DeleteSampleEdit removes the persisted descriptor. No-op if absent.
	DeleteSampleEdit(instID string) error
}

// Options configure a Store at construction. Zero values pick
// production defaults; tests pass overrides.
type Options struct {
	// Path overrides the desktop file location. Empty = use the
	// platform default (os.UserConfigDir/beatmo/prefs.json).
	// Ignored by the WASM backend.
	Path string

	// PoolName overrides the async-pool name. Empty = "userprefs.persist".
	// Tests pass a per-test name to avoid sharing a long-lived production
	// pool with sibling tests.
	PoolName string

	// Logger receives non-fatal warnings (corrupt file, pool backpressure).
	// nil = silent.
	Logger func(format string, args ...any)
}

// NewBackingStore returns the platform-appropriate Store. The body lives
// in store_js.go / store_notjs.go.
func NewBackingStore(opts Options) Store {
	return newBackingStore(opts)
}

// marshalPrefs serializes the unified prefs document with deterministic
// ordering for every section. Sorting keeps the on-disk diff stable
// across saves with the same content (important for git-tracked prefs
// dotfiles and snapshot tests).
func marshalPrefsV3(favs map[string]bool, overrides map[string]map[string]float64, userRecipes map[string][]byte, audioPanel AudioPanelStateDoc) ([]byte, error) {
	data, err := marshalPrefs(favs, overrides, userRecipes)
	if err != nil {
		return nil, err
	}
	if audioPanel.IsDefault() {
		return data, nil
	}
	// Re-marshal with audio_panel field set. Implementation choice:
	// parse the just-marshalled bytes back to a prefsDoc, set the
	// AudioPanel field, re-marshal. Avoids changing marshalPrefs's
	// signature and keeps the deterministic-ordering guarantee.
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return data, nil
	}
	cp := audioPanel
	doc.AudioPanel = &cp
	return json.MarshalIndent(doc, "", "  ")
}

func marshalPrefs(favs map[string]bool, overrides map[string]map[string]float64, userRecipes map[string][]byte) ([]byte, error) {
	ids := make([]string, 0, len(favs))
	for id, ok := range favs {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	var doc prefsDoc
	doc.Favorites = ids
	if len(overrides) > 0 {
		doc.RecipeOverrides = make(map[string]map[string]float64, len(overrides))
		for recipeID, params := range overrides {
			if len(params) == 0 {
				continue
			}
			cp := make(map[string]float64, len(params))
			for k, v := range params {
				cp[k] = v
			}
			doc.RecipeOverrides[recipeID] = cp
		}
	}
	if len(userRecipes) > 0 {
		doc.UserRecipes = make(map[string]json.RawMessage, len(userRecipes))
		for recipeID, raw := range userRecipes {
			if len(raw) == 0 {
				continue
			}
			cp := make(json.RawMessage, len(raw))
			copy(cp, raw)
			doc.UserRecipes[recipeID] = cp
		}
	}
	return json.MarshalIndent(doc, "", "  ")
}

// marshalPrefsV4 extends marshalPrefsV3 with the sample-edit section,
// using the same parse-back-and-set trick so the deterministic-ordering
// guarantee and the older sections' shapes stay untouched.
func marshalPrefsV4(favs map[string]bool, overrides map[string]map[string]float64, userRecipes map[string][]byte, audioPanel AudioPanelStateDoc, sampleEdits map[string]map[string]float64) ([]byte, error) {
	data, err := marshalPrefsV3(favs, overrides, userRecipes, audioPanel)
	if err != nil {
		return nil, err
	}
	if len(sampleEdits) == 0 {
		return data, nil
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return data, nil
	}
	doc.SampleEdits = make(map[string]map[string]float64, len(sampleEdits))
	for id, fields := range sampleEdits {
		if len(fields) == 0 {
			continue
		}
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
		doc.SampleEdits[id] = cp
	}
	return json.MarshalIndent(doc, "", "  ")
}

// parseSampleEdits extracts the sample-edit section from a prefs document,
// applying the same boundary sanitization as the recipe overrides. Empty /
// corrupt input returns a fresh empty map.
func parseSampleEdits(data []byte) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	if len(data) == 0 {
		return out
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return out
	}
	for id, fields := range doc.SampleEdits {
		if id == "" || len(fields) == 0 {
			continue
		}
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			if !isFiniteFloat(v) {
				continue
			}
			cp[k] = v
		}
		if len(cp) == 0 {
			continue
		}
		out[id] = cp
	}
	return out
}

// parsePrefs is the cross-platform load path. Returns fresh, non-nil
// cache state. Corrupt docs return empty state plus a logged warning so
// the caller is never blocked by bad on-disk state.
func parsePrefs(data []byte, logf func(string, ...any)) (map[string]bool, map[string]map[string]float64, map[string][]byte) {
	emptyFavs := map[string]bool{}
	emptyOverrides := map[string]map[string]float64{}
	emptyUserRecipes := map[string][]byte{}
	if len(data) == 0 {
		return emptyFavs, emptyOverrides, emptyUserRecipes
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		if logf != nil {
			logf("[USERPREFS] prefs: corrupted JSON, ignoring: %v", err)
		}
		return emptyFavs, emptyOverrides, emptyUserRecipes
	}
	favs := make(map[string]bool, len(doc.Favorites))
	for _, id := range doc.Favorites {
		if id == "" {
			continue
		}
		favs[id] = true
	}
	overrides := make(map[string]map[string]float64, len(doc.RecipeOverrides))
	for recipeID, params := range doc.RecipeOverrides {
		if recipeID == "" || len(params) == 0 {
			continue
		}
		cp := make(map[string]float64, len(params))
		for k, v := range params {
			// Non-finite values are silently dropped at the boundary —
			// the audio.SetInstrumentParam path applies the same rule;
			// keeping it here means a hand-edited prefs file can't poison
			// the registry with NaN/Inf.
			if !isFiniteFloat(v) {
				continue
			}
			cp[k] = v
		}
		if len(cp) == 0 {
			continue
		}
		overrides[recipeID] = cp
	}
	userRecipes := make(map[string][]byte, len(doc.UserRecipes))
	for recipeID, raw := range doc.UserRecipes {
		if recipeID == "" || len(raw) == 0 {
			continue
		}
		cp := make([]byte, len(raw))
		copy(cp, raw)
		userRecipes[recipeID] = cp
	}
	return favs, overrides, userRecipes
}

// parsePrefsV3 is the v3 extension of parsePrefs: in addition to the
// existing maps it returns the audio-panel section when present.
func parsePrefsV3(data []byte, logf func(string, ...any)) (map[string]bool, map[string]map[string]float64, map[string][]byte, AudioPanelStateDoc) {
	favs, overrides, userRecipes := parsePrefs(data, logf)
	if len(data) == 0 {
		return favs, overrides, userRecipes, AudioPanelStateDoc{}
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil || doc.AudioPanel == nil {
		return favs, overrides, userRecipes, AudioPanelStateDoc{}
	}
	return favs, overrides, userRecipes, *doc.AudioPanel
}

// isFiniteFloat is the boundary-sanitization predicate. Lives here so
// parsePrefs and SaveRecipeOverride apply the same rule. Avoids pulling
// math/Inf checks into call sites and matches the contract
// audio.sanitizeParamValue documents.
func isFiniteFloat(v float64) bool {
	// IEEE 754: NaN != NaN; ±Inf has absolute value larger than any
	// finite. The expression below is the standard idiom that doesn't
	// need a math import in a leaf package.
	if v != v {
		return false
	}
	if v > 1e308 || v < -1e308 {
		return false
	}
	return true
}
