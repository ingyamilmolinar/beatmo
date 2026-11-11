# Instruments that cost only while used, bounded circuits, and one resource budget

Date: 2026-10-25 · Status: APPROVED 2026-10-30 ("Sounds good! Please write the implementation plan") — nothing built yet. Plans: `docs/superpowers/plans/2026-10-30-residency-a-measure.md`, `…-b-audio-bundles.md`, `…-c-residency-and-switch.md`, `…-d-caps-and-governor.md`. Amended 2026-10-31 by `docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md`, which wins where the two disagree.

- Revision 2 (10-25): the user's answers folded in — audio bundles owned by
  the instrument (§5.2), descriptions load for every instrument (§5.1).
- Revision 3 (10-25): the Sampler edit joins the bundle.
- Revision 4 (10-30): Save and Reset go, and Save As is the only save
  (§5.2). The document was re-checked against the one-audio-pipeline refactor
  of 10-27 (`docs/superpowers/specs/2026-10-27-one-audio-pipeline-design.md`):
  pan and the send buses no longer exist, and effects and EQ run desktop's C
  on both platforms.

Follows the live-edit churn fix of 10-25 (CLAUDE.md § "Live edits retune the
audio graph, never rebuild it"; memory `project_live_edit_graph_churn`).

## 1. What the user asked for (their words, condensed)

1. **Thousands of instruments** must be hostable "and everything should
   work". Switching a row to an instrument not used recently may take a bit
   longer, as long as the user cannot notice.
2. **The switch takes effect at the row's next note** — there should be time
   to build its channel and load what the next beats need (cached). Proven by
   the audio the user HEARS: no disruption, no lag. Always evidence based.
3. **A memory budget, not a fixed number of instruments**, decides what stays
   loaded. The instrument menu loads lazily; selectable options are
   pre-loaded only if measurement shows a cold load is too slow.
4. **Metadata strictly decoupled from resources**: an instrument's synth /
   sampler / FX description is separate from its channel, audio nodes and
   buffers; resources load only when needed and are cleaned up.
5. **CPU and memory limits defined in Go that the browser enforces too.**
   Priority: the audio thread first, then UI rendering. A lower frame rate is
   acceptable; **a note triggered later than exactly where it should fire is
   not**.
6. **Bounded circuits**: a maximum number of instruments per circuit, so the
   load is never unbounded; within the limits the game plays smoothly on most
   normal phones and laptops.
7. Robust memory infrastructure and tests; **load tests, not only soaks**;
   always tested **in a real browser** and **in isolation with predictable
   load**.
8. **The instrument owns its effects and settings** — "everything belongs to
   the instrument, not to the row", and the row points to the instrument's
   EQ, effects "or whatever is on top of the synth instrument data". These
   form **audio bundles**, groups of audio settings that the row and the
   instrument share. When the instrument being loaded into a row has its own
   EQ and effects, **the user is asked** whether to keep the row's settings
   or take the instrument's — "we cannot make the decision for them".
   **The EQ and the Sampler settings are in the bundle too**: "they are part
   of the instrument but could be used as part of the row as well (although
   most likely better tied to instrument)".
9. **Descriptions load for every instrument**; only the audio side loads
   lazily.
10. **No Save and no Reset.** "Save As will start with a prepopulated input
    of current instrument and user can choose to override it by using the
    same name or save it as a different instrument if using a different
    name." Reset to factory "does not make sense anymore". Going back to
    "Factory" or "Upstream" versions of anyone's instruments and circuits may
    come later — "but not now".

## 2. Measured starting point (real Chrome, 2026-10-25)

| Measurement | Result |
|---|---|
| WebAudio channel chains at boot, 5-row circuit | **151** (one per known instrument, not per row) |
| Audio-thread work for 10 s of audio, idle chains (2 GainNodes each, OfflineAudioContext, repeated) | 150: ~0.25 s (2.5 %) · 300: 0.46 s · 1 000: 1.1–1.9 s (11–19 %) · 2 000: ~6 s (60 %) · 5 000: 8–26 s (cannot keep real time) |
| Cold voice render, idle (`ensureSynthSample`) | 2–12 ms |
| Cold switch of a row during playback (5 instruments) | voice ready 42–80 ms after the switch |
| …first note due before the voice was ready (organ: due +79 ms, ready +80 ms) | **that note started 16 ms late** |
| Page JS heap, boot → after ~10 instruments played | 13 MB → 46 MB; `renderCache` has no size limit |
| Go heap (WASM) | 23–25 MB; runtime limit set to 1 500 MB (`runtime_defaults_wasm.go`) |
| Chrome CPU throttling (CDP, 4×) on the same fixed workload | main thread 157 → 470 ms; **Web Worker 117 → 112 ms; AudioWorklet 100 → 100 ms (not throttled)** |

Consequences: idle channels are not free on the audio thread and grow
super-linearly, so "thousands of instruments" breaks audio with today's
design; memory grows with every instrument ever played; "enough time to
switch" is usually but not always true; and Chrome's throttle alone cannot
emulate a phone — workers and the audio thread need their own calibrated load.

**Since the 10-27 refactor** a channel with effects or EQ costs more than
the idle chains measured above. Each stage is a `beatmo-dsp` AudioWorklet
node running C. It runs every render quantum while it exists, even with no
voice playing, so tails keep ringing (`dsp_worklet.js`; desktop likewise runs
such a channel every block). Phase 0 re-measures every row of this table on
the new pipeline, including the cost of N idle channels that carry effects
or EQ.

## 3. What exists today (code survey, 10-25; re-checked 10-30)

Paths relative to `src/go/internal/` unless `src/js/` or `cmd/`. Claims
marked *(verified)* were re-read in the code on 10-30, after the refactor; the
rest come from a read-only survey and are re-checked by the tests that replace
them.

**Already decoupled:** the document stores (shipped pack, user library,
project store, circuit docs) are separate from the voice caches (content-hash
keys; doc edits never evict); desktop EQ records are separate from EQ
processors; sample-edit descriptors; the synth-graph cache (LRU 16); the gated
warm set (row instruments only, cap 512); recording (rows in use).

**Created for EVERY known instrument at boot, not only the ones in use:**

- a project-store entry + flattened block (1 321 doubles ≈ 10.6 KB) per
  library id (`ResetInstrumentDocs`, `audio/instrument_docs.go`) and
  `setInstrumentBlock` pushes per id to JS on WASM;
- a playable registration per id whose `Register` creates a channel
  (`audio/engine.go`, `audio/engine_wasm.go`: `Register` →
  `InstrumentChannel`) — *metadata registration creates a resource*
  *(verified)*;
- a `Channel` per id plus one `platformChannelVolumeChanged` per id
  (`resetInstrumentChannels`, `audio/channels.go`) *(verified)*, which on WASM
  queue `pendingChannelOps` that build a 2-GainNode chain per id when the
  AudioContext appears — the measured 151 chains;
- every persisted user sample's PCM (Go keeps two copies, JS a third).

**Reads that create resources:** `ChannelVolume` (`ensureChannel`), JS
`channelVolume`, `getChannelChain` (builds a chain for any id it is asked
about) and the analyser snapshot calls, `MergedInstrumentParamsRO` →
`EnsureInstrumentDoc`.

**Metadata that lives only inside a resource:** JS channel volume (a node
value), the insert chain (`insertChainMgr` in Go beside its processors;
`chain.fxChain` in JS), the JS EQ program (`chain.eqProgram`), user-sample
PCM.

**Never released (session-long):** Go `docStore` entries, `chanMgr`
channels, desktop mixer + analyzer slots, the desktop voice cache's
`lastSamples` (one buffer per instrument ever rendered, outside its 48 MB
budget), per-id param/def caches; JS `channelNodes` (cleared only by the
offline-render test seam, `__offlineRenderForTest` *(verified)*), analysers,
`instrumentBlocks` + render tables, `renderCache` (every pitch of every
instrument ever played, PCM plus an AudioBuffer copy, **no cap** — ≈ 368 KB
per melodic pitch at 48 kHz), debug maps. **The `beatmo-dsp` nodes** (one
for inserts, one for EQ, per channel) are created on the channel's first
effect or EQ and never removed. Emptying the chain keeps its node, which
keeps running every quantum (`updateInsertEffects`, `audio.js`)
*(verified)*. Row delete releases nothing. Import releases only DSP state;
the import EQ clear `InstrumentChannelIDs()` has no production caller, so a
later pick of an instrument from the previous project may inherit its EQ or
effects (to be confirmed by a test before it is fixed).

**Mixer settings are split between the row and the channel** *(verified)*:

- `DrumRow` (`ui/drumview.go`) stores EQ (band gains, band mutes, high-pass
  / low-pass on and cutoff) and a copy of the insert-effect chain, and
  applies them to the INSTRUMENT's channel, so two rows on one instrument
  share one chain.
- The FX panel writes only the audio layer, so export reads insert effects
  from the audio layer but EQ from the row (`exportInstrument` and its
  writer, `ui/export.go`): two sources for one mixer strip.
- A switch (`SetInstrument`, `ui/drumview_transport.go`) re-applies none of
  the row's settings: the new instrument plays through whatever its channel
  holds.
- Pan and the send buses were deleted on 10-27 (CLAUDE.md: "every stored
  setting needs a control (or goes)"); a file carrying them imports with
  those keys ignored.
- Row volume is not a channel setting: it is a per-note gain (nothing in the
  UI calls `SetChannelVolume`).
- The Sampler edit (trim, pitch, gain, reverse, normalise, fade) is already
  per instrument: applied to the rendered voice at trigger time and folded
  into the voice-cache key. Its saved form lives in the PREFS store's
  `sample_edits` (`userprefs/store.go`, `SampleEditStore`); the instrument
  store's own `sample_edits` section has no production caller (corrected
  10-30). Saved edits are applied at startup (`ApplySavedSampleEdits`,
  `cmd/beatmo.go`) BEFORE the startup demo is imported, and that import
  clears the edit of every instrument whose row in the file has none, so a
  saved Sampler edit never shows on a demo instrument after a relaunch. Under
  §5.2 that is the intended rule (the circuit's bundle on open; the saved one
  on a pick), so it is replaced rather than fixed.
- The user library stores documents only. The instrument file
  (`.beatmo-inst.json`, `audio/instrument_bundle.go` `BundleInstrument`)
  already carries `sample_edit`, `effect_chain` and `eq`.
- Shipped files (84 templates + the startup demo, 593 rows, recounted
  10-30): EQ on 5 rows, effects on 4, Sampler edits on none. No file has two
  rows on one instrument; templates use instances (`<base>--N`,
  `instrument.Instance`) when two rows need two set-ups of one instrument.

**Save and Reset today** *(verified)*:

- **Synth tab**: Preview, Save, Save As, Reset, and "Restore shipped" in the
  ⋯ menu (`ui/recipe_save_sink.go`).
  - Save writes the instrument's document to the user library under its
    identity.
  - Reset goes back to the document the open circuit carried
    (`ResetInstrumentToCircuit`).
  - Restore shipped deletes the user's library entry.
  - Save As opens a dialog prefilled with the instrument's current name
    (`openSaveAsDialog`) and always mints a NEW user instrument whose id
    hashes the name and the document (`NewUserInstrumentID`), so the same
    name with a changed sound makes a second instrument of that name.
- **Sampler tab**: Save, Save As and Reset (`ui/sampler_panel_zone.go`).
  - Save persists the edit descriptor for the instrument.
  - Save As BAKES the edit into new PCM, a new user-sample instrument whose
    name is suggested as "<name> chop".
  - Reset returns to the factory state.

**Switch path today** (`SetInstrument`): doc reset if edited → `WarmInstrument`
(7 pitches melodic, 1 otherwise) → EQ panel follow → undo snapshot (a full
project export) → the lane replaces committed notes over its horizon →
`warmFromBeatInfos` adds ±0.5 st neighbours. Desktop warms **synchronously on
the UI goroutine** (`voice_warm_desktop.go`; its comment says ~0.3 ms per
voice, the prewarmer's says ~24 ms — measured in phase 0 before anything
moves). The instrument menu's pick IS a full switch and the menu stays open,
so auditioning N instruments warms N instruments; hover does nothing.

**Caps that exist:** warm set 512, render workers 4, desktop voice cache
48 MB, JS audio queue 8 192. **No cap** on rows, nodes, insert effects per
channel or voices.

## 4. Goals, non-goals, anti-goals

**Goals**

- G1 · *Cost follows use.* Audio resources exist only for instruments a row
  uses (a pick in the menu is the audition, §5.6) and a recently-used warm set that a **memory
  budget** bounds. Boot builds none but the master bus.
- G2 · *Description ≠ resource.* An instrument's description — its document,
  name, category and its **audio bundle** (§5.2) — is plain data owned by Go
  and applied to a resource when one is built. Building, evicting and
  rebuilding a resource loses nothing and sounds sample-identical.
- G3 · *Switch at the next note that can play on time.* A row switched to an
  instrument keeps SOUNDING its previous instrument until the new one is
  ready, then changes from its first note that starts after that. No note is
  ever late, dropped or cut by a switch.
- G4 · *Bounded circuits.* A circuit holds at most N instruments (and the
  other dimensions in §6 are capped); N is chosen from load tests on
  phone- and laptop-class profiles (§9), never by feel.
- G5 · *One budget, defined in Go, enforced everywhere.* Memory budgets per
  resource class and CPU priorities live in one Go table (a sibling of
  `RuntimeProfile`); desktop Go, WASM Go and audio.js all obey it, and
  audio.js reports its usage back so one ledger sees the whole process.
- G6 · *Priority.* Audio output > note timing > recording > UI drawing >
  everything else. Under pressure the game sheds work in that order, never
  touching note timing.
- G7 · *Evidence.* Every claim above is a test: real Chrome, predictable
  synthetic load, load tests to the breaking point, soaks, judged on what is
  heard (playback probe) and on the resource ledger.
- G8 · *The instrument owns its settings; the user decides conflicts.* One
  audio bundle per instrument, shared by every row that plays it; a row
  stores none of its own. Choosing an instrument never discards or
  overwrites settings without asking (§5.2).
- G9 · *One way to save.* Save As, prefilled with the current name: the same
  name saves over the instrument, a new name makes a new one (§5.2).

**Non-goals**

- Multi-threaded Go on WASM (SharedArrayBuffer / wasm threads).
- Changing the synth engine or the sound of any instrument.
- OS thread-priority changes on desktop (revisit only if a desktop load test
  shows the audio callback starved after the budgets land).
- A real-device farm in this pass (BrowserStack minutes are exhausted; the
  profiles in §9 are calibrated stand-ins and are re-checked on devices later).
- Loading instrument descriptions on demand, or moving the browser's user
  library out of localStorage (the ceiling this leaves is stated in §5.1).
- Going back to factory or earlier ("upstream") versions of an instrument or
  circuit — the user's possible later feature. Nothing here may make it
  harder: shipped documents stay in the pack, untouched.

**Anti-goals — failure even if everything "works"**

- Any note heard later than its grid time because of a switch, an eviction,
  a budget action or UI load.
- A cap a normal user hits while making a normal song (every shipped
  template must fit with room to spare), or a cap that silently refuses.
- An evicted-and-rebuilt instrument that sounds different, even in the last
  bit (export→import audio identity must keep holding).
- Pre-loading the whole library "to be safe".
- A limit enforced on one platform only.
- Settings lost or changed by choosing an instrument without the user being
  asked — including a change to another row that plays the same instrument.
- A Save As that overwrites any instrument other than the one being edited.

## 5. Instrument residency (design)

### 5.1 Three states

| State | Holds | Enters when | Leaves when |
|---|---|---|---|
| **Known** | the description: document, name, category, audio bundle (Sampler edit included); the param block is derived from the document when first needed | the library / pack / circuit knows the instrument | never |
| **Warm** | rendered voices (per pitch the circuit needs, with the bundle's Sampler edit applied), sample PCM, the flattened block pushed to JS, NO channel | used recently, or prefetched by evidence (§5.6) | memory budget pressure (LRU), or circuit closed |
| **Live** | a channel (WebAudio chain / desktop mixer channel) materialised from the audio bundle — its EQ and insert stages (`beatmo-dsp` nodes in the browser, C state on desktop) and analyzers — plus Warm's voices | a row selects it or is still sounding it (§5.4) | no row selects or sounds it and its last voice has ended → demoted to Warm |

**Descriptions load for every instrument** (user decision): a document is
1.4–10 KB, a bundle under 1 KB, so every Known instrument's description is in
memory from boot and menus and search read it directly. Only audio is lazy.
The ceiling this leaves, stated rather than fixed in this pass: the browser
keeps the user library in localStorage (~5 MB), which holds roughly 500
(10 KB documents) to 3 500 (1.4 KB) saved instruments. The shipped pack is
embedded in the binary and has no such limit.

One owner — a **residency manager** in Go (new, `internal/audio/residency`
or similar) — decides every transition. Platforms only *acquire* and
*release* on its instruction; nothing creates a channel as a side effect of
registering metadata or of reading (today `Register` does, for every id, and
so does `getChannelChain`).

### 5.2 Audio bundles (the user's model)

**What a bundle is.** Every setting on top of the instrument's synth or
sample data, in two sides:

- **voice side — the Sampler edit** (trim, pitch, gain, reverse, normalise,
  fade), applied to the rendered voice (user decision);
- **channel side** — between the voice and the master bus: the **EQ** (10
  band gains, band mutes, high-pass and low-pass on/off and cutoff) and the
  **insert-effect chain** (type, on/off, params, order, instance uids).

Plain data, under a kilobyte, with a neutral value (no Sampler edit, flat,
no filters, no effects) and a content hash. Pan and sends no longer exist
(deleted 10-27), so they are not in it.

Not in the bundle (my call): the row's volume (a per-note gain today, not a
channel setting), mute, solo, name, colour and kit role, which say how a row
plays in this song rather than how the instrument sounds. The existing
"instrument bundle" file format (`.beatmo-inst.json`, `InstrumentBundle`) is
a different thing — a file that already carries everything the audio bundle
holds.

**Who owns it: the instrument** (user decision). In a circuit, each
instrument has exactly one bundle, and every row that plays the instrument
points to it. The row stores no mixer settings of its own: `DrumRow`'s EQ
and Effects fields are removed. The audio layer's per-channel stores (insert
slots, EQ records) become what the bundle is materialised into (§5.3), never
a second source, and the per-instrument Sampler-edit store becomes the
bundle's voice side. Editing a row's EQ, effects or Sampler settings edits
its instrument's bundle. Two rows on one instrument share it, just as they
share one channel today. Channels stay keyed by instrument.

