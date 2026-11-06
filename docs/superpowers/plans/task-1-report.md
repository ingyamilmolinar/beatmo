# Task 1 Report — EQPanelZone live-write seam + open-wheel callback

Plan: `docs/superpowers/plans/2026-07-02-eq-wheel-popup-value-editor.md`, Task 1 only.

## Changes

### `src/go/internal/ui/eq_panel_zone.go` (modified)

1. **`EQCallbacks` struct** (~line 19): added a new field after `OnApplyEQ`:
   ```go
   // OnOpenValueWheel opens the mobile-style precision wheel popup for a band's
   // dB gain (desktop + mobile). Wired by DrumView; when nil the dB cell falls
   // back to the inline numeric editor.
   OnOpenValueWheel func(band int)
   ```

2. **`applyBandGainLive`** — new method added immediately before the
   `eqDBOpenAdapter` type (functionally "immediately after `openEQDBEditor`" per
   the brief; the adapter type sits directly below `openEQDBEditor`'s closing
   brace, so the new method was inserted at that seam):
   ```go
   func (z *EQPanelZone) applyBandGainLive(band int, v float64) {
       if band < 0 || band >= len(z.bandGainsDB) {
           return
       }
       v = clampF64(v, -12.0, 12.0)
       z.bandGainsDB[band] = v
       z.curveDirty = true
       if z.callbacks.OnGainChange != nil {
           z.callbacks.OnGainChange(band, v)
       }
       if z.callbacks.OnApplyEQ != nil {
           z.callbacks.OnApplyEQ()
       }
   }
   ```
   Writes the source of truth (`bandGainsDB`), clamps to [-12, +12] via the
   existing `clampF64` helper, fires `OnGainChange` + `OnApplyEQ` (live path),
   and deliberately does NOT fire `OnEQBandCommit` (commit stays a
   release-only event, matching the curve-drag path).

3. **`eqDBOpenAdapter.OnPress`** — rerouted to prefer the wheel callback,
   falling back to the legacy numeric editor when nil:
   ```go
   func (a *eqDBOpenAdapter) OnPress(x, y int) InputResult {
       if a.z.callbacks.OnOpenValueWheel != nil {
           a.z.callbacks.OnOpenValueWheel(a.band)
           return InputConsumed
       }
       a.z.openEQDBEditor(a.band)
       return InputConsumed
   }
   ```

Ran `gofmt -w` on the file afterward (the struct-field edit shifted column
alignment on the pre-existing fields; gofmt re-aligned `OnEQBandCommit`
through `OnApplyEQ`'s tab stops — no semantic change).

### `src/go/internal/ui/eq_wheel_seam_test.go` (created)

Transcribed verbatim from the plan brief (Step 1): two tests —
`TestApplyBandGainLive_WritesClampsAndFiresLive` and
`TestEQDBOpenAdapter_RoutesToWheelCallback`.

## Test commands run

### Step 2 — confirm the new tests fail before implementation

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestApplyBandGainLive_WritesClampsAndFiresLive|TestEQDBOpenAdapter_RoutesToWheelCallback' \
  ./internal/ui/
```
Result: **FAIL** (build failure, as expected):
```
internal/ui/eq_wheel_seam_test.go:19:4: z.applyBandGainLive undefined (type *EQPanelZone has no field or method applyBandGainLive)
internal/ui/eq_wheel_seam_test.go:33:4: z.applyBandGainLive undefined (type *EQPanelZone has no field or method applyBandGainLive)
internal/ui/eq_wheel_seam_test.go:37:4: z.applyBandGainLive undefined (type *EQPanelZone has no field or method applyBandGainLive)
internal/ui/eq_wheel_seam_test.go:48:3: unknown field OnOpenValueWheel in struct literal of type EQCallbacks
FAIL	github.com/ingyamilmolinar/beatmo/internal/ui [build failed]
FAIL
```

### Step 6 — confirm the new tests pass after implementation

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestApplyBandGainLive_WritesClampsAndFiresLive|TestEQDBOpenAdapter_RoutesToWheelCallback' \
  ./internal/ui/ -v
```
Result: **PASS**
```
--- PASS: TestApplyBandGainLive_WritesClampsAndFiresLive (0.00s)
--- PASS: TestEQDBOpenAdapter_RoutesToWheelCallback (0.00s)
PASS
ok  	github.com/ingyamilmolinar/beatmo/internal/ui	0.168s
```
Re-verified again after the `gofmt -w` pass — still PASS (cached run,
identical result).

### Step 7 — existing EQ dB regression check

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'EQDB|EQ_DBInput|Esc_EQdB|Functional_ArrowKeys_EQdBEditor' \
  ./internal/ui/ -v
```
Result: **PASS** — 24/24 tests green, including the 2 new tests picked up by
the broader regex (`TestEQDBOpenAdapter_RoutesToWheelCallback` matches
`EQDB`):
```
--- PASS: TestFunctional_ArrowKeys_EQdBEditor (0.10s)
--- PASS: TestEQDBInput_InitialValues (0.00s)
--- PASS: TestEQDBInput_CurveDragSyncsText (0.00s)
--- PASS: TestEQDBInput_TextCommitSyncsCurve (0.00s)
--- PASS: TestEQDBInput_ValidationClamp (0.00s)
--- PASS: TestEQDBInput_InvalidTextReverts (0.00s)
--- PASS: TestEQDBInput_EmptyTextReverts (0.00s)
--- PASS: TestEQDBInput_EscapeReverts (0.00s)
--- PASS: TestEQDBInput_EnterCommits (0.00s)
--- PASS: TestEQDBInput_OnlyOneFocusedAtOnce (0.00s)
--- PASS: TestEQDBInput_SyncBandStateUpdatesTexts (0.00s)
--- PASS: TestEQDBInput_MutedBandShowsValue (0.00s)
--- PASS: TestEQDBInput_HitAreaRegistered (0.00s)
--- PASS: TestEQDBInput_HandleKeyEnterCommits (0.00s)
--- PASS: TestEQDBInput_HandleKeyEscapeReverts (0.00s)
--- PASS: TestEQDBInput_DirectBandGainsDBMutationSyncsTexts (0.00s)
--- PASS: TestEQDBInput_NoBiDirectionalLoop (0.00s)
--- PASS: TestEQDBEditorPrefillShowsValueAndCommits (0.00s)
--- PASS: TestEQ_DBInputCommit (0.00s)
--- PASS: TestEQ_DBInputEscapeReverts (0.00s)
--- PASS: TestEQDBOpenAdapter_RoutesToWheelCallback (0.00s)
--- PASS: TestEscRevertsEQDBEdit (0.00s)
--- PASS: TestFunctional_Esc_EQdBInput (0.07s)
--- PASS: TestEsc_EQdBInput_RevertsAndCloses (0.00s)
PASS
ok  	github.com/ingyamilmolinar/beatmo/internal/ui	(cached)
```

### Extra: gofmt check

```
../../.tools/go/bin/gofmt -l internal/ui/eq_panel_zone.go internal/ui/eq_wheel_seam_test.go
```
Initially flagged `eq_panel_zone.go` for a column-alignment diff (existing
struct-tag alignment shifted by the new field's comment/name length); ran
`gofmt -w` and reconfirmed clean + tests still green.

## Concerns

None. Task 1 is scoped exactly as specified — no popup/DrumView/portal code
was touched (that's Tasks 2–4). All work is uncommitted per the standing
no-`git add`/`git commit` rule.
