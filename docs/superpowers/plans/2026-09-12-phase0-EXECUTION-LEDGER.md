# SDD ledger — plan: docs/superpowers/plans/2026-09-12-phase0-kill-suffix-semantics.md

Spec: docs/superpowers/specs/2026-09-12-uuid-sound-library-design.md (read)

## Pre-flight rulings

Ruling: NO COMMITS and NO WORKTREE — the user's standing rule is "never stage
or commit", and the branch `add-core-node-types` carries 1089 uncommitted
files this work depends on, so isolating into a worktree would orphan them.
The skill's commit-range review packages are replaced by snapshot diffs
(snapshot.sh before dispatch, taskdiff.sh after). Cost if wrong: no
per-task git history; review packages are file snapshots, and a task that
touches a file it did not declare is invisible to its review.

Ruling: Task 2's `TestLegacyInstrumentIDResolvesARetiredVariant` MOVES to
Task 3. As written the plan ends Task 2 with a knowingly-failing test (the
legacy table is not filled until Task 3), so Task 2 could never pass its own
review and the suite would be red at a checkpoint. Task 2 keeps the two tests
that are meaningful against an empty table; Task 3 adds the resolve test as
its own failing-test step, where the rename that makes it pass also lands.
Cost if wrong: Task 2's mechanism ships with thinner tests for one task.

## Pre-flight conflict scan

| Pair | Shared file / interface | Finding |
|---|---|---|
| T1 → T3 | `loudness_amp.go` (`loudnessAliases`) | Clean. T1 creates the table; T3 renames its keys. Ordered. |
| T1 → T3 | `loudness_alias_test.go` | Clean. T3 renames the ids; expected amplitudes deliberately unchanged. |
| T2 → T3 | `legacy_instrument_ids.go` | CONFLICT — ruled above. T2 creates an empty table; T3 fills it. |
| T3 → T4 | roster (`instrument_ids_gen.go`) | Clean. T4's roster guard is a no-op until T3 removes the `-N` ids; T4 Step 2 states this. |
| T4 → T5 | `instance_instrument.go` | Clean. T5 replaces the strip block T4 wrote, preserving T4's registered-id guard. |
| T4 → T5 | `instance_instrument_test.go` | Clean. Both append; file already exists. |
| T3 → T6 | roster; `cmd/refs/doctor.go` | Clean. T6's test names `snare-lofi`/`snare-crack` from T3's table. |
| T5 → templates | `internal/assets/templates/*.json` | Clean. Templates are generated; `make templates` regenerates. |

| Task | Internal consistency |
|---|---|
| T1 | Clean. Characterization test green before the refactor, same values after. |
| T2 | Defect ruled above; otherwise clean. |
| T3 | Clean. Step 6's grep `-[1-9]",$` cannot match `bass-808`/`kick-808` (3 digits). |
| T4 | Clean. Step 2 anticipates the test already passing after T3. |
| T5 | Clean. `TrimSuffix(base,"-")` yields `organ` from `organ--2` and `piano-grand` from `piano-grand-3`. |
| T6 | Clean. Step 2 anticipates the already-passing case; golden regen command verified against `doctor_test.go:97`. |

## Progress

Task 1: dispatched (sonnet) — loudness alias map replaces the suffix fold.
  Files declared: internal/audio/{loudness_amp.go,synth_recipe_dispatch.go,loudness_alias_test.go}
  Snapshot taken for the review package (no-commit policy).
Ruling applied to plan: Task 2's resolve-test moved to Task 3 Step 1; Task 3
  steps renumbered 1-10. Plan file updated so briefs match execution.
Task 1: implementer returned DONE_WITH_CONCERNS. Review dispatched (opus).
  Concern A (implementer): brief Step 2's characterization claim is wrong —
    the fold was applied AT THE CALL SITE, so ConfigForInstrument("snare-1")
    was 0.8 before, not 1.0. Controller verified: my own pre-plan measurement
    shows the same (ampOwn=0.8). The plan text was wrong, not the code.
  Concern B (implementer): variants.go:77,125 carry an inline COPY of the
    same -1/-2 strip, feeding normalizeAndScale for the CVariantInstrument
    path — i.e. a FOURTH decoder the spec never counted. Left untouched
    (correctly) as outside the declared three-file scope.
  Controller verification of the blast radius:
    - amplitudeForInstrument callers: export.go:29,58 (inside normalizeAndScale)
      and export.go:111 AmplitudeForInstrumentExport -> cmd/export_audio only.
    - measure-loudness iterates InstrumentConfigs, which does NOT contain the
      variants, so regenerating the loudness table is unaffected.
    - AmplitudeForInstrumentExport now returns 1.0 (was 0.8) for the 17
      variants, matching what native playback already used. Possible xplat
      golden impact — flagged to the reviewer as an open question.
