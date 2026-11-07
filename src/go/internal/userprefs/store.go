// Package userprefs persists per-user state outside the project file (favorites, language, theme, audio-panel state,
// notifications, knob steps, sample edits): localStorage key beatmo.prefs under js, else prefs.json under UserConfigDir,
// written atomically on the lazily acquired userprefs.persist pool. Missing or malformed data loads empty, never fails.
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

// A constant so tests and the CLAUDE.md standing-pools table name the same string.
const PoolName = "userprefs.persist"

// Every section is omitempty, so a document grows only once the user changes that setting. Adding a section is a field
// here plus a sanitizer, never a per-section marshal wrapper. Retired recipe sections are ignored and dropped on write.
type prefsDoc struct {
	schema.Envelope
	Favorites []string `json:"favorites,omitempty"`
	// Audio-panel state that survives sessions; nil when everything is at its default.
	AudioPanel *AudioPanelStateDoc `json:"audio_panel,omitempty"`
	// Instrument id → field → value (bools 0/1): tiny maps, so in prefs.json, not the PCM SampleStore. audio owns the
	// conversion.
	SampleEdits map[instrument.ID]map[string]float64 `json:"sample_edits,omitempty"`
	// Bounded, oldest to newest.
	Notifications []NotificationRecord `json:"notifications,omitempty"`
	// param name → step rung for the synth/sampler step badges.
	KnobSteps map[string]float64 `json:"knob_steps,omitempty"`
	// "" = English.
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

// The JSON shape is pinned here so a rename in the UI's wrapping struct never breaks older save files.
type AudioPanelStateDoc struct {
	// SynthGraphDetails: the Synth graph shows its NUMBERS readout instead of
	// the plain WORDS one (the graph's Details toggle).
	SynthGraphDetails bool `json:"synth_graph_details,omitempty"`
}

// An all-default section is not serialised at all.
func (s AudioPanelStateDoc) IsDefault() bool {
	return !s.SynthGraphDetails
}

// Safe from the UI goroutine and the background writer. SaveFavorites only queues on desktop; WaitFlushed is for tests
// and shutdown. Both backends implement the sibling section surfaces too, reached by type assertion.
type Store interface {
	LoadFavorites() (map[instrument.ID]bool, error)
	SaveFavorites(map[instrument.ID]bool) error
	WaitFlushed() error
	Close() error
}

// Non-blocking on desktop, synchronous localStorage in the browser.
type AudioPanelStore interface {
	LoadAudioPanelState() (AudioPanelStateDoc, error)
	SaveAudioPanelState(state AudioPanelStateDoc) error
}

// audio owns the float32 ↔ bytes conversion, keeping userprefs audio-agnostic.
type SampleBlob struct {
	SampleRate int
	PCM        []byte
}

// PCM is large, so OUTSIDE prefs.json / localStorage: one file per sample on desktop, IndexedDB in the browser.
type SampleStore interface {
	// LoadSamples returns id → blob for every persisted sample (never nil).
	LoadSamples() (map[instrument.ID]SampleBlob, error)
	// SaveSample upserts the blob for id.
	SaveSample(id instrument.ID, blob SampleBlob) error
	// DeleteSample removes the persisted sample for id. No-op if absent.
	DeleteSample(id instrument.ID) error
}

// Instrument-id keyed flat maps inside the prefs document.
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

// param-name keyed values inside the prefs document.
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
	// Empty = os.UserConfigDir/beatmo/prefs.json. Ignored on WASM.
	Path string

	// Tests pass their own, so they never share the long-lived production pool.
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

// One field per section, the single currency of this package's IO for both backends and both directions: a section
// is one field here, one on prefsDoc and one sanitizer, and the document is marshalled once.
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

// Deterministic (a dotfile people diff, snapshot tests) by representation: struct fields in order, map keys sorted by
// encoding/json; only the favorites slice needs sorting.
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
	if !snap.theme.Doc.IsEmpty() || snap.theme.OverrideCircuits {
		cp := snap.theme.Clone()
		doc.Theme = &cp
	}
	return json.MarshalIndent(doc, "", "  ")
}

// The inverse of marshalPrefs. Every map comes back fresh and non-nil. An empty input, malformed JSON or an unreadable
// schema_version (absent included) degrade to empty with a warning: prefs load at boot, and a hard error would fail
// startup on a file the app wrote.
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

// Shared by both directions, so a hand-edited file cannot smuggle in what the writer would reject; nil for an empty
// section, pairing with omitempty.

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

// Drops empty ids, empty groups and non-finite values at the boundary, as SetInstrumentParam does, so a hand-edited file
// cannot poison anything with NaN/Inf.
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

// One predicate for parsePrefs and SaveSampleEdit, matching audio's sanitizeParamValue.
func isFiniteFloat(v float64) bool {
	// NaN != NaN, and ±Inf exceeds every finite value: no math import in a leaf package.
	if v != v {
		return false
	}
	if v > 1e308 || v < -1e308 {
		return false
	}
	return true
}
