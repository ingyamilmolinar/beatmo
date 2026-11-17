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

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
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

// prefsDoc is the on-the-wire shape. (The retired recipe_overrides /
// user_recipes sections of older files are ignored and dropped on the next
// write: instruments live in the instrument store as documents.)
//
// Every section is omitempty, so a document only grows once the user
// actually changes the corresponding setting. Adding a section is one
// field here plus one sanitizer in the prefsSnapshot⇄prefsDoc pair — there
// is deliberately no per-section marshal wrapper.
type prefsDoc struct {
	schema.Envelope
	Favorites []string `json:"favorites,omitempty"`
	// Phase 5 audio-panel redesign (userprefs v3): tab-specific
	// audio-panel state that should survive across sessions. omitempty
	// so v1/v2 prefs files don't grow until the user actually changes
	// any value; SaveAudioPanelState writes nil when the whole struct
	// is at defaults.
	AudioPanel *AudioPanelStateDoc `json:"audio_panel,omitempty"`
	// SampleEdits carries the non-destructive Sampler-edit descriptors:
	// instrument-id → field-name → value (bools as 0/1). Tiny flat maps,
	// so they live in prefs.json — NOT in the PCM
	// SampleStore. The audio package owns the SampleEdit↔map conversion.
	SampleEdits map[instrument.ID]map[string]float64 `json:"sample_edits,omitempty"`
	// Notifications carries the persisted user-notification history
	// (bounded, oldest..newest). omitempty so older prefs files don't grow
	// until the user actually raises a notification.
	Notifications []NotificationRecord `json:"notifications,omitempty"`
	// KnobSteps carries per-param-name chosen step-multiplier rungs for the
	// synth/sampler knob step badges. param-name → step value. omitempty so
	// older prefs files don't grow until the user actually changes any rung.
	KnobSteps map[string]float64 `json:"knob_steps,omitempty"`
	// Language carries the chosen UI locale (e.g. "es", "fr"). Empty =
	// default (English) upstream. omitempty so older prefs files don't grow
	// until the user actually picks a non-default language.
	Language string `json:"language,omitempty"`
	// Theme carries the per-user UI color scheme + the circuit-override
	// flag. omitempty so files don't grow until the user opens the Theme
	// editor.
	Theme *theme.UserDoc `json:"theme,omitempty"`
}

// notifHistoryMax bounds the persisted notification history (drop-oldest).
const notifHistoryMax = 100

// NotificationRecord is one persisted user notification (the on-the-wire
// shape; the UI maps its internal struct to/from this).
type NotificationRecord struct {
	Text   string   `json:"text"`
	IsErr  bool     `json:"is_err,omitempty"`
	UnixMs int64    `json:"unix_ms,omitempty"`
	Key    string   `json:"key,omitempty"`  // i18n key; when set the entry retranslates on load
	Args   []string `json:"args,omitempty"` // Tf args for Key
}

// NotificationHistoryStore persists the bounded notification history across
// sessions. Expose via type assertion on the Store returned by NewBackingStore.
type NotificationHistoryStore interface {
	LoadNotifications() ([]NotificationRecord, error)
	SaveNotifications(recs []NotificationRecord) error
}

// AudioPanelStateDoc mirrors the persisted shape of the audio-panel
// state. Exposed via LoadAudioPanelState / SaveAudioPanelState. The
// UI uses an internal struct that wraps this with sensible defaults
// when the field is nil — we pin the JSON shape here so future
// renames don't break older save files.
type AudioPanelStateDoc struct {
	// SynthGraphDetails: the Synth graph shows its NUMBERS readout instead of
	// the plain WORDS one (the graph's Details toggle).
	SynthGraphDetails bool `json:"synth_graph_details,omitempty"`
}

// IsDefault reports whether every field is at its zero value. The
// persistence layer uses this to skip serialising the section when
// no user override is active, keeping older prefs files small.
func (s AudioPanelStateDoc) IsDefault() bool {
	return !s.SynthGraphDetails
}

// Store persists a single set of favorited instrument IDs. Implementations
// must be safe for concurrent use from the UI goroutine (LoadFavorites,
// SaveFavorites) and from the background worker that drains the queue.
//
// SaveFavorites is non-blocking on the desktop backend (returns once the
// job is queued); WaitFlushed exists for tests and clean shutdown that
// need to know the disk is consistent.
//
// Both concrete backends (fileStore, localStorageStore) also implement the
// sibling section surfaces below — callers type-assert the return value from
// NewBackingStore.
type Store interface {
	LoadFavorites() (map[instrument.ID]bool, error)
	SaveFavorites(map[instrument.ID]bool) error
	WaitFlushed() error
	Close() error
}

