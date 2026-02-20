---
name: comprehensive_sanity
platform: desktop
tags: [sanity, comprehensive, regression]
fixture: null
max_iterations: 70
viewport: 1280x720
---

# Comprehensive Sanity Test

## Demo State

The app loads a demo circuit with:
- **7 rows** (0-indexed): Kick-deep (teal), Snare (orange), Hi-Hat (yellow), Clap (purple), Tom (blue), Cowbell (brown-orange), Fm-epiano-1 (pink)
- **BPM**: 120, **Subdiv**: 8
- **59 nodes** in the grid pane — colored squares matching each row's instrument color, connected by edges with arrows
- **Existing FX**: Kick-deep has Distortion, Snare has Distortion, Hi-Hat has Delay, Clap has Bitcrusher, Cowbell has Distortion
- Tom volume=0 (intentionally silent), Cowbell volume=0.05 (very quiet)
- Only ~5 rows visible at once — scroll down to see Cowbell (row 5) and Fm-epiano-1 (row 6)
- **Transport**: Single row — Play, Stop, BPM input, BPM ±, Subdiv, Length ±, Track/Follow, Upload, Import, Export, Master Vol slider
- **Row controls** (desktop): label | edit/save (pencil) | color swatch | volume % + slider | M (mute) | S (solo) | FX | O (origin) | X (delete)

## Tool Usage Rules

- **Buttons**: ALWAYS use `click_ui` for single clicks or `repeat_click_ui` for batch clicks. Never use the `computer` tool to click named buttons.
- **Batch clicks**: Use `repeat_click_ui` with `count` parameter for BPM +/- and similar repetitive clicks (saves iterations).
- **Non-button interactions**: Use `computer` tool for: grid clicks, drag operations, shift-drag edges, right-click context menus, slider drags, scroll wheel, typing.
- **Layout queries**: Use `query_ui` before slider drags to get exact rect bounds.
- **Retry limit**: If an action fails after **2 attempts**, report it as `ISSUE` and move on to the next step. Do not retry more than twice.
- **No page reload**: Do NOT press F5 or reload the page at any point during the test.
- **Avoid grid clicks for unfocusing**: Do NOT click on the grid pane to unfocus text fields — this creates node popups and wastes iterations. Press Enter or Escape instead.

### Slider Drag Formula

Horizontal sliders (volume, master vol) have a label on the left ~35% and an interactive track on the right ~65%. To drag a slider to a target percentage P (0–100):

1. Call `query_ui` to get the slider rect `{x, y, w, h}`.
2. Compute: `trackLeft = x + round(w * 0.35)`, `trackRight = x + w`, `centerY = y + round(h / 2)`.
3. Target X: `targetX = trackLeft + round((trackRight - trackLeft) * P / 100)`.
4. Use the `computer` tool to **drag** from `(trackLeft, centerY)` to `(targetX, centerY)`.

### BPM Text Input Procedure

1. Click the **BPM text input** field using the `computer` tool at the BPM coordinates.
2. Press **Ctrl+A** to select all, then type the new value (e.g., "90"), then press **Enter**.
3. Enter commits the value and unfocuses the field. No need to press Escape or click elsewhere.
4. Take a screenshot. The State line will show the new BPM.

### Dropdown Selection Tips

When a dropdown opens (subdiv, EQ channel, instrument), items appear stacked vertically below the button. Each item is ~25–30px tall. Take a screenshot to see the exact item positions, then click the desired item.

## Testing Philosophy

At **every phase**, after **every action**, report anything unusual. Use this format for ALL issues:

```
ISSUE: [P0|P1|P2|P3] [Bug|Glitch|Suggestion] - <description>
```

Do not skip reporting issues to "save iterations." Finding bugs is the primary purpose of this test.

**Be a critical UX reviewer.** At the end of each phase, spend 1–2 sentences evaluating: Are controls easy to discover? Is feedback clear? Are hit targets large enough? Is text readable? Are there any confusing or ugly elements? Report each observation as `ISSUE: P3 Suggestion - <description>`. You must provide **at least 1 UI suggestion per phase** and **at least 5 total** across all phases.

---

## Phase 1 — Initial State Audit

