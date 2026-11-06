# Internationalization (i18n) + Settings Menu — Design

**Date:** 2026-06-15
**Branch:** add-core-node-types
**Status:** Approved design, pending spec review

## Goal

Add a central internationalization system for **all user-facing chrome** in Beatmo,
ship **English** and **neutral Latin-American Spanish (es-419)** to start, and replace
the desktop grid "?" help button with a **Settings menu** that lets the user switch
language and view keyboard mappings. The system must be **CJK-ready** (Japanese/Chinese
glyphs down the road) without call-site changes, and must cover **text inputs** and
**pre-baked instrument names**.

## Non-Goals

- Bundling CJK fonts now (we build the seam; fonts come later).
- Translating project **data**: user-renamed instrument names, numeric readouts,
  physical key glyphs (`Space`, `Esc`, `[ ]`), single-char control letters (M/S/O/X).
- Plural/gender grammar engines (`Tf` placeholder formatting is enough for 2 locales).
- Dev/debug/log/test strings.

## Scope of "user-facing chrome" (translated)

Buttons, menu & context-menu items, panel labels & captions, legends
(`audioPanelLegendText`), `PlainEnglish` glosses, settings + keyboard-shortcut
descriptions, notifications/toast text, text-input prompts/errors/placeholders, and
**pre-baked instrument default display names**.

---

## Architecture

### 1. `internal/i18n` package (leaf — no internal deps)

```
internal/i18n/
  locale.go      // Locale (LocaleEN, LocaleES), AllLocales, Parse/String
  keys.go        // typed Key constants, grouped by area
  catalog.go     // registry map[Locale]map[Key]string + active-locale atomic
  catalog_en.go  // en = map[Key]string{...}   (canonical key set)
  catalog_es.go  // es = map[Key]string{...}   (neutral es-419)
  i18n.go        // T, Tf, SetLocale, Locale, OnChange, font-face provider seam
```

- `T(key Key) string` — active-locale string. Fallback chain: active → English →
  raw key string. Never panics, missing translations degrade visibly.
- `Tf(key Key, args ...any) string` — `fmt.Sprintf` over the resolved template.
  Placeholders are positional so es-419 can reorder. Arg-count parity is tested.
- Active `Locale` is an atomic. `SetLocale(l)` flips it and fires registered
  `OnChange` listeners.
- All values are UTF-8 (Go-native), so the catalog is encoding-ready for CJK today.

### 2. CJK-ready font seam

The current renderer (`textcache.go`) is Latin-only (Inter). To allow CJK later
without touching call sites:

- `i18n` exposes a **font-face provider**: `FontGeneration() int` + a hook the UI
  registers so the renderer can resolve the correct face for the active locale.
  Today EN/ES both return the Inter face; a future `ja`/`zh` returns a CJK face.
- The text **sprite-cache key gains a font-generation token** so two locales using
  different faces can never collide in the cache.
- Bundling CJK fonts is future work; this design only guarantees no call-site or
  cache-key rework is needed when they arrive.

### 3. Reactivity on language switch

- **Resolve-at-draw rule:** call sites resolve via `i18n.T(...)` at draw/layout time,
  not at construction. Migrating a string means wrapping its literal in `T(key)` at
  the point it is rendered.
- `Button` gains a **key-aware variant** (`NewButtonKey`) whose `Label()` resolves
  `i18n.T(key)` live each frame. Existing `NewButton(literal)` stays for
  non-localized/data labels.
- On `SetLocale`, the UI's `OnChange` listener:
  - bumps a UI text epoch,
  - forces `tree` + `gridTree` relayout (`needLayout`),
  - invalidates text-baking caches (row/sprite/edge/curve/legend).
  The text sprite cache itself needs no key surgery for EN↔ES (keyed by string
  content); the font-generation token handles future cross-face cases.

### 4. Text inputs (Unicode-correct)

- The shared `ParamValueEditor` and the mobile native input become **rune-aware**:
  caret position and width measured over `[]rune`, never byte offsets.