**Where bundles live** — beside the document, in every store that holds
documents:

| Store | Bundle |
|---|---|
| shipped pack | neutral (no shipped instrument carries settings today) |
| user library | **Save As** (below) stores the whole instrument: its document and its current bundle. The saved Sampler edits (the prefs store's `sample_edits`, and the instrument store's unused section) are read into the bundles when the library loads and are no longer written (my call: nothing a user saved is lost) |
| circuit (project store) | one per instrument. **The project file's shape does not change**: each row entry's `sample_edit` / `eq` / `effects` are read as its instrument's bundle, and export writes the instrument's bundle into every row entry of that instrument. Two entries of one instrument that disagree (possible only in files saved before this change) → the last one wins, which is what import applies to the channel today, so such a file sounds exactly as it does now |
| instrument file | already carries `sample_edit`, `effect_chain` and `eq` — no format change |

**Open and undo** (my calls):

- Opening a circuit uses the **circuit's** bundles, Sampler edits included.
  A bundle the user saved in the library applies when the user *picks* that
  instrument (the rule below), never silently on open. The document rule the
  user chose on 2026-10-14 (a saved document trumps the circuit's on open) is
  unchanged.
- Undo and redo restore bundles verbatim with their snapshot.
- When no row plays an instrument any more, its circuit bundle is dropped
  along with its edited document (today's rule for documents); a later pick
  starts from the library's. Undo brings both back.

**Saving: Save As is the only save** (user decision, 10-30).

- **Removed:** Save and Reset in the Synth and Sampler tabs, "Restore
  shipped", and the code only they use (for example
  `ResetInstrumentToCircuit`, `CircuitInstrumentName`, the Sampler's
  `save` / `reset` / `ResetActiveSampler`, and the circuit state carried
  beside undo snapshots if nothing else reads it). Their tests and scenes are
  rewritten for Save As or deleted.
- **Save As** opens the existing dialog in both tabs, prefilled with the
  instrument's current name — the one the header and the row show. The
  Sampler's "<name> chop" suggestion goes (my call: one rule in both tabs).
- As the user types, the confirm button says what will happen (my call):
  - **the same name** (ignoring case and surrounding spaces) → "Save over
    <name>". The instrument's library entry is replaced by its current
    document and bundle under the SAME id, so every circuit that uses it
    plays the new version on open (the saved-document rule). Saving over a
    shipped instrument (Kick as "Kick") stores the user's version, which
    trumps the shipped one everywhere. The shipped document itself stays in
    the pack, untouched, for the later "back to factory" feature.
  - **a new name** → "Save as new". A new user instrument with the current
    document and bundle; the rows that played the instrument now play the new
    one, as Save As does today; the instrument it came from is untouched.
  - **the name of another instrument** → refused in place ("<name> is
    another instrument"), confirm disabled. Save As never overwrites an
    instrument other than the one being edited.
- **Save As never bakes audio** (my call). The Sampler edit is saved as data
  in the bundle, so the Sampler's baked-sample Save As goes; a WAV instrument
  keeps referencing its own PCM.
- Undo covers the circuit side (rows renamed or repointed), one step, as
  today. The library write is not undone.
- Unsaved edits live in the circuit: they are exported with it and undoable.
  They reach the library only through Save As.

**Choosing an instrument for a row.** Row R plays instrument Y and the user
picks instrument X:

| Y's bundle (the row's settings) | X's bundle | Result |
|---|---|---|
| neutral | anything | X's — the row has nothing to lose |
| set | neutral | the row's settings carry over to X (my call: dropping them would lose the user's work, and X brings nothing to choose against) |
| set | the same as Y's | nothing to choose |
| set | set, different | **the user is asked**: "Keep this row's settings" or "Use X's" |