1. Take a screenshot. Verify the **grid pane** (top) shows colored square nodes connected by edges with arrows, and the **drum pane** (bottom) shows the transport bar and instrument rows.
2. Check the State line: BPM should be **120**, Subdiv should be **8**.
3. Scroll down in the drum rows area to verify all **7 rows** exist — you should find Cowbell (row 5) and Fm-epiano-1 (row 6) at the bottom.
4. Scroll back up to the top rows. Note any visual issues — overlapping elements, clipped text, misaligned controls.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the initial layout or visual design.**

## Phase 2 — Transport: Play and Stop

1. Use `click_ui button="play"` to start playback.
2. Wait 3 seconds. Observe: highlights should animate through nodes in the grid pane (nodes glow when fired), and the drum timeline should show a moving highlight or playhead.
3. Take a screenshot while playing to capture the animation state.
4. Use `click_ui button="stop"` to stop playback.
5. Verify playback stopped: the State line should show `playing=false`. Note: the beat counter resets to **Beat 0/128** on stop.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about playback feedback or transport controls.**

## Phase 3 — BPM, Subdivision, and Master Volume

### BPM text entry (set to 90)
1. Click the **BPM text input** field using the `computer` tool at the BPM coordinates.
2. Press Ctrl+A to select all, type **90**, press **Enter**. Enter commits and unfocuses — do NOT press Escape or click elsewhere.
3. Take a screenshot. Verify State shows bpm=90.

### Subdivision dropdown
4. Use `click_ui button="subdiv"` to open the dropdown.
5. Take a screenshot. Items appear stacked below the button (4, 8, 16, 32 from top to bottom). Click the **16** item.
6. Verify subdiv changed to 16. If the dropdown stayed open, press Escape.

### Master volume slider
7. Call `query_ui` to get the master volume slider rect (`mainVol`).
8. Use the slider drag formula to drag the master volume to approximately **50%**. Drag from `(trackLeft, centerY)` to the 50% target X position.
9. Take a screenshot to verify the slider visually updated to ~50%. Leave it at 50% — no need to restore.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the BPM, subdivision, or volume controls.**

## Phase 4 — Mute, Solo, and Row Volume

### Mute
1. Use `click_ui button="mute" row=1` to mute **Snare (row 1)**.
2. Use `click_ui button="play"`. Take a screenshot during playback — Snare should be muted while other instruments play.
3. Use `click_ui button="stop"`.
4. Use `click_ui button="mute" row=1` to unmute Snare.

### Solo
5. Use `click_ui button="solo" row=2` to solo **Hi-Hat (row 2)**.
6. Use `click_ui button="play"`. Take a screenshot — only Hi-Hat should produce sound.
7. Use `click_ui button="stop"`.
8. Use `click_ui button="solo" row=2` to unsolo Hi-Hat.

### Row volume slider
9. Call `query_ui` to get the volume slider rect for row 0.
10. Use the slider drag formula to drag row 0's volume to approximately **25%**.
11. Take a screenshot. Verify the slider moved. Leave it — no need to restore.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about mute/solo feedback or volume controls.**

## Phase 5 — Instrument, Color, FX, EQ, and Row Management

### Instrument selector
1. Use `click_ui button="label" row=3` to open the instrument selector on **Clap (row 3)**.
2. A popup menu should appear with instrument categories. Scroll to find the **Synth (Other)** category — this contains FM instruments (Fm-bass, Fm-bell, Fm-lead, Fm-epiano-1, Fm-pluck). Select any FM instrument.
3. Take a screenshot. Verify the row 3 label changed to the selected instrument name.

### Color wheel
4. Use `click_ui button="color" row=1` to open the color wheel on **Snare (row 1)**.
5. Take a screenshot. The color wheel is a **circular overlay**. Identify a distinctly different color area (e.g., if Snare is orange, click the green or blue area of the wheel).
6. Click on that area using the `computer` tool at coordinates within the wheel circle.
7. Verify the Snare row 1 color swatch updated to the new color. Close the color wheel if still open (click outside it or press Escape).
8. If the color didn't change after 1 attempt, report it as an `ISSUE` and move on.

### Insert effects (FX) panel
9. Use `click_ui button="fx" row=0` to open the FX panel on **Kick-deep (row 0)**.
10. An FX panel overlay should appear showing an existing **Distortion** effect. Look for a "+" button to add an effect.
11. Click "+" and select **Reverb** from the list. Verify Reverb appeared in the chain.
12. Click Reverb's **remove** button to delete it. Verify only the original Distortion remains.
13. Close the FX panel (click close button or outside).