- Their prompt/placeholder/error-flash text routes through the catalog.
- The instrument-rename input accepts full Unicode (already a DOM `<input>` on
  browser; ensure the Go-side editing model is rune-indexed).

### 5. Pre-baked instrument names

- New `builtinID → Key` map in `i18n` giving a **localized default display name**
  (e.g. `kick` → "Kick"/"Bombo").
- **Display rule:** show the localized name when the instrument's stored name equals
  the builtin's canonical English default (or is empty); once the user renames it,
  the stored name wins verbatim.
- **Export/import stays byte-identical** — localization is display-only. Projects
  remain language-independent and portable; templates/demo JSON are unchanged.

### 6. Settings menu (folds in keyboard mappings)

- The desktop `gridHelpBtn` ("?") becomes a **Settings entry**. New `IconSettings`
  gear added to `icons.go` (forbidden-glyph rule → IconID, not a raw glyph).
- New `SettingsOverlay` (portal, `menu_chrome` styling) with two sections:
  - **Language** — segmented control (English / Español); click live-switches.
  - **Keyboard shortcuts** — the existing `shortcutRows`, descriptions now localized
    (key glyphs stay literal). On mobile this section shows a localized
    "desktop only" note.
- The standalone `ShortcutsOverlay` is replaced by this section. The `/` and
  audio-panel `?` shortcuts now open Settings.
- **Mobile entry point:** a Settings item in the mobile sidebar/overflow
  (`sidebar_zone.go`) so language switching works on phones.

### 7. Persistence (`userprefs` v7)

- Add `Language string` field, `LanguageStore` interface, `marshalPrefsV7`,
  implemented in both `store_js.go` / `store_notjs.go`.
- Loaded in `game_new.go` → `i18n.SetLocale` at startup; saved on switch.
- Absent field defaults to English; v6→v7 upgrade is non-destructive.

---

## Testing Strategy (emphasis)

1. **Catalog completeness** — every `Key` exists in every `Locale`; no extra ES keys;
   no empty values. (`AllLocales` × all keys.)
2. **No-orphan keys** — every declared `Key` const appears in the EN catalog and vice
   versa.
3. **Resolution unit tests** — `T`/`Tf` fallback chain (missing ES → EN → raw key);
   placeholder arg-count parity between locales.
4. **Drift/lint guard** — scan `internal/ui/` draw/button call sites for new bare
   user-facing literals, with an allowlist for data/physical strings; ratchet like
   `token_discipline_test.go`.
5. **Live-switch behavior** (ebitenstub) — set ES, drive `Game.Update()`, assert a
   known surface renders the ES string and that relayout/cache invalidation fired.
6. **Settings overlay functional test** — open via gear/`/`, click Español, assert
   locale flips + persists (fake `LanguageStore`), shortcuts section present &
   localized; mobile entry path covered.
7. **Persistence round-trip** — v7 marshal/parse, v6→v7 upgrade, absent-field default.
8. **Spanish length sanity** — layout smoke under ES for narrow zones (transport,
   mobile readout) to catch clipping from longer strings.
9. **Instrument-name localization** — builtin default shows localized; renamed shows
   stored; export bytes unchanged across locale switch.
10. **Rune-aware input** — caret/width correctness for multi-byte input in
    `ParamValueEditor` and rename.
11. **Font seam** — cache key includes font generation; switching the (stubbed) face
    provider produces distinct cache entries.

## Risks / Mitigations

- **Scale of migration** (hundreds of literals): drift guard + completeness test keep
  it honest; migrate by area (transport → grid/settings → menus → panels).
- **Spanish string length** overflowing narrow zones: length-sanity layout test (#8).
- **Reactivity gaps** (a label resolved once at construction): resolve-at-draw rule +
  live-switch test (#5) catch stale surfaces.
- **CJK reality check**: only the seam ships now; loading/bundling fonts is a tracked
  follow-up, not in this plan.

## Out-of-tree note

Per the project's standing instruction, this spec and all subsequent changes are left
in the working tree and **not** committed/staged by the assistant.