// AudioPanelStore is the sibling surface for audio-panel UI state
// persistence (Phase 5 redesign). Implementations share the same
// concrete struct that satisfies Store; expose this
// view via type assertion. Save/Load are non-blocking on the
// desktop backend and synchronous in localStorage on browser builds.
type AudioPanelStore interface {
	LoadAudioPanelState() (AudioPanelStateDoc, error)
	SaveAudioPanelState(state AudioPanelStateDoc) error
}

// SampleBlob is one persisted Sampler-tab user sample: raw little-endian
// float32 PCM bytes plus the sample rate. The audio package owns the
// float32↔bytes conversion (keeping userprefs audio-agnostic).
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
	LoadSamples() (map[instrument.ID]SampleBlob, error)
	// SaveSample upserts the blob for id.
	SaveSample(id instrument.ID, blob SampleBlob) error
	// DeleteSample removes the persisted sample for id. No-op if absent.
	DeleteSample(id instrument.ID) error
}

// SampleEditStore is the sibling surface for the non-destructive sample-edit
// descriptors (Sampler-tab Save on a synth source): instrument-id keyed flat
// float maps inside the unified prefs document. Expose via type assertion on the Store
// returned by NewBackingStore.
type SampleEditStore interface {
	// LoadSampleEdits returns instrument-id → field → value for every
	// persisted descriptor. Empty map (never nil) when none exist.
	LoadSampleEdits() (map[instrument.ID]map[string]float64, error)
	// SaveSampleEdit upserts the descriptor fields for an instrument.
	// Nil or empty fields deletes the entry.
	SaveSampleEdit(instID instrument.ID, fields map[string]float64) error
	// DeleteSampleEdit removes the persisted descriptor. No-op if absent.
	DeleteSampleEdit(instID instrument.ID) error
}

// KnobStepStore is the sibling surface for per-param knob step-multiplier
// rung persistence (synth/sampler step badges). param-name keyed flat
// float64 map inside the unified prefs document. Expose via type assertion
// on the Store returned by NewBackingStore.
type KnobStepStore interface {
	// LoadKnobSteps returns param-name → step value for every persisted rung.
	// Empty map (never nil) when none exist.
	LoadKnobSteps() map[string]float64
	// SaveKnobStep upserts the chosen step for the named param.
	SaveKnobStep(name string, step float64) error
}

// ThemeStore persists the user's UI color scheme (internal/theme.UserDoc).
// Expose via type assertion on the Store returned by NewBackingStore.
type ThemeStore interface {
	LoadTheme() (theme.UserDoc, error)
	SaveTheme(d theme.UserDoc) error
}