Task 1 review (opus): SPEC ✅, QUALITY ❌ — 1 Critical, 2 Important, 2 Minor.
  CRITICAL (verified by controller): the deleted fold covered ALL ids ending
    -1/-2, including INSTANCE ids from EnsureInstanceInstrument. 16 rows in
    shipped templates (celesta-2, guitar-nylon-2 x2, piano-grand-2 x7,
    violin-2 x5, voice-ahh-2) now render at 0.8 — celesta +2.31 dB. Both
    gates iterate BuiltinInstrumentIDs so neither could see it.
  Reviewer also answered the xplat question: under //go:build js nothing
    reads .Amplitude, so browser parity is unaffected; measure-loudness's
    newAmp is scale-invariant so the generator output is unchanged;
    instrument_clone.go:68 gains a latent-bug FIX. No golden risk.
  Reviewer on variants.go: no double-apply (the strip is idempotent and
    passes v.Name), but suffix semantics now live in THREE places.

Ruling: EXPAND Task 1's file scope to include instance_instrument.go and its
  test. The Critical cannot be fixed inside the declared three files —
  instance ids are unbounded, so it needs the runtime seam
  (setRuntimeLoudnessAmp) rather than more static rows. Cost if wrong: Task 1
  now touches a file Tasks 4 and 5 also edit; they run strictly after, so no
  conflict, but Task 1's review package had to be re-snapshotted.

Ruling: Minor "synth_recipe.go:590 dangling reference to the dispatch-file
  baseInstrumentID" is DEFERRED out of the loop and carried into Task 3,
  which already edits synth_recipe.go. Cost if wrong: one stale comment
  survives until Task 3.

Task 1: minor (deferred): synth_recipe.go:590 references the deleted
  dispatch-file baseInstrumentID — fix during Task 3.
Task 1: fix round 1/5 dispatched (resumed original implementer).
Ruling: the FOURTH decoder (variants.go:77,125 inline -1/-2 strip) is folded
  into Task 3 as new Steps 10-11, not left for a later phase. Rationale: after
  Task 3's rename no registered v.Name ends in an ordinal, so the strip becomes
  provably dead — and Phase 0's stated goal ("nothing parses an id") is not met
  while it stands. A proof test (TestNoVariantNameEndsInAnOrdinal) gates the
  deletion instead of assuming it. Cost if wrong: Task 3 grows two steps and
  one more file; if the strip turns out NOT to be dead, byte-identity fails at
  Step 11 and the deletion is reverted.
Ruling: the plan's "Done when" grep gate was too weak — it matched the
  function NAME only, and the fourth decoder was an inline copy no such grep
  would catch. Gate strengthened to also look for id-shape tests.
Task 1: fix round 1/5 (3 addressed, 0 open — instance-variant amplitude via
  setRuntimeLoudnessAmp; godoc binding; false characterization comment).
  Re-review confirmed: seeding sits after all three early-return guards and
  before any render; the violin-2 test is genuinely non-roster and would fail
  if the seeding line were removed; cleanup via withDefaultAudio->ResetInstruments.
  No collision with instrument_clone's use of runtimeLoudnessAmp (clone ids are
  re-namespaced to user.*, disjoint from instance ids).
Task 1: complete (5 files, review clean, byte-identity green)
Task 2: implementer DONE (2 new files, audio suite green). Review dispatched.
Task 2 review (sonnet): SPEC ✅, QUALITY approved. 2 Minors, 1 ⚠️.
Task 2: complete (2 new files, review clean)
Task 2: minor (deferred): test file carries //go:build !js, used by only
  9/269 test files in the package; came from plan boilerplate. Folded into
  new Task 4 Step 6.

