# Auto-select the sole audible instrument in the audio-panel channel dropdown

Date: 2026-05-27
Branch: add-core-node-types

## Goal

Quality-of-life: when a user **solos a single instrument**, or **mutes all but one
instrument**, the audio-panel "Master"/instrument dropdown (the channel picker in the
audio sticky bar) should automatically switch to select the only audible instrument.
The dropdown is the single source of truth for the channel shown on **every** audio-panel
tab, so when it auto-switches, all tabs must follow.

## Behavior rule (single, unified)

Both triggers reduce to one underlying state: **exactly one row is audible.**

A row is *audible* iff `!Muted && (!anySolo || Solo)`, where `anySolo` is true when any
row has `Solo == true`. This is the same predicate the timeline uses to dim rows
(`timeline_zone.go`).

After any solo/mute toggle:

- Compute `anySolo` and the count of audible rows.
- If **exactly one** row is audible **AND** there are **≥ 2 rows total** **AND** that
  row's `Instrument` id is non-empty → select that instrument's channel.
- Otherwise do nothing.

Decisions:

- **Stay put (no auto-revert).** When the user leaves the single-audible state (un-solo,
  or un-mute a second instrument), the dropdown keeps its current selection. It never
  reverts to Master on its own. Auto-select only ever fires *toward* a sole-audible
  instrument.
- **≥ 2-rows guard.** A single-instrument project never auto-switches away from Master on
  a stray toggle. The feature is about disambiguating among multiple instruments.
- **Non-empty instrument guard.** A row with an empty instrument id is never auto-selected
  (would otherwise collapse to "main").

## Single-source-of-truth consolidation

Investigation found the channel selection is currently **fragmented**: only the dropdown
click handler updates `z.activeChannel` (the field Wave/Spectrum/Synth read). Every other
entry point — the channel-cycle button (`cycleEQChannel`), row-rename
(`onRowInstrumentChanged`), import, and the WASM `setEQChannel` bridge — calls
`setEQActiveChannel`, which updates `dv.eqActiveChannel` but **not** `z.activeChannel`.
So those paths fail to update the Wave/Spectrum/Synth tabs today.

Fix by centralizing:

1. **`setEQActiveChannel(id)`** (`drumview_audio_eq.go`) gains one responsibility: also sync
   `dv.eqPanelZone.activeChannel` (normalized: `"" → "main"`). This makes **every** channel
   change keep `dv.eqActiveChannel` and `z.activeChannel` in lockstep, so Wave/Spectrum/Synth
   follow any programmatic selection (cycle, rename, import, auto-select).

2. **New `(*DrumView).selectAudioChannel(id string)`** (`drumview_audio_eq.go`) — the full
   cross-tab selector. Exactly the current `OnChannelChange` body:
   `setEQActiveChannel(id)` + `AnalyzerService().SetDetailChannel` (Wave/Spectrum data) +
   `ScopeService().SetInstrument` (Chain, desktop) + `chainZone.instrumentID = id` (Chain,
   WASM/test). One chokepoint for "select this channel everywhere."

3. **`OnChannelChange` closure** (`drumview_ctor.go`) → delegates to `dv.selectAudioChannel(id)`.

4. **WASM `setEQChannel`** (`js_exports_eq_widgets.go`) → delegates to
   `g.drum.selectAudioChannel(id)` (+ `bgDirty = true`). Removes the duplicated fan-out.

5. **Dropdown overlay click handler** (`eq_panel_zone.go buildCallbacks`) is left as-is.
   Its direct `z.activeChannel = id` / button-text writes are now redundant in production
   (the real `OnChannelChange` does them via `setEQActiveChannel`) but are still required by
   `eq_panel_zone_input_test.go`, which wires a *fake* logging `OnChannelChange`. Removing
   them would regress those tests for no benefit, so we keep them.

6. **New `(*DrumView).autoSelectSoleAudibleChannel()`** (`drumview_rows.go`) implements the
   rule above and calls `selectAudioChannel(soleRow.Instrument)`. Called at the end of both
   `toggleMute` and `toggleSolo`.

`toggleMute`/`toggleSolo` run in the same Update/input-dispatch context as the dropdown
click, and `selectAudioChannel` touches only DrumView + audio-package state (not `seqMu`),
so there is no new deadlock risk.

