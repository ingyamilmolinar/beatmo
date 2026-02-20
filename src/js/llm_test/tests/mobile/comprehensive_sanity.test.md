---
name: comprehensive_sanity
platform: mobile
device: iPhone 12 landscape
tags: [sanity, comprehensive, regression]
fixture: null
max_iterations: 70
viewport: 844x390
---

# Mobile Comprehensive Sanity Test

## Demo State

The app loads a demo circuit with:
- **7 rows** (0-indexed): Kick-deep (teal), Snare (orange), Hi-Hat (yellow), Clap (purple), Tom (blue), Cowbell (brown-orange), Fm-epiano-1 (pink)
- **BPM**: 120, **Subdiv**: 8
- **59 nodes** in the grid pane — colored squares matching each row's instrument color, connected by edges with arrows
- **Existing FX**: Kick-deep has Distortion, Snare has Distortion, Hi-Hat has Delay, Clap has Bitcrusher, Cowbell has Distortion
- Tom volume=0 (intentionally silent), Cowbell volume=0.05 (very quiet)
- **Transport** (compact, single row): Play | Stop | BPM input | BPM ± | Subdiv | Len ± | ViewSwitch | Overflow
- **Row controls** (mobile): Label + Speaker icon only — no inline M/S/FX/O/X buttons
- **Row height**: 44px tall rows, only ~4-5 visible at once — scroll to see all 7

## Tool Usage Rules

### What Works with `click_ui`
- **Transport**: play, stop, bpmInc, bpmDec, subdiv, lenInc, lenDec, addRow, overflow, viewSwitch
- **Per-row**: label (opens context menu), volume (opens volume popup)
- **Batch clicks**: Use `repeat_click_ui` with `count` for BPM ± and similar repetitive clicks.

### What FAILS with `click_ui` on Mobile
- `click_ui button="mute"` — zero rect, use context menu instead
- `click_ui button="solo"` — zero rect, use context menu instead
- `click_ui button="fx"` — zero rect, use context menu instead
- `click_ui button="color"` — zero rect, use context menu instead
- `click_ui button="edit"` — zero rect, use context menu instead
- `click_ui button="delete"` — zero rect, use context menu instead
- `click_ui button="origin"` — zero rect, use context menu instead
- `click_ui button="mainVol"` — not on mobile transport

### `computer` Tool Usage
- Context menu item clicks (after opening via `click_ui button="label"`)
- Volume popup slider drags (after opening via `click_ui button="volume"`)
- Grid node taps (to open node sidebar)
- Node sidebar interactions (expand sections, use +/- buttons, dropdowns)
- Slider drags, scroll wheel, typing

### Mobile Interaction Patterns

**Context Menu Pattern** (for Mute, Solo, Instrument, Color, FX, Origin, Delete):
1. `click_ui button="label" row=N` — opens bottom-sheet context menu
2. Take a screenshot to see the menu items and their positions
3. Use `computer` tool to click the desired item (Mute, Solo, Instrument, etc.)
4. Menu closes automatically after selection

**Volume Popup Pattern**:
1. `click_ui button="volume" row=N` — opens vertical slider popup
2. Take a screenshot to see the popup and slider position
3. Use `computer` tool to drag the slider vertically (up = louder, down = quieter)
4. Tap outside the popup to close it

**Node Sidebar Pattern**:
1. Use `computer` tool to tap a node in the grid pane
2. Sidebar opens on the left side (260px wide) with instrument name header
3. Take a screenshot to see collapsible sections (Volume, Pitch, Duration, Logic, Groove, Audible)
4. Tap section headers to expand/collapse, use +/- buttons to adjust values
5. Tap outside or tap the node again to close

### General Rules
- **Layout queries**: Use `query_ui` before interactions to get exact rect bounds.
- **Retry limit**: If an action fails after **2 attempts**, report it as `ISSUE` and move on.
- **No page reload**: Do NOT press F5 or reload the page.
- **Avoid grid clicks for unfocusing**: Press Enter or Escape instead.

### BPM Text Input Procedure
1. Click the **BPM text input** field using the `computer` tool at the BPM coordinates.
2. Press **Ctrl+A** to select all, then type the new value (e.g., "90"), then press **Enter**.
3. Enter commits the value and unfocuses the field.
4. Take a screenshot. The State line will show the new BPM.

### Dropdown Selection Tips
When a dropdown opens (subdiv, EQ channel, instrument), items appear stacked vertically. Each item is ~30px tall. Take a screenshot to see exact positions, then click the desired item.

## Testing Philosophy

At **every phase**, after **every action**, report anything unusual. Use this format for ALL issues:

```
ISSUE: [P0|P1|P2|P3] [Bug|Glitch|Suggestion] - <description>
```

Do not skip reporting issues to "save iterations." Finding bugs is the primary purpose of this test.

**Be a critical mobile UX reviewer.** At the end of each phase, evaluate: Are touch targets large enough? Is feedback clear on touch? Are context menus discoverable? Is text readable at mobile resolution? Report each observation as `ISSUE: P3 Suggestion - <description>`. You must provide **at least 1 UI suggestion per phase** and **at least 5 total** across all phases.

