---
name: play_stop_basic
platform: desktop
tags: [smoke, transport]
fixture: simple_loop.json
max_iterations: 15
---

# Play/Stop Basic Flow

Verify that the Play and Stop transport controls work correctly.

## Steps
1. Look at the drum view at the bottom. Find the Play button (triangle icon).
2. Click the Play button.
3. Wait 3 seconds. Observe:
   - Animated highlights should traverse the nodes in the grid
   - The drum timeline should show a moving playhead
4. Click the Stop button (square icon).
5. Wait 1 second. Verify:
   - All animation has stopped
   - Playhead is stationary
   - No visual glitches or artifacts

## Expected Outcomes
- Play starts visible animation within 1 second of clicking
- Stop halts all animation within 1 second of clicking
- No rendering artifacts, overlapping elements, or broken UI
- Buttons provide clear visual feedback (pressed state)
