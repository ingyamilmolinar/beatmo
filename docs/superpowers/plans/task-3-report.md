# Task 3 Report — Input isolation + audio-apply parity (EQ wheel popup)

## Scope

Task 3 of `2026-07-02-eq-wheel-popup-value-editor.md`. Test-only: no production
code was modified. Tasks 1–2 (`applyBandGainLive`, `OnOpenValueWheel`,
`openEQKnobWheelPopup`, the `"eq-wheel-popup"` Modal+Scrim portal) were
confirmed already in place before writing tests (verified via grep in
`drumview_ctor.go`, `eq_panel_zone.go`, `eq_wheel_popup.go`).

## File created

`src/go/internal/ui/eq_wheel_popup_isolation_test.go`

## Tests added (6 total: 2 spec'd in the plan + 2 extra audio-apply-parity tests)

1. **`TestEQWheelPopup_IsModalBlocking`** — opens the EQ wheel popup via
   `dv.openEQKnobWheelPopup(0)` and asserts `g.drum.PortalHasBlocking()` and
   `g.modalOverlayActive()` are both `true` while it's open. Confirms the
   `"eq-wheel-popup"` portal entry registers as a blocking (Modal+Scrim) entry.

2. **`TestEQWheelPopup_BlocksGridTap`** — with the wheel open, calls
   `g.handleTapInGrid(gx, gy)` at a grid-pane point outside the popup rect and
   asserts `len(g.nodes)` is unchanged. Confirms the scrim/modal blocks the
   Game-level out-of-tree grid-tap handler (the same class of leak the
   synth-wheel isolation tests guard against).

3. **`TestEQWheelPopup_AppliesToMasterChannel`** — with the default channel
   ("main"), drives one wheel step by setting `binding.Knob.Value` directly and
   calling `binding.OnChange()` (per the plan's guidance: `HandleWheel`'s
   notcher needs many events to move, so a single call is a no-op — this
   mirrors the existing `TestEQWheelPopup_SharesSourceOfTruthWithCurve` in
   `eq_wheel_popup_test.go`). Asserts:
   - `dv.eqPanelZone.bandGainsDB[0]` reflects the new dB value (the live-write
     seam landed on the source of truth).
   - `dv.eqApplied[0].GainDB` reflects the same value — proving the wheel's
     `OnChange` → `applyBandGainLive` → `OnGainChange`/`OnApplyEQ` callbacks →
     `dv.applyEQ()` → `dv.applyMasterEQ()` → `dv.eqApplied` chain actually ran
     (i.e., the wheel edit reached the audio-apply path, not just the UI
     state).

4. **`TestEQWheelPopup_AppliesToRowChannel`** — switches the active EQ channel
   to a row instrument (`dv.setEQActiveChannel(rowInst)`), then drives a wheel
   step the same way, and asserts `dv.Rows[0].EQGainsDB[0]` reflects the new
   value. This exercises the per-row branch of `OnGainChange` (writes directly
   into `r.EQGainsDB[band]` then calls `dv.applyRowEQ(j)`), the counterpart to
   the master-channel test — the plan only asked for "master or row, whichever
   the existing tests use"; I added both since the codebase already
   distinguishes master vs. row apply paths (`applyMasterEQ` vs `applyRowEQ`)
   and a wheel-popup regression could plausibly hit only one.

## Observation seam reused (not invented)

`dv.eqApplied[band].GainDB` / `.Muted` — the exact seam used by
`TestEQBandMuteUnmutePreservesGain` (`eq_per_instrument_test.go:962-999`) and
`TestEQPanelMuteVisualState` (`eq_per_instrument_test.go:675-687`). Evidence:

```
internal/ui/eq_per_instrument_test.go:962:  dv.applyMasterEQ()
internal/ui/eq_per_instrument_test.go:965:  if dv.eqApplied[0].GainDB != 6.0 {
```

Traced the wiring to confirm `applyBandGainLive`'s `OnApplyEQ` callback
(`drumview_ctor.go:284-286`, `OnApplyEQ: func() { dv.applyEQ() }`) resolves to
`dv.applyMasterEQ()` when channel == "main" (`drumview_audio_eq.go:136-151`,
`applyEQ`), which populates `dv.eqApplied` (`drumview_audio_eq.go:57-73`,
`applyMasterEQ`). For the row-channel test, traced `OnGainChange`
(`drumview_ctor.go:249-268`) which writes `r.EQGainsDB[band]` directly and
calls `dv.applyRowEQ(j)` when the active channel isn't "main".

