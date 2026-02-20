---
name: row_add_during_playback
platform: desktop
tags: [regression, workflow]
fixture: simple_loop.json
max_iterations: 30
---

# Add Row During Playback

Test adding a new drum row while the sequencer is playing.

## Steps
1. Note the current number of drum rows in the drum view.
2. Click the Play button to start playback.
3. Wait 2 seconds to confirm animation is running.
4. Look for an "Add Row" button (often a "+" button) in the drum view area.
5. Click the Add Row button.
6. Observe:
   - A new drum row should appear in the drum view
   - Playback should continue without interruption
   - The new row may have a default instrument assigned
7. Wait 2 seconds to confirm playback is still smooth.
8. Click the Stop button.
9. Verify the new row persists after stopping.

## Expected Outcomes
- New drum row appears immediately when added
- Existing playback continues without glitches
- The drum view layout adjusts to accommodate the new row
- No visual artifacts or overlapping elements
- The new row integrates smoothly with existing rows
- Stop works cleanly after row addition