### EQ channel switching
14. Use `click_ui button="eqChannel"` to click the EQ channel selector. This opens a **dropdown** — take a screenshot to see the available channels, then click an instrument channel (e.g., Kick-deep). Verify the EQ display updated.
15. Use `click_ui button="eqChannel"` again to open the dropdown, and select **Master** to switch back.

### HP/LP filter buttons
16. Use `click_ui button="hpf"` to toggle the high-pass filter. Take a screenshot — the HPF button should appear active/highlighted.
17. Use `click_ui button="hpf"` again to disable it.
18. Use `click_ui button="lpf"` to toggle the low-pass filter. Take a screenshot — the LPF button should appear active/highlighted.
19. Use `click_ui button="lpf"` again to disable it.

### EQ toggle
20. Use `click_ui button="eqToggle"` to toggle the EQ panel visibility. This **hides** the EQ display area. Take a screenshot to verify.
21. Use `click_ui button="eqToggle"` again to restore the EQ display.

### Node Sidebar — Logic
22. Left-click a visible node in the **grid pane** (top area) using the `computer` tool. This selects the node and opens the node sidebar on the left. Take a screenshot to verify the sidebar appeared with sections: Volume, Pitch, Duration, Logic, Groove, Audible.
23. Click the **Logic** section header (labeled "v Logic" or "> Logic") to expand it. Take a screenshot — you should see a "Logic:" dropdown showing "None".
24. Click the **Logic dropdown** button (the row showing "Logic: None") to open the dropdown menu.
25. Take a screenshot. You should see dropdown items: None, Trigger Every N, Skip Every N, Probability, If Prev Skipped, If Prev Triggered. Click **"Skip Every N"**.
26. Take a screenshot. The Logic section should now show "Logic: Skip Every N" with an "N:" parameter and -/+ buttons. The N value should default to **2**.
27. Now verify the **drum row prediction changed**: look at the instrument row in the drum pane that corresponds to this node's instrument. The step pattern (colored bars in the timeline) should have changed — with Skip Every 2, roughly every other occurrence of this node should now be empty. Take a screenshot and note whether the pattern visibly differs from before.
28. Click the Logic dropdown again, and select **"None"** to restore the original logic. Verify the row pattern returns to its original state.
29. Close the sidebar by clicking the **X** button in the sidebar header (top-right corner), or click outside the sidebar area. Verify it closed.

### Row management
30. Use `click_ui button="addRow"` to add a new row. Verify the row count is now **8** (check State line).
31. Use `click_ui button="delete" row=7` to delete the new row. Verify the row count returned to **7**.

**End of phase: Report at least 1 `ISSUE: P3 Suggestion` about the instrument selector, color wheel, FX panel, EQ, or row management.**

## Phase 6 — UI Critique

Dedicate this phase to evaluating the overall user experience. Take screenshots and be thorough.

1. Take a screenshot of the full UI. Evaluate overall layout: pane balance, button sizes, text readability, visual hierarchy, use of whitespace. Report **2+ suggestions**.
2. Scroll to the bottom rows. Evaluate row controls alignment, scrollbar visibility, button clarity, label truncation (especially long names like "Fm-epiano-1"). Report **1+ suggestion**.
3. Look at the EQ panel. Evaluate slider distinguishability, label clarity, band spacing. Report **1+ suggestion**.
4. Start playback with `click_ui button="play"`. Watch for 2–3 seconds. Evaluate animation quality, highlight visibility, playhead feedback. Stop playback with `click_ui button="stop"`. Report **1+ suggestion**.

You must report at least **5 specific, actionable** `ISSUE: P3 Suggestion - ...` lines in this phase.

Beyond the structured checklist above, freely report any additional UX issues, missing features, confusing behaviors, or visual polish opportunities you notice. Be harsh and thorough — pretend you are a professional UX auditor reviewing this app for the first time. Consider:
- Does row deletion lack a confirmation dialog? (It does — is that safe?)
- Are label areas wide enough for long instrument names?
- Are button icons/labels self-explanatory for first-time users?
- Is there visual feedback when toggling mute/solo?
- Could any controls benefit from tooltips or hover states?

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
(list every issue you reported during testing, one per line — at least 5 UI suggestions required)

### OVERALL RATING
<1-10> — <explanation of rating>
```

Use the exact `ISSUE:` prefix format for every issue line so they can be parsed automatically.
