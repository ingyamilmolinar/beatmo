# Typed entities — models, typed IDs, one registry each

2026-10-24 · status: **approved 2026-10-24; Phase 1 revised to the lean plan (rev 2) the same day** · inventory:
`2026-10-24-typed-entities-inventory.md` · background:
`2026-10-23-registered-names-report.md`

## Intent

The code refers to most of its named things by hand-typed strings: `"oboe"`,
`"filter_cutoff"`, `"snare.rimshot"`, `"oboe-A4"`. Nothing ties a string to the
place that registers it. So:

- a rename turns code into code about nothing, with no compile error;
- one word means several things (`"snare"` is an instrument, a voice block, a
  module prefix, a kit role, a category);
- properties get guessed from spellings: bassoon sits in the Bass tab because
  its id contains "bass".

The user's words: every entity gets a model, its IDs are typed, and code
receives the typed ID wherever it needs an identity, resolving it to the full
model when it needs properties. "Simple and straightforward." Build slowly, test
well, preserve existing behaviour, adapt tests step by step.

## The pattern every phase applies

For each entity kind:

1. **A typed ID**: `type <Entity>ID string` (or int), declared in the lowest
   package that owns the entity.
2. **A model**: a struct of the entity's properties. Reuse the existing struct
   where there is one (`instrument.Doc`, `ModuleTypeDecl`, `EffectRegistration`,
   `wavref.Ref`, `Descriptor`, …). Do not duplicate it.
3. **One registry with one lookup**: `Lookup(id) (Model, bool)`. Add one where
   none exists (scores, templates, scenes, roles).
4. **Generated typed constants for closed sets**, produced from the registry's
   data by an existing or new generator. A rename then breaks the build.
5. **Functions say what they need.** They take the typed ID when they only
   refer to the entity, and take the model when they read its properties.
   No function re-derives a property from the ID's spelling.
6. **Boundaries convert once.** Data enters as a string (JSON, the JS bridge,
   flags, prefs) and is converted to the typed ID at the reader. The JSON shape
   never changes: the Go field type changes, the `json:` tag does not.
7. **Open sets share the type.** User instruments (`user.<slug>.<hash>`),
   instance rows (`<base>--N`) and sample ids are `instrument.ID` values too.
   They have no constant and resolve through the lookup.
8. **No ratchet by default.** Constant-keyed tables break the build on a rename;
   a literal-budget guard is added only if hand-typed IDs demonstrably come back.

## Goals

- Every entity in the inventory's tiers 1–3 has a typed ID, a model and a
  lookup. Production code names closed-set members through constants.
- Mixing namespaces (an instrument id passed as a voice block) fails to compile.
- Instrument properties (category, kit role, GM note) are data, not spelling.
- Each step leaves the tree green with behaviour unchanged, and is small enough
  to review and commit on its own.

## Non-goals