---

## Phase 1 — Initial State Audit (Mobile)

1. Take a screenshot. Verify the **grid pane** (top) shows colored square nodes connected by edges with arrows, and the **drum pane** (bottom) shows the compact transport bar and instrument rows.
2. Check the State line: BPM should be **120**, Subdiv should be **8**.
3. Verify the compact transport shows: Play | Stop | BPM | BPM± | Subdiv | ViewSwitch | Overflow. Note: no master volume slider, no Upload/Import/Export buttons visible (they're in the overflow menu).
4. Verify rows show ONLY Label + Speaker icon — no inline M, S, FX, O, X buttons should be visible.
5. Scroll down in the drum rows area to verify all **7 rows** exist — find Cowbell (row 5) and Fm-epiano-1 (row 6) at the bottom.
6. Scroll back up. Note alternating row backgrounds and color accent stripes on the left of each row.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the mobile initial layout, touch target sizes, or visual design.**

## Phase 2 — Transport: Play and Stop

1. Use `click_ui button="play"` to start playback.
2. Wait 3 seconds. Observe: highlights should animate through nodes in the grid pane (nodes glow when fired), and the drum timeline should show a moving highlight or playhead.
3. Take a screenshot while playing to capture the animation state.
4. Use `click_ui button="stop"` to stop playback.
5. Verify playback stopped: the State line should show `playing=false`. Note: the beat counter resets to **Beat 0/128** on stop.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about playback feedback or transport controls on mobile.**

## Phase 3 — BPM and Subdivision (No Master Volume)

### BPM text entry (set to 90)
1. Click the **BPM text input** field using the `computer` tool at the BPM coordinates.
2. Press Ctrl+A to select all, type **90**, press **Enter**. Enter commits and unfocuses — do NOT press Escape or click elsewhere.
3. Take a screenshot. Verify State shows bpm=90.

### Subdivision dropdown
4. Use `click_ui button="subdiv"` to open the dropdown.
5. Take a screenshot. Items appear stacked below the button (4, 8, 16, 32 from top to bottom). Click the **16** item.
6. Verify subdiv changed to 16. If the dropdown stayed open, press Escape.

Note: Master volume is NOT on the mobile transport — skip master volume testing. It can only be accessed on desktop.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about BPM or subdivision controls on mobile.**

## Phase 4 — Mute, Solo, Volume via Context Menu and Popup

### Mute (via context menu)
1. Use `click_ui button="label" row=1` to open the context menu on **Snare (row 1)**.
2. Take a screenshot. Identify the **Mute** item in the bottom-sheet context menu.
3. Click the **Mute** item using the `computer` tool at its coordinates.
4. Use `click_ui button="play"`. Take a screenshot during playback — Snare should be muted while other instruments play.
5. Use `click_ui button="stop"`.
6. Use `click_ui button="label" row=1` to reopen the context menu.
7. Click **Mute** again to unmute Snare.

### Solo (via context menu)
8. Use `click_ui button="label" row=2` to open the context menu on **Hi-Hat (row 2)**.
9. Take a screenshot. Click the **Solo** item.
10. Use `click_ui button="play"`. Take a screenshot — only Hi-Hat should produce sound.
11. Use `click_ui button="stop"`.
12. Use `click_ui button="label" row=2` to reopen the context menu.
13. Click **Solo** again to unsolo Hi-Hat.

### Volume (via volume popup)
14. Use `click_ui button="volume" row=0` to open the volume popup on **Kick-deep (row 0)**.
15. Take a screenshot. A vertical slider popup should appear (52×160px).
16. Use the `computer` tool to drag the slider to approximately **25%** (drag from current position downward).
17. Tap outside the popup to close it. Take a screenshot to verify.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the context menu, volume popup, or mute/solo feedback on mobile.**

## Phase 5 — Instrument, Color, FX, EQ, Node Sidebar, and Row Management

### Instrument selector (via context menu)
1. Use `click_ui button="label" row=3` to open the context menu on **Clap (row 3)**.
2. Take a screenshot. Click the **Instrument** item.
3. A popup menu should appear with instrument categories. Scroll to find the **Synth (Other)** category — this contains FM instruments (Fm-bass, Fm-bell, Fm-lead, Fm-epiano-1, Fm-pluck). Select any FM instrument.
4. Take a screenshot. Verify the row 3 label changed to the selected instrument name.

### Color wheel (via context menu)
5. Use `click_ui button="label" row=1` to open the context menu on **Snare (row 1)**.
6. Click the **Color** item.
7. Take a screenshot. The color wheel is a circular overlay. Click a distinctly different color area (e.g., if Snare is orange, click the green or blue area).
8. Verify the Snare row 1 color accent stripe updated. Close the color wheel if still open (tap outside).
9. If the color didn't change after 1 attempt, report as `ISSUE` and move on.

### Insert effects (FX) via context menu
10. Use `click_ui button="label" row=0` to open the context menu on **Kick-deep (row 0)**.
11. Click the **Effects** item.
12. An FX panel overlay should appear showing an existing **Distortion** effect. Look for a "+" button to add an effect.
13. Click "+" and select **Reverb** from the list. Verify Reverb appeared in the chain.
14. Click Reverb's remove button to delete it. Verify only the original Distortion remains.
15. Close the FX panel (tap close button or outside).

### EQ / View Switch
16. Use `click_ui button="viewSwitch"` to switch to Audio/EQ view. Take a screenshot — verify the EQ panel is now visible with band sliders.
17. Use `click_ui button="viewSwitch"` again to switch back to Rows view.

### Node Sidebar — Logic and Prediction Verification
18. Use the `computer` tool to tap a visible **colored square node** in the grid pane (use coordinate hints to find one). The node sidebar should open on the left side (260px wide panel).
19. Take a screenshot. Verify the sidebar shows the **instrument name** header and collapsible sections: Volume, Pitch, Duration, Logic, Groove, Audible.
20. Tap the **Logic** section header (shows "> Logic" or "v Logic") to expand it. Take a screenshot — you should see a "Logic:" dropdown showing "None".
21. Tap the **Logic dropdown** button (the row showing "Logic: None") to open the dropdown. Take a screenshot — dropdown items should appear: None, Trigger Every N, Skip Every N, Probability, If Prev Skipped, If Prev Triggered.
22. Tap **"Skip Every N"** in the dropdown. Take a screenshot. The Logic section should now show "Logic: Skip Every N" with "N: 2" and -/+ buttons.
23. Verify the **drum row prediction changed**: scroll down to find the instrument row matching this node's instrument. The step pattern (colored bars in the timeline) should now have gaps — with Skip Every 2, every other occurrence is skipped. Take a screenshot and note whether the pattern visibly changed.
24. Tap the Logic dropdown again and select **"None"** to restore original logic. Verify the row pattern returns to its original full state.
25. Tap outside the sidebar to close it. Verify it closed.

### Add and delete row
26. Use `click_ui button="addRow"` to add a new row. Verify the row count is now **8** (check State line).
27. Use `click_ui button="label" row=7` to open the context menu on the new row.
28. Click the **Delete** item. Verify the row count returned to **7**.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the instrument selector, color wheel, FX panel, EQ, node sidebar, or row management on mobile.**

## Phase 6 — Mobile UX Critique

Dedicate this phase to evaluating the overall mobile user experience. Take screenshots and be thorough.

1. Take a screenshot of the full UI. Evaluate overall layout: pane balance, transport compactness, row height, text readability at mobile resolution. Report **2+ suggestions**.
2. Test context menu discoverability: is it obvious that tapping a label opens a context menu? Is there any visual affordance (icon, chevron, border)? Report **1+ suggestion**.
3. Evaluate the volume popup: is the slider easy to drag on touch? Is the popup large enough? Is there visual feedback? Report **1+ suggestion**.
4. Evaluate the node sidebar: is it easy to discover by tapping nodes? Are the +/- buttons large enough for touch? Are sections clearly collapsible? Report **1+ suggestion**.
5. Start playback with `click_ui button="play"`. Watch for 2–3 seconds. Evaluate animation quality and highlight visibility at mobile resolution. Stop with `click_ui button="stop"`. Report any animation issues.

You must report at least **5 specific, actionable** `ISSUE: P3 Suggestion - ...` lines in this phase.

Beyond the structured checklist above, freely report any additional mobile UX issues. Consider:
- Are touch targets at least 44px? (iOS Human Interface Guidelines recommend 44pt minimum)
- Is the overflow menu discoverable for Upload/Import/Export?
- Does the context menu feel native-like or awkward on mobile?
- Is scrolling through rows smooth? Does the scroll thumb work well on touch?
- Could any controls benefit from haptic feedback or visual press states?
- Is the ViewSwitch button label clear about what it toggles?
- Are long instrument names (e.g., "Fm-epiano-1") readable or truncated in the compact row?
- Does row deletion via context menu feel safe without a confirmation dialog?

---

## Final Summary

After completing all 6 phases, provide your report in this EXACT format:

```
### PHASE RESULTS
- Phase 1: PASS | PARTIAL | FAIL — <note>
- Phase 2: PASS | PARTIAL | FAIL — <note>
- Phase 3: PASS | PARTIAL | FAIL — <note>
- Phase 4: PASS | PARTIAL | FAIL — <note>
- Phase 5: PASS | PARTIAL | FAIL — <note>
- Phase 6: PASS | PARTIAL | FAIL — <note>

### ISSUES
ISSUE: P1 Bug - <description>
ISSUE: P2 Glitch - <description>
ISSUE: P3 Suggestion - <description>
(list every issue you reported during testing, one per line — at least 5 mobile-specific UI suggestions required)

### OVERALL RATING
<1-10> — <explanation of rating>
```

Use the exact `ISSUE:` prefix format for every issue line so they can be parsed automatically.