Ruling: the ⚠️ "no call-site usage to verify" is a REAL PLAN GAP, not a
  deferrable observation. Verified: `grep -rn LegacyInstrumentID` outside its
  own file returns NOTHING. The plan built the resolver (T2), filled it (T3),
  and never wired it — so renaming 18 instruments would silently silence every
  saved project naming an old id, the exact outcome the table exists to
  prevent. Inserted a new Task 4 ("Wire the legacy table into the registry")
  and renumbered old 4/5/6 -> 5/6/7. It registers each retired id as a playable
  alias inside bindBuiltinInstrumentRecipes (the one tag-neutral chokepoint all
  THREE platform ResetInstruments implementations call), mirroring
  EnsureInstanceInstrument's promotion body. Its test asserts recipe AND
  amplitude equality through the real registry, not a map read — a table-only
  test would pass while the feature is dead. Cost if wrong: one extra task; if
  registry aliasing proves the wrong layer, the alternative is a load-time
  rewrite at each read path (project, kit, bundle, prefs), which is what
  Phase 2's legacy_ids.json would do anyway.

Ruling: "Done when" gains an end-to-end clause — a project naming a retired id
  must LOAD AND PLAY, not merely appear in a lookup table.
Task 3: implementer returned a NON-STATUS ("I'll stop polling now and wait for
  the monitor's notification") after 100 tool calls / 739s, and wrote no report.
  Controller verified the tree directly rather than re-dispatching blindly:
    DONE  — 18 renames in engine_instruments.go + synth_recipe.go + regenerated
            roster; legacyInstrumentIDs has 18 rows; 0 roster ids end in an
            ordinal; synth_recipe.go's dangling baseInstrumentID comment fixed;
            native build OK; audio suite green.
    DONE  — TestSynthGoldenByteIdentity PASSES in the REAL build. NOTE: it is
            tagged `!test && !js`, so `-tags test` runs silently skip it. The
            plan's test commands for it were wrong in every task; the correct
            form is `CGO_ENABLED=1 go test ./internal/audio/ -run ...` with NO
            `-tags test`.
    NOT DONE — Step 10 (variants.go strip deletion + TestNoVariantNameEndsInAnOrdinal
            proof test) and Steps 11-12. Resumed the implementer scoped to
            exactly those, with an explicit instruction not to poll.
Task 3: implementer DONE (steps 10-12). Scope expanded beyond the declared
  list, all reported: startup_demo.json, src/js/audio.js, 6 audio test files.
  Controller VERIFIED both of its "pre-existing/out-of-scope" claims rather
  than accepting them:
  - "cmd/refs deferred to Task 6" — WRONG for 18 of 20. TestRunAdopt_*/
    TestRunAdoptScan_* (17) + TestInstrumentFromFilename_Resolves fail because
    the test files hard-code renamed ids ("snare-2" is not a registered
    instrument). That is a regression this rename caused. Fix round dispatched.
  - The other 2 (TestRunDoctor_MatchesGolden, TestCoverage_NoUnclassified-
    Instruments) ARE legitimately Task 7's.
  - "internal/deploy pre-existing stale main.wasm" — NOT established. The file
    was rebuilt today at 20:40, inside Task 3's window; raw size is 26.54 MiB
    vs a 26.50 budget (memory records 26.52 earlier today). UNATTRIBUTED —
    carried to the final review, not dismissed.
  Controller also checked persisted data for renamed ids: catalogue.json has
  ZERO refs pointing at a renamed instrument, and no other JSON under internal/
  references one. No data migration needed.

Ruling: cmd/refs stays RED for exactly two tests between Task 3 and Task 7.
  Unavoidable — the rename necessarily invalidates the doctor golden, and the
  coverage test fails because the renamed variants stop folding onto their
  bases and become visibly uncovered. Task 7 amended to own both, including
  classifying the newly-visible ids in catalogue.json (status synthetic /
  needs-source per sound — NOT inventing reference recordings). Cost if wrong:
  the tree carries two known-red tests for three tasks; every other package
  stays green.
Task 3: fix round 1/5 (18 addressed, 0 open — cmd/refs test files + 2 stale doc
  comments re-pointed at the new ids). Controller re-ran ./... : the ONLY
  failures tree-wide are the 2 Task-7 tests and internal/deploy's wasm budget.
WASM BUDGET RESOLVED (was unattributed): rebuilt main.wasm from current source
  via `make wasm` -> 26.54 MiB vs a 26.50 budget. Memory records 26.52 earlier
  today, i.e. the budget was ALREADY breached before Phase 0; this work added
  ~0.02 MiB (~21 KB, consistent with two new maps + a function). So: NOT a
  stale artifact as the implementer claimed, and NOT solely ours. Carried to
  the final review; the choice is trim or deliberately re-baseline the budget.
Task 3: review dispatched (opus) — package covers the 8 declared files; the
  expanded-scope files have no git baseline (branch never committed), so the
  reviewer was pointed at them to read directly.
Task 3 review (opus): SPEC ❌, QUALITY not approved. 1 Critical, 2 Important,
  4 Minor. Fix round 2 dispatched.
  CRITICAL: 8 tests fail in the REAL build (!test && !js) — the implementer
    only ever ran `-tags test`, which SKIPS them silently. Same blind spot as
    the byte-identity gate. One-line id swaps across 7 files.
  IMPORTANT #2 (load-bearing, routed to Task 4): with snare-1 no longer
    registered, instanceBaseID now STRIPS it, so
    factoryRecipeForInstrument("snare-1") -> "snare" -> drum-snare — the PLAIN
    snare, not drum-snare-rimshot which is what snare-crack is. A legacy
    project would load the WRONG SOUND silently. Registering the alias is not
    sufficient: factoryRecipeForInstrument reads the STATIC bindings map, so
    the legacy lookup must precede the instanceBaseID fallback.
  IMPORTANT #3: stale docs naming renamed ids as live (AGENTS.synth.instruments.md,
    AGENTS.synth.md, CLAUDE.md L53/L511, Makefile L178) — folded into fix round 2.
  Reviewer confirmed independently: instrument_ids_gen.go is byte-identical to
    a clean regeneration; zero comment lines lost in engine_instruments.go;
    src/js/audio.js and Go id sets agree exactly (2 occurrences each, RENDER +
    RENDER_INFO); legacy table has 18 correct, non-reversed, non-colliding rows.

