---
name: bpm_stress
platform: desktop
tags: [stress, transport]
fixture: simple_loop.json
max_iterations: 25
---

# BPM Stress Test

Rapidly change BPM while playing to check for visual stability.

## Steps
1. Click the Play button to start playback.
2. Wait 2 seconds to confirm animation is running smoothly.
3. Click the BPM + button 10 times rapidly to increase tempo.
4. Wait 2 seconds. Observe:
   - Animation should be noticeably faster
   - No visual corruption, flickering, or freezing
5. Click the BPM - button 15 times rapidly to decrease tempo below original.
6. Wait 2 seconds. Observe:
   - Animation should be noticeably slower
   - Grid and drum views still render correctly
7. Click BPM + button 5 times to return to a moderate tempo.
8. Wait 2 seconds for stable playback.
9. Click the Stop button.
10. Verify the UI is clean and responsive after stopping.

## Expected Outcomes
- BPM changes are reflected in animation speed immediately
- No visual artifacts during rapid BPM changes
- No frozen or stuck animation frames
- UI remains responsive throughout the stress test
- Clean stop after BPM stress