## Per-tab follow map (after the change)

| Tab | Field that selects its instrument | Follows dropdown? |
|---|---|---|
| EQ | `dv.eqActiveChannel` | yes |
| Wave | `z.activeChannel` → `resolveChannel` (Master vs Detail) + analyzer Detail channel | yes |
| Spectrum | `z.activeChannel` → `resolveChannel` + analyzer Detail channel | yes |
| Synth | `synthTabActiveInstrument()` → `eqPanelZone.ActiveChannel()` (`z.activeChannel`) | yes |
| Chain | `chainZone.instrumentID` (+ `ScopeService().Instrument()` on desktop) | yes |
| Levels | shows all channels equally (no per-channel focus) | n/a by design |

Under `-tags test` and WASM, `AnalyzerService()`/`ScopeService()` return `nil`, so the
always-observable fields in fast Go tests are `dv.eqActiveChannel`, `z.activeChannel`,
`synthTabActiveInstrument()`, and `chainZone.instrumentID`. The analyzer-Detail and
scope-service fan-out is preserved byte-for-byte from the existing `OnChannelChange`, so it
keeps working on real desktop.

## Files to change

- `src/go/internal/ui/drumview_audio_eq.go` — sync `z.activeChannel` in `setEQActiveChannel`; add `selectAudioChannel`.
- `src/go/internal/ui/drumview_ctor.go` — `OnChannelChange` → `selectAudioChannel`.
- `src/go/internal/ui/drumview_rows.go` — add `autoSelectSoleAudibleChannel`; call it from `toggleMute`/`toggleSolo`.
- `src/go/internal/ui/js_exports_eq_widgets.go` — `setEQChannel` → `selectAudioChannel` (verify `GOOS=js` build).

## Test plan (fast Go path, `internal/ui`)

New file `eq_auto_select_on_solo_mute_test.go` (build tag `test`). Written RED first.

1. **Solo auto-selects** — 3 rows, channel Master; `toggleSolo(1)`; assert all four
   observables == row 1's instrument; button text == row 1's name.
2. **Mute all-but-one auto-selects** — 3 rows; `toggleMute(0)`, then `toggleMute(2)`;
   after the second (one audible) all observables == row 1's instrument. Also assert that
   after only the *first* mute (two audible) the channel is still "main".
3. **Re-solo moves selection** — solo row 1 (selects row 1), then `toggleSolo(2)`
   (single-solo flips); observables == row 2's instrument.
4. **Stay put on un-solo** — solo row 1 (selects row 1), then `toggleSolo(1)` again
   (all audible); channel stays row 1, does **not** revert to "main".
5. **No auto-select when 2+ audible** — channel Master; `toggleMute(0)` on 3 rows (two
   still audible); channel stays "main".
6. **Single-row guard** — 1 row; `toggleSolo(0)` then `toggleMute(0)`; channel stays "main".
7. **Empty-instrument guard** — 2 rows, sole-audible row has empty `Instrument`; toggle to
   make it sole audible; channel stays "main".
8. **All tabs follow `selectAudioChannel` (regression lock)** — call
   `selectAudioChannel(row2.Instrument)` directly; assert EQ/Wave-Spectrum/Synth/Chain
   observables all == row 2; then `selectAudioChannel("main")` and assert they reset
   (Synth falls back to first row by design).
9. **Cycle/rename now sync `z.activeChannel` (consolidation regression)** — call
   `cycleEQChannel()` / `onRowInstrumentChanged`; assert `eqPanelZone.ActiveChannel()` tracks
   `dv.eqActiveChannel` (previously it did not).

Plus: re-run existing `eq_channel_curve_sync_test.go`, `eq_per_instrument_test.go`,
`eq_panel_zone_input_test.go`, `row_rack_zone_test.go`, and the full `internal/ui` +
`internal/audio` suites to confirm no regression, and a `GOOS=js -tags test` build to verify
the WASM bridge change compiles.

## Out of scope

- Auto-select on row add/delete.
- Auto-revert / remembering prior selection.
- Multi-solo (the toggle handler is single-solo by design).
- Levels-tab per-channel focus.