// LanguageStore persists the chosen UI locale. Expose via type assertion on
// the Store returned by NewBackingStore.
type LanguageStore interface {
	LoadLanguage() (string, error)
	SaveLanguage(lang string) error
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

// prefsSnapshot is the in-memory form of a prefs document — one field per
// section, shared by both backends (fileStore and localStorageStore) and by
// both directions of serialization.
//
// It is the single currency of this package's IO: marshalPrefs takes one,
// parsePrefs returns one. Historically each section was threaded through a
// positional argument list that grew a new marshalPrefsV<N> wrapper every
// time a section was added; that chain re-marshalled the whole document
// once per section (6 marshals + 5 unmarshals for a full save) and had
// drifted into two different error policies. Adding a section is now one
// field here, one field on prefsDoc, and one sanitizer.
type prefsSnapshot struct {
	favorites     map[instrument.ID]bool
	audioPanel    AudioPanelStateDoc
	sampleEdits   map[instrument.ID]map[string]float64
	notifications []NotificationRecord
	knobSteps     map[string]float64
	language      string
	theme         theme.UserDoc
}

// emptyPrefsSnapshot returns fresh, non-nil, empty cache state — what a
// missing, corrupt, or version-rejected document degrades to.
func emptyPrefsSnapshot() prefsSnapshot {
	return prefsSnapshot{
		favorites:   map[instrument.ID]bool{},
		sampleEdits: map[instrument.ID]map[string]float64{},
		knobSteps:   map[string]float64{},
	}
}

// marshalPrefs serializes a snapshot to the on-the-wire document in a
// single pass.
//
// Ordering is deterministic — important for a git-trackable dotfile and for
// snapshot tests — but that falls out of the representation, not out of any
// sequencing: encoding/json emits struct fields in declaration order and
// sorts map keys itself. The one thing needing explicit care is the
// favorites slice, which sanitizeFavorites sorts.
func marshalPrefs(snap prefsSnapshot) ([]byte, error) {
	doc := prefsDoc{
		Envelope:      schema.Current(),
		Favorites:     sanitizeFavorites(snap.favorites),
		SampleEdits:   sanitizeNestedFloats(snap.sampleEdits),
		Notifications: boundNotifications(snap.notifications),
		KnobSteps:     sanitizeKnobSteps(snap.knobSteps),
		Language:      snap.language,
	}
	if !snap.audioPanel.IsDefault() {
		cp := snap.audioPanel
		doc.AudioPanel = &cp
	}
	if !snap.theme.IsEmpty() || snap.theme.OverrideCircuits {
		cp := snap.theme.Clone()
		doc.Theme = &cp
	}
	return json.MarshalIndent(doc, "", "  ")
}

// parsePrefs is the cross-platform load path, the exact inverse of
// marshalPrefs and likewise a single pass.
//
// Every returned map is fresh and non-nil so callers can assume a usable
// result. Three failure modes all degrade to empty-but-usable state with a
// logged warning, because prefs are loaded automatically at boot and a hard
// error would fail startup on a file this app itself wrote:
//
//   - empty input (no file yet)
//   - malformed JSON
//   - a schema_version this build cannot read
//
// A document with NO schema_version is a legacy pre-versioning file: it is
// accepted with its contents intact and gets stamped on the next write.
func parsePrefs(data []byte, logf func(string, ...any)) (prefsSnapshot, error) {
	empty := emptyPrefsSnapshot()
	if len(data) == 0 {
		return empty, nil
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		if logf != nil {
			logf("[userprefs] prefs: corrupted JSON, ignoring: %v", err)
		}
		return empty, nil
	}
	if err := doc.Validate("prefs"); err != nil {
		if logf != nil {
			logf("[userprefs] prefs: %v; starting from empty", err)
		}
		return empty, err
	}
	snap := prefsSnapshot{
		favorites:     parseFavorites(doc.Favorites),
		sampleEdits:   sanitizeNestedFloats(doc.SampleEdits),
		notifications: boundNotifications(doc.Notifications),
		knobSteps:     sanitizeKnobSteps(doc.KnobSteps),
		language:      doc.Language,
	}
	if doc.AudioPanel != nil {
		snap.audioPanel = *doc.AudioPanel
	}
	if doc.Theme != nil {
		snap.theme = doc.Theme.Clone()
	}
	// sanitize* return nil for an empty section (so the field is omitted on
	// write); the load path promises non-nil maps instead.
	if snap.sampleEdits == nil {
		snap.sampleEdits = map[instrument.ID]map[string]float64{}
	}
	if snap.knobSteps == nil {
		snap.knobSteps = map[string]float64{}
	}
	return snap, nil
}

// --- section sanitizers ---------------------------------------------------
//
// Each owns the boundary rules for one section and is shared by both
// directions, so a hand-edited prefs file cannot smuggle in a value that
// the writer would have rejected. They return nil for an empty section,
// which pairs with prefsDoc's omitempty tags.

// sanitizeFavorites drops empty ids and false entries, and sorts — the one
// piece of ordering that does not fall out of encoding/json.
func sanitizeFavorites(favs map[instrument.ID]bool) []string {
	ids := make([]string, 0, len(favs))
	for id, ok := range favs {
		if ok && id != "" {
			ids = append(ids, string(id))
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	return ids
}

// parseFavorites is the read-side inverse of sanitizeFavorites.
func parseFavorites(ids []string) map[instrument.ID]bool {
	favs := make(map[instrument.ID]bool, len(ids))
	for _, s := range ids {
		if id, err := instrument.ParseID(s); err == nil {
			favs[id] = true
		}
	}
	return favs
}

// sanitizeNestedFloats deep-copies an id → field → value section, dropping
// empty ids, empty groups, and non-finite values. Non-finite values are
// dropped at the boundary because audio.SetInstrumentParam applies the same
// rule — keeping it here means a hand-edited file cannot poison the
// registry with NaN/Inf. Used by SampleEdits.
func sanitizeNestedFloats(in map[instrument.ID]map[string]float64) map[instrument.ID]map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[instrument.ID]map[string]float64, len(in))
	for id, fields := range in {
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
		if len(cp) > 0 {
			out[id] = cp
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// sanitizeKnobSteps drops empty param names and non-finite step values.
func sanitizeKnobSteps(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for name, step := range in {
		if name == "" || !isFiniteFloat(step) {
			continue
		}
		out[name] = step
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// boundNotifications trims to the newest notifHistoryMax entries and drops
// empty records (no Text AND no translation Key — boundary sanitization).
func boundNotifications(recs []NotificationRecord) []NotificationRecord {
	out := make([]NotificationRecord, 0, len(recs))
	for _, r := range recs {
		if r.Text == "" && r.Key == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) > notifHistoryMax {
		out = out[len(out)-notifHistoryMax:]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isFiniteFloat is the boundary-sanitization predicate. Lives here so
// parsePrefs and SaveSampleEdit apply the same rule. Avoids pulling
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