## Other existing helpers reused (per plan instruction, confirmed via grep before use)

- `setupEQTabForTest(g, w, h)` — canonical EQ-tab bring-up, from
  `eq_wheel_popup_test.go` (`g.Layout` → `recalcButtons` → `tabState.SetActiveTab(TabEQ)` →
  `Invalidate` → `zone.Layout`). Confirmed `dv.SetActiveTab`/`dv.Layout` do NOT
  exist on `DrumView` — used this helper instead, per the task brief's warning.
- `assertDefaultParityState(t)`, `withDefaultStart(t, false)`,
  `globalTouchState.Reset()`, `New(testLogger)`, `g.drum`, `gridTopOffset()`,
  `g.split.InGridPane`, `g.handleTapInGrid`, `g.modalOverlayActive()`,
  `g.drum.PortalHasBlocking()` — all mirrored 1:1 from
  `synth_wheel_popup_isolation_test.go`'s `openMobileSynthWheel` /
  `TestSynthWheelPopupBlocksGridTap` pattern.

## Exact test command + output

```
cd /home/ymolinar/Repos/beatmo/src/go
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup' ./internal/ui/ -v
```

Result (grep'd to the RUN/PASS lines):

```
=== RUN   TestEQWheelPopup_IsModalBlocking
--- PASS: TestEQWheelPopup_IsModalBlocking (0.00s)
=== RUN   TestEQWheelPopup_BlocksGridTap
--- PASS: TestEQWheelPopup_BlocksGridTap (0.00s)
=== RUN   TestEQWheelPopup_AppliesToMasterChannel
--- PASS: TestEQWheelPopup_AppliesToMasterChannel (0.00s)
=== RUN   TestEQWheelPopup_AppliesToRowChannel
--- PASS: TestEQWheelPopup_AppliesToRowChannel (0.00s)
=== RUN   TestEQWheelPopup_OpensOnBothProfiles
=== RUN   TestEQWheelPopup_OpensOnBothProfiles/desktop
=== RUN   TestEQWheelPopup_OpensOnBothProfiles/mobile
--- PASS: TestEQWheelPopup_OpensOnBothProfiles (0.02s)
=== RUN   TestEQWheelPopup_SharesSourceOfTruthWithCurve
--- PASS: TestEQWheelPopup_SharesSourceOfTruthWithCurve (0.00s)
=== RUN   TestEQWheelPopup_CenterBoxOpensNumericEditor
--- PASS: TestEQWheelPopup_CenterBoxOpensNumericEditor (0.01s)
PASS
ok  	github.com/ingyamilmolinar/beatmo/internal/ui	(cached)
```

(6 tests match `TestEQWheelPopup` in the new file's scope: the 4 I added plus
2 pre-existing from Task 2's `eq_wheel_popup_test.go`, which also matched the
`-run` pattern and are unaffected regression guards.)

Broader regression check (per the plan's Task 2 Step 10 precedent, extended to
cover this task's surface area):

```
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'EQ|WheelPopup|CloseAllPopups|Portal' ./internal/ui/
ok  	github.com/ingyamilmolinar/beatmo/internal/ui	3.247s
```

## Concerns

**None — no isolation leak found.** Both `TestEQWheelPopup_IsModalBlocking`
and `TestEQWheelPopup_BlocksGridTap` passed on the first run with zero
production changes, confirming the Task 2 `Modal: true, Scrim: true` portal
wiring (`drumview_portal_open.go`, `openEQWheelPortal`) already correctly
isolates the grid pane, exactly as the plan predicted ("the synth/sampler
wheels already prove this path"). No fix was needed. The two additional
audio-apply-parity tests (master + row channel) also passed without any
production change, confirming `applyBandGainLive`'s live-callback chain
(`OnGainChange` → `OnApplyEQ`) correctly reaches both `applyMasterEQ` and
`applyRowEQ` depending on the active channel.

Left untested per Task 3's explicit scope (not required by the plan): camera
pinch/pan blocking (covered structurally by the same Modal+Scrim mechanism the
synth wheel popup already regression-tests; Task 3 only asked for grid-tap +
modal-blocking + apply-parity).

## Files changed

- Created: `src/go/internal/ui/eq_wheel_popup_isolation_test.go`
- No production files modified.
- Nothing staged or committed (per standing user rule).
