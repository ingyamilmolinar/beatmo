---
name: tap_play_stop
platform: mobile
device: iPhone 12 landscape
tags: [smoke, transport, touch]
fixture: simple_loop.json
max_iterations: 15
---

# Tap Play/Stop on Mobile

Test basic play/stop via touch taps on mobile.

## Steps
1. Look at the drum view at the bottom of the screen. Find the Play button (triangle icon).
   Note: on mobile, touch targets may be larger than desktop.
2. Tap the Play button.
3. Wait 3 seconds. Observe:
   - Animated highlights should traverse the nodes in the grid
   - The drum timeline should show a moving playhead
4. Tap the Stop button (square icon).
5. Wait 1 second. Verify:
   - All animation has stopped
   - Playhead is stationary
   - No visual glitches

## Expected Outcomes
- Play starts animation within 1 second of tapping
- Stop halts animation within 1 second of tapping
- Touch targets are large enough to tap accurately
- No rendering artifacts or broken layout on mobile viewport
- UI elements are properly sized for mobile screen