X's bundle is its circuit bundle when another row already plays X, else its
library bundle (neutral for shipped instruments the user never saved over).
So browsing the shipped library never asks; the question comes up with saved
or circuit instruments that carry their own settings.

**Never change another row** (my call). When another row already plays X,
keeping R's settings would change that row's sound too. In that case R gets
its own copy of X: an instance (`<X>--N`, the mechanism templates already
use for two set-ups of one instrument) with X's document and R's bundle, on
its own channel. The other row is untouched. The question's wording names
this ("…a separate copy of X for this row").

**The question** (my calls on the details):

- It is a must-answer card stacked on the instrument menu — the existing
  pattern for a picker stacked on an open menu (`MenuCard`,
  `menuCardEntry` with `mustAnswer`). Each choice lists what it keeps (for
  example "Sampler edit, EQ, 2 effects").
- Esc or a tap outside cancels the pick: the row stays on Y and nothing
  changes.
- The pick and the answer are one undo step and one log line.
- The answer covers that pick only. There is no "remember my choice" in this
  pass.
- Until the answer, the row keeps sounding Y. The switch rule (§5.4) starts
  from the answer.
- An instrument-file import into a row follows the same table.

**Cost.** A bundle is description (Known). Its voice side costs nothing
until voices render: the Sampler edit is applied to Warm voices (it is
already in the voice-cache key, so two instruments with the same document and
edit share renders). Its channel side — the EQ and insert stages — exists
only while the instrument is Live. Each stage runs every block while it
exists, even in silence, so a non-Live instrument has none, and a Live one
has a stage only while that stage does something (§5.3).

