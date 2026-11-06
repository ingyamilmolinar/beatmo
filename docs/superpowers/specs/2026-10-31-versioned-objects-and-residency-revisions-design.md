# Versioned game objects, and the simpler residency design

Date: 2026-10-31 · Status: DRAFT, not approved. Amends
`docs/superpowers/specs/2026-10-25-instrument-residency-and-resource-budgets-design.md`
(revision 4, "the residency spec" below). Where this document and the
residency spec disagree, this document wins. Sections of the residency spec
not named here stand as written.

The four plans of 10-30 (`docs/superpowers/plans/2026-10-30-residency-{a,b,c,d}-*.md`)
are superseded where they conflict with this document and are re-planned from
it: A is slimmed, B is replaced by Part 2, C absorbs B's channel work, and D
waits for evidence.

## 0. Why

The residency spec's direction stands: audio resources cost only while used,
circuits are bounded, the instrument owns its settings, and Save As is the
only save. A review on 10-31 found it rebuilt infrastructure that already
exists, risked cutting effect tails, put audio switching state on the drum
view, and built a CPU governor before any evidence asked for one. The user
agreed to the simpler designs, with these instructions (their words,
condensed):

1. "Keep audio and drum view cleanly separated."
2. "Never drop or affect tails or any other part of the sound."
3. "Gather some evidence and build something simple and isolated."
4. "We should never drop or change users settings."
5. Every entity and config (circuits, instrument configs, FX, audio
   bundles…) is **versioned**, with a configurable limit on how many versions
   are kept locally. Going to a version in the past or the future should be
   straightforward. A version is **baked only when the user saves**; real-time
   edits are undone and redone through the existing undo/redo.
6. Versioning exists because these objects **will be shared** between users,
   with or without the circuits they publish. Authors will publish versions
   that may or may not replace versions others consume, and users **pin**
   circuits and game state to exact configs so the sound never changes under
   them (everything is deterministic and config-driven).
7. When a client later fetches newer versions of instruments it uses, **the
   user is told and confirms**; a user preference can skip the question. The
   user is always aware of any change, or opts into following updates.
8. **No publishing, fetching or update prompt is built now.** This effort
   makes the internal data model right so those are easy later. Versioning
   itself is not user-visible.
9. Circuits are objects that behave the same way. The circuit is split from
   its instruments; instruments point to audio bundles (Sampler edit, EQ,
   FX). **The circuit can also point to bundles**, with a setting per bundle
   to override the instrument's, merge with it (both have FX chains, for
   example), or ignore the circuit's or the instrument's. No UI for this now.
10. **Save and Reset are removed; only Save As stays** (the residency spec's
    user decision, confirmed 10-31). This work complements the residency
    spec's vision.
11. Opening a circuit that pins an older version of an instrument the user
    has since saved plays **the newest version's sound** (10-31; today's
    10-14 rule). The circuit **keeps its own EQ, FX and Sampler settings** for
    each instrument and each copy (10-31, "sound only").
12. Circuits are versioned objects in the model now, but **no circuit history
    is recorded and the file gains no circuit id** until a circuit save or
    library exists (10-31, "model only for now").

## Part 1 · Residency, simplified

### 1.1 Evidence first (replaces residency spec §7.2 and the ledger of §10 step 0)

- **One JS stats export.** Extend `window.audioGraphStats()` (audio.js:2293)
  with the counters residency needs: live channels, live `beatmo-dsp` stages,
  voice-cache entries and bytes, AudioBuffer bytes, pushed param blocks. No
  parallel `audioResourceStats()`.
- **Go counts go into `perfStats()`** (or a `__` test seam if they are
  test-only): registered instruments, channels the mixer holds, voice-cache
  bytes on desktop. There is no merged Go+JS ledger and no TTL bridge cache:
  nothing at runtime decides from a ledger. Go owns the policy constants (the
  budgets), audio.js applies its own LRU against them (10-30 decision 4).