- Renaming any wire string. Instrument ids, module type names, param names and
  JSON keys stay byte-for-byte as they are. (A rename would change document
  content hashes, template pins, listening verdicts and users' saved files.)
- Changing sound, layout or the instrument menu, except through the gated
  behaviour changes listed below.
- Typing internal int enums that are already typed (`GestureKind`, `log.Level`, …).

## Anti-goals (a failure even if it compiles)

- Any change to exported project / instrument / bundle / prefs bytes, or to a
  render. The existing byte-identity tests are the proof
  (`TestExportImportMasterMixIsByteIdentical*`,
  `TestEveryBuiltinTemplateRoundTripsByteIdentical`, `TestAssetPackIsCurrent`,
  `TestSynthGoldenByteIdentity`, …).
- A changed `instrument.ContentHash` / `DocSHA` for any shipped document.
- A big-bang edit that leaves the tree red between steps.
- A typed ID with no registry or lookup behind it (a type that is just a new
  spelling of `string`).
- A behaviour change slipped in as "cleanup". Every one is listed below and
  needs the user's yes first.

## Constraints

- The tree carries a lot of uncommitted work. **The user confirmed (2026-10-24)
  that this refactor is the only work in the tree while it runs**, and that
  pervasive type changes and straightforward wiring across many files are fine
  in one step. The work still goes inline (no parallel editing subagents), and
  each step ends green. Steps do not wait for checkpoint commits. `git stash`
  is never used, and the user makes the commits.
- The repo has three build configurations: default, `-tags test` (stubs) and
  `GOOS=js`. Build-tag twins (`catalog_desktop.go` / `catalog_stub.go`,
  `*_js.go`) must change together.
- Generators already exist for the instrument roster (`gen_instrument_ids.go`)
  and the param ABI (`cmd/gen-synth-abi`). Extend them rather than adding
  parallel ones.

## Verification, every step

- `go vet` + build in all three configurations: default, `-tags test`, and
  `GOOS=js GOARCH=wasm`.
- The fast suite: `go test -tags test -modfile=go.test.mod ./...`.
- `make test-real` for any step that touches `internal/audio` or rendering. The
  byte-identity tests live there.
- `wasm_bridge_smoke.browser.test.js` for any step that touches `js_exports_*`.
- **Characterization goldens before any step that moves data or behaviour**
  (not for pure type changes): record the current answers, then prove the step
  reproduces them.
- A screenshot captured and inspected (CLAUDE.md rule) for any step whose output
  is visible. Only the gated category change (B1) is expected to be visible.

## Phases

| Phase | Entities | Exit criterion |
|---|---|---|
| **1. Instrument ID** | `instrument.ID`, roster constants | Every instrument identity in production code is `instrument.ID`; production code names built-ins through `roster.X` |
| **2. Synth model** | ABI param, knob, stage (one entity for module/group/toggle/section), voice block, voice family, module type, numeric selectors (source, osc_type, variants, filter_type, …) | One `Param` model replaces the ~12 parallel param tables; one `Stage` entity with its names as properties; selectors generated for Go/C/JS |
| **3. Effects + catalogs** | insert effect type (close the gaps), effect param, effect category, send params, kit (model), GM table (one), descriptor + group, reference recording, score, template, synth dial, macro card, node type / logic kind / groove kind / chord quality in files + hooks, sample-edit field, refs-tool vocabulary | Each has typed ID + model + lookup; Go/JS duplicates generated from one source |
| **4. UI** | panel tab / view mode slugs, icon, hooks label table, i18n key / locale strings, subject list, scene, theme token / category / preset / swatch, inst-menu mode duplicate | No string API gets around an existing type; dead code from the inventory removed |

**Phases 2–4 each get their own detailed design before they start**, built on
what Phase 1 teaches. Phase 2 especially needs a modelling decision (what a
"stage" is), not just types. Only Phase 1 is specified step by step here.

## Phase 1 in detail (revision 2, 2026-10-24)

Revision 1 bundled the typing with a data move, a custom guard tool, a literal
ratchet and the mixer-channel type. On review the user asked for a leaner,
more direct approach, so Phase 1 is now **the instrument ID only**; the plan
`docs/superpowers/plans/2026-10-24-typed-entities-phase1.md` has the steps:

1. `instrument.ID` (`type ID string`, leaf package) + `Doc.ID` / `Doc.Origin`;
   the existing roster generator also emits `internal/instrument/roster`
   (one constant per built-in + `All`).
2. JS safety at the one choke point: `jsFn` normalises named string types
   before they reach `syscall/js`; the `case string` sites get a
   `case instrument.ID`; direct `Set`/`ValueOf` calls audited once.
3. Signatures bottom-up (audio → subpackages / hooks / prefs / i18n /
   eventlog → ui → cmd), the compiler finding every caller; conversions only
   where data enters.
4. Production literals → `roster.X`; the worklist comes from the compiler
   (temporarily make `ID` a struct and build with `-gcflags=-e`). Tables
   keyed from data files get "keys ⊆ roster" tests.
5. CLAUDE.md.

Moved out of Phase 1, each its own approved follow-up: `audio.ChannelID`;
deleting `InstrumentConfigs`; category / role / GM note as data in
`index.json` — done together with correcting them (B1), because writing
today's guessed values into data while keeping the spelling rules for
non-roster ids would leave two sources of truth. No literal ratchet for now:
constant-keyed tables already break the build on a rename; a ratchet is added
only if hand-typed ids demonstrably creep back.

## Behaviour changes — each needs the user's yes, each its own step

| # | Change | Phase | Why it is gated |
|---|---|---|---|
| B1 | Correct categories: bassoon, double-bass, tubular-bells, fm-bell(-soft), and the 26 in Other (incl. marimba, vibraphone, xylophone, glockenspiel, tuba, piccolo, sitar, koto, accordion, theremin, …) | follow-up 3 of Phase 1 (data move + correction together) | Changes the instrument menu's tabs and kit roles: a product decision per instrument, with before/after screenshots |
| B2 | Unknown instrument ids fail loudly (`SetInstrument`, scene setups, id-keyed tables at load) | after Phase 1 | Changes what happens on bad input; the accepted id sets (roster, user, instance, sample) must be right first |
| B3 | A user / Save-As / instance instrument's category, role and GM note come from its base or origin instead of its own spelling | later; needs its own design | Today "My Bass" lands in Bass because of its name, and `kick-deep--2` gets GM 38. There is no data to resolve from yet: a Save-As library entry is stored with an empty origin, so this needs a provenance field, which changes stored bytes |
| B4 | One General MIDI table (the two disagree on note 37) | Phase 3 | Changes capture/MIDI output or templates |
| B5 | One spelling for the master channel (`"main"` vs `"master"`) | Phase 4 | Touches the JS bridge and scope labels |

B1 and B3 are not steps in the Phase 1 plan. B1 needs a decision per
instrument for about 31 ids, which means the user's category table, possibly
new categories with i18n strings, and screenshots. It gets planned once the user has
decided that table, together with moving the properties into `index.json`.

### Settled details

- The JS bridge converts with explicit `string(...)`, because `js.ValueOf`
  panics on named string types; `jsFn` normalises return values.
- Kit roles, display names and mixer channel ids stay `string` in Phase 1.