### 5.3 Materialise from the description

A channel is built by one function from the channel side of the
instrument's bundle, the same function import and undo use, so a rebuilt
channel is identical to one that never went away. Voices are rendered with
the bundle's Sampler edit, as they are today. Every mixer setter writes the
bundle and, if the channel is Live, applies the change in place (the C
stages' `set` / program-update paths). `pendingChannelOps` (queued chain
writes before the AudioContext exists) disappears: the bundle is applied when
the chain is materialised.

Concretely, from §3:

- Registering an instrument creates nothing but its description.
- No read creates a resource: `ChannelVolume`, `getChannelChain`, analyser
  snapshots and document lookups answer from the description or report "no
  channel".
- The EQ and the insert chain are read from the bundle, not from the audio
  layer. The export reads the bundle, so the two sources of §3 become one.
- A stage exists only while it does something: an emptied insert chain or a
  flat EQ removes its `beatmo-dsp` node (today the inserts node stays and
  runs every quantum). A flat EQ stays ABSENT, never a unity chain, which
  keeps export→import bit-exact (CLAUDE.md: a unity biquad chain differs from
  an absent one in the last bits).
- The flattened param block is pushed to JS when the instrument becomes
  Warm, not at boot for every id.
- User-sample PCM loads on first use, with one copy per side.
- Import and row delete release what the new state no longer uses.

