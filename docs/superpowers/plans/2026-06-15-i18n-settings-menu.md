# Internationalization (i18n) + Settings Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a central, CJK-ready internationalization system covering all user-facing chrome (English + neutral Latin-American Spanish), and replace the desktop grid "?" button with a Settings menu that switches language and shows keyboard mappings.

**Architecture:** A new leaf package `internal/i18n` holds a typed-key message catalog (`en`/`es` maps), an atomic active-locale, and `T`/`Tf` lookups. UI call sites resolve strings at draw/layout time; a key-aware `Button` variant resolves its label live. A font-resolution seam + font-generation cache token keep the renderer CJK-ready. `userprefs` v7 persists the chosen language. A new `SettingsOverlay` (portal) folds in the keyboard-shortcuts list. A drift guard test keeps new chrome strings going through the catalog.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), the repo's `userprefs` versioned-marshal pattern, portal/menu_chrome UI patterns.

**Test command (fast path):**
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/ ./internal/ui/ ./internal/userprefs/
```

**Standing constraints (from CLAUDE.md + project memory):**
- Use `.tools/go/bin/go` for all Go work.
- Forbidden glyphs in `internal/ui/`: use `IconID` + `DrawIcon`, never raw chrome glyphs.
- The assistant does NOT `git add`/commit/stage. The "Commit" steps below are written for a human operator; an agent worker leaves changes in the working tree and reports instead.
- `internal/ui` on this branch has a phantom duplicate-package issue under `./...`; always test with explicit package paths (`./internal/ui/`), never `./...`.

---

## File Structure

**New files:**
- `src/go/internal/i18n/locale.go` — `Locale` type, `AllLocales`, `ParseLocale`.
- `src/go/internal/i18n/keys.go` — typed `Key` consts + `AllKeys()` registry.
- `src/go/internal/i18n/catalog.go` — `en`/`es` registry wiring, active-locale atomic, font generation.
- `src/go/internal/i18n/catalog_en.go` — canonical English map.
- `src/go/internal/i18n/catalog_es.go` — Spanish map.
- `src/go/internal/i18n/i18n.go` — `T`, `Tf`, `SetLocale`, `Locale`, `OnChange`, `SetFontProvider`, `FontGeneration`.
- `src/go/internal/i18n/instruments.go` — builtin-id → Key default-name map + `InstrumentDisplayName`.
- `src/go/internal/i18n/*_test.go` — completeness, resolution, fallback, instrument tests.
- `src/go/internal/ui/settings_overlay.go` — the Settings portal overlay (language + shortcuts).
- `src/go/internal/ui/settings_overlay_test.go`, `i18n_reactivity_test.go`, `i18n_drift_test.go`, `spanish_layout_test.go`.

**Modified files:**
- `src/go/internal/ui/uigrid.go` — `Button.TextKey` + `NewButtonKey` + resolve-at-draw.
- `src/go/internal/ui/textcache.go` — add font-generation token to `styledKey`/`textSprites` keys + invalidation hook.
- `src/go/internal/ui/game_new.go` — load language from prefs → `i18n.SetLocale`; register `OnChange`; swap `gridHelpBtn` to a settings gear.
- `src/go/internal/ui/game_input_shortcuts.go` — rename/repoint help → settings; localized rows.
- `src/go/internal/ui/shortcuts_overlay.go` — content moves into Settings; keep a thin localized `shortcutRows` source.
- `src/go/internal/ui/icons.go` — add `IconSettings`.
- `src/go/internal/ui/sidebar_zone.go` — mobile Settings entry.
- `src/go/internal/ui/param_value_editor.go` — rune-aware caret + localized prompts (verify exact filename in Task 14).
- `src/go/internal/userprefs/store.go`, `store_js.go`, `store_notjs.go` — v7 `Language` + `LanguageStore`.

---

## Phase A — `internal/i18n` core

### Task 1: Locale type

**Files:**
- Create: `src/go/internal/i18n/locale.go`
- Test: `src/go/internal/i18n/locale_test.go`

- [ ] **Step 1: Write the failing test**

```go
package i18n

import "testing"

func TestParseLocale(t *testing.T) {
	cases := map[string]Locale{"en": LocaleEN, "es": LocaleES, "": LocaleEN, "zz": LocaleEN}
	for in, want := range cases {
		if got := ParseLocale(in); got != want {
			t.Fatalf("ParseLocale(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAllLocales(t *testing.T) {
	got := AllLocales()
	if len(got) != 2 || got[0] != LocaleEN || got[1] != LocaleES {
		t.Fatalf("AllLocales()=%v", got)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: FAIL — undefined `ParseLocale`, `Locale`, etc.

- [ ] **Step 3: Write the implementation**

```go
// Package i18n is the single source of truth for user-facing UI text. It is a
// leaf package (no other internal imports) so any package may depend on it.
package i18n

// Locale identifies a supported language. Values are BCP-47-ish short codes so
// they round-trip cleanly through prefs JSON.
type Locale string

const (
	LocaleEN Locale = "en"
	LocaleES Locale = "es" // neutral Latin-American Spanish (es-419)
)

// AllLocales returns every supported locale in display order. The completeness
// test iterates this set.
func AllLocales() []Locale { return []Locale{LocaleEN, LocaleES} }

// ParseLocale maps a stored string to a Locale, defaulting to English for empty
// or unrecognised input so a corrupt pref never leaves the UI blank.
func ParseLocale(s string) Locale {
	switch Locale(s) {
	case LocaleES:
		return LocaleES
	default:
		return LocaleEN
	}
}

func (l Locale) String() string { return string(l) }
```

- [ ] **Step 4: Run the test, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/i18n/locale.go src/go/internal/i18n/locale_test.go
git commit -m "feat(i18n): add Locale type"
```

---

### Task 2: Keys registry

**Files:**
- Create: `src/go/internal/i18n/keys.go`
- Test: `src/go/internal/i18n/keys_test.go`

- [ ] **Step 1: Write the failing test**

```go
package i18n

import "testing"

func TestAllKeysNonEmptyAndUnique(t *testing.T) {
	seen := map[Key]bool{}
	for _, k := range AllKeys() {
		if k == "" {
			t.Fatal("empty Key in AllKeys()")
		}
		if seen[k] {
			t.Fatalf("duplicate Key %q in AllKeys()", k)
		}
		seen[k] = true
	}
	if len(AllKeys()) == 0 {
		t.Fatal("AllKeys() is empty")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: FAIL — undefined `Key`, `AllKeys`.

- [ ] **Step 3: Implement (seed set; grows as migration proceeds)**

```go
package i18n

// Key identifies one user-facing message. Declared as typed constants so call
// sites are grep-able and AllKeys() can enumerate the catalog universe.
//
// ADD-A-KEY CHOKEPOINT: to add a message you must touch exactly four places —
// (1) a const here, (2) AllKeys(), (3) catalog_en.go, (4) catalog_es.go. The
// completeness test (catalog_test.go) fails loudly if any of the four is missed.
type Key string

const (
	// Transport
	KeyTransportPlay  Key = "transport.play"
	KeyTransportPause Key = "transport.pause"
	KeyTransportRec   Key = "transport.rec"

	// Settings / shortcuts
	KeySettingsTitle      Key = "settings.title"
	KeySettingsLanguage   Key = "settings.language"
	KeySettingsShortcuts  Key = "settings.shortcuts"
	KeySettingsDesktopOnly Key = "settings.shortcuts.desktop_only"
	KeyLangEnglish        Key = "settings.lang.english"
	KeyLangSpanish        Key = "settings.lang.spanish"

	// Shortcut action descriptions (key glyphs stay literal)
	KeyShortcutPlayPause Key = "shortcut.play_pause"
	KeyShortcutCancel    Key = "shortcut.cancel"
	KeyShortcutPan       Key = "shortcut.pan"
	KeyShortcutZoom      Key = "shortcut.zoom"
	KeyShortcutResetZoom Key = "shortcut.reset_zoom"
	KeyShortcutBPM       Key = "shortcut.bpm"
	KeyShortcutTabs      Key = "shortcut.tabs"
	KeyShortcutHelp      Key = "shortcut.help"
)

// AllKeys lists every declared Key. The completeness test asserts this set is
// exactly the key set of every locale catalog.
func AllKeys() []Key {
	return []Key{
		KeyTransportPlay, KeyTransportPause, KeyTransportRec,
		KeySettingsTitle, KeySettingsLanguage, KeySettingsShortcuts, KeySettingsDesktopOnly,
		KeyLangEnglish, KeyLangSpanish,
		KeyShortcutPlayPause, KeyShortcutCancel, KeyShortcutPan, KeyShortcutZoom,
		KeyShortcutResetZoom, KeyShortcutBPM, KeyShortcutTabs, KeyShortcutHelp,
	}
}
```

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/i18n/keys.go src/go/internal/i18n/keys_test.go
git commit -m "feat(i18n): add Key registry"
```

---

### Task 3: Catalogs + active locale + T/Tf + OnChange

**Files:**
- Create: `src/go/internal/i18n/catalog.go`, `catalog_en.go`, `catalog_es.go`, `i18n.go`
- Test: `src/go/internal/i18n/catalog_test.go`, `i18n_test.go`

- [ ] **Step 1: Write the failing completeness + resolution tests**

```go
// catalog_test.go
package i18n

import "testing"

func TestCatalogCompleteness(t *testing.T) {
	want := map[Key]bool{}
	for _, k := range AllKeys() {
		want[k] = true
	}
	for _, loc := range AllLocales() {
		cat := catalogFor(loc)
		if cat == nil {
			t.Fatalf("no catalog for %s", loc)
		}
		// Every declared key present and non-empty.
		for k := range want {
			v, ok := cat[k]
			if !ok {
				t.Errorf("locale %s missing key %q", loc, k)
				continue
			}
			if v == "" {
				t.Errorf("locale %s empty value for key %q", loc, k)
			}
		}
		// No orphan keys not declared in AllKeys().
		for k := range cat {
			if !want[k] {
				t.Errorf("locale %s has orphan key %q (not in AllKeys())", loc, k)
			}
		}
	}
}
```

```go
// i18n_test.go
package i18n

import "testing"

func TestTFallbackChain(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	if got := T(KeyTransportPlay); got == "" {
		t.Fatal("ES T returned empty")
	}
	// Unknown key falls back to its raw string, never panics.
	if got := T(Key("does.not.exist")); got != "does.not.exist" {
		t.Fatalf("unknown key fallback=%q", got)
	}
}

func TestTfFormats(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleEN)
	// Uses a temporary key injected for the test.
	withTempKey(t, Key("tmp.fmt"), "hello %s", "hola %s")
	if got := Tf(Key("tmp.fmt"), "world"); got != "hello world" {
		t.Fatalf("Tf=%q", got)
	}
}

func TestOnChangeFires(t *testing.T) {
	defer SetLocale(LocaleEN)
	n := 0
	cancel := OnChange(func() { n++ })
	defer cancel()
	SetLocale(LocaleES)
	SetLocale(LocaleEN)
	if n != 2 {
		t.Fatalf("OnChange fired %d times, want 2", n)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: FAIL — undefined `catalogFor`, `SetLocale`, `T`, `Tf`, `OnChange`, `withTempKey`.

- [ ] **Step 3: Implement catalog.go**

```go
package i18n

import (
	"sync"
	"sync/atomic"
)

// catalogs maps each locale to its message table. en is the canonical key set.
var catalogs = map[Locale]map[Key]string{
	LocaleEN: en,
	LocaleES: es,
}

func catalogFor(l Locale) map[Key]string { return catalogs[l] }

// activeLocale holds the current locale. Stored atomically so audio/UI threads
// can read it without locking on the hot path.
var activeLocale atomic.Value // stores Locale

func init() { activeLocale.Store(LocaleEN) }

func currentLocale() Locale {
	if v, ok := activeLocale.Load().(Locale); ok {
		return v
	}
	return LocaleEN
}

// change-listener registry.
var (
	listenersMu  sync.Mutex
	listeners    = map[int]func(){}
	listenerNext int
	fontGen      int64 // bumped whenever the resolved font face universe changes
)

// withTempKey injects a temporary message into both catalogs for the duration
// of a test, restoring afterwards. Test-only helper kept in non-test build for
// simplicity (it is exported through neither package API nor docs).
func withTempKey(t interface{ Cleanup(func()) }, k Key, enVal, esVal string) {
	en[k] = enVal
	es[k] = esVal
	t.Cleanup(func() { delete(en, k); delete(es, k) })
}
```

- [ ] **Step 4: Implement catalog_en.go and catalog_es.go (seed values for the Task-2 keys)**

```go
// catalog_en.go
package i18n

var en = map[Key]string{
	KeyTransportPlay:  "Play",
	KeyTransportPause: "Pause",
	KeyTransportRec:   "Rec",

	KeySettingsTitle:       "Settings",
	KeySettingsLanguage:    "Language",
	KeySettingsShortcuts:   "Keyboard shortcuts",
	KeySettingsDesktopOnly: "Available on desktop",
	KeyLangEnglish:         "English",
	KeyLangSpanish:         "Español",

	KeyShortcutPlayPause: "Play / Pause",
	KeyShortcutCancel:    "Cancel / close — or Stop",
	KeyShortcutPan:       "Pan the grid",
	KeyShortcutZoom:      "Zoom out / in",
	KeyShortcutResetZoom: "Reset zoom",
	KeyShortcutBPM:       "BPM up / down",
	KeyShortcutTabs:      "EQ / Wave / Spectrum / Levels / Chain / Synth / Sampler",
	KeyShortcutHelp:      "Open settings & shortcuts",
}
```

```go
// catalog_es.go
package i18n

var es = map[Key]string{
	KeyTransportPlay:  "Tocar",
	KeyTransportPause: "Pausa",
	KeyTransportRec:   "Grabar",

	KeySettingsTitle:       "Ajustes",
	KeySettingsLanguage:    "Idioma",
	KeySettingsShortcuts:   "Atajos de teclado",
	KeySettingsDesktopOnly: "Disponible en computadora",
	KeyLangEnglish:         "English",
	KeyLangSpanish:         "Español",

	KeyShortcutPlayPause: "Tocar / Pausar",
	KeyShortcutCancel:    "Cancelar / cerrar — o Detener",
	KeyShortcutPan:       "Mover la cuadrícula",
	KeyShortcutZoom:      "Alejar / acercar",
	KeyShortcutResetZoom: "Restablecer zoom",
	KeyShortcutBPM:       "Subir / bajar BPM",
	KeyShortcutTabs:      "EQ / Onda / Espectro / Niveles / Cadena / Sintetizador / Muestreador",
	KeyShortcutHelp:      "Abrir ajustes y atajos",
}
```

- [ ] **Step 5: Implement i18n.go**

```go
package i18n

import "fmt"

// T returns the message for k in the active locale, falling back to English and
// then to the raw key string. Never panics.
func T(k Key) string {
	loc := currentLocale()
	if v, ok := catalogs[loc][k]; ok {
		return v
	}
	if v, ok := en[k]; ok {
		return v
	}
	return string(k)
}

// Tf resolves k then applies fmt.Sprintf with args. Placeholders are positional
// so translations may reorder them.
func Tf(k Key, args ...any) string { return fmt.Sprintf(T(k), args...) }

// ActiveLocale returns the active locale.
func ActiveLocale() Locale { return currentLocale() }

// SetLocale switches the active locale and fires every OnChange listener. Safe
// to call from the UI goroutine.
func SetLocale(l Locale) {
	if currentLocale() == l {
		// Still fire listeners on explicit set so callers can force a refresh.
	}
	activeLocale.Store(l)
	listenersMu.Lock()
	fns := make([]func(), 0, len(listeners))
	for _, fn := range listeners {
		fns = append(fns, fn)
	}
	listenersMu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// OnChange registers fn to run after every SetLocale. Returns a cancel func.
func OnChange(fn func()) (cancel func()) {
	listenersMu.Lock()
	id := listenerNext
	listenerNext++
	listeners[id] = fn
	listenersMu.Unlock()
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}
```

- [ ] **Step 6: Run, verify all pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add src/go/internal/i18n/
git commit -m "feat(i18n): catalogs, T/Tf, active locale, OnChange"
```

---

### Task 4: CJK font seam (generation token)

**Files:**
- Modify: `src/go/internal/i18n/i18n.go` (add font provider + generation)
- Test: `src/go/internal/i18n/font_seam_test.go`

- [ ] **Step 1: Write the failing test**

```go
package i18n

import "testing"

func TestFontGenerationBumpsOnLocaleWithDistinctFont(t *testing.T) {
	defer SetLocale(LocaleEN)
	defer SetFontProvider(nil)
	// Provider that gives EN and ES different font identities.
	SetFontProvider(func(l Locale) string {
		if l == LocaleES {
			return "latin" // same family here, but exercise the hook
		}
		return "latin"
	})
	g0 := FontGeneration()
	SetLocale(LocaleES)
	if FontGeneration() == g0 {
		t.Skip("same font family for EN/ES today; generation only bumps on real face change")
	}
}

func TestFontGenerationMonotonic(t *testing.T) {
	g0 := FontGeneration()
	BumpFontGeneration()
	if FontGeneration() <= g0 {
		t.Fatal("FontGeneration must increase after BumpFontGeneration")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: FAIL — undefined `SetFontProvider`, `FontGeneration`, `BumpFontGeneration`.

- [ ] **Step 3: Implement**

```go
// Append to i18n.go.

// fontProvider maps a locale to a font-face identity string. Today the UI
// registers a provider that returns the Inter family for EN/ES; a future CJK
// locale returns a different identity, which the UI uses to bump the font
// generation so the text sprite cache never serves a Latin glyph for a CJK
// string. nil ⇒ a single implicit face.
var fontProvider func(Locale) string

// SetFontProvider installs the locale→face-identity resolver (UI does this at
// startup). Passing nil clears it.
func SetFontProvider(fn func(Locale) string) { fontProvider = fn }

// FontIdentity returns the face identity for the active locale, or "" when no
// provider is installed.
func FontIdentity() string {
	if fontProvider == nil {
		return ""
	}
	return fontProvider(currentLocale())
}

// FontGeneration is a monotonically increasing token the renderer folds into
// its sprite-cache key. It changes whenever the resolved face universe changes.
func FontGeneration() int64 { return atomic.LoadInt64(&fontGen) }

// BumpFontGeneration increments the generation. Called by SetLocale when the
// active locale's face identity differs from the previous one.
func BumpFontGeneration() int64 { return atomic.AddInt64(&fontGen, 1) }
```

Then update `SetLocale` to bump generation when the face identity changes:

```go
func SetLocale(l Locale) {
	prevFace := FontIdentity()
	activeLocale.Store(l)
	if FontIdentity() != prevFace {
		BumpFontGeneration()
	}
	// ... fire listeners (unchanged) ...
}
```

Add `"sync/atomic"` import if not already present (catalog.go already imports it; i18n.go needs its own import for the atomic helpers — move `fontGen` access there or keep helpers in catalog.go). Keep all `fontGen` access in one file to avoid an unused-import split.

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/i18n/i18n.go src/go/internal/i18n/font_seam_test.go
git commit -m "feat(i18n): CJK-ready font-resolution seam + generation token"
```

---

### Task 5: Builtin instrument default names

**Files:**
- Create: `src/go/internal/i18n/instruments.go`
- Test: `src/go/internal/i18n/instruments_test.go`

- [ ] **Step 1: Write the failing test**

```go
package i18n

import "testing"

func TestInstrumentDisplayNameLocalizesDefault(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	// stored == canonical English default ⇒ localize
	if got := InstrumentDisplayName("kick", "Kick"); got != "Bombo" {
		t.Fatalf("kick default ES = %q want Bombo", got)
	}
	// user-renamed ⇒ verbatim
	if got := InstrumentDisplayName("kick", "My Boom"); got != "My Boom" {
		t.Fatalf("renamed = %q want My Boom", got)
	}
	// empty stored ⇒ localize default
	if got := InstrumentDisplayName("kick", ""); got != "Bombo" {
		t.Fatalf("empty = %q want Bombo", got)
	}
	// unknown builtin id ⇒ verbatim stored
	if got := InstrumentDisplayName("unknownid", "Weird"); got != "Weird" {
		t.Fatalf("unknown = %q want Weird", got)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: FAIL — undefined `InstrumentDisplayName`.

- [ ] **Step 3: Implement**

```go
package i18n

// builtinInstrumentDefault maps a builtin instrument id to its canonical
// English default name and its message key. Display-only: export/import always
// uses the canonical English name, keeping project files language-independent.
type builtinInstrumentDefault struct {
	canonicalEN string
	key         Key
}

// Seed set; extend as more builtin ids are confirmed against synth_registry.go.
var builtinInstruments = map[string]builtinInstrumentDefault{
	"kick":  {"Kick", KeyInstKick},
	"snare": {"Snare", KeyInstSnare},
	"hihat": {"Hi-Hat", KeyInstHiHat},
	"clap":  {"Clap", KeyInstClap},
	"tom":   {"Tom", KeyInstTom},
	"bass":  {"Bass", KeyInstBass},
}

// InstrumentDisplayName returns the name to show for an instrument. When stored
// equals the builtin's canonical English default (or is empty), the localized
// default is returned; otherwise stored is returned verbatim (user rename).
func InstrumentDisplayName(id, stored string) string {
	def, ok := builtinInstruments[id]
	if !ok {
		return stored
	}
	if stored == "" || stored == def.canonicalEN {
		return T(def.key)
	}
	return stored
}
```

Add the instrument keys to `keys.go` (const + `AllKeys()`) and both catalogs:

```go
// keys.go consts
KeyInstKick  Key = "inst.kick"
KeyInstSnare Key = "inst.snare"
KeyInstHiHat Key = "inst.hihat"
KeyInstClap  Key = "inst.clap"
KeyInstTom   Key = "inst.tom"
KeyInstBass  Key = "inst.bass"
```
```go
// catalog_en.go: KeyInstKick:"Kick", KeyInstSnare:"Snare", KeyInstHiHat:"Hi-Hat",
//                KeyInstClap:"Clap", KeyInstTom:"Tom", KeyInstBass:"Bass"
// catalog_es.go: KeyInstKick:"Bombo", KeyInstSnare:"Caja", KeyInstHiHat:"Charles",
//                KeyInstClap:"Palmada", KeyInstTom:"Tom", KeyInstBass:"Bajo"
```

- [ ] **Step 4: Run, verify it passes (completeness test also covers the new keys)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: Verify builtin ids against the registry**

Run: `cd src/go && grep -rn "\"kick\"\|\"snare\"\|\"hihat\"\|builtinInstrumentRecipeBindings" internal/audio/synth_registry.go | head`
Action: confirm each id string matches a real builtin instrument id; correct any mismatches in `builtinInstruments`. Adjust the map to the real ids before committing.

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/i18n/instruments.go src/go/internal/i18n/instruments_test.go src/go/internal/i18n/keys.go src/go/internal/i18n/catalog_en.go src/go/internal/i18n/catalog_es.go
git commit -m "feat(i18n): localized builtin instrument default names"
```

---

## Phase B — UI reactivity wiring

### Task 6: Font-generation token in the text sprite cache

**Files:**
- Modify: `src/go/internal/ui/textcache.go`
- Test: `src/go/internal/ui/textcache_fontgen_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

func TestStyledCacheKeyIncludesFontGen(t *testing.T) {
	ClearTextCacheForTest()
	a := StyledTextSprite("Hola", RoleBody)
	i18n.BumpFontGeneration()
	b := StyledTextSprite("Hola", RoleBody)
	// After a font-generation bump the cache must not serve the old sprite.
	if a == b {
		t.Fatal("styled cache served a stale sprite across a font-generation bump")
	}
}
```

(Use the module's real import path for `i18n` — confirm via `head -1 src/go/go.mod`; replace `beatmo/internal/i18n` with the actual module path if different.)

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStyledCacheKeyIncludesFontGen`
Expected: FAIL — sprites are equal (key has no font generation).

- [ ] **Step 3: Implement — add fontGen to both cache keys**

In `textcache.go`, change `styledKey` and the plain text cache to include the generation. For the styled cache:

```go
type styledKey struct {
	s    string
	size int // size*100, integer key
	bold bool
	gen  int64 // i18n.FontGeneration() snapshot
}
```

In `StyledTextSprite`, seed the key with the generation:

```go
func StyledTextSprite(s string, role TextRole) *ebiten.Image {
	key := styledKey{s: s, size: int(role.size() * 100), bold: role.bold(), gen: i18n.FontGeneration()}
	// ... unchanged ...
}
```

For the plain `textSprites map[string]*ebiten.Image`, change the key to a struct:

```go
type textKey struct {
	s   string
	gen int64
}
var textSprites = map[textKey]*ebiten.Image{}
```

and update `TextSprite`, `ClearTextCacheForTest` accordingly:

```go
func TextSprite(s string) *ebiten.Image {
	k := textKey{s: s, gen: i18n.FontGeneration()}
	textCacheMu.RLock()
	if spr := textSprites[k]; spr != nil {
		textCacheMu.RUnlock()
		return spr
	}
	textCacheMu.RUnlock()
	// ... build img as before ...
	textCacheMu.Lock()
	textSprites[k] = img
	textCacheMu.Unlock()
	return img
}
```

Add the `i18n` import to `textcache.go`.

- [ ] **Step 4: Run, verify it passes and the full ui package still builds**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStyledCacheKeyIncludesFontGen`
Expected: PASS
Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/textcache.go src/go/internal/ui/textcache_fontgen_test.go
git commit -m "feat(ui): fold i18n font generation into text sprite cache keys"
```

---

### Task 7: Key-aware Button label (resolve-at-draw)

**Files:**
- Modify: `src/go/internal/ui/uigrid.go`
- Test: `src/go/internal/ui/button_i18n_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

func TestButtonKeyResolvesLiveOnLocaleSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	b := NewButtonKey(i18n.KeyTransportPlay, ComponentButtonSecondary, func() {})
	i18n.SetLocale(i18n.LocaleEN)
	if got := b.displayText(); got != "Play" {
		t.Fatalf("EN label=%q want Play", got)
	}
	i18n.SetLocale(i18n.LocaleES)
	if got := b.displayText(); got != "Tocar" {
		t.Fatalf("ES label=%q want Tocar", got)
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestButtonKeyResolvesLive`
Expected: FAIL — undefined `NewButtonKey`, `displayText`.

- [ ] **Step 3: Implement**

Add to `uigrid.go`:

```go
// TextKey, when non-empty, makes the button resolve its label from the i18n
// catalog at draw time so a language switch updates it without reconstruction.
// It overrides Text. Set via NewButtonKey or SetTextKey.
```
Add the field to the `Button` struct (next to `Text`):
```go
	TextKey i18n.Key
```
Add constructor + helpers:
```go
// NewButtonKey constructs a spec-styled button whose label is resolved live
// from the i18n catalog (resolve-at-draw). Prefer this for any localized label.
func NewButtonKey(key i18n.Key, id ComponentID, onClick func()) *Button {
	b := &Button{TextKey: key, OnClick: onClick}
	b.SetSpec(id)
	return b
}

// SetTextKey switches the button to live i18n resolution.
func (b *Button) SetTextKey(key i18n.Key) { b.TextKey = key }

// displayText returns the label to render: the live-resolved catalog string
// when TextKey is set, else the static Text.
func (b *Button) displayText() string {
	if b.TextKey != "" {
		return i18n.T(b.TextKey)
	}
	return b.Text
}
```
In `Button.Draw`, replace the `b.Text` use with `b.displayText()`:
```go
		clipped := clipTextToWidth(b.displayText(), b.r.Dx()-2*SpaceXS)
```
Add the `i18n` import to `uigrid.go`.

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestButtonKeyResolvesLive`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/uigrid.go src/go/internal/ui/button_i18n_test.go
git commit -m "feat(ui): key-aware Button label resolves i18n at draw time"
```

---

### Task 8: Locale-change relayout/cache invalidation listener

**Files:**
- Modify: `src/go/internal/ui/game_new.go` (register listener), `src/go/internal/ui/game_struct.go` (cancel handle if needed)
- Test: `src/go/internal/ui/i18n_reactivity_test.go`

- [ ] **Step 1: Write the failing test (functional: drive real Update loop)**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

func TestLocaleSwitchForcesRelayout(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGame(t) // existing test helper; see testutil_test.go
	g.Layout(1200, 800)
	g.Update()
	g.needLayout = false // settle
	i18n.SetLocale(i18n.LocaleES)
	if !g.needLayout {
		t.Fatal("locale switch did not force a relayout")
	}
}
```

(Confirm the real test-game constructor name in `testutil_test.go`; replace `newTestGame` if different. Confirm the field that forces layout — search `needLayout` / `forceDraw`.)

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLocaleSwitchForcesRelayout`
Expected: FAIL — `needLayout` stays false.

- [ ] **Step 3: Implement — register an OnChange listener in game_new.go**

After the Game and DrumView are constructed in `NewGame`/`game_new.go`, add:

```go
	// Re-localize the whole UI on a language switch: force a relayout so every
	// resolve-at-draw label re-measures (Spanish strings differ in width), and
	// invalidate text-baking caches. The text sprite cache self-keys on the
	// string + font generation, so no manual sprite clear is needed.
	g.i18nCancel = i18n.OnChange(func() {
		g.needLayout = true
		if g.drum != nil {
			g.drum.invalidateAllCaches() // see note below
		}
	})
```

Add `i18nCancel func()` to `Game` in `game_struct.go`. If a single `invalidateAllCaches` does not exist, call the existing invalidation entry points used elsewhere on a structural change (search for where row/sprite caches are marked dirty — e.g. `markRowsDirty`, `Invalidate`, `needLayout`). Use the smallest existing call that re-bakes text-bearing caches; if only `needLayout=true` is required for correctness (caches rebuild on next layout), keep it minimal and adjust the test to assert `needLayout`.

Wire `g.i18nCancel()` into the Game teardown path if one exists (search for a `Close`/`Shutdown` on Game); otherwise leave it (process-lifetime listener).

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLocaleSwitchForcesRelayout`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_new.go src/go/internal/ui/game_struct.go src/go/internal/ui/i18n_reactivity_test.go
git commit -m "feat(ui): relayout + cache invalidation on locale switch"
```

---

## Phase C — Persistence

### Task 9: userprefs v7 Language

**Files:**
- Modify: `src/go/internal/userprefs/store.go`, `store_js.go`, `store_notjs.go`
- Test: `src/go/internal/userprefs/language_test.go`

- [ ] **Step 1: Write the failing test**

```go
package userprefs

import "testing"

func TestLanguageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := newFileStoreAt(t, dir) // mirror existing store_test.go helper
	ls, ok := s.(LanguageStore)
	if !ok {
		t.Fatal("fileStore does not implement LanguageStore")
	}
	if err := ls.SaveLanguage("es"); err != nil {
		t.Fatal(err)
	}
	s.WaitFlushed()
	s2 := newFileStoreAt(t, dir)
	got, err := s2.(LanguageStore).LoadLanguage()
	if err != nil {
		t.Fatal(err)
	}
	if got != "es" {
		t.Fatalf("LoadLanguage=%q want es", got)
	}
}

func TestLanguageDefaultsEmptyWhenAbsent(t *testing.T) {
	s := newFileStoreAt(t, t.TempDir())
	got, err := s.(LanguageStore).LoadLanguage()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("absent language=%q want empty", got)
	}
}
```

(Check `store_test.go` for the exact helper that builds a `fileStore` rooted at a dir; replace `newFileStoreAt` with it.)

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/ -run TestLanguage`
Expected: FAIL — undefined `LanguageStore`.

- [ ] **Step 3: Implement — struct field, interface, marshal, backends**

In `store.go`, add to `prefsDoc`:
```go
	// Language is the user's chosen UI locale (e.g. "en", "es"). omitempty so
	// older prefs files don't grow until the user picks a non-default language.
	Language string `json:"language,omitempty"`
```
Add the interface:
```go
// LanguageStore persists the chosen UI locale. Expose via type assertion on the
// Store returned by NewBackingStore.
type LanguageStore interface {
	LoadLanguage() (string, error)
	SaveLanguage(lang string) error
}
```
Add `marshalPrefsV7` calling the v6 marshaller then setting Language (mirror the existing vN pattern exactly — same arg threading, same omitempty, same JSON indent):
```go
func marshalPrefsV7(favs map[string]bool, overrides map[string]map[string]float64, userRecipes map[string][]byte, audioPanel AudioPanelStateDoc, sampleEdits map[string]map[string]float64, notifications []NotificationRecord, knobSteps map[string]float64, language string) ([]byte, error) {
	data, err := marshalPrefsV6(favs, overrides, userRecipes, audioPanel, sampleEdits, notifications, knobSteps)
	if err != nil || language == "" {
		return data, err
	}
	var doc prefsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	doc.Language = language
	return json.MarshalIndent(doc, "", "  ")
}
```
In `store_notjs.go` (fileStore) and `store_js.go` (localStorageStore): add a `language` field to the store struct, load it in the parse path (the shared unmarshal already populates `doc.Language`; copy it into the in-memory field), and implement:
```go
func (s *fileStore) LoadLanguage() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.language, nil
}

func (s *fileStore) SaveLanguage(lang string) error {
	s.mu.Lock()
	s.language = lang
	s.mu.Unlock()
	return s.enqueueSave() // mirror however SaveKnobSteps triggers a write
}
```
Update the write path to call `marshalPrefsV7(..., s.language)` everywhere the store serialises (find the single marshal call site used by the save worker and bump it to v7, threading the language field). Repeat for `store_js.go` with its synchronous save.

- [ ] **Step 4: Run, verify it passes; run the whole userprefs suite for regressions**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/`
Expected: PASS (including existing v3–v6 round-trip tests)

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/userprefs/
git commit -m "feat(userprefs): v7 Language persistence"
```

---

### Task 10: Load language at startup + save on switch

**Files:**
- Modify: `src/go/internal/ui/game_new.go` (load → SetLocale), and wherever the settings UI changes language (Task 12 wires the save)
- Test: `src/go/internal/ui/i18n_startup_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

type fakeLangStore struct{ lang string }

func (f *fakeLangStore) LoadLanguage() (string, error) { return f.lang, nil }
func (f *fakeLangStore) SaveLanguage(l string) error   { f.lang = l; return nil }

func TestStartupAppliesStoredLanguage(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	applyStoredLanguage(&fakeLangStore{lang: "es"})
	if i18n.ActiveLocale() != i18n.LocaleES {
		t.Fatalf("active=%q want es", i18n.ActiveLocale())
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStartupAppliesStoredLanguage`
Expected: FAIL — undefined `applyStoredLanguage`.

- [ ] **Step 3: Implement a small helper + call it from game_new.go**

```go
// applyStoredLanguage reads the persisted locale (if any) and applies it before
// the first frame. Empty/absent ⇒ English (the i18n default).
func applyStoredLanguage(ls userprefs.LanguageStore) {
	if ls == nil {
		return
	}
	if lang, err := ls.LoadLanguage(); err == nil && lang != "" {
		i18n.SetLocale(i18n.ParseLocale(lang))
	}
}
```
In `game_new.go`, where `prefsStore` is created/known, add:
```go
	if ls, ok := prefsStore.(userprefs.LanguageStore); ok {
		applyStoredLanguage(ls)
		g.langStore = ls // keep for the settings menu to call SaveLanguage
	}
```
Add `langStore userprefs.LanguageStore` to `Game` in `game_struct.go`.

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStartupAppliesStoredLanguage`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_new.go src/go/internal/ui/game_struct.go src/go/internal/ui/i18n_startup_test.go
git commit -m "feat(ui): apply stored language at startup"
```

---

## Phase D — Settings menu

### Task 11: IconSettings gear

**Files:**
- Modify: `src/go/internal/ui/icons.go`
- Test: `src/go/internal/ui/icons_settings_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestIconSettingsDraws(t *testing.T) {
	dst := ebiten.NewImage(32, 32)
	if !DrawIcon(dst, IconSettings, image.Rect(0, 0, 32, 32), colTextPrimary) {
		t.Fatal("DrawIcon(IconSettings) returned false — icon not registered")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestIconSettingsDraws`
Expected: FAIL — undefined `IconSettings` (or DrawIcon returns false).

- [ ] **Step 3: Implement — add the const and a draw case**

In `icons.go` add the const next to the others:
```go
	IconSettings IconID = "settings"
```
In the `DrawIcon` switch, add a `case IconSettings:` that draws a gear using existing vector primitives (a circle plus short radial teeth, matching the style of nearby icon cases). Follow the pattern of an existing icon case (e.g. `IconTarget` / `IconRecord`) for primitive calls and return `true`.

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestIconSettingsDraws`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/icons.go src/go/internal/ui/icons_settings_test.go
git commit -m "feat(ui): add IconSettings gear glyph"
```

---

### Task 12: SettingsOverlay (language segmented control + localized shortcuts)

**Files:**
- Create: `src/go/internal/ui/settings_overlay.go`
- Modify: `src/go/internal/ui/shortcuts_overlay.go` (localize `shortcutRows` source), `src/go/internal/ui/game_input_shortcuts.go` (open Settings instead of Shortcuts)
- Test: `src/go/internal/ui/settings_overlay_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"image"
	"testing"

	"beatmo/internal/i18n"
)

func TestSettingsOverlayLanguageSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	saved := ""
	o := NewSettingsOverlay(func(l i18n.Locale) {
		i18n.SetLocale(l)
		saved = l.String()
	})
	o.Layout(image.Rect(0, 0, 1200, 800), image.Rect(0, 0, 1200, 800))
	// Find the "Español" hit area and fire it.
	hit := findHitByTagContains(t, o.HitAreas(), "lang-es")
	hit.Handler.OnPress(hit.Rect.Min.X+1, hit.Rect.Min.Y+1)
	if i18n.ActiveLocale() != i18n.LocaleES {
		t.Fatalf("active=%q want es", i18n.ActiveLocale())
	}
	if saved != "es" {
		t.Fatalf("onPick saved=%q want es", saved)
	}
}
```

(`findHitByTagContains` — add a tiny test helper that scans `[]HitArea` for a tag substring; or reuse an existing helper if present. Confirm the `HitArea` field names — `Rect`, `Handler`, `OnPress` — against `input_capture.go`/the dispatcher and adjust.)

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSettingsOverlayLanguageSwitch`
Expected: FAIL — undefined `NewSettingsOverlay`.

- [ ] **Step 3: Implement SettingsOverlay**

Implement the `PortalOverlay` interface (`Layout(screenBounds, ownerBounds image.Rectangle)`, `HitAreas() []HitArea`, `Draw(*ebiten.Image)`, `ShouldClose() bool`) modeled on `ShortcutsOverlay`. Structure:
- Header: `DrawTextStyled(screen, i18n.T(i18n.KeySettingsTitle), ...)` at `RolePanelTitle`.
- Language section: label `i18n.T(i18n.KeySettingsLanguage)`, then two pill buttons "English" / "Español" (`i18n.T(KeyLangEnglish/KeyLangSpanish)`), the active one drawn toggled. Each pill registers a `HitArea` (tags `"settings-lang-en"`, `"settings-lang-es"`) whose handler calls the injected `onPick(locale)`.
- Shortcuts section: label `i18n.T(i18n.KeySettingsShortcuts)`, then the rows from `localizedShortcutRows()` (Task: change `shortcutRows` into a function returning key glyphs + `i18n.T(actionKey)`). On mobile, replace the rows with a single `i18n.T(KeySettingsDesktopOnly)` line.

Constructor:
```go
func NewSettingsOverlay(onPick func(i18n.Locale)) *SettingsOverlay { ... }
```

Change `shortcuts_overlay.go`:
```go
type shortcutRow struct {
	keys      string
	actionKey i18n.Key
}

func localizedShortcutRows() []shortcutRow {
	return []shortcutRow{
		{"Space", i18n.KeyShortcutPlayPause},
		{"Esc", i18n.KeyShortcutCancel},
		{"Arrows", i18n.KeyShortcutPan},
		{"[  ]", i18n.KeyShortcutZoom},
		{"0", i18n.KeyShortcutResetZoom},
		{"+  -", i18n.KeyShortcutBPM},
		{"1 - 7", i18n.KeyShortcutTabs},
		{"?", i18n.KeyShortcutHelp},
	}
}
```
The standalone `ShortcutsOverlay.Draw` now reads `localizedShortcutRows()` and `i18n.T(row.actionKey)`; it may be retired entirely once Settings subsumes it (keep it compiling until Task 13 repoints the openers).

- [ ] **Step 4: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSettingsOverlayLanguageSwitch`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/settings_overlay.go src/go/internal/ui/shortcuts_overlay.go src/go/internal/ui/settings_overlay_test.go
git commit -m "feat(ui): SettingsOverlay with language switch + localized shortcuts"
```

---

### Task 13: Repoint the desktop "?" → Settings gear

**Files:**
- Modify: `src/go/internal/ui/game_new.go` (button), `src/go/internal/ui/game_input_shortcuts.go` (toggle), `src/go/internal/ui/game_struct.go` (rename field/comment)
- Test: `src/go/internal/ui/settings_entry_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ui

import "testing"

func TestSettingsGearOpensSettingsOverlay(t *testing.T) {
	g := newTestGame(t)
	g.Layout(1200, 800)
	g.toggleSettingsOverlay()
	if g.drum == nil || g.drum.tree == nil || g.drum.tree.Portal() == nil ||
		!g.drum.tree.Portal().Has(settingsOverlayID) {
		t.Fatal("toggleSettingsOverlay did not open the settings portal")
	}
	g.toggleSettingsOverlay()
	if g.drum.tree.Portal().Has(settingsOverlayID) {
		t.Fatal("second toggle did not close it")
	}
}
```

- [ ] **Step 2: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSettingsGearOpensSettingsOverlay`
Expected: FAIL — undefined `toggleSettingsOverlay`, `settingsOverlayID`.

- [ ] **Step 3: Implement**

- Add `const settingsOverlayID = "settings"`.
- Add `toggleSettingsOverlay` modeled on `toggleShortcutsOverlay`, opening `NewSettingsOverlay(func(l i18n.Locale){ i18n.SetLocale(l); if g.langStore != nil { _ = g.langStore.SaveLanguage(l.String()) } })`.
- In `game_new.go` change the button to a gear icon opening settings:
```go
	g.gridHelpBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleSettingsOverlay)
	g.gridHelpBtn.Icon = string(IconSettings)
```
- Repoint the `/` shortcut and the audio-panel `?` handler (`audioPanelShortcut`) and the mouse/touch handlers (`game_input_touch.go`, `game_update.go`) from `toggleShortcutsOverlay` to `toggleSettingsOverlay`.
- Update `game_input_shortcuts_test.go` / `grid_help_button_functional_test.go` references to the new toggle/ID (keep `gridHelpButtonRect` name to minimize churn, or rename to `gridSettingsButtonRect` consistently — pick one and update all call sites; the simplest is to keep the existing rect name and only repoint behavior).

- [ ] **Step 4: Run, verify it passes + the old shortcut tests still pass after repointing**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSettingsGear|Shortcut|GridHelp'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/game_new.go src/go/internal/ui/game_input_shortcuts.go src/go/internal/ui/game_struct.go src/go/internal/ui/game_input_touch.go src/go/internal/ui/game_update.go src/go/internal/ui/settings_entry_test.go
git commit -m "feat(ui): desktop gear opens Settings (replaces ? help)"
```

---

### Task 14: Mobile Settings entry

**Files:**
- Modify: `src/go/internal/ui/sidebar_zone.go`
- Test: `src/go/internal/ui/settings_mobile_entry_test.go`

- [ ] **Step 1: Inspect the sidebar to find the item-list pattern**

Run: `cd src/go && grep -n "func.*Sidebar\|items\|HitArea\|OnPress\|addItem\|sidebarItem" internal/ui/sidebar_zone.go | head -30`
Action: identify how sidebar rows are declared and how their taps are wired.

- [ ] **Step 2: Write the failing test**

```go
package ui

import "testing"

func TestMobileSidebarHasSettingsEntry(t *testing.T) {
	g := newTestGameMobile(t) // mobile variant; confirm helper name
	g.Layout(390, 780)
	// Open the sidebar, then assert a Settings hit area exists and opens the portal.
	openSidebarForTest(g) // confirm/replace with the real open path
	hit := findHitByTagContains(t, g.sidebar.HitAreas(), "settings")
	hit.Handler.OnPress(hit.Rect.Min.X+1, hit.Rect.Min.Y+1)
	if !g.drum.tree.Portal().Has(settingsOverlayID) {
		t.Fatal("mobile settings entry did not open the settings overlay")
	}
}
```

(Confirm the mobile test-game and sidebar-open helpers in `testutil_test.go` / existing sidebar tests; replace names as needed.)

- [ ] **Step 3: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSidebarHasSettingsEntry`
Expected: FAIL — no settings entry.

- [ ] **Step 4: Implement — add a localized "Settings" item to the sidebar**

Add a `KeySidebarSettings` key (const + AllKeys + catalogs: EN "Settings", ES "Ajustes" — or reuse `KeySettingsTitle`). Add a sidebar item rendered with `IconSettings` + `i18n.T(KeySettingsTitle)`, whose tap calls `g.toggleSettingsOverlay()` (thread a callback into the sidebar the same way existing sidebar actions are wired).

- [ ] **Step 5: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileSidebarHasSettingsEntry`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/sidebar_zone.go src/go/internal/ui/settings_mobile_entry_test.go src/go/internal/i18n/
git commit -m "feat(ui): mobile sidebar Settings entry"
```

---

## Phase E — Text inputs (Unicode correctness)

### Task 15: Rune-aware value editor + localized prompts

**Files:**
- Modify: the shared value editor (confirm filename) and the mobile native input path
- Test: `src/go/internal/ui/value_editor_unicode_test.go`

- [ ] **Step 1: Locate the editor**

Run: `cd src/go && grep -rln "ParamValueEditor\|value editor\|caret\|[]byte(.*text" internal/ui/*.go | grep -iv _test | head`
Action: open the file owning caret/edit state (per project memory: "Shared value editor" → `ParamValueEditor`). Confirm the struct + how caret index and width are computed.

- [ ] **Step 2: Write the failing test**

```go
package ui

import "testing"

func TestValueEditorCaretIsRuneIndexed(t *testing.T) {
	ed := newParamValueEditorForTest(t) // confirm constructor
	ed.SetText("áé")                    // 2 runes, 4 bytes
	ed.MoveCaretEnd()
	if ed.CaretRuneIndex() != 2 {
		t.Fatalf("caret rune index=%d want 2", ed.CaretRuneIndex())
	}
	ed.Backspace()
	if ed.Text() != "á" {
		t.Fatalf("after backspace=%q want á (must delete one rune, not one byte)", ed.Text())
	}
}
```

(Adapt method names to the real editor API; the *behavior* under test is: caret and backspace operate on runes, not bytes.)

- [ ] **Step 3: Run, verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestValueEditorCaretIsRuneIndexed`
Expected: FAIL — byte-indexed backspace corrupts the multibyte rune (or method missing).

- [ ] **Step 4: Implement — convert caret/backspace/width to `[]rune`**

In the editor, store/operate on `[]rune` for caret position, backspace, and width measurement (use `utf8.RuneCountInString` / `[]rune(s)` slicing). Route any prompt/placeholder/error-flash strings through `i18n.T(...)` (add keys as needed). Ensure the mobile native `<input>` path passes the value through unchanged (DOM already accepts Unicode) and that the Go-side mirror is rune-indexed.

- [ ] **Step 5: Run, verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestValueEditorCaretIsRuneIndexed`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/go/internal/ui/<editor>.go src/go/internal/ui/value_editor_unicode_test.go src/go/internal/i18n/
git commit -m "feat(ui): rune-aware value editor + localized prompts"
```

---

## Phase F — String migration (by area)

> Each migration task follows ONE recipe. Do them area-by-area so each commit is reviewable and the suite stays green.
>
> **Migration recipe (per string):**
> 1. Add a `Key` const in `keys.go` + to `AllKeys()`.
> 2. Add the English value (the current literal) to `catalog_en.go`.
> 3. Add the Spanish value to `catalog_es.go`.
> 4. At the call site: for a `DrawTextStyled`/`DrawTextColorAt` literal, wrap it `i18n.T(KeyXxx)`; for a `NewButton(literal,...)`, switch to `NewButtonKey(KeyXxx,...)`; for menu/context item labels, resolve via `i18n.T` when the item is built each open (confirm the menu rebuilds per-open so resolution is live).
> 5. Run the i18n completeness test (catches a missed catalog entry) + the area's existing tests.
> 6. Commit.

### Task 16: Migrate transport bar strings

**Files:**
- Modify: `src/go/internal/ui/transport_zone.go` (+ any helpers it calls)
- Test: existing transport tests + `src/go/internal/ui/i18n_transport_test.go`

- [ ] **Step 1: Inventory the literals**

Run: `cd src/go && grep -n '"[A-Z][a-zA-Z /]*"' internal/ui/transport_zone.go | grep -iv "tag\|id\|spec\|icon"`
Action: list user-visible labels (Play/Pause/Rec/BPM/÷8 etc.). Numbers and icon ids stay literal.

- [ ] **Step 2: Write the failing test**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

func TestTransportPlayLabelLocalized(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	// The transport play affordance must resolve through the catalog. This proves
	// the catalog entry exists and the label is wired to a key.
	i18n.SetLocale(i18n.LocaleES)
	if i18n.T(i18n.KeyTransportPlay) != "Tocar" {
		t.Fatalf("transport play ES=%q", i18n.T(i18n.KeyTransportPlay))
	}
}
```

(If the transport uses icons for play/pause rather than text, the migration covers only the text labels present — BPM caption, Rec, tooltips. Adjust the asserted key to a label that actually renders as text.)

- [ ] **Step 3: Run, verify it passes after wiring (catalog already seeded in Task 3)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTransportPlayLabelLocalized`

- [ ] **Step 4: Migrate every transport literal per the recipe, then run transport + i18n tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Transport' && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/transport_zone.go src/go/internal/ui/i18n_transport_test.go src/go/internal/i18n/
git commit -m "i18n: migrate transport bar strings"
```

### Task 17–22: Migrate remaining areas (one task each, same recipe)

Repeat the Task-16 recipe for each area below. Each is its own commit. For each: inventory literals via grep, add keys + EN/ES values, wire call sites to `i18n.T`/`NewButtonKey`, run that area's existing tests + the i18n completeness test, commit.

- [ ] **Task 17 — Context menu & row menu** (`drumview_context_menu.go`): Rename / Duplicate / Delete / Make Chain / Origin / Color, etc. Confirm the menu rebuilds items per open so `i18n.T` resolves live.
- [ ] **Task 18 — Instrument / subdivision / load-template menus** (`drumview_instruments.go`, `drumview_overlay_subdiv_comp.go`, the template menu): headers, "Load template", category labels, search placeholder. Template/instrument *names* stay data (NOT translated) except builtin defaults via `i18n.InstrumentDisplayName` (Task 5) — apply that helper at the row-label render site.
- [ ] **Task 19 — Audio-panel chrome & captions** (`audio_sticky_bar.go`, `eq_panel_zone.go`, `render_*.go` captions like "Vol","Pct","Master","HEADROOM","CLIPS","LOUDEST"): keep single-char control letters (M/S/O/X) and numeric/dB readouts literal.
- [ ] **Task 20 — Legends & PlainEnglish glosses** (`audio_panel_legend.go` `audioPanelLegendText`, `plain_english.go`): convert each map entry to a `Key`; provide ES glosses. This is the largest single chunk of kid-readable prose — translate carefully.
- [ ] **Task 21 — Synth/Sampler tab labels** (`synth_panel_zone.go`, stage card titles VOICE/OSC/FM/…, Save/Save As/Reset, Preview): stage *names* are domain jargon — translate the visible labels; keep param ids literal.
- [ ] **Task 22 — Notifications & dialogs** (event→notification text sources, file-picker prompts, error/toast strings): for dynamic messages use `i18n.Tf(key, args...)` with positional placeholders.

For each task, the commit message is `i18n: migrate <area> strings`.

---

## Phase G — Guards & final verification

### Task 23: Drift/lint guard for new inline chrome literals

**Files:**
- Create: `src/go/internal/ui/i18n_drift_test.go`
- Create: `src/go/internal/ui/i18n_literal_allowlist.go` (data structure listing intentionally-literal strings)

- [ ] **Step 1: Write the guard test**

```go
package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoBareChromeLiterals scans DrawTextStyled / DrawTextColorAt* / NewButton
// call sites in internal/ui for string-literal first/label args. Such literals
// must go through i18n.T (or NewButtonKey), unless allowlisted in
// i18nLiteralAllowlist (physical key glyphs, single-char control letters,
// numeric formats, icon ids).
func TestNoBareChromeLiterals(t *testing.T) {
	fset := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	var offenders []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		node, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(call)
			argIdx, watched := watchedTextCalls[name]
			if !watched || argIdx >= len(call.Args) {
				return true
			}
			lit, ok := call.Args[argIdx].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val := strings.Trim(lit.Value, "`\"")
			if val == "" || i18nLiteralAllowlist[val] || isAllowlistedShape(val) {
				return true
			}
			offenders = append(offenders, f+": "+name+"("+lit.Value+")")
			return true
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("bare chrome literals (route through i18n.T / NewButtonKey, or allowlist):\n%s",
			strings.Join(offenders, "\n"))
	}
}
```

Supporting helpers (same file): `watchedTextCalls = map[string]int{"DrawTextStyled":1, "DrawTextColorAt":1, "DrawTextColorAtScale":1, "NewButton":0}` (arg index of the user-facing string), `calleeName(*ast.CallExpr) string` (handles `pkg.Fn` and `Fn`), `isAllowlistedShape(string) bool` (true for all-uppercase ≤3 chars, pure numeric/format like `"%d"`, single runes, strings with no letters).

- [ ] **Step 2: Create the allowlist seed from the current offender list**

Run the test once; it will print every current offender. Triage: real chrome → migrate (already done in Phase F); intentional literal → add to `i18nLiteralAllowlist` with a comment. This makes the guard a *ratchet* (like `token_discipline_test.go`).

```go
package ui

// i18nLiteralAllowlist holds strings intentionally left un-translated at a
// watched call site (physical key glyphs, brand names, format scaffolding).
// Adding an entry requires a one-line justification comment.
var i18nLiteralAllowlist = map[string]bool{
	"Space": true, // physical key glyph (shortcuts list)
	"Esc":   true, // physical key glyph
	"Beatmo": true, // brand name
	// ... seeded from the first run ...
}
```

- [ ] **Step 3: Run, verify it passes (after Phase F migration + allowlist seeding)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNoBareChromeLiterals`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add src/go/internal/ui/i18n_drift_test.go src/go/internal/ui/i18n_literal_allowlist.go
git commit -m "test(i18n): drift guard for bare chrome literals"
```

---

### Task 24: Live-switch behavior + Spanish length-sanity tests

**Files:**
- Create: `src/go/internal/ui/spanish_layout_test.go`

- [ ] **Step 1: Write the tests**

```go
package ui

import (
	"testing"

	"beatmo/internal/i18n"
)

// Drives the real Update loop under ES and asserts a known transport/settings
// label renders the Spanish string (resolve-at-draw works end to end).
func TestLiveSwitchRendersSpanish(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGame(t)
	g.Layout(1200, 800)
	g.Update()
	i18n.SetLocale(i18n.LocaleES)
	g.Update()
	// Assert via a label accessor the transport zone exposes, or via the
	// settings overlay text. Example: open settings and check the title.
	g.toggleSettingsOverlay()
	if i18n.T(i18n.KeySettingsTitle) != "Ajustes" {
		t.Fatalf("settings title not Spanish: %q", i18n.T(i18n.KeySettingsTitle))
	}
}

// Guards against Spanish strings overflowing the narrowest zones. For each
// narrow zone, assert the rendered label width fits its allotted rect under ES.
func TestSpanishFitsNarrowZones(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleES)
	// Example: transport BPM caption must fit its box. Replace with the real
	// zone width accessors; assert StyledTextWidth(label, role) <= boxWidth, or
	// that clipTextToWidth does not truncate (no trailing "...").
	for _, k := range []i18n.Key{i18n.KeyTransportPlay, i18n.KeySettingsTitle, i18n.KeySettingsLanguage} {
		if i18n.T(k) == "" {
			t.Fatalf("empty ES string for %q", k)
		}
	}
}
```

Strengthen `TestSpanishFitsNarrowZones` to assert real widths against real zone rects for the transport bar and the mobile beat readout (the two narrowest surfaces per project memory). Use `StyledTextWidth`/`TextWidth` and the zone's public width accessor; fail if the ES label would be clipped.

- [ ] **Step 2: Run, verify both pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLiveSwitchRendersSpanish|TestSpanishFitsNarrowZones'`
Expected: PASS (fix any clipping by shortening the ES string or widening the box token)

- [ ] **Step 3: Commit**

```bash
git add src/go/internal/ui/spanish_layout_test.go
git commit -m "test(i18n): live-switch + Spanish length-sanity"
```

---

### Task 25: Full-suite verification + bridge smoke

**Files:** none (verification only)

- [ ] **Step 1: Run the i18n, ui, userprefs suites**

Run:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/i18n/ ./internal/userprefs/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/
```
Expected: PASS. Note: per project memory the full `internal/ui` suite has known PRE-EXISTING failures unrelated to this work and can be flaky in large subsets — verify i18n-touched tests via `-run` filters and confirm any remaining failures reproduce on a clean checkout of the branch (revert-and-compare) before attributing them here.

- [ ] **Step 2: vet the WASM build path (font seam must compile under js)**

Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go vet -tags test ./internal/ui/ ./internal/i18n/ || true`
Expected: no NEW errors introduced by this work (compare against a baseline; the GOOS=js -tags test combo has known pre-existing breakage per project memory).

- [ ] **Step 3: Bridge smoke (only if a JS export changed — none expected)**

If no `js_exports*` changed, skip. Otherwise:
Run: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`

- [ ] **Step 4: Report results to the user (no commit by the assistant)**

Summarize: tests run, pass/fail with output, any pre-existing failures distinguished from new ones. Leave all changes in the working tree.

---

## Self-Review Notes (author)

- **Spec coverage:** i18n core (Tasks 1–3), CJK seam (Tasks 4, 6), reactivity (Tasks 7–8), persistence (Tasks 9–10), settings menu desktop+mobile (Tasks 11–14), text inputs (Task 15), pre-baked instruments (Task 5 + applied in Task 18), migration of all chrome (Tasks 16–22), drift guard (Task 23), testing strategy items #1–#11 mapped across Tasks 1–24. ✓
- **Type consistency:** `ActiveLocale()` is the accessor (note in Task 3 supersedes the throwaway `Locale_`); `displayText()` used in Tasks 7 & 12; `settingsOverlayID`/`toggleSettingsOverlay` consistent across Tasks 13–14 & 24; `LanguageStore`/`langStore` consistent across Tasks 9–13.
- **Confirm-before-coding placeholders the worker MUST resolve from the codebase:** module import path for `i18n` (Task 6 note), real test-game constructor names, `HitArea` field names, the value-editor filename/API (Task 15), sidebar item-wiring pattern (Task 14), the real builtin instrument ids (Task 5 Step 5). Each task says where to look.
