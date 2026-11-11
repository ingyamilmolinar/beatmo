# Typed Entities — closing the gaps (closed enums, one name per entity)

> Execute inline (superpowers:executing-plans). Never `git stash`, never stage
> or commit. Follows `2026-10-24-typed-entities-phase1.md`.

**Goal (user, 2026-10-24):** close every gap the Phase 1 review found.
Validate and restrict the possible values, one name/ID per entity (`""` is
not the master bus), and typed enums so the compiler protects them.

**Rules.**
- A closed set is a Go enum (`type X int` + `iota`), never a named string:
  an untyped string literal cannot become one, so a misspelling does not
  compile. It carries `String()`, `MarshalText`, `UnmarshalText` (strict:
  an unknown spelling is a decode error) and its zero value is the explicit
  "none" when absence is legitimate. Wire spellings are unchanged.
- An open identity (instrument id) stays `type ID string` but is minted and
  parsed only through package `instrument` (constructors + `ParseID` at the
  boundaries).
- One spelling per entity: no aliases in parsers, no `"" means X`.

## Steps (each ends with BUILD3 + FAST; REAL/BRIDGE where noted)

1. **`channel.ID` opaque.** `struct{name string}`; `Master`, `Of(inst)`,
   `Parse(s) (ID, error)` (`"main"` → Master, an instrument id → `Of`; `""`
   and anything else → error); zero = no channel; `IsMaster()` is
   `== Master`; text marshaling (`""` ↔ zero). Every `""`-as-master site goes.
2. **Instrument id model.** `instrument.ParseID`, `ID.Base()`,
   `instrument.Instance(base, n)`, `instrument.UserID(...)`,
   `instrument.UserSampleID(...)`; roster generator rejects `main`;
   `roster.Has`; `audio.BuiltinInstrumentIDs` / `ShippedInstrumentIDs`
   removed in favour of `roster.All`; `jsArgs.Instrument` / `jsArgs.Channel`.
3. **WAV upload mints a user-sample id**; the typed text is its display name
   (a WAV named "Kick" no longer replaces the built-in kick). Screenshot.
4. **Closed enums:** `instrument.Role`, `audio.Category` (the UI keys the menu
   by it, not by its display label), `model.NodeType` / `LogicKind` /
   `GrooveKind` on the wire and in hooks, catalog source + scope, project
   instrument kind (legacy `sample-` branch removed), bundle kind, WAV source
   kind, hooks send kind / filter / theme scope / seek origin, view-mode
   slugs (aliases removed).
5. **Cleanups:** `InstrumentParamPayload{Instrument, Origin}`; dead category
   registry; id round-trips in `cmd/*`; `slices` instead of local helpers;
   stale comments.
6. **UI composite ids:** node-sidebar hit ids and analyzer keys become typed
   values instead of prefix-encoded strings.
7. **Committed JS-bridge guard** (`scripts/jsaudit`, run by `make test`): no
   named type reaches `syscall/js`.
8. **Docs + memory.**

## Outcome (2026-10-24)

All eight steps done. Behaviour changes (each one a failure mode removed):

- `""` is no channel: UI setters refuse it, the mixer drops it, audio.js
  `getChannelChain('')` fails closed. `setEQChannel('')` returns false.
- A WAV the user uploads is a user sample (`wavInstrumentID(name)`), labelled
  with the typed name; it can no longer replace the built-in its name matches.
  A project row with a WAV `path` on a built-in id is rejected on import.
- Strict decode: unknown node type / logic / groove / group param / kit role /
  effect type / bundle kind / WAV source kind / `-ui-state` view mode rejects
  the file (the old logic aliases and case-folding are gone; unknown node types
  no longer import as regular, unknown grooves are no longer dropped).
- Non-built-in instruments are classified by DISPLAY name (a WAV named
  "My Kick" lands in Kick; its id is a hash).
- Found while converting: on wasm a pre-EQ / synth analyser's `Snapshot()`
  asked for channel `"preEQ:kick"` (string-encoded tap); desktop and test
  builds minted catalog sample ids with two different slug rules; the
  `obj.Set("logicKind", LogicKind)` browser crash (caught by `make js-audit`).
- Removed dead code: the category registry (`RegisterCategory` /
  `AssignCategory`), `SetInstrumentOrder`, `SoundMeta.Scope`,
  `audio.BuiltinInstrumentIDs` / `ShippedInstrumentIDs` (→ `roster.All`),
  `instrument_ids_gen.go`, the duplicated groove-offset rule (→
  `GrooveKind.Seconds`).

Kept as named strings on purpose: `audio.EffectType` (strictly decoded),
theme category / swatch / preset names, synth param / module / stage names
(Phase 2 of the spec), i18n keys, hook payload free-text fields.