### 5.4 Switch semantics (G3)

The row keeps two instruments: **selected** (what the UI shows, what is
saved) and **sounding** (what `laneNoteAt` plays). A switch — once any
bundle question (§5.2) is answered — sets selected at once, asks the
residency manager to make the new instrument Live for the pitches the row's
upcoming notes use (the predictor knows them), and leaves sounding
unchanged. When the manager reports ready at time R, sounding changes for
notes whose start is after R + `CancelGuardSec`; the lane's reconcile
replaces any already-committed notes after that point through its atomic
Replace. Notes within the guard keep the old voice. A switch back before the
new one is ready cancels the pending switch. Export always writes
**selected**. A switch to a new instance (§5.2) is the same rule.

Measured on 10-25: ready in 42–80 ms during playback, so at 120 BPM
sixteenths (125 ms) almost every switch lands on the very next note; the
rule only matters when a note is due inside the render time (the organ case,
which then played 16 ms late). Phase 0 re-measures it on the new pipeline.

### 5.5 Memory budget and eviction

Warm voices are accounted in bytes (AudioBuffer memory is not in any JS heap
figure, so audio.js counts it itself). When the warm set exceeds its budget,
least-recently-used instruments are evicted. Never evicted: anything Live,
any voice a committed lane note references, the voices for the next
`AudioHorizonSec` of every row.