Ruling: Important #2 is routed to Task 4 rather than Task 3, because Task 4 is
  the task that wires legacy resolution at all. Task 4 amended with a new
  Step 4 that inserts the legacy lookup BEFORE the instanceBaseID fallback in
  factoryRecipeForInstrument, and its test now fails without it. Cost if wrong:
  the wrong-sound window persists until Task 4 lands (it is already open now,
  and no shipped template uses a retired id — only user projects could).

Task 3: minor (deferred): ~16 stale fixtures in internal/ui + internal/audio
  tests still LABEL rows kick-1 / fm-epiano-1. They pass, but now drive
  UNREGISTERED ids, so e.g. synth_header_layout_test exercises a fallback path
  rather than the instrument it names. Real test-quality erosion; not blocking.
Task 3: minor (deferred): synth_golden_pinned_test.go's comment says "fm-*-drive",
  a wildcard matching only 2 of the 5 FM variants.
Task 3: minor (deferred): gofmt reflowed 14 lines of an unrelated protected
  comment block — pre-existing debt (HEAD's copy was not gofmt-clean), diff noise only.
Ruling: SYSTEMIC PLAN DEFECT fixed plan-wide. All 3 byte-identity invocations
  used `-tags test`, which matches NOTHING (the test is tagged !test && !js)
  and exits 0 — so the gate I repeatedly called "the proof Phase 0 changed no
  audio" was never executing in those commands. All 3 rewritten to
  `CGO_ENABLED=1 go test ./internal/audio/ -run ...`, with the trap documented
  inline. Global Constraints also now REQUIRE both builds per task, citing
  Task 3 reporting complete on the fast path while 8 real-build tests failed.
  Cost if wrong: none — this strictly widens verification.
Task 3: fix round 2/5 dispatched and returned; controller independently
  confirmed real build green + byte-identity PASS + fast suite green except
  the 3 known. Scoped re-review dispatched (sonnet).
Task 3: fix round 2/5 (4 addressed, 0 open). Re-review verified every changed
  line in the 8 test files is a 1:1 id substitution — no assertion, error
  message or list length altered (it checked list lengths explicitly: bass 7
  ids, fm 10 ids, unchanged). Docs verified line-by-line, no prose mangled.
Task 3: minor (deferred): bass_native_fastpath_test.go:51 comment still says
  "sub-bass + its -1 variant".
Task 3: complete (renames + legacy table + variants.go strip deleted; both
  builds green except the 3 known-deferred; byte-identity PASS)
Task 4: implementer escalated NEEDS_CONTEXT — correctly. Implementing the
  brief exactly made the 18 legacy ids first-class registered instruments,
  growing Instruments() 133 -> 151 and breaking TestInstrumentParityAfterReset.
  It stopped rather than touching a 4th file. Good call.

Ruling: REGISTRY ALIASING WAS THE WRONG DESIGN — my error, caught by that
  test. Verified: audio.Instruments() feeds dv.instOptions
  (drumview_transport.go:404, drumview_ctor.go:25, demo.go:232) — it IS the
  instrument picker's source. Registering legacy ids would put snare-1 /
  snare-2 / kick-1 back in the user's menu beside snare-crack / snare-lofi /
  kick-snap, i.e. reintroduce the exact duplicate naming this phase deletes.
  New design: resolve at the READ path. ResolveInstrumentID rewrites a retired
  id to the current one at the top of import.go's instrument loop, so the row
  plays the right sound, the picker stays at 133, and the project migrates on
  next save. The factoryRecipeForInstrument ordering fix STAYS as defence in
  depth. Scope expanded to internal/ui/import.go + one new internal/ui test.
  Cost if wrong: read paths other than project import (kits, bundles,
  favourites) still carry old ids — acceptable for Phase 0 and the same
  boundary Phase 2's legacy_ids.json will formalise; the defence-in-depth
  ordering fix means those still resolve to the right RECIPE even unmigrated.

Ruling: TestInstrumentParityAfterReset is NOT to be relaxed. It is a correct
  invariant that caught a real design flaw; the design changes, not the test.
Task 4: fix round 1/5 — redesign landed. Controller verified independently
  (the agent returned polling non-statuses twice more before reporting):
    ResolveInstrumentID present; registerLegacyInstrumentAliases deleted from
    BOTH files; import.go:521 wired at the top of the instrument loop; the
    factoryRecipeForInstrument ordering fix retained; new e2e test
    internal/ui/legacy_instrument_import_test.go.
    TestInstrumentParityAfterReset PASS (133, restored). Byte-identity PASS.
    wasm build of audio+ui PASS. TestImportResolvesRetiredInstrumentID PASS.
  Controller read the new e2e test to confirm it is not vacuous: it builds a
  real project JSON naming snare-1, drives the real g.Import path, and asserts
  Rows[0].Instrument == "snare-crack". It would fail if the wiring were removed.
Task 4: fix round 2/5 dispatched — TestImportEmbeddedPCM asserts IsUserSample
  on the retired literals "kick-1"/"fm-epiano-1". Runtime behaviour is CORRECT
  (the embedded sample now registers under the resolved id, consistent with the
  row); the test holds stale literals. Authorised that one file, and asked the
  implementer to prefer keeping a RETIRED id in the fixture while asserting the
  RESOLVED registration — that would prove embedded PCM survives a rename,
  which is a real user guarantee currently untested.
Task 4: fix round 2/5 — implementer chose the STRONGER option: kept the retired
  ids in the fixture JSON and asserted the RESOLVED ids in the loop, so the test
  now proves embedded user PCM survives a rename. internal/ui fully green (325s).
  (The agent's repeated "waiting" replies were explained: internal/ui takes
  ~350s and the tool auto-backgrounds past its timeout — not the agent's doing.)
Task 4 review (opus): SPEC ✅ (against the superseding design), QUALITY approved
  with findings. Fix round 3 dispatched.
  IMPORTANT: the load-bearing ordering in factoryRecipeForInstrument is
    UNTESTED — fix round 1 swapped the aliases test for TABLE tests, so moving
    the legacy branch below instanceBaseID keeps the suite green while snare-1
    returns drum-snare instead of drum-snare-rimshot. Guard test ordered, and
    the implementer must prove it fails when the branch is reordered.
  Reviewer confirmed independently: inst is a value copy so the rewrite covers
    every later read and escapes nowhere; ALL project-read entry points
    (cmd/beatmo.go:268, demo.go:72/124, JS importJSON) funnel through g.Import,
    so the single line covers them; the e2e test fails if that line is removed;
    import_embedded_pcm_test.go has 2 retired ids and ZERO new ids in the
    fixture, so IsUserSample("kick-snap") can only pass via real resolution.

READ-PATH INVENTORY for Phase 2 (reviewer-verified, recorded because this is
  what Phase 2 must close — NOT assumed):
  (1) Kits -> RegisterKit (import.go:148): retired member id sets a dead
      row.Instrument. Biggest gap, same function -> FIXED NOW in fix round 3.
  (2) PinnedInstruments -> SetProjectPins (:166): stale pin never matches
      -> FIXED NOW.
  (3) userprefs Favorites: stale star lost. Benign. DEFERRED.
  (4) instruments store (RecipeOverrides, SampleEdits, UserRecipes,
      WavRegistry) — all id-keyed; a retired key is dead, so a user's saved
      tone or sample-edit SILENTLY REVERTS to shipped. DEFERRED - highest
      user-visible risk of the deferred set.
  (5) instrument bundles (ApplyInstrumentBundle): no resolution on ids or
      base_recipe. DEFERRED.
  Clean: shipped templates/assets (no retired ids), uistate (no instrument ids).

Ruling: fix (1) and (2) now — they are two lines in the function already being
  edited, and leaving a known dead-instrument path to a later phase when the
  fix is adjacent is worse than the scope discipline is worth. (3)(4)(5) stay
  deferred to Phase 2 with the inventory above. Cost if wrong: Phase 0 touches
  two more call sites in a file it already edits.

Ruling: the kick-2 ambiguity (retired variant vs gen-showcase's second-kick
  instance id) is resolved IN FAVOUR of the retired variant, and documented in
  the table's doc comment. Any project predating today genuinely meant the
  shipped variant, and no shipped template mints a colliding id (reviewer
  grepped internal/assets). Task 6's --N suffix makes the collision impossible
  going forward. Cost if wrong: a hand-authored project that used kick-2 as an
  instance id would resolve to kick-lofi, a different sound.
Task 4: fix round 3/5 (5 addressed, 0 open). Implementer PROVED the guard test
  falsifiable — reordered the branch, saw it fail on both snare-1 and kick-1,
  restored. Re-review independently confirmed the kick-1 row is not decorative:
  base "kick" binds drum-kick (synth_recipe.go:507) while kick-snap binds
  drum-kick-punchy (:522), a genuine divergence. Kit-member mutation semantics
  verified (Members is map[string]string, so in-place mutation is visible to
  the later RegisterKit(k), which deep-copies AFTER the resolve loop).
  hihat-open-bright confirmed present in startup_demo.json:158.
Task 4: complete (5 files + 2 ui files, review clean; parity 133 restored)
Task 5: implementer DONE. Controller verified: roster guard in place, stale
  prose paragraph and its 3 dead ids gone, both builds green, wasm OK.
  Hotness investigated as asked: instanceBaseID has only two call chains
  (EnsureInstanceInstrument at import time, factoryRecipeForInstrument for
  menu categorisation / Sampler save / tooling) — no per-frame or per-sample
  caller, so the 133-id linear scan stays and NO cache was added. Good YAGNI.
  It also checked the pre-existing TestInstanceBaseID table cases (violin-1,
  organ-2) do not collide with a roster id, so the guard changes none of them.
Task 5 review (sonnet): SPEC ✅, QUALITY approved, no findings above Minor.
  Reviewer independently confirmed the guard is the first statement with no
  second route to the strip; the doc comment is accurate and free of renamed
  ids; and the perf claim holds (only EnsureInstanceInstrument at import,
  category.go:134 CategoryOf via refreshInstruments — a list rebuild, not a
  per-frame draw — and user_samples.go for Sampler save/import).
  CONFIRMED DECORATIVE: TestInstanceBaseIDNeverStripsARegisteredInstrument
  passes identically with the guard REMOVED, because after the rename no
  roster id matches the -N shape. A synthetic registered -N id would be
  strictly stronger.
Task 5: complete (2 files, review clean)

Ruling: the decorative-test Minor is NOT merely deferred — it is folded into
  Task 6, which edits the same two files. A guard whose test cannot fail is
  exactly the failure class this whole session kept hitting (the byte-identity
  gate that never ran, the gate iterating the wrong population, the grep that
  matched only a function name). Fixing it costs one test inside a dispatch
  that was happening anyway. Cost if wrong: Task 6 grows by one test.
Task 6: implementer DONE. Controller verified independently: gen-showcase mints
  --N (main.go:210), instanceBaseID parses BOTH forms, templates regenerated
  (celesta--2, guitar-nylon--2/3, piano-grand--2/3, violin--2, voice-ahh--2),
  byte-identity PASS, all 6 instanceBaseID tests PASS.
  CONTROLLER RAN THE FALSIFIABILITY EXPERIMENT ITSELF (the agent had not yet
  reported): disabled the roster guard -> the NEW synthetic test FAILED while
  the OLD roster test still PASSED, proving the new test is real AND confirming
  the old one was decorative. File restored byte-identically (diff clean).
  Agent self-reported dispatching one forbidden Explore subagent, caught it,
  did not use its output. Accepted; noted.

REGRESSION FOUND BY CONTROLLER (agent's own suites missed it — it never ran
  ./... ): internal/assets TestTemplates_AllInstrumentsRegistered fails 14x on
  "violin--2" etc. Cause is a FIFTH inline copy of the id strip, in
  internal/assets/templates_test.go:34, introduced with the comment
  "instanceBaseStrip mirrors audio.instanceBaseID (unexported)". It knows only
  the single-hyphen form.

Ruling: do NOT teach the fifth copy about "--". EXPORT audio.InstanceBaseID and
  have the assets test call it, deleting the copy. Teaching the copy would
  perpetuate exactly the duplication this phase exists to remove and would go
  stale again at the next form change. Also ordered a repo-wide grep for
  further copies, to be REPORTED not silently fixed. Cost if wrong: one new
  exported symbol on the audio package.

  Running count of id decoders found: 3 in the spec, +1 (variants.go) during
  Task 1, +1 (internal/assets test) during Task 6 = FIVE. The spec's
  "three decoders" figure was wrong and should be corrected before Phase 1.
Task 6: fix round 1/5 (fifth decoder removed). audio.InstanceBaseID exported;
  internal/assets/templates_test.go now calls it instead of its own copy.
  Repo-wide grep for further copies found ONE hit —
  loudness_alias_test.go:111 (TestNoVariantNameEndsInAnOrdinal), correctly
  assessed as a roster invariant, not an instance resolver, and left alone.
  Full ./... is back to only the 2 known-red packages (cmd/refs, internal/deploy).
NOTE: taskdiff.sh disappeared from the workspace mid-session (snapshot.sh and
  the ledger survived); recreated it. Cause unknown — flagged, not assumed.
Task 6 review (sonnet): SPEC ✅, QUALITY approved, 1 Important (for the
  orchestrator, not the implementer) + 1 Minor.
  Reviewer walked the edge cases: organ--2 -> organ; piano-grand-3 -> piano-grand;
  kick-808 untouched (second-to-last char is '0', not '-'); bass-8 -> roster
  guard short-circuits if registered, else strips to bass (intended); "--0"
  untouched (fails the 1..9 guard). Minor: a bare "--2" yields an empty base —
  unreachable, gen-showcase never mints an empty id. Theoretical: an id whose
  base legitimately ends in '-' would over-strip; none exists (0 of 133).
  Falsifiability test verified sound (mutates BuiltinInstrumentIDs, restores via
  t.Cleanup, no t.Parallel in the file); reviewer reproduced the disable/restore
  proof independently. Fifth-decoder removal confirmed complete (grep: 0 hits).

IMPORTANT, verified by controller: `make templates` regenerated against a
  gen-showcase/main.go that carries the USER'S OWN unstaged sibling fix
  (unitsPerStep := s.Subdiv/4 -> 4, with a comment about schubert-ave-maria
  playing at half speed). So the template diff now contains BOTH that fix and
  our --N change: node-layout rewrites, BPM changes and row removals in
  beethoven-quartet-op18, borodin-quartet-2, dvorak-american, haydn-quartet-op76,
  mozart-quartet-k387, schubert-ave-maria and others.
Ruling: LEAVE IT. This is not contamination we introduced — templates were
  ALREADY stale before this task (the implementer saw 12 TestDrift failures
  pre-regen, caused by the user's unreflected main.go fix), and templates are
  DERIVED artifacts whose contract is to match current generator output. They
  now do. Reverting them would re-break drift and would mean hand-editing
  generated files, which the codebase forbids. The user must know their
  in-flight fix is now baked into the committed templates. Cost if wrong: the
  template diff attributable to Phase 0 is larger than Phase 0's own change,
  and must be read alongside the user's gen-showcase WIP.
Task 6: complete (4 files + regenerated templates, review clean)
Task 7: implementer complete. cmd/refs FULLY GREEN — both long-red tests closed.
  Classified the 18 newly-visible ids: 13 borrowed (same recipe as an existing
  covered instrument), 2 synthetic (kick-lofi, kick-snap), 3 needs-source
  (hihat-open-bright, tom-high-drive, tom-lofi).
  It caught that tom-lofi's recipe is actually "Tom Low" — a real acoustic tom,
  NOT a bitcrushed effect — despite its name, and classified it needs-source
  accordingly. That independently confirms the Task 3 reviewer's naming quibble
  that tom-lofi loses the high/low distinction.
  Doctor counts: covered 57->64, synthetic 33->41, needs-source 25->28,
  unclassified 18->0, over the full 133 roster (previously a folded 115).
  Deviation flagged by the implementer: it also regenerated
  internal/audio/testdata/refs/MANIFEST.md, a generated file that desynced
  purely because of the mandated catalogue.json edit.
  Its own concern, recorded: kick-lofi/kick-snap inherit a PRE-EXISTING
  convention treating the whole native drum-gen "kick" family as synthetic even
  though it is C drum-gen rather than FM/modular. Not introduced here; flagged
  for whoever revisits kick's own classification.
CONTROLLER VERIFIED: full `-tags test ./...` now fails ONLY internal/deploy
  (the pre-existing wasm budget breach). No baseInstrumentID remains anywhere
  in the tree except one historical reference inside a test comment.

=== FINAL WHOLE-PHASE REVIEW (opus) ===
Merge verdict: blocked ONLY by 4 broken browser tests. Everything else follow-up.
  CRITICAL: the JS browser suite was NEVER RUN during the phase — no task's
    authored set contained a .browser.test.js, and `./...` does not reach JS.
    4 tests broken by the rename (import_embedded_pcm, webaudio_wav_bus,
    instrument_params, synth_param_extremes). CI runs these.
  SIXTH DECODER: cmd/refs/instrument_name.go instrumentFromFilename. The rename
    silently changed its behaviour AND falsified its own comment ("the LONGEST
    match wins, which is what keeps snare-2.wav from collapsing onto snare" —
    with snare-2 off the roster it now DOES collapse).
  SEAM: EnsureInstanceInstrument never resolves retired ids — fed snare-1 it
    would voice PLAIN snare while binding drum-snare-rimshot. Latent only
    because import.go resolves first and is the sole caller.
  SEAM: RecipeForInstrument (live map / recipeForInstrument JS export) had no
    legacy branch; returned "" where FactoryRecipeForInstrument was correct.

CORRECTION TO MY OWN CLAIM: "byte-identity proves no rendered audio changed" is
  NOT literally true. Byte-identity iterates BuiltinInstrumentIDs; instance ids
  are outside it. Duplicate rows with ordinal >=3 (piano-grand-3,
  guitar-nylon-3 — both SHIP in templates) previously fell through to
  DefaultAmplitude and now inherit the base: guitar-nylon +0.91 dB,
  piano-grand +0.02 dB. This is a correctness FIX (a third duplicate should
  match its base exactly as the second already did), but the honest claim is
  "no shipped-instrument audio changed; third-and-later duplicate rows are
  re-levelled onto their base, correcting a pre-existing fallthrough."

CORRECTION TO MY TRIAGE: I called the instruments store the highest deferred
  risk. WRONG — RecipeOverrides is keyed by RECIPE id and no recipe id was
  renamed, so saved TONES survive intact. What actually breaks is
  ApplySavedSampleEdits / ApplySavedSamples (instrument-id-keyed): a Sampler
  edit or uploaded WAV on a retired id goes unmatched. Nothing destroyed,
  18 ids, one ResolveInstrumentID per loader.

Final fix wave: all 9 items done; the 4 browser tests were REBUILT (make wasm)
  and actually RUN — all 4 pass. Implementer caught an error in MY dispatch
  prose (I wrote snare-2 -> snare-crack; the real table is snare-2 -> snare-lofi,
  snare-1 -> snare-crack) and followed the table, not my text.
Controller REJECTED the fix wave's "pre-existing" claim for
  TestManifestMarkdown_MatchesCommittedFile: cmd/refs was fully green after
  task 7 (whose implementer regenerated MANIFEST.md for exactly this reason),
  so the item-8 catalogue.json reword desynced it again. Regeneration dispatched.