- **Gates reuse the existing helpers.** New limits go into
  `audio_probe_bench_helpers.js` beside `PROBE_LIMITS_DEFAULT` /
  `probeGateFailures`; the `/proc/loadavg` discard in `scripts/audio_lane_ab.mjs`
  moves into those helpers and both use it. No new `load_gate.js`.
- **Only the measurements that feed a decision:** instruments and
  instruments-with-FX (the instrument cap), effects and DSP nodes (their
  caps), switch (the switch rule), library size (boot stays flat as the
  library grows). Preview is the same as switch (a pick is the audition);
  live edits are already gated by `webaudio_live_edit_budget`; BPM feeds no
  decision. Memory ballast and the `deviceMemory` override wait for Part 3.
- The baseline runs on today's code before anything in Part 1 or Part 2
  changes behaviour.

### 1.2 Channels: acquire on use, release only after silence (replaces residency spec §5.1's "Live → Warm when its last voice has ended")

- **The held set** is the set of instruments any row plays. It comes from the
  same function the warm-up controller (`warmup_controller.go`) uses to know
  which instruments the rows use; one function serves both.
- A small manager compares the held set with the live channels after every
  change: it **acquires** a channel for an instrument that became held (built
  by one materialise function from the instrument's effective bundle, §2.6),
  and **marks for release** a channel whose instrument left the held set.
- **A channel marked for release is released only once nothing in it can
  still sound.** Output silence alone is not proof: a 1000 ms delay at full
  wet is silent between echoes. So the release looks inside the chain (my
  call): every stage reports whether its internal state still holds energy
  above −90 dBFS (delay lines, reverb tanks, filter state), and the channel is
  released only after its output AND every stage have been below that for a
  hold of 250 ms. This is measured where the stages run: the `beatmo-dsp`
  worklet in the browser, the mixer's channel block on desktop. A reverb or
  delay tail therefore always rings out in full. The hold also stops A→B→A
  switching from tearing a channel down and rebuilding it. Re-acquiring before
  release cancels the release.
- **A rebuilt channel equals a freshly built one** (my call). After release
  there is nothing left to hear, so the next acquire starts every stage from
  its initial state, including a modulation effect's LFO phase, exactly like
  a channel created when the circuit opens. Today a live channel's LFO phase
  already depends on how long the session has run, so no deterministic
  property is lost; offline renders reset state as they do now
  (`audio.ResetDSPState`).
- There is no Known/Warm/Live enum. "Live" is "has a channel"; "Warm" is "the
  voice cache has its renders", which the cache already tracks under its byte
  budget. Tests check whether the channel exists.
- **Lazy channels and stages** (formerly plan B task 3) live here: boot builds
  only the master bus; registering an instrument and every read (
  `ChannelVolume`, `getChannelChain`, analyser snapshots) create nothing;
  a stage exists only while it does something; a flat EQ stays absent, never
  a unity chain (export→import bit-exactness). The residency spec §5.3 list
  applies, built here.
- The voice-cache eviction of residency spec §5.5 stands, simplified: audio.js
  keeps bytes and last use per render and evicts least-recently-used renders
  over the budget, never a render of a held instrument and never one a
  committed lane note references. User-sample PCM is data and never evicted
  while referenced (as specified).
- **Lazy param-block push** (the block pushed to JS when an instrument is
  first held instead of for every id at boot) is built only if the 1.1 library
  measurement shows boot cost growing with library size.

### 1.3 Switching lives in the audio lane (replaces residency spec §5.4)

- The drum view knows only **the instrument a row plays** (what the UI shows,
  what is saved). It holds no "sounding" or pending-switch state, so export,
  undo, row delete and import need no knowledge of switching.
- The lane enforces one rule: **it never commits or replaces a note with a
  voice that is not rendered.** Until the new instrument's voice for a note is
  ready, the lane keeps the instrument it last committed for that row
  (`lastHeard[row]`, lane-local, reset on stop and seek like the rest of the
  lane's state) and commits the note with that voice. Once the voice is ready,
  the existing near-window recommit (`voiceStale` / `voiceDone`) replaces the
  notes that have not started, through the atomic Replace.
- No note is late, dropped or cut by a switch; a note already started is never
  touched (CancelGuardSec, as today).

### 1.4 Not changed by this effort

- The late-note rule (residency spec §7.3: play if at most 25 ms late, skip
  beyond) is unchanged. Changing it is still the user's open decision and is
  not part of any task here.
- Effect delay compensation and menu prefetch are not planned (10-30
  decision 5).

## Part 2 · Versioned objects, bundles and Save As (replaces residency spec §5.2's storage, pick and save rules where named)

### 2.1 Three kinds of object

| Kind | Holds | Points to |
|---|---|---|
| **Bundle** | the settings on top of a sound: Sampler edit, EQ (band gains, band mutes, high-pass / low-pass on/off and cutoff), insert-effect chain (type, on/off, params, order, instance uids). Each part optional | nothing |
| **Instrument** | the instrument document (`instrument.Doc`, the whole sound) | one bundle version, or none |
| **Circuit** | the graph, rows (name, colour, volume, mute, solo, kit role), tempo, subdivision, master volume, master EQ, kits, groups, theme, pins | for each instrument it uses: an instrument version, and optionally a circuit bundle version with a combine mode (§2.5) |

**Sample instruments** (WAV uploads, `user.sample.*`, catalogue samples)
have no `instrument.Doc`. Their instrument version holds a reference to the
PCM by its SHA-256 (the `HashSamplePCM` / `wav_ref` hash that already exists)
in place of a document, plus the bundle (my call). The PCM stays in the
existing sample store, never in the object store. Save As on a sample
instrument makes a version that points to the same PCM hash with the current
bundle.

What stays out of the bundle is as the residency spec decided: row volume,
mute, solo, name, colour and kit role describe how a row plays in this song.
The master EQ stays a circuit field (my call; it can become a circuit-level
bundle later without changing the model).

The circuit's per-instrument entries match the mixer's unit: one channel per
instrument, so every row that plays an instrument shares its circuit entry,
as rows share one channel today.

### 2.2 Versions are content hashes

- **A version is the SHA-256 digest of the object's canonical JSON** (Go's
  `encoding/json` of the struct: fields in declaration order, map keys
  sorted, the same canonical form `instrument.ContentHash` relies on). Written
  `sha256:<64 hex>`. It is the same digest the backend spec's CIDv1
  (`2026-10-24-p2p-groups-and-backend-design.md`: raw codec, sha2-256) wraps,
  so a version converts to a shareable address with no re-hashing.
- **The digest excludes identity** (the id and origin fields), as
  `ContentHash` does today: the same content saved under two names is one
  version. It also excludes **effect instance uids** (my call): they are
  per-session counters (`insertFXUIDs`, `effect_chain.go`) that are not in
  the project file. The `Bundle` value carries them for the live engine, but
  outside its canonical form. A version is immutable, identical on every machine and platform,
  and free to verify.
- A referencing object stores the digest of what it references, so an
  instrument version fixes its bundle exactly, and a circuit version fixes
  every sound in it.
- `instrument.ContentHash`'s truncated 16-hex form stays what it is today: the
  key of the shipped/pinned pack documents that built-in templates reference
  (`{"ref":"sha256:…"}`). It is not a version id (64 bits is too short for a
  shared namespace). Unifying the two is left for the publishing work.

### 2.3 Identity, history and the current pointer

- **Identity** names an object across its versions. Instruments keep
  `instrument.ID` (roster ids, `user.<slug>.<hash>`, instances, sample ids).
  Bundles have no identity of their own in this effort: they are reached
  through the instrument or circuit that points to them. **Circuits have a
  version digest but no identity or history in this effort** (user, 10-31).
  The circuit object kind, its canonical form and its digest exist and are
  tested. Identity, history and a `circuit_id` in the file come with a
  circuit save or library.
- **History** = for each identity, the list of its saved versions in save
  order, plus a **current** pointer to one of them. Moving the pointer to an
  older version keeps the newer ones; saving after moving back appends a new
  version at the end. So going to any retained version, past or future, is
  moving one pointer. No UI does that in this effort.
- **Shipped instruments are history too:** the pack document is version 0 of
  its roster id, kept in the binary, never pruned. Saving over Kick appends a
  user version and moves Kick's pointer; the later "back to factory" feature is
  moving the pointer back to version 0.
- **Only a save makes a version** (§2.7). Every edit between saves is
  circuit working state, covered by undo/redo exactly as today.

### 2.4 The local store

- One content-addressed **object store** (digest → kind + canonical bytes) and
  one **history table** (identity → versions + current), persisted through
  the existing userprefs pattern: desktop a JSON file beside
  `instruments.json`, browser a localStorage key beside
  `beatmo.instruments` (store files `objects*.go` mirroring
  `instrument_store*.go`). The user instrument library becomes a view of this
  store: an instrument's library entry is its current version.
- **The existing user library is read into it on first load** (my call). Each
  saved document becomes version 1 of its id. A saved Sampler edit (prefs
  `sample_edits`) joins that version's bundle **only when its instrument has
  a saved library document**. An edit with no saved document (keyed by a
  roster id or an instance) creates no version, because a new version would
  start replacing that instrument in every circuit opened (§2.8). It stays
  in its old section, unread, and is never deleted. The persisted user
  samples registered over built-in ids (`audio/user_samples.go`) are
  untouched by this effort. The user's saved instruments are never lost, and
  nothing they hear changes.
- **Retention:** keep the newest N versions per identity (one preference,
  my call: default 20). Never pruned: the current version, shipped version 0,
  and any object a retained version still points to. Pruning is a
  mark-and-sweep from the retained versions. It runs after a save.
- Pruning cannot break a circuit file: the file carries every object it uses
  (§2.8).
- A write that fails (localStorage quota) leaves the previous store intact.
  A **Save As that cannot persist tells the user**, through the existing
  error notification (`notifyError`, the logs panel), and the rows keep
  playing what they played. A save that silently vanishes on reload would
  lose a saved instrument (my call). The
  localStorage ceiling of residency spec §5.1 applies to this store; versions
  share bytes through the digest (an instrument version whose document did not
  change stores no second document).

### 2.5 Circuit bundles and combine modes (data model only, no UI)

A circuit's entry for an instrument may point to a **circuit bundle** with a
**mode**:

| Mode | Effective bundle |
|---|---|
| `instrument` (default; the only mode in use in this effort) | the instrument's bundle; the circuit bundle is ignored |
| `circuit` | the circuit bundle; the instrument's is ignored |
| `override` | part by part: the circuit bundle's part where it has one, else the instrument's |
| `merge` | FX: the instrument's chain, then the circuit's. Sampler edit and EQ: as `override` (my call: merging two edits or two EQs has no single meaning, and two EQ stages per channel would be engine work) |

- One pure function, `Effective(instrumentBundle, circuitBundle, mode)`, is
  the only place the combination happens, and the only way the audio layer
  receives a bundle. It is unit-tested for every mode and part.
- No UI creates a circuit bundle in this effort, so every circuit entry is
  `instrument` with no circuit bundle, and what is heard is exactly the
  instrument's bundle.
- `mode` is a closed set, an `internal/enum` enum with strict decoding.

### 2.6 The bundle is a value over the existing audio stores

The live settings stay where live edits write them today, so no setter is
rewired:

- insert slots in `effectChainManager` (`effect_chain.go`), the Sampler edit
  in `sampleEdits` (`sample_edit_descriptor.go`), plus one new per-instrument
  store of EQ **settings** (gains, mutes, filters — the audio layer's
  `eqRecord` holds only compiled bands).
- `CurrentBundle(id)` reads those three into a `Bundle` value;
  `ApplyBundle(id, b)` writes them (import, undo, pick, materialise). There is
  no second bundle store, no `Hash()` method beyond the version digest, and no
  `BundleRevision` counter.
- **No library fallback when reading a circuit's bundle.** A circuit entry has
  its bundle or none; the library's bundle reaches a circuit only by a pick
  (§2.9) or by opening (§2.8), never as a silent fallback for a missing entry.
- `DrumRow`'s seven mixer fields go (residency spec §5.2): a row reads its
  instrument's settings.

### 2.7 Save As is the only save

As the residency spec §5.2 specifies, with these changes:

- **Save and Reset are removed** from the Synth and Sampler tabs, with
  "Restore shipped" and the code only they use (`ResetInstrumentToCircuit`,
  `CircuitInstrumentName`, the Sampler's save/reset, `ResetActiveSampler`,
  and the circuit state carried beside undo snapshots if nothing else reads
  it).
- **Save As appends a version.** The confirm button reads by what was typed
  (10-30 decision 2):
  - the name of the library instrument the current one comes from (its
    `InstrumentLibraryIdentity`) → "Save over <name>": a new instrument
    version (current document + current bundle) appended to that identity's
    history and made current. Nothing is overwritten; the previous version is
    kept.
  - a new name → "Save as new": a new identity whose history starts with this
    version; the rows that played the instrument play the new one, as today.
  - another instrument's name, compared against display names **in every
    locale** → refused in place.
- The existing save paths are kept and changed, not rewritten: "Save over" is
  today's `saveRecipeForInstrument` appending a version instead of replacing
  the entry; "Save as new" is today's `SaveActiveRecipeAsFor`. The dialog
  classifies the name and calls one of them.
- Saving bakes no audio (a Sampler edit is bundle data). The library write is
  not undoable; the circuit side (rows renamed or repointed) is one undo step,
  as today.
- Exporting records nothing in the local store (instruction 12).

### 2.8 Opening a circuit, and the project file

- **Opening plays the newest version's sound** (user, 10-31). For each
  instrument the circuit uses, if the user's history has a current version
  for its identity (`InstrumentLibraryIdentity`, so instances `kick--N` match
  `kick`, as today), that version's **document** plays instead of the
  circuit's copy. This is today's 10-14 rule, unchanged.
- **The circuit keeps its own bundle** for every instrument and every
  instance (user, 10-31, "sound only"). A saved version's bundle reaches a
  circuit only when the user picks the instrument (§2.9). So two set-ups of
  one instrument in a template stay distinct, and nothing the circuit holds
  is replaced on open.
- "Has a current version" means **the current pointer is on a user-saved
  version** (my call). The shipped version 0 is in every roster id's history,
  but a pointer on version 0 means "no user version": the circuit's copy
  plays, as it does today for an instrument the user never saved. That covers
  the later "back to factory" feature too.
- This runs through one resolver with a policy, `follow current` in this
  effort. The later update prompt is a second policy (`ask`), and the user
  preference to skip asking selects `follow current`; neither the prompt nor
  the preference is built now.
- **The project file keeps its shape.** Today's per-instrument `eq`,
  `effects` and `sample_edit` are the instrument's bundle; `instrument_docs`
  are the documents. Every version is computable from the file (§2.2), so
  pins need no new field. The only new optional fields are a circuit bundle
  and its mode per instrument. They are written only when set, and nothing
  sets them in this effort, so they are absent from every file. Every shipped template, the startup demo and every
  export stay byte-identical, and all export→import identity tests hold
  unchanged.
- The file stays complete on its own: it carries every document and bundle it
  uses, so it plays on a clean machine and is unaffected by local pruning.

### 2.9 Choosing an instrument never drops settings

The residency spec's pick table stands (§5.2 "Choosing an instrument for a
row"), with two changes:

- **No instance fork** (the simpler design agreed 10-31). When another row
  already plays the picked instrument, the question says so: "Keep this row's
  settings (also changes row N)" / "Use <X>'s". The user decides; nothing
  changes another row without being asked. `nextFreeInstance`, the 2..9 limit
  and the fork-at-cap rule are not built.
- **A kit swap follows the same table, never dropping settings** (replaces
  10-30 decision 1). Shipped kit instruments carry no bundle, so a row's
  settings carry over to the kit's instrument with no question. If any row's
  pick would need the question, one question covers the whole swap ("Keep
  each row's settings" / "Use the kit's"). Cancel leaves every row as it was.

Unsaved document edits on an instrument no row plays any more are dropped from
the circuit as today and come back with undo (real-time edits belong to
undo/redo, instruction 5).

### 2.10 Built later on this model (nothing here builds them)

- **Publishing:** upload an object and the objects it points to by digest;
  the backend's CID is the digest.
- **Fetching and following:** a fetched version appends to the identity's
  history; the `ask` policy (§2.8) tells the user and confirms, with the
  preference to skip asking.
- **Browsing versions / back to factory:** moving the current pointer.
- **Publisher namespaces:** identities are opaque strings, so a publisher
  prefix can be added without changing versions.

## Part 3 · Caps and the governor, only on evidence (replaces residency spec §7.3–7.4 and plan D's scope)

- The instrument cap and the other caps of residency spec §6 are sized from the
  Part 1 measurements after Part 1 lands (10-30 decision 3).
- The step-down ladder is built only if those measurements show late notes,
  inside the caps, that a ladder step would fix. Its first step, if built, is
  pausing background pools (`async.Pool` gains Pause/Resume).
- If render-worker ordering is needed, it is soonest-needed first, the rule
  `orderQueueByStart` already applies in audio.js: a note's start time, a
  pending switch's first upcoming note, background and graph renders last.
- Not built: device classes (any per-platform budget goes in
  `RuntimeProfile`), skipped frames via `SetScreenClearedEveryFrame(false)`,
  a generalised `Slicer`, undo byte limits and recording byte changes. Each
  comes back only with a measurement that asks for it.

## 3. Goals, non-goals, anti-goals

**Goals**

- Every amendment in Part 1, proven in real Chrome against the 1.1 baseline.
- Instruments and bundles are versioned objects with the history, current
  pointer, retention and digest rules of Part 2; circuits have the object kind
  and digest, with history to come.
- Save As is the only save and appends versions.
- A future publishing / fetching / update-prompt feature needs no change to
  the object model, the digests or the file format.

**Non-goals**

- Publishing, fetching, an update prompt, its preference, a version browser,
  "back to factory", and any UI for circuit bundles or merge.
- A circuit library UI.
- Changing the late-note rule.

**Anti-goals — failure even if everything "works"**

- Any part of a sound cut, shortened or changed: a tail, a voice, a stage
  (instruction 2).
- A user setting dropped or changed without the user being asked, including
  by a pick, a kit swap, an open (beyond the 10-31 newest-version rule), a
  prune or a failed write (instruction 4).
- The drum view holding audio switching state (instruction 1).
- Any shipped template, demo or existing export that no longer imports
  byte-identically, or sounds different by one bit.
- A version id that differs between desktop and browser for the same object.
- A saved instrument lost when the store changes format.

## 4. Testing

- **Tails:** a long reverb, and a 1000 ms full-wet delay with feedback, on a
  row switched away while ringing. Until release, the master render must
  equal one where the channel was never released, sample for sample, on
  desktop and in the browser (`xplat_audio_stages` style); every echo is
  heard. After a rebuild, the channel must equal a freshly built one. A→B→A
  within the hold: no rebuild (`audioGraphStats` counters).
- **Switch:** the existing fake-clock lane tests gain a cold-voice switch
  (old instrument heard until ready, then new, no note late or doubled); the
  real-Chrome switch measurement of 1.1 gates it on heard audio.
- **Drum view separation:** a test that the drum view's row state has no
  sounding/pending field and that export/undo/delete never consult the lane.
- **Digests:** the same object hashes the same natively and under `GOOS=js`
  (a browser test reproduces a native golden of version ids); identity fields
  do not change a digest; a one-bit param change does.
- **History:** save, move back, save again keeps every version; pruning keeps
  current, version 0 and everything referenced; a failed write leaves the
  store intact; the old library and `sample_edits` load into version 1 with
  nothing lost.
- **Combine:** every mode × part, table-driven.
- **Identity of sound and file:** every export→import identity test, the
  template round-trip and `TestStartupDemoLoads` unchanged and green; every
  shipped template re-exports byte-identically.
- **Open:** a saved newer version's document plays when a circuit pinning
  the older one opens, while the circuit's own EQ/FX/Sampler settings for that
  instrument and each of its instances are kept. A circuit whose instruments
  have no user-saved version (or whose pointer is on version 0) plays its own
  copies.
- **UI** (Save As states, the pick question including "also changes row N",
  the kit-swap question): screenshot scenes on desktop and mobile, opened and
  inspected; the question card gets a row in `TestEveryMenuClosesTheSameWay`.

## 5. Build order

1. **Measure** (1.1) on today's code.
2. **Residency** (1.2, 1.3): lazy channels and stages, silence-gated release,
   the lane switch rule, voice-cache LRU. Reads bundles through
   `CurrentBundle` (§2.6), which Part 2 extends rather than replaces.
3. **Objects** (2.1–2.6, 2.8): digests, the store, history, retention, the
   library read-in, `Effective`, `CurrentBundle` / `ApplyBundle`, the
   resolver. No visible change.
4. **Save As and picks** (2.7, 2.9): UI work with its scenes.
5. **Caps**, then the governor only if Part 3's condition is met.

## 6. Decisions

**The user's (10-31):** instructions 1–12 in §0, and the simpler designs of
the 10-31 review: tail-safe release, switching inside the lane, one stats
export, reused gates, trimmed measurements, the bundle as a value over the
existing stores, no library fallback, no instance fork, Plan D on evidence,
no device classes.

**Calls made in this document (check these):**

- release only after the channel's output and every stage's internal state
  have been below −90 dBFS for 250 ms, and a rebuilt channel starts fresh,
  LFO phase included (1.2);
- the master EQ stays a circuit field, not a bundle (2.1);
- a sample instrument's version references its PCM by SHA-256 and the PCM
  stays in the sample store (2.1);
- version = full SHA-256 over canonical JSON, excluding identity and effect
  instance uids; the pack's 16-hex refs stay separate (2.2);
- bundles have no identity of their own (2.3);
- retention default 20 versions per identity; current, version 0 and anything
  referenced are never pruned (2.4);
- the existing user library is read into the store once; a saved Sampler edit
  joins a version only when its instrument has a saved document, otherwise it
  stays untouched in its old section (2.4);
- a Save As that cannot persist tells the user through the error
  notification (2.4);
- `merge` concatenates FX (instrument then circuit) and treats the Sampler
  edit and EQ as `override` (2.5);
- "has a current version" means the pointer is on a user-saved version;
  version 0 plays the circuit's copy (2.8);
- the kit swap asks at most one question for the whole swap (2.9).

**Left to the builder:** package names (my suggestion: a pure
`internal/objects` with no game imports), the store's file and key names, the
exact hold values if measurement says otherwise, test file names, and the
wording of the buttons and questions.