**Source audio is data, not cache** (my call). A user sample's PCM (a WAV the
user loaded, or one embedded in an imported project and held only in memory)
is the instrument's description, so it is never evicted while the circuit,
the library or an undo snapshot references it; it counts in the ledger, and
only its derived renders are evictable. Nothing new is written to storage.

### 5.6 Instrument menu

Browsing renders nothing and builds no channel. The menu has no separate
preview: a pick IS the audition, a full switch of the row with the menu left
open (§3), so a previewed instrument is simply the row's instrument and needs
no preview channel (corrected 10-30).
Pre-loading the rows under the pointer is added **only if** the load test
shows cold preview p95 above a threshold the user agrees with (proposal:
50 ms from tap to sound); the evidence decides, and the bench that decides
stays in the suite.

## 6. Bounded circuits

- **Instruments per circuit: at most N** (instances count: each has its own
  channel). N comes from the load test (§9.3): the largest instrument count
  at which the reference circuit plays with zero late, missed or dropped
  notes on the *low phone* profile, with margin. Floor: every shipped
  template fits (templates are capped at 12 rows today,
  `TestTemplatesAreReasonablySized`).
- **One cap for every device** (my call): sized for the *low phone*
  profile, so a file made on a laptop always opens on a phone. N = 80 % of
  the largest count that passes every §9.3 gate on that profile. If that
  falls below the template floor (12), the floor wins and the shortfall is
  reported as an engine/profile problem to fix, not hidden by a smaller cap.
- Other unbounded dimensions get the same treatment, each with its own
  measured cap: insert effects per bundle, nodes per circuit, undo history
  bytes. **No polyphony cap** in this pass (my call): enforcing one means
  stealing or refusing a note, both anti-goals; voices are bounded by rows ×
  note density, and the load test measures them.
- **Never silent.** At a cap the control that would exceed it says so (the
  "Add instrument" strip shows the limit; the FX "Add Effect" row likewise;
  a pick that would create an instance past the cap offers only "Use X's").
  An imported file over a cap is refused with a message naming the cap —
  before anything resets, like every other import rejection (my call).
- The caps live in the same Go table as the budgets (§7), so desktop and
  browser agree and a test enumerates them.

## 7. One resource budget (CPU and memory)

### 7.1 The table

One Go table (sibling of `RuntimeProfile`, chosen per platform and device
class) holds:

- **Memory budgets per resource class**: warm voices + sample PCM (the
  instrument cache), live channels and their stages, analyzer/scope buffers,
  synth-graph cache, undo history, recording outbox, Go heap soft limit
  (`debug.SetMemoryLimit`, today a flat 1 500 MB on WASM).
- **Caps** (§6).
- **CPU classes**, in priority order: `audio` (never throttled), `timing`
  (sequencer + audio lane), `record`, `draw`, `interactive` (edits, menus),
  `background` (prewarm beyond the horizon, synth-graph analysis, dial
  relevance, persistence).

Device class comes from what the platform reports (browser:
`navigator.deviceMemory`, `hardwareConcurrency`, mobile layout; desktop:
`runtime.NumCPU`, system memory) with floors, never above the class's
ceiling.

### 7.2 One ledger, both sides

Every resource class reports its bytes and counts to one Go ledger. audio.js
keeps its own counters (it alone knows AudioBuffer bytes, chains,
`beatmo-dsp` nodes — `audioGraphStats` is the start) and reports them in the
existing per-frame bridge snapshot, cheaply. The ledger is a JS export
(`resourceLedger()`) and a hook event for logs. Eviction decisions are made
in Go from the ledger and executed by whichever side owns the resource.

### 7.3 Audio health and the step-down ladder

An always-on, cheap **audio health** reading combines: how far ahead the
lane has notes committed (its minimum over the last second), render waits on
notes inside the horizon, main-thread stalls, and on desktop the mixer
callback's time against its deadline. A production-safe audio-thread load
meter (a tiny worklet timing its own quanta, or `AudioContext.renderCapacity`
where Chrome provides it) covers the browser's audio thread.

When health drops, work is shed in this order, with hysteresis on the way
back:

1. pause `background` (graph analysis, dial relevance, prewarm beyond the
   horizon, coalesce persistence);
2. lower the draw rate (skip frames to a floor, e.g. 30 → 20 fps);
3. simplify visuals (glow, live analyzers and scope off).

`timing` and `audio` are never shed. On WASM, long Go tasks in lower classes
run in time slices from a per-frame budget (generalising the synth graph's
`yieldEvery`), so they cannot hold the one thread past the frame.

**When protection fails anyway** — the page frozen longer than the prepared
second of audio, a background tab — the lane keeps today's rule (my call;
the user did not answer this one): a note found late plays if it is at most
`LateToleranceSec` (25 ms) late and is skipped beyond that. Every such note
is counted in the audio health reading and fails the §9.3 gates, so it is
never an accepted outcome inside the limits.

