---
name: mobile_audio_playback
platform: mobile
device: iPhone 12 landscape
tags: [smoke, transport, touch]
fixture: multi_row.json
max_iterations: 20
---

# Mobile Audio Playback with Multiple Rows

Test playback on mobile with a multi-row circuit (kick, snare, hihat).

## Steps
1. Observe the drum view. There should be multiple drum rows with different instruments
   (Kick, Snare, HiHat).
2. Verify each row has a visible instrument label and controls.
3. Tap the Play button.
4. Wait 3 seconds. Observe:
   - Multiple rows should show activity/highlights
   - The grid should show node highlights traversing the circuits
   - Each instrument row should show independent timeline progression
5. Tap the Stop button.
6. Wait 1 second. Verify:
   - All animation has stopped across all rows
   - No visual glitches or stuck highlights
7. Check the overall layout:
   - Drum rows should not overlap
   - Labels should be readable
   - Controls should be accessible (not cut off by screen edges)

## Expected Outcomes
- All three instrument rows are visible and properly laid out
- Playback shows activity in multiple rows simultaneously
- Stop halts all rows cleanly
- Mobile layout does not cut off any drum row controls
- No overlapping UI elements or misaligned text
- The grid pane and drum pane both fit in the mobile viewport