### 7.4 Browser-side priority

The render workers get the same classes: renders for notes inside the
horizon first, then a pending switch or preview, then warm-ups, then graph
renders (today one FIFO per worker serves all of them).

## 8. Effect delay compensation (proposal — the user has not decided)

An insert chain declares its latency (the pitch shifter: up to its window;
every other effect: 0). The lane starts that row's notes earlier by the
declared latency (it already commits 0.25–1 s ahead), so what is heard lands
on the grid. On 10-25 two shifters put the startup demo's kick ~45 ms behind
the beat (measured p99 43–45 ms, max 50 ms). Desktop and browser identical
(both now run the same C pitch shifter); the probe verifies onsets land on the
grid on such a channel.

## 9. Testing and evidence

### 9.1 Always in a real browser, always in isolation

Every timing or memory claim is checked in **real Chrome** (`TEST_PLATFORM=
chrome-local TEST_HEADED=1`), serving a **private build** (never the shared
`src/js/main.wasm`), with the host's load recorded and runs discarded above
a threshold (as `make bench-lane-ab` does). Headless runs keep only the
structural gates (counts, bytes, silent/dropped notes); timing gates are
enforced in the real-Chrome runs. Sample-exactness across platforms stays
with `xplat_audio_stages.browser.test.js`, extended with an evict-rebuild
case.

### 9.2 Predictable load

Chrome's CPU throttle slows only the page's main thread (measured: 3× on
main, ~1× on workers and the audio thread), so the load harness adds its own
calibrated burners:

- **main thread**: CDP `setCPUThrottlingRate` + the isolation bench's stall
  injector;
- **render workers**: a burner occupying a set fraction of each worker;
- **audio thread**: a burner worklet consuming a set fraction of every
  render quantum;
- **memory**: ballast (held ArrayBuffers + Go allocations) and a capped
  `deviceMemory`.

Profiles (stand-ins until devices are measured; numbers are a starting
proposal): *laptop* (1× main, no burners), *mid phone* (4× main, workers and
audio thread 50 % busy, 4 GB), *low phone* (6× main, 65 % busy, 2 GB).

### 9.3 Load tests (to the breaking point), not only soaks

Each load test raises one dimension in steps until something gives,
records the breaking point, and asserts that inside the limits nothing
audible did:

- instruments in the circuit (find N for §6), with and without effects and
  EQ on each;
- library size 100 → 5 000 instruments (boot time, memory, chain count must
  stay flat);
- switch storm during playback (G3: zero late/dropped/cut notes; heard
  onsets switch on the first possible note);
- menu browse + preview storm;
- edit storm (the live-edit budget phases) under each profile;
- effects per bundle on Live instruments (the audio-thread cost of the
  `beatmo-dsp` stages);
- BPM / notes per second.

Gates per step (my call on how "never late" is tested): every note's
**scheduled** start (`BufferSource.start(when)` / the desktop mixer's start
sample) equals its grid time exactly — sample-accurate, after any declared
effect-delay compensation; the **heard** onset sits within the detector's
tolerance of it (today the probe's 20 ms window; tightened to the measured
anti-pop ramp + detector latency in phase 0); no missed, dropped or cut
notes; ledger within budget; frame rate above the floor or the ladder
engaged; no page error.

### 9.4 Soaks

A long session (switches, previews, edits, imports, Save As) with the ledger
sampled: bytes plateau, chain count = rows + warm set, `beatmo-dsp` nodes =
non-neutral stages of Live instruments, Go heap plateau, no growth per cycle.

### 9.5 Deterministic Go tests

- The residency state machine; the switch rule (fake clock through the lane:
  sounding changes exactly at the first note after ready + guard); eviction
  order and the never-evict set; the ledger's accounting; the caps on every
  entry path (add row, import, template, instance creation).
- Bundle → channel materialisation round trip: build, evict, rebuild gives an
  identical processor chain; the export→import audio-identity tests are
  extended with an evict-rebuild step.
- **Bundles, through real input (`Game.Update`):**
  - every row of the §5.2 pick table, including the question's two answers
    and its cancel (cancel leaves the export byte-identical);
  - a pick that creates an instance leaves the other row's rendered audio
    byte-identical;
  - undo carries the whole bundle, Sampler edit included;
  - a library saved before the change loads its `sample_edits` into the
    bundles with nothing lost;
  - a saved Sampler edit on a startup-demo instrument is still in the
    library after a relaunch and applies when that instrument is picked;
  - a row stores no mixer field: an AST guard keeps the removed `DrumRow`
    fields from returning, and every mixer writer goes through the bundle.
- **Save As, through real input, in both tabs:**
  - the prefill is the current name;
  - the same name saves over the instrument under the same id, and a circuit
    using it plays the new version on reopen;
  - a new name creates an instrument and leaves the original's library entry
    byte-identical;
  - another instrument's name is refused, with nothing written;
  - saving over a shipped instrument leaves the pack untouched;
  - nothing is baked: no new PCM after a Sampler Save As;
  - guards that Save, Reset and Restore shipped are gone (no button, no
    export, no i18n key in use).
- **Files:** every shipped template and the startup demo still round-trip
  export→import byte-identical (the file shape is unchanged). A pre-change
  file with two disagreeing entries for one instrument renders exactly as the
  current code renders it.
- **UI:** the question card and the Save As dialog's three button states have
  screenshot scenes (desktop + mobile), opened and inspected per CLAUDE.md;
  the card gets a row in `TestEveryMenuClosesTheSameWay`.

## 10. Build order

0. **Measure first** (no behaviour change): the resource ledger (§7.2) and
   `audioGraphStats` extensions, the load harness with its burners and
   profiles (§9.2), and baseline runs of every §9.3 load test on today's
   code — the post-10-27 pipeline. Includes the desktop warm cost (0.3 vs
   24 ms per voice).
1. **Audio bundles and decoupling** (§5.2, §5.3), in three steps, each
   keeping export→import audio identity, `xplat_audio_stages` and the
   live-edit budget green:
   a. the bundle type and the per-instrument circuit store as the single
      source; `DrumRow`'s mixer fields removed; one materialise function;
      registration and reads stop creating resources; stages exist only
      while they do something (no audible change);
   b. the user library carries the bundle (absorbing `sample_edits`); Save As
      as the only save in both tabs; Save, Reset, Restore shipped and the
      Sampler's baking removed (UI work with its screenshot scenes);
   c. the pick table, the question card and instance creation (UI work with
      its screenshot scenes).
2. **Residency** (§5.1, §5.5): the manager, lazy Live/Warm, LRU eviction
   under the memory budget, release on row delete / import / circuit close.
3. **Switch rule** (§5.4) with the fake-clock lane test and the real-Chrome
   switch-storm load test.
4. **Caps** (§6) at the measured N, with UI messages and import refusal.
5. **Budget governor** (§7): health reading, step-down ladder, worker
   priority, WASM time slices, Go memory limit from the device class.
6. **Effect delay compensation** (§8) — only if the user approves it.
7. Menu prefetch (§5.6) — only if phase 0/2 numbers say cold preview is too
   slow.

Each phase ends with the §9 runs in real Chrome under all three profiles,
compared with the phase-0 baseline.

## 11. Decisions

**The user's:** §1 in full. In particular:

- the switch takes effect at the next note;
- a memory budget, not an instrument count;
- lazy menu loading, prefetch only on evidence;
- bounded circuits sized for normal phones and laptops;
- the instrument owns its EQ, effects and Sampler settings as one audio
  bundle that its rows point to, and a conflict on a pick is the user's
  choice;
- descriptions load for every instrument, and only audio loads lazily;
- no Save, no Reset: Save As prefilled with the current name; the same name
  saves over the instrument, a new name makes a new one; factory/upstream
  versions maybe later, not now.

**Calls made in this document (check these):**

- *Bundles (§5.2):*
  - row volume, mute, solo, name, colour and kit role stay out of the
    bundle;
  - opening a circuit uses the circuit's bundles (Sampler edits included),
    and a saved bundle applies only when the instrument is picked (the
    saved-*document* rule on open is unchanged);
  - the old `sample_edits` store section is read into bundles on load;
  - when the row has settings and the picked instrument has none, the row's
    settings carry over without a question;
  - a pick never changes another row: keeping the row's settings on an
    instrument another row already plays creates an instance;
  - the question: cancel keeps everything, one undo step, no "remember my
    choice";
  - the project file keeps its shape, and the last entry wins when an older
    file disagrees with itself.
- *Save As (§5.2):*
  - the same name means the same name ignoring case and surrounding spaces;
  - the confirm button says what will happen ("Save over <name>" / "Save as
    new");
  - typing another instrument's name is refused;
  - the Sampler tab uses the same prefill (its "<name> chop" suggestion
    goes);
  - nothing is baked: a Sampler edit is saved as data;
  - the library write is not undoable (the circuit side is, as today).
- *Timing:*
  - a cold instrument not ready by its next note keeps the old sound until
    it is ready (§5.4) — never late, never silent;
  - when protection fails (a frozen page, a background tab), a late note
    keeps today's lane rule: played if at most 25 ms late, skipped beyond
    that, and counted as a failure (§7.3). **The user has not answered this
    one** — zero tolerance (skip anything late) is the alternative;
  - "never late" is gated as: scheduled start exactly on the grid, heard
    onset within the detector's measured tolerance (§9.3).
- *Caps:*
  - one cap for every device, sized for the low-phone profile at 80 % of the
    largest passing count; the 12-row template floor wins over the
    measurement (§6);
  - an imported file over a cap is refused with a message (§6);
  - no polyphony cap in this pass (§6).
- *Other:*
  - the three load profiles and their numbers (§9.2) are stand-ins until
    real devices are measured;
  - recording ranks just below audio and timing (§4 G6);
  - source audio (user samples) is data, never evicted while referenced;
    only derived renders are evictable (§5.5);
  - the cold-preview threshold for prefetch is proposed at 50 ms (§5.6).

**Still open:** effect delay compensation (§8) is a proposal, built only if
the user approves it.

**Left to the builder:**

- package layout and names (the bundle type, the residency manager);
- how the ledger snapshot rides the existing bridge;
- the burner implementations;
- exact hysteresis values;
- the order of evictions within the warm set beyond LRU;
- the instance's display name;
- the exact wording of the Save As button and refusal;
- test file names.
